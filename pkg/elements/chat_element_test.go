package elements

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// Load .env from repo root
	for _, path := range []string{".env", "../.env", "../../.env"} {
		if _, err := os.Stat(path); err == nil {
			godotenv.Load(path)
			break
		}
	}
}

// TestNewChatElement tests element creation
func TestNewChatElement(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := ChatConfig{
			APIKey:       "test-key",
			Model:        "gpt-4o-mini",
			SystemPrompt: "You are helpful.",
			Streaming:    true,
			MaxHistory:   10,
		}

		elem, err := NewChatElement(config)
		require.NoError(t, err)
		assert.NotNil(t, elem)
		assert.Equal(t, "gpt-4o-mini", elem.config.Model)
		assert.Equal(t, 10, elem.config.MaxHistory)
	})

	t.Run("missing API key", func(t *testing.T) {
		config := ChatConfig{
			Model: "gpt-4o-mini",
		}

		elem, err := NewChatElement(config)
		assert.Error(t, err)
		assert.Nil(t, elem)
		assert.Contains(t, err.Error(), "API key is required")
	})

	t.Run("default values", func(t *testing.T) {
		config := ChatConfig{
			APIKey: "test-key",
		}

		elem, err := NewChatElement(config)
		require.NoError(t, err)
		assert.Equal(t, "gpt-4o-mini", elem.config.Model)
		assert.Equal(t, 20, elem.config.MaxHistory)
		assert.NotEmpty(t, elem.config.SystemPrompt)
	})
}

// TestChatElementHistory tests history management
func TestChatElementHistory(t *testing.T) {
	config := ChatConfig{
		APIKey:     "test-key",
		MaxHistory: 4, // Small limit for testing
	}

	elem, err := NewChatElement(config)
	require.NoError(t, err)

	// Initial state
	assert.Equal(t, 0, elem.GetHistoryLength())

	// Test clear history
	elem.ClearHistory()
	assert.Equal(t, 0, elem.GetHistoryLength())
}

// TestChatElementShouldFlushSentence tests sentence detection
func TestChatElementShouldFlushSentence(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		{"Hello", false},
		{"Hello.", true},
		{"Hello!", true},
		{"Hello?", true},
		{"Hello;", true},
		{"Hello:", true},
		{"", false},
		{"  ", false},
		{"Hello world", false},
		{"Hello world.", true},
		{"你好。", true},
		{"你好？", true},
	}

	for _, tt := range tests {
		result := shouldFlushSentence(tt.text)
		assert.Equal(t, tt.expected, result, "text: %q", tt.text)
	}
}

// TestChatElementWithRealAPI tests with real OpenAI API (skipped if no key)
func TestChatElementWithRealAPI(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	config := ChatConfig{
		APIKey:       apiKey,
		Model:        "gpt-4o-mini",
		SystemPrompt: "Reply with exactly one word: 'OK'",
		Streaming:    false,
		MaxHistory:   5,
	}

	chat, err := NewChatElement(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create pipeline
	p := pipeline.NewPipeline("test-chat-api")
	p.AddElement(chat)

	err = p.Start(ctx)
	require.NoError(t, err)
	defer p.Stop()

	// Send test message
	msg := &pipeline.PipelineMessage{
		Type:      pipeline.MsgTypeData,
		SessionID: "test-session",
		Timestamp: time.Now(),
		TextData: &pipeline.TextData{
			Data:      []byte("Hello"),
			TextType:  "final",
			Timestamp: time.Now(),
		},
	}
	p.Push(msg)

	// Wait for response
	select {
	case outMsg := <-chat.Out():
		require.NotNil(t, outMsg)
		require.NotNil(t, outMsg.TextData)
		response := string(outMsg.TextData.Data)
		assert.NotEmpty(t, response)
		t.Logf("Response: %s", response)
	case <-time.After(25 * time.Second):
		t.Fatal("Timeout waiting for response")
	}

	// Verify history was updated
	assert.Equal(t, 2, chat.GetHistoryLength())
}

// TestChatElementStreamingWithRealAPI tests streaming with real API
func TestChatElementStreamingWithRealAPI(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	config := ChatConfig{
		APIKey:       apiKey,
		Model:        "gpt-4o-mini",
		SystemPrompt: "Reply with exactly 3 sentences.",
		Streaming:    true,
		MaxHistory:   5,
	}

	chat, err := NewChatElement(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p := pipeline.NewPipeline("test-chat-streaming")
	p.AddElement(chat)

	err = p.Start(ctx)
	require.NoError(t, err)
	defer p.Stop()

	// Send test message
	msg := &pipeline.PipelineMessage{
		Type:      pipeline.MsgTypeData,
		SessionID: "test-session",
		Timestamp: time.Now(),
		TextData: &pipeline.TextData{
			Data:      []byte("Tell me about the weather."),
			TextType:  "final",
			Timestamp: time.Now(),
		},
	}
	p.Push(msg)

	// Collect streaming responses
	var responses []string
	timeout := time.After(25 * time.Second)

	for {
		select {
		case outMsg := <-chat.Out():
			if outMsg != nil && outMsg.TextData != nil {
				text := string(outMsg.TextData.Data)
				responses = append(responses, text)
				t.Logf("Chunk: %s", truncateForLog(text, 50))

				if outMsg.TextData.TextType == "final" {
					goto done
				}
			}
		case <-timeout:
			goto done
		}
	}

done:
	assert.NotEmpty(t, responses, "Should receive streaming responses")
	t.Logf("Received %d chunks", len(responses))

	// Combine all text
	var fullResponse strings.Builder
	for _, r := range responses {
		fullResponse.WriteString(r)
	}
	t.Logf("Full response: %s", truncateForLog(fullResponse.String(), 200))
}

