// ElevenLabs Scribe V2 Realtime ASR Integration Test
//
// This test verifies the ElevenLabs ASR provider with real audio input.
// It streams audio to the ElevenLabs WebSocket API and displays transcription results.
//
// Usage:
//   go run tests/elevenlabs/elevenlabs_run.go
//
// Environment:
//   ELEVENLABS_API_KEY - Required ElevenLabs API key
//
// Test audio: uses tests/audiofiles/vad_test_en.wav (16kHz mono)

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
	"github.com/realtime-ai/realtime-ai/pkg/asr"
	"github.com/realtime-ai/realtime-ai/pkg/vad"
)

const (
	audioSampleRate = 16000
	audioChannels   = 1
)

type speechSegment struct {
	StartMs int
	EndMs   int
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	log.Println("=== ElevenLabs Scribe V2 Realtime ASR Test (VAD segmented, single WS, per-segment commit) ===")
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

	// Find test audio
	audioPath := filepath.Join(repoRoot, "tests", "audiofiles", "vad_test_en.wav")
	if _, err := os.Stat(audioPath); err != nil {
		log.Fatalf("Test audio not found: %s", audioPath)
	}
	log.Printf("[OK] Test audio: %s", audioPath)

	// Decode audio to PCM
	pcmData, err := decodeToPCM(ctx, audioPath)
	if err != nil {
		log.Fatalf("Failed to decode audio: %v", err)
	}
	audioDuration := float64(len(pcmData)) / float64(audioSampleRate*audioChannels*2)
	log.Printf("[OK] Decoded audio: %d bytes (%.1f seconds)", len(pcmData), audioDuration)

	// Ensure VAD model exists (auto-download)
	modelPath := filepath.Join(repoRoot, "models", "silero_vad.onnx")
	if err := ensureSileroVADModel(modelPath); err != nil {
		log.Fatalf("Failed to ensure VAD model: %v", err)
	}

	// Run VAD segmentation first (offline)
	segments, err := segmentPCMWithSileroVAD(ctx, pcmData, modelPath, vadSegConfig{
		Threshold:       0.5,
		MinSilenceDurMs: 250,
		SpeechPadMs:     30,
	})
	if err != nil {
		log.Fatalf("Failed to segment audio with VAD: %v", err)
	}
	if len(segments) == 0 {
		log.Printf("[WARN] No VAD segments detected; falling back to a single segment (whole audio)")
		segments = []speechSegment{{StartMs: 0, EndMs: int(audioDuration * 1000)}}
	}
	log.Printf("[OK] VAD segments: %d", len(segments))

	// Create ElevenLabs provider
	provider, err := asr.NewElevenLabsProvider(asr.ElevenLabsConfig{
		APIKey: apiKey,
	})
	if err != nil {
		log.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Close()
	log.Printf("[OK] Created ElevenLabs provider (model: scribe_v2_realtime)")

	// Create streaming recognizer
	audioConfig := asr.AudioConfig{
		SampleRate:    audioSampleRate,
		Channels:      audioChannels,
		Encoding:      "pcm",
		BitsPerSample: 16,
	}

	recognitionConfig := asr.RecognitionConfig{
		Language:             "en",
		EnablePartialResults: true,
	}

	recognizer, err := provider.StreamingRecognize(ctx, audioConfig, recognitionConfig)
	if err != nil {
		log.Fatalf("Failed to create recognizer: %v", err)
	}
	defer recognizer.Close()
	log.Println("[OK] Streaming recognizer created (single WS connection)")
	log.Println()

	er, ok := asr.IsElevenLabsRecognizer(recognizer)
	if !ok {
		log.Fatal("recognizer is not an ElevenLabs recognizer (Commit not available)")
	}

	resultsChan := recognizer.Results()
	startTime := time.Now()

	// Per-segment: send segment audio -> commit -> wait for FINAL -> print
	type segResult struct {
		Index   int
		StartMs int
		EndMs   int
		Text    string
	}
	var segResults []segResult

	log.Println("Streaming VAD segments...")
	for i, s := range segments {
		segPCM := slicePCMByMs(pcmData, s.StartMs, s.EndMs, audioSampleRate, audioChannels)
		if len(segPCM) == 0 {
			continue
		}

		// Drain any pending results from previous segment to avoid cross-talk.
		drainResults(resultsChan)

		log.Printf("[SEG %02d] %6d -> %6d ms | bytes=%d", i+1, s.StartMs, s.EndMs, len(segPCM))
		if err := sendPCMInChunks(ctx, recognizer, segPCM, audioSampleRate, audioChannels); err != nil {
			log.Printf("[SEG %02d] send error: %v", i+1, err)
			continue
		}

		if err := er.Commit(ctx); err != nil {
			log.Printf("[SEG %02d] commit error: %v", i+1, err)
			continue
		}

		segCtx, segCancel := context.WithTimeout(ctx, 45*time.Second)
		text, err := waitForFinal(segCtx, resultsChan)
		segCancel()
		if err != nil {
			log.Printf("[SEG %02d] wait final error: %v", i+1, err)
			continue
		}

		fmt.Printf("[SEG %02d] %6d -> %6d ms | %s\n", i+1, s.StartMs, s.EndMs, text)
		segResults = append(segResults, segResult{
			Index:   i + 1,
			StartMs: s.StartMs,
			EndMs:   s.EndMs,
			Text:    text,
		})
	}

	elapsed := time.Since(startTime)
	log.Println()
	log.Println("========================================")
	log.Println("         Segmented ASR Summary")
	log.Println("========================================")
	log.Printf("Audio duration: %.1f seconds", audioDuration)
	log.Printf("VAD segments: %d", len(segments))
	log.Printf("Segments transcribed: %d", len(segResults))
	log.Printf("Processing time: %.1f seconds", elapsed.Seconds())

	if len(segResults) > 0 {
		log.Println()
		log.Println("Per-segment FINAL:")
		for _, r := range segResults {
			log.Printf("  [%02d] %6d->%6d ms | %s", r.Index, r.StartMs, r.EndMs, r.Text)
		}
		log.Println()
		log.Println("[PASS] ElevenLabs segmented ASR completed")
	} else {
		log.Println()
		log.Println("[WARN] No segments produced FINAL results")
	}
}

// decodeToPCM uses ffmpeg to decode audio to 16kHz mono PCM
func decodeToPCM(ctx context.Context, audioPath string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-i", audioPath,
		"-f", "s16le",
		"-acodec", "pcm_s16le",
		"-ac", "1",
		"-ar", "16000",
		"pipe:1",
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("ffmpeg failed: %w (%s)", err, stderr.String())
		}
		return nil, fmt.Errorf("ffmpeg failed: %w", err)
	}

	return output, nil
}

type vadSegConfig struct {
	Threshold       float32
	MinSilenceDurMs int
	SpeechPadMs     int
}

func segmentPCMWithSileroVAD(ctx context.Context, pcm []byte, modelPath string, cfg vadSegConfig) ([]speechSegment, error) {
	if cfg.Threshold <= 0 {
		cfg.Threshold = 0.5
	}
	if cfg.MinSilenceDurMs <= 0 {
		cfg.MinSilenceDurMs = 250
	}
	if cfg.SpeechPadMs < 0 {
		cfg.SpeechPadMs = 0
	}

	det, err := vad.NewDetector(vad.DetectorConfig{
		ModelPath:  modelPath,
		SampleRate: audioSampleRate,
		LogLevel:   vad.LogLevelWarn,
	})
	if err != nil {
		return nil, err
	}
	defer det.Destroy()
	_ = det.Reset()

	samples := pcm16leToFloat32(pcm)
	const windowSize = 512
	sampleRate := audioSampleRate

	minSilenceSamples := cfg.MinSilenceDurMs * sampleRate / 1000
	speechPadSamples := cfg.SpeechPadMs * sampleRate / 1000

	currSample := 0
	triggered := false
	tempEnd := 0

	var (
		inSpeech bool
		startMs  int
		segs     []speechSegment
	)

	for i := 0; i+windowSize <= len(samples); i += windowSize {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		window := samples[i : i+windowSize]
		speechProb, err := det.Infer(window)
		if err != nil {
			return nil, err
		}

		currSample += windowSize

		if speechProb >= cfg.Threshold && tempEnd != 0 {
			tempEnd = 0
		}

		if speechProb >= cfg.Threshold && !triggered {
			triggered = true
			speechStartSample := currSample - windowSize - speechPadSamples
			if speechStartSample < 0 {
				speechStartSample = 0
			}
			speechStartMs := speechStartSample * 1000 / sampleRate

			if !inSpeech {
				inSpeech = true
				startMs = speechStartMs
			}
		}

		if speechProb < (cfg.Threshold-0.15) && triggered {
			if tempEnd == 0 {
				tempEnd = currSample
			}
			if currSample-tempEnd >= minSilenceSamples {
				speechEndSample := tempEnd + speechPadSamples
				speechEndMs := speechEndSample * 1000 / sampleRate
				tempEnd = 0
				triggered = false

				if inSpeech {
					inSpeech = false
					if speechEndMs > startMs {
						segs = append(segs, speechSegment{StartMs: startMs, EndMs: speechEndMs})
					}
				}
			}
		}
	}

	// Close trailing segment at end-of-audio
	durationMs := len(pcm) * 1000 / (audioSampleRate * audioChannels * 2)
	if inSpeech && startMs < durationMs {
		segs = append(segs, speechSegment{StartMs: startMs, EndMs: durationMs})
	}

	return normalizeSegments(segs, durationMs), nil
}

func normalizeSegments(segs []speechSegment, durationMs int) []speechSegment {
	// clamp + drop tiny segments + merge overlaps
	const minDurMs = 200
	var out []speechSegment
	for _, s := range segs {
		if s.StartMs < 0 {
			s.StartMs = 0
		}
		if s.EndMs > durationMs {
			s.EndMs = durationMs
		}
		if s.EndMs-s.StartMs < minDurMs {
			continue
		}
		if len(out) == 0 {
			out = append(out, s)
			continue
		}
		last := &out[len(out)-1]
		if s.StartMs <= last.EndMs {
			if s.EndMs > last.EndMs {
				last.EndMs = s.EndMs
			}
			continue
		}
		out = append(out, s)
	}
	return out
}

func pcm16leToFloat32(pcm []byte) []float32 {
	n := len(pcm) / 2
	samples := make([]float32, n)
	for i := 0; i < n; i++ {
		v := int16(binary.LittleEndian.Uint16(pcm[i*2 : i*2+2]))
		samples[i] = float32(v) / 32768.0
	}
	return samples
}

func slicePCMByMs(pcm []byte, startMs, endMs, sampleRate, channels int) []byte {
	if sampleRate <= 0 || channels <= 0 {
		return nil
	}
	if startMs < 0 {
		startMs = 0
	}
	if endMs < startMs {
		endMs = startMs
	}
	bytesPerMs := (sampleRate * channels * 2) / 1000
	start := startMs * bytesPerMs
	end := endMs * bytesPerMs
	if start < 0 {
		start = 0
	}
	if end > len(pcm) {
		end = len(pcm)
	}
	if start >= end {
		return nil
	}
	// Ensure 2-byte alignment
	start -= start % 2
	end -= end % 2
	if start >= end {
		return nil
	}
	return pcm[start:end]
}

func sendPCMInChunks(ctx context.Context, r asr.StreamingRecognizer, pcm []byte, sampleRate, channels int) error {
	// Use 100ms chunks like the original test, but with minimal sleeping.
	chunkSize := sampleRate * channels * 2 / 10
	for off := 0; off < len(pcm); off += chunkSize {
		end := off + chunkSize
		if end > len(pcm) {
			end = len(pcm)
		}
		if err := r.SendAudio(ctx, pcm[off:end]); err != nil {
			return err
		}
		// tiny pacing to avoid overwhelming WS/send loop
		time.Sleep(5 * time.Millisecond)
	}
	return nil
}

func waitForFinal(ctx context.Context, ch <-chan *asr.RecognitionResult) (string, error) {
	lastPartial := ""
	for {
		select {
		case <-ctx.Done():
			if lastPartial != "" {
				return lastPartial, fmt.Errorf("timeout waiting final (last partial: %q)", lastPartial)
			}
			return "", ctx.Err()
		case r, ok := <-ch:
			if !ok {
				return "", errors.New("results channel closed")
			}
			if r == nil {
				continue
			}
			if r.IsFinal {
				return r.Text, nil
			}
			if r.Text != "" {
				lastPartial = r.Text
			}
		}
	}
}

func drainResults(ch <-chan *asr.RecognitionResult) {
	for {
		select {
		case <-ch:
			// drop
		default:
			return
		}
	}
}

func ensureSileroVADModel(modelPath string) error {
	if modelPath == "" {
		return fmt.Errorf("modelPath is empty")
	}
	if st, err := os.Stat(modelPath); err == nil && st.Size() > 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		return err
	}

	const url = "https://github.com/snakers4/silero-vad/raw/master/src/silero_vad/data/silero_vad.onnx"
	log.Printf("VAD model missing, downloading to %s ...", modelPath)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "realtime-ai/elevenlabs-vad-seg")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	tmp := modelPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	w := io.MultiWriter(f, h)
	n, err := io.Copy(w, resp.Body)
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if n <= 0 {
		_ = os.Remove(tmp)
		return errors.New("downloaded model is empty")
	}
	if err := os.Rename(tmp, modelPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	sum := fmt.Sprintf("%x", h.Sum(nil))
	log.Printf("Downloaded VAD model (%d bytes, sha256=%s)", n, sum[:16])
	return nil
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

	return "", fmt.Errorf("unable to find repository root")
}

func loadRootEnv(root string) error {
	envPath := filepath.Join(root, ".env")
	if _, err := os.Stat(envPath); err != nil {
		return fmt.Errorf(".env not found at %s", envPath)
	}
	return godotenv.Load(envPath)
}
