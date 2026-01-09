// WebRTC Realtime API Example
//
// This example demonstrates the hybrid WebRTC + Realtime API architecture:
// - Audio is transmitted via WebRTC RTP tracks (Opus 48kHz)
// - Signaling uses WebRTC DataChannel with Realtime API JSON events
//
// Usage:
//
//	go run examples/webrtc-realtime-api/main.go
//	open http://localhost:8080
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi"
	"github.com/realtime-ai/realtime-ai/pkg/server"
)

func main() {
	// Load environment variables from .env file
	godotenv.Load()

	// Get API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Println("Warning: OPENAI_API_KEY not set, using echo mode")
	}

	// Create server configuration
	config := server.DefaultWebRTCRealtimeConfig()
	config.RTCUDPPort = 9000
	config.ICELite = false
	config.DefaultModel = "gpt-realtime"

	// Create server
	srv := server.NewWebRTCRealtimeServer(config)

	// Set pipeline factory
	srv.SetPipelineFactory(func(ctx context.Context, session *realtimeapi.Session) (*pipeline.Pipeline, error) {
		return createPipeline(ctx, session, apiKey)
	})

	// Start WebRTC server
	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start WebRTC server: %v", err)
	}

	// HTTP handlers
	http.HandleFunc("/session", srv.HandleNegotiate)
	http.Handle("/", http.FileServer(http.Dir("examples/openai-realtime")))

	log.Println("OpenAI Realtime API server started")
	log.Println("Open http://localhost:8081 in your browser")
	log.Println("UDP port: 9000")

	if err := http.ListenAndServe(":8081", nil); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}

// createPipeline creates the audio processing pipeline.
// Audio flow: Input (48kHz) -> Resample (16kHz) -> [AI] -> Resample (48kHz) -> Output
func createPipeline(ctx context.Context, session *realtimeapi.Session, apiKey string) (*pipeline.Pipeline, error) {
	p := pipeline.NewPipeline("webrtc-realtime-" + session.ID)

	// Enable interrupt manager with hybrid mode for best user experience
	// Hybrid mode: VAD provides fast response, API confirms accuracy
	interruptConfig := pipeline.DefaultInterruptConfig()
	interruptConfig.EnableHybridMode = true
	interruptConfig.MinSpeechForConfirmMs = 300 // Confirm interrupt after 300ms speech
	p.EnableInterruptManager(interruptConfig)

	if apiKey != "" {
		// Full pipeline with OpenAI AI
		// Input: WebRTC audio at 48kHz
		// Resample to 16kHz for OpenAI
		inputResample := elements.NewAudioResampleElement(48000, 16000, 1, 1)

		// OpenAI AI processing
		openai := elements.NewOpenAIRealtimeAPIElement()

		openai.SetProperty("prompt", os.Getenv("OPENAI_PROMPT"))

		audioPacerSinkElement := elements.NewAudioPacerSinkElement()

		// Resample output to 48kHz for WebRTC
		// Note: OpenAI outputs at 24kHz, we need to resample to 48kHz
		outputResample := elements.NewAudioResampleElement(24000, 48000, 1, 1)

		// Add elements
		p.AddElements([]pipeline.Element{inputResample, openai, outputResample, audioPacerSinkElement})

		// Link elements
		p.Link(inputResample, openai)
		p.Link(openai, outputResample)
		p.Link(outputResample, audioPacerSinkElement)

		log.Printf("[Pipeline] Created OpenAI pipeline for session %s with interrupt support", session.ID)
	} else {
		// Echo pipeline for testing without API key
		echo := NewEchoElement()
		p.AddElement(echo)

		log.Printf("[Pipeline] Created Echo pipeline for session %s (no API key)", session.ID)
	}

	return p, nil
}

// EchoElement is a simple element that echoes audio back.
type EchoElement struct {
	*pipeline.BaseElement
}

// NewEchoElement creates a new echo element.
func NewEchoElement() *EchoElement {
	return &EchoElement{
		BaseElement: pipeline.NewBaseElement("echo", 100),
	}
}

// Start starts the echo element.
func (e *EchoElement) Start(ctx context.Context) error {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-e.InChan:
				if !ok {
					return
				}
				// Echo audio back
				if msg.AudioData != nil {
					e.OutChan <- msg
				}
			}
		}
	}()
	return nil
}

// Stop stops the echo element.
func (e *EchoElement) Stop() error {
	return nil
}
