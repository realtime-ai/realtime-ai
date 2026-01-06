// LLM -> SentenceSegmenter -> TTS Pipeline Latency Test
//
// This test measures the latency of each stage in the LLM to TTS pipeline:
//   - LLM: OpenAI Chat Completion (streaming)
//   - SentenceSegmenter: Rule-based sentence boundary detection
//   - TTS: OpenAI TTS API
//
// Key metrics:
//   - Time to first LLM token
//   - Time to first complete sentence
//   - Time to first TTS audio chunk
//   - Total end-to-end latency
//
// Usage:
//
//	go run tests/llm_tts_pipeline/llm_tts_pipeline_test.go
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/joho/godotenv"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
	"github.com/realtime-ai/realtime-ai/pkg/tts"
)

// LatencyMetrics holds timing information for each pipeline stage
type LatencyMetrics struct {
	RequestStart       time.Time
	FirstLLMToken      time.Time
	FirstSentenceReady time.Time
	FirstTTSChunk      time.Time
	PipelineEnd        time.Time

	// Derived metrics
	LLMFirstTokenLatency    time.Duration
	SentenceSegmentLatency  time.Duration
	TTSFirstChunkLatency    time.Duration
	LLMToTTSLatency         time.Duration // Key metric: first token to first audio
	TotalEndToEndLatency    time.Duration
}

// SegmentMode defines the segmentation strategy
type SegmentMode int

const (
	ModeSentence SegmentMode = iota // Wait for complete sentences (.!?)
	ModePhrase                      // Split on phrases (,;: etc.) - lower latency
	ModeHybrid                      // First segment uses Phrase, rest use Sentence (best of both)
)

// TTSMode defines whether to use streaming or non-streaming TTS
type TTSMode int

const (
	TTSNonStreaming TTSMode = iota // Wait for full TTS response
	TTSStreaming                   // Stream TTS audio chunks - lower TTFB
)

// TextSegmenter provides configurable text boundary detection
type TextSegmenter struct {
	buffer              strings.Builder
	minLen              int
	mode                SegmentMode
	firstSegmentEmitted bool // For ModeHybrid: track if first segment was sent
}

func NewTextSegmenter(minLen int, mode SegmentMode) *TextSegmenter {
	return &TextSegmenter{minLen: minLen, mode: mode}
}

func (s *TextSegmenter) Feed(text string) []string {
	s.buffer.WriteString(text)
	return s.extractSegments()
}

func (s *TextSegmenter) Flush() string {
	remaining := strings.TrimSpace(s.buffer.String())
	s.buffer.Reset()
	return remaining
}

func (s *TextSegmenter) extractSegments() []string {
	var segments []string
	text := s.buffer.String()

	// Define delimiters based on mode
	var delimiters string
	switch s.mode {
	case ModePhrase:
		// More aggressive splitting for lower latency
		delimiters = ".!?;:,。！？；：，"
	case ModeHybrid:
		// Hybrid mode: use Phrase delimiters for first segment (fast TTFB),
		// then switch to Sentence delimiters for natural speech quality
		if !s.firstSegmentEmitted {
			delimiters = ".!?;:,。！？；：，"
		} else {
			delimiters = ".!?。！？"
		}
	default:
		// Sentence-level splitting
		delimiters = ".!?。！？"
	}

	lastEnd := 0

	for i, r := range text {
		if strings.ContainsRune(delimiters, r) {
			segment := strings.TrimSpace(text[lastEnd : i+utf8.RuneLen(r)])
			if len(segment) >= s.minLen {
				segments = append(segments, segment)
				lastEnd = i + utf8.RuneLen(r)
				// For hybrid mode: mark first segment as emitted
				if s.mode == ModeHybrid && !s.firstSegmentEmitted {
					s.firstSegmentEmitted = true
				}
			}
		}
	}

	// Keep remaining text in buffer
	s.buffer.Reset()
	s.buffer.WriteString(text[lastEnd:])

	return segments
}

// Legacy alias for backward compatibility
func NewSentenceSegmenter(minLen int) *TextSegmenter {
	return NewTextSegmenter(minLen, ModeSentence)
}

func main() {
	// Load environment variables
	godotenv.Load()
	godotenv.Load("../../.env")

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY is required")
	}

	baseURL := os.Getenv("OPENAI_BASE_URL")

	log.Println("╔═══════════════════════════════════════════════════════════════╗")
	log.Println("║   LLM -> Segmenter -> TTS Pipeline Latency Comparison Test   ║")
	log.Println("╚═══════════════════════════════════════════════════════════════╝")
	log.Println()
	log.Println("Comparing segmentation strategies:")
	log.Println("  • Sentence: Full sentences only (.!?)")
	log.Println("  • Phrase: Split on phrases (,;:) - lower TTFB")
	log.Println("  • Hybrid: First segment uses Phrase, then Sentence (best of both)")
	log.Println()

	// Test prompt for comparison
	prompt := "Explain what Go programming language is in 2-3 sentences."

	// Create output directory
	outputDir := filepath.Join("tests", "llm_tts_pipeline", "output")
	os.MkdirAll(outputDir, 0755)

	// Test configurations
	configs := []struct {
		name     string
		segMode  SegmentMode
		ttsMode  TTSMode
	}{
		{"1. Sentence + Non-streaming TTS (baseline)", ModeSentence, TTSNonStreaming},
		{"2. Phrase + Non-streaming TTS", ModePhrase, TTSNonStreaming},
		{"3. Hybrid (Phrase first, then Sentence) + Non-streaming TTS", ModeHybrid, TTSNonStreaming},
	}

	var allMetrics []LatencyMetrics
	var configNames []string

	for _, cfg := range configs {
		log.Println()
		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Printf("Testing: %s", cfg.name)
		log.Printf("Prompt: %s", prompt)
		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		metrics, audioData, err := runPipelineTestWithConfig(apiKey, baseURL, prompt, cfg.segMode, cfg.ttsMode)
		if err != nil {
			log.Printf("  ❌ FAILED: %v", err)
			continue
		}

		printMetrics(metrics)

		// Save audio
		audioPath := filepath.Join(outputDir, fmt.Sprintf("%s.wav", sanitizeFilename(cfg.name)))
		if len(audioData) > 0 {
			saveWAV(audioData, audioPath, 24000)
			log.Printf("  📁 Audio saved: %s", audioPath)
		}

		allMetrics = append(allMetrics, *metrics)
		configNames = append(configNames, cfg.name)
	}

	// Print comparison
	printComparisonMulti(allMetrics, configNames)

	// Save report
	saveComparisonReport(outputDir, allMetrics, configNames)
}

func runPipelineTestWithConfig(apiKey, baseURL, prompt string, segMode SegmentMode, ttsMode TTSMode) (*LatencyMetrics, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	metrics := &LatencyMetrics{
		RequestStart: time.Now(),
	}

	// Stage 1: LLM with streaming
	log.Println("  [1/3] LLM: Sending request to OpenAI...")

	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	client := openai.NewClient(opts...)

	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("You are a helpful assistant. Respond concisely and naturally."),
			openai.UserMessage(prompt),
		},
		Model: shared.ChatModel("gpt-4o-mini"),
	}

	stream := client.Chat.Completions.NewStreaming(ctx, params)

	// Stage 2: Process streaming tokens with configurable segmentation
	segmenter := NewTextSegmenter(5, segMode)
	var fullResponse strings.Builder
	var segments []string
	firstTokenReceived := false
	firstSegmentReady := false

	segmentChan := make(chan string, 10)
	var wg sync.WaitGroup

	// Goroutine for TTS processing
	var allAudioData []byte
	var audioMu sync.Mutex
	ttsProvider := tts.NewOpenAITTSProvider(apiKey)
	firstTTSChunkReceived := false

	wg.Add(1)
	go func() {
		defer wg.Done()
		for segment := range segmentChan {
			log.Printf("  [2/3] Segmenter: Got segment: %q", truncate(segment, 50))

			ttsStart := time.Now()

			if ttsMode == TTSStreaming {
				// Use streaming TTS for lower TTFB
				log.Println("  [3/3] TTS: Streaming synthesis...")
				audioChan, errChan := ttsProvider.StreamSynthesize(ctx, &tts.SynthesizeRequest{
					Text:  segment,
					Voice: "coral",
				})

				for {
					select {
					case chunk, ok := <-audioChan:
						if !ok {
							goto done
						}
						audioMu.Lock()
						if !firstTTSChunkReceived {
							metrics.FirstTTSChunk = time.Now()
							firstTTSChunkReceived = true
							log.Printf("  [3/3] TTS: First chunk received! (TTFB: %v)", time.Since(ttsStart))
						}
						allAudioData = append(allAudioData, chunk...)
						audioMu.Unlock()
					case err := <-errChan:
						if err != nil {
							log.Printf("  Warning: TTS streaming error: %v", err)
						}
						goto done
					}
				}
			done:
				audioMu.Lock()
				log.Printf("  [3/3] TTS: Streamed %d bytes total", len(allAudioData))
				audioMu.Unlock()
			} else {
				// Non-streaming TTS (original behavior)
				log.Println("  [3/3] TTS: Non-streaming synthesis...")
				resp, err := ttsProvider.Synthesize(ctx, &tts.SynthesizeRequest{
					Text:  segment,
					Voice: "coral",
				})
				if err != nil {
					log.Printf("  Warning: TTS failed: %v", err)
					continue
				}

				audioMu.Lock()
				if !firstTTSChunkReceived {
					metrics.FirstTTSChunk = time.Now()
					firstTTSChunkReceived = true
					log.Printf("  [3/3] TTS: Audio received (latency: %v)", time.Since(ttsStart))
				}
				allAudioData = append(allAudioData, resp.AudioData...)
				audioMu.Unlock()

				log.Printf("  [3/3] TTS: Generated %d bytes", len(resp.AudioData))
			}
		}
	}()

	// Process streaming tokens
	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta.Content
		if delta == "" {
			continue
		}

		// Record first token timing
		if !firstTokenReceived {
			metrics.FirstLLMToken = time.Now()
			firstTokenReceived = true
			log.Printf("  [1/3] LLM: First token received (latency: %v)", metrics.FirstLLMToken.Sub(metrics.RequestStart))
		}

		fullResponse.WriteString(delta)

		// Feed to segmenter
		newSegments := segmenter.Feed(delta)
		for _, s := range newSegments {
			if !firstSegmentReady {
				metrics.FirstSentenceReady = time.Now()
				firstSegmentReady = true
				log.Printf("  [2/3] Segmenter: First segment ready! (latency: %v)", metrics.FirstSentenceReady.Sub(metrics.RequestStart))
			}
			segments = append(segments, s)
			segmentChan <- s
		}
	}

	if err := stream.Err(); err != nil {
		close(segmentChan)
		return nil, nil, fmt.Errorf("streaming error: %w", err)
	}

	// Flush remaining text
	remaining := segmenter.Flush()
	if remaining != "" {
		if !firstSegmentReady {
			metrics.FirstSentenceReady = time.Now()
		}
		segments = append(segments, remaining)
		segmentChan <- remaining
	}

	close(segmentChan)
	wg.Wait()

	metrics.PipelineEnd = time.Now()

	// Calculate derived metrics
	if !metrics.FirstLLMToken.IsZero() {
		metrics.LLMFirstTokenLatency = metrics.FirstLLMToken.Sub(metrics.RequestStart)
	}
	if !metrics.FirstSentenceReady.IsZero() {
		metrics.SentenceSegmentLatency = metrics.FirstSentenceReady.Sub(metrics.FirstLLMToken)
	}
	if !metrics.FirstTTSChunk.IsZero() {
		metrics.TTSFirstChunkLatency = metrics.FirstTTSChunk.Sub(metrics.FirstSentenceReady)
		// Key metric: LLM first token to TTS first chunk
		metrics.LLMToTTSLatency = metrics.FirstTTSChunk.Sub(metrics.FirstLLMToken)
	}
	metrics.TotalEndToEndLatency = metrics.PipelineEnd.Sub(metrics.RequestStart)

	log.Printf("  LLM Response: %q", truncate(fullResponse.String(), 80))
	log.Printf("  Segments extracted: %d", len(segments))

	return metrics, allAudioData, nil
}

// Legacy functions for backward compatibility
func runPipelineTestWithMode(apiKey, baseURL, prompt string, mode SegmentMode) (*LatencyMetrics, []byte, error) {
	return runPipelineTestWithConfig(apiKey, baseURL, prompt, mode, TTSNonStreaming)
}

func runPipelineTest(apiKey, baseURL, prompt string) (*LatencyMetrics, []byte, error) {
	return runPipelineTestWithConfig(apiKey, baseURL, prompt, ModeSentence, TTSNonStreaming)
}

func printMetrics(m *LatencyMetrics) {
	log.Println()
	log.Println("  ┌─────────────────────────────────────────────────────────────┐")
	log.Println("  │                    LATENCY METRICS                          │")
	log.Println("  ├─────────────────────────────────────────────────────────────┤")
	log.Printf("  │ LLM First Token Latency:        %12v               │", m.LLMFirstTokenLatency)
	log.Printf("  │ Sentence Segmentation Latency:  %12v               │", m.SentenceSegmentLatency)
	log.Printf("  │ TTS First Chunk Latency:        %12v               │", m.TTSFirstChunkLatency)
	log.Println("  ├─────────────────────────────────────────────────────────────┤")
	log.Printf("  │ ★ LLM Token → TTS Chunk:        %12v               │", m.LLMToTTSLatency)
	log.Println("  ├─────────────────────────────────────────────────────────────┤")
	log.Printf("  │ Total End-to-End Latency:       %12v               │", m.TotalEndToEndLatency)
	log.Println("  └─────────────────────────────────────────────────────────────┘")
	log.Println()
}

func printComparisonMulti(metrics []LatencyMetrics, names []string) {
	if len(metrics) < 2 {
		return
	}

	log.Println()
	log.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
	log.Println("║                      LATENCY COMPARISON: All Configurations                  ║")
	log.Println("╠══════════════════════════════════════════════════════════════════════════════╣")

	baseline := metrics[0]

	// Print each configuration
	for i, m := range metrics {
		name := names[i]
		if i == 0 {
			log.Printf("║  %s", name)
			log.Printf("║    ★ LLM→TTS: %v (baseline)", m.LLMToTTSLatency)
		} else {
			improvement := float64(baseline.LLMToTTSLatency-m.LLMToTTSLatency) / float64(baseline.LLMToTTSLatency) * 100
			log.Printf("║  %s", name)
			log.Printf("║    ★ LLM→TTS: %v (%+.1f%% vs baseline)", m.LLMToTTSLatency, improvement)
		}
		log.Println("║")
	}

	log.Println("╠══════════════════════════════════════════════════════════════════════════════╣")

	// Best result
	best := metrics[len(metrics)-1]
	totalImprovement := float64(baseline.LLMToTTSLatency-best.LLMToTTSLatency) / float64(baseline.LLMToTTSLatency) * 100
	timeSaved := baseline.LLMToTTSLatency - best.LLMToTTSLatency

	log.Printf("║  🏆 Best Configuration: %s", names[len(names)-1])
	log.Printf("║     Total Improvement: %.1f%% (saved %v)", totalImprovement, timeSaved)
	log.Println("╚══════════════════════════════════════════════════════════════════════════════╝")
	log.Println()

	log.Println("💡 Summary:")
	log.Printf("   • Baseline LLM→TTS:  %v", baseline.LLMToTTSLatency)
	log.Printf("   • Optimized LLM→TTS: %v", best.LLMToTTSLatency)
	log.Printf("   • Improvement:       %.1f%%", totalImprovement)
}

func saveComparisonReport(outputDir string, metrics []LatencyMetrics, names []string) {
	reportPath := filepath.Join(outputDir, "latency_comparison.txt")

	var report strings.Builder
	report.WriteString("LLM -> Segmenter -> TTS Pipeline Latency Comparison\n")
	report.WriteString("===================================================\n\n")
	report.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format(time.RFC3339)))

	for i, m := range metrics {
		name := "Unknown"
		if i < len(names) {
			name = names[i]
		}
		report.WriteString(fmt.Sprintf("%s:\n", name))
		report.WriteString(fmt.Sprintf("  LLM First Token:       %v\n", m.LLMFirstTokenLatency))
		report.WriteString(fmt.Sprintf("  Segment Latency:       %v\n", m.SentenceSegmentLatency))
		report.WriteString(fmt.Sprintf("  TTS First Chunk:       %v\n", m.TTSFirstChunkLatency))
		report.WriteString(fmt.Sprintf("  LLM→TTS Latency:       %v (KEY)\n", m.LLMToTTSLatency))
		report.WriteString(fmt.Sprintf("  Total End-to-End:      %v\n\n", m.TotalEndToEndLatency))
	}

	if len(metrics) >= 2 {
		sentence := metrics[0]
		phrase := metrics[1]
		improvement := float64(sentence.LLMToTTSLatency-phrase.LLMToTTSLatency) / float64(sentence.LLMToTTSLatency) * 100

		report.WriteString("Comparison Summary:\n")
		report.WriteString(fmt.Sprintf("  LLM→TTS Improvement: %.1f%%\n", improvement))
		report.WriteString(fmt.Sprintf("  Time Saved: %v\n", sentence.LLMToTTSLatency-phrase.LLMToTTSLatency))
	}

	os.WriteFile(reportPath, []byte(report.String()), 0644)
	log.Printf("Report saved: %s", reportPath)
}

func saveWAV(pcmData []byte, filePath string, sampleRate int) error {
	header := make([]byte, 44)
	dataSize := len(pcmData)

	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+dataSize))
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], 1)
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(header[28:32], uint32(sampleRate*2))
	binary.LittleEndian.PutUint16(header[32:34], 2)
	binary.LittleEndian.PutUint16(header[34:36], 16)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(dataSize))

	return os.WriteFile(filePath, append(header, pcmData...), 0644)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func sanitizeFilename(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), " ", "_")
}

// Unused import fix
var _ = bytes.NewReader
