package connection

import (
	"github.com/pion/webrtc/v4"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

type ConnectionEventHandler interface {
	// 连接状态变化
	OnConnectionStateChange(state webrtc.PeerConnectionState)

	// 数据回调
	OnMessage(msg *pipeline.PipelineMessage)

	// 错误
	OnError(err error)
}

// NoOpConnectionEventHandler 一个空实现，方便不想实现所有方法的场景
type NoOpConnectionEventHandler struct{}

// 连接状态变化
func (h *NoOpConnectionEventHandler) OnConnectionStateChange(state webrtc.PeerConnectionState) {
}

// 错误
func (h *NoOpConnectionEventHandler) OnError(err error) {
}

// 数据回调
func (h *NoOpConnectionEventHandler) OnMessage(msg *pipeline.PipelineMessage) {
}

type RTCConnection interface {
	// PeerID 返回此连接对应的唯一标识
	PeerID() string
	// PeerConnection 返回底层的 *webrtc.PeerConnection
	// RegisterEventHandler 注册事件处理器
	RegisterEventHandler(handler ConnectionEventHandler)

	// SetAudioEncodeParam 设置音频编码参数
	// SetAudioEncodeParam(sampleRate int, channels int, bitRate int)

	// // SetAudioOutputParam 设置音频输出参数
	// SetAudioOutputParam(sampleRate int, channels int)

	// SendMessage 发送消息
	SendMessage(msg *pipeline.PipelineMessage)

	// Close 关闭底层的 PeerConnection (并执行相应清理)
	Close() error
}
