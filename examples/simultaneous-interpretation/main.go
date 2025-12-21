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
	"github.com/realtime-ai/realtime-ai/pkg/tts"
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

	log.Println("=== Real-time Simultaneous Interpretation ===")
	log.Println("This demo demonstrates:")
	log.Println("  - Speech-to-Text using OpenAI Whisper")
	log.Println("  - Real-time translation using GPT/Gemini")
	log.Println("  - Text-to-Speech using OpenAI TTS")
	log.Println("  - Voice Activity Detection (VAD) using Silero")
	log.Println("  - Complete audio-to-audio interpretation")
	log.Println()

	// Check for required API key
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// Get configuration from environment variables
	sourceLang := getEnv("SOURCE_LANG", "zh")                   // Default: Chinese
	targetLang := getEnv("TARGET_LANG", "en")                   // Default: English
	translateProvider := getEnv("TRANSLATE_PROVIDER", "openai") // openai or gemini
	translateModel := getEnv("TRANSLATE_MODEL", "")
	ttsVoice := getEnv("TTS_VOICE", "alloy")                        // OpenAI TTS voice
	ttsSpeed := getEnv("TTS_SPEED", "1.0")                          // Speech speed (0.25-4.0)
	enableSubtitles := getEnv("ENABLE_SUBTITLES", "true") == "true" // Show text subtitles

	log.Printf("Configuration:")
	log.Printf("  Source Language: %s", sourceLang)
	log.Printf("  Target Language: %s", targetLang)
	log.Printf("  Translation Provider: %s", translateProvider)
	if translateModel != "" {
		log.Printf("  Translation Model: %s", translateModel)
	}
	log.Printf("  TTS Voice: %s", ttsVoice)
	log.Printf("  TTS Speed: %s", ttsSpeed)
	log.Printf("  Show Subtitles: %v", enableSubtitles)
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
		p, err := createInterpretationPipeline(
			conn,
			sourceLang,
			targetLang,
			translateProvider,
			translateModel,
			ttsVoice,
			enableSubtitles,
		)
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

// createInterpretationPipeline sets up the complete simultaneous interpretation pipeline
func createInterpretationPipeline(
	conn connection.Connection,
	sourceLang, targetLang string,
	translateProvider, translateModel string,
	ttsVoice string,
	enableSubtitles bool,
) (*pipeline.Pipeline, error) {
	p := pipeline.NewPipeline("simultaneous-interpretation")

	log.Println("Building interpretation pipeline:")

	// ============================================================
	// PART 1: Audio Input Processing (Speech Recognition)
	// ============================================================

	// 1. Audio Resample Element (ensure 16kHz for Whisper)
	// AudioResampleElement(inputRate, outputRate, inputChannels, outputChannels)
	resample16k := elements.NewAudioResampleElement(48000, 16000, 1, 1)
	p.AddElement(resample16k)
	log.Println("  [1] AudioResampleElement (48kHz → 16kHz mono)")

	// 2. VAD Element (optional, for optimization)
	var vadElement pipeline.Element
	vadConfig := elements.SileroVADConfig{
		ModelPath:       "models/silero_vad.onnx",
		Threshold:       0.5,
		MinSilenceDurMs: 300,
		SpeechPadMs:     30,
		Mode:            elements.VADModePassthrough,
	}

	vadElem, err := elements.NewSileroVADElement(vadConfig)
	if err != nil {
		log.Printf("  [2] VAD not available (build with -tags vad to enable): %v", err)
	} else {
		vadElement = vadElem
		p.AddElement(vadElement)
		log.Println("  [2] SileroVADElement (Voice Activity Detection)")
	}

	// 3. Whisper STT Element (Speech Recognition)
	whisperConfig := elements.WhisperSTTConfig{
		APIKey:               os.Getenv("OPENAI_API_KEY"),
		Language:             sourceLang,
		Model:                "whisper-1",
		EnablePartialResults: false, // Only final results for better translation
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
	log.Printf("  [3] WhisperSTTElement (Language: %s)", sourceLang)

	// ============================================================
	// PART 2: Translation
	// ============================================================

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
	log.Printf("  [4] TranslateElement (%s: %s → %s)", translateProvider, sourceLang, targetLang)

	// ============================================================
	// PART 3: Speech Synthesis (Text-to-Speech)
	// ============================================================

	// 5. TTS Element (Text-to-Speech)
	ttsProvider := tts.NewOpenAITTSProvider(os.Getenv("OPENAI_API_KEY"))
	ttsElement := elements.NewUniversalTTSElement(ttsProvider)

	// Set TTS voice
	ttsElement.SetProperty("voice", ttsVoice)

	p.AddElement(ttsElement)
	log.Printf("  [5] UniversalTTSElement (Provider: OpenAI, Voice: %s)", ttsVoice)

	// ============================================================
	// PART 4: Audio Output Processing
	// ============================================================

	// 6. Audio Resample Element (TTS outputs 24kHz, WebRTC needs 48kHz)
	// AudioResampleElement(inputRate, outputRate, inputChannels, outputChannels)
	resample48k := elements.NewAudioResampleElement(24000, 48000, 1, 1)
	p.AddElement(resample48k)
	log.Println("  [6] AudioResampleElement (24kHz → 48kHz)")

	// 7. Opus Encode Element (for WebRTC transmission)
	// NewOpusEncodeElement(bufferSize, sampleRate, channels)
	opusEncode := elements.NewOpusEncodeElement(960, 48000, 1)
	p.AddElement(opusEncode)
	log.Println("  [7] OpusEncodeElement (Audio compression)")

	// ============================================================
	// Link all elements in the pipeline
	// ============================================================

	log.Println("\nLinking pipeline elements:")

	// Input path: Audio → Resample → VAD → Whisper
	if vadElement != nil {
		p.Link(resample16k, vadElement)
		p.Link(vadElement, whisperElement)
		log.Println("  Audio Input → Resample(16kHz) → VAD → Whisper STT")
	} else {
		p.Link(resample16k, whisperElement)
		log.Println("  Audio Input → Resample(16kHz) → Whisper STT")
	}

	// Translation path: Whisper → Translate
	p.Link(whisperElement, translateElement)
	log.Println("  Whisper STT → Translate")

	// Output path: Translate → TTS → Resample → Opus → Audio Output
	p.Link(translateElement, ttsElement)
	p.Link(ttsElement, resample48k)
	p.Link(resample48k, opusEncode)
	log.Println("  Translate → TTS → Resample(48kHz) → Opus Encode → Audio Output")

	// Subscribe to pipeline events for logging and subtitles
	subscribeToEvents(p, conn, enableSubtitles)

	log.Println("\n✓ Pipeline configured successfully")
	return p, nil
}

// subscribeToEvents subscribes to pipeline events and forwards them to the client
func subscribeToEvents(p *pipeline.Pipeline, conn connection.Connection, enableSubtitles bool) {
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
				if enableSubtitles {
					sendEventToClient(conn, "vad", map[string]interface{}{
						"type": "speech_start",
					})
				}
			case pipeline.EventVADSpeechEnd:
				log.Println("🔇 Speech ended - processing...")
				if enableSubtitles {
					sendEventToClient(conn, "vad", map[string]interface{}{
						"type": "speech_end",
					})
				}
			}
		}
	}()

	// Handle STT events (original transcription before translation)
	go func() {
		for event := range sttEventsChan {
			if text, ok := event.Payload.(string); ok {
				switch event.Type {
				case pipeline.EventPartialResult:
					log.Printf("📝 [Original] %s", text)
					if enableSubtitles {
						sendEventToClient(conn, "transcription", map[string]interface{}{
							"type": "partial",
							"text": text,
						})
					}
				case pipeline.EventFinalResult:
					log.Printf("✅ [Original] %s", text)
					if enableSubtitles {
						sendEventToClient(conn, "transcription", map[string]interface{}{
							"type": "final",
							"text": text,
						})
					}
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

		// Log different message types
		if msg.Type == pipeline.MsgTypeData {
			if msg.TextData != nil {
				text := string(msg.TextData.Data)
				if text != "" && msg.TextData.TextType != "application/json" {
					log.Printf("🌐 [Translation] %s", text)
				}
			} else if msg.AudioData != nil {
				// Audio data (TTS output)
				log.Printf("🔊 [Audio] Sending %d bytes of interpreted audio", len(msg.AudioData.Data))
			}
		}

		// Send message back to client (audio or text)
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
