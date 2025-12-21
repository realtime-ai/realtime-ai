package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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

	log.Println("=== Real-time Transcription + Translation Demo ===")
	log.Println("This demo demonstrates:")
	log.Println("  - Speech-to-Text using OpenAI Whisper")
	log.Println("  - Real-time translation using GPT/Gemini")
	log.Println("  - Voice Activity Detection (VAD) using Silero")
	log.Println("  - Live bilingual subtitles")
	log.Println()

	// Check for required API key
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// Get configuration from environment variables
	sourceLang := getEnv("SOURCE_LANG", "zh")     // Default: Chinese
	targetLang := getEnv("TARGET_LANG", "en")     // Default: English
	translateProvider := getEnv("TRANSLATE_PROVIDER", "openai") // openai or gemini
	translateModel := getEnv("TRANSLATE_MODEL", "")

	log.Printf("Configuration:")
	log.Printf("  Source Language: %s", sourceLang)
	log.Printf("  Target Language: %s", targetLang)
	log.Printf("  Translation Provider: %s", translateProvider)
	if translateModel != "" {
		log.Printf("  Translation Model: %s", translateModel)
	}
	log.Println()

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
		p, err := createPipeline(conn, sourceLang, targetLang, translateProvider, translateModel)
		if err != nil {
			log.Printf("Failed to create pipeline: %v", err)
			return
		}
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

	// Serve static files (HTML, CSS, JS)
	exePath, err := os.Executable()
	if err != nil {
		log.Printf("Warning: Could not determine executable path: %v", err)
		exePath = "."
	}
	exeDir := filepath.Dir(exePath)
	staticDir := filepath.Join(exeDir, "static")

	// If running with 'go run', use the source directory
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		// Try current directory
		if _, err := os.Stat("static"); err == nil {
			staticDir = "static"
		}
	}

	http.Handle("/", http.FileServer(http.Dir(staticDir)))
	log.Printf("Serving static files from: %s", staticDir)

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

// createPipeline sets up the audio processing pipeline
func createPipeline(conn connection.Connection, sourceLang, targetLang, translateProvider, translateModel string) (*pipeline.Pipeline, error) {
	p := pipeline.NewPipeline("translation-pipeline")

	// 1. Audio Resample Element (ensure 16kHz for Whisper)
	// AudioResampleElement(inputRate, outputRate, inputChannels, outputChannels)
	resampleElement := elements.NewAudioResampleElement(48000, 16000, 1, 1)
	p.AddElement(resampleElement)
	log.Println("Added: AudioResampleElement (48kHz → 16kHz, mono)")

	// 2. VAD Element (optional, for optimization)
	var vadElement pipeline.Element
	vadConfig := elements.SileroVADConfig{
		ModelPath:       "models/silero_vad.onnx",
		Threshold:       0.5,
		MinSilenceDurMs: 300,
		SpeechPadMs:     30,
		Mode:            elements.VADModePassthrough, // Passthrough mode
	}

	vadElem, err := elements.NewSileroVADElement(vadConfig)
	if err != nil {
		log.Printf("VAD not available (build with -tags vad to enable): %v", err)
		log.Println("Continuing without VAD optimization...")
	} else {
		vadElement = vadElem
		p.AddElement(vadElement)
		log.Println("Added: SileroVADElement (Passthrough mode)")
	}

	// 3. Whisper STT Element
	whisperConfig := elements.WhisperSTTConfig{
		APIKey:               os.Getenv("OPENAI_API_KEY"),
		Language:             sourceLang,
		Model:                "whisper-1",
		EnablePartialResults: false,
		VADEnabled:           vadElement != nil,
		SampleRate:           16000,
		Channels:             1,
		BitsPerSample:        16,
		Prompt:               "",
	}

	whisperElement, err := elements.NewWhisperSTTElement(whisperConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Whisper STT element: %v", err)
	}
	p.AddElement(whisperElement)
	log.Printf("Added: WhisperSTTElement (Language: %s, VAD: %v)", whisperConfig.Language, whisperConfig.VADEnabled)

	// 4. Translate Element
	translateAPIKey := os.Getenv("OPENAI_API_KEY")
	if translateProvider == "gemini" {
		translateAPIKey = os.Getenv("GOOGLE_API_KEY")
		if translateAPIKey == "" {
			return nil, fmt.Errorf("GOOGLE_API_KEY is required for Gemini translation")
		}
	}

	translateConfig := elements.TranslateConfig{
		Provider:   translateProvider,
		APIKey:     translateAPIKey,
		SourceLang: sourceLang,
		TargetLang: targetLang,
		Model:      translateModel,
		Streaming:  false, // Set to true for lower latency
	}

	translateElement, err := elements.NewTranslateElement(translateConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Translate element: %v", err)
	}
	p.AddElement(translateElement)
	log.Printf("Added: TranslateElement (%s: %s -> %s)", translateProvider, sourceLang, targetLang)

	// Link elements together
	if vadElement != nil {
		// Pipeline: resample -> VAD -> Whisper STT -> Translate
		p.Link(resampleElement, vadElement)
		p.Link(vadElement, whisperElement)
		p.Link(whisperElement, translateElement)
	} else {
		// Pipeline: resample -> Whisper STT -> Translate
		p.Link(resampleElement, whisperElement)
		p.Link(whisperElement, translateElement)
	}

	// Subscribe to pipeline events for logging
	subscribeToEvents(p, conn)

	log.Println("Pipeline configured successfully")
	return p, nil
}

// subscribeToEvents subscribes to pipeline events and forwards them to the client
func subscribeToEvents(p *pipeline.Pipeline, conn connection.Connection) {
	bus := p.Bus()
	if bus == nil {
		return
	}

	// Subscribe to VAD events
	vadEventsChan := make(chan pipeline.Event, 10)
	bus.Subscribe(pipeline.EventVADSpeechStart, vadEventsChan)
	bus.Subscribe(pipeline.EventVADSpeechEnd, vadEventsChan)

	// Subscribe to STT events (original transcription)
	sttEventsChan := make(chan pipeline.Event, 10)
	bus.Subscribe(pipeline.EventPartialResult, sttEventsChan)
	bus.Subscribe(pipeline.EventFinalResult, sttEventsChan)

	// Handle VAD events
	go func() {
		for event := range vadEventsChan {
			switch event.Type {
			case pipeline.EventVADSpeechStart:
				log.Println("🎤 Speech detected - recording...")
				// Send event to client
				sendEventToClient(conn, "vad", map[string]interface{}{
					"type": "speech_start",
				})
			case pipeline.EventVADSpeechEnd:
				log.Println("🔇 Speech ended - processing...")
				sendEventToClient(conn, "vad", map[string]interface{}{
					"type": "speech_end",
				})
			}
		}
	}()

	// Handle STT events (this captures the original transcription before translation)
	go func() {
		for event := range sttEventsChan {
			if text, ok := event.Payload.(string); ok {
				switch event.Type {
				case pipeline.EventPartialResult:
					log.Printf("📝 [Original] %s", text)
					sendEventToClient(conn, "transcription", map[string]interface{}{
						"type": "partial",
						"text": text,
					})
				case pipeline.EventFinalResult:
					log.Printf("✅ [Original] %s", text)
					sendEventToClient(conn, "transcription", map[string]interface{}{
						"type": "final",
						"text": text,
					})
				}
			}
		}
	}()
}

// sendEventToClient sends an event to the client
func sendEventToClient(conn connection.Connection, eventType string, data map[string]interface{}) {
	// Create event structure
	event := map[string]interface{}{
		"event": eventType,
		"data":  data,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to marshal event: %v", err)
		return
	}

	// Create a text message with event data
	msg := &pipeline.PipelineMessage{
		Type: pipeline.MsgTypeData,
		TextData: &pipeline.TextData{
			Data:     jsonData,
			TextType: "application/json",
		},
	}
	conn.SendMessage(msg)
}

// handlePipelineOutput processes pipeline output and sends it back to the client
func handlePipelineOutput(conn connection.Connection, p *pipeline.Pipeline) {
	for {
		msg := p.Pull()
		if msg == nil {
			// Pipeline closed
			break
		}

		// Log text data (translations)
		if msg.Type == pipeline.MsgTypeData && msg.TextData != nil {
			text := string(msg.TextData.Data)
			if text != "" {
				log.Printf("🌐 [Translation] %s", text)
			}
		}

		// Send message back to client
		conn.SendMessage(msg)
	}

	log.Println("Output handler stopped")
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
