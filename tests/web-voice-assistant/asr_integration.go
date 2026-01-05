// ASR Integration Test
//
// Tests Whisper ASR functionality using TTS-generated audio.
//
// Usage:
//   go run tests/web-voice-assistant/asr_integration.go
package main

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
	"github.com/realtime-ai/realtime-ai/pkg/asr"
	"github.com/realtime-ai/realtime-ai/pkg/tts"
)

func main() {
	// Load .env
	godotenv.Load()
	godotenv.Load("../../.env")

	openaiKey := os.Getenv("OPENAI_API_KEY")
	elevenlabsKey := os.Getenv("ELEVENLABS_API_KEY")
	baseURL := os.Getenv("OPENAI_BASE_URL")

	if openaiKey == "" {
		log.Fatal("OPENAI_API_KEY is required")
	}
	if elevenlabsKey == "" {
		log.Fatal("ELEVENLABS_API_KEY is required")
	}

	log.Println("===========================================")
	log.Println("  ASR Integration Test (Whisper)")
	log.Println("===========================================")
	log.Println()

	// Generate test audio using TTS
	log.Println("[STEP 1] Generating test audio with TTS...")
	testText := "Hello, this is a test of the speech recognition system."
	audioData, err := generateTestAudio(elevenlabsKey, testText)
	if err != nil {
		log.Fatalf("Failed to generate test audio: %v", err)
	}
	log.Printf("  Generated %d bytes of audio", len(audioData))

	// Save test audio
	audioPath := filepath.Join("tests", "web-voice-assistant", "output", "asr_test_input.wav")
	os.MkdirAll(filepath.Dir(audioPath), 0755)
	if err := saveAudio(audioData, audioPath); err != nil {
		log.Printf("  Warning: Failed to save audio: %v", err)
	} else {
		log.Printf("  Saved to: %s", audioPath)
	}

	// Test Whisper ASR
	log.Println()
	log.Println("[STEP 2] Testing Whisper ASR...")

	passed := 0
	failed := 0

	if testWhisperASR(openaiKey, baseURL, audioData, testText) {
		passed++
	} else {
		failed++
	}

	// Summary
	log.Println()
	log.Println("===========================================")
	log.Printf("  Results: %d passed, %d failed", passed, failed)
	log.Println("===========================================")

	if failed > 0 {
		os.Exit(1)
	}
}

func generateTestAudio(apiKey, text string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := tts.ElevenLabsHTTPTTSConfig{
		APIKey:              apiKey,
		VoiceID:             "21m00Tcm4TlvDq8ikWAM", // Rachel
		Model:               "eleven_multilingual_v2",
		LatencyOptimization: 3,
	}

	provider, err := tts.NewElevenLabsHTTPTTSProvider(config)
	if err != nil {
		return nil, err
	}

	req := &tts.SynthesizeRequest{
		Text: text,
	}

	resp, err := provider.Synthesize(ctx, req)
	if err != nil {
		return nil, err
	}

	return resp.AudioData, nil
}

func testWhisperASR(apiKey, baseURL string, audioData []byte, expectedText string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// baseURL is read from OPENAI_BASE_URL env by NewWhisperProvider
	provider, err := asr.NewWhisperProvider(apiKey)
	if err != nil {
		log.Printf("  [FAIL] Failed to create provider: %v", err)
		return false
	}

	// Create audio config for 16kHz mono
	audioConfig := asr.AudioConfig{
		SampleRate: 16000,
		Channels:   1,
		Encoding:   "pcm",
	}

	// Recognition config
	recognitionConfig := asr.RecognitionConfig{
		Model:    "whisper-1",
		Language: "en",
	}

	// Transcribe using Recognize method
	result, err := provider.Recognize(ctx, bytes.NewReader(audioData), audioConfig, recognitionConfig)
	if err != nil {
		log.Printf("  [FAIL] Transcription error: %v", err)
		return false
	}

	if result.Text == "" {
		log.Println("  [FAIL] Empty transcription result")
		return false
	}

	log.Printf("  Expected: %s", expectedText)
	log.Printf("  Got:      %s", result.Text)
	log.Printf("  Confidence: %.2f", result.Confidence)

	// Save result
	resultPath := filepath.Join("tests", "web-voice-assistant", "output", "asr_result.txt")
	os.WriteFile(resultPath, []byte(result.Text), 0644)
	log.Printf("  Saved to: %s", resultPath)

	log.Println("  [PASS]")
	return true
}

func saveAudio(pcmData []byte, filePath string) error {
	// Create WAV header for 16kHz mono 16-bit
	header := make([]byte, 44)
	dataSize := len(pcmData)
	sampleRate := 16000

	copy(header[0:4], "RIFF")
	putLE32(header[4:8], uint32(36+dataSize))
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	putLE32(header[16:20], 16)
	putLE16(header[20:22], 1)
	putLE16(header[22:24], 1)
	putLE32(header[24:28], uint32(sampleRate))
	putLE32(header[28:32], uint32(sampleRate*2))
	putLE16(header[32:34], 2)
	putLE16(header[34:36], 16)
	copy(header[36:40], "data")
	putLE32(header[40:44], uint32(dataSize))

	return os.WriteFile(filePath, append(header, pcmData...), 0644)
}

func putLE16(b []byte, v uint16) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
}

func putLE32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}
