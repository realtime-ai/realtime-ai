// ChatElement Integration Test
//
// This is a standalone test for ChatElement that doesn't require
// the full elements package to be buildable.
//
// Usage:
//   go run tests/web-voice-assistant/chat_test.go
package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

func main() {
	// Load .env
	godotenv.Load()
	godotenv.Load("../../.env")

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY is required")
	}

	baseURL := os.Getenv("OPENAI_BASE_URL")
	if baseURL != "" {
		log.Printf("Using base URL: %s", baseURL)
	}

	log.Println("===========================================")
	log.Println("  ChatElement Integration Test")
	log.Println("===========================================")
	log.Println()

	// Run tests
	passed := 0
	failed := 0

	if testBasicChat(apiKey, baseURL) {
		passed++
	} else {
		failed++
	}

	if testStreamingChat(apiKey, baseURL) {
		passed++
	} else {
		failed++
	}

	if testConversationHistory(apiKey, baseURL) {
		passed++
	} else {
		failed++
	}

	// Summary
	log.Println()
	log.Println("===========================================")
	log.Printf("  Results: %d passed, %d failed", passed, failed)
	log.Println("===========================================")

	if failed > 0 {
		os.Exit(1)
	}
}

func testBasicChat(apiKey, baseURL string) bool {
	log.Println("[TEST] Basic Chat Completion")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	client := openai.NewClient(opts...)

	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("Reply with exactly one word: OK"),
			openai.UserMessage("Hello"),
		},
		Model: shared.ChatModel("gpt-4o-mini"),
	}

	resp, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		log.Printf("  [FAIL] Error: %v", err)
		return false
	}

	if len(resp.Choices) == 0 {
		log.Println("  [FAIL] No response choices")
		return false
	}

	response := resp.Choices[0].Message.Content
	log.Printf("  Response: %s", response)
	log.Println("  [PASS]")
	return true
}

func testStreamingChat(apiKey, baseURL string) bool {
	log.Println("[TEST] Streaming Chat Completion")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	client := openai.NewClient(opts...)

	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("Reply with exactly 3 short sentences."),
			openai.UserMessage("Tell me about Go programming."),
		},
		Model: shared.ChatModel("gpt-4o-mini"),
	}

	stream := client.Chat.Completions.NewStreaming(ctx, params)

	var chunks []string
	var fullResponse strings.Builder

	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta.Content
			if delta != "" {
				chunks = append(chunks, delta)
				fullResponse.WriteString(delta)
			}
		}
	}

	if err := stream.Err(); err != nil {
		log.Printf("  [FAIL] Error: %v", err)
		return false
	}

	if len(chunks) == 0 {
		log.Println("  [FAIL] No streaming chunks received")
		return false
	}

	log.Printf("  Received %d chunks", len(chunks))
	log.Printf("  Response: %s", truncate(fullResponse.String(), 100))
	log.Println("  [PASS]")
	return true
}

func testConversationHistory(apiKey, baseURL string) bool {
	log.Println("[TEST] Conversation History")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	client := openai.NewClient(opts...)

	// First message - tell the assistant a name
	history := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("You are a helpful assistant. Remember what the user tells you."),
		openai.UserMessage("My name is TestUser123."),
	}

	params := openai.ChatCompletionNewParams{
		Messages: history,
		Model:    shared.ChatModel("gpt-4o-mini"),
	}

	resp1, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		log.Printf("  [FAIL] First message error: %v", err)
		return false
	}

	if len(resp1.Choices) == 0 {
		log.Println("  [FAIL] No response to first message")
		return false
	}

	assistantReply1 := resp1.Choices[0].Message.Content
	log.Printf("  First response: %s", truncate(assistantReply1, 80))

	// Add to history
	history = append(history, openai.AssistantMessage(assistantReply1))
	history = append(history, openai.UserMessage("What is my name?"))

	params2 := openai.ChatCompletionNewParams{
		Messages: history,
		Model:    shared.ChatModel("gpt-4o-mini"),
	}

	resp2, err := client.Chat.Completions.New(ctx, params2)
	if err != nil {
		log.Printf("  [FAIL] Second message error: %v", err)
		return false
	}

	if len(resp2.Choices) == 0 {
		log.Println("  [FAIL] No response to second message")
		return false
	}

	assistantReply2 := resp2.Choices[0].Message.Content
	log.Printf("  Second response: %s", truncate(assistantReply2, 80))

	// Check if the name is remembered
	if !strings.Contains(strings.ToLower(assistantReply2), "testuser123") {
		log.Println("  [FAIL] Name not remembered in response")
		return false
	}

	log.Println("  [PASS] Name correctly remembered")
	return true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
