package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

func main() {
	text := flag.String("text", "你好，世界！", "Text to translate")
	sourceLang := flag.String("source", "auto", "Source language code (e.g. auto, zh, en)")
	targetLang := flag.String("target", "en", "Target language code (e.g. en, zh)")
	provider := flag.String("provider", "openai", "Translation provider: openai or gemini")
	model := flag.String("model", "", "Model to use (optional)")
	streaming := flag.Bool("streaming", true, "Enable streaming translation")
	timeout := flag.Duration("timeout", 45*time.Second, "Overall timeout for the translation")
	flag.Parse()

	_ = godotenv.Overload(".env")

	apiKey := os.Getenv("OPENAI_API_KEY")
	if *provider == "gemini" {
		apiKey = os.Getenv("GOOGLE_API_KEY")
	}

	if apiKey == "" {
		log.Fatalf("API key is required for provider %s", *provider)
	}

	cfg := elements.TranslateConfig{
		Provider:   *provider,
		APIKey:     apiKey,
		SourceLang: *sourceLang,
		TargetLang: *targetLang,
		Model:      *model,
		Streaming:  *streaming,
	}

	translateElement, err := elements.NewTranslateElement(cfg)
	if err != nil {
		log.Fatalf("failed to create translate element: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	bus := pipeline.NewEventBus()
	if err := bus.Start(ctx); err != nil {
		log.Fatalf("failed to start event bus: %v", err)
	}
	defer bus.Stop()

	translateElement.SetBus(bus)

	if err := translateElement.Start(ctx); err != nil {
		log.Fatalf("failed to start translate element: %v", err)
	}
	defer translateElement.Stop()

	partialCh := make(chan pipeline.Event, 16)
	finalCh := make(chan pipeline.Event, 4)
	bus.Subscribe(pipeline.EventPartialResult, partialCh)
	bus.Subscribe(pipeline.EventFinalResult, finalCh)

	go func() {
		for {
			select {
			case evt := <-partialCh:
				fmt.Printf("[partial] %s\n", evt.Payload)
			case evt := <-finalCh:
				fmt.Printf("[final-event] %s\n", evt.Payload)
			case <-ctx.Done():
				return
			}
		}
	}()

	msg := &pipeline.PipelineMessage{
		Type: pipeline.MsgTypeData,
		TextData: &pipeline.TextData{
			Data:      []byte(*text),
			TextType:  "final",
			Timestamp: time.Now(),
		},
	}

	select {
	case translateElement.In() <- msg:
	case <-ctx.Done():
		log.Fatalf("context canceled before sending message: %v", ctx.Err())
	}

	var translation string
	select {
	case out := <-translateElement.Out():
		if out.TextData != nil {
			translation = string(out.TextData.Data)
		}
	case <-ctx.Done():
		log.Fatalf("translation timed out: %v", ctx.Err())
	}

	fmt.Println("======== Translation Result ========")
	fmt.Printf("Provider : %s\n", cfg.Provider)
	fmt.Printf("Model    : %s\n", cfg.Model)
	fmt.Printf("Source   : %s\n", cfg.SourceLang)
	fmt.Printf("Target   : %s\n", cfg.TargetLang)
	fmt.Printf("Input    : %s\n", *text)
	fmt.Printf("Output   : %s\n", translation)
	fmt.Println("====================================")
}
