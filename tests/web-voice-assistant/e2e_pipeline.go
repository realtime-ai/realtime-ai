// End-to-End Pipeline Test
//
// Tests the complete voice assistant pipeline:
//   TTS (generate test audio) → ASR → Chat → TTS (response)
//
// Usage:
//   go run tests/web-voice-assistant/e2e_pipeline.go
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
	"github.com/realtime-ai/realtime-ai/pkg/asr"
	"github.com/realtime-ai/realtime-ai/pkg/tts"
)

type PipelineResult struct {
	InputText      string
	GeneratedAudio []byte
	ASRText        string
	LLMResponse    string
	OutputAudio    []byte
}

func main() {
	// Load .env
	godotenv.Load()
	godotenv.Load("../../.env")

	openaiKey := os.Getenv("OPENAI_API_KEY")
	elevenlabsKey := os.Getenv("ELEVENLABS_API_KEY")
	baseURL := os.Getenv("OPENAI_BASE_URL")

	if openaiKey == "" {
		log.Fatal("OPENAI_API_KEY is required")
	}
	if elevenlabsKey == "" {
		log.Fatal("ELEVENLABS_API_KEY is required")
	}

	log.Println("===========================================")
	log.Println("  End-to-End Voice Assistant Pipeline Test")
	log.Println("===========================================")
	log.Println()
	log.Println("Pipeline: Input Text → TTS → ASR → Chat → TTS → Output Audio")
	log.Println()

	// Test cases
	testCases := []string{
		"Hello, how are you today?",
		"What is the capital of France?",
	}

	passed := 0
	failed := 0
	outputDir := filepath.Join("tests", "web-voice-assistant", "output")
	os.MkdirAll(outputDir, 0755)

	for i, inputText := range testCases {
		log.Printf("=== Test Case %d ===", i+1)
		result, err := runPipelineTest(openaiKey, elevenlabsKey, baseURL, inputText)
		if err != nil {
			log.Printf("[FAIL] %v", err)
			failed++
			continue
		}

		// Print results
		log.Printf("  Input:       %s", result.InputText)
		log.Printf("  ASR Result:  %s", result.ASRText)
		log.Printf("  LLM Response: %s", truncate(result.LLMResponse, 80))
		log.Printf("  Input Audio: %d bytes", len(result.GeneratedAudio))
		log.Printf("  Output Audio: %d bytes", len(result.OutputAudio))

		// Save outputs
		inputPath := filepath.Join(outputDir, fmt.Sprintf("e2e_test%d_input.wav", i+1))
		outputPath := filepath.Join(outputDir, fmt.Sprintf("e2e_test%d_output.wav", i+1))

		saveWAV(result.GeneratedAudio, inputPath, 16000)
		saveWAV(result.OutputAudio, outputPath, 16000)
		log.Printf("  Saved: %s, %s", inputPath, outputPath)

		log.Println("  [PASS]")
		passed++
		log.Println()
	}

	// Summary
	log.Println("===========================================")
	log.Printf("  Results: %d passed, %d failed", passed, failed)
	log.Println("===========================================")

	// Save summary
	summaryPath := filepath.Join(outputDir, "e2e_summary.txt")
	summary := fmt.Sprintf("End-to-End Pipeline Test Results\n\nPassed: %d\nFailed: %d\n", passed, failed)
	os.WriteFile(summaryPath, []byte(summary), 0644)

	if failed > 0 {
		os.Exit(1)
	}
}

func runPipelineTest(openaiKey, elevenlabsKey, baseURL, inputText string) (*PipelineResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result := &PipelineResult{
		InputText: inputText,
	}

	// Step 1: Generate audio from input text using TTS
	log.Println("  [1/4] Generating input audio (TTS)...")
	audioData, err := generateAudio(ctx, elevenlabsKey, inputText)
	if err != nil {
		return nil, err
	}
	result.GeneratedAudio = audioData

	// Step 2: Transcribe audio using ASR
	log.Println("  [2/4] Transcribing audio (ASR)...")
	asrText, err := transcribeAudio(ctx, openaiKey, audioData)
	if err != nil {
		return nil, err
	}
	result.ASRText = asrText

	// Step 3: Get response from LLM
	log.Println("  [3/4] Getting LLM response (Chat)...")
	llmResponse, err := chatCompletion(ctx, openaiKey, baseURL, asrText)
	if err != nil {
		return nil, err
	}
	result.LLMResponse = llmResponse

	// Step 4: Generate audio response using TTS
	log.Println("  [4/4] Generating response audio (TTS)...")
	outputAudio, err := generateAudio(ctx, elevenlabsKey, llmResponse)
	if err != nil {
		return nil, err
	}
	result.OutputAudio = outputAudio

	return result, nil
}

func generateAudio(ctx context.Context, apiKey, text string) ([]byte, error) {
	config := tts.ElevenLabsHTTPTTSConfig{
		APIKey:              apiKey,
		VoiceID:             "21m00Tcm4TlvDq8ikWAM", // Rachel
		Model:               "eleven_multilingual_v2",
		LatencyOptimization: 3,
	}

	provider, err := tts.NewElevenLabsHTTPTTSProvider(config)
	if err != nil {
		return nil, err
	}

	resp, err := provider.Synthesize(ctx, &tts.SynthesizeRequest{Text: text})
	if err != nil {
		return nil, err
	}

	return resp.AudioData, nil
}

func transcribeAudio(ctx context.Context, apiKey string, audioData []byte) (string, error) {
	provider, err := asr.NewWhisperProvider(apiKey)
	if err != nil {
		return "", err
	}

	audioConfig := asr.AudioConfig{
		SampleRate: 16000,
		Channels:   1,
		Encoding:   "pcm",
	}

	recognitionConfig := asr.RecognitionConfig{
		Model:    "whisper-1",
		Language: "en",
	}

	result, err := provider.Recognize(ctx, bytes.NewReader(audioData), audioConfig, recognitionConfig)
	if err != nil {
		return "", err
	}

	return result.Text, nil
}

func chatCompletion(ctx context.Context, apiKey, baseURL, userMessage string) (string, error) {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	client := openai.NewClient(opts...)

	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("You are a helpful voice assistant. Reply concisely in 1-2 sentences."),
			openai.UserMessage(userMessage),
		},
		Model: shared.ChatModel("gpt-4o-mini"),
	}

	resp, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", err
	}

	return resp.Choices[0].Message.Content, nil
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
