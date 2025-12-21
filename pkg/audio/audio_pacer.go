package audio

import (
	"log"
	"sync"
)

const (
	// 默认采样率
	DefaultSampleRate = 48000
	// 通道数
	Channels = 1
	// 每个采样点的字节数 (16-bit)
	BytesPerSample = 2
	// 帧时长 (毫秒)
	FrameDurationMs = 20
)

// AudioPacerConfig 配置
type AudioPacerConfig struct {
	SampleRate int // 采样率
	Channels   int // 通道数
}

// DefaultAudioPacerConfig 返回默认配置
func DefaultAudioPacerConfig() AudioPacerConfig {
	return AudioPacerConfig{
		SampleRate: DefaultSampleRate,
		Channels:   Channels,
	}
}

// AudioPacer 控制音频输出节奏，实现固定20ms间隔的音频帧输出
// 只做缓冲和帧切分，不做重采样
type AudioPacer struct {
	buffer       []byte
	mu           sync.Mutex
	accumulating bool // 是否正在积累数据

	// 配置
	sampleRate    int
	channels      int
	bytesPerFrame int
}

// NewAudioPacer 创建新的 AudioPacer (使用默认配置)
func NewAudioPacer() (*AudioPacer, error) {
	return NewAudioPacerWithConfig(DefaultAudioPacerConfig())
}

// NewAudioPacerWithConfig 创建新的 AudioPacer (使用自定义配置)
func NewAudioPacerWithConfig(cfg AudioPacerConfig) (*AudioPacer, error) {
	if cfg.SampleRate <= 0 {
		cfg.SampleRate = DefaultSampleRate
	}
	if cfg.Channels <= 0 {
		cfg.Channels = Channels
	}

	// 计算每帧字节数: 采样率 * 帧时长(秒) * 通道数 * 每采样字节数
	samplesPerFrame := cfg.SampleRate * FrameDurationMs / 1000
	bytesPerFrame := samplesPerFrame * BytesPerSample * cfg.Channels

	return &AudioPacer{
		buffer:        make([]byte, 0, bytesPerFrame*100), // 预分配2秒的容量
		accumulating:  false,
		sampleRate:    cfg.SampleRate,
		channels:      cfg.Channels,
		bytesPerFrame: bytesPerFrame,
	}, nil
}

// Write 写入 PCM 音频数据
func (ap *AudioPacer) Write(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	ap.mu.Lock()
	defer ap.mu.Unlock()
	ap.buffer = append(ap.buffer, data...)
	return nil
}

// ReadFrame 读取固定20ms的音频帧
// 如果没有足够的数据，将返回静音数据
func (ap *AudioPacer) ReadFrame() []byte {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	// 准备输出缓冲区
	frame := make([]byte, ap.bytesPerFrame)

	// 如果正在积累数据且缓冲区小于200ms，返回静音
	if ap.accumulating && len(ap.buffer) < ap.bytesPerFrame*10 { // 10帧 = 200ms
		return frame
	}

	// 如果有足够数据，关闭积累状态
	if ap.accumulating && len(ap.buffer) >= ap.bytesPerFrame*10 {
		ap.accumulating = false
		log.Printf("accumulated enough data (%d bytes), starting playback", len(ap.buffer))
	}

	if len(ap.buffer) >= ap.bytesPerFrame {
		// 有足够的数据，复制一帧
		copy(frame, ap.buffer[:ap.bytesPerFrame])
		// 移除已读取的数据
		ap.buffer = ap.buffer[ap.bytesPerFrame:]
	} else if len(ap.buffer) > 0 {
		// 有部分数据，复制可用部分，其余填充静音
		copy(frame, ap.buffer)
		// 清空缓冲区
		ap.buffer = ap.buffer[:0]
	}
	// 如果没有数据，frame 保持为零值（静音）

	return frame
}

// Clear 清空缓冲区并开始积累新数据
func (ap *AudioPacer) Clear() {
	ap.mu.Lock()
	defer ap.mu.Unlock()
	log.Printf("clear buffer: %d, starting accumulation", len(ap.buffer))
	ap.buffer = ap.buffer[:0]
	ap.accumulating = true
}

// Available 返回当前可用的音频数据长度（字节）
func (ap *AudioPacer) Available() int {
	ap.mu.Lock()
	defer ap.mu.Unlock()
	return len(ap.buffer)
}

// BytesPerFrame 返回每帧字节数
func (ap *AudioPacer) BytesPerFrame() int {
	return ap.bytesPerFrame
}

// SampleRate 返回采样率
func (ap *AudioPacer) SampleRate() int {
	return ap.sampleRate
}

// Close 释放资源
func (ap *AudioPacer) Close() {
	ap.mu.Lock()
	defer ap.mu.Unlock()
	ap.buffer = nil
}
