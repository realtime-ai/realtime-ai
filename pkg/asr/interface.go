// Package asr provides a unified interface for Automatic Speech Recognition (ASR) systems.
// It abstracts different ASR providers (OpenAI Whisper, Google Cloud Speech, etc.)
// allowing for easy integration and extension.
package asr

import (
	"context"
	"io"
	"time"
)

// RecognitionResult represents the output of speech recognition.
type RecognitionResult struct {
	// Text is the recognized text
	Text string

	// IsFinal indicates if this is a final result (true) or partial/interim (false)
	IsFinal bool

	// Confidence score (0.0-1.0) if available, otherwise -1
	Confidence float32

	// Language detected or used for recognition
	Language string

	// Duration of the audio segment that was recognized
	Duration time.Duration

	// Timestamp when recognition completed
	Timestamp time.Time

	// Additional provider-specific metadata
	Metadata map[string]interface{}
}

// AudioConfig specifies the audio format for recognition.
type AudioConfig struct {
	// SampleRate in Hz (e.g., 16000, 48000)
	SampleRate int

	// Channels (1 for mono, 2 for stereo)
	Channels int

	// Encoding format (e.g., "pcm", "opus", "flac")
	Encoding string

	// BitsPerSample (e.g., 16, 24)
	BitsPerSample int
}

// RecognitionConfig contains settings for speech recognition.
type RecognitionConfig struct {
	// Language code (e.g., "en-US", "zh-CN", "auto" for auto-detection)
	Language string

	// Model to use (provider-specific, e.g., "whisper-1" for OpenAI)
	Model string

	// EnablePartialResults determines if partial/interim results should be returned
	// during streaming recognition
	EnablePartialResults bool

	// MaxAlternatives specifies the maximum number of alternative transcriptions
	// to return (if supported by provider)
	MaxAlternatives int

	// ProfanityFilter enables profanity filtering if supported
	ProfanityFilter bool

	// Prompt or context to guide the recognition (if supported)
	Prompt string

	// Temperature for sampling (OpenAI Whisper specific, 0.0-1.0)
	Temperature float32

	// Additional provider-specific configuration
	Extra map[string]interface{}
}

// StreamingRecognizer handles continuous speech recognition from an audio stream.
type StreamingRecognizer interface {
	// SendAudio sends audio data to the recognizer.
	// Audio must match the AudioConfig provided during initialization.
	SendAudio(ctx context.Context, audioData []byte) error

	// Results returns a channel that receives recognition results.
	// The channel will be closed when the recognizer is closed.
	Results() <-chan *RecognitionResult

	// Close stops recognition and releases resources.
	Close() error
}

// Provider is the main interface for ASR systems.
type Provider interface {
	// Name returns the provider name (e.g., "openai-whisper", "google-cloud", "azure")
	Name() string

	// Recognize performs speech recognition on a complete audio segment.
	// This is suitable for batch processing or when audio is already buffered.
	Recognize(ctx context.Context, audio io.Reader, audioConfig AudioConfig, config RecognitionConfig) (*RecognitionResult, error)

	// StreamingRecognize creates a streaming recognizer for continuous audio input.
	// This is suitable for real-time recognition with VAD integration.
	StreamingRecognize(ctx context.Context, audioConfig AudioConfig, config RecognitionConfig) (StreamingRecognizer, error)

	// SupportsStreaming indicates if the provider supports streaming recognition.
	SupportsStreaming() bool

	// SupportedLanguages returns a list of supported language codes.
	// Returns empty slice if all languages are supported.
	SupportedLanguages() []string

	// Close releases any resources held by the provider.
	Close() error
}

// Error types for ASR operations
type Error struct {
	Code    ErrorCode
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Err
}

type ErrorCode int

const (
	ErrCodeUnknown ErrorCode = iota
	ErrCodeInvalidConfig
	ErrCodeInvalidAudio
	ErrCodeUnsupportedLanguage
	ErrCodeUnsupportedFeature
	ErrCodeAuthenticationFailed
	ErrCodeQuotaExceeded
	ErrCodeNetworkError
	ErrCodeProviderError
)
