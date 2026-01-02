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

	expectedVoices := []string{"alloy", "echo", "fable", "onyx", "nova", "shimmer"}
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

	if defaultVoice != "alloy" {
		t.Errorf("Expected default voice 'alloy', got '%s'", defaultVoice)
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

	// Test setting to HD model
	provider.SetModel("tts-1-hd")
	if provider.model != "tts-1-hd" {
		t.Errorf("Expected model 'tts-1-hd', got '%s'", provider.model)
	}

	// Test setting back to standard model
	provider.SetModel("tts-1")
	if provider.model != "tts-1" {
		t.Errorf("Expected model 'tts-1', got '%s'", provider.model)
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
			name: "Basic synthesis",
			request: &SynthesizeRequest{
				Text:  "Hello, this is a test.",
				Voice: "alloy",
			},
		},
		{
			name: "With custom voice",
			request: &SynthesizeRequest{
				Text:  "Testing with Nova voice.",
				Voice: "nova",
			},
		},
		{
			name: "With speed option",
			request: &SynthesizeRequest{
				Text:  "Testing with speed control.",
				Voice: "alloy",
				Options: map[string]interface{}{
					"speed": 1.5,
				},
			},
		},
		{
			name: "With opus format",
			request: &SynthesizeRequest{
				Text:  "Testing opus format.",
				Voice: "alloy",
				Options: map[string]interface{}{
					"format": "opus",
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

func TestNewOpenAITTSProviderHD(t *testing.T) {
	provider := NewOpenAITTSProviderHD("test-key")

	if provider.model != "tts-1-hd" {
		t.Errorf("Expected HD provider to use 'tts-1-hd', got '%s'", provider.model)
	}
}
