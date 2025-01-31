package server

import (
	"context"

	"github.com/realtime-ai/realtime-ai/pkg/connection"
)

type ServerEventHandler interface {
	// 当 PeerConnection 创建完成，传入封装好的 wrapper
	OnConnectionCreated(ctx context.Context, conn connection.RTCConnection)

	// 当 PeerConnection 创建或协商过程中出错
	OnConnectionError(ctx context.Context, peerID string, err error)
}

// NoOpConnectionEventHandler 一个空实现，方便不想实现所有方法的场景
type NoOpServerEventHandler struct{}

func (h *NoOpServerEventHandler) OnConnectionCreated(ctx context.Context, conn connection.RTCConnection) {
}

func (h *NoOpServerEventHandler) OnConnectionError(ctx context.Context, peerID string, err error) {
}
