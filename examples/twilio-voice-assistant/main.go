//go:build vad

// Twilio Voice Assistant Example
//
// This example demonstrates a complete phone customer service application using:
//   - Twilio Media Streams for phone audio
//   - Silero VAD for voice activity detection
//   - ElevenLabs Realtime STT (~150ms latency)
//   - OpenAI GPT-4 for conversation
//   - ElevenLabs WebSocket TTS for speech synthesis
//
// Architecture:
//
//   Twilio Phone Call
//          ↓
//   TwiML <Connect><Stream>
//          ↓
//   ┌──────────────────────────────────────────────────────────────┐
//   │              Twilio WebSocket Server                         │
//   │  (receives μ-law 8kHz → sends μ-law 8kHz)                    │
//   └──────────────────────────────────────────────────────────────┘
//          ↓                                      ↑
//       mulaw→PCM                              PCM→mulaw
//       8kHz→16kHz                            16kHz→8kHz
//          ↓                                      ↑
//   ┌──────────────────────────────────────────────────────────────┐
//   │                     Pipeline                                  │
//   │                                                              │
//   │  Audio → VAD → STT → GPT → TTS → Output                     │
//   │  (16kHz)       (ElevenLabs)  (ElevenLabs)                    │
//   └──────────────────────────────────────────────────────────────┘
//
// Environment Variables:
//   - OPENAI_API_KEY: OpenAI API key for GPT-4
//   - ELEVENLABS_API_KEY: ElevenLabs API key for STT and TTS
//   - TWILIO_STREAM_URL: Public WebSocket URL for Twilio (e.g., wss://your-domain.com/media)
//   - PORT: HTTP server port (default: 8080)
//
// Usage:
//   1. Set environment variables
//   2. Run: go run main.go
//   3. Configure Twilio phone number webhook to http://your-server/twiml
//   4. Call the phone number

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/connection"
	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/server"
	"github.com/realtime-ai/realtime-ai/pkg/tts"
)

// Config holds the application configuration.
type Config struct {
	// Server
	Port           string
	TwilioStreamURL string

	// OpenAI
	OpenAIAPIKey string

	// ElevenLabs
	ElevenLabsAPIKey string
	ElevenLabsVoice  string

	// VAD
	VADModelPath string
	VADEnabled   bool

	// Assistant
	SystemPrompt string
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Println("=== Twilio Voice Assistant ===")

	// Load configuration
	config := loadConfig()
	validateConfig(config)

	// Create server
	twilioServer := server.NewTwilioMediaServer(server.TwilioServerConfig{
		Address:       ":" + config.Port,
		StreamURL:     config.TwilioStreamURL,
		WebSocketPath: "/media",
		TwiMLPath:     "/twiml",
	}, &voiceAssistantFactory{config: config})

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := twilioServer.Start(ctx); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	log.Printf("Server started on port %s", config.Port)
	log.Printf("Configure Twilio webhook URL to: http://your-server:%s/twiml", config.Port)
	log.Printf("Twilio will connect to: %s", config.TwilioStreamURL)

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	twilioServer.Stop()
	log.Println("Goodbye!")
}

func loadConfig() *Config {
	config := &Config{
		Port:            getEnv("PORT", "8080"),
		TwilioStreamURL: os.Getenv("TWILIO_STREAM_URL"),
		OpenAIAPIKey:    os.Getenv("OPENAI_API_KEY"),
		ElevenLabsAPIKey: os.Getenv("ELEVENLABS_API_KEY"),
		ElevenLabsVoice:  getEnv("ELEVENLABS_VOICE_ID", "21m00Tcm4TlvDq8ikWAM"), // Rachel
		VADModelPath:     getEnv("VAD_MODEL_PATH", "models/silero_vad.onnx"),
		VADEnabled:       getEnv("VAD_ENABLED", "true") == "true",
		SystemPrompt: getEnv("SYSTEM_PROMPT", `You are a helpful AI phone assistant for customer service.

Guidelines:
- Be concise and natural in your responses - this is a phone call
- Keep responses short (1-2 sentences when possible)
- Be friendly and professional
- If you don't understand, ask for clarification
- Avoid technical jargon unless the customer uses it first
- Say "goodbye" or similar when the customer wants to end the call`),
	}
	return config
}

func validateConfig(config *Config) {
	var missing []string

	if config.TwilioStreamURL == "" {
		missing = append(missing, "TWILIO_STREAM_URL")
	}
	if config.OpenAIAPIKey == "" {
		missing = append(missing, "OPENAI_API_KEY")
	}
	if config.ElevenLabsAPIKey == "" {
		missing = append(missing, "ELEVENLABS_API_KEY")
	}

	if len(missing) > 0 {
		log.Fatalf("Missing required environment variables: %s", strings.Join(missing, ", "))
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// voiceAssistantFactory creates pipelines for voice assistant.
type voiceAssistantFactory struct {
	config *Config
}

func (f *voiceAssistantFactory) CreatePipeline(ctx context.Context, conn *connection.TwilioConnection) (*pipeline.Pipeline, error) {
	log.Printf("[Factory] Creating voice assistant pipeline for call %s", conn.CallSid())

	// Create pipeline
	p := pipeline.NewPipeline("voice-assistant-" + conn.CallSid())

	// Create elements
	var elems []pipeline.Element
	var prevElem pipeline.Element

	// 1. VAD Element (optional but recommended)
	if f.config.VADEnabled {
		vadElem, err := elements.NewSileroVADElement(elements.SileroVADConfig{
			ModelPath:       f.config.VADModelPath,
			Threshold:       0.5,
			MinSilenceDurMs: 500, // 500ms silence to trigger speech end
			SpeechPadMs:     100,
			PreRollMs:       300, // 300ms pre-roll for better STT
			Mode:            elements.VADModePassthrough,
		})
		if err != nil {
			log.Printf("[Factory] VAD not available: %v", err)
		} else {
			if err := vadElem.Init(ctx); err != nil {
				log.Printf("[Factory] VAD init failed: %v", err)
			} else {
				p.AddElement(vadElem)
				elems = append(elems, vadElem)
				prevElem = vadElem
				log.Printf("[Factory] VAD element added")
			}
		}
	}

	// 2. ElevenLabs Realtime STT Element
	sttElem, err := elements.NewElevenLabsRealtimeSTTElement(elements.ElevenLabsRealtimeSTTConfig{
		APIKey:               f.config.ElevenLabsAPIKey,
		Language:             "en",
		EnablePartialResults: true,
		VADEnabled:           f.config.VADEnabled && prevElem != nil,
		SampleRate:           16000,
		Channels:             1,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create STT element: %w", err)
	}
	p.AddElement(sttElem)
	elems = append(elems, sttElem)
	if prevElem != nil {
		p.Link(prevElem, sttElem)
	}
	prevElem = sttElem
	log.Printf("[Factory] STT element added (VAD integration: %v)", f.config.VADEnabled)

	// 3. Chat Element (using ChatElement for GPT)
	chatElem, err := elements.NewChatElement(elements.ChatConfig{
		APIKey:       f.config.OpenAIAPIKey,
		Model:        "gpt-4o-mini",
		SystemPrompt: f.config.SystemPrompt,
		MaxTokens:    150, // Keep responses short for phone
		Temperature:  0.7,
		MaxHistory:   10,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create chat element: %w", err)
	}
	p.AddElement(chatElem)
	elems = append(elems, chatElem)
	p.Link(prevElem, chatElem)
	prevElem = chatElem
	log.Printf("[Factory] Chat element added (GPT-4o-mini)")

	// 4. TTS Element (ElevenLabs WebSocket)
	ttsProvider, err := tts.NewElevenLabsWSTTSProvider(tts.ElevenLabsWSTTSConfig{
		APIKey:  f.config.ElevenLabsAPIKey,
		VoiceID: f.config.ElevenLabsVoice,
		Model:   "eleven_turbo_v2_5",
		Speed:   1.0,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create TTS provider: %w", err)
	}

	ttsElem := elements.NewUniversalTTSElement(ttsProvider)
	p.AddElement(ttsElem)
	elems = append(elems, ttsElem)
	p.Link(prevElem, ttsElem)
	prevElem = ttsElem
	log.Printf("[Factory] TTS element added (ElevenLabs)")

	// 5. Output handler - sends audio back to Twilio
	outputHandler := &twilioOutputHandler{
		conn: conn,
		wg:   &sync.WaitGroup{},
	}

	// Start output handler
	outputHandler.wg.Add(1)
	go outputHandler.handleOutput(ctx, p)

	// Register cleanup
	go func() {
		<-ctx.Done()
		outputHandler.wg.Wait()
	}()

	log.Printf("[Factory] Pipeline created with %d elements", len(elems))
	return p, nil
}

// twilioOutputHandler sends pipeline output to Twilio.
type twilioOutputHandler struct {
	conn *connection.TwilioConnection
	wg   *sync.WaitGroup
}

func (h *twilioOutputHandler) handleOutput(ctx context.Context, p *pipeline.Pipeline) {
	defer h.wg.Done()

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			msg := p.Pull()
			if msg == nil {
				continue
			}

			if msg.Type == pipeline.MsgTypeAudio && msg.AudioData != nil {
				h.conn.SendMessage(msg)
			}
		}
	}
}
