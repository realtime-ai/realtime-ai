package tts

import (
	"context"
)

// AudioFormat defines the audio format configuration
type AudioFormat struct {
	SampleRate int                        // Sample rate in Hz (e.g., 24000, 16000)
	Channels   int                        // Number of audio channels (1 for mono, 2 for stereo)
	MediaType  interface{}                // MIME type (AudioMediaType or string for compatibility)
	Encoding   string                     // Audio encoding format (e.g., "pcm_s16le", "opus")
}

// SynthesizeRequest represents a request to synthesize speech
type SynthesizeRequest struct {
	Text     string                 // Text to synthesize
	Voice    string                 // Voice ID or name
	Language string                 // Language code (e.g., "en-US", "zh-CN")
	Options  map[string]interface{} // Additional provider-specific options
}

// SynthesizeResponse represents the response from speech synthesis
type SynthesizeResponse struct {
	AudioData   []byte      // Raw audio data
	AudioFormat AudioFormat // Format of the audio data
	Duration    float64     // Duration in seconds (if available)
}

// TTSProvider defines the interface that all TTS services must implement
// This allows for easy extension to support multiple TTS providers
type TTSProvider interface {
	// Name returns the name of the TTS provider (e.g., "openai", "azure", "elevenlabs")
	Name() string

	// Synthesize converts text to speech
	// It takes a context for cancellation and a request with text and voice settings
	// Returns the synthesized audio data and format, or an error
	Synthesize(ctx context.Context, req *SynthesizeRequest) (*SynthesizeResponse, error)

	// GetSupportedVoices returns a list of available voices for this provider
	// Returns voice IDs/names that can be used in SynthesizeRequest
	GetSupportedVoices() []string

	// GetDefaultVoice returns the default voice for this provider
	GetDefaultVoice() string

	// ValidateConfig validates the provider's configuration
	// Returns an error if credentials or required settings are missing
	ValidateConfig() error
}

// StreamingTTSProvider extends TTSProvider with streaming capabilities
// Not all providers support streaming, so this is optional
type StreamingTTSProvider interface {
	TTSProvider

	// StreamSynthesize streams audio data as it's generated
	// Returns a channel that receives audio chunks and an error channel
	StreamSynthesize(ctx context.Context, req *SynthesizeRequest) (<-chan []byte, <-chan error)
}
