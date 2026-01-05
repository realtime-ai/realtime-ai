// Web Voice Assistant End-to-End Pipeline Test
//
// Tests the complete voice assistant pipeline with audio file input:
//   Audio File → Resample → [VAD →] ASR → Chat → TTS → Audio Output
//
// Usage:
//
//	go build -tags vad -o voice_test tests/web-voice-assistant/run.go
//	./voice_test
//
// Or without VAD:
//
//	go run tests/web-voice-assistant/run.go
//
// Environment:
//
//	OPENAI_API_KEY     - Required for Chat (gpt-4o-mini)
//	ELEVENLABS_API_KEY - Required for ASR and TTS
//	TEST_AUDIO         - Optional: path to test audio file
//	REALTIME_SPEED     - Optional: playback speed (1.0=realtime, 0=fastest)
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
	inputSampleRate  = 16000
	outputSampleRate = 24000 // ElevenLabs TTS output
	channels         = 1
	chunkDurationMs  = 100
)

// TestResult holds the test results
type TestResult struct {
	ASRTexts     []string
	LLMResponses []string
	TTSAudio     []byte
	StartTime    time.Time
	Duration     time.Duration
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	log.Println("===========================================")
	log.Println("  Web Voice Assistant - Pipeline Test")
	log.Println("===========================================")
	log.Println()

	// Find repo root and load .env
	repoRoot, err := findRepoRoot()
	if err != nil {
		log.Fatalf("Failed to find repo root: %v", err)
	}

	if err := loadEnv(repoRoot); err != nil {
		log.Fatalf("Failed to load .env: %v", err)
	}

	// Validate API keys
	openaiKey := os.Getenv("OPENAI_API_KEY")
	if openaiKey == "" {
		log.Fatal("OPENAI_API_KEY is required")
	}
	log.Println("[OK] OPENAI_API_KEY loaded")

	elevenlabsKey := os.Getenv("ELEVENLABS_API_KEY")
	if elevenlabsKey == "" {
		log.Fatal("ELEVENLABS_API_KEY is required")
	}
	log.Println("[OK] ELEVENLABS_API_KEY loaded")

	// Get test configuration
	audioPath := os.Getenv("TEST_AUDIO")
	if audioPath == "" {
		// Try default test audio files
		candidates := []string{
			filepath.Join(repoRoot, "tests", "audiofiles", "test_speech.wav"),
			filepath.Join(repoRoot, "tests", "audiofiles", "vad_test_en.wav"),
		}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				audioPath = p
				break
			}
		}
	}
	if audioPath == "" {
		log.Fatal("No test audio file found. Set TEST_AUDIO environment variable.")
	}
	log.Printf("[OK] Test audio: %s", audioPath)

	realtimeSpeed := 1.0
	if s := os.Getenv("REALTIME_SPEED"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			realtimeSpeed = v
		}
	}
	log.Printf("[CONFIG] Realtime speed: %.1fx", realtimeSpeed)

	// Find VAD model
	vadModelPath := filepath.Join(repoRoot, "models", "silero_vad.onnx")
	enableVAD := false
	if _, err := os.Stat(vadModelPath); err == nil {
		enableVAD = true
		log.Printf("[OK] VAD model found: %s", vadModelPath)
	} else {
		log.Println("[WARN] VAD model not found, running without VAD")
	}

	// Create pipeline
	p, err := createPipeline(ctx, PipelineConfig{
		OpenAIKey:     openaiKey,
		ElevenLabsKey: elevenlabsKey,
		VADModelPath:  vadModelPath,
		EnableVAD:     enableVAD,
	})
	if err != nil {
		log.Fatalf("Failed to create pipeline: %v", err)
	}

	// Start pipeline
	if err := p.Start(ctx); err != nil {
		log.Fatalf("Failed to start pipeline: %v", err)
	}
	defer p.Stop()
	log.Println("[OK] Pipeline started")
	log.Println()

	// Run test
	result, err := runTest(ctx, p, audioPath, realtimeSpeed)
	if err != nil {
		log.Fatalf("Test failed: %v", err)
	}

	// Print and save results
	printResults(result)
	saveResults(result, filepath.Join(repoRoot, "tests", "web-voice-assistant", "output"))
}

// PipelineConfig holds pipeline configuration
type PipelineConfig struct {
	OpenAIKey     string
	ElevenLabsKey string
	VADModelPath  string
	EnableVAD     bool
}

// createPipeline creates the test pipeline
func createPipeline(ctx context.Context, cfg PipelineConfig) (*pipeline.Pipeline, error) {
	p := pipeline.NewPipeline("voice-assistant-test")

	var elems []pipeline.Element
	var prevElem pipeline.Element

	// 1. VAD (optional)
	if cfg.EnableVAD {
		vadConfig := elements.SileroVADConfig{
			ModelPath:       cfg.VADModelPath,
			Threshold:       0.5,
			MinSilenceDurMs: 500,
			SpeechPadMs:     30,
			Mode:            elements.VADModePassthrough,
		}
		vadElem, err := elements.NewSileroVADElement(vadConfig)
		if err != nil {
			log.Printf("[WARN] Failed to create VAD: %v", err)
		} else {
			if err := vadElem.Init(ctx); err != nil {
				log.Printf("[WARN] Failed to init VAD: %v", err)
			} else {
				elems = append(elems, vadElem)
				prevElem = vadElem
				log.Println("  [1] SileroVAD")
			}
		}
	}

	// 2. ASR (ElevenLabs)
	asrConfig := elements.ElevenLabsRealtimeSTTConfig{
		APIKey:               cfg.ElevenLabsKey,
		Language:             "en",
		Model:                "scribe_v2_realtime",
		EnablePartialResults: false,
		VADEnabled:           cfg.EnableVAD,
		SampleRate:           inputSampleRate,
		Channels:             channels,
	}
	asrElem, err := elements.NewElevenLabsRealtimeSTTElement(asrConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create ASR: %w", err)
	}
	elems = append(elems, asrElem)
	if prevElem != nil {
		p.Link(prevElem, asrElem)
	}
	prevElem = asrElem
	log.Println("  [2] ElevenLabs ASR (scribe_v2_realtime)")

	// 3. Chat (OpenAI)
	chatConfig := elements.ChatConfig{
		APIKey:       cfg.OpenAIKey,
		Model:        "gpt-4o-mini",
		SystemPrompt: "You are a helpful voice assistant. Reply concisely in 1-2 sentences.",
		Streaming:    true,
		MaxHistory:   10,
	}
	chatElem, err := elements.NewChatElement(chatConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Chat: %w", err)
	}
	elems = append(elems, chatElem)
	p.Link(prevElem, chatElem)
	prevElem = chatElem
	log.Println("  [3] OpenAI Chat (gpt-4o-mini)")

	// 4. TTS (ElevenLabs)
	ttsConfig := tts.ElevenLabsWSTTSConfig{
		APIKey:  cfg.ElevenLabsKey,
		VoiceID: "21m00Tcm4TlvDq8ikWAM", // Rachel
		Model:   "eleven_turbo_v2_5",
	}
	ttsProvider, err := tts.NewElevenLabsWSTTSProvider(ttsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create TTS: %w", err)
	}
	ttsElem := elements.NewUniversalTTSElement(ttsProvider)
	elems = append(elems, ttsElem)
	p.Link(prevElem, ttsElem)
	log.Println("  [4] ElevenLabs TTS (eleven_turbo_v2_5)")

	// Add all elements
	p.AddElements(elems)

	return p, nil
}

// runTest runs the pipeline test with audio file input
func runTest(ctx context.Context, p *pipeline.Pipeline, audioPath string, speed float64) (*TestResult, error) {
	// Decode audio
	pcmData, err := decodeToPCM(ctx, audioPath, inputSampleRate, channels)
	if err != nil {
		return nil, fmt.Errorf("failed to decode audio: %w", err)
	}
	audioDuration := float64(len(pcmData)) / float64(inputSampleRate*channels*2)
	log.Printf("[OK] Decoded audio: %d bytes (%.1f seconds)", len(pcmData), audioDuration)

	result := &TestResult{
		StartTime: time.Now(),
	}

	var mu sync.Mutex

	// Subscribe to events
	eventChan := make(chan pipeline.Event, 100)
	p.Bus().Subscribe(pipeline.EventFinalResult, eventChan)
	p.Bus().Subscribe(pipeline.EventTextDelta, eventChan)
	defer func() {
		p.Bus().Unsubscribe(pipeline.EventFinalResult, eventChan)
		p.Bus().Unsubscribe(pipeline.EventTextDelta, eventChan)
	}()

	// Collect outputs
	done := make(chan struct{})
	streamDone := make(chan struct{})

	// Output collector
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			default:
				msg := p.Pull()
				if msg != nil && msg.AudioData != nil && len(msg.AudioData.Data) > 0 {
					mu.Lock()
					result.TTSAudio = append(result.TTSAudio, msg.AudioData.Data...)
					mu.Unlock()
					log.Printf("[TTS] Audio chunk: %d bytes", len(msg.AudioData.Data))
				}
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()

	// Event collector
	var asrPending bool
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case event := <-eventChan:
				text, ok := event.Payload.(string)
				if !ok || strings.TrimSpace(text) == "" {
					continue
				}

				mu.Lock()
				if !asrPending {
					// This is ASR output
					result.ASRTexts = append(result.ASRTexts, text)
					asrPending = true
					log.Printf("[ASR] %s", text)
				} else {
					// This is LLM output
					result.LLMResponses = append(result.LLMResponses, text)
					asrPending = false
					log.Printf("[LLM] %s", truncate(text, 80))
				}
				mu.Unlock()
			}
		}
	}()

	// Stream audio
	go func() {
		defer close(streamDone)
		log.Printf("Streaming audio (%.1fx speed)...", speed)
		streamAudio(ctx, p, pcmData, speed)
		log.Println("[OK] Audio streaming complete")
	}()

	// Wait for stream to complete plus processing time
	<-streamDone
	log.Println("Waiting for processing...")

	// Wait for results with timeout
	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastAudioLen := 0
	idleCount := 0

	for {
		select {
		case <-timeout:
			goto finish
		case <-ticker.C:
			mu.Lock()
			currentLen := len(result.TTSAudio)
			mu.Unlock()

			if currentLen == lastAudioLen {
				idleCount++
				if idleCount >= 5 { // 5 seconds of no new audio
					goto finish
				}
			} else {
				idleCount = 0
				lastAudioLen = currentLen
			}
		case <-ctx.Done():
			goto finish
		}
	}

finish:
	close(done)
	result.Duration = time.Since(result.StartTime)

	mu.Lock()
	defer mu.Unlock()

	if len(result.ASRTexts) == 0 {
		return result, fmt.Errorf("no ASR results received")
	}

	return result, nil
}

// streamAudio streams PCM audio to the pipeline
func streamAudio(ctx context.Context, p *pipeline.Pipeline, pcmData []byte, speed float64) {
	chunkSize := inputSampleRate * channels * 2 * chunkDurationMs / 1000
	var delay time.Duration
	if speed > 0 {
		delay = time.Duration(float64(chunkDurationMs)*float64(time.Millisecond)) / time.Duration(speed)
	}

	for offset := 0; offset < len(pcmData); offset += chunkSize {
		end := offset + chunkSize
		if end > len(pcmData) {
			end = len(pcmData)
		}

		chunk := make([]byte, end-offset)
		copy(chunk, pcmData[offset:end])

		msg := &pipeline.PipelineMessage{
			Type:      pipeline.MsgTypeAudio,
			Timestamp: time.Now(),
			AudioData: &pipeline.AudioData{
				Data:       chunk,
				SampleRate: inputSampleRate,
				Channels:   channels,
				MediaType:  pipeline.AudioMediaTypeRaw,
				Timestamp:  time.Now(),
			},
		}

		select {
		case <-ctx.Done():
			return
		default:
			p.Push(msg)
		}

		if delay > 0 {
			time.Sleep(delay)
		}
	}
}

// printResults prints the test results
func printResults(r *TestResult) {
	fmt.Println()
	fmt.Println("===========================================")
	fmt.Println("           Test Results")
	fmt.Println("===========================================")
	fmt.Println()
	fmt.Printf("Duration: %.1f seconds\n", r.Duration.Seconds())
	fmt.Println()

	fmt.Println("[ASR Transcriptions]")
	fmt.Println("-------------------------------------------")
	for i, text := range r.ASRTexts {
		fmt.Printf("  [%d] %s\n", i+1, text)
	}
	fmt.Println()

	fmt.Println("[LLM Responses]")
	fmt.Println("-------------------------------------------")
	for i, text := range r.LLMResponses {
		fmt.Printf("  [%d] %s\n", i+1, truncate(text, 200))
	}
	fmt.Println()

	fmt.Println("[TTS Audio]")
	fmt.Println("-------------------------------------------")
	audioDur := float64(len(r.TTSAudio)) / float64(outputSampleRate*2)
	fmt.Printf("  Total: %d bytes (%.1f seconds)\n", len(r.TTSAudio), audioDur)
	fmt.Println()
}

// saveResults saves results to files
func saveResults(r *TestResult, outputDir string) {
	os.MkdirAll(outputDir, 0755)

	// Save ASR transcripts
	if len(r.ASRTexts) > 0 {
		var lines []string
		for i, text := range r.ASRTexts {
			lines = append(lines, fmt.Sprintf("[%d] %s", i+1, text))
		}
		path := filepath.Join(outputDir, "asr_transcript.txt")
		os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
		fmt.Printf("Saved: %s\n", path)
	}

	// Save LLM responses
	if len(r.LLMResponses) > 0 {
		var lines []string
		for i, text := range r.LLMResponses {
			lines = append(lines, fmt.Sprintf("[%d] %s", i+1, text))
		}
		path := filepath.Join(outputDir, "llm_response.txt")
		os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
		fmt.Printf("Saved: %s\n", path)
	}

	// Save TTS audio as WAV
	if len(r.TTSAudio) > 0 {
		path := filepath.Join(outputDir, "tts_output.wav")
		if err := saveAsWAV(r.TTSAudio, path); err != nil {
			fmt.Printf("Failed to save WAV: %v\n", err)
		} else {
			fmt.Printf("Saved: %s\n", path)
		}
	}

	fmt.Println()
	fmt.Println("===========================================")
	if len(r.ASRTexts) > 0 && len(r.TTSAudio) > 0 {
		fmt.Println("  TEST PASSED")
	} else {
		fmt.Println("  TEST FAILED")
	}
	fmt.Println("===========================================")
}

// saveAsWAV saves PCM data as WAV file
func saveAsWAV(pcmData []byte, filePath string) error {
	header := createWAVHeader(len(pcmData))
	return os.WriteFile(filePath, append(header, pcmData...), 0644)
}

// createWAVHeader creates WAV header for 24kHz mono 16-bit
func createWAVHeader(dataSize int) []byte {
	header := make([]byte, 44)

	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+dataSize))
	copy(header[8:12], "WAVE")

	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], 1)
	binary.LittleEndian.PutUint32(header[24:28], outputSampleRate)
	binary.LittleEndian.PutUint32(header[28:32], outputSampleRate*2)
	binary.LittleEndian.PutUint16(header[32:34], 2)
	binary.LittleEndian.PutUint16(header[34:36], 16)

	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(dataSize))

	return header
}

// decodeToPCM decodes audio file to PCM using ffmpeg
func decodeToPCM(ctx context.Context, path string, sampleRate, channels int) ([]byte, error) {
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-i", path,
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
			return nil, fmt.Errorf("ffmpeg: %w (%s)", err, strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}

	return output, nil
}

// Helper functions

func findRepoRoot() (string, error) {
	dir, _ := os.Getwd()
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
	return "", errors.New("repo root not found")
}

func loadEnv(root string) error {
	path := filepath.Join(root, ".env")
	if _, err := os.Stat(path); err != nil {
		return err
	}
	return godotenv.Overload(path)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
