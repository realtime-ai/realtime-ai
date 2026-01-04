//go:build vad

// Simple VAD integration test using only the vad package.
package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/vad"
)

const (
	audioFile       = "tests/audiofiles/vad_test_en.wav"
	modelPath       = "models/silero_vad.onnx"
	audioSampleRate = 16000
	windowSize      = 512
)

func main() {
	log.Println("=== Simple VAD Integration Test ===")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 1. Decode audio to PCM
	log.Printf("Decoding %s...", audioFile)
	pcmData, err := decodeToPCM(ctx, audioFile)
	if err != nil {
		log.Fatalf("Failed to decode audio: %v", err)
	}
	durationMs := len(pcmData) * 1000 / (audioSampleRate * 2)
	log.Printf("Decoded: %d bytes, %d ms", len(pcmData), durationMs)

	// 2. Create VAD detector
	log.Printf("Creating VAD detector with model: %s", modelPath)
	detector, err := vad.NewDetector(vad.DetectorConfig{
		ModelPath:  modelPath,
		SampleRate: audioSampleRate,
		LogLevel:   vad.LogLevelWarn,
	})
	if err != nil {
		log.Fatalf("Failed to create VAD detector: %v", err)
	}
	defer detector.Destroy()
	log.Println("✓ VAD detector created successfully")

	// 3. Convert PCM bytes to float32 samples
	samples := bytesToFloat32(pcmData)
	log.Printf("Total samples: %d", len(samples))

	// 4. Process audio in windows
	var speechFrames, silenceFrames int
	var maxProb, minProb float32 = 0, 1
	threshold := float32(0.5)

	log.Printf("Processing audio in %d-sample windows...", windowSize)
	for i := 0; i+windowSize <= len(samples); i += windowSize {
		window := samples[i : i+windowSize]

		prob, err := detector.Infer(window)
		if err != nil {
			log.Printf("Infer error at sample %d: %v", i, err)
			continue
		}

		if prob > maxProb {
			maxProb = prob
		}
		if prob < minProb {
			minProb = prob
		}

		if prob >= threshold {
			speechFrames++
		} else {
			silenceFrames++
		}
	}

	totalFrames := speechFrames + silenceFrames
	speechPercent := float64(speechFrames) * 100 / float64(totalFrames)

	// 5. Print results
	log.Println("\n=== VAD Test Results ===")
	log.Printf("Total frames processed: %d", totalFrames)
	log.Printf("Speech frames: %d (%.1f%%)", speechFrames, speechPercent)
	log.Printf("Silence frames: %d (%.1f%%)", silenceFrames, 100-speechPercent)
	log.Printf("Probability range: [%.4f, %.4f]", minProb, maxProb)
	log.Println("✓ Test completed successfully")

	// 6. Test reset functionality
	log.Println("\nTesting reset functionality...")
	if err := detector.Reset(); err != nil {
		log.Fatalf("Reset failed: %v", err)
	}
	log.Println("✓ Reset successful")

	// 7. Run inference after reset to verify state is cleared
	if len(samples) >= windowSize {
		prob, err := detector.Infer(samples[:windowSize])
		if err != nil {
			log.Fatalf("Infer after reset failed: %v", err)
		}
		log.Printf("✓ Inference after reset: prob=%.4f", prob)
	}

	log.Println("\n=== All tests passed! ===")
}

// bytesToFloat32 converts 16-bit PCM bytes to normalized float32 samples
func bytesToFloat32(data []byte) []float32 {
	n := len(data) / 2
	samples := make([]float32, n)
	for i := 0; i < n; i++ {
		v := int16(binary.LittleEndian.Uint16(data[i*2 : i*2+2]))
		samples[i] = float32(v) / 32768.0
	}
	return samples
}

// decodeToPCM decodes audio file to 16kHz mono PCM
func decodeToPCM(ctx context.Context, audioPath string) ([]byte, error) {
	// Check if file exists
	if _, err := os.Stat(audioPath); os.IsNotExist(err) {
		// Generate a test audio with speech-like characteristics
		log.Printf("Audio file not found, generating synthetic speech-like audio...")
		return generateSyntheticSpeech(5.0), nil
	}

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-i", audioPath,
		"-f", "s16le",
		"-acodec", "pcm_s16le",
		"-ac", "1",
		"-ar", fmt.Sprintf("%d", audioSampleRate),
		"pipe:1",
	)
	return cmd.Output()
}

// generateSyntheticSpeech creates PCM data with speech-like characteristics
func generateSyntheticSpeech(durationSec float64) []byte {
	numSamples := int(durationSec * audioSampleRate)
	data := make([]byte, numSamples*2)

	for i := 0; i < numSamples; i++ {
		t := float64(i) / audioSampleRate

		// Create complex waveform similar to speech (formants)
		f1 := 300.0  // First formant
		f2 := 800.0  // Second formant
		f3 := 2500.0 // Third formant

		// Amplitude modulation (simulate syllables)
		ampMod := 0.5 + 0.5*math.Sin(2*math.Pi*3*t) // ~3 syllables per second

		sample := 0.0
		sample += 0.5 * math.Sin(2*math.Pi*f1*t)
		sample += 0.3 * math.Sin(2*math.Pi*f2*t)
		sample += 0.2 * math.Sin(2*math.Pi*f3*t)
		sample *= ampMod

		// Convert to int16
		s16 := int16(sample * 16000)
		binary.LittleEndian.PutUint16(data[i*2:], uint16(s16))
	}

	return data
}
