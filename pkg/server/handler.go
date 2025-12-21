package server

import (
	"context"

	"github.com/realtime-ai/realtime-ai/pkg/connection"
)

type ServerEventHandler interface {
	// OnConnectionCreated is called when a connection is established.
	OnConnectionCreated(ctx context.Context, conn connection.Connection)

	// OnConnectionError is called when a connection error occurs.
	OnConnectionError(ctx context.Context, peerID string, err error)
}

// NoOpServerEventHandler is a no-op implementation of ServerEventHandler.
type NoOpServerEventHandler struct{}

func (h *NoOpServerEventHandler) OnConnectionCreated(ctx context.Context, conn connection.Connection) {
}

func (h *NoOpServerEventHandler) OnConnectionError(ctx context.Context, peerID string, err error) {
}
