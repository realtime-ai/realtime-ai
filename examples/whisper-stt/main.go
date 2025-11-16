package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/pion/webrtc/v4"
	"github.com/realtime-ai/realtime-ai/pkg/connection"
	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/server"
)

type connectionEventHandler struct {
	connection.ConnectionEventHandler

	conn     connection.RTCConnection
	pipeline *pipeline.Pipeline
}

func (c *connectionEventHandler) OnConnectionStateChange(state webrtc.PeerConnectionState) {
	log.Printf("Connection state changed: %v", state)

	if state == webrtc.PeerConnectionStateConnected {
		log.Println("WebRTC connection established")
	} else if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
		log.Println("WebRTC connection ended")
		if c.pipeline != nil {
			c.pipeline.Stop()
		}
	}
}

func (c *connectionEventHandler) OnMessage(msg *pipeline.PipelineMessage) {
	// Push incoming audio to the pipeline
	c.pipeline.Push(msg)
}

func (c *connectionEventHandler) OnError(err error) {
	log.Printf("Connection error: %v", err)
}

func main() {
	// Load environment variables
	godotenv.Load()

	log.Println("=== Whisper STT Example with VAD Integration ===")
	log.Println("This example demonstrates:")
	log.Println("  - Speech-to-Text using OpenAI Whisper")
	log.Println("  - Voice Activity Detection (VAD) using Silero")
	log.Println("  - Real-time audio processing pipeline")
	log.Println()

	// Check for required API key
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// Create WebRTC server configuration
	cfg := &server.ServerConfig{
		RTCUDPPort: 9000,
	}

	// Create WebRTC server
	rtcServer := server.NewRTCServer(cfg)

	// Set up connection handlers
	rtcServer.OnConnectionCreated(func(ctx context.Context, conn connection.RTCConnection) {
		log.Printf("New connection created: %s", conn.GetConnectionID())

		// Create event handler
		eventHandler := &connectionEventHandler{
			conn: conn,
		}
		conn.RegisterEventHandler(eventHandler)

		// Create and configure pipeline
		p := createPipeline(conn)
		eventHandler.pipeline = p

		// Start pipeline
		if err := p.Start(ctx); err != nil {
			log.Printf("Failed to start pipeline: %v", err)
			return
		}

		// Start output handler
		go handlePipelineOutput(conn, p)

		log.Println("Pipeline started successfully")
	})

	rtcServer.OnConnectionError(func(ctx context.Context, conn connection.RTCConnection, err error) {
		log.Printf("Connection error: %v", err)
	})

	// Start HTTP server
	go func() {
		log.Println("Starting HTTP server on :8080")
		log.Println("Open http://localhost:8080 in your browser")
		if err := rtcServer.Start(":8080"); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	log.Println("\nShutting down...")
}

// createPipeline sets up the audio processing pipeline with VAD and Whisper STT
func createPipeline(conn connection.RTCConnection) *pipeline.Pipeline {
	p := pipeline.NewPipeline("whisper-stt-pipeline")

	// 1. Audio Resample Element (ensure 16kHz for VAD and Whisper)
	resampleElement := elements.NewAudioResampleElement(16000, 1)
	p.AddElement(resampleElement)
	log.Println("Added: AudioResampleElement (16kHz, mono)")

	// 2. VAD Element (optional, but recommended for optimization)
	// Set VAD mode based on build tags
	var vadElement pipeline.Element
	vadConfig := elements.SileroVADConfig{
		ModelPath:       "models/silero_vad.onnx",
		Threshold:       0.5,
		MinSilenceDurMs: 300,
		SpeechPadMs:     30,
		Mode:            elements.VADModePassthrough, // Passthrough mode forwards all audio
	}

	// Try to create VAD element (will fail gracefully if not built with vad tag)
	vadElement = elements.NewSileroVADElement(vadConfig)
	if err := vadElement.Init(context.Background()); err != nil {
		log.Printf("VAD not available (build with -tags vad to enable): %v", err)
		log.Println("Continuing without VAD optimization...")
		vadElement = nil
	} else {
		p.AddElement(vadElement)
		log.Println("Added: SileroVADElement (Passthrough mode, emits events)")
	}

	// 3. Whisper STT Element
	whisperConfig := elements.WhisperSTTConfig{
		APIKey:               os.Getenv("OPENAI_API_KEY"),
		Language:             "auto", // Auto-detect language (or specify: "en", "zh", etc.)
		Model:                "whisper-1",
		EnablePartialResults: false,        // Only send final results
		VADEnabled:           vadElement != nil, // Enable VAD integration if VAD is available
		SampleRate:           16000,
		Channels:             1,
		BitsPerSample:        16,
		Prompt:               "", // Optional: provide context to guide recognition
	}

	whisperElement, err := elements.NewWhisperSTTElement(whisperConfig)
	if err != nil {
		log.Fatalf("Failed to create Whisper STT element: %v", err)
	}
	p.AddElement(whisperElement)
	log.Printf("Added: WhisperSTTElement (Language: %s, VAD: %v)",
		whisperConfig.Language, whisperConfig.VADEnabled)

	// Link elements together
	if vadElement != nil {
		// Pipeline: resample -> VAD -> Whisper STT
		p.Link(resampleElement, vadElement)
		p.Link(vadElement, whisperElement)
	} else {
		// Pipeline: resample -> Whisper STT
		p.Link(resampleElement, whisperElement)
	}

	// Subscribe to pipeline events for logging
	subscribeToEvents(p)

	log.Println("Pipeline configured successfully")
	return p
}

// subscribeToEvents subscribes to pipeline events for monitoring
func subscribeToEvents(p *pipeline.Pipeline) {
	bus := p.GetBus()
	if bus == nil {
		return
	}

	// Subscribe to VAD events
	vadEventsChan := make(chan pipeline.Event, 10)
	bus.Subscribe(pipeline.EventVADSpeechStart, vadEventsChan)
	bus.Subscribe(pipeline.EventVADSpeechEnd, vadEventsChan)

	// Subscribe to STT events
	sttEventsChan := make(chan pipeline.Event, 10)
	bus.Subscribe(pipeline.EventPartialResult, sttEventsChan)
	bus.Subscribe(pipeline.EventFinalResult, sttEventsChan)

	// Handle VAD events
	go func() {
		for event := range vadEventsChan {
			switch event.Type {
			case pipeline.EventVADSpeechStart:
				log.Println("🎤 Speech detected - recording...")
			case pipeline.EventVADSpeechEnd:
				log.Println("🔇 Speech ended - processing...")
			}
		}
	}()

	// Handle STT events
	go func() {
		for event := range sttEventsChan {
			if text, ok := event.Payload.(string); ok {
				switch event.Type {
				case pipeline.EventPartialResult:
					log.Printf("📝 [Partial] %s", text)
				case pipeline.EventFinalResult:
					log.Printf("✅ [Final] %s", text)
				}
			}
		}
	}()
}

// handlePipelineOutput processes pipeline output and sends it back to the connection
func handlePipelineOutput(conn connection.RTCConnection, p *pipeline.Pipeline) {
	for {
		msg := p.Pull()
		if msg == nil {
			// Pipeline closed
			break
		}

		// Log text data (transcriptions)
		if msg.Type == pipeline.MsgTypeData && msg.TextData != nil {
			text := string(msg.TextData.Data)
			if text != "" {
				log.Printf("📨 Sending transcription to client: %s", text)
			}
		}

		// Send message back to client (if needed)
		// For STT-only applications, you might send the text data back
		if err := conn.SendMessage(msg); err != nil {
			log.Printf("Failed to send message: %v", err)
			break
		}
	}

	log.Println("Output handler stopped")
}
