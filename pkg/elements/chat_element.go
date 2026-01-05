// Package elements provides pipeline processing elements.
//
// ChatElement implements conversational AI using OpenAI Chat Completion API.
// It maintains conversation history and supports streaming responses for low latency.
//
// Main features:
//   - OpenAI Chat Completion API integration (gpt-4o-mini, gpt-4o, etc.)
//   - Conversation history management with configurable limit
//   - Streaming response for reduced time-to-first-token
//   - Integration with pipeline event system
//
// Usage:
//
//	chat, err := NewChatElement(ChatConfig{
//	    APIKey:       "sk-xxx",
//	    Model:        "gpt-4o-mini",
//	    SystemPrompt: "You are a helpful assistant.",
//	    Streaming:    true,
//	})
package elements

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

// Make sure ChatElement implements pipeline.Element
var _ pipeline.Element = (*ChatElement)(nil)

// ChatConfig holds configuration for the chat element
type ChatConfig struct {
	APIKey       string // OpenAI API key
	Model        string // Model name (e.g., "gpt-4o-mini", "gpt-4o")
	SystemPrompt string // System prompt for the assistant
	MaxTokens    int    // Maximum tokens in response (0 = default)
	Streaming    bool   // Enable streaming responses
	MaxHistory   int    // Maximum number of history messages to retain (0 = unlimited)
	Temperature  float64 // Temperature for response generation (0.0-2.0)
}

// ChatElement processes text input through OpenAI Chat Completion API
type ChatElement struct {
	*pipeline.BaseElement

	config  ChatConfig
	client  *openai.Client
	history []openai.ChatCompletionMessageParamUnion

	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// NewChatElement creates a new chat element
func NewChatElement(config ChatConfig) (*ChatElement, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if config.Model == "" {
		config.Model = "gpt-4o-mini"
	}
	if config.SystemPrompt == "" {
		config.SystemPrompt = "You are a helpful voice assistant. Keep your responses concise and conversational."
	}
	if config.MaxHistory == 0 {
		config.MaxHistory = 20 // Default: keep last 20 messages
	}
	if config.Temperature == 0 {
		config.Temperature = 0.7
	}

	return &ChatElement{
		BaseElement: pipeline.NewBaseElement("chat-element", 100),
		config:      config,
		history:     make([]openai.ChatCompletionMessageParamUnion, 0),
	}, nil
}

// Start initializes the chat element and begins processing
func (e *ChatElement) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	// Initialize OpenAI client
	opts := []option.RequestOption{
		option.WithAPIKey(e.config.APIKey),
	}
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	client := openai.NewClient(opts...)
	e.client = &client

	// Start message processing goroutine
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.processLoop(ctx)
	}()

	log.Printf("[ChatElement] Started (model: %s, streaming: %v, max_history: %d)",
		e.config.Model, e.config.Streaming, e.config.MaxHistory)
	return nil
}

// Stop stops the chat element
func (e *ChatElement) Stop() error {
	if e.cancel != nil {
		e.cancel()
		e.wg.Wait()
		e.cancel = nil
	}

	log.Println("[ChatElement] Stopped")
	return nil
}

// ClearHistory clears the conversation history
func (e *ChatElement) ClearHistory() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.history = make([]openai.ChatCompletionMessageParamUnion, 0)
	log.Println("[ChatElement] History cleared")
}

// GetHistoryLength returns the current number of messages in history
func (e *ChatElement) GetHistoryLength() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.history)
}

// processLoop handles incoming messages
func (e *ChatElement) processLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-e.BaseElement.InChan:
			if !ok {
				return
			}
			if msg.Type == pipeline.MsgTypeData && msg.TextData != nil {
				text := strings.TrimSpace(string(msg.TextData.Data))
				if text == "" {
					continue
				}

				// Process the message
				if err := e.processMessage(ctx, text, msg.SessionID); err != nil {
					log.Printf("[ChatElement] Error processing message: %v", err)
					e.BaseElement.Bus().Publish(pipeline.Event{
						Type:      pipeline.EventError,
						Timestamp: time.Now(),
						Payload:   fmt.Sprintf("Chat error: %v", err),
					})
				}
			} else {
				// Pass through non-text messages
				e.BaseElement.OutChan <- msg
			}
		}
	}
}

// processMessage handles a single user message
func (e *ChatElement) processMessage(ctx context.Context, userText string, sessionID string) error {
	log.Printf("[ChatElement] User: %s", userText)

	// Add user message to history
	e.addToHistory(openai.UserMessage(userText))

	// Publish response start event
	e.BaseElement.Bus().Publish(pipeline.Event{
		Type:      pipeline.EventResponseStart,
		Timestamp: time.Now(),
		Payload:   sessionID,
	})

	var response string
	var err error

	if e.config.Streaming {
		response, err = e.chatStreaming(ctx, sessionID)
	} else {
		response, err = e.chatNonStreaming(ctx, sessionID)
	}

	if err != nil {
		// Publish response end with error
		e.BaseElement.Bus().Publish(pipeline.Event{
			Type:      pipeline.EventResponseEnd,
			Timestamp: time.Now(),
			Payload:   map[string]interface{}{"error": err.Error()},
		})
		return err
	}

	// Add assistant response to history
	e.addToHistory(openai.AssistantMessage(response))

	// Publish response end event
	e.BaseElement.Bus().Publish(pipeline.Event{
		Type:      pipeline.EventResponseEnd,
		Timestamp: time.Now(),
		Payload:   map[string]interface{}{"text": response},
	})

	log.Printf("[ChatElement] Assistant: %s", truncateForLog(response, 100))
	return nil
}

// chatStreaming performs streaming chat completion
func (e *ChatElement) chatStreaming(ctx context.Context, sessionID string) (string, error) {
	messages := e.buildMessages()

	params := openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    shared.ChatModel(e.config.Model),
	}
	if e.config.MaxTokens > 0 {
		params.MaxTokens = openai.Int(int64(e.config.MaxTokens))
	}

	stream := e.client.Chat.Completions.NewStreaming(ctx, params)

	var builder strings.Builder
	var sentenceBuffer strings.Builder

	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta.Content
		if delta == "" {
			continue
		}

		builder.WriteString(delta)
		sentenceBuffer.WriteString(delta)

		// Check if we have a complete sentence to send to TTS
		// Send on sentence boundaries for natural speech
		sentence := sentenceBuffer.String()
		if shouldFlushSentence(sentence) {
			e.sendToTTS(sentence, sessionID, false)
			sentenceBuffer.Reset()

			// Publish partial result event
			e.BaseElement.Bus().Publish(pipeline.Event{
				Type:      pipeline.EventTextDelta,
				Timestamp: time.Now(),
				Payload:   sentence,
			})
		}
	}

	if err := stream.Err(); err != nil {
		return "", fmt.Errorf("streaming error: %w", err)
	}

	// Send remaining text
	remaining := sentenceBuffer.String()
	if remaining != "" {
		e.sendToTTS(remaining, sessionID, true)
	}

	return builder.String(), nil
}

// chatNonStreaming performs non-streaming chat completion
func (e *ChatElement) chatNonStreaming(ctx context.Context, sessionID string) (string, error) {
	messages := e.buildMessages()

	params := openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    shared.ChatModel(e.config.Model),
	}
	if e.config.MaxTokens > 0 {
		params.MaxTokens = openai.Int(int64(e.config.MaxTokens))
	}

	completion, err := e.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("completion error: %w", err)
	}

	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("no response from model")
	}

	response := completion.Choices[0].Message.Content

	// Send complete response to TTS
	e.sendToTTS(response, sessionID, true)

	return response, nil
}

// buildMessages builds the message array for API call
func (e *ChatElement) buildMessages() []openai.ChatCompletionMessageParamUnion {
	e.mu.RLock()
	defer e.mu.RUnlock()

	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(e.history)+1)

	// Add system message
	messages = append(messages, openai.SystemMessage(e.config.SystemPrompt))

	// Add history
	messages = append(messages, e.history...)

	return messages
}

// addToHistory adds a message to history with limit enforcement
func (e *ChatElement) addToHistory(msg openai.ChatCompletionMessageParamUnion) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.history = append(e.history, msg)

	// Enforce history limit (keep pairs of user/assistant messages)
	if e.config.MaxHistory > 0 && len(e.history) > e.config.MaxHistory {
		// Remove oldest messages, keeping pairs
		excess := len(e.history) - e.config.MaxHistory
		if excess%2 != 0 {
			excess++ // Keep pairs
		}
		e.history = e.history[excess:]
	}
}

// sendToTTS sends text to the TTS element
func (e *ChatElement) sendToTTS(text string, sessionID string, isFinal bool) {
	if strings.TrimSpace(text) == "" {
		return
	}

	textType := "partial"
	if isFinal {
		textType = "final"
	}

	msg := &pipeline.PipelineMessage{
		Type:      pipeline.MsgTypeData,
		SessionID: sessionID,
		Timestamp: time.Now(),
		TextData: &pipeline.TextData{
			Data:      []byte(text),
			TextType:  textType,
			Timestamp: time.Now(),
		},
	}
	e.BaseElement.OutChan <- msg
}

// shouldFlushSentence checks if the buffer contains a complete sentence
func shouldFlushSentence(text string) bool {
	// Flush on sentence-ending punctuation
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}

	// Use DecodeLastRuneInString to correctly handle multi-byte UTF-8 characters
	// like Chinese punctuation (。！？etc.)
	lastRune, _ := utf8.DecodeLastRuneInString(trimmed)
	if lastRune == utf8.RuneError {
		return false
	}

	sentenceEnders := ".!?;:。！？；："

	return strings.ContainsRune(sentenceEnders, lastRune)
}

// truncateForLog truncates text for logging
func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
