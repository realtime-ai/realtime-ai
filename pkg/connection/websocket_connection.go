package connection

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

const (
	DefaultWSWriteWait  = 10 * time.Second
	DefaultWSPongWait   = 60 * time.Second
	DefaultWSPingPeriod = 54 * time.Second // Must be less than pongWait
)

// WebSocketConfig holds configuration for WebSocket connection.
type WebSocketConfig struct {
	SampleRate int
	Channels   int
	WriteWait  time.Duration
	PongWait   time.Duration
	PingPeriod time.Duration
}

// DefaultWebSocketConfig returns the default WebSocket configuration.
func DefaultWebSocketConfig() WebSocketConfig {
	return WebSocketConfig{
		SampleRate: 48000,
		Channels:   1,
		WriteWait:  DefaultWSWriteWait,
		PongWait:   DefaultWSPongWait,
		PingPeriod: DefaultWSPingPeriod,
	}
}

// WSMessage represents the JSON message structure for WebSocket communication.
type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// WSAudioPayload represents audio data in WebSocket messages.
type WSAudioPayload struct {
	Data       string `json:"data"`        // Base64 encoded audio data
	SampleRate int    `json:"sample_rate"` // Sample rate in Hz
	Channels   int    `json:"channels"`    // Number of channels
	MediaType  string `json:"media_type"`  // e.g., pipeline.AudioMediaTypeRaw
}

type websocketConnection struct {
	peerID string
	conn   *websocket.Conn

	// Event handler
	handler ConnectionEventHandler

	// Audio parameters
	sampleRate int
	channels   int

	// Timing parameters
	writeWait  time.Duration
	pongWait   time.Duration
	pingPeriod time.Duration

	// Output channel for async writes
	outChan chan *pipeline.PipelineMessage

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once
	mu     sync.RWMutex
	closed bool
}

var _ Connection = (*websocketConnection)(nil)

// NewWebSocketConnection creates a new WebSocket connection with default config.
func NewWebSocketConnection(peerID string, conn *websocket.Conn) Connection {
	return NewWebSocketConnectionWithConfig(peerID, conn, DefaultWebSocketConfig())
}

// NewWebSocketConnectionWithConfig creates a new WebSocket connection with custom config.
func NewWebSocketConnectionWithConfig(peerID string, conn *websocket.Conn, cfg WebSocketConfig) Connection {
	ctx, cancel := context.WithCancel(context.Background())

	ws := &websocketConnection{
		peerID:     peerID,
		conn:       conn,
		handler:    &NoOpConnectionEventHandler{},
		sampleRate: cfg.SampleRate,
		channels:   cfg.Channels,
		writeWait:  cfg.WriteWait,
		pongWait:   cfg.PongWait,
		pingPeriod: cfg.PingPeriod,
		outChan:    make(chan *pipeline.PipelineMessage, 50),
		ctx:        ctx,
		cancel:     cancel,
	}

	ws.start()

	return ws
}

func (w *websocketConnection) PeerID() string {
	return w.peerID
}

func (w *websocketConnection) RegisterEventHandler(handler ConnectionEventHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handler = handler
}

func (w *websocketConnection) start() {
	// Notify connected state
	w.mu.RLock()
	handler := w.handler
	w.mu.RUnlock()
	handler.OnConnectionStateChange(ConnectionStateConnected)

	// Set up pong handler
	w.conn.SetReadDeadline(time.Now().Add(w.pongWait))
	w.conn.SetPongHandler(func(string) error {
		w.conn.SetReadDeadline(time.Now().Add(w.pongWait))
		return nil
	})

	// Start read pump
	w.wg.Add(1)
	go w.readPump()

	// Start write pump
	w.wg.Add(1)
	go w.writePump()

	// Start ping ticker
	w.wg.Add(1)
	go w.pingPump()
}

func (w *websocketConnection) readPump() {
	defer w.wg.Done()
	defer w.Close()

	for {
		select {
		case <-w.ctx.Done():
			return
		default:
			_, message, err := w.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
					log.Printf("[websocket %s] read error: %v", w.peerID, err)
					w.mu.RLock()
					handler := w.handler
					w.mu.RUnlock()
					handler.OnError(err)
				}
				return
			}

			w.handleMessage(message)
		}
	}
}

func (w *websocketConnection) handleMessage(data []byte) {
	var wsMsg WSMessage
	if err := json.Unmarshal(data, &wsMsg); err != nil {
		log.Printf("[websocket %s] failed to unmarshal message: %v", w.peerID, err)
		return
	}

	w.mu.RLock()
	handler := w.handler
	w.mu.RUnlock()

	switch wsMsg.Type {
	case "audio":
		var audioPayload WSAudioPayload
		if err := json.Unmarshal(wsMsg.Payload, &audioPayload); err != nil {
			// Try legacy format (raw base64 or byte array)
			var audioData []byte
			if err := json.Unmarshal(wsMsg.Payload, &audioData); err != nil {
				log.Printf("[websocket %s] failed to unmarshal audio data: %v", w.peerID, err)
				return
			}
			msg := &pipeline.PipelineMessage{
				Type: pipeline.MsgTypeAudio,
				AudioData: &pipeline.AudioData{
					Data:       audioData,
					SampleRate: w.sampleRate,
					Channels:   w.channels,
					MediaType:  pipeline.AudioMediaTypeRaw,
					Timestamp:  time.Now(),
				},
			}
			handler.OnMessage(msg)
			return
		}

		// Decode base64 audio data
		audioData, err := base64.StdEncoding.DecodeString(audioPayload.Data)
		if err != nil {
			log.Printf("[websocket %s] failed to decode base64 audio: %v", w.peerID, err)
			return
		}

		sampleRate := audioPayload.SampleRate
		if sampleRate == 0 {
			sampleRate = w.sampleRate
		}
		channels := audioPayload.Channels
		if channels == 0 {
			channels = w.channels
		}

		// Convert string MediaType to AudioMediaType
		mediaType := pipeline.AudioMediaType(audioPayload.MediaType)
		if mediaType == "" {
			mediaType = pipeline.AudioMediaTypeRaw
		}

		msg := &pipeline.PipelineMessage{
			Type: pipeline.MsgTypeAudio,
			AudioData: &pipeline.AudioData{
				Data:       audioData,
				SampleRate: sampleRate,
				Channels:   channels,
				MediaType:  mediaType,
				Timestamp:  time.Now(),
			},
		}
		handler.OnMessage(msg)

	case "text":
		var textData string
		if err := json.Unmarshal(wsMsg.Payload, &textData); err != nil {
			log.Printf("[websocket %s] failed to unmarshal text data: %v", w.peerID, err)
			return
		}

		msg := &pipeline.PipelineMessage{
			Type: pipeline.MsgTypeData,
			TextData: &pipeline.TextData{
				Data:      []byte(textData),
				TextType:  "text",
				Timestamp: time.Now(),
			},
		}
		handler.OnMessage(msg)

	default:
		log.Printf("[websocket %s] unknown message type: %s", w.peerID, wsMsg.Type)
	}
}

func (w *websocketConnection) writePump() {
	defer w.wg.Done()

	for {
		select {
		case <-w.ctx.Done():
			return
		case msg, ok := <-w.outChan:
			if !ok {
				return
			}
			w.writeMessage(msg)
		}
	}
}

func (w *websocketConnection) writeMessage(msg *pipeline.PipelineMessage) {
	w.conn.SetWriteDeadline(time.Now().Add(w.writeWait))

	var wsMsg WSMessage

	switch msg.Type {
	case pipeline.MsgTypeAudio:
		if msg.AudioData == nil {
			return
		}
	wsMsg.Type = "audio"
		payload := WSAudioPayload{
			Data:       base64.StdEncoding.EncodeToString(msg.AudioData.Data),
			SampleRate: msg.AudioData.SampleRate,
			Channels:   msg.AudioData.Channels,
			MediaType:  string(msg.AudioData.MediaType),
		}
		wsMsg.Payload, _ = json.Marshal(payload)

	case pipeline.MsgTypeData:
		if msg.TextData == nil {
			return
		}
		wsMsg.Type = "text"
		wsMsg.Payload, _ = json.Marshal(string(msg.TextData.Data))

	default:
		return
	}

	if err := w.conn.WriteJSON(wsMsg); err != nil {
		log.Printf("[websocket %s] write error: %v", w.peerID, err)
		w.mu.RLock()
		handler := w.handler
		w.mu.RUnlock()
		handler.OnError(err)
	}
}

func (w *websocketConnection) pingPump() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.conn.SetWriteDeadline(time.Now().Add(w.writeWait))
			if err := w.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("[websocket %s] ping error: %v", w.peerID, err)
				return
			}
		}
	}
}

func (w *websocketConnection) SendMessage(msg *pipeline.PipelineMessage) {
	w.mu.RLock()
	closed := w.closed
	w.mu.RUnlock()

	if closed {
		return
	}

	select {
	case <-w.ctx.Done():
		return
	case w.outChan <- msg:
	default:
		log.Printf("[websocket %s] outChan is full, dropping message", w.peerID)
	}
}

func (w *websocketConnection) Close() error {
	w.once.Do(func() {
		w.mu.Lock()
		w.closed = true
		w.mu.Unlock()

		// Notify disconnected state
		w.mu.RLock()
		handler := w.handler
		w.mu.RUnlock()
		handler.OnConnectionStateChange(ConnectionStateClosed)

		// Cancel context and wait for goroutines
		w.cancel()

		// Close the WebSocket connection with a proper close message
		w.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		w.conn.Close()

		// Close output channel
		close(w.outChan)

		w.wg.Wait()
	})
	return nil
}
