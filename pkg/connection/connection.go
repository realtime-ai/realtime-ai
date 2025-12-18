// Package connection provides abstractions for real-time bidirectional connections.
package connection

import (
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

// ConnectionState represents the state of a connection.
type ConnectionState int

const (
	// ConnectionStateNew - Initial state, connection not yet started
	ConnectionStateNew ConnectionState = iota
	// ConnectionStateConnecting - Connection is being established
	ConnectionStateConnecting
	// ConnectionStateConnected - Connection is established and ready
	ConnectionStateConnected
	// ConnectionStateDisconnected - Connection temporarily lost (may reconnect)
	ConnectionStateDisconnected
	// ConnectionStateFailed - Connection failed permanently
	ConnectionStateFailed
	// ConnectionStateClosed - Connection closed by user or server
	ConnectionStateClosed
)

func (s ConnectionState) String() string {
	switch s {
	case ConnectionStateNew:
		return "new"
	case ConnectionStateConnecting:
		return "connecting"
	case ConnectionStateConnected:
		return "connected"
	case ConnectionStateDisconnected:
		return "disconnected"
	case ConnectionStateFailed:
		return "failed"
	case ConnectionStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// ConnectionEventHandler handles connection lifecycle events.
type ConnectionEventHandler interface {
	// OnConnectionStateChange is called when the connection state changes.
	OnConnectionStateChange(state ConnectionState)

	// OnMessage is called when a message is received.
	OnMessage(msg *pipeline.PipelineMessage)

	// OnError is called when an error occurs.
	OnError(err error)
}

// NoOpConnectionEventHandler is a no-op implementation for convenience.
type NoOpConnectionEventHandler struct{}

func (h *NoOpConnectionEventHandler) OnConnectionStateChange(state ConnectionState) {}
func (h *NoOpConnectionEventHandler) OnMessage(msg *pipeline.PipelineMessage)       {}
func (h *NoOpConnectionEventHandler) OnError(err error)                             {}

// Connection represents a bidirectional real-time connection.
// Implementations include WebRTC and WebSocket transports.
type Connection interface {
	// PeerID returns the unique identifier for this connection.
	PeerID() string

	// RegisterEventHandler registers an event handler for connection events.
	RegisterEventHandler(handler ConnectionEventHandler)

	// SendMessage sends a message to the peer.
	SendMessage(msg *pipeline.PipelineMessage)

	// Close closes the connection and releases resources.
	Close() error
}
