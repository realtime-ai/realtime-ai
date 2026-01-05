// TTS Integration Test
//
// Tests ElevenLabs TTS functionality.
//
// Usage:
//   go run tests/web-voice-assistant/tts_integration.go
package main

import (
	"context"
	"encoding/binary"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
	"github.com/realtime-ai/realtime-ai/pkg/tts"
)

func main() {
	// Load .env
	godotenv.Load()
	godotenv.Load("../../.env")

	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	if apiKey == "" {
		log.Fatal("ELEVENLABS_API_KEY is required")
	}

	log.Println("===========================================")
	log.Println("  TTS Integration Test (ElevenLabs)")
	log.Println("===========================================")
	log.Println()

	// Run tests
	passed := 0
	failed := 0

	if testHTTPTTS(apiKey) {
		passed++
	} else {
		failed++
	}

	if testWebSocketTTS(apiKey) {
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

func testHTTPTTS(apiKey string) bool {
	log.Println("[TEST] ElevenLabs HTTP TTS")

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
		log.Printf("  [FAIL] Failed to create provider: %v", err)
		return false
	}

	req := &tts.SynthesizeRequest{
		Text: "Hello! This is a test of the ElevenLabs text to speech system.",
	}

	resp, err := provider.Synthesize(ctx, req)
	if err != nil {
		log.Printf("  [FAIL] Synthesis error: %v", err)
		return false
	}

	if len(resp.AudioData) == 0 {
		log.Println("  [FAIL] No audio data received")
		return false
	}

	// Calculate duration (assuming 16kHz, 16-bit mono)
	duration := float64(len(resp.AudioData)) / float64(16000*2)
	log.Printf("  Received %d bytes (%.1f seconds)", len(resp.AudioData), duration)

	// Save to file
	outputPath := filepath.Join("tests", "web-voice-assistant", "output", "tts_http_test.wav")
	os.MkdirAll(filepath.Dir(outputPath), 0755)
	if err := saveAsWAV(resp.AudioData, outputPath, 16000); err != nil {
		log.Printf("  Warning: Failed to save WAV: %v", err)
	} else {
		log.Printf("  Saved to: %s", outputPath)
	}

	log.Println("  [PASS]")
	return true
}

func testWebSocketTTS(apiKey string) bool {
	log.Println("[TEST] ElevenLabs WebSocket TTS")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := tts.ElevenLabsWSTTSConfig{
		APIKey:  apiKey,
		VoiceID: "21m00Tcm4TlvDq8ikWAM", // Rachel
		Model:   "eleven_turbo_v2_5",
	}

	provider, err := tts.NewElevenLabsWSTTSProvider(config)
	if err != nil {
		log.Printf("  [FAIL] Failed to create provider: %v", err)
		return false
	}

	req := &tts.SynthesizeRequest{
		Text: "Hello! This is a test of the ElevenLabs WebSocket streaming text to speech.",
	}

	// Use streaming synthesis
	audioChan, errChan := provider.StreamSynthesize(ctx, req)

	var allAudio []byte
	chunkCount := 0

	for {
		select {
		case chunk, ok := <-audioChan:
			if !ok {
				goto done
			}
			allAudio = append(allAudio, chunk...)
			chunkCount++
		case err := <-errChan:
			if err != nil {
				log.Printf("  [FAIL] Streaming error: %v", err)
				return false
			}
		case <-ctx.Done():
			log.Println("  [FAIL] Timeout")
			return false
		}
	}

done:
	if len(allAudio) == 0 {
		log.Println("  [FAIL] No audio data received")
		return false
	}

	// Calculate duration (ElevenLabs turbo outputs at 24kHz)
	duration := float64(len(allAudio)) / float64(24000*2)
	log.Printf("  Received %d chunks, %d bytes (%.1f seconds)", chunkCount, len(allAudio), duration)

	// Save to file
	outputPath := filepath.Join("tests", "web-voice-assistant", "output", "tts_ws_test.wav")
	os.MkdirAll(filepath.Dir(outputPath), 0755)
	if err := saveAsWAV(allAudio, outputPath, 24000); err != nil {
		log.Printf("  Warning: Failed to save WAV: %v", err)
	} else {
		log.Printf("  Saved to: %s", outputPath)
	}

	log.Println("  [PASS]")
	return true
}

func saveAsWAV(pcmData []byte, filePath string, sampleRate int) error {
	header := createWAVHeader(len(pcmData), sampleRate)
	return os.WriteFile(filePath, append(header, pcmData...), 0644)
}

func createWAVHeader(dataSize, sampleRate int) []byte {
	header := make([]byte, 44)

	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+dataSize))
	copy(header[8:12], "WAVE")

	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], 1) // mono
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(header[28:32], uint32(sampleRate*2))
	binary.LittleEndian.PutUint16(header[32:34], 2)
	binary.LittleEndian.PutUint16(header[34:36], 16)

	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(dataSize))

	return header
}
