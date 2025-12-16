// Package realtimeapi provides transport abstractions for Realtime API connections.
package realtimeapi

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hraban/opus"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi/events"
)

// Transport abstracts the underlying transport for Realtime API connections.
// Implementations include WebSocket and WebRTC DataChannel.
type Transport interface {
	// SendEvent sends a server event to the client.
	SendEvent(event events.ServerEvent) error

	// Close closes the transport connection.
	Close() error
}

// AudioTransport extends Transport with RTP audio capabilities.
// Used for WebRTC-based connections where audio is sent via RTP tracks.
type AudioTransport interface {
	Transport

	// SendAudio sends audio data via RTP track.
	// Data should be PCM audio at the specified sample rate and channels.
	SendAudio(data []byte, sampleRate, channels int) error

	// SupportsRTPAudio returns true if audio should be sent via RTP
	// instead of base64-encoded in events.
	SupportsRTPAudio() bool
}

// WebSocketTransport wraps a WebSocket connection for Realtime API events.
type WebSocketTransport struct {
	conn   *websocket.Conn
	mu     sync.Mutex
	closed bool
}

// NewWebSocketTransport creates a new WebSocket transport.
func NewWebSocketTransport(conn *websocket.Conn) *WebSocketTransport {
	return &WebSocketTransport{
		conn: conn,
	}
}

// SendEvent sends a server event via WebSocket.
func (t *WebSocketTransport) SendEvent(event events.ServerEvent) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return t.conn.WriteMessage(websocket.TextMessage, data)
}

// Close closes the WebSocket connection.
func (t *WebSocketTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	return t.conn.Close()
}

// DataChannelTransport wraps a WebRTC DataChannel and audio track for Realtime API.
// Events are sent via DataChannel, audio is sent via RTP track.
type DataChannelTransport struct {
	dataChannel *webrtc.DataChannel
	audioTrack  *webrtc.TrackLocalStaticSample
	encoder     *opus.Encoder

	// Audio configuration
	sampleRate int
	channels   int

	mu     sync.Mutex
	closed bool
}

// DataChannelTransportConfig holds configuration for DataChannelTransport.
type DataChannelTransportConfig struct {
	DataChannel *webrtc.DataChannel
	AudioTrack  *webrtc.TrackLocalStaticSample
	SampleRate  int // Output sample rate (typically 48000 for WebRTC)
	Channels    int // Number of channels (typically 1)
}

// NewDataChannelTransport creates a new DataChannel transport with RTP audio support.
func NewDataChannelTransport(config DataChannelTransportConfig) (*DataChannelTransport, error) {
	// Create Opus encoder for audio output
	encoder, err := opus.NewEncoder(config.SampleRate, config.Channels, opus.AppVoIP)
	if err != nil {
		return nil, err
	}
	encoder.SetBitrate(50000)
	encoder.SetComplexity(10)
	encoder.SetDTX(true)

	return &DataChannelTransport{
		dataChannel: config.DataChannel,
		audioTrack:  config.AudioTrack,
		encoder:     encoder,
		sampleRate:  config.SampleRate,
		channels:    config.Channels,
	}, nil
}

// SendEvent sends a server event via DataChannel.
func (t *DataChannelTransport) SendEvent(event events.ServerEvent) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}

	if t.dataChannel == nil || t.dataChannel.ReadyState() != webrtc.DataChannelStateOpen {
		return nil
	}

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return t.dataChannel.Send(data)
}

// SendAudio sends audio data via RTP track.
// The input data should be PCM audio (int16 samples, little-endian).
func (t *DataChannelTransport) SendAudio(data []byte, sampleRate, channels int) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed || t.audioTrack == nil {
		return nil
	}

	// Convert bytes to int16 samples
	samples := make([]int16, len(data)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(data[i*2]) | int16(data[i*2+1])<<8
	}

	// Encode to Opus
	// Opus frame size: 20ms at 48kHz = 960 samples
	frameSize := t.sampleRate * 20 / 1000 // 20ms frame
	opusBuffer := make([]byte, 4000)      // Max Opus frame size

	// Process audio in frames
	for offset := 0; offset+frameSize <= len(samples); offset += frameSize {
		frame := samples[offset : offset+frameSize]
		n, err := t.encoder.Encode(frame, opusBuffer)
		if err != nil {
			continue
		}

		// Write to RTP track
		if err := t.audioTrack.WriteSample(media.Sample{
			Data:     opusBuffer[:n],
			Duration: 20 * time.Millisecond,
		}); err != nil {
			return err
		}
	}

	return nil
}

// SupportsRTPAudio returns true since this transport sends audio via RTP.
func (t *DataChannelTransport) SupportsRTPAudio() bool {
	return true
}

// Close closes the DataChannel transport.
func (t *DataChannelTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	if t.dataChannel != nil {
		return t.dataChannel.Close()
	}
	return nil
}
