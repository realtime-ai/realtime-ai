//go:build vad

package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/vad"
)

const (
	defaultAudioFile = "vad_test_en.wav"
	defaultModelPath = "../../models/silero_vad.onnx"
	audioSampleRate  = 16000
	windowSize       = 512 // Silero VAD window size for 16kHz
)

// VADAnalysisPoint represents a single analysis data point
type VADAnalysisPoint struct {
	Time        float64 `json:"time"`        // Time in seconds
	Probability float32 `json:"probability"` // Speech probability [0, 1]
	AudioRMS    float64 `json:"audio_rms"`   // Audio RMS level
	IsSpeech    bool    `json:"is_speech"`   // Whether this is detected as speech
}

// VADAnalysisResult contains the complete analysis output
type VADAnalysisResult struct {
	AudioFile    string             `json:"audio_file"`
	SampleRate   int                `json:"sample_rate"`
	WindowSize   int                `json:"window_size"`
	Threshold    float32            `json:"threshold"`
	DurationMs   int                `json:"duration_ms"`
	TotalFrames  int                `json:"total_frames"`
	SpeechFrames int                `json:"speech_frames"`
	DataPoints   []VADAnalysisPoint `json:"data_points"`
}

func main() {
	// Parse command line flags
	audioFile := flag.String("audio", defaultAudioFile, "Path to the audio file to analyze")
	modelPath := flag.String("model", defaultModelPath, "Path to the Silero VAD ONNX model")
	outputJSON := flag.String("output", "", "Output JSON file path (default: <audio_name>_vad.json)")
	threshold := flag.Float64("threshold", 0.5, "VAD speech detection threshold (0.0-1.0)")
	flag.Parse()

	// Handle positional argument for audio file
	if flag.NArg() > 0 {
		*audioFile = flag.Arg(0)
	}

	// Generate default output filename if not specified
	if *outputJSON == "" {
		base := strings.TrimSuffix(filepath.Base(*audioFile), filepath.Ext(*audioFile))
		*outputJSON = base + "_vad.json"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	thresholdF32 := float32(*threshold)

	// 1. Decode audio to PCM
	log.Printf("Decoding %s...", *audioFile)
	pcmData, err := decodeToPCM(ctx, *audioFile)
	if err != nil {
		log.Fatalf("Failed to decode audio: %v", err)
	}
	durationMs := len(pcmData) * 1000 / (audioSampleRate * 2) // 2 bytes per sample
	log.Printf("Decoded: %d bytes, %d ms", len(pcmData), durationMs)

	// 2. Create VAD detector directly (not through element)
	detector, err := vad.NewDetector(vad.DetectorConfig{
		ModelPath:  *modelPath,
		SampleRate: audioSampleRate,
		LogLevel:   vad.LogLevelWarn,
	})
	if err != nil {
		log.Fatalf("Failed to create VAD detector: %v", err)
	}
	defer detector.Destroy()

	// 3. Convert PCM bytes to float32 samples
	samples := bytesToFloat32(pcmData)
	log.Printf("Total samples: %d", len(samples))

	// 4. Process each window and collect results
	result := VADAnalysisResult{
		AudioFile:  *audioFile,
		SampleRate: audioSampleRate,
		WindowSize: windowSize,
		Threshold:  thresholdF32,
		DurationMs: durationMs,
		DataPoints: make([]VADAnalysisPoint, 0),
	}

	speechFrames := 0
	frameCount := 0

	for i := 0; i+windowSize <= len(samples); i += windowSize {
		window := samples[i : i+windowSize]

		// Run inference
		prob, err := detector.Infer(window)
		if err != nil {
			log.Printf("Infer error at sample %d: %v", i, err)
			continue
		}

		// Calculate time and RMS
		timestamp := float64(i) / float64(audioSampleRate)
		rms := calculateRMS(window)
		isSpeech := prob >= thresholdF32

		result.DataPoints = append(result.DataPoints, VADAnalysisPoint{
			Time:        timestamp,
			Probability: prob,
			AudioRMS:    rms,
			IsSpeech:    isSpeech,
		})

		if isSpeech {
			speechFrames++
		}
		frameCount++
	}

	result.TotalFrames = frameCount
	result.SpeechFrames = speechFrames

	// 5. Save results to JSON
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal JSON: %v", err)
	}

	if err := os.WriteFile(*outputJSON, jsonData, 0644); err != nil {
		log.Fatalf("Failed to write JSON file: %v", err)
	}

	log.Printf("\n=== VAD Analysis Complete ===")
	log.Printf("Total frames: %d", frameCount)
	log.Printf("Speech frames: %d (%.1f%%)", speechFrames, float64(speechFrames)*100/float64(frameCount))
	log.Printf("Results saved to: %s", *outputJSON)
	log.Printf("\nTo visualize, run:")
	log.Printf("  python3 visualize_vad.py %s %s", *audioFile, *outputJSON)
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

// calculateRMS computes the root mean square of audio samples
func calculateRMS(samples []float32) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range samples {
		sum += float64(s * s)
	}
	return math.Sqrt(sum / float64(len(samples)))
}

// decodeToPCM decodes audio file to 16kHz mono PCM
func decodeToPCM(ctx context.Context, audioPath string) ([]byte, error) {
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
