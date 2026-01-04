// Unit tests for ElevenLabs HTTP TTS Provider

package tts

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestNewElevenLabsHTTPTTSProvider(t *testing.T) {
	tests := []struct {
		name    string
		config  ElevenLabsHTTPTTSConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: ElevenLabsHTTPTTSConfig{
				APIKey:  "test-api-key",
				VoiceID: "test-voice-id",
			},
			wantErr: false,
		},
		{
			name: "valid config with model",
			config: ElevenLabsHTTPTTSConfig{
				APIKey:  "test-api-key",
				VoiceID: "test-voice-id",
				Model:   "eleven_multilingual_v2",
			},
			wantErr: false,
		},
		{
			name: "valid config with all options",
			config: ElevenLabsHTTPTTSConfig{
				APIKey:              "test-api-key",
				VoiceID:             "test-voice-id",
				Model:               "eleven_turbo_v2_5",
				Speed:               1.2,
				LatencyOptimization: 4,
				Stability:           0.7,
				SimilarityBoost:     0.8,
			},
			wantErr: false,
		},
		{
			name: "missing API key",
			config: ElevenLabsHTTPTTSConfig{
				VoiceID: "test-voice-id",
			},
			wantErr: true,
		},
		{
			name: "missing voice ID",
			config: ElevenLabsHTTPTTSConfig{
				APIKey: "test-api-key",
			},
			wantErr: true,
		},
		{
			name:    "empty config",
			config:  ElevenLabsHTTPTTSConfig{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewElevenLabsHTTPTTSProvider(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewElevenLabsHTTPTTSProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && provider == nil {
				t.Error("NewElevenLabsHTTPTTSProvider() returned nil provider")
			}
		})
	}
}

func TestElevenLabsHTTPTTSProvider_Name(t *testing.T) {
	provider, err := NewElevenLabsHTTPTTSProvider(ElevenLabsHTTPTTSConfig{
		APIKey:  "test-api-key",
		VoiceID: "test-voice-id",
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	if provider.Name() != "elevenlabs-http" {
		t.Errorf("Expected name 'elevenlabs-http', got '%s'", provider.Name())
	}
}

func TestElevenLabsHTTPTTSProvider_GetSupportedVoices(t *testing.T) {
	provider, err := NewElevenLabsHTTPTTSProvider(ElevenLabsHTTPTTSConfig{
		APIKey:  "test-api-key",
		VoiceID: "test-voice-id",
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	voices := provider.GetSupportedVoices()
	if len(voices) == 0 {
		t.Error("GetSupportedVoices() returned empty list")
	}
}

func TestElevenLabsHTTPTTSProvider_GetDefaultVoice(t *testing.T) {
	voiceID := "custom-voice-id"
	provider, err := NewElevenLabsHTTPTTSProvider(ElevenLabsHTTPTTSConfig{
		APIKey:  "test-api-key",
		VoiceID: voiceID,
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	if provider.GetDefaultVoice() != voiceID {
		t.Errorf("Expected default voice '%s', got '%s'", voiceID, provider.GetDefaultVoice())
	}
}

func TestElevenLabsHTTPTTSProvider_ValidateConfig(t *testing.T) {
	// Valid config
	provider, _ := NewElevenLabsHTTPTTSProvider(ElevenLabsHTTPTTSConfig{
		APIKey:  "test-api-key",
		VoiceID: "test-voice-id",
	})
	if err := provider.ValidateConfig(); err != nil {
		t.Errorf("ValidateConfig() should pass for valid config: %v", err)
	}

	// Invalid - empty API key
	provider2 := &ElevenLabsHTTPTTSProvider{
		voiceID: "test-voice-id",
	}
	if err := provider2.ValidateConfig(); err == nil {
		t.Error("ValidateConfig() should fail for empty API key")
	}

	// Invalid - empty voice ID
	provider3 := &ElevenLabsHTTPTTSProvider{
		apiKey: "test-api-key",
	}
	if err := provider3.ValidateConfig(); err == nil {
		t.Error("ValidateConfig() should fail for empty voice ID")
	}
}

func TestElevenLabsHTTPTTSProvider_DefaultModel(t *testing.T) {
	provider, err := NewElevenLabsHTTPTTSProvider(ElevenLabsHTTPTTSConfig{
		APIKey:  "test-api-key",
		VoiceID: "test-voice-id",
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	if provider.model != elevenLabsHTTPDefaultModel {
		t.Errorf("Expected default model '%s', got '%s'", elevenLabsHTTPDefaultModel, provider.model)
	}
}

func TestElevenLabsHTTPTTSProvider_CustomModel(t *testing.T) {
	customModel := "eleven_turbo_v2_5"
	provider, err := NewElevenLabsHTTPTTSProvider(ElevenLabsHTTPTTSConfig{
		APIKey:  "test-api-key",
		VoiceID: "test-voice-id",
		Model:   customModel,
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	if provider.model != customModel {
		t.Errorf("Expected model '%s', got '%s'", customModel, provider.model)
	}
}

func TestElevenLabsHTTPTTSProvider_DefaultSpeed(t *testing.T) {
	provider, err := NewElevenLabsHTTPTTSProvider(ElevenLabsHTTPTTSConfig{
		APIKey:  "test-api-key",
		VoiceID: "test-voice-id",
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	if provider.speed != 1.0 {
		t.Errorf("Expected default speed 1.0, got %f", provider.speed)
	}
}

func TestElevenLabsHTTPTTSProvider_CustomSpeed(t *testing.T) {
	customSpeed := 1.2
	provider, err := NewElevenLabsHTTPTTSProvider(ElevenLabsHTTPTTSConfig{
		APIKey:  "test-api-key",
		VoiceID: "test-voice-id",
		Speed:   customSpeed,
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	if provider.speed != customSpeed {
		t.Errorf("Expected speed %f, got %f", customSpeed, provider.speed)
	}
}

func TestElevenLabsHTTPTTSProvider_DefaultLatencyOptimization(t *testing.T) {
	provider, err := NewElevenLabsHTTPTTSProvider(ElevenLabsHTTPTTSConfig{
		APIKey:  "test-api-key",
		VoiceID: "test-voice-id",
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	if provider.latencyOptimization != elevenLabsHTTPLatencyOptimize {
		t.Errorf("Expected default latency optimization %d, got %d", elevenLabsHTTPLatencyOptimize, provider.latencyOptimization)
	}
}

func TestElevenLabsHTTPTTSProvider_CustomLatencyOptimization(t *testing.T) {
	customLatency := 4
	provider, err := NewElevenLabsHTTPTTSProvider(ElevenLabsHTTPTTSConfig{
		APIKey:              "test-api-key",
		VoiceID:             "test-voice-id",
		LatencyOptimization: customLatency,
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	if provider.latencyOptimization != customLatency {
		t.Errorf("Expected latency optimization %d, got %d", customLatency, provider.latencyOptimization)
	}
}

func TestElevenLabsHTTPTTSProvider_VoiceSettings(t *testing.T) {
	stability := 0.7
	similarityBoost := 0.8
	provider, err := NewElevenLabsHTTPTTSProvider(ElevenLabsHTTPTTSConfig{
		APIKey:          "test-api-key",
		VoiceID:         "test-voice-id",
		Stability:       stability,
		SimilarityBoost: similarityBoost,
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	if provider.stability != stability {
		t.Errorf("Expected stability %f, got %f", stability, provider.stability)
	}
	if provider.similarityBoost != similarityBoost {
		t.Errorf("Expected similarity boost %f, got %f", similarityBoost, provider.similarityBoost)
	}
}

// Interface compliance test
func TestElevenLabsHTTPTTSProvider_ImplementsStreamingTTSProvider(t *testing.T) {
	provider, err := NewElevenLabsHTTPTTSProvider(ElevenLabsHTTPTTSConfig{
		APIKey:  "test-api-key",
		VoiceID: "test-voice-id",
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	// Check that it implements StreamingTTSProvider
	var _ StreamingTTSProvider = provider
}

// Integration test that requires a valid ElevenLabs API key
func TestElevenLabsHTTPTTSProvider_Integration(t *testing.T) {
	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: ELEVENLABS_API_KEY not set")
	}

	// Use a known voice ID (Rachel)
	voiceID := "21m00Tcm4TlvDq8ikWAM"

	provider, err := NewElevenLabsHTTPTTSProvider(ElevenLabsHTTPTTSConfig{
		APIKey:  apiKey,
		VoiceID: voiceID,
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test StreamSynthesize
	req := &SynthesizeRequest{
		Text: "Hello, this is a test of the ElevenLabs HTTP streaming text to speech API.",
	}

	audioChan, errChan := provider.StreamSynthesize(ctx, req)

	var totalBytes int
	var chunkCount int

	for {
		select {
		case chunk, ok := <-audioChan:
			if !ok {
				// Channel closed
				select {
				case err := <-errChan:
					if err != nil {
						t.Fatalf("StreamSynthesize error: %v", err)
					}
				default:
				}
				goto done
			}
			totalBytes += len(chunk)
			chunkCount++
			t.Logf("Received chunk %d: %d bytes", chunkCount, len(chunk))

		case err := <-errChan:
			if err != nil {
				t.Fatalf("StreamSynthesize error: %v", err)
			}

		case <-ctx.Done():
			t.Fatalf("Context cancelled: %v", ctx.Err())
		}
	}

done:
	if totalBytes == 0 {
		t.Error("No audio data received")
	} else {
		// Calculate duration (16kHz, 16-bit mono = 32000 bytes/sec)
		duration := float64(totalBytes) / float64(elevenLabsHTTPSampleRate*2)
		t.Logf("Total audio: %d bytes (%d chunks), ~%.1f seconds", totalBytes, chunkCount, duration)
	}
}

// Integration test for batch Synthesize
func TestElevenLabsHTTPTTSProvider_Integration_Batch(t *testing.T) {
	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: ELEVENLABS_API_KEY not set")
	}

	voiceID := "21m00Tcm4TlvDq8ikWAM"

	provider, err := NewElevenLabsHTTPTTSProvider(ElevenLabsHTTPTTSConfig{
		APIKey:  apiKey,
		VoiceID: voiceID,
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := &SynthesizeRequest{
		Text: "Hello world.",
	}

	resp, err := provider.Synthesize(ctx, req)
	if err != nil {
		t.Fatalf("Synthesize error: %v", err)
	}

	if len(resp.AudioData) == 0 {
		t.Error("No audio data received")
	}

	if resp.AudioFormat.SampleRate != elevenLabsHTTPSampleRate {
		t.Errorf("Expected sample rate %d, got %d", elevenLabsHTTPSampleRate, resp.AudioFormat.SampleRate)
	}

	if resp.AudioFormat.Channels != 1 {
		t.Errorf("Expected 1 channel, got %d", resp.AudioFormat.Channels)
	}

	duration := float64(len(resp.AudioData)) / float64(elevenLabsHTTPSampleRate*2)
	t.Logf("Synthesized audio: %d bytes, ~%.1f seconds", len(resp.AudioData), duration)
}
