// Unit tests for ElevenLabs Scribe V2 Realtime ASR Provider

package asr

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"
)

func TestElevenLabsProvider_Name(t *testing.T) {
	provider, err := NewElevenLabsProvider(ElevenLabsConfig{
		APIKey: "test-api-key",
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	if provider.Name() != "elevenlabs" {
		t.Errorf("Expected name 'elevenlabs', got '%s'", provider.Name())
	}
}

func TestElevenLabsProvider_SupportsStreaming(t *testing.T) {
	provider, err := NewElevenLabsProvider(ElevenLabsConfig{
		APIKey: "test-api-key",
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	if !provider.SupportsStreaming() {
		t.Error("ElevenLabs provider should support streaming")
	}
}

func TestElevenLabsProvider_SupportedLanguages(t *testing.T) {
	provider, err := NewElevenLabsProvider(ElevenLabsConfig{
		APIKey: "test-api-key",
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	langs := provider.SupportedLanguages()
	if langs == nil || len(langs) == 0 {
		t.Error("SupportedLanguages should return a non-empty slice")
	}

	// Check that common languages are supported
	langMap := make(map[string]bool)
	for _, lang := range langs {
		langMap[lang] = true
	}

	expectedLangs := []string{"en", "zh", "es", "fr", "de", "ja", "ko"}
	for _, expected := range expectedLangs {
		if !langMap[expected] {
			t.Errorf("Expected language '%s' to be supported", expected)
		}
	}
}

func TestNewElevenLabsProvider_NoAPIKey(t *testing.T) {
	_, err := NewElevenLabsProvider(ElevenLabsConfig{})
	if err == nil {
		t.Error("Expected error when API key is empty")
	}

	asrErr, ok := err.(*Error)
	if !ok {
		t.Errorf("Expected *Error, got %T", err)
	} else if asrErr.Code != ErrCodeInvalidConfig {
		t.Errorf("Expected ErrCodeInvalidConfig, got %v", asrErr.Code)
	}
}

func TestNewElevenLabsProvider_DefaultModel(t *testing.T) {
	provider, err := NewElevenLabsProvider(ElevenLabsConfig{
		APIKey: "test-api-key",
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	if provider.model != elevenlabsDefaultModel {
		t.Errorf("Expected default model '%s', got '%s'", elevenlabsDefaultModel, provider.model)
	}
}

func TestNewElevenLabsProvider_CustomModel(t *testing.T) {
	customModel := "custom_model"
	provider, err := NewElevenLabsProvider(ElevenLabsConfig{
		APIKey: "test-api-key",
		Model:  customModel,
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	if provider.model != customModel {
		t.Errorf("Expected model '%s', got '%s'", customModel, provider.model)
	}
}

func TestElevenLabsProvider_Recognize_InvalidSampleRate(t *testing.T) {
	provider, err := NewElevenLabsProvider(ElevenLabsConfig{
		APIKey: "test-api-key",
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	ctx := context.Background()

	// Test with invalid sample rate (not 16kHz)
	audioConfig := AudioConfig{
		SampleRate:    48000, // Wrong sample rate
		Channels:      1,
		Encoding:      "pcm",
		BitsPerSample: 16,
	}
	recognitionConfig := RecognitionConfig{
		Language: "en",
	}

	_, err = provider.Recognize(ctx, bytes.NewReader([]byte{0, 0}), audioConfig, recognitionConfig)
	if err == nil {
		t.Error("Expected error for invalid sample rate")
	}

	asrErr, ok := err.(*Error)
	if ok && asrErr.Code != ErrCodeInvalidConfig {
		t.Errorf("Expected ErrCodeInvalidConfig, got %v", asrErr.Code)
	}
}

func TestElevenLabsProvider_Recognize_EmptyAudio(t *testing.T) {
	provider, err := NewElevenLabsProvider(ElevenLabsConfig{
		APIKey: "test-api-key",
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	ctx := context.Background()
	audioConfig := AudioConfig{
		SampleRate:    16000,
		Channels:      1,
		Encoding:      "pcm",
		BitsPerSample: 16,
	}
	recognitionConfig := RecognitionConfig{
		Language: "en",
	}

	// Test with empty audio
	_, err = provider.Recognize(ctx, bytes.NewReader([]byte{}), audioConfig, recognitionConfig)
	if err == nil {
		t.Error("Expected error for empty audio")
	}

	asrErr, ok := err.(*Error)
	if ok && asrErr.Code != ErrCodeInvalidAudio {
		t.Errorf("Expected ErrCodeInvalidAudio, got %v", asrErr.Code)
	}
}

func TestElevenLabsProvider_StreamingRecognize_InvalidSampleRate(t *testing.T) {
	provider, err := NewElevenLabsProvider(ElevenLabsConfig{
		APIKey: "test-api-key",
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	ctx := context.Background()

	// Test with invalid sample rate
	audioConfig := AudioConfig{
		SampleRate:    44100, // Wrong sample rate
		Channels:      1,
		Encoding:      "pcm",
		BitsPerSample: 16,
	}
	recognitionConfig := RecognitionConfig{
		Language: "en",
	}

	_, err = provider.StreamingRecognize(ctx, audioConfig, recognitionConfig)
	if err == nil {
		t.Error("Expected error for invalid sample rate")
	}

	asrErr, ok := err.(*Error)
	if ok && asrErr.Code != ErrCodeInvalidConfig {
		t.Errorf("Expected ErrCodeInvalidConfig, got %v", asrErr.Code)
	}
}

func TestElevenLabsProvider_Close(t *testing.T) {
	provider, err := NewElevenLabsProvider(ElevenLabsConfig{
		APIKey: "test-api-key",
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	err = provider.Close()
	if err != nil {
		t.Errorf("Failed to close provider: %v", err)
	}
}

func TestNormalizeLanguageCode(t *testing.T) {
	// Create a recognizer to test the normalizeLanguageCode method
	r := &elevenlabsStreamingRecognizer{}

	tests := []struct {
		input    string
		expected string
	}{
		{"zh-CN", "zh"},
		{"zh-TW", "zh"},
		{"en-US", "en"},
		{"en-GB", "en"},
		{"ja-JP", "ja"},
		{"ko-KR", "ko"},
		{"es-ES", "es"},
		{"fr-FR", "fr"},
		{"de-DE", "de"},
		{"en", "en"},
		{"zh", "zh"},
		{"ja", "ja"},
		{"auto", "auto"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := r.normalizeLanguageCode(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeLanguageCode(%s) = %s, expected %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsElevenLabsRecognizer(t *testing.T) {
	// Test with nil
	_, ok := IsElevenLabsRecognizer(nil)
	if ok {
		t.Error("IsElevenLabsRecognizer should return false for nil")
	}

	// Note: We can't easily test with a real recognizer without connecting,
	// but we've verified the interface compliance at compile time via the var _ check
}

// Integration test that requires a valid ElevenLabs API key
func TestElevenLabsProvider_Integration(t *testing.T) {
	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: ELEVENLABS_API_KEY not set")
	}

	provider, err := NewElevenLabsProvider(ElevenLabsConfig{
		APIKey: apiKey,
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	audioConfig := AudioConfig{
		SampleRate:    16000,
		Channels:      1,
		Encoding:      "pcm",
		BitsPerSample: 16,
	}

	recognitionConfig := RecognitionConfig{
		Language:             "en",
		EnablePartialResults: true,
	}

	recognizer, err := provider.StreamingRecognize(ctx, audioConfig, recognitionConfig)
	if err != nil {
		t.Fatalf("Failed to create streaming recognizer: %v", err)
	}
	defer recognizer.Close()

	// Test sending audio (1 second of silence)
	testAudio := make([]byte, 16000*2) // 1 second at 16kHz, 16-bit
	err = recognizer.SendAudio(ctx, testAudio)
	if err != nil {
		t.Errorf("Failed to send audio: %v", err)
	}

	// Test commit
	if er, ok := IsElevenLabsRecognizer(recognizer); ok {
		err = er.Commit(ctx)
		if err != nil {
			t.Errorf("Failed to commit: %v", err)
		}
	}

	// Wait for potential results
	resultsChan := recognizer.Results()
	if resultsChan == nil {
		t.Error("Results channel should not be nil")
	}

	// Give some time for processing
	select {
	case result := <-resultsChan:
		t.Logf("Received result: %+v", result)
	case <-time.After(5 * time.Second):
		t.Log("No result received (expected for silence)")
	}

	t.Log("Integration test completed successfully")
}
