// Package connection provides connection abstractions for different transports.
//
// TwilioConnection implements the Connection interface for Twilio Media Streams.
// It handles bidirectional audio streaming with Twilio's WebSocket protocol.
//
// Features:
//   - Twilio Media Streams WebSocket protocol handling
//   - μ-law (8kHz) ↔ PCM (16kHz) audio conversion
//   - Bidirectional audio support
//   - DTMF event handling
//   - Mark events for audio synchronization
//
// Audio Format:
//   - Twilio: μ-law, 8kHz, mono
//   - Pipeline: PCM, 16kHz, mono (configurable)
//
// Reference: https://www.twilio.com/docs/voice/media-streams

package connection

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/gorilla/websocket"
	"github.com/realtime-ai/realtime-ai/pkg/audio"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

// Twilio Media Streams constants
const (
	TwilioInputSampleRate  = 8000  // Twilio sends μ-law at 8kHz
	TwilioOutputSampleRate = 8000  // Twilio expects μ-law at 8kHz
	TwilioChannels         = 1     // Mono only
	PipelineSampleRate     = 16000 // Pipeline uses 16kHz PCM
)

// TwilioConnection implements Connection interface for Twilio Media Streams.
type TwilioConnection struct {
	conn     *websocket.Conn
	peerID   string
	handler  ConnectionEventHandler
	handlers []ConnectionEventHandler

	// Twilio stream metadata
	streamSid      string
	callSid        string
	accountSid     string
	sequenceNumber int64

	// Audio resampler
	resampler8to16 *audio.Resample
	resampler16to8 *audio.Resample

	// I/O channels
	inChan  chan *pipeline.PipelineMessage
	outChan chan *pipeline.PipelineMessage

	// State management
	state    ConnectionState
	stateMu  sync.RWMutex
	closed   atomic.Bool
	closeMu  sync.Mutex
	closeWg  sync.WaitGroup

	// WebSocket write mutex (gorilla/websocket requires synchronized writes)
	writeMu sync.Mutex

	// Mark tracking for audio synchronization
	markCounter int64
	markChan    chan string
}

// TwilioMediaMessage represents a Twilio Media Streams WebSocket message.
type TwilioMediaMessage struct {
	Event          string              `json:"event"`
	SequenceNumber string              `json:"sequenceNumber,omitempty"`
	StreamSid      string              `json:"streamSid,omitempty"`
	Protocol       string              `json:"protocol,omitempty"`
	Version        string              `json:"version,omitempty"`
	Start          *TwilioStartPayload `json:"start,omitempty"`
	Media          *TwilioMediaPayload `json:"media,omitempty"`
	Stop           *TwilioStopPayload  `json:"stop,omitempty"`
	Mark           *TwilioMarkPayload  `json:"mark,omitempty"`
	DTMF           *TwilioDTMFPayload  `json:"dtmf,omitempty"`
}

// TwilioStartPayload contains stream initialization data.
type TwilioStartPayload struct {
	AccountSid       string                 `json:"accountSid"`
	StreamSid        string                 `json:"streamSid"`
	CallSid          string                 `json:"callSid"`
	Tracks           []string               `json:"tracks"`
	MediaFormat      TwilioMediaFormat      `json:"mediaFormat"`
	CustomParameters map[string]string      `json:"customParameters,omitempty"`
}

// TwilioMediaFormat describes the audio format.
type TwilioMediaFormat struct {
	Encoding   string `json:"encoding"`   // "audio/x-mulaw"
	SampleRate int    `json:"sampleRate"` // 8000
	Channels   int    `json:"channels"`   // 1
}

// TwilioMediaPayload contains audio data.
type TwilioMediaPayload struct {
	Track     string `json:"track,omitempty"`   // "inbound" or "outbound"
	Chunk     string `json:"chunk,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	Payload   string `json:"payload"` // Base64 encoded μ-law audio
}

// TwilioStopPayload contains stream termination data.
type TwilioStopPayload struct {
	AccountSid string `json:"accountSid"`
	CallSid    string `json:"callSid"`
}

// TwilioMarkPayload contains mark event data.
type TwilioMarkPayload struct {
	Name string `json:"name"`
}

// TwilioDTMFPayload contains DTMF digit data.
type TwilioDTMFPayload struct {
	Track string `json:"track"`
	Digit string `json:"digit"`
}

// NewTwilioConnection creates a new Twilio Media Streams connection.
func NewTwilioConnection(conn *websocket.Conn) (*TwilioConnection, error) {
	// Create resamplers for audio format conversion (mono channel layout)
	resampler8to16, err := audio.NewResample(TwilioInputSampleRate, PipelineSampleRate,
		astiav.ChannelLayoutMono, astiav.ChannelLayoutMono)
	if err != nil {
		return nil, err
	}

	resampler16to8, err := audio.NewResample(PipelineSampleRate, TwilioOutputSampleRate,
		astiav.ChannelLayoutMono, astiav.ChannelLayoutMono)
	if err != nil {
		resampler8to16.Free()
		return nil, err
	}

	tc := &TwilioConnection{
		conn:           conn,
		peerID:         "twilio-pending", // Will be set from Start message
		inChan:         make(chan *pipeline.PipelineMessage, 100),
		outChan:        make(chan *pipeline.PipelineMessage, 100),
		resampler8to16: resampler8to16,
		resampler16to8: resampler16to8,
		state:          ConnectionStateNew,
		markChan:       make(chan string, 10),
	}

	return tc, nil
}

// PeerID returns the connection identifier.
func (tc *TwilioConnection) PeerID() string {
	return tc.peerID
}

// StreamSid returns the Twilio stream SID.
func (tc *TwilioConnection) StreamSid() string {
	return tc.streamSid
}

// CallSid returns the Twilio call SID.
func (tc *TwilioConnection) CallSid() string {
	return tc.callSid
}

// RegisterEventHandler registers a single event handler.
func (tc *TwilioConnection) RegisterEventHandler(handler ConnectionEventHandler) {
	tc.handler = handler
	tc.handlers = append(tc.handlers, handler)
}

// SendMessage sends a pipeline message (audio) to Twilio.
func (tc *TwilioConnection) SendMessage(msg *pipeline.PipelineMessage) {
	if tc.closed.Load() {
		return
	}

	select {
	case tc.outChan <- msg:
	default:
		log.Printf("[TwilioConn] Output channel full, dropping message")
	}
}

// In returns the input channel for receiving messages from Twilio.
func (tc *TwilioConnection) In() <-chan *pipeline.PipelineMessage {
	return tc.inChan
}

// Close closes the connection.
func (tc *TwilioConnection) Close() error {
	tc.closeMu.Lock()
	defer tc.closeMu.Unlock()

	if tc.closed.Load() {
		return nil
	}
	tc.closed.Store(true)

	log.Printf("[TwilioConn] Closing connection for stream %s", tc.streamSid)

	// Close WebSocket first to trigger goroutine exits
	if tc.conn != nil {
		tc.conn.Close()
	}

	// Close channels
	close(tc.inChan)
	close(tc.outChan)
	close(tc.markChan)

	// Wait for goroutines to exit BEFORE freeing resamplers
	tc.closeWg.Wait()

	// Clean up resamplers (safe now that goroutines have exited)
	if tc.resampler8to16 != nil {
		tc.resampler8to16.Free()
	}
	if tc.resampler16to8 != nil {
		tc.resampler16to8.Free()
	}

	// Notify handlers
	tc.setState(ConnectionStateClosed)

	return nil
}

// Start begins processing the WebSocket connection.
func (tc *TwilioConnection) Start() {
	tc.setState(ConnectionStateConnecting)

	// Start read pump
	tc.closeWg.Add(1)
	go tc.readPump()

	// Start write pump
	tc.closeWg.Add(1)
	go tc.writePump()
}

// readPump reads messages from Twilio WebSocket.
func (tc *TwilioConnection) readPump() {
	defer tc.closeWg.Done()
	defer func() {
		tc.Close()
	}()

	for {
		if tc.closed.Load() {
			return
		}

		_, message, err := tc.conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("[TwilioConn] Read error: %v", err)
				tc.notifyError(err)
			}
			return
		}

		var msg TwilioMediaMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("[TwilioConn] Failed to parse message: %v", err)
			continue
		}

		tc.handleMessage(&msg)
	}
}

// writePump sends messages to Twilio WebSocket.
func (tc *TwilioConnection) writePump() {
	defer tc.closeWg.Done()

	for {
		select {
		case msg, ok := <-tc.outChan:
			if !ok {
				return
			}

			if msg.Type == pipeline.MsgTypeAudio && msg.AudioData != nil {
				tc.sendAudioToTwilio(msg.AudioData)
			}
		}
	}
}

// handleMessage processes incoming Twilio messages.
func (tc *TwilioConnection) handleMessage(msg *TwilioMediaMessage) {
	switch msg.Event {
	case "connected":
		log.Printf("[TwilioConn] Connected to Twilio Media Streams (protocol: %s, version: %s)",
			msg.Protocol, msg.Version)

	case "start":
		tc.handleStart(msg)

	case "media":
		tc.handleMedia(msg)

	case "stop":
		tc.handleStop(msg)

	case "mark":
		tc.handleMark(msg)

	case "dtmf":
		tc.handleDTMF(msg)

	default:
		log.Printf("[TwilioConn] Unknown event: %s", msg.Event)
	}
}

// handleStart processes the start event.
func (tc *TwilioConnection) handleStart(msg *TwilioMediaMessage) {
	if msg.Start == nil {
		log.Printf("[TwilioConn] Start event missing payload")
		return
	}

	tc.streamSid = msg.Start.StreamSid
	tc.callSid = msg.Start.CallSid
	tc.accountSid = msg.Start.AccountSid
	tc.peerID = tc.callSid

	log.Printf("[TwilioConn] Stream started - StreamSid: %s, CallSid: %s, Tracks: %v",
		tc.streamSid, tc.callSid, msg.Start.Tracks)
	log.Printf("[TwilioConn] Media format: %s, %dHz, %d channel(s)",
		msg.Start.MediaFormat.Encoding,
		msg.Start.MediaFormat.SampleRate,
		msg.Start.MediaFormat.Channels)

	if len(msg.Start.CustomParameters) > 0 {
		log.Printf("[TwilioConn] Custom parameters: %v", msg.Start.CustomParameters)
	}

	tc.setState(ConnectionStateConnected)
}

// handleMedia processes incoming audio data.
func (tc *TwilioConnection) handleMedia(msg *TwilioMediaMessage) {
	if msg.Media == nil || msg.Media.Payload == "" {
		return
	}

	// Only process inbound track
	if msg.Media.Track != "" && msg.Media.Track != "inbound" {
		return
	}

	// Decode base64 μ-law audio
	mulawData, err := base64.StdEncoding.DecodeString(msg.Media.Payload)
	if err != nil {
		log.Printf("[TwilioConn] Failed to decode audio: %v", err)
		return
	}

	// Convert μ-law to PCM
	pcmData := audio.MuLawToPCM(mulawData)

	// Resample 8kHz → 16kHz
	pcm16kData, err := tc.resampler8to16.Resample(pcmData)
	if err != nil {
		log.Printf("[TwilioConn] Failed to resample audio: %v", err)
		return
	}

	// Create pipeline message
	pipelineMsg := &pipeline.PipelineMessage{
		Type:      pipeline.MsgTypeAudio,
		SessionID: tc.callSid,
		Timestamp: time.Now(),
		AudioData: &pipeline.AudioData{
			Data:       pcm16kData,
			SampleRate: PipelineSampleRate,
			Channels:   TwilioChannels,
			MediaType:  pipeline.AudioMediaTypePCM,
		},
	}

	// Send to pipeline
	select {
	case tc.inChan <- pipelineMsg:
	default:
		log.Printf("[TwilioConn] Input channel full, dropping audio")
	}

	// Notify handler
	if tc.handler != nil {
		tc.handler.OnMessage(pipelineMsg)
	}
}

// handleStop processes the stop event.
func (tc *TwilioConnection) handleStop(msg *TwilioMediaMessage) {
	log.Printf("[TwilioConn] Stream stopped - CallSid: %s", tc.callSid)
	tc.setState(ConnectionStateDisconnected)
}

// handleMark processes mark events.
func (tc *TwilioConnection) handleMark(msg *TwilioMediaMessage) {
	if msg.Mark == nil {
		return
	}
	log.Printf("[TwilioConn] Mark received: %s", msg.Mark.Name)

	select {
	case tc.markChan <- msg.Mark.Name:
	default:
	}
}

// handleDTMF processes DTMF digit events.
func (tc *TwilioConnection) handleDTMF(msg *TwilioMediaMessage) {
	if msg.DTMF == nil {
		return
	}
	log.Printf("[TwilioConn] DTMF digit received: %s (track: %s)", msg.DTMF.Digit, msg.DTMF.Track)

	// Create text message for DTMF
	dtmfMsg := &pipeline.PipelineMessage{
		Type:      pipeline.MsgTypeData,
		SessionID: tc.callSid,
		Timestamp: time.Now(),
		TextData: &pipeline.TextData{
			Data:     []byte(msg.DTMF.Digit),
			TextType: "dtmf",
		},
	}

	if tc.handler != nil {
		tc.handler.OnMessage(dtmfMsg)
	}
}

// sendAudioToTwilio converts and sends audio to Twilio.
func (tc *TwilioConnection) sendAudioToTwilio(audioData *pipeline.AudioData) {
	if tc.streamSid == "" || tc.closed.Load() {
		return
	}

	// Get PCM data at pipeline sample rate
	pcmData := audioData.Data

	// Resample to 8kHz if needed
	if audioData.SampleRate != TwilioOutputSampleRate {
		var err error
		pcmData, err = tc.resampler16to8.Resample(pcmData)
		if err != nil {
			log.Printf("[TwilioConn] Failed to resample output audio: %v", err)
			return
		}
	}

	// Convert PCM to μ-law
	mulawData := audio.PCMToMuLaw(pcmData)

	// Encode to base64
	payload := base64.StdEncoding.EncodeToString(mulawData)

	// Create Twilio media message
	msg := TwilioMediaMessage{
		Event:     "media",
		StreamSid: tc.streamSid,
		Media: &TwilioMediaPayload{
			Payload: payload,
		},
	}

	// Send to Twilio (synchronized write)
	tc.writeMu.Lock()
	err := tc.conn.WriteJSON(msg)
	tc.writeMu.Unlock()
	if err != nil {
		log.Printf("[TwilioConn] Failed to send audio: %v", err)
	}
}

// SendMark sends a mark message to Twilio for audio synchronization.
func (tc *TwilioConnection) SendMark(name string) error {
	if tc.streamSid == "" || tc.closed.Load() {
		return nil
	}

	msg := TwilioMediaMessage{
		Event:     "mark",
		StreamSid: tc.streamSid,
		Mark: &TwilioMarkPayload{
			Name: name,
		},
	}

	tc.writeMu.Lock()
	defer tc.writeMu.Unlock()
	return tc.conn.WriteJSON(msg)
}

// ClearAudio sends a clear message to stop any buffered audio.
func (tc *TwilioConnection) ClearAudio() error {
	if tc.streamSid == "" || tc.closed.Load() {
		return nil
	}

	msg := TwilioMediaMessage{
		Event:     "clear",
		StreamSid: tc.streamSid,
	}

	log.Printf("[TwilioConn] Clearing audio buffer")
	tc.writeMu.Lock()
	defer tc.writeMu.Unlock()
	return tc.conn.WriteJSON(msg)
}

// WaitForMark waits for a specific mark to be returned.
func (tc *TwilioConnection) WaitForMark(name string, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case mark := <-tc.markChan:
			if mark == name {
				return true
			}
		case <-timer.C:
			return false
		}
	}
}

// setState updates the connection state and notifies handlers.
func (tc *TwilioConnection) setState(state ConnectionState) {
	tc.stateMu.Lock()
	tc.state = state
	tc.stateMu.Unlock()

	for _, h := range tc.handlers {
		h.OnConnectionStateChange(state)
	}
}

// notifyError notifies handlers of an error.
func (tc *TwilioConnection) notifyError(err error) {
	for _, h := range tc.handlers {
		h.OnError(err)
	}
}

// State returns the current connection state.
func (tc *TwilioConnection) State() ConnectionState {
	tc.stateMu.RLock()
	defer tc.stateMu.RUnlock()
	return tc.state
}

// Ensure TwilioConnection implements Connection interface.
var _ Connection = (*TwilioConnection)(nil)
