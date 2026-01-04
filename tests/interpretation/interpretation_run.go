// End-to-end Pipeline test for simultaneous-interpretation (Modular)
//
// Tests the complete interpretation pipeline with VAD and real-time simulation:
//   Audio (English) -> VAD -> Whisper STT -> Translate (en->zh) -> TTS -> Audio (Chinese)
//
// Usage:
//   go build -tags vad -o interpretation_test tests/interpretation/interpretation_run.go
//   ./interpretation_test
//
// With official OpenAI API (skip custom OPENAI_BASE_URL in .env):
//   USE_OFFICIAL_OPENAI=1 ./interpretation_test
//
// Environment variables:
//   USE_OFFICIAL_OPENAI=1  - Use official OpenAI API instead of custom base URL
//   REALTIME_SPEED=1.0     - Real-time simulation speed (1.0=real-time, 2.0=2x faster, 0=no delay)
//   DISABLE_VAD=1          - Disable VAD element
//
// Requirements:
//   - OPENAI_API_KEY in .env (must be compatible with OPENAI_BASE_URL if set)
//   - Test audio: tests/audiofiles/vad_test_en.wav
//   - FFmpeg installed
//   - Build with -tags vad for VAD support (requires ONNX Runtime)

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/tts"
)

const (
	audioSampleRate = 16000
	audioChannels   = 1
	bitsPerSample   = 16
	chunkDuration   = 100 * time.Millisecond // 100ms chunks for VAD
)

// Config holds test configuration
type Config struct {
	RealtimeSpeed float64 // 1.0 = real-time, 2.0 = 2x faster, 0 = no delay
	EnableVAD     bool
	VADModelPath  string
}

// SegmentResult holds one STT->Translation->TTS segment
type SegmentResult struct {
	Index          int
	STTText        string
	TranslatedText string
	STTTimestamp   time.Time
	TransTimestamp time.Time
}

// TestResults holds the output from each pipeline stage
type TestResults struct {
	Segments     []SegmentResult // All STT->Translation segments
	TTSAudio     []byte          // All TTS audio bytes concatenated
	TTSChunks    [][]byte        // Individual TTS audio chunks
	StartTime    time.Time
	TotalLatency time.Duration
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	log.Println("=== Simultaneous Interpretation E2E Pipeline Test ===")
	log.Println()

	// Find repo root and load .env
	repoRoot, err := findRepoRoot()
	if err != nil {
		log.Fatalf("Failed to locate repo root: %v", err)
	}

	if err := loadRootEnv(repoRoot); err != nil {
		log.Fatalf("Failed to load .env: %v", err)
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY is required - set it in the root .env or environment")
	}
	log.Println("[OK] OPENAI_API_KEY loaded")

	// Check for custom base URL - allow override via USE_OFFICIAL_OPENAI=1
	if os.Getenv("USE_OFFICIAL_OPENAI") == "1" {
		os.Unsetenv("OPENAI_BASE_URL")
		log.Println("[INFO] Using official OpenAI API (USE_OFFICIAL_OPENAI=1)")
	} else if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		log.Printf("[INFO] Using custom OPENAI_BASE_URL: %s", baseURL)
		log.Println("[INFO] If you see 401 errors, try: USE_OFFICIAL_OPENAI=1 go run ...")
	}

	// Parse configuration
	cfg := Config{
		RealtimeSpeed: 1.0, // Default: real-time
		EnableVAD:     true,
		VADModelPath:  filepath.Join(repoRoot, "models", "silero_vad.onnx"),
	}

	// Override from environment
	if speedStr := os.Getenv("REALTIME_SPEED"); speedStr != "" {
		if speed, err := strconv.ParseFloat(speedStr, 64); err == nil {
			cfg.RealtimeSpeed = speed
		}
	}
	if os.Getenv("DISABLE_VAD") == "1" {
		cfg.EnableVAD = false
	}

	// Check VAD model exists
	if cfg.EnableVAD {
		if _, err := os.Stat(cfg.VADModelPath); err != nil {
			log.Printf("[WARN] VAD model not found: %s, disabling VAD", cfg.VADModelPath)
			cfg.EnableVAD = false
		}
	}

	log.Printf("[CONFIG] Realtime speed: %.1fx, VAD: %v", cfg.RealtimeSpeed, cfg.EnableVAD)

	// Locate test audio
	audioPath := filepath.Join(repoRoot, "tests", "audiofiles", "vad_test_en.wav")
	if _, err := os.Stat(audioPath); err != nil {
		log.Fatalf("Test audio not found: %s", audioPath)
	}
	log.Printf("[OK] Test audio: %s", audioPath)

	// Create pipeline
	p, err := createInterpretationPipeline(apiKey, cfg)
	if err != nil {
		log.Fatalf("Failed to create pipeline: %v", err)
	}
	if cfg.EnableVAD {
		log.Println("[OK] Pipeline created: VAD -> Whisper STT -> Translate (en->zh) -> TTS")
	} else {
		log.Println("[OK] Pipeline created: Whisper STT -> Translate (en->zh) -> TTS")
	}

	// Start pipeline
	if err := p.Start(ctx); err != nil {
		log.Fatalf("Failed to start pipeline: %v", err)
	}
	defer p.Stop()
	log.Println("[OK] Pipeline started")
	log.Println()

	// Run test
	log.Println("Running pipeline test...")
	results, err := runPipelineTest(ctx, p, audioPath, cfg)
	if err != nil {
		log.Fatalf("Pipeline test failed: %v", err)
	}

	// Print results
	printResults(results)

	// Validate and save outputs
	outputDir := filepath.Join(repoRoot, "tests", "interpretation", "output")
	os.MkdirAll(outputDir, 0755)
	validateAndSave(results, outputDir)
}

// createInterpretationPipeline creates a pipeline: [VAD ->] STT -> Translate -> TTS
func createInterpretationPipeline(apiKey string, cfg Config) (*pipeline.Pipeline, error) {
	p := pipeline.NewPipeline("e2e-interpretation-test")

	var prevElem pipeline.Element

	// Element 0 (optional): VAD
	if cfg.EnableVAD {
		vadConfig := elements.SileroVADConfig{
			ModelPath:       cfg.VADModelPath,
			Threshold:       0.5,
			MinSilenceDurMs: 550,
			SpeechPadMs:     30,
			Mode:            elements.VADModePassthrough, // Pass all audio, emit events
		}
		vadElem, err := elements.NewSileroVADElement(vadConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create VAD element: %w", err)
		}
		// Initialize VAD detector before adding to pipeline
		if err := vadElem.Init(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to initialize VAD element: %w", err)
		}
		p.AddElement(vadElem)
		prevElem = vadElem
		log.Println("  [1] SileroVAD (threshold: 0.5, mode: passthrough)")
	}

	// Element 1: Whisper STT
	whisperConfig := elements.WhisperSTTConfig{
		APIKey:               apiKey,
		Language:             "en",
		Model:                "whisper-1",
		EnablePartialResults: false,
		VADEnabled:           false, // External VAD
		SampleRate:           audioSampleRate,
		Channels:             audioChannels,
		BitsPerSample:        bitsPerSample,
	}
	whisperElem, err := elements.NewWhisperSTTElement(whisperConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Whisper element: %w", err)
	}
	p.AddElement(whisperElem)
	if cfg.EnableVAD {
		log.Println("  [2] WhisperSTT (en, whisper-1)")
	} else {
		log.Println("  [1] WhisperSTT (en, whisper-1)")
	}

	// Link VAD -> Whisper if VAD is enabled
	if prevElem != nil {
		p.Link(prevElem, whisperElem)
	}

	// Element 2: Translate (en -> zh)
	translateConfig := elements.TranslateConfig{
		Provider:   "openai",
		APIKey:     apiKey,
		SourceLang: "en",
		TargetLang: "zh",
		Model:      "gpt-4o-mini",
		Streaming:  false,
	}
	translateElem, err := elements.NewTranslateElement(translateConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Translate element: %w", err)
	}
	p.AddElement(translateElem)
	if cfg.EnableVAD {
		log.Println("  [3] Translate (en -> zh, gpt-4o-mini)")
	} else {
		log.Println("  [2] Translate (en -> zh, gpt-4o-mini)")
	}

	// Element 3: TTS
	ttsProvider := tts.NewOpenAITTSProvider(apiKey)
	ttsElem := elements.NewUniversalTTSElement(ttsProvider)
	ttsElem.SetProperty("voice", "alloy")
	p.AddElement(ttsElem)
	if cfg.EnableVAD {
		log.Println("  [4] TTS (OpenAI, voice: alloy)")
	} else {
		log.Println("  [3] TTS (OpenAI, voice: alloy)")
	}

	// Link elements
	p.Link(whisperElem, translateElem)
	p.Link(translateElem, ttsElem)

	return p, nil
}

// runPipelineTest streams audio and collects all outputs
func runPipelineTest(ctx context.Context, p *pipeline.Pipeline, audioPath string, cfg Config) (*TestResults, error) {
	// Decode audio to PCM
	pcmData, err := decodeToPCM(ctx, audioPath, audioSampleRate, audioChannels)
	if err != nil {
		return nil, fmt.Errorf("failed to decode audio: %w", err)
	}
	audioDuration := float64(len(pcmData)) / float64(audioSampleRate*audioChannels*2)
	log.Printf("[OK] Decoded audio: %d bytes (%.1f seconds)", len(pcmData), audioDuration)

	results := &TestResults{
		StartTime: time.Now(),
		Segments:  make([]SegmentResult, 0),
		TTSChunks: make([][]byte, 0),
	}
	var mu sync.Mutex

	// Subscribe to events on the bus for STT and Translation results
	eventChan := make(chan pipeline.Event, 100)
	p.Bus().Subscribe(pipeline.EventFinalResult, eventChan)
	defer p.Bus().Unsubscribe(pipeline.EventFinalResult, eventChan)

	// Track pending STT result waiting for translation
	var pendingSTT *SegmentResult
	segmentIndex := 0

	// Collect audio from pipeline output (TTS output)
	var wg sync.WaitGroup
	doneChan := make(chan struct{})
	streamDone := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		chunkIndex := 0
		pullTicker := time.NewTicker(100 * time.Millisecond)
		defer pullTicker.Stop()

		for {
			select {
			case <-doneChan:
				log.Println("[TTS] Collection goroutine exiting")
				return
			case <-ctx.Done():
				return
			case <-pullTicker.C:
				// Non-blocking pull with periodic check
				msg := p.Pull()
				if msg != nil && msg.AudioData != nil && len(msg.AudioData.Data) > 0 {
					chunkIndex++
					chunkData := make([]byte, len(msg.AudioData.Data))
					copy(chunkData, msg.AudioData.Data)

					mu.Lock()
					results.TTSAudio = append(results.TTSAudio, chunkData...)
					results.TTSChunks = append(results.TTSChunks, chunkData)
					mu.Unlock()

					chunkDur := float64(len(chunkData)) / float64(24000*2) // 24kHz, 16-bit
					log.Printf("[TTS] Chunk #%d: %d bytes (%.1fs)", chunkIndex, len(chunkData), chunkDur)
				}
			}
		}
	}()

	// Stream audio to pipeline in background with real-time simulation
	go func() {
		defer close(streamDone)
		log.Printf("Streaming audio to pipeline (%.1fx real-time)...", cfg.RealtimeSpeed)
		if err := streamPCMToPipeline(ctx, p, pcmData, cfg.RealtimeSpeed); err != nil {
			log.Printf("Audio streaming error: %v", err)
		}
		log.Println("[OK] Audio streaming complete")
	}()

	// Wait for outputs with timeout (audio duration + 3 min buffer for processing)
	timeoutDuration := time.Duration(audioDuration/cfg.RealtimeSpeed)*time.Second + 3*time.Minute
	if cfg.RealtimeSpeed == 0 {
		timeoutDuration = 3 * time.Minute
	}
	log.Printf("[INFO] Timeout set to %.0f seconds", timeoutDuration.Seconds())
	timeout := time.After(timeoutDuration)

	// Wait for stream to complete, then allow extra time for final processing
	streamComplete := false
	finalTimeout := time.NewTimer(24 * time.Hour) // Placeholder, reset when stream completes
	finalTimeout.Stop()

	// Idle detection: if no new results for 5 seconds after stream completes, consider done
	idleTimeout := time.NewTimer(24 * time.Hour)
	idleTimeout.Stop()
	lastActivityTime := time.Now()

	for {
		select {
		case event := <-eventChan:
			text, ok := event.Payload.(string)
			if !ok || strings.TrimSpace(text) == "" {
				continue
			}

			// Reset idle timer on activity
			lastActivityTime = time.Now()
			if streamComplete {
				idleTimeout.Reset(3 * time.Second)
			}

			mu.Lock()
			if pendingSTT == nil {
				// This is an STT result
				segmentIndex++
				pendingSTT = &SegmentResult{
					Index:        segmentIndex,
					STTText:      text,
					STTTimestamp: time.Now(),
				}
				log.Printf("[STT #%d] (%.1fs) %s", segmentIndex, time.Since(results.StartTime).Seconds(), text)
			} else {
				// This is a Translation result for the pending STT
				pendingSTT.TranslatedText = text
				pendingSTT.TransTimestamp = time.Now()
				results.Segments = append(results.Segments, *pendingSTT)
				log.Printf("[Translation #%d] (%.1fs) %s", pendingSTT.Index, time.Since(results.StartTime).Seconds(), text)
				pendingSTT = nil
			}
			mu.Unlock()

		case <-streamDone:
			if !streamComplete {
				streamComplete = true
				// Give extra 30 seconds max for final processing after stream completes
				finalTimeout.Reset(30 * time.Second)
				// Start idle detection (3 seconds without new results = done)
				idleTimeout.Reset(3 * time.Second)
				log.Println("[INFO] Stream complete, waiting for final processing...")
			}

		case <-idleTimeout.C:
			// No new results for 5 seconds after stream complete, consider done
			mu.Lock()
			segmentCount := len(results.Segments)
			audioLen := len(results.TTSAudio)
			mu.Unlock()

			if segmentCount > 0 && audioLen > 0 {
				results.TotalLatency = time.Since(results.StartTime)
				close(doneChan)
				wg.Wait()
				log.Printf("[INFO] Idle timeout reached after %.1fs of inactivity", time.Since(lastActivityTime).Seconds())
				return results, nil
			}
			// Not enough results yet, continue waiting
			idleTimeout.Reset(3 * time.Second)

		case <-finalTimeout.C:
			// Final processing timeout
			results.TotalLatency = time.Since(results.StartTime)
			close(doneChan)
			wg.Wait()

			mu.Lock()
			segmentCount := len(results.Segments)
			audioLen := len(results.TTSAudio)
			mu.Unlock()

			if segmentCount > 0 && audioLen > 0 {
				return results, nil
			}
			return nil, fmt.Errorf("incomplete results: %d segments, %d audio bytes", segmentCount, audioLen)

		case <-timeout:
			results.TotalLatency = time.Since(results.StartTime)
			close(doneChan)
			wg.Wait()

			mu.Lock()
			segmentCount := len(results.Segments)
			audioLen := len(results.TTSAudio)
			mu.Unlock()

			if segmentCount > 0 {
				return results, nil
			}
			return nil, fmt.Errorf("timeout: %d segments, %d audio bytes", segmentCount, audioLen)

		case <-ctx.Done():
			close(doneChan)
			wg.Wait()
			return nil, ctx.Err()
		}
	}
}

// streamPCMToPipeline streams audio chunks to the pipeline with real-time simulation
// realtimeSpeed: 1.0 = real-time, 2.0 = 2x faster, 0 = no delay
func streamPCMToPipeline(ctx context.Context, p *pipeline.Pipeline, pcmData []byte, realtimeSpeed float64) error {
	chunkSize := int(float64(audioSampleRate*audioChannels*2) * (float64(chunkDuration) / float64(time.Second)))
	if chunkSize <= 0 {
		chunkSize = len(pcmData)
	}

	// Calculate delay per chunk for real-time simulation
	var delayPerChunk time.Duration
	if realtimeSpeed > 0 {
		delayPerChunk = time.Duration(float64(chunkDuration) / realtimeSpeed)
	}

	totalChunks := (len(pcmData) + chunkSize - 1) / chunkSize
	chunkNum := 0

	for offset := 0; offset < len(pcmData); offset += chunkSize {
		end := offset + chunkSize
		if end > len(pcmData) {
			end = len(pcmData)
		}
		chunkNum++

		chunk := make([]byte, end-offset)
		copy(chunk, pcmData[offset:end])

		msg := &pipeline.PipelineMessage{
			Type:      pipeline.MsgTypeAudio,
			Timestamp: time.Now(),
			AudioData: &pipeline.AudioData{
				Data:       chunk,
				SampleRate: audioSampleRate,
				Channels:   audioChannels,
				MediaType:  pipeline.AudioMediaTypeRaw,
				Timestamp:  time.Now(),
			},
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			p.Push(msg)
		}

		// Real-time simulation delay
		if delayPerChunk > 0 {
			// Log progress every 10 seconds
			if chunkNum%(int(10*time.Second/chunkDuration)) == 0 {
				progress := float64(offset) / float64(len(pcmData)) * 100
				log.Printf("  [Streaming] %.0f%% (%d/%d chunks)", progress, chunkNum, totalChunks)
			}
			time.Sleep(delayPerChunk)
		}
	}

	return nil
}

// decodeToPCM uses ffmpeg to decode audio to PCM
func decodeToPCM(ctx context.Context, audioPath string, sampleRate, channels int) ([]byte, error) {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", audioPath,
		"-f", "s16le",
		"-acodec", "pcm_s16le",
		"-ac", strconv.Itoa(channels),
		"-ar", strconv.Itoa(sampleRate),
		"pipe:1",
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("ffmpeg failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("ffmpeg failed: %w", err)
	}

	return output, nil
}

// printResults prints the test results
func printResults(r *TestResults) {
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("       Pipeline Test Results")
	fmt.Println("========================================")
	fmt.Println()

	fmt.Printf("Total Segments: %d\n", len(r.Segments))
	fmt.Printf("Total Duration: %.1fs\n", r.TotalLatency.Seconds())
	fmt.Println()

	for i, seg := range r.Segments {
		fmt.Printf("--- Segment #%d ---\n", i+1)
		fmt.Printf("STT (%.1fs): %s\n", seg.STTTimestamp.Sub(r.StartTime).Seconds(), seg.STTText)
		fmt.Printf("Translation (%.1fs): %s\n", seg.TransTimestamp.Sub(r.StartTime).Seconds(), seg.TranslatedText)
		fmt.Println()
	}

	fmt.Println("[TTS Audio Output]")
	fmt.Println("----------------------------------------")
	audioDuration := float64(len(r.TTSAudio)) / float64(24000*2) // 24kHz, 16-bit
	fmt.Printf("Total: %d bytes (%.1f seconds)\n", len(r.TTSAudio), audioDuration)
	fmt.Printf("Chunks: %d\n", len(r.TTSChunks))
	fmt.Println()
}

// createWAVHeader creates a WAV file header for 24kHz mono 16-bit PCM
func createWAVHeader(dataSize int) []byte {
	header := make([]byte, 44)

	// RIFF header
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+dataSize))
	copy(header[8:12], "WAVE")

	// fmt subchunk
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)      // Subchunk1Size
	binary.LittleEndian.PutUint16(header[20:22], 1)       // AudioFormat (PCM)
	binary.LittleEndian.PutUint16(header[22:24], 1)       // NumChannels (mono)
	binary.LittleEndian.PutUint32(header[24:28], 24000)   // SampleRate
	binary.LittleEndian.PutUint32(header[28:32], 24000*2) // ByteRate
	binary.LittleEndian.PutUint16(header[32:34], 2)       // BlockAlign
	binary.LittleEndian.PutUint16(header[34:36], 16)      // BitsPerSample

	// data subchunk
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(dataSize))

	return header
}

// saveAsWAV saves PCM data as a WAV file
func saveAsWAV(pcmData []byte, filePath string) error {
	header := createWAVHeader(len(pcmData))
	wavData := append(header, pcmData...)
	return os.WriteFile(filePath, wavData, 0644)
}

// validateAndSave validates results and saves outputs
func validateAndSave(r *TestResults, outputDir string) {
	fmt.Println("[Validation]")
	fmt.Println("----------------------------------------")

	passed := true

	// Validate segments
	if len(r.Segments) == 0 {
		fmt.Println("  [FAIL] No segments received")
		passed = false
	} else {
		fmt.Printf("  [PASS] %d segments received\n", len(r.Segments))
	}

	// Validate TTS
	if len(r.TTSAudio) == 0 {
		fmt.Println("  [FAIL] TTS output is empty")
		passed = false
	} else {
		fmt.Printf("  [PASS] TTS audio: %d bytes, %d chunks\n", len(r.TTSAudio), len(r.TTSChunks))
	}

	fmt.Println()

	// Save segment details
	var sttLines, transLines []string
	for i, seg := range r.Segments {
		sttLines = append(sttLines, fmt.Sprintf("[%d] %s", i+1, seg.STTText))
		transLines = append(transLines, fmt.Sprintf("[%d] %s", i+1, seg.TranslatedText))
	}

	if len(sttLines) > 0 {
		sttPath := filepath.Join(outputDir, "stt_all.txt")
		os.WriteFile(sttPath, []byte(strings.Join(sttLines, "\n")), 0644)
		fmt.Printf("  Saved: %s\n", sttPath)
	}

	if len(transLines) > 0 {
		transPath := filepath.Join(outputDir, "translation_all.txt")
		os.WriteFile(transPath, []byte(strings.Join(transLines, "\n")), 0644)
		fmt.Printf("  Saved: %s\n", transPath)
	}

	// Save TTS as WAV
	if len(r.TTSAudio) > 0 {
		wavPath := filepath.Join(outputDir, "tts_all.wav")
		if err := saveAsWAV(r.TTSAudio, wavPath); err != nil {
			fmt.Printf("  [ERROR] Failed to save WAV: %v\n", err)
		} else {
			audioDuration := float64(len(r.TTSAudio)) / float64(24000*2)
			fmt.Printf("  Saved: %s (%.1fs)\n", wavPath, audioDuration)
		}

		// Save individual TTS chunks as WAV
		for i, chunk := range r.TTSChunks {
			chunkPath := filepath.Join(outputDir, fmt.Sprintf("tts_chunk_%02d.wav", i+1))
			if err := saveAsWAV(chunk, chunkPath); err != nil {
				fmt.Printf("  [ERROR] Failed to save chunk %d: %v\n", i+1, err)
			} else {
				chunkDur := float64(len(chunk)) / float64(24000*2)
				fmt.Printf("  Saved: %s (%.1fs)\n", chunkPath, chunkDur)
			}
		}
	}

	fmt.Println()
	if passed {
		fmt.Println("========================================")
		fmt.Println("  ALL STAGES PASSED")
		fmt.Println("========================================")
	} else {
		fmt.Println("========================================")
		fmt.Println("  SOME STAGES FAILED")
		fmt.Println("========================================")
	}
}

// Helper functions

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
		return fmt.Errorf(".env not found at %s: %w", envPath, err)
	}

	return godotenv.Overload(envPath)
}

func truncateText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
