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
)

// TextSegmenter provides configurable text boundary detection
type TextSegmenter struct {
	buffer strings.Builder
	minLen int
	mode   SegmentMode
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
	log.Println("Comparing two segmentation strategies:")
	log.Println("  • Sentence Mode: Wait for complete sentences (.!?)")
	log.Println("  • Phrase Mode:   Split on phrases (,;: etc.) - LOWER LATENCY")
	log.Println()

	// Test prompt for comparison
	prompt := "Explain what Go programming language is in 2-3 sentences."

	// Create output directory
	outputDir := filepath.Join("tests", "llm_tts_pipeline", "output")
	os.MkdirAll(outputDir, 0755)

	// Test both modes
	modes := []struct {
		name string
		mode SegmentMode
	}{
		{"Sentence Mode (baseline)", ModeSentence},
		{"Phrase Mode (optimized)", ModePhrase},
	}

	var allMetrics []LatencyMetrics
	var modeNames []string

	for _, m := range modes {
		log.Println()
		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Printf("Testing: %s", m.name)
		log.Printf("Prompt: %s", prompt)
		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		metrics, audioData, err := runPipelineTestWithMode(apiKey, baseURL, prompt, m.mode)
		if err != nil {
			log.Printf("  ❌ FAILED: %v", err)
			continue
		}

		printMetrics(metrics)

		// Save audio
		audioPath := filepath.Join(outputDir, fmt.Sprintf("%s.wav", sanitizeFilename(m.name)))
		if len(audioData) > 0 {
			saveWAV(audioData, audioPath, 24000)
			log.Printf("  📁 Audio saved: %s", audioPath)
		}

		allMetrics = append(allMetrics, *metrics)
		modeNames = append(modeNames, m.name)
	}

	// Print comparison
	printComparison(allMetrics, modeNames)

	// Save report
	saveComparisonReport(outputDir, allMetrics, modeNames)
}

func runPipelineTestWithMode(apiKey, baseURL, prompt string, mode SegmentMode) (*LatencyMetrics, []byte, error) {
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
	segmenter := NewTextSegmenter(5, mode) // Use configurable mode
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

			// TTS synthesis
			log.Printf("  [3/3] TTS: Synthesizing audio...")
			ttsStart := time.Now()

			resp, err := ttsProvider.Synthesize(ctx, &tts.SynthesizeRequest{
				Text:  segment,
				Voice: "alloy",
			})
			if err != nil {
				log.Printf("  Warning: TTS failed: %v", err)
				continue
			}

			audioMu.Lock()
			if !firstTTSChunkReceived {
				metrics.FirstTTSChunk = time.Now()
				firstTTSChunkReceived = true
				log.Printf("  [3/3] TTS: First audio chunk! (TTS latency: %v)", time.Since(ttsStart))
			}
			allAudioData = append(allAudioData, resp.AudioData...)
			audioMu.Unlock()

			log.Printf("  [3/3] TTS: Generated %d bytes", len(resp.AudioData))
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

// Legacy function for backward compatibility
func runPipelineTest(apiKey, baseURL, prompt string) (*LatencyMetrics, []byte, error) {
	return runPipelineTestWithMode(apiKey, baseURL, prompt, ModeSentence)
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

func printComparison(metrics []LatencyMetrics, names []string) {
	if len(metrics) < 2 {
		return
	}

	log.Println()
	log.Println("╔═══════════════════════════════════════════════════════════════╗")
	log.Println("║              LATENCY COMPARISON: Sentence vs Phrase           ║")
	log.Println("╠═══════════════════════════════════════════════════════════════╣")

	// Header
	log.Println("║                          Sentence     Phrase      Improvement ║")
	log.Println("╠═══════════════════════════════════════════════════════════════╣")

	sentence := metrics[0]
	phrase := metrics[1]

	// Segment latency comparison
	segImprove := float64(sentence.SentenceSegmentLatency-phrase.SentenceSegmentLatency) / float64(sentence.SentenceSegmentLatency) * 100
	log.Printf("║  Segment Latency:    %10v  %10v     %+6.1f%%    ║",
		sentence.SentenceSegmentLatency, phrase.SentenceSegmentLatency, segImprove)

	// TTS latency comparison
	ttsImprove := float64(sentence.TTSFirstChunkLatency-phrase.TTSFirstChunkLatency) / float64(sentence.TTSFirstChunkLatency) * 100
	log.Printf("║  TTS First Chunk:    %10v  %10v     %+6.1f%%    ║",
		sentence.TTSFirstChunkLatency, phrase.TTSFirstChunkLatency, ttsImprove)

	// Key metric: LLM to TTS
	llmTTSImprove := float64(sentence.LLMToTTSLatency-phrase.LLMToTTSLatency) / float64(sentence.LLMToTTSLatency) * 100
	log.Println("╠═══════════════════════════════════════════════════════════════╣")
	log.Printf("║  ★ LLM→TTS:          %10v  %10v     %+6.1f%%    ║",
		sentence.LLMToTTSLatency, phrase.LLMToTTSLatency, llmTTSImprove)
	log.Println("╠═══════════════════════════════════════════════════════════════╣")

	// Total latency
	totalImprove := float64(sentence.TotalEndToEndLatency-phrase.TotalEndToEndLatency) / float64(sentence.TotalEndToEndLatency) * 100
	log.Printf("║  Total End-to-End:   %10v  %10v     %+6.1f%%    ║",
		sentence.TotalEndToEndLatency, phrase.TotalEndToEndLatency, totalImprove)

	log.Println("╚═══════════════════════════════════════════════════════════════╝")
	log.Println()

	// Additional insights
	log.Println("💡 Optimization Insights:")
	if llmTTSImprove > 0 {
		log.Printf("   • Phrase mode reduced LLM→TTS latency by %.1f%%", llmTTSImprove)
		log.Printf("   • Time saved: %v", sentence.LLMToTTSLatency-phrase.LLMToTTSLatency)
	}
	log.Println()
	log.Println("📊 Further Optimization Options:")
	log.Println("   1. Use streaming TTS (ElevenLabs WebSocket) for even lower latency")
	log.Println("   2. Use OpenAI Realtime API for native voice output (~200ms)")
	log.Println("   3. Reduce minLen to trigger TTS on shorter phrases")
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
