package elements

import (
	"context"
	"fmt"
	"log"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/asr"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

// Ensure ElevenLabsRealtimeSTTElement implements pipeline.Element
var _ pipeline.Element = (*ElevenLabsRealtimeSTTElement)(nil)

// ElevenLabsRealtimeSTTElement implements speech-to-text using ElevenLabs Scribe V2 Realtime API.
// It provides true streaming ASR with ~150ms latency via WebSocket.
// Supports partial and committed transcripts with manual commit for VAD integration.
type ElevenLabsRealtimeSTTElement struct {
	*pipeline.BaseElement

	// ASR provider
	provider *asr.ElevenLabsProvider

	// ASR configuration
	language             string
	model                string
	enablePartialResults bool

	// Audio configuration (ElevenLabs requires 16kHz)
	sampleRate    int
	channels      int
	bitsPerSample int

	// VAD integration
	vadEnabled    bool
	vadEventsSub  chan pipeline.Event
	isSpeaking    bool
	speakingMutex sync.Mutex

	// Audio buffering (for VAD mode)
	audioBuffer     []byte
	audioBufferLock sync.Mutex

	// Streaming recognizer
	recognizer     asr.StreamingRecognizer
	recognizerLock sync.Mutex

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ElevenLabsRealtimeSTTConfig holds configuration for ElevenLabsRealtimeSTTElement.
type ElevenLabsRealtimeSTTConfig struct {
	// APIKey is the ElevenLabs API key (if empty, will use ELEVENLABS_API_KEY env var)
	APIKey string

	// Language code (e.g., "en", "zh", "auto" for auto-detection)
	// Leave empty for auto-detection
	Language string

	// Model to use (default: "scribe_v2_realtime")
	Model string

	// EnablePartialResults enables interim results during recognition
	EnablePartialResults bool

	// VADEnabled determines if element should listen to VAD events
	// When true, recognition is triggered by VAD speech start/end events
	// When false, audio is sent continuously to recognizer
	VADEnabled bool

	// SampleRate in Hz (must be 16000 for ElevenLabs)
	SampleRate int

	// Channels (must be 1 for ElevenLabs - mono only)
	Channels int

	// BitsPerSample (default: 16)
	BitsPerSample int
}

// NewElevenLabsRealtimeSTTElement creates a new ElevenLabs Realtime STT element.
func NewElevenLabsRealtimeSTTElement(config ElevenLabsRealtimeSTTConfig) (*ElevenLabsRealtimeSTTElement, error) {
	// Get API key from config or environment
	apiKey := config.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("ELEVENLABS_API_KEY")
	}

	if apiKey == "" {
		return nil, fmt.Errorf("ElevenLabs API key is required (set APIKey or ELEVENLABS_API_KEY env var)")
	}

	// Create ElevenLabs provider
	provider, err := asr.NewElevenLabsProvider(asr.ElevenLabsConfig{
		APIKey: apiKey,
		Model:  config.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create ElevenLabs provider: %w", err)
	}

	// Set defaults and validate
	if config.SampleRate == 0 {
		config.SampleRate = 16000
	}
	if config.SampleRate != 16000 {
		return nil, fmt.Errorf("ElevenLabs requires 16kHz sample rate, got %d", config.SampleRate)
	}

	if config.Channels == 0 {
		config.Channels = 1
	}
	if config.Channels != 1 {
		return nil, fmt.Errorf("ElevenLabs only supports mono audio, got %d channels", config.Channels)
	}

	if config.BitsPerSample == 0 {
		config.BitsPerSample = 16
	}

	elem := &ElevenLabsRealtimeSTTElement{
		BaseElement:          pipeline.NewBaseElement("elevenlabs-realtime-stt", 100),
		provider:             provider,
		language:             config.Language,
		model:                config.Model,
		enablePartialResults: config.EnablePartialResults,
		vadEnabled:           config.VADEnabled,
		sampleRate:           config.SampleRate,
		channels:             config.Channels,
		bitsPerSample:        config.BitsPerSample,
		audioBuffer:          make([]byte, 0, 16000*2*10), // 10 seconds buffer
	}

	// Register properties for runtime configuration
	elem.registerProperties()

	return elem, nil
}

// registerProperties sets up the property system for runtime configuration.
func (e *ElevenLabsRealtimeSTTElement) registerProperties() {
	e.RegisterProperty(pipeline.PropertyDesc{
		Name:     "language",
		Type:     reflect.TypeOf(""),
		Writable: true,
		Readable: true,
		Default:  e.language,
	})

	e.RegisterProperty(pipeline.PropertyDesc{
		Name:     "model",
		Type:     reflect.TypeOf(""),
		Writable: true,
		Readable: true,
		Default:  e.model,
	})

	e.RegisterProperty(pipeline.PropertyDesc{
		Name:     "enable_partial_results",
		Type:     reflect.TypeOf(false),
		Writable: true,
		Readable: true,
		Default:  e.enablePartialResults,
	})

	e.RegisterProperty(pipeline.PropertyDesc{
		Name:     "vad_enabled",
		Type:     reflect.TypeOf(false),
		Writable: true,
		Readable: true,
		Default:  e.vadEnabled,
	})
}

// Start starts the ElevenLabs Realtime STT element.
func (e *ElevenLabsRealtimeSTTElement) Start(ctx context.Context) error {
	e.ctx, e.cancel = context.WithCancel(ctx)

	log.Printf("[ElevenLabsSTT] Starting element (VAD: %v, Language: %s, Model: %s)",
		e.vadEnabled, e.language, e.model)

	// Subscribe to VAD events if VAD is enabled
	if e.vadEnabled && e.BaseElement.Bus() != nil {
		e.vadEventsSub = make(chan pipeline.Event, 10)
		e.BaseElement.Bus().Subscribe(pipeline.EventVADSpeechStart, e.vadEventsSub)
		e.BaseElement.Bus().Subscribe(pipeline.EventVADSpeechEnd, e.vadEventsSub)

		log.Printf("[ElevenLabsSTT] Subscribed to VAD events")
	}

	// Start streaming recognizer
	if err := e.startRecognizer(e.ctx); err != nil {
		e.cancel()
		return fmt.Errorf("failed to start recognizer: %w", err)
	}

	// Start audio processing goroutine
	e.wg.Add(1)
	go e.processAudio(e.ctx)

	// Start VAD event handler if enabled
	if e.vadEnabled {
		e.wg.Add(1)
		go e.handleVADEvents(e.ctx)
	}

	// Start result handler
	e.wg.Add(1)
	go e.handleResults(e.ctx)

	log.Printf("[ElevenLabsSTT] Element started successfully")
	return nil
}

// Stop stops the ElevenLabs Realtime STT element.
func (e *ElevenLabsRealtimeSTTElement) Stop() error {
	log.Printf("[ElevenLabsSTT] Stopping element")

	if e.cancel != nil {
		e.cancel()
	}

	// Close recognizer first
	e.recognizerLock.Lock()
	if e.recognizer != nil {
		e.recognizer.Close()
		e.recognizer = nil
	}
	e.recognizerLock.Unlock()

	// Wait for goroutines
	e.wg.Wait()

	// Close provider
	if e.provider != nil {
		e.provider.Close()
	}

	// Unsubscribe from VAD events
	if e.vadEventsSub != nil {
		if e.BaseElement.Bus() != nil {
			e.BaseElement.Bus().Unsubscribe(pipeline.EventVADSpeechStart, e.vadEventsSub)
			e.BaseElement.Bus().Unsubscribe(pipeline.EventVADSpeechEnd, e.vadEventsSub)
		}
		close(e.vadEventsSub)
		e.vadEventsSub = nil
	}

	log.Printf("[ElevenLabsSTT] Stopped")
	return nil
}

// startRecognizer creates and starts a streaming recognizer.
func (e *ElevenLabsRealtimeSTTElement) startRecognizer(ctx context.Context) error {
	e.recognizerLock.Lock()
	defer e.recognizerLock.Unlock()

	audioConfig := asr.AudioConfig{
		SampleRate:    e.sampleRate,
		Channels:      e.channels,
		Encoding:      "pcm",
		BitsPerSample: e.bitsPerSample,
	}

	recognitionConfig := asr.RecognitionConfig{
		Language:             e.language,
		Model:                e.model,
		EnablePartialResults: e.enablePartialResults,
	}

	recognizer, err := e.provider.StreamingRecognize(ctx, audioConfig, recognitionConfig)
	if err != nil {
		return fmt.Errorf("failed to create streaming recognizer: %w", err)
	}

	e.recognizer = recognizer
	log.Printf("[ElevenLabsSTT] Streaming recognizer started")
	return nil
}

// processAudio processes incoming audio messages.
func (e *ElevenLabsRealtimeSTTElement) processAudio(ctx context.Context) {
	defer e.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return

		case msg, ok := <-e.BaseElement.InChan:
			if !ok {
				return
			}

			// Only process audio messages
			if msg.Type != pipeline.MsgTypeAudio || msg.AudioData == nil {
				continue
			}

			// Validate audio format
			if msg.AudioData.SampleRate != e.sampleRate {
				log.Printf("[ElevenLabsSTT] Warning: Audio sample rate mismatch (expected %d, got %d)",
					e.sampleRate, msg.AudioData.SampleRate)
				continue
			}

			// If VAD is disabled, send audio directly to recognizer
			if !e.vadEnabled {
				e.sendAudioToRecognizer(ctx, msg.AudioData.Data)
			} else {
				// With VAD, buffer audio and send when speaking
				e.speakingMutex.Lock()
				isSpeaking := e.isSpeaking
				e.speakingMutex.Unlock()

				if isSpeaking {
					// Buffer audio for potential commit
					e.audioBufferLock.Lock()
					e.audioBuffer = append(e.audioBuffer, msg.AudioData.Data...)
					e.audioBufferLock.Unlock()

					// Send audio to recognizer
					e.sendAudioToRecognizer(ctx, msg.AudioData.Data)
				}
			}
		}
	}
}

// handleVADEvents processes VAD speech start/end events.
func (e *ElevenLabsRealtimeSTTElement) handleVADEvents(ctx context.Context) {
	defer e.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-e.vadEventsSub:
			if !ok {
				return
			}

			switch event.Type {
			case pipeline.EventVADSpeechStart:
				// Extract pre-roll audio from VAD payload
				if payload, ok := event.Payload.(pipeline.VADPayload); ok {
					// Send pre-roll audio first (before setting isSpeaking)
					if len(payload.PreRollAudio) > 0 {
						log.Printf("[ElevenLabsSTT] VAD speech started with %d bytes pre-roll audio",
							len(payload.PreRollAudio))
						e.sendAudioToRecognizer(ctx, payload.PreRollAudio)
					} else {
						log.Printf("[ElevenLabsSTT] VAD speech started (no pre-roll)")
					}
				} else {
					log.Printf("[ElevenLabsSTT] VAD speech started (legacy payload)")
				}

				e.speakingMutex.Lock()
				e.isSpeaking = true
				e.speakingMutex.Unlock()

				// Clear buffer and start fresh
				e.audioBufferLock.Lock()
				e.audioBuffer = e.audioBuffer[:0]
				e.audioBufferLock.Unlock()

			case pipeline.EventVADSpeechEnd:
				log.Printf("[ElevenLabsSTT] VAD speech ended")
				e.speakingMutex.Lock()
				e.isSpeaking = false
				e.speakingMutex.Unlock()

				// Commit to trigger final transcription
				e.commitRecognizer(ctx)
			}
		}
	}
}

// sendAudioToRecognizer sends audio data to the streaming recognizer.
func (e *ElevenLabsRealtimeSTTElement) sendAudioToRecognizer(ctx context.Context, audioData []byte) {
	e.recognizerLock.Lock()
	recognizer := e.recognizer
	e.recognizerLock.Unlock()

	if recognizer == nil {
		return
	}

	if err := recognizer.SendAudio(ctx, audioData); err != nil {
		log.Printf("[ElevenLabsSTT] Error sending audio to recognizer: %v", err)
	}
}

// commitRecognizer commits the audio buffer to trigger final transcription.
func (e *ElevenLabsRealtimeSTTElement) commitRecognizer(ctx context.Context) {
	e.recognizerLock.Lock()
	recognizer := e.recognizer
	e.recognizerLock.Unlock()

	if recognizer == nil {
		return
	}

	// Check if recognizer supports Commit method (ElevenLabs specific)
	if er, ok := asr.IsElevenLabsRecognizer(recognizer); ok {
		if err := er.Commit(ctx); err != nil {
			log.Printf("[ElevenLabsSTT] Error committing audio: %v", err)
		} else {
			log.Printf("[ElevenLabsSTT] Committed audio for final transcription")
		}
	}
}

// handleResults processes recognition results from the streaming recognizer.
func (e *ElevenLabsRealtimeSTTElement) handleResults(ctx context.Context) {
	defer e.wg.Done()

	e.recognizerLock.Lock()
	recognizer := e.recognizer
	e.recognizerLock.Unlock()

	if recognizer == nil {
		log.Printf("[ElevenLabsSTT] No recognizer available for results")
		return
	}

	resultsChan := recognizer.Results()

	for {
		select {
		case <-ctx.Done():
			return

		case result, ok := <-resultsChan:
			if !ok {
				log.Printf("[ElevenLabsSTT] Results channel closed")
				return
			}

			if result == nil {
				continue
			}

			// Skip empty results
			if result.Text == "" {
				continue
			}

			// Determine text type
			textType := "text/partial"
			eventType := pipeline.EventPartialResult
			if result.IsFinal {
				textType = "text/final"
				eventType = pipeline.EventFinalResult
			}

			log.Printf("[ElevenLabsSTT] Recognition result (%s): %s", textType, result.Text)

			// Create text data message
			textMsg := &pipeline.PipelineMessage{
				Type:      pipeline.MsgTypeData,
				Timestamp: time.Now(),
				TextData: &pipeline.TextData{
					Data:      []byte(result.Text),
					TextType:  textType,
					Timestamp: result.Timestamp,
				},
			}

			// Send to output channel
			select {
			case e.BaseElement.OutChan <- textMsg:
			case <-ctx.Done():
				return
			}

			// Publish event to bus
			if e.BaseElement.Bus() != nil {
				e.BaseElement.Bus().Publish(pipeline.Event{
					Type:      eventType,
					Timestamp: result.Timestamp,
					Payload:   result.Text,
				})
			}
		}
	}
}

// SetProperty sets a property value at runtime.
func (e *ElevenLabsRealtimeSTTElement) SetProperty(name string, value interface{}) error {
	switch name {
	case "language":
		if lang, ok := value.(string); ok {
			e.language = lang
			log.Printf("[ElevenLabsSTT] Language set to: %s", lang)
			return nil
		}
	case "model":
		if model, ok := value.(string); ok {
			e.model = model
			log.Printf("[ElevenLabsSTT] Model set to: %s", model)
			return nil
		}
	case "enable_partial_results":
		if enable, ok := value.(bool); ok {
			e.enablePartialResults = enable
			log.Printf("[ElevenLabsSTT] Partial results: %v", enable)
			return nil
		}
	case "vad_enabled":
		if enable, ok := value.(bool); ok {
			e.vadEnabled = enable
			log.Printf("[ElevenLabsSTT] VAD enabled: %v", enable)
			return nil
		}
	}

	return e.BaseElement.SetProperty(name, value)
}

// GetProperty gets a property value.
func (e *ElevenLabsRealtimeSTTElement) GetProperty(name string) (interface{}, error) {
	switch name {
	case "language":
		return e.language, nil
	case "model":
		return e.model, nil
	case "enable_partial_results":
		return e.enablePartialResults, nil
	case "vad_enabled":
		return e.vadEnabled, nil
	}

	return e.BaseElement.GetProperty(name)
}

// Commit commits the current audio buffer to trigger final transcription.
// This is useful in non-VAD mode when you want to manually control when
// final transcriptions are generated.
func (e *ElevenLabsRealtimeSTTElement) Commit(ctx context.Context) error {
	e.commitRecognizer(ctx)
	return nil
}
