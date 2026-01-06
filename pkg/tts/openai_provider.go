// OpenAI GPT-4o-mini TTS Provider
//
// Implements StreamingTTSProvider using OpenAI's gpt-4o-mini-tts model.
// Supports SSE streaming for low-latency audio generation and voice instructions.
//
// Features:
//   - SSE streaming support for real-time audio
//   - Voice instructions for tone/style control
//   - 13 built-in voices including marin and cedar (recommended)
//   - 24kHz PCM/Opus/MP3/WAV output formats
//
// Usage:
//
//	provider := tts.NewOpenAITTSProvider(apiKey)
//	provider.SetInstructions("Speak in a cheerful tone")
//	audioChan, errChan := provider.StreamSynthesize(ctx, req)

package tts

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

const (
	openAITTSEndpoint       = "https://api.openai.com/v1/audio/speech"
	openAIDefaultModel      = "gpt-4o-mini-tts"
	openAIDefaultVoice      = "coral" // Recommended voice
	openAIDefaultFormat     = "pcm"   // Raw PCM for pipeline compatibility
	openAIDefaultSampleRate = 24000
)

// OpenAI supported voices (gpt-4o-mini-tts)
// Recommended: marin, cedar
var openAIVoices = []string{
	"alloy",   // Neutral and balanced
	"ash",     // Clear and precise
	"ballad",  // Melodic and warm
	"coral",   // Natural and conversational (default)
	"echo",    // More expressive
	"fable",   // British accent
	"nova",    // Energetic and lively
	"onyx",    // Deep and authoritative
	"sage",    // Calm and thoughtful
	"shimmer", // Soft and gentle
	"verse",   // Versatile and adaptive
	"marin",   // High quality, recommended
	"cedar",   // High quality, recommended
}

// OpenAITTSProvider implements StreamingTTSProvider for OpenAI's gpt-4o-mini-tts
type OpenAITTSProvider struct {
	apiKey       string
	model        string
	instructions string // Voice style instructions
	httpClient   *http.Client
}

// OpenAITTSRequest represents the request payload for OpenAI TTS API
type OpenAITTSRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format,omitempty"`
	Speed          float64 `json:"speed,omitempty"`          // 0.25 to 4.0, default 1.0
	Instructions   string  `json:"instructions,omitempty"`   // Voice style instructions
	StreamFormat   string  `json:"stream_format,omitempty"`  // "sse" for streaming
}

// OpenAISSEEvent represents an SSE event from OpenAI streaming response
type OpenAISSEEvent struct {
	Type  string `json:"type"`            // "speech.audio.delta" or "speech.audio.done"
	Audio string `json:"audio,omitempty"` // Base64 encoded audio chunk
}

// NewOpenAITTSProvider creates a new OpenAI TTS provider with gpt-4o-mini-tts
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

// Name returns the provider name
func (p *OpenAITTSProvider) Name() string {
	return "openai"
}

// SetModel sets the TTS model (gpt-4o-mini-tts or gpt-4o-mini-tts-2025-12-15)
func (p *OpenAITTSProvider) SetModel(model string) {
	p.model = model
}

// SetInstructions sets the voice style instructions
// Example: "Speak in a cheerful and positive tone"
func (p *OpenAITTSProvider) SetInstructions(instructions string) {
	p.instructions = instructions
}

// GetInstructions returns the current voice style instructions
func (p *OpenAITTSProvider) GetInstructions() string {
	return p.instructions
}

// Synthesize converts text to speech using OpenAI TTS API (non-streaming)
func (p *OpenAITTSProvider) Synthesize(ctx context.Context, req *SynthesizeRequest) (*SynthesizeResponse, error) {
	if err := p.ValidateConfig(); err != nil {
		return nil, err
	}

	// Set default voice if not specified
	voice := req.Voice
	if voice == "" {
		voice = openAIDefaultVoice
	}

	// Extract options
	speed := 1.0
	if req.Options != nil {
		if s, ok := req.Options["speed"].(float64); ok {
			speed = s
		}
	}

	format := openAIDefaultFormat
	if req.Options != nil {
		if f, ok := req.Options["format"].(string); ok {
			format = f
		}
	}

	// Get instructions from options or provider default
	instructions := p.instructions
	if req.Options != nil {
		if inst, ok := req.Options["instructions"].(string); ok {
			instructions = inst
		}
	}

	// Create request payload
	payload := OpenAITTSRequest{
		Model:          p.model,
		Input:          req.Text,
		Voice:          voice,
		ResponseFormat: format,
		Speed:          speed,
		Instructions:   instructions,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Determine base URL
	baseURL := openAITTSEndpoint
	if envBaseURL := os.Getenv("OPENAI_BASE_URL"); envBaseURL != "" {
		baseURL = envBaseURL
		if baseURL[len(baseURL)-1] != '/' {
			baseURL += "/"
		}
		baseURL += "audio/speech"
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL, bytes.NewReader(payloadBytes))
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

// StreamSynthesize streams audio data as it's generated using SSE
func (p *OpenAITTSProvider) StreamSynthesize(ctx context.Context, req *SynthesizeRequest) (<-chan []byte, <-chan error) {
	audioChan := make(chan []byte, 100)
	errChan := make(chan error, 1)

	go func() {
		defer close(audioChan)
		defer close(errChan)

		if err := p.doStreamSynthesize(ctx, req, audioChan); err != nil {
			errChan <- err
		}
	}()

	return audioChan, errChan
}

// doStreamSynthesize performs the actual SSE streaming request
func (p *OpenAITTSProvider) doStreamSynthesize(ctx context.Context, req *SynthesizeRequest, audioChan chan<- []byte) error {
	if err := p.ValidateConfig(); err != nil {
		return err
	}

	// Set default voice if not specified
	voice := req.Voice
	if voice == "" {
		voice = openAIDefaultVoice
	}

	// Extract options
	speed := 1.0
	if req.Options != nil {
		if s, ok := req.Options["speed"].(float64); ok {
			speed = s
		}
	}

	format := openAIDefaultFormat
	if req.Options != nil {
		if f, ok := req.Options["format"].(string); ok {
			format = f
		}
	}

	// Get instructions from options or provider default
	instructions := p.instructions
	if req.Options != nil {
		if inst, ok := req.Options["instructions"].(string); ok {
			instructions = inst
		}
	}

	// Create request payload with SSE streaming
	payload := OpenAITTSRequest{
		Model:          p.model,
		Input:          req.Text,
		Voice:          voice,
		ResponseFormat: format,
		Speed:          speed,
		Instructions:   instructions,
		StreamFormat:   "sse", // Enable SSE streaming
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Determine base URL
	baseURL := openAITTSEndpoint
	if envBaseURL := os.Getenv("OPENAI_BASE_URL"); envBaseURL != "" {
		baseURL = envBaseURL
		if baseURL[len(baseURL)-1] != '/' {
			baseURL += "/"
		}
		baseURL += "audio/speech"
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	log.Printf("[OpenAI-TTS] Starting SSE stream with voice: %s", voice)

	// Send request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Check content type to determine response format
	contentType := resp.Header.Get("Content-Type")
	log.Printf("[OpenAI-TTS] Response Content-Type: %s", contentType)

	// If response is text/event-stream, parse as SSE
	if strings.Contains(contentType, "text/event-stream") {
		return p.parseSSEStream(ctx, resp.Body, audioChan)
	}

	// Otherwise, response is raw audio stream - read in chunks
	log.Printf("[OpenAI-TTS] Reading raw audio stream")
	buffer := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := resp.Body.Read(buffer)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buffer[:n])
			select {
			case audioChan <- chunk:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err != nil {
			if err == io.EOF {
				log.Printf("[OpenAI-TTS] Audio stream completed")
				return nil
			}
			return fmt.Errorf("failed to read audio stream: %w", err)
		}
	}
}

// parseSSEStream handles SSE formatted responses
func (p *OpenAITTSProvider) parseSSEStream(ctx context.Context, body io.Reader, audioChan chan<- []byte) error {
	reader := bufio.NewReader(body)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				log.Printf("[OpenAI-TTS] SSE stream ended")
				return nil
			}
			return fmt.Errorf("failed to read SSE stream: %w", err)
		}

		lineStr := strings.TrimSpace(string(line))
		if lineStr == "" {
			continue
		}

		// Parse SSE data line
		if strings.HasPrefix(lineStr, "data: ") {
			data := strings.TrimPrefix(lineStr, "data: ")

			// Check for stream end marker
			if data == "[DONE]" {
				log.Printf("[OpenAI-TTS] SSE stream completed")
				return nil
			}

			// Parse JSON event
			var event OpenAISSEEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				log.Printf("[OpenAI-TTS] Failed to parse SSE event: %v", err)
				continue
			}

			// Handle different event types
			switch event.Type {
			case "speech.audio.delta":
				// Decode base64 audio chunk
				if event.Audio != "" {
					audioData, err := base64.StdEncoding.DecodeString(event.Audio)
					if err != nil {
						log.Printf("[OpenAI-TTS] Failed to decode audio chunk: %v", err)
						continue
					}

					select {
					case audioChan <- audioData:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			case "speech.audio.done":
				log.Printf("[OpenAI-TTS] SSE stream completed")
				return nil
			}
		}
	}
}

// getAudioFormat returns the audio format configuration based on the response format
func (p *OpenAITTSProvider) getAudioFormat(format string) AudioFormat {
	switch format {
	case "pcm":
		return AudioFormat{
			SampleRate: openAIDefaultSampleRate,
			Channels:   1,
			MediaType:  pipeline.AudioMediaTypePCM,
			Encoding:   "pcm_s16le",
		}
	case "opus":
		return AudioFormat{
			SampleRate: 24000,
			Channels:   1,
			MediaType:  pipeline.AudioMediaTypeOpusStandard,
			Encoding:   "opus",
		}
	case "mp3":
		return AudioFormat{
			SampleRate: 24000,
			Channels:   1,
			MediaType:  pipeline.AudioMediaTypeMPEG,
			Encoding:   "mp3",
		}
	case "wav":
		return AudioFormat{
			SampleRate: 24000,
			Channels:   1,
			MediaType:  pipeline.AudioMediaTypeWAV,
			Encoding:   "wav",
		}
	default:
		// Default to PCM
		return AudioFormat{
			SampleRate: openAIDefaultSampleRate,
			Channels:   1,
			MediaType:  pipeline.AudioMediaTypePCM,
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

// Ensure OpenAITTSProvider implements StreamingTTSProvider
var _ StreamingTTSProvider = (*OpenAITTSProvider)(nil)
