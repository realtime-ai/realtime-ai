// ElevenLabs WebSocket TTS Provider
//
// Implements StreamingTTSProvider using ElevenLabs WebSocket API for low-latency
// text-to-speech synthesis. Outputs 16kHz mono PCM audio.
//
// Reference: https://elevenlabs.io/docs/api-reference/text-to-speech/v-1-text-to-speech-voice-id-stream-input

package tts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

const (
	elevenLabsWSEndpoint     = "wss://api.elevenlabs.io/v1/text-to-speech"
	elevenLabsDefaultModel   = "eleven_turbo_v2_5"
	elevenLabsOutputFormat   = "pcm_16000" // 16kHz mono PCM
	elevenLabsSampleRate     = 16000
	elevenLabsConnectTimeout = 10 * time.Second
)

// ElevenLabs supported voices (partial list - use API to get full list)
var elevenLabsVoices = []string{
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

// ElevenLabsWSTTSConfig holds the configuration for ElevenLabs WebSocket TTS
type ElevenLabsWSTTSConfig struct {
	APIKey  string  // Required: ElevenLabs API key
	VoiceID string  // Required: Voice ID to use
	Model   string  // Optional: Model ID (default: eleven_turbo_v2_5)
	Speed   float64 // Optional: Speed 0.7-1.2 (default: 1.0)
}

// ElevenLabsWSTTSProvider implements StreamingTTSProvider using WebSocket
type ElevenLabsWSTTSProvider struct {
	apiKey  string
	voiceID string
	model   string
	speed   float64

	mu sync.RWMutex
}

// NewElevenLabsWSTTSProvider creates a new ElevenLabs WebSocket TTS provider
func NewElevenLabsWSTTSProvider(config ElevenLabsWSTTSConfig) (*ElevenLabsWSTTSProvider, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("ElevenLabs API key is required")
	}
	if config.VoiceID == "" {
		return nil, fmt.Errorf("ElevenLabs Voice ID is required")
	}

	model := config.Model
	if model == "" {
		model = elevenLabsDefaultModel
	}

	speed := config.Speed
	if speed == 0 {
		speed = 1.0
	}

	return &ElevenLabsWSTTSProvider{
		apiKey:  config.APIKey,
		voiceID: config.VoiceID,
		model:   model,
		speed:   speed,
	}, nil
}

// Name returns the provider name
func (p *ElevenLabsWSTTSProvider) Name() string {
	return "elevenlabs-ws"
}

// Synthesize converts text to speech (batch mode - collects all audio)
func (p *ElevenLabsWSTTSProvider) Synthesize(ctx context.Context, req *SynthesizeRequest) (*SynthesizeResponse, error) {
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
						SampleRate: elevenLabsSampleRate,
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
func (p *ElevenLabsWSTTSProvider) StreamSynthesize(ctx context.Context, req *SynthesizeRequest) (<-chan []byte, <-chan error) {
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

// doStreamSynthesize performs the actual WebSocket streaming
func (p *ElevenLabsWSTTSProvider) doStreamSynthesize(ctx context.Context, req *SynthesizeRequest, audioChan chan<- []byte) error {
	// Build WebSocket URL
	voiceID := req.Voice
	if voiceID == "" {
		voiceID = p.voiceID
	}

	params := url.Values{}
	params.Set("model_id", p.model)
	params.Set("output_format", elevenLabsOutputFormat)

	wsURL := fmt.Sprintf("%s/%s/stream-input?%s", elevenLabsWSEndpoint, voiceID, params.Encode())

	log.Printf("[ElevenLabs-TTS] Connecting to %s", wsURL)

	// Create WebSocket dialer with timeout
	dialer := websocket.Dialer{
		HandshakeTimeout: elevenLabsConnectTimeout,
	}

	// Set headers
	headers := http.Header{}
	headers.Set("xi-api-key", p.apiKey)

	// Connect
	conn, _, err := dialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		return fmt.Errorf("failed to connect to ElevenLabs WebSocket: %w", err)
	}
	defer conn.Close()

	log.Printf("[ElevenLabs-TTS] WebSocket connected")

	// Track connection state
	var closed atomic.Bool
	var wg sync.WaitGroup

	// Start read loop
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.readLoop(ctx, conn, audioChan, &closed)
	}()

	// Send initialization message (BOS - Beginning of Stream)
	initMsg := elevenlabsTTSInitMessage{
		Text:   " ", // Required: single space to initialize
		APIKey: p.apiKey,
		VoiceSettings: &elevenlabsVoiceSettings{
			Stability:       0.5,
			SimilarityBoost: 0.75,
			Speed:           p.speed,
		},
	}

	if err := conn.WriteJSON(initMsg); err != nil {
		closed.Store(true)
		return fmt.Errorf("failed to send init message: %w", err)
	}

	log.Printf("[ElevenLabs-TTS] Sent init message")

	// Send the text content
	textMsg := elevenlabsTTSTextMessage{
		Text:                 req.Text + " ", // Add trailing space as recommended
		TryTriggerGeneration: true,
	}

	if err := conn.WriteJSON(textMsg); err != nil {
		closed.Store(true)
		return fmt.Errorf("failed to send text message: %w", err)
	}

	log.Printf("[ElevenLabs-TTS] Sent text: %d chars", len(req.Text))

	// Send EOS (End of Stream) with flush
	eosMsg := elevenlabsTTSTextMessage{
		Text:  "",
		Flush: true,
	}

	if err := conn.WriteJSON(eosMsg); err != nil {
		closed.Store(true)
		return fmt.Errorf("failed to send EOS message: %w", err)
	}

	log.Printf("[ElevenLabs-TTS] Sent EOS message")

	// Wait for read loop to complete
	wg.Wait()

	return nil
}

// readLoop reads audio chunks from WebSocket
func (p *ElevenLabsWSTTSProvider) readLoop(ctx context.Context, conn *websocket.Conn, audioChan chan<- []byte, closed *atomic.Bool) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			if !closed.Load() && !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("[ElevenLabs-TTS] WebSocket read error: %v", err)
			}
			return
		}

		// Parse response
		var resp elevenlabsTTSResponse
		if err := json.Unmarshal(message, &resp); err != nil {
			log.Printf("[ElevenLabs-TTS] Failed to parse response: %v", err)
			continue
		}

		// Check for final message
		if resp.IsFinal {
			log.Printf("[ElevenLabs-TTS] Received final message")
			return
		}

		// Decode and send audio
		if resp.Audio != "" {
			audioData, err := base64.StdEncoding.DecodeString(resp.Audio)
			if err != nil {
				log.Printf("[ElevenLabs-TTS] Failed to decode audio: %v", err)
				continue
			}

			select {
			case audioChan <- audioData:
			case <-ctx.Done():
				return
			}
		}
	}
}

// GetSupportedVoices returns a list of known voice IDs
func (p *ElevenLabsWSTTSProvider) GetSupportedVoices() []string {
	return elevenLabsVoices
}

// GetDefaultVoice returns the configured voice ID
func (p *ElevenLabsWSTTSProvider) GetDefaultVoice() string {
	return p.voiceID
}

// ValidateConfig validates the provider configuration
func (p *ElevenLabsWSTTSProvider) ValidateConfig() error {
	if p.apiKey == "" {
		return fmt.Errorf("ElevenLabs API key is not set")
	}
	if p.voiceID == "" {
		return fmt.Errorf("ElevenLabs Voice ID is not set")
	}
	return nil
}

// WebSocket message types

type elevenlabsTTSInitMessage struct {
	Text          string                  `json:"text"`
	APIKey        string                  `json:"xi-api-key,omitempty"`
	VoiceSettings *elevenlabsVoiceSettings `json:"voice_settings,omitempty"`
}

type elevenlabsVoiceSettings struct {
	Stability       float64 `json:"stability"`
	SimilarityBoost float64 `json:"similarity_boost"`
	Style           float64 `json:"style,omitempty"`
	Speed           float64 `json:"speed,omitempty"`
}

type elevenlabsTTSTextMessage struct {
	Text                 string `json:"text"`
	TryTriggerGeneration bool   `json:"try_trigger_generation,omitempty"`
	Flush                bool   `json:"flush,omitempty"`
}

type elevenlabsTTSResponse struct {
	Audio   string `json:"audio,omitempty"`
	IsFinal bool   `json:"isFinal,omitempty"`
}

// Ensure ElevenLabsWSTTSProvider implements StreamingTTSProvider
var _ StreamingTTSProvider = (*ElevenLabsWSTTSProvider)(nil)
