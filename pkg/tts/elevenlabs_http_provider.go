// ElevenLabs HTTP TTS Provider
//
// Implements StreamingTTSProvider using ElevenLabs HTTP streaming API for
// text-to-speech synthesis. Outputs 16kHz mono PCM audio.
//
// Reference: https://elevenlabs.io/docs/api-reference/text-to-speech/stream

package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

const (
	elevenLabsHTTPEndpoint          = "https://api.elevenlabs.io/v1/text-to-speech"
	elevenLabsHTTPDefaultModel      = "eleven_multilingual_v2"
	elevenLabsHTTPOutputFormat      = "pcm_16000" // 16kHz mono PCM
	elevenLabsHTTPSampleRate        = 16000
	elevenLabsHTTPLatencyOptimize   = 3 // Max latency optimizations
	elevenLabsHTTPStreamingChunkSize = 4096
)

// ElevenLabs HTTP supported voices (partial list - use API to get full list)
var elevenLabsHTTPVoices = []string{
	"21m00Tcm4TlvDq8ikWAM", // Rachel
	"AZnzlk1XvdvUeBnXmlld", // Domi
	"EXAVITQu4vr4xnSDxMaL", // Bella
	"ErXwobaYiN019PkySvjV", // Antoni
	"MF3mGyEYCl7XYWbV9V6O", // Elli
	"TxGEqnHWrfWFTfGW9XjX", // Josh
	"VR6AewLTigWG4xSOukaG", // Arnold
	"pNInz6obpgDQGcFmaJgB", // Adam
	"yoZ06aMxZJJ28mfd3POQ", // Sam
}

// ElevenLabsHTTPTTSConfig holds the configuration for ElevenLabs HTTP TTS
type ElevenLabsHTTPTTSConfig struct {
	APIKey               string  // Required: ElevenLabs API key
	VoiceID              string  // Required: Voice ID to use
	Model                string  // Optional: Model ID (default: eleven_multilingual_v2)
	Speed                float64 // Optional: Speed 0.7-1.2 (default: 1.0)
	LatencyOptimization  int     // Optional: Latency optimization level 0-4 (default: 3)
	Stability            float64 // Optional: Voice stability 0-1 (default: 0.5)
	SimilarityBoost      float64 // Optional: Similarity boost 0-1 (default: 0.75)
}

// ElevenLabsHTTPTTSProvider implements StreamingTTSProvider using HTTP streaming
type ElevenLabsHTTPTTSProvider struct {
	apiKey              string
	voiceID             string
	model               string
	speed               float64
	latencyOptimization int
	stability           float64
	similarityBoost     float64
	httpClient          *http.Client
}

// NewElevenLabsHTTPTTSProvider creates a new ElevenLabs HTTP TTS provider
func NewElevenLabsHTTPTTSProvider(config ElevenLabsHTTPTTSConfig) (*ElevenLabsHTTPTTSProvider, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("ElevenLabs API key is required")
	}
	if config.VoiceID == "" {
		return nil, fmt.Errorf("ElevenLabs Voice ID is required")
	}

	model := config.Model
	if model == "" {
		model = elevenLabsHTTPDefaultModel
	}

	speed := config.Speed
	if speed == 0 {
		speed = 1.0
	}

	latencyOpt := config.LatencyOptimization
	if latencyOpt == 0 {
		latencyOpt = elevenLabsHTTPLatencyOptimize
	}

	stability := config.Stability
	if stability == 0 {
		stability = 0.5
	}

	similarityBoost := config.SimilarityBoost
	if similarityBoost == 0 {
		similarityBoost = 0.75
	}

	return &ElevenLabsHTTPTTSProvider{
		apiKey:              config.APIKey,
		voiceID:             config.VoiceID,
		model:               model,
		speed:               speed,
		latencyOptimization: latencyOpt,
		stability:           stability,
		similarityBoost:     similarityBoost,
		httpClient:          &http.Client{},
	}, nil
}

// Name returns the provider name
func (p *ElevenLabsHTTPTTSProvider) Name() string {
	return "elevenlabs-http"
}

// Synthesize converts text to speech (batch mode - collects all audio)
func (p *ElevenLabsHTTPTTSProvider) Synthesize(ctx context.Context, req *SynthesizeRequest) (*SynthesizeResponse, error) {
	if err := p.ValidateConfig(); err != nil {
		return nil, err
	}

	// Use streaming internally and collect all chunks
	audioChan, errChan := p.StreamSynthesize(ctx, req)

	var audioData []byte
	for {
		select {
		case chunk, ok := <-audioChan:
			if !ok {
				// Channel closed, check for errors
				select {
				case err := <-errChan:
					if err != nil {
						return nil, err
					}
				default:
				}
				// Return collected audio
				return &SynthesizeResponse{
					AudioData: audioData,
					AudioFormat: AudioFormat{
						SampleRate: elevenLabsHTTPSampleRate,
						Channels:   1,
						MediaType:  pipeline.AudioMediaTypePCM,
						Encoding:   "pcm_s16le",
					},
				}, nil
			}
			audioData = append(audioData, chunk...)

		case err := <-errChan:
			if err != nil {
				return nil, err
			}

		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// StreamSynthesize streams audio data as it's generated
func (p *ElevenLabsHTTPTTSProvider) StreamSynthesize(ctx context.Context, req *SynthesizeRequest) (<-chan []byte, <-chan error) {
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

// doStreamSynthesize performs the actual HTTP streaming request
func (p *ElevenLabsHTTPTTSProvider) doStreamSynthesize(ctx context.Context, req *SynthesizeRequest, audioChan chan<- []byte) error {
	// Build URL with voice ID and query parameters
	voiceID := req.Voice
	if voiceID == "" {
		voiceID = p.voiceID
	}

	params := url.Values{}
	params.Set("output_format", elevenLabsHTTPOutputFormat)
	params.Set("optimize_streaming_latency", fmt.Sprintf("%d", p.latencyOptimization))

	requestURL := fmt.Sprintf("%s/%s/stream?%s", elevenLabsHTTPEndpoint, voiceID, params.Encode())

	// Create request body
	requestBody := elevenLabsHTTPRequestBody{
		Text:    req.Text,
		ModelID: p.model,
		VoiceSettings: &elevenLabsHTTPVoiceSettings{
			Stability:       p.stability,
			SimilarityBoost: p.similarityBoost,
			Speed:           p.speed,
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", requestURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("xi-api-key", p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "audio/mpeg") // Server returns binary audio

	// Send request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ElevenLabs API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read streaming response in chunks
	buffer := make([]byte, elevenLabsHTTPStreamingChunkSize)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := resp.Body.Read(buffer)
		if n > 0 {
			// Send chunk to audio channel
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
				// Stream completed successfully
				return nil
			}
			return fmt.Errorf("failed to read response body: %w", err)
		}
	}
}

// GetSupportedVoices returns a list of known voice IDs
func (p *ElevenLabsHTTPTTSProvider) GetSupportedVoices() []string {
	return elevenLabsHTTPVoices
}

// GetDefaultVoice returns the configured voice ID
func (p *ElevenLabsHTTPTTSProvider) GetDefaultVoice() string {
	return p.voiceID
}

// ValidateConfig validates the provider configuration
func (p *ElevenLabsHTTPTTSProvider) ValidateConfig() error {
	if p.apiKey == "" {
		return fmt.Errorf("ElevenLabs API key is not set")
	}
	if p.voiceID == "" {
		return fmt.Errorf("ElevenLabs Voice ID is not set")
	}
	return nil
}

// HTTP request body types

type elevenLabsHTTPRequestBody struct {
	Text          string                      `json:"text"`
	ModelID       string                      `json:"model_id,omitempty"`
	VoiceSettings *elevenLabsHTTPVoiceSettings `json:"voice_settings,omitempty"`
	LanguageCode  string                      `json:"language_code,omitempty"`
}

type elevenLabsHTTPVoiceSettings struct {
	Stability       float64 `json:"stability"`
	SimilarityBoost float64 `json:"similarity_boost"`
	Style           float64 `json:"style,omitempty"`
	Speed           float64 `json:"speed,omitempty"`
}

// Ensure ElevenLabsHTTPTTSProvider implements StreamingTTSProvider
var _ StreamingTTSProvider = (*ElevenLabsHTTPTTSProvider)(nil)
