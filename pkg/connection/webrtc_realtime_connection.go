// Package connection provides WebRTC connection implementations for Realtime API.
package connection

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hraban/opus"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi/events"
	"github.com/realtime-ai/realtime-ai/pkg/utils"
)

// WebRTCRealtimeEventHandler handles events from WebRTC Realtime connection.
type WebRTCRealtimeEventHandler interface {
	// OnConnectionStateChange is called when WebRTC connection state changes.
	OnConnectionStateChange(state webrtc.PeerConnectionState)

	// OnAudioReceived is called when audio is received via RTP.
	// Data is PCM audio (int16 samples, little-endian).
	OnAudioReceived(data []byte, sampleRate, channels int, timestamp time.Time)

	// OnClientEvent is called when a Realtime API event is received via DataChannel.
	OnClientEvent(event events.ClientEvent)

	// OnError is called when an error occurs.
	OnError(err error)
}

// NoOpWebRTCRealtimeEventHandler is a no-op implementation of WebRTCRealtimeEventHandler.
type NoOpWebRTCRealtimeEventHandler struct{}

func (h *NoOpWebRTCRealtimeEventHandler) OnConnectionStateChange(state webrtc.PeerConnectionState) {
}
func (h *NoOpWebRTCRealtimeEventHandler) OnAudioReceived(data []byte, sampleRate, channels int, timestamp time.Time) {
}
func (h *NoOpWebRTCRealtimeEventHandler) OnClientEvent(event events.ClientEvent) {}
func (h *NoOpWebRTCRealtimeEventHandler) OnError(err error)                      {}

// WebRTCRealtimeConnection combines WebRTC media transport with Realtime API signaling.
// Audio is transmitted via RTP tracks, signaling via DataChannel.
type WebRTCRealtimeConnection interface {
	// PeerID returns the unique identifier for this connection.
	PeerID() string

	// SessionID returns the session ID for this connection.
	SessionID() string

	// RegisterEventHandler registers the event handler.
	RegisterEventHandler(handler WebRTCRealtimeEventHandler)

	// SendEvent sends a Realtime API server event via DataChannel.
	SendEvent(event events.ServerEvent) error

	// SendAudio sends audio data via RTP track.
	// Data should be PCM audio (int16 samples, little-endian).
	SendAudio(data []byte, sampleRate, channels int) error

	// Start initializes the connection handlers.
	Start(ctx context.Context) error

	// Close closes the connection.
	Close() error

	// SupportsRTPAudio returns true (always supports RTP audio).
	SupportsRTPAudio() bool
}

// webrtcRealtimeConnectionImpl implements WebRTCRealtimeConnection.
type webrtcRealtimeConnectionImpl struct {
	peerID    string
	sessionID string

	// WebRTC core
	pc          *webrtc.PeerConnection
	dataChannel *webrtc.DataChannel

	// Audio tracks
	localAudioTrack  *webrtc.TrackLocalStaticSample
	remoteAudioTrack *webrtc.TrackRemote

	// Audio codecs
	audioEncoder *opus.Encoder
	audioDecoder *opus.Decoder

	// Event handler
	handler WebRTCRealtimeEventHandler

	// Synchronization
	mu     sync.RWMutex
	once   sync.Once
	closed bool
}

// NewWebRTCRealtimeConnection creates a new WebRTC Realtime connection.
func NewWebRTCRealtimeConnection(pc *webrtc.PeerConnection) (WebRTCRealtimeConnection, error) {
	peerID := uuid.New().String()[:8]
	sessionID := "sess_" + uuid.New().String()[:12]

	// Create Opus encoder for audio output (48kHz mono)
	audioEncoder, err := opus.NewEncoder(48000, 1, opus.AppVoIP)
	if err != nil {
		return nil, err
	}
	audioEncoder.SetBitrate(50000)
	audioEncoder.SetComplexity(10)
	audioEncoder.SetDTX(true)

	// Create Opus decoder for audio input (48kHz mono)
	audioDecoder, err := opus.NewDecoder(48000, 1)
	if err != nil {
		return nil, err
	}

	return &webrtcRealtimeConnectionImpl{
		peerID:       peerID,
		sessionID:    sessionID,
		pc:           pc,
		audioEncoder: audioEncoder,
		audioDecoder: audioDecoder,
		handler:      &NoOpWebRTCRealtimeEventHandler{},
	}, nil
}

func (c *webrtcRealtimeConnectionImpl) PeerID() string {
	return c.peerID
}

func (c *webrtcRealtimeConnectionImpl) SessionID() string {
	return c.sessionID
}

func (c *webrtcRealtimeConnectionImpl) RegisterEventHandler(handler WebRTCRealtimeEventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = handler
}

// Start initializes WebRTC handlers and starts processing.
func (c *webrtcRealtimeConnectionImpl) Start(ctx context.Context) error {
	// Handle connection state changes
	c.pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		c.handler.OnConnectionStateChange(state)
	})

	// Handle incoming DataChannel (for Realtime API events)
	c.pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		c.mu.Lock()
		c.dataChannel = dc
		c.mu.Unlock()

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			c.handleDataChannelMessage(msg.Data)
		})

		dc.OnOpen(func() {
			log.Printf("[webrtc-realtime %s] DataChannel opened", c.sessionID)
		})
	})

	// Create local audio track explicitly for sending audio
	localAudioTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"audio",
		"realtime-audio-"+c.sessionID,
	)
	if err != nil {
		return fmt.Errorf("failed to create local audio track: %w", err)
	}
	c.localAudioTrack = localAudioTrack

	// Add the track to peer connection
	_, err = c.pc.AddTrack(localAudioTrack)
	if err != nil {
		return fmt.Errorf("failed to add audio track: %w", err)
	}

	// Handle incoming audio track
	c.pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("[webrtc-realtime %s] OnTrack: %v, codec: %v", c.sessionID, track.ID(), track.Codec().MimeType)
		if track.Kind() == webrtc.RTPCodecTypeAudio {
			c.mu.Lock()
			c.remoteAudioTrack = track
			c.mu.Unlock()

			go c.readRemoteAudio(ctx)
		}
	})

	return nil
}

// handleDataChannelMessage parses and dispatches Realtime API events.
func (c *webrtcRealtimeConnectionImpl) handleDataChannelMessage(data []byte) {
	event, err := events.ParseClientEvent(data)
	if err != nil {
		log.Printf("[webrtc-realtime %s] failed to parse client event: %v", c.sessionID, err)
		c.handler.OnError(err)
		return
	}

	c.handler.OnClientEvent(event)
}

// readRemoteAudio reads and decodes audio from the remote RTP track.
func (c *webrtcRealtimeConnectionImpl) readRemoteAudio(ctx context.Context) {
	pcmBuf := make([]int16, 1920) // 20ms at 48kHz mono

	for {
		select {
		case <-ctx.Done():
			return
		default:
			c.mu.RLock()
			track := c.remoteAudioTrack
			closed := c.closed
			c.mu.RUnlock()

			if closed || track == nil {
				return
			}

			rtpPacket, _, err := track.ReadRTP()
			if err != nil {
				log.Printf("[webrtc-realtime %s] RTP read error: %v", c.sessionID, err)
				continue
			}

			// Decode Opus to PCM
			n, err := c.audioDecoder.Decode(rtpPacket.Payload, pcmBuf)
			if err != nil {
				log.Printf("[webrtc-realtime %s] Opus decode error: %v", c.sessionID, err)
				continue
			}

			// Convert int16 to bytes
			audioData := utils.Int16SliceToByteSlice(pcmBuf[:n])

			// Notify handler
			c.handler.OnAudioReceived(audioData, 48000, 1, time.Now())
		}
	}
}

// SendEvent sends a Realtime API server event via DataChannel.
func (c *webrtcRealtimeConnectionImpl) SendEvent(event events.ServerEvent) error {
	c.mu.RLock()
	dc := c.dataChannel
	closed := c.closed
	c.mu.RUnlock()

	if closed {
		return nil
	}

	if dc == nil || dc.ReadyState() != webrtc.DataChannelStateOpen {
		return nil
	}

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return dc.Send(data)
}

// SendAudio sends PCM audio via RTP track (encoded as Opus).
func (c *webrtcRealtimeConnectionImpl) SendAudio(data []byte, sampleRate, channels int) error {
	c.mu.RLock()
	track := c.localAudioTrack
	closed := c.closed
	c.mu.RUnlock()

	if closed || track == nil {
		return nil
	}

	// Convert bytes to int16 samples
	samples := utils.ByteSliceToInt16Slice(data)

	// Encode to Opus (20ms frames at 48kHz = 960 samples)
	frameSize := 960
	opusBuf := make([]byte, 1275) // Max Opus frame size

	// Process audio in frames
	for offset := 0; offset+frameSize <= len(samples); offset += frameSize {
		frame := samples[offset : offset+frameSize]
		n, err := c.audioEncoder.Encode(frame, opusBuf)
		if err != nil {
			log.Printf("[webrtc-realtime %s] Opus encode error: %v", c.sessionID, err)
			continue
		}

		// Write to RTP track
		if err := track.WriteSample(media.Sample{
			Data:     opusBuf[:n],
			Duration: 20 * time.Millisecond,
		}); err != nil {
			return err
		}
	}

	return nil
}

// SupportsRTPAudio returns true since this connection sends audio via RTP.
func (c *webrtcRealtimeConnectionImpl) SupportsRTPAudio() bool {
	return true
}

// Close closes the WebRTC connection.
func (c *webrtcRealtimeConnectionImpl) Close() error {
	c.once.Do(func() {
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()

		if c.pc != nil {
			c.pc.Close()
		}
	})
	return nil
}
