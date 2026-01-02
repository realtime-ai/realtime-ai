// Unit tests for ElevenLabs WebSocket TTS Provider

package tts

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestNewElevenLabsWSTTSProvider(t *testing.T) {
	tests := []struct {
		name    string
		config  ElevenLabsWSTTSConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: ElevenLabsWSTTSConfig{
				APIKey:  "test-api-key",
				VoiceID: "test-voice-id",
			},
			wantErr: false,
		},
		{
			name: "valid config with model",
			config: ElevenLabsWSTTSConfig{
				APIKey:  "test-api-key",
				VoiceID: "test-voice-id",
				Model:   "eleven_multilingual_v2",
			},
			wantErr: false,
		},
		{
			name: "missing API key",
			config: ElevenLabsWSTTSConfig{
				VoiceID: "test-voice-id",
			},
			wantErr: true,
		},
		{
			name: "missing voice ID",
			config: ElevenLabsWSTTSConfig{
				APIKey: "test-api-key",
			},
			wantErr: true,
		},
		{
			name:    "empty config",
			config:  ElevenLabsWSTTSConfig{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewElevenLabsWSTTSProvider(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewElevenLabsWSTTSProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && provider == nil {
				t.Error("NewElevenLabsWSTTSProvider() returned nil provider")
			}
		})
	}
}

func TestElevenLabsWSTTSProvider_Name(t *testing.T) {
	provider, err := NewElevenLabsWSTTSProvider(ElevenLabsWSTTSConfig{
		APIKey:  "test-api-key",
		VoiceID: "test-voice-id",
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	if provider.Name() != "elevenlabs-ws" {
		t.Errorf("Expected name 'elevenlabs-ws', got '%s'", provider.Name())
	}
}

func TestElevenLabsWSTTSProvider_GetSupportedVoices(t *testing.T) {
	provider, err := NewElevenLabsWSTTSProvider(ElevenLabsWSTTSConfig{
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

func TestElevenLabsWSTTSProvider_GetDefaultVoice(t *testing.T) {
	voiceID := "custom-voice-id"
	provider, err := NewElevenLabsWSTTSProvider(ElevenLabsWSTTSConfig{
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

func TestElevenLabsWSTTSProvider_ValidateConfig(t *testing.T) {
	// Valid config
	provider, _ := NewElevenLabsWSTTSProvider(ElevenLabsWSTTSConfig{
		APIKey:  "test-api-key",
		VoiceID: "test-voice-id",
	})
	if err := provider.ValidateConfig(); err != nil {
		t.Errorf("ValidateConfig() should pass for valid config: %v", err)
	}

	// Invalid - empty API key
	provider2 := &ElevenLabsWSTTSProvider{
		voiceID: "test-voice-id",
	}
	if err := provider2.ValidateConfig(); err == nil {
		t.Error("ValidateConfig() should fail for empty API key")
	}

	// Invalid - empty voice ID
	provider3 := &ElevenLabsWSTTSProvider{
		apiKey: "test-api-key",
	}
	if err := provider3.ValidateConfig(); err == nil {
		t.Error("ValidateConfig() should fail for empty voice ID")
	}
}

func TestElevenLabsWSTTSProvider_DefaultModel(t *testing.T) {
	provider, err := NewElevenLabsWSTTSProvider(ElevenLabsWSTTSConfig{
		APIKey:  "test-api-key",
		VoiceID: "test-voice-id",
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	if provider.model != elevenLabsDefaultModel {
		t.Errorf("Expected default model '%s', got '%s'", elevenLabsDefaultModel, provider.model)
	}
}

func TestElevenLabsWSTTSProvider_CustomModel(t *testing.T) {
	customModel := "eleven_multilingual_v2"
	provider, err := NewElevenLabsWSTTSProvider(ElevenLabsWSTTSConfig{
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

func TestElevenLabsWSTTSProvider_DefaultSpeed(t *testing.T) {
	provider, err := NewElevenLabsWSTTSProvider(ElevenLabsWSTTSConfig{
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

func TestElevenLabsWSTTSProvider_CustomSpeed(t *testing.T) {
	customSpeed := 1.2
	provider, err := NewElevenLabsWSTTSProvider(ElevenLabsWSTTSConfig{
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

// Interface compliance test
func TestElevenLabsWSTTSProvider_ImplementsStreamingTTSProvider(t *testing.T) {
	provider, err := NewElevenLabsWSTTSProvider(ElevenLabsWSTTSConfig{
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
func TestElevenLabsWSTTSProvider_Integration(t *testing.T) {
	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: ELEVENLABS_API_KEY not set")
	}

	// Use a known voice ID (Rachel)
	voiceID := "21m00Tcm4TlvDq8ikWAM"

	provider, err := NewElevenLabsWSTTSProvider(ElevenLabsWSTTSConfig{
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
		Text: "Hello, this is a test of the ElevenLabs text to speech API.",
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
		duration := float64(totalBytes) / float64(elevenLabsSampleRate*2)
		t.Logf("Total audio: %d bytes (%d chunks), ~%.1f seconds", totalBytes, chunkCount, duration)
	}
}

// Integration test for batch Synthesize
func TestElevenLabsWSTTSProvider_Integration_Batch(t *testing.T) {
	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: ELEVENLABS_API_KEY not set")
	}

	voiceID := "21m00Tcm4TlvDq8ikWAM"

	provider, err := NewElevenLabsWSTTSProvider(ElevenLabsWSTTSConfig{
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

	if resp.AudioFormat.SampleRate != elevenLabsSampleRate {
		t.Errorf("Expected sample rate %d, got %d", elevenLabsSampleRate, resp.AudioFormat.SampleRate)
	}

	if resp.AudioFormat.Channels != 1 {
		t.Errorf("Expected 1 channel, got %d", resp.AudioFormat.Channels)
	}

	duration := float64(len(resp.AudioData)) / float64(elevenLabsSampleRate*2)
	t.Logf("Synthesized audio: %d bytes, ~%.1f seconds", len(resp.AudioData), duration)
}
