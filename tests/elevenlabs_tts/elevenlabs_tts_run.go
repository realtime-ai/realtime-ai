// ElevenLabs WebSocket TTS Integration Test
//
// Tests the ElevenLabs WebSocket TTS provider with real API calls.
// Synthesizes speech and saves the output as a WAV file.
//
// Usage:
//   go run tests/elevenlabs_tts/elevenlabs_tts_run.go
//
// Environment:
//   ELEVENLABS_API_KEY - Required ElevenLabs API key
//
// Output:
//   tests/audiofiles/elevenlabs_tts_output.wav (16kHz mono)

package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
	"github.com/realtime-ai/realtime-ai/pkg/tts"
)

const (
	// Rachel voice - clear female voice
	defaultVoiceID = "21m00Tcm4TlvDq8ikWAM"

	// Audio format
	sampleRate = 16000
	channels   = 1
	bitsPerSample = 16
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	log.Println("=== ElevenLabs WebSocket TTS Integration Test ===")
	log.Println()

	// Find repo root and load .env
	repoRoot, err := findRepoRoot()
	if err != nil {
		log.Fatalf("Failed to locate repo root: %v", err)
	}

	if err := loadRootEnv(repoRoot); err != nil {
		log.Printf("Note: .env not loaded: %v", err)
	}

	// Get API key
	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	if apiKey == "" {
		log.Fatal("ELEVENLABS_API_KEY is required")
	}
	log.Println("[OK] ELEVENLABS_API_KEY loaded")

	// Create provider
	provider, err := tts.NewElevenLabsWSTTSProvider(tts.ElevenLabsWSTTSConfig{
		APIKey:  apiKey,
		VoiceID: defaultVoiceID,
	})
	if err != nil {
		log.Fatalf("Failed to create provider: %v", err)
	}
	log.Printf("[OK] Created ElevenLabs WS TTS provider (voice: %s)", defaultVoiceID)

	// Test text
	testText := "Hello! This is a test of the ElevenLabs WebSocket text to speech API. " +
		"The audio is being streamed in real-time at 16 kilohertz mono PCM format for low latency."

	log.Printf("[INFO] Test text: %s", testText)
	log.Println()

	// Test 1: Streaming synthesis
	log.Println("=== Test 1: Streaming Synthesis ===")
	streamingAudio, err := testStreamingSynthesis(ctx, provider, testText)
	if err != nil {
		log.Fatalf("Streaming synthesis failed: %v", err)
	}

	// Test 2: Batch synthesis
	log.Println()
	log.Println("=== Test 2: Batch Synthesis ===")
	batchAudio, err := testBatchSynthesis(ctx, provider, "Hello world, this is a batch test.")
	if err != nil {
		log.Fatalf("Batch synthesis failed: %v", err)
	}

	// Save streaming output
	outputDir := filepath.Join(repoRoot, "tests", "audiofiles")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	streamingOutputPath := filepath.Join(outputDir, "elevenlabs_tts_streaming.wav")
	if err := saveAsWAV(streamingAudio, streamingOutputPath); err != nil {
		log.Fatalf("Failed to save streaming audio: %v", err)
	}
	log.Printf("[OK] Saved streaming audio: %s", streamingOutputPath)

	batchOutputPath := filepath.Join(outputDir, "elevenlabs_tts_batch.wav")
	if err := saveAsWAV(batchAudio, batchOutputPath); err != nil {
		log.Fatalf("Failed to save batch audio: %v", err)
	}
	log.Printf("[OK] Saved batch audio: %s", batchOutputPath)

	// Print summary
	log.Println()
	log.Println("========================================")
	log.Println("         Test Results Summary")
	log.Println("========================================")
	log.Printf("Streaming audio: %d bytes (%.1f seconds)", len(streamingAudio), float64(len(streamingAudio))/float64(sampleRate*2))
	log.Printf("Batch audio: %d bytes (%.1f seconds)", len(batchAudio), float64(len(batchAudio))/float64(sampleRate*2))
	log.Println()
	log.Println("[PASS] ElevenLabs WebSocket TTS test completed successfully")
}

func testStreamingSynthesis(ctx context.Context, provider *tts.ElevenLabsWSTTSProvider, text string) ([]byte, error) {
	startTime := time.Now()

	req := &tts.SynthesizeRequest{
		Text: text,
	}

	audioChan, errChan := provider.StreamSynthesize(ctx, req)

	var audioData []byte
	var chunkCount int
	var firstChunkTime time.Duration

	for {
		select {
		case chunk, ok := <-audioChan:
			if !ok {
				// Channel closed
				select {
				case err := <-errChan:
					if err != nil {
						return nil, err
					}
				default:
				}
				goto done
			}

			if chunkCount == 0 {
				firstChunkTime = time.Since(startTime)
				log.Printf("  [TTFB] First chunk received in %v", firstChunkTime)
			}

			audioData = append(audioData, chunk...)
			chunkCount++

			// Log progress every 5 chunks
			if chunkCount%5 == 0 {
				log.Printf("  [Progress] Received %d chunks, %d bytes", chunkCount, len(audioData))
			}

		case err := <-errChan:
			if err != nil {
				return nil, err
			}

		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

done:
	elapsed := time.Since(startTime)
	audioDuration := float64(len(audioData)) / float64(sampleRate*2)

	log.Printf("[OK] Streaming complete: %d chunks, %d bytes (%.1fs audio) in %v",
		chunkCount, len(audioData), audioDuration, elapsed)
	log.Printf("  [Latency] TTFB: %v, Total: %v, Real-time factor: %.2fx",
		firstChunkTime, elapsed, audioDuration/elapsed.Seconds())

	return audioData, nil
}

func testBatchSynthesis(ctx context.Context, provider *tts.ElevenLabsWSTTSProvider, text string) ([]byte, error) {
	startTime := time.Now()

	req := &tts.SynthesizeRequest{
		Text: text,
	}

	resp, err := provider.Synthesize(ctx, req)
	if err != nil {
		return nil, err
	}

	elapsed := time.Since(startTime)
	audioDuration := float64(len(resp.AudioData)) / float64(sampleRate*2)

	log.Printf("[OK] Batch complete: %d bytes (%.1fs audio) in %v",
		len(resp.AudioData), audioDuration, elapsed)
	log.Printf("  [Format] %dHz, %d channel(s), %s",
		resp.AudioFormat.SampleRate, resp.AudioFormat.Channels, resp.AudioFormat.Encoding)

	return resp.AudioData, nil
}

// saveAsWAV saves PCM data as a WAV file
func saveAsWAV(pcmData []byte, filePath string) error {
	header := createWAVHeader(len(pcmData))
	wavData := append(header, pcmData...)
	return os.WriteFile(filePath, wavData, 0644)
}

// createWAVHeader creates a WAV file header for 16kHz mono 16-bit PCM
func createWAVHeader(dataSize int) []byte {
	header := make([]byte, 44)

	// RIFF header
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+dataSize))
	copy(header[8:12], "WAVE")

	// fmt subchunk
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)                       // Subchunk1Size
	binary.LittleEndian.PutUint16(header[20:22], 1)                        // AudioFormat (PCM)
	binary.LittleEndian.PutUint16(header[22:24], uint16(channels))         // NumChannels
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))       // SampleRate
	binary.LittleEndian.PutUint32(header[28:32], uint32(sampleRate*channels*bitsPerSample/8)) // ByteRate
	binary.LittleEndian.PutUint16(header[32:34], uint16(channels*bitsPerSample/8))            // BlockAlign
	binary.LittleEndian.PutUint16(header[34:36], uint16(bitsPerSample))    // BitsPerSample

	// data subchunk
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(dataSize))

	return header
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", errors.New("unable to find repository root")
}

func loadRootEnv(root string) error {
	envPath := filepath.Join(root, ".env")
	if _, err := os.Stat(envPath); err != nil {
		return fmt.Errorf(".env not found at %s", envPath)
	}
	return godotenv.Overload(envPath)
}
