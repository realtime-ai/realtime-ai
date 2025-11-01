package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

const (
	openAITTSEndpoint       = "https://api.openai.com/v1/audio/speech"
	openAIDefaultModel      = "tts-1"
	openAIDefaultVoice      = "alloy"
	openAIDefaultFormat     = "pcm" // Raw PCM for pipeline compatibility
	openAIDefaultSampleRate = 24000
)

// OpenAI supported voices
var openAIVoices = []string{
	"alloy",   // Neutral and balanced
	"echo",    // More expressive
	"fable",   // British accent
	"onyx",    // Deep and authoritative
	"nova",    // Energetic and lively
	"shimmer", // Soft and gentle
}

// OpenAITTSProvider implements TTSProvider for OpenAI's TTS API
type OpenAITTSProvider struct {
	apiKey     string
	model      string // "tts-1" or "tts-1-hd"
	httpClient *http.Client
}

// OpenAITTSRequest represents the request payload for OpenAI TTS API
type OpenAITTSRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format,omitempty"`
	Speed          float64 `json:"speed,omitempty"` // 0.25 to 4.0, default 1.0
}

// NewOpenAITTSProvider creates a new OpenAI TTS provider
func NewOpenAITTSProvider(apiKey string) *OpenAITTSProvider {
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}

	return &OpenAITTSProvider{
		apiKey:     apiKey,
		model:      openAIDefaultModel,
		httpClient: &http.Client{},
	}
}

// NewOpenAITTSProviderHD creates a new OpenAI TTS provider with HD model
func NewOpenAITTSProviderHD(apiKey string) *OpenAITTSProvider {
	provider := NewOpenAITTSProvider(apiKey)
	provider.model = "tts-1-hd"
	return provider
}

// Name returns the provider name
func (p *OpenAITTSProvider) Name() string {
	return "openai"
}

// SetModel sets the TTS model ("tts-1" or "tts-1-hd")
func (p *OpenAITTSProvider) SetModel(model string) {
	p.model = model
}

// Synthesize converts text to speech using OpenAI TTS API
func (p *OpenAITTSProvider) Synthesize(ctx context.Context, req *SynthesizeRequest) (*SynthesizeResponse, error) {
	if err := p.ValidateConfig(); err != nil {
		return nil, err
	}

	// Set default voice if not specified
	voice := req.Voice
	if voice == "" {
		voice = openAIDefaultVoice
	}

	// Extract speed from options if provided
	speed := 1.0
	if req.Options != nil {
		if s, ok := req.Options["speed"].(float64); ok {
			speed = s
		}
	}

	// Extract format from options if provided
	format := openAIDefaultFormat
	if req.Options != nil {
		if f, ok := req.Options["format"].(string); ok {
			format = f
		}
	}

	// Create request payload
	payload := OpenAITTSRequest{
		Model:          p.model,
		Input:          req.Text,
		Voice:          voice,
		ResponseFormat: format,
		Speed:          speed,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", openAITTSEndpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	// Send request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read audio data
	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Determine audio format based on response format
	audioFormat := p.getAudioFormat(format)

	return &SynthesizeResponse{
		AudioData:   audioData,
		AudioFormat: audioFormat,
	}, nil
}

// getAudioFormat returns the audio format configuration based on the response format
func (p *OpenAITTSProvider) getAudioFormat(format string) AudioFormat {
	switch format {
	case "pcm":
		return AudioFormat{
			SampleRate: openAIDefaultSampleRate,
			Channels:   1,
			MediaType:  "audio/pcm",
			Encoding:   "pcm_s16le",
		}
	case "opus":
		return AudioFormat{
			SampleRate: 24000,
			Channels:   1,
			MediaType:  "audio/opus",
			Encoding:   "opus",
		}
	case "mp3":
		return AudioFormat{
			SampleRate: 24000,
			Channels:   1,
			MediaType:  "audio/mpeg",
			Encoding:   "mp3",
		}
	case "wav":
		return AudioFormat{
			SampleRate: 24000,
			Channels:   1,
			MediaType:  "audio/wav",
			Encoding:   "wav",
		}
	default:
		// Default to PCM
		return AudioFormat{
			SampleRate: openAIDefaultSampleRate,
			Channels:   1,
			MediaType:  "audio/pcm",
			Encoding:   "pcm_s16le",
		}
	}
}

// GetSupportedVoices returns the list of supported OpenAI voices
func (p *OpenAITTSProvider) GetSupportedVoices() []string {
	return openAIVoices
}

// GetDefaultVoice returns the default voice
func (p *OpenAITTSProvider) GetDefaultVoice() string {
	return openAIDefaultVoice
}

// ValidateConfig validates the provider configuration
func (p *OpenAITTSProvider) ValidateConfig() error {
	if p.apiKey == "" {
		return fmt.Errorf("OpenAI API key is not set. Please set OPENAI_API_KEY environment variable")
	}
	return nil
}
