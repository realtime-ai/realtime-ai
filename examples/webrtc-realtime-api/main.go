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

	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi"
	"github.com/realtime-ai/realtime-ai/pkg/server"
)

func main() {
	// Get API key from environment
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		log.Println("Warning: GOOGLE_API_KEY not set, using echo mode")
	}

	// Create server configuration
	config := server.DefaultWebRTCRealtimeConfig()
	config.RTCUDPPort = 9000
	config.ICELite = true
	config.DefaultModel = "gemini-2.0-flash"

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
	http.Handle("/", http.FileServer(http.Dir("examples/webrtc-realtime-api")))

	log.Println("WebRTC Realtime API server started")
	log.Println("Open http://localhost:8080 in your browser")
	log.Println("UDP port: 9000")

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}

// createPipeline creates the audio processing pipeline.
// Audio flow: Input (48kHz) -> Resample (16kHz) -> [AI] -> Resample (48kHz) -> Output
func createPipeline(ctx context.Context, session *realtimeapi.Session, apiKey string) (*pipeline.Pipeline, error) {
	p := pipeline.NewPipeline("webrtc-realtime-" + session.ID)

	if apiKey != "" {
		// Full pipeline with Gemini AI
		// Input: WebRTC audio at 48kHz
		// Resample to 16kHz for Gemini
		inputResample := elements.NewAudioResampleElement(48000, 16000, 1, 1)

		// Gemini AI processing
		gemini := elements.NewGeminiElement()

		// Resample output to 48kHz for WebRTC
		// Note: Gemini outputs at 24kHz, we need to resample to 48kHz
		outputResample := elements.NewAudioResampleElement(24000, 48000, 1, 1)

		// Add elements
		p.AddElements([]pipeline.Element{inputResample, gemini, outputResample})

		// Link elements
		p.Link(inputResample, gemini)
		p.Link(gemini, outputResample)

		log.Printf("[Pipeline] Created Gemini pipeline for session %s", session.ID)
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
