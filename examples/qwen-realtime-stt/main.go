package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/realtime-ai/realtime-ai/pkg/connection"
	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/server"
)

type connectionEventHandler struct {
	connection.ConnectionEventHandler

	conn     connection.Connection
	pipeline *pipeline.Pipeline
}

func (c *connectionEventHandler) OnConnectionStateChange(state connection.ConnectionState) {
	log.Printf("Connection state changed: %v", state)

	if state == connection.ConnectionStateConnected {
		log.Println("WebRTC connection established")
	} else if state == connection.ConnectionStateFailed || state == connection.ConnectionStateClosed {
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

	log.Println("=== Qwen Realtime STT Example with VAD Integration ===")
	log.Println("This example demonstrates:")
	log.Println("  - True streaming Speech-to-Text using Alibaba Cloud DashScope Qwen ASR")
	log.Println("  - Real-time partial and final transcription results")
	log.Println("  - Voice Activity Detection (VAD) using Silero")
	log.Println("  - Real-time audio processing pipeline")
	log.Println()
	log.Println("Qwen Realtime ASR uses WebSocket for true streaming,")
	log.Println("similar to OpenAI's Realtime API.")
	log.Println()

	// Check for required API key
	if os.Getenv("DASHSCOPE_API_KEY") == "" {
		log.Fatal("DASHSCOPE_API_KEY environment variable is required")
	}

	// Create WebRTC server configuration
	cfg := &server.ServerConfig{
		RTCUDPPort: 9000,
	}

	// Create WebRTC server
	rtcServer := server.NewRealtimeServer(cfg)

	// Set up connection handlers
	rtcServer.OnConnectionCreated(func(ctx context.Context, conn connection.Connection) {
		log.Printf("New connection created: %s", conn.PeerID())

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

	rtcServer.OnConnectionError(func(ctx context.Context, conn connection.Connection, err error) {
		log.Printf("Connection error: %v", err)
	})

	// Start WebRTC server
	if err := rtcServer.Start(); err != nil {
		log.Fatalf("Failed to start WebRTC server: %v", err)
	}

	// Set up HTTP handlers
	http.HandleFunc("/session", rtcServer.HandleNegotiate)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "examples/qwen-realtime-stt/index.html")
	})

	// Start HTTP server in a goroutine
	go func() {
		log.Println("Starting HTTP server on :8080")
		log.Println("Open http://localhost:8080 in your browser")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatalf("Failed to start HTTP server: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	log.Println("\nShutting down...")
}

// createPipeline sets up the audio processing pipeline with VAD and Qwen Realtime STT
func createPipeline(conn connection.Connection) *pipeline.Pipeline {
	p := pipeline.NewPipeline("qwen-realtime-stt-pipeline")

	// 1. Audio Resample Element (ensure 16kHz for VAD and Qwen)
	// AudioResampleElement(inputRate, outputRate, inputChannels, outputChannels)
	resampleElement := elements.NewAudioResampleElement(48000, 16000, 1, 1)
	p.AddElement(resampleElement)
	log.Println("Added: AudioResampleElement (48kHz → 16kHz, mono)")

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
	vadElem, err := elements.NewSileroVADElement(vadConfig)
	if err != nil {
		log.Printf("VAD not available (build with -tags vad to enable): %v", err)
		log.Println("Continuing without VAD optimization...")
	} else {
		if err := vadElem.Init(context.Background()); err != nil {
			log.Printf("[Pipeline] Warning: Failed to init VAD element: %v", err)
		} else {
			vadElement = vadElem
			p.AddElement(vadElement)
			log.Println("Added: SileroVADElement (Passthrough mode, emits events)")
		}
	}

	// 3. Qwen Realtime STT Element
	// Get language from environment variable or default to Chinese
	language := os.Getenv("QWEN_ASR_LANGUAGE")
	if language == "" {
		language = "zh" // Default to Chinese
	}

	qwenConfig := elements.QwenRealtimeSTTConfig{
		APIKey:               os.Getenv("DASHSCOPE_API_KEY"),
		Language:             language,
		Model:                "qwen3-asr-flash-realtime",
		EnablePartialResults: true,              // Enable real-time partial results
		VADEnabled:           vadElement != nil, // Enable VAD integration if VAD is available
		SampleRate:           16000,
		Channels:             1,
		BitsPerSample:        16,
	}

	qwenElement, err := elements.NewQwenRealtimeSTTElement(qwenConfig)
	if err != nil {
		log.Fatalf("Failed to create Qwen Realtime STT element: %v", err)
	}
	p.AddElement(qwenElement)
	log.Printf("Added: QwenRealtimeSTTElement (Language: %s, VAD: %v, Partial: %v)",
		qwenConfig.Language, qwenConfig.VADEnabled, qwenConfig.EnablePartialResults)

	// Link elements together
	if vadElement != nil {
		// Pipeline: resample -> VAD -> Qwen Realtime STT
		p.Link(resampleElement, vadElement)
		p.Link(vadElement, qwenElement)
	} else {
		// Pipeline: resample -> Qwen Realtime STT
		p.Link(resampleElement, qwenElement)
	}

	// Subscribe to pipeline events for logging
	subscribeToEvents(p)

	log.Println("Pipeline configured successfully")
	return p
}

// subscribeToEvents subscribes to pipeline events for monitoring
func subscribeToEvents(p *pipeline.Pipeline) {
	bus := p.Bus()
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
				log.Println("[VAD] Speech detected - streaming audio...")
			case pipeline.EventVADSpeechEnd:
				log.Println("[VAD] Speech ended - committing for final result...")
			}
		}
	}()

	// Handle STT events
	go func() {
		for event := range sttEventsChan {
			if text, ok := event.Payload.(string); ok {
				switch event.Type {
				case pipeline.EventPartialResult:
					log.Printf("[Partial] %s", text)
				case pipeline.EventFinalResult:
					log.Printf("[Final] %s", text)
				}
			}
		}
	}()
}

// handlePipelineOutput processes pipeline output and sends it back to the connection
func handlePipelineOutput(conn connection.Connection, p *pipeline.Pipeline) {
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
				textType := msg.TextData.TextType
				if textType == "text/final" {
					log.Printf("[Output] Final transcription: %s", text)
				} else {
					log.Printf("[Output] Partial transcription: %s", text)
				}
			}
		}

		// Send message back to client (if needed)
		// For STT-only applications, you might send the text data back
		// Marshal to JSON
		jsonData, err := json.Marshal(msg.TextData)
		if err != nil {
			log.Printf("Failed to marshal event: %v", err)
			return
		}

		// Create a text message with event data
		jsonmsg := &pipeline.PipelineMessage{
			Type: pipeline.MsgTypeData,
			TextData: &pipeline.TextData{
				Data: jsonData,
			},
		}
		conn.SendMessage(jsonmsg)
	}

	log.Println("Output handler stopped")
}
