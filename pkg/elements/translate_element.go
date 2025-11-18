package elements

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"google.golang.org/genai"
)

// Make sure TranslateElement implements pipeline.Element
var _ pipeline.Element = (*TranslateElement)(nil)

// TranslateConfig holds configuration for the translate element
type TranslateConfig struct {
	Provider     string // "openai" or "gemini"
	APIKey       string
	SourceLang   string // "auto", "zh", "en", "ja", etc.
	TargetLang   string // "en", "zh", "ja", etc.
	Model        string // "gpt-4o-mini", "gemini-2.0-flash-exp"
	SystemPrompt string // Custom translation prompt
	Streaming    bool   // Enable streaming translation
}

// TranslateElement translates text from one language to another
type TranslateElement struct {
	*pipeline.BaseElement

	config        TranslateConfig
	openaiClient  *openai.Client
	geminiClient  *genai.Client
	geminiSession *genai.Session

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewTranslateElement creates a new translate element
func NewTranslateElement(config TranslateConfig) (*TranslateElement, error) {
	if config.Provider == "" {
		config.Provider = "openai"
	}
	if config.Model == "" {
		if config.Provider == "openai" {
			config.Model = "gpt-4o-mini"
		} else if config.Provider == "gemini" {
			config.Model = "gemini-2.0-flash-exp"
		}
	}
	if config.SourceLang == "" {
		config.SourceLang = "auto"
	}
	if config.TargetLang == "" {
		return nil, fmt.Errorf("target language is required")
	}
	if config.SystemPrompt == "" {
		config.SystemPrompt = buildDefaultPrompt(config.SourceLang, config.TargetLang)
	}

	return &TranslateElement{
		BaseElement: pipeline.NewBaseElement("translate-element", 100),
		config:      config,
	}, nil
}

// buildDefaultPrompt creates a default translation prompt
func buildDefaultPrompt(sourceLang, targetLang string) string {
	sourceLangName := getLanguageName(sourceLang)
	targetLangName := getLanguageName(targetLang)

	if sourceLang == "auto" {
		return fmt.Sprintf("You are a professional translator. Translate the following text to %s. Only output the translation, no explanations.", targetLangName)
	}
	return fmt.Sprintf("You are a professional translator. Translate the following text from %s to %s. Only output the translation, no explanations.", sourceLangName, targetLangName)
}

// getLanguageName converts language code to full name
func getLanguageName(code string) string {
	names := map[string]string{
		"auto": "auto-detect",
		"zh":   "Chinese",
		"en":   "English",
		"ja":   "Japanese",
		"ko":   "Korean",
		"es":   "Spanish",
		"fr":   "French",
		"de":   "German",
		"ru":   "Russian",
		"ar":   "Arabic",
	}
	if name, ok := names[code]; ok {
		return name
	}
	return code
}

// Start initializes the translate element
func (e *TranslateElement) Start(ctx context.Context) error {
	if e.config.APIKey == "" {
		return fmt.Errorf("API key is required")
	}

	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	// Initialize the appropriate client
	var err error
	if e.config.Provider == "openai" {
		client := openai.NewClient(option.WithAPIKey(e.config.APIKey))
		e.openaiClient = &client
	} else if e.config.Provider == "gemini" {
		e.geminiClient, err = genai.NewClient(ctx, &genai.ClientConfig{
			APIKey:  e.config.APIKey,
			Backend: genai.BackendGoogleAI,
		})
		if err != nil {
			return fmt.Errorf("failed to create Gemini client: %v", err)
		}
	} else {
		return fmt.Errorf("unsupported provider: %s", e.config.Provider)
	}

	// Start message processing goroutine
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-e.BaseElement.InChan:
				if msg.Type == pipeline.MsgTypeData && msg.TextData != nil {
					text := string(msg.TextData.Data)
					if text == "" {
						continue
					}

					// Translate the text
					translated, err := e.translate(ctx, text)
					if err != nil {
						log.Printf("Translation error: %v", err)
						e.BaseElement.Bus().Publish(pipeline.Event{
							Type:      pipeline.EventError,
							Timestamp: time.Now(),
							Payload:   fmt.Sprintf("Translation error: %v", err),
						})
						continue
					}

					if translated != "" {
						// Send translated text to output
						outMsg := &pipeline.PipelineMessage{
							Type: pipeline.MsgTypeData,
							TextData: &pipeline.TextData{
								Data:      []byte(translated),
								TextType:  msg.TextData.TextType, // Preserve text type (partial/final)
								Timestamp: time.Now(),
							},
						}
						e.BaseElement.OutChan <- outMsg

						// Publish translation event
						e.BaseElement.Bus().Publish(pipeline.Event{
							Type:      pipeline.EventFinalResult,
							Timestamp: time.Now(),
							Payload:   translated,
						})
					}
				} else {
					// Pass through non-text messages
					e.BaseElement.OutChan <- msg
				}
			}
		}
	}()

	log.Printf("TranslateElement started (provider: %s, model: %s, %s -> %s)",
		e.config.Provider, e.config.Model, e.config.SourceLang, e.config.TargetLang)
	return nil
}

// translate performs the actual translation
func (e *TranslateElement) translate(ctx context.Context, text string) (string, error) {
	if e.config.Provider == "openai" {
		return e.translateWithOpenAI(ctx, text)
	} else if e.config.Provider == "gemini" {
		return e.translateWithGemini(ctx, text)
	}
	return "", fmt.Errorf("unsupported provider: %s", e.config.Provider)
}

// translateWithOpenAI uses OpenAI API for translation
func (e *TranslateElement) translateWithOpenAI(ctx context.Context, text string) (string, error) {
	if e.config.Streaming {
		return e.translateWithOpenAIStreaming(ctx, text)
	}

	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(e.config.SystemPrompt),
			openai.UserMessage(text),
		},
		Model: shared.ChatModel(e.config.Model),
	}

	completion, err := e.openaiClient.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", err
	}

	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("no response from OpenAI")
	}

	return completion.Choices[0].Message.Content, nil
}

// translateWithOpenAIStreaming uses OpenAI streaming API for lower latency
func (e *TranslateElement) translateWithOpenAIStreaming(ctx context.Context, text string) (string, error) {
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(e.config.SystemPrompt),
			openai.UserMessage(text),
		},
		Model: shared.ChatModel(e.config.Model),
	}

	stream := e.openaiClient.Chat.Completions.NewStreaming(ctx, params)

	var builder strings.Builder
	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) == 0 {
			continue
		}
		if delta := chunk.Choices[0].Delta.Content; delta != "" {
			builder.WriteString(delta)
			e.BaseElement.Bus().Publish(pipeline.Event{
				Type:      pipeline.EventPartialResult,
				Timestamp: time.Now(),
				Payload:   builder.String(),
			})
		}
	}

	if err := stream.Err(); err != nil {
		return "", err
	}

	return builder.String(), nil
}

// translateWithGemini uses Gemini API for translation
func (e *TranslateElement) translateWithGemini(ctx context.Context, text string) (string, error) {
	if e.config.Streaming {
		return e.translateWithGeminiStreaming(ctx, text)
	}

	resp, err := e.geminiClient.Models.GenerateContent(
		ctx,
		e.config.Model,
		genai.Text(text),
		e.geminiRequestConfig(),
	)
	if err != nil {
		return "", err
	}

	chunk := collectGeminiText(resp)
	if chunk == "" {
		return "", fmt.Errorf("no response from Gemini")
	}

	return chunk, nil
}

// translateWithGeminiStreaming uses Gemini streaming API
func (e *TranslateElement) translateWithGeminiStreaming(ctx context.Context, text string) (string, error) {
	stream := e.geminiClient.Models.GenerateContentStream(
		ctx,
		e.config.Model,
		genai.Text(text),
		e.geminiRequestConfig(),
	)

	var builder strings.Builder
	for resp, err := range stream {
		if err != nil {
			return "", err
		}

		if chunk := collectGeminiText(resp); chunk != "" {
			builder.WriteString(chunk)
			e.BaseElement.Bus().Publish(pipeline.Event{
				Type:      pipeline.EventPartialResult,
				Timestamp: time.Now(),
				Payload:   builder.String(),
			})
		}
	}

	return builder.String(), nil
}

// Stop stops the translate element
func (e *TranslateElement) Stop() error {
	if e.cancel != nil {
		e.cancel()
		e.wg.Wait()
		e.cancel = nil
	}

	e.geminiClient = nil

	log.Println("TranslateElement stopped")
	return nil
}

func (e *TranslateElement) geminiRequestConfig() *genai.GenerateContentConfig {
	if e.config.SystemPrompt == "" {
		return nil
	}
	return &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: e.config.SystemPrompt},
			},
		},
	}
}

func collectGeminiText(resp *genai.GenerateContentResponse) string {
	if resp == nil {
		return ""
	}

	var builder strings.Builder
	for _, cand := range resp.Candidates {
		if cand == nil || cand.Content == nil {
			continue
		}
		for _, part := range cand.Content.Parts {
			if part == nil || part.Text == "" {
				continue
			}
			builder.WriteString(part.Text)
		}
	}

	return builder.String()
}
