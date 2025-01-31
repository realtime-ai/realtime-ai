package connection

import (
	"github.com/pion/webrtc/v4"
)

type ConnectionEventHandler interface {
	OnConnectionStateChange(conn RTCConnection, state webrtc.PeerConnectionState)
	OnDataChannel(conn RTCConnection, dc *webrtc.DataChannel)
	OnTrack(conn RTCConnection, track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver)
}

// NoOpConnectionEventHandler 一个空实现，方便不想实现所有方法的场景
type NoOpConnectionEventHandler struct{}

func (h *NoOpConnectionEventHandler) OnConnectionStateChange(conn RTCConnection, state webrtc.PeerConnectionState) {
}

func (h *NoOpConnectionEventHandler) OnDataChannel(conn RTCConnection, dc *webrtc.DataChannel) {
}

func (h *NoOpConnectionEventHandler) OnTrack(conn RTCConnection, track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
}
