// Web Voice Assistant Example
//
// A real-time voice conversation assistant using WebRTC for audio transport.
//
// Architecture:
//   WebRTC (48kHz) → Resample → VAD → 11labs ASR → OpenAI Chat → 11labs TTS → Resample → WebRTC
//
// Features:
//   - Real-time voice conversation with AI
//   - Low-latency streaming responses (gpt-4o-mini)
//   - Voice activity detection with interrupt support
//   - ElevenLabs high-quality ASR and TTS
//
// Usage:
//
//	export OPENAI_API_KEY=sk-xxx
//	export ELEVENLABS_API_KEY=xi-xxx
//	go run examples/web-voice-assistant/main.go
//	open http://localhost:8082
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi"
	"github.com/realtime-ai/realtime-ai/pkg/server"
	"github.com/realtime-ai/realtime-ai/pkg/tts"
)

const (
	defaultHTTPPort = ":8082"
	defaultUDPPort  = 9001
)

func main() {
	// Load environment variables
	godotenv.Load()

	// Validate required environment variables
	openaiKey := os.Getenv("OPENAI_API_KEY")
	if openaiKey == "" {
		log.Fatal("OPENAI_API_KEY is required")
	}

	elevenlabsKey := os.Getenv("ELEVENLABS_API_KEY")
	if elevenlabsKey == "" {
		log.Fatal("ELEVENLABS_API_KEY is required")
	}

	// Find VAD model
	vadModelPath := findVADModel()
	if vadModelPath == "" {
		log.Println("Warning: VAD model not found, interrupt feature will be limited")
	}

	// Get configuration from environment
	httpPort := getEnv("VOICE_ASSISTANT_PORT", defaultHTTPPort)
	udpPort := getEnvInt("VOICE_ASSISTANT_UDP_PORT", defaultUDPPort)
	voice := getEnv("VOICE_ASSISTANT_VOICE", "Rachel")
	systemPrompt := getEnv("VOICE_ASSISTANT_SYSTEM_PROMPT",
		"You are a helpful voice assistant. Keep your responses concise, natural, and conversational. Respond in the same language as the user.")

	// Create server configuration
	config := server.DefaultWebRTCRealtimeConfig()
	config.RTCUDPPort = udpPort
	config.ICELite = true

	// Create WebRTC server
	srv := server.NewWebRTCRealtimeServer(config)

	// Set pipeline factory
	srv.SetPipelineFactory(func(ctx context.Context, session *realtimeapi.Session) (*pipeline.Pipeline, error) {
		return createPipeline(ctx, session, PipelineConfig{
			OpenAIKey:     openaiKey,
			ElevenLabsKey: elevenlabsKey,
			VADModelPath:  vadModelPath,
			Voice:         voice,
			SystemPrompt:  systemPrompt,
		})
	})

	// Start WebRTC server
	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start WebRTC server: %v", err)
	}

	// HTTP handlers
	http.HandleFunc("/session", srv.HandleNegotiate)
	http.Handle("/", http.FileServer(http.Dir("examples/web-voice-assistant")))

	log.Println("===========================================")
	log.Println("  Web Voice Assistant")
	log.Println("===========================================")
	log.Printf("  HTTP: http://localhost%s", httpPort)
	log.Printf("  UDP:  %d", udpPort)
	log.Printf("  Voice: %s", voice)
	log.Printf("  VAD:  %v", vadModelPath != "")
	log.Println("===========================================")

	if err := http.ListenAndServe(httpPort, nil); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}

// PipelineConfig holds configuration for pipeline creation
type PipelineConfig struct {
	OpenAIKey     string
	ElevenLabsKey string
	VADModelPath  string
	Voice         string
	SystemPrompt  string
}

// createPipeline creates the voice assistant pipeline
func createPipeline(ctx context.Context, session *realtimeapi.Session, cfg PipelineConfig) (*pipeline.Pipeline, error) {
	p := pipeline.NewPipeline("voice-assistant-" + session.ID)

	// Enable interrupt manager with hybrid mode
	interruptConfig := pipeline.DefaultInterruptConfig()
	interruptConfig.EnableHybridMode = true
	interruptConfig.MinSpeechForConfirmMs = 300
	interruptConfig.InterruptCooldownMs = 500
	p.EnableInterruptManager(interruptConfig)

	// Create elements
	var elems []pipeline.Element
	var prevElem pipeline.Element

	// 1. Input resample: 48kHz → 16kHz (WebRTC to processing)
	inputResample := elements.NewAudioResampleElement(48000, 16000, 1, 1)
	elems = append(elems, inputResample)
	prevElem = inputResample

	// 2. VAD (optional, but recommended for interrupt)
	if cfg.VADModelPath != "" {
		vadConfig := elements.SileroVADConfig{
			ModelPath:       cfg.VADModelPath,
			Threshold:       0.5,
			MinSilenceDurMs: 500,
			SpeechPadMs:     30,
			Mode:            elements.VADModePassthrough,
		}
		vadElem, err := elements.NewSileroVADElement(vadConfig)
		if err != nil {
			log.Printf("[Pipeline] Warning: Failed to create VAD element: %v", err)
		} else {
			if err := vadElem.Init(ctx); err != nil {
				log.Printf("[Pipeline] Warning: Failed to init VAD element: %v", err)
			} else {
				elems = append(elems, vadElem)
				p.Link(prevElem, vadElem)
				prevElem = vadElem
				log.Printf("[Pipeline] VAD enabled")
			}
		}
	}

	// 3. ElevenLabs ASR
	asrConfig := elements.ElevenLabsRealtimeSTTConfig{
		APIKey:               cfg.ElevenLabsKey,
		Language:             "en", // Auto-detect works well too
		Model:                "scribe_v2_realtime",
		EnablePartialResults: false, // Only final results to reduce LLM calls
		VADEnabled:           true,  // Use VAD events for commit
		SampleRate:           16000,
		Channels:             1,
	}
	asrElem, err := elements.NewElevenLabsRealtimeSTTElement(asrConfig)
	if err != nil {
		return nil, err
	}
	elems = append(elems, asrElem)
	p.Link(prevElem, asrElem)
	prevElem = asrElem

	// 4. Chat (OpenAI gpt-4o-mini)
	chatConfig := elements.ChatConfig{
		APIKey:       cfg.OpenAIKey,
		Model:        "gpt-4o-mini",
		SystemPrompt: cfg.SystemPrompt,
		Streaming:    true,
		MaxHistory:   20,
		Temperature:  0.7,
	}
	chatElem, err := elements.NewChatElement(chatConfig)
	if err != nil {
		return nil, err
	}
	elems = append(elems, chatElem)
	p.Link(prevElem, chatElem)
	prevElem = chatElem

	// 5. ElevenLabs TTS
	ttsConfig := tts.ElevenLabsWSTTSConfig{
		APIKey:  cfg.ElevenLabsKey,
		VoiceID: getVoiceID(cfg.Voice),
		Model:   "eleven_turbo_v2_5",
		Speed:   1.0,
	}
	ttsProvider, err := tts.NewElevenLabsWSTTSProvider(ttsConfig)
	if err != nil {
		return nil, err
	}
	ttsElem := elements.NewUniversalTTSElement(ttsProvider)
	elems = append(elems, ttsElem)
	p.Link(prevElem, ttsElem)
	prevElem = ttsElem

	// 6. Output resample: 16kHz → 48kHz (processing to WebRTC)
	// Note: ElevenLabs outputs 22050Hz or 24000Hz depending on model
	// We'll use 24000 → 48000 for eleven_turbo_v2_5
	outputResample := elements.NewAudioResampleElement(24000, 48000, 1, 1)
	elems = append(elems, outputResample)
	p.Link(prevElem, outputResample)

	// Add all elements to pipeline
	p.AddElements(elems)

	log.Printf("[Pipeline] Created voice assistant pipeline for session %s", session.ID)
	log.Printf("[Pipeline] Flow: Resample(48k→16k) → VAD → ASR(11labs) → Chat(gpt-4o-mini) → TTS(11labs) → Resample(24k→48k)")

	return p, nil
}

// getVoiceID returns the ElevenLabs voice ID for a voice name
func getVoiceID(name string) string {
	voices := map[string]string{
		"Rachel":  "21m00Tcm4TlvDq8ikWAM",
		"Domi":    "AZnzlk1XvdvUeBnXmlld",
		"Bella":   "EXAVITQu4vr4xnSDxMaL",
		"Antoni":  "ErXwobaYiN019PkySvjV",
		"Elli":    "MF3mGyEYCl7XYWbV9V6O",
		"Josh":    "TxGEqnHWrfWFTfGW9XjX",
		"Arnold":  "VR6AewLTigWG4xSOukaG",
		"Adam":    "pNInz6obpgDQGcFmaJgB",
		"Sam":     "yoZ06aMxZJJ28mfd3POQ",
	}

	if id, ok := voices[name]; ok {
		return id
	}
	// Return as-is if it looks like an ID
	if len(name) > 10 {
		return name
	}
	return voices["Rachel"] // Default
}

// findVADModel looks for the VAD model in common locations
func findVADModel() string {
	paths := []string{
		"models/silero_vad.onnx",
		"../models/silero_vad.onnx",
		"../../models/silero_vad.onnx",
	}

	// Try relative to executable
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		paths = append(paths, filepath.Join(dir, "models", "silero_vad.onnx"))
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}

	return ""
}

// getEnv returns environment variable value or default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt returns environment variable as int or default
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var result int
		if _, err := os.Stdout.Write([]byte("")); err == nil {
			// Just return default if parsing fails
		}
		if n, err := parseInt(value); err == nil {
			return n
		}
		return result
	}
	return defaultValue
}

func parseInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, nil
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
