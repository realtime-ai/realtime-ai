package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/tts"
)

func main() {
	// Load environment variables from .env file
	godotenv.Load()

	// Check for API key
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is not set")
	}

	// Create OpenAI TTS provider
	// Use NewOpenAITTSProvider for standard quality
	// Use NewOpenAITTSProviderHD for high definition quality
	provider := tts.NewOpenAITTSProvider(apiKey)

	// Create TTS element with the provider
	ttsElement := elements.NewUniversalTTSElement(provider)

	// Configure voice (optional)
	// Available voices: alloy, echo, fable, onyx, nova, shimmer
	ttsElement.SetVoice("nova")

	// Set language (optional)
	ttsElement.SetLanguage("en-US")

	// Set additional options (optional)
	// Speed: 0.25 to 4.0, default 1.0
	ttsElement.SetOption("speed", 1.0)
	ttsElement.SetOption("format", "pcm") // pcm, opus, mp3, wav

	// Create pipeline
	p := pipeline.NewPipeline("openai-tts-demo")
	p.AddElement(ttsElement)

	// Start the pipeline
	ctx := context.Background()
	if err := ttsElement.Start(ctx); err != nil {
		log.Fatalf("Failed to start TTS element: %v", err)
	}
	defer ttsElement.Stop()

	// Example: Synthesize some text
	texts := []string{
		"Hello! This is a test of OpenAI's text to speech API.",
		"I'm using the Nova voice, which sounds energetic and lively.",
		"The universal TTS element makes it easy to switch between different providers.",
	}

	for i, text := range texts {
		fmt.Printf("\n[%d] Synthesizing: %s\n", i+1, text)

		// Create text message
		msg := &pipeline.PipelineMessage{
			Type: pipeline.MsgTypeData,
			TextData: &pipeline.TextData{
				Data:      []byte(text),
				Timestamp: time.Now(),
			},
		}

		// Send to TTS element
		ttsElement.In() <- msg

		// Receive audio output
		select {
		case audioMsg := <-ttsElement.Out():
			if audioMsg.AudioData != nil {
				fmt.Printf("✓ Received audio: %d bytes, %d Hz, %d channels\n",
					len(audioMsg.AudioData.Data),
					audioMsg.AudioData.SampleRate,
					audioMsg.AudioData.Channels)

				// Here you could save the audio to a file or send it to another element
				// For example, you could link it to an AudioPacerSinkElement for playback
			}
		case <-time.After(10 * time.Second):
			log.Printf("Timeout waiting for audio output")
		}

		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println("\n✓ Demo completed successfully!")

	// Example: List supported voices
	fmt.Println("\nSupported voices:")
	for _, voice := range ttsElement.GetSupportedVoices() {
		fmt.Printf("  - %s\n", voice)
	}
}
