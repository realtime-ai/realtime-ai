package audio

import (
	"log"
	"sync"

	"github.com/asticode/go-astiav"
)

const (
	// 采样率
	InputSampleRate  = 24000
	OutputSampleRate = 48000
	// 通道数
	Channels = 1
	// 每个采样点的字节数 (16-bit)
	BytesPerSample = 2
	// 24kHz下20ms对应的采样点数
	SamplesPerFrame24kHz = InputSampleRate * 20 / 1000 // 480 samples
	BytesPerFrame24kHz   = SamplesPerFrame24kHz * BytesPerSample * Channels
	// 48kHz下20ms对应的采样点数
	SamplesPerFrame48kHz = OutputSampleRate * 20 / 1000 // 960 samples
	BytesPerFrame48kHz   = SamplesPerFrame48kHz * BytesPerSample * Channels
)

// AudioPacer 控制音频输出节奏，实现固定20ms间隔的音频帧输出，支持24kHz输入重采样到48kHz输出
type AudioPacer struct {
	buffer       []byte
	mu           sync.Mutex
	resampler    *Resample
	accumulating bool // 是否正在积累数据
}

// NewAudioPacer 创建新的 AudioPacer
func NewAudioPacer() (*AudioPacer, error) {
	resampler, err := NewResample(InputSampleRate, OutputSampleRate, astiav.ChannelLayoutMono, astiav.ChannelLayoutMono)
	if err != nil {
		return nil, err
	}

	return &AudioPacer{
		buffer:       make([]byte, 0, BytesPerFrame48kHz*100), // 预分配2秒的容量
		resampler:    resampler,
		accumulating: false,
	}, nil
}

// Write 写入24kHz采样率的音频数据
func (ap *AudioPacer) Write(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	// 重采样到48kHz
	resampledData, err := ap.resampler.Resample(data)
	if err != nil {
		return err
	}

	ap.mu.Lock()
	defer ap.mu.Unlock()
	ap.buffer = append(ap.buffer, resampledData...)
	return nil
}

// ReadFrame 读取固定20ms的48kHz音频帧
// 如果没有足够的数据，将返回静音数据
func (ap *AudioPacer) ReadFrame() []byte {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	// 准备输出缓冲区
	frame := make([]byte, BytesPerFrame48kHz)

	// 如果正在积累数据且缓冲区小于100ms，返回静音
	if ap.accumulating && len(ap.buffer) < BytesPerFrame48kHz*10 { // 10帧 = 200ms
		return frame
	}

	// 如果有足够数据，关闭积累状态
	if ap.accumulating && len(ap.buffer) >= BytesPerFrame48kHz*10 {
		ap.accumulating = false
		log.Printf("accumulated enough data (%d bytes), starting playback", len(ap.buffer))
	}

	if len(ap.buffer) >= BytesPerFrame48kHz {
		// 有足够的数据，复制一帧
		copy(frame, ap.buffer[:BytesPerFrame48kHz])
		// 移除已读取的数据
		ap.buffer = ap.buffer[BytesPerFrame48kHz:]
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

// Close 释放资源
func (ap *AudioPacer) Close() {
	ap.mu.Lock()
	defer ap.mu.Unlock()
	if ap.resampler != nil {
		ap.resampler.Free()
		ap.resampler = nil
	}
	ap.buffer = nil
}
