package asr

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"
)

func TestWhisperProvider_Name(t *testing.T) {
	provider, err := NewWhisperProvider("test-api-key")
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	if provider.Name() != "openai-whisper" {
		t.Errorf("Expected name 'openai-whisper', got '%s'", provider.Name())
	}
}

func TestWhisperProvider_SupportsStreaming(t *testing.T) {
	provider, err := NewWhisperProvider("test-api-key")
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	if !provider.SupportsStreaming() {
		t.Error("Whisper provider should support streaming")
	}
}

func TestWhisperProvider_SupportedLanguages(t *testing.T) {
	provider, err := NewWhisperProvider("test-api-key")
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	// Should return empty slice indicating all languages supported
	langs := provider.SupportedLanguages()
	if langs == nil {
		t.Error("SupportedLanguages should return empty slice, not nil")
	}
}

func TestNewWhisperProvider_NoAPIKey(t *testing.T) {
	_, err := NewWhisperProvider("")
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

func TestConvertPCMToWAV(t *testing.T) {
	// Create simple PCM data (1 second of silence at 16kHz, mono, 16-bit)
	sampleRate := 16000
	channels := 1
	bitsPerSample := 16
	duration := 1 * time.Second

	samples := sampleRate * int(duration.Seconds())
	pcmData := make([]byte, samples*channels*bitsPerSample/8)

	config := AudioConfig{
		SampleRate:    sampleRate,
		Channels:      channels,
		Encoding:      "pcm",
		BitsPerSample: bitsPerSample,
	}

	wavData, err := convertPCMToWAV(pcmData, config)
	if err != nil {
		t.Fatalf("Failed to convert PCM to WAV: %v", err)
	}

	// Check WAV header
	if len(wavData) < 44 {
		t.Errorf("WAV data too short: %d bytes", len(wavData))
	}

	// Check RIFF header
	if string(wavData[0:4]) != "RIFF" {
		t.Error("Invalid RIFF header")
	}

	// Check WAVE format
	if string(wavData[8:12]) != "WAVE" {
		t.Error("Invalid WAVE format")
	}

	// Check fmt chunk
	if string(wavData[12:16]) != "fmt " {
		t.Error("Invalid fmt chunk")
	}

	// Check data chunk
	if string(wavData[36:40]) != "data" {
		t.Error("Invalid data chunk")
	}
}

func TestWhisperProvider_Recognize_EmptyAudio(t *testing.T) {
	provider, err := NewWhisperProvider("test-api-key")
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
		Model:    "whisper-1",
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

// TestWhisperProvider_Recognize_Integration is an integration test that requires
// a valid OpenAI API key. It is skipped by default.
func TestWhisperProvider_Recognize_Integration(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: OPENAI_API_KEY not set")
	}

	provider, err := NewWhisperProvider(apiKey)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Close()

	// Create a simple test audio (1 second of silence)
	// In a real test, you would use actual speech audio
	sampleRate := 16000
	channels := 1
	bitsPerSample := 16
	duration := 1 * time.Second

	samples := sampleRate * int(duration.Seconds())
	pcmData := make([]byte, samples*channels*bitsPerSample/8)

	audioConfig := AudioConfig{
		SampleRate:    sampleRate,
		Channels:      channels,
		Encoding:      "pcm",
		BitsPerSample: bitsPerSample,
	}

	recognitionConfig := RecognitionConfig{
		Language: "en",
		Model:    "whisper-1",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := provider.Recognize(ctx, bytes.NewReader(pcmData), audioConfig, recognitionConfig)
	if err != nil {
		// This might fail if the audio is just silence
		t.Logf("Recognition failed (expected for silence): %v", err)
	} else {
		t.Logf("Recognition result: %+v", result)
		if !result.IsFinal {
			t.Error("Expected final result from batch recognition")
		}
	}
}

func TestWhisperProvider_StreamingRecognize(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		apiKey = "test-api-key" // Use dummy key for basic testing
	}

	provider, err := NewWhisperProvider(apiKey)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	audioConfig := AudioConfig{
		SampleRate:    16000,
		Channels:      1,
		Encoding:      "pcm",
		BitsPerSample: 16,
	}

	recognitionConfig := RecognitionConfig{
		Language:             "en",
		Model:                "whisper-1",
		EnablePartialResults: true,
	}

	recognizer, err := provider.StreamingRecognize(ctx, audioConfig, recognitionConfig)
	if err != nil {
		t.Fatalf("Failed to create streaming recognizer: %v", err)
	}
	defer recognizer.Close()

	// Test sending audio
	testAudio := make([]byte, 1024)
	err = recognizer.SendAudio(ctx, testAudio)
	if err != nil {
		t.Errorf("Failed to send audio: %v", err)
	}

	// Test results channel
	resultsChan := recognizer.Results()
	if resultsChan == nil {
		t.Error("Results channel should not be nil")
	}

	// Close and verify
	err = recognizer.Close()
	if err != nil {
		t.Errorf("Failed to close recognizer: %v", err)
	}

	// Try to send after close (should fail)
	err = recognizer.SendAudio(ctx, testAudio)
	if err == nil {
		t.Error("Expected error when sending to closed recognizer")
	}
}
