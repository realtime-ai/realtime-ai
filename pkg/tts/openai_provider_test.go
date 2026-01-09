package tts

import (
	"context"
	"os"
	"testing"

	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

func TestOpenAITTSProvider_Name(t *testing.T) {
	provider := NewOpenAITTSProvider("test-key")
	if provider.Name() != "openai" {
		t.Errorf("Expected name 'openai', got '%s'", provider.Name())
	}
}

func TestOpenAITTSProvider_GetSupportedVoices(t *testing.T) {
	provider := NewOpenAITTSProvider("test-key")
	voices := provider.GetSupportedVoices()

	expectedVoices := []string{
		"alloy", "ash", "ballad", "coral", "echo", "fable",
		"nova", "onyx", "sage", "shimmer", "verse", "marin", "cedar",
	}
	if len(voices) != len(expectedVoices) {
		t.Errorf("Expected %d voices, got %d", len(expectedVoices), len(voices))
	}

	// Check if all expected voices are present
	voiceMap := make(map[string]bool)
	for _, v := range voices {
		voiceMap[v] = true
	}

	for _, expected := range expectedVoices {
		if !voiceMap[expected] {
			t.Errorf("Expected voice '%s' not found", expected)
		}
	}
}

func TestOpenAITTSProvider_GetDefaultVoice(t *testing.T) {
	provider := NewOpenAITTSProvider("test-key")
	defaultVoice := provider.GetDefaultVoice()

	if defaultVoice != "coral" {
		t.Errorf("Expected default voice 'coral', got '%s'", defaultVoice)
	}
}

func TestOpenAITTSProvider_ValidateConfig(t *testing.T) {
	tests := []struct {
		name      string
		apiKey    string
		wantError bool
	}{
		{
			name:      "Valid API key",
			apiKey:    "sk-test-key",
			wantError: false,
		},
		{
			name:      "Empty API key",
			apiKey:    "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and clear OPENAI_API_KEY to ensure test isolation
			originalKey := os.Getenv("OPENAI_API_KEY")
			os.Unsetenv("OPENAI_API_KEY")
			defer func() {
				if originalKey != "" {
					os.Setenv("OPENAI_API_KEY", originalKey)
				}
			}()

			provider := NewOpenAITTSProvider(tt.apiKey)
			err := provider.ValidateConfig()

			if tt.wantError && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
		})
	}
}

func TestOpenAITTSProvider_SetModel(t *testing.T) {
	provider := NewOpenAITTSProvider("test-key")

	// Default model should be gpt-4o-mini-tts
	if provider.model != "gpt-4o-mini-tts" {
		t.Errorf("Expected default model 'gpt-4o-mini-tts', got '%s'", provider.model)
	}

	// Test setting to latest snapshot
	provider.SetModel("gpt-4o-mini-tts-2025-12-15")
	if provider.model != "gpt-4o-mini-tts-2025-12-15" {
		t.Errorf("Expected model 'gpt-4o-mini-tts-2025-12-15', got '%s'", provider.model)
	}
}

func TestOpenAITTSProvider_Instructions(t *testing.T) {
	provider := NewOpenAITTSProvider("test-key")

	// Default instructions should be empty
	if provider.GetInstructions() != "" {
		t.Errorf("Expected empty default instructions, got '%s'", provider.GetInstructions())
	}

	// Test setting instructions
	testInstructions := "Speak in a cheerful and positive tone"
	provider.SetInstructions(testInstructions)
	if provider.GetInstructions() != testInstructions {
		t.Errorf("Expected instructions '%s', got '%s'", testInstructions, provider.GetInstructions())
	}
}

func TestOpenAITTSProvider_GetAudioFormat(t *testing.T) {
	provider := NewOpenAITTSProvider("test-key")

	tests := []struct {
		format        string
		expectedMedia pipeline.AudioMediaType
		expectedRate  int
		expectedEnc   string
	}{
		{"pcm", pipeline.AudioMediaTypePCM, 24000, "pcm_s16le"},
		{"opus", pipeline.AudioMediaTypeOpusStandard, 24000, "opus"},
		{"mp3", pipeline.AudioMediaTypeMPEG, 24000, "mp3"},
		{"wav", pipeline.AudioMediaTypeWAV, 24000, "wav"},
		{"unknown", pipeline.AudioMediaTypePCM, 24000, "pcm_s16le"}, // defaults to PCM
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			format := provider.getAudioFormat(tt.format)

			if format.MediaType != tt.expectedMedia {
				t.Errorf("Expected media type '%s', got '%s'", tt.expectedMedia, format.MediaType)
			}
			if format.SampleRate != tt.expectedRate {
				t.Errorf("Expected sample rate %d, got %d", tt.expectedRate, format.SampleRate)
			}
			if format.Encoding != tt.expectedEnc {
				t.Errorf("Expected encoding '%s', got '%s'", tt.expectedEnc, format.Encoding)
			}
			if format.Channels != 1 {
				t.Errorf("Expected 1 channel, got %d", format.Channels)
			}
		})
	}
}

func TestOpenAITTSProvider_ImplementsStreamingInterface(t *testing.T) {
	provider := NewOpenAITTSProvider("test-key")

	// Verify provider implements StreamingTTSProvider
	var _ StreamingTTSProvider = provider
}

// Integration test - only runs if OPENAI_API_KEY is set
func TestOpenAITTSProvider_Synthesize_Integration(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: OPENAI_API_KEY not set")
	}

	provider := NewOpenAITTSProvider(apiKey)

	tests := []struct {
		name    string
		request *SynthesizeRequest
	}{
		{
			name: "Basic synthesis with coral voice",
			request: &SynthesizeRequest{
				Text:  "Hello, this is a test.",
				Voice: "coral",
			},
		},
		{
			name: "With marin voice (recommended)",
			request: &SynthesizeRequest{
				Text:  "Testing with marin voice.",
				Voice: "marin",
			},
		},
		{
			name: "With speed option",
			request: &SynthesizeRequest{
				Text:  "Testing with speed control.",
				Voice: "coral",
				Options: map[string]interface{}{
					"speed": 1.5,
				},
			},
		},
		{
			name: "With instructions",
			request: &SynthesizeRequest{
				Text:  "I am so excited to tell you this news!",
				Voice: "coral",
				Options: map[string]interface{}{
					"instructions": "Speak in a cheerful and enthusiastic tone",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			resp, err := provider.Synthesize(ctx, tt.request)

			if err != nil {
				t.Fatalf("Synthesize failed: %v", err)
			}

			if len(resp.AudioData) == 0 {
				t.Error("Expected audio data, got empty")
			}

			if resp.AudioFormat.SampleRate == 0 {
				t.Error("Expected sample rate to be set")
			}

			if resp.AudioFormat.Channels == 0 {
				t.Error("Expected channels to be set")
			}

			t.Logf("Synthesized %d bytes of audio at %d Hz",
				len(resp.AudioData), resp.AudioFormat.SampleRate)
		})
	}
}

// Integration test for streaming - only runs if OPENAI_API_KEY is set
func TestOpenAITTSProvider_StreamSynthesize_Integration(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: OPENAI_API_KEY not set")
	}

	provider := NewOpenAITTSProvider(apiKey)
	provider.SetInstructions("Speak calmly and clearly")

	ctx := context.Background()
	req := &SynthesizeRequest{
		Text:  "This is a streaming test. The audio should arrive in chunks.",
		Voice: "coral",
	}

	audioChan, errChan := provider.StreamSynthesize(ctx, req)

	var totalBytes int
	var chunkCount int

	for {
		select {
		case chunk, ok := <-audioChan:
			if !ok {
				// Channel closed, check for errors
				select {
				case err := <-errChan:
					if err != nil {
						t.Fatalf("StreamSynthesize error: %v", err)
					}
				default:
				}
				t.Logf("Stream completed: %d chunks, %d total bytes", chunkCount, totalBytes)
				if totalBytes == 0 {
					t.Error("Expected audio data, got empty")
				}
				return
			}
			totalBytes += len(chunk)
			chunkCount++

		case err := <-errChan:
			if err != nil {
				t.Fatalf("StreamSynthesize error: %v", err)
			}
		}
	}
}
