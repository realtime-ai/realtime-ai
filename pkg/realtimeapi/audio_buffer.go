package realtimeapi

import (
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi/events"
)

// AudioBufferConfig holds the configuration for the audio buffer.
type AudioBufferConfig struct {
	// MaxSize is the maximum size of the buffer in bytes.
	MaxSize int
	// SampleRate is the sample rate of the audio.
	SampleRate int
	// Channels is the number of audio channels.
	Channels int
	// Format is the audio format (e.g., "pcm16").
	Format events.AudioFormat
}

// DefaultAudioBufferConfig returns the default audio buffer configuration.
func DefaultAudioBufferConfig() AudioBufferConfig {
	return AudioBufferConfig{
		MaxSize:    10 * 1024 * 1024, // 10 MB
		SampleRate: 24000,
		Channels:   1,
		Format:     events.AudioFormatPCM16,
	}
}

// AudioBuffer manages audio data buffering for the Realtime API.
type AudioBuffer struct {
	config AudioBufferConfig
	data   []byte

	// Speech detection state
	speechStarted bool
	speechStartMs int

	// Timing
	startTime    time.Time
	totalSamples int64

	mu sync.Mutex
}

// NewAudioBuffer creates a new AudioBuffer with the given configuration.
func NewAudioBuffer(config AudioBufferConfig) *AudioBuffer {
	return &AudioBuffer{
		config:    config,
		data:      make([]byte, 0),
		startTime: time.Now(),
	}
}

// Append adds audio data to the buffer.
// The audio should be base64 encoded.
func (b *AudioBuffer) Append(base64Audio string) error {
	audioData, err := base64.StdEncoding.DecodeString(base64Audio)
	if err != nil {
		return errors.New("invalid base64 audio data")
	}

	return b.AppendRaw(audioData)
}

// AppendRaw adds raw audio data to the buffer.
func (b *AudioBuffer) AppendRaw(audioData []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if adding this data would exceed the max size
	if len(b.data)+len(audioData) > b.config.MaxSize {
		return errors.New("audio buffer overflow")
	}

	b.data = append(b.data, audioData...)

	// Update sample count (assuming 16-bit samples)
	b.totalSamples += int64(len(audioData) / 2)

	return nil
}

// Commit returns the buffered audio data and clears the buffer.
// Returns the audio data and the duration in milliseconds.
func (b *AudioBuffer) Commit() ([]byte, int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.data) == 0 {
		return nil, 0, nil
	}

	// Calculate duration
	durationMs := b.calculateDurationMs(len(b.data))

	// Get the data and clear the buffer
	data := b.data
	b.data = make([]byte, 0)
	b.speechStarted = false
	b.speechStartMs = 0

	return data, durationMs, nil
}

// Clear removes all data from the buffer.
func (b *AudioBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.data = make([]byte, 0)
	b.speechStarted = false
	b.speechStartMs = 0
}

// Size returns the current size of the buffer in bytes.
func (b *AudioBuffer) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.data)
}

// Duration returns the duration of the buffered audio in milliseconds.
func (b *AudioBuffer) Duration() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calculateDurationMs(len(b.data))
}

// IsEmpty returns true if the buffer is empty.
func (b *AudioBuffer) IsEmpty() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.data) == 0
}

// GetData returns a copy of the current buffer data.
func (b *AudioBuffer) GetData() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.data) == 0 {
		return nil
	}

	data := make([]byte, len(b.data))
	copy(data, b.data)
	return data
}

// GetBase64Data returns the buffer data as base64 encoded string.
func (b *AudioBuffer) GetBase64Data() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.data) == 0 {
		return ""
	}

	return base64.StdEncoding.EncodeToString(b.data)
}

// SetSpeechStarted marks the start of speech in the buffer.
func (b *AudioBuffer) SetSpeechStarted(startMs int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.speechStarted {
		b.speechStarted = true
		b.speechStartMs = startMs
	}
}

// SetSpeechStopped marks the end of speech.
func (b *AudioBuffer) SetSpeechStopped() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.speechStarted = false
}

// IsSpeechStarted returns true if speech has been detected.
func (b *AudioBuffer) IsSpeechStarted() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.speechStarted
}

// SpeechStartMs returns the millisecond offset where speech started.
func (b *AudioBuffer) SpeechStartMs() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.speechStartMs
}

// TotalDurationMs returns the total duration of audio that has passed through the buffer.
func (b *AudioBuffer) TotalDurationMs() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Calculate based on total samples
	samples := b.totalSamples
	return int(samples * 1000 / int64(b.config.SampleRate))
}

// Reset resets the buffer to its initial state.
func (b *AudioBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.data = make([]byte, 0)
	b.speechStarted = false
	b.speechStartMs = 0
	b.totalSamples = 0
	b.startTime = time.Now()
}

// calculateDurationMs calculates the duration in milliseconds for the given byte count.
// Assumes 16-bit PCM audio.
func (b *AudioBuffer) calculateDurationMs(byteCount int) int {
	// 16-bit = 2 bytes per sample
	samples := byteCount / 2 / b.config.Channels
	return samples * 1000 / b.config.SampleRate
}

// Config returns the buffer configuration.
func (b *AudioBuffer) Config() AudioBufferConfig {
	return b.config
}

// UpdateConfig updates the buffer configuration.
// This clears the existing buffer data.
func (b *AudioBuffer) UpdateConfig(config AudioBufferConfig) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.config = config
	b.data = make([]byte, 0)
	b.speechStarted = false
	b.speechStartMs = 0
}
