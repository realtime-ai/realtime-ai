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

// SentenceSegmenter provides simple sentence boundary detection
type SentenceSegmenter struct {
	buffer   strings.Builder
	minLen   int
}

func NewSentenceSegmenter(minLen int) *SentenceSegmenter {
	return &SentenceSegmenter{minLen: minLen}
}

func (s *SentenceSegmenter) Feed(text string) []string {
	s.buffer.WriteString(text)
	return s.extractSentences()
}

func (s *SentenceSegmenter) Flush() string {
	remaining := strings.TrimSpace(s.buffer.String())
	s.buffer.Reset()
	return remaining
}

func (s *SentenceSegmenter) extractSentences() []string {
	var sentences []string
	text := s.buffer.String()

	sentenceEnders := ".!?。！？"
	lastEnd := 0

	for i, r := range text {
		if strings.ContainsRune(sentenceEnders, r) {
			// Check if we have enough text for a sentence
			sentence := strings.TrimSpace(text[lastEnd : i+utf8.RuneLen(r)])
			if len(sentence) >= s.minLen {
				sentences = append(sentences, sentence)
				lastEnd = i + utf8.RuneLen(r)
			}
		}
	}

	// Keep remaining text in buffer
	s.buffer.Reset()
	s.buffer.WriteString(text[lastEnd:])

	return sentences
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
	log.Println("║     LLM -> SentenceSegmenter -> TTS Pipeline Latency Test    ║")
	log.Println("╚═══════════════════════════════════════════════════════════════╝")
	log.Println()
	log.Println("Pipeline: User Query → LLM (streaming) → SentenceSegmenter → TTS → Audio")
	log.Println()

	// Test cases with different query complexities
	testCases := []struct {
		name   string
		prompt string
	}{
		{
			name:   "Short Response",
			prompt: "Say hello in one sentence.",
		},
		{
			name:   "Medium Response",
			prompt: "Explain what Go programming language is in 2-3 sentences.",
		},
		{
			name:   "Long Response",
			prompt: "Describe the benefits of microservices architecture in 3-4 sentences.",
		},
	}

	// Create output directory
	outputDir := filepath.Join("tests", "llm_tts_pipeline", "output")
	os.MkdirAll(outputDir, 0755)

	var allMetrics []LatencyMetrics
	passed := 0
	failed := 0

	for i, tc := range testCases {
		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Printf("Test Case %d: %s", i+1, tc.name)
		log.Printf("Prompt: %s", tc.prompt)
		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		metrics, audioData, err := runPipelineTest(apiKey, baseURL, tc.prompt)
		if err != nil {
			log.Printf("  ❌ FAILED: %v", err)
			failed++
			continue
		}

		// Print detailed metrics
		printMetrics(metrics)

		// Save audio output
		if len(audioData) > 0 {
			audioPath := filepath.Join(outputDir, fmt.Sprintf("test%d_%s.wav", i+1, sanitizeFilename(tc.name)))
			if err := saveWAV(audioData, audioPath, 24000); err != nil {
				log.Printf("  Warning: Failed to save audio: %v", err)
			} else {
				log.Printf("  📁 Audio saved: %s (%d bytes)", audioPath, len(audioData))
			}
		}

		allMetrics = append(allMetrics, *metrics)
		log.Println("  ✅ PASSED")
		passed++
		log.Println()
	}

	// Print summary
	printSummary(allMetrics, passed, failed)

	// Save detailed report
	saveReport(outputDir, allMetrics, passed, failed)

	if failed > 0 {
		os.Exit(1)
	}
}

func runPipelineTest(apiKey, baseURL, prompt string) (*LatencyMetrics, []byte, error) {
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

	// Stage 2: Process streaming tokens with sentence segmentation
	segmenter := NewSentenceSegmenter(5)
	var fullResponse strings.Builder
	var sentences []string
	firstTokenReceived := false
	firstSentenceReady := false

	sentenceChan := make(chan string, 10)
	var wg sync.WaitGroup

	// Goroutine for TTS processing
	var allAudioData []byte
	var audioMu sync.Mutex
	ttsProvider := tts.NewOpenAITTSProvider(apiKey)
	firstTTSChunkReceived := false

	wg.Add(1)
	go func() {
		defer wg.Done()
		for sentence := range sentenceChan {
			log.Printf("  [2/3] SentenceSegmenter: Got sentence: %q", truncate(sentence, 50))

			// TTS synthesis
			log.Printf("  [3/3] TTS: Synthesizing audio for sentence...")
			ttsStart := time.Now()

			resp, err := ttsProvider.Synthesize(ctx, &tts.SynthesizeRequest{
				Text:  sentence,
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
				log.Printf("  [3/3] TTS: First audio chunk received (latency: %v)", time.Since(ttsStart))
			}
			allAudioData = append(allAudioData, resp.AudioData...)
			audioMu.Unlock()

			log.Printf("  [3/3] TTS: Generated %d bytes of audio", len(resp.AudioData))
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

		// Feed to sentence segmenter
		newSentences := segmenter.Feed(delta)
		for _, s := range newSentences {
			if !firstSentenceReady {
				metrics.FirstSentenceReady = time.Now()
				firstSentenceReady = true
				log.Printf("  [2/3] SentenceSegmenter: First sentence ready (latency: %v)", metrics.FirstSentenceReady.Sub(metrics.RequestStart))
			}
			sentences = append(sentences, s)
			sentenceChan <- s
		}
	}

	if err := stream.Err(); err != nil {
		close(sentenceChan)
		return nil, nil, fmt.Errorf("streaming error: %w", err)
	}

	// Flush remaining text
	remaining := segmenter.Flush()
	if remaining != "" {
		if !firstSentenceReady {
			metrics.FirstSentenceReady = time.Now()
		}
		sentences = append(sentences, remaining)
		sentenceChan <- remaining
	}

	close(sentenceChan)
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

	log.Printf("  [1/3] LLM: Full response: %q", truncate(fullResponse.String(), 100))
	log.Printf("  [2/3] SentenceSegmenter: Extracted %d sentences", len(sentences))

	return metrics, allAudioData, nil
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

func printSummary(metrics []LatencyMetrics, passed, failed int) {
	log.Println()
	log.Println("╔═══════════════════════════════════════════════════════════════╗")
	log.Println("║                        TEST SUMMARY                           ║")
	log.Println("╠═══════════════════════════════════════════════════════════════╣")
	log.Printf("║  Tests Passed: %d                                              ║", passed)
	log.Printf("║  Tests Failed: %d                                              ║", failed)
	log.Println("╠═══════════════════════════════════════════════════════════════╣")

	if len(metrics) > 0 {
		var avgLLMLatency, avgLLMToTTS, avgTotal time.Duration
		for _, m := range metrics {
			avgLLMLatency += m.LLMFirstTokenLatency
			avgLLMToTTS += m.LLMToTTSLatency
			avgTotal += m.TotalEndToEndLatency
		}
		avgLLMLatency /= time.Duration(len(metrics))
		avgLLMToTTS /= time.Duration(len(metrics))
		avgTotal /= time.Duration(len(metrics))

		log.Printf("║  Avg LLM First Token:     %12v                      ║", avgLLMLatency)
		log.Printf("║  Avg LLM→TTS Latency:     %12v  ← KEY METRIC        ║", avgLLMToTTS)
		log.Printf("║  Avg End-to-End:          %12v                      ║", avgTotal)
	}
	log.Println("╚═══════════════════════════════════════════════════════════════╝")
}

func saveReport(outputDir string, metrics []LatencyMetrics, passed, failed int) {
	reportPath := filepath.Join(outputDir, "latency_report.txt")

	var report strings.Builder
	report.WriteString("LLM -> SentenceSegmenter -> TTS Pipeline Latency Report\n")
	report.WriteString("========================================================\n\n")
	report.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format(time.RFC3339)))
	report.WriteString(fmt.Sprintf("Tests Passed: %d\n", passed))
	report.WriteString(fmt.Sprintf("Tests Failed: %d\n\n", failed))

	for i, m := range metrics {
		report.WriteString(fmt.Sprintf("Test %d:\n", i+1))
		report.WriteString(fmt.Sprintf("  LLM First Token Latency:       %v\n", m.LLMFirstTokenLatency))
		report.WriteString(fmt.Sprintf("  Sentence Segmentation Latency: %v\n", m.SentenceSegmentLatency))
		report.WriteString(fmt.Sprintf("  TTS First Chunk Latency:       %v\n", m.TTSFirstChunkLatency))
		report.WriteString(fmt.Sprintf("  LLM Token → TTS Chunk:         %v (KEY METRIC)\n", m.LLMToTTSLatency))
		report.WriteString(fmt.Sprintf("  Total End-to-End Latency:      %v\n\n", m.TotalEndToEndLatency))
	}

	if len(metrics) > 0 {
		var avgLLMLatency, avgLLMToTTS, avgTotal time.Duration
		for _, m := range metrics {
			avgLLMLatency += m.LLMFirstTokenLatency
			avgLLMToTTS += m.LLMToTTSLatency
			avgTotal += m.TotalEndToEndLatency
		}
		avgLLMLatency /= time.Duration(len(metrics))
		avgLLMToTTS /= time.Duration(len(metrics))
		avgTotal /= time.Duration(len(metrics))

		report.WriteString("Averages:\n")
		report.WriteString(fmt.Sprintf("  Avg LLM First Token:      %v\n", avgLLMLatency))
		report.WriteString(fmt.Sprintf("  Avg LLM→TTS Latency:      %v (KEY METRIC)\n", avgLLMToTTS))
		report.WriteString(fmt.Sprintf("  Avg End-to-End:           %v\n", avgTotal))
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
