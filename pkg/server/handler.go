package server

import (
	"context"

	"github.com/pion/webrtc/v4"
	"github.com/realtime-ai/realtime-ai/pkg/connection"
)

type ConnectionEventHandler interface {
	// 当 PeerConnection 创建完成，传入封装好的 wrapper
	OnPeerConnectionCreated(ctx context.Context, wrapper connection.RTCConnection)

	// 当 PeerConnection 创建或协商过程中出错
	OnPeerConnectionError(ctx context.Context, peerID string, err error)

	// 当协商完成，本地 SDP 就绪时
	OnPeerConnectionNegotiationComplete(ctx context.Context, wrapper connection.RTCConnection, localSDP *webrtc.SessionDescription)

	// PeerConnection 状态变更 (连接、关闭、失败等)
	OnPeerConnectionStateChange(ctx context.Context, wrapper connection.RTCConnection, state webrtc.PeerConnectionState)

	// ICE 状态变更
	OnICEConnectionStateChange(ctx context.Context, wrapper connection.RTCConnection, state webrtc.ICEConnectionState)

	// 当发现新的 DataChannel (无论本地创建还是远端创建)
	OnDataChannel(ctx context.Context, wrapper connection.RTCConnection, dc *webrtc.DataChannel)

	// 当远端 Track 到达
	OnTrack(ctx context.Context, wrapper connection.RTCConnection, track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver)
}

// NoOpConnectionEventHandler 一个空实现，方便不想实现所有方法的场景
type NoOpConnectionEventHandler struct{}

func (h *NoOpConnectionEventHandler) OnPeerConnectionCreated(ctx context.Context, wrapper connection.RTCConnection) {
}

func (h *NoOpConnectionEventHandler) OnPeerConnectionError(ctx context.Context, peerID string, err error) {
}

func (h *NoOpConnectionEventHandler) OnPeerConnectionNegotiationComplete(ctx context.Context, wrapper connection.RTCConnection, localSDP *webrtc.SessionDescription) {
}

func (h *NoOpConnectionEventHandler) OnPeerConnectionStateChange(ctx context.Context, wrapper connection.RTCConnection, state webrtc.PeerConnectionState) {
}

func (h *NoOpConnectionEventHandler) OnICEConnectionStateChange(ctx context.Context, wrapper connection.RTCConnection, state webrtc.ICEConnectionState) {
}

func (h *NoOpConnectionEventHandler) OnDataChannel(ctx context.Context, wrapper connection.RTCConnection, dc *webrtc.DataChannel) {
}

func (h *NoOpConnectionEventHandler) OnTrack(ctx context.Context, wrapper connection.RTCConnection, track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
}
