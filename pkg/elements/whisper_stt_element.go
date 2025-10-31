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

// Ensure WhisperSTTElement implements pipeline.Element
var _ pipeline.Element = (*WhisperSTTElement)(nil)

// WhisperSTTElement implements speech-to-text using OpenAI Whisper API.
// It can work standalone or integrate with VAD for optimized recognition.
type WhisperSTTElement struct {
	*pipeline.BaseElement

	// ASR provider (abstraction allows for easy replacement)
	provider asr.Provider

	// ASR configuration
	language            string
	model               string
	enablePartialResults bool
	prompt              string
	temperature         float32

	// Audio configuration
	sampleRate    int
	channels      int
	bitsPerSample int

	// VAD integration
	vadEnabled    bool
	vadEventsSub  chan pipeline.Event
	isSpeaking    bool
	speakingMutex sync.Mutex

	// Audio buffering
	audioBuffer     []byte
	audioBufferLock sync.Mutex

	// Streaming recognizer
	recognizer     asr.StreamingRecognizer
	recognizerLock sync.Mutex

	// Lifecycle management
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// WhisperSTTConfig holds configuration for WhisperSTTElement.
type WhisperSTTConfig struct {
	// APIKey is the OpenAI API key (if empty, will use OPENAI_API_KEY env var)
	APIKey string

	// Language code (e.g., "en", "zh", "auto" for auto-detection)
	// Leave empty for auto-detection
	Language string

	// Model to use (default: "whisper-1")
	Model string

	// EnablePartialResults enables interim results during recognition
	EnablePartialResults bool

	// Prompt provides context to guide the recognition
	Prompt string

	// Temperature for sampling (0.0-1.0, default: 0.0)
	Temperature float32

	// VADEnabled determines if element should listen to VAD events
	// When true, recognition is triggered by VAD speech start/end events
	// When false, recognition runs continuously on buffered audio
	VADEnabled bool

	// SampleRate in Hz (default: 16000)
	SampleRate int

	// Channels (default: 1 for mono)
	Channels int

	// BitsPerSample (default: 16)
	BitsPerSample int
}

// NewWhisperSTTElement creates a new Whisper STT element.
func NewWhisperSTTElement(config WhisperSTTConfig) (*WhisperSTTElement, error) {
	// Get API key from config or environment
	apiKey := config.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}

	if apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required (set APIKey or OPENAI_API_KEY env var)")
	}

	// Create Whisper provider
	provider, err := asr.NewWhisperProvider(apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create Whisper provider: %w", err)
	}

	// Set defaults
	if config.Model == "" {
		config.Model = "whisper-1"
	}
	if config.SampleRate == 0 {
		config.SampleRate = 16000
	}
	if config.Channels == 0 {
		config.Channels = 1
	}
	if config.BitsPerSample == 0 {
		config.BitsPerSample = 16
	}

	elem := &WhisperSTTElement{
		BaseElement:          pipeline.NewBaseElement("whisper-stt", 100),
		provider:             provider,
		language:             config.Language,
		model:                config.Model,
		enablePartialResults: config.EnablePartialResults,
		prompt:               config.Prompt,
		temperature:          config.Temperature,
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
func (e *WhisperSTTElement) registerProperties() {
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

// Start starts the Whisper STT element.
func (e *WhisperSTTElement) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	log.Printf("[WhisperSTT] Starting element (VAD: %v, Language: %s, Model: %s)",
		e.vadEnabled, e.language, e.model)

	// Subscribe to VAD events if VAD is enabled
	if e.vadEnabled && e.BaseElement.Bus() != nil {
		e.vadEventsSub = make(chan pipeline.Event, 10)
		e.BaseElement.Bus().Subscribe(pipeline.EventVADSpeechStart, e.vadEventsSub)
		e.BaseElement.Bus().Subscribe(pipeline.EventVADSpeechEnd, e.vadEventsSub)

		log.Printf("[WhisperSTT] Subscribed to VAD events")
	}

	// Start audio processing goroutine
	e.wg.Add(1)
	go e.processAudio(ctx)

	// Start VAD event handler if enabled
	if e.vadEnabled {
		e.wg.Add(1)
		go e.handleVADEvents(ctx)
	}

	// Start streaming recognizer
	if err := e.startRecognizer(ctx); err != nil {
		cancel()
		e.wg.Wait()
		return fmt.Errorf("failed to start recognizer: %w", err)
	}

	// Start result handler
	e.wg.Add(1)
	go e.handleResults(ctx)

	return nil
}

// Stop stops the Whisper STT element.
func (e *WhisperSTTElement) Stop() error {
	log.Printf("[WhisperSTT] Stopping element")

	if e.cancel != nil {
		e.cancel()
		e.wg.Wait()
		e.cancel = nil
	}

	// Close recognizer
	e.recognizerLock.Lock()
	if e.recognizer != nil {
		e.recognizer.Close()
		e.recognizer = nil
	}
	e.recognizerLock.Unlock()

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

	log.Printf("[WhisperSTT] Stopped")
	return nil
}

// startRecognizer creates and starts a streaming recognizer.
func (e *WhisperSTTElement) startRecognizer(ctx context.Context) error {
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
		Prompt:               e.prompt,
		Temperature:          e.temperature,
	}

	recognizer, err := e.provider.StreamingRecognize(ctx, audioConfig, recognitionConfig)
	if err != nil {
		return fmt.Errorf("failed to create streaming recognizer: %w", err)
	}

	e.recognizer = recognizer
	log.Printf("[WhisperSTT] Streaming recognizer started")
	return nil
}

// processAudio processes incoming audio messages.
func (e *WhisperSTTElement) processAudio(ctx context.Context) {
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
				log.Printf("[WhisperSTT] Warning: Audio sample rate mismatch (expected %d, got %d)",
					e.sampleRate, msg.AudioData.SampleRate)
				continue
			}

			// Buffer audio
			e.audioBufferLock.Lock()
			e.audioBuffer = append(e.audioBuffer, msg.AudioData.Data...)
			e.audioBufferLock.Unlock()

			// If VAD is disabled, send audio directly to recognizer
			if !e.vadEnabled {
				e.sendAudioToRecognizer(ctx, msg.AudioData.Data)
			} else {
				// With VAD, we only send audio when speaking
				e.speakingMutex.Lock()
				isSpeaking := e.isSpeaking
				e.speakingMutex.Unlock()

				if isSpeaking {
					e.sendAudioToRecognizer(ctx, msg.AudioData.Data)
				}
			}
		}
	}
}

// handleVADEvents processes VAD speech start/end events.
func (e *WhisperSTTElement) handleVADEvents(ctx context.Context) {
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
				log.Printf("[WhisperSTT] VAD speech started")
				e.speakingMutex.Lock()
				e.isSpeaking = true
				e.speakingMutex.Unlock()

				// Clear buffer and start fresh
				e.audioBufferLock.Lock()
				e.audioBuffer = e.audioBuffer[:0]
				e.audioBufferLock.Unlock()

			case pipeline.EventVADSpeechEnd:
				log.Printf("[WhisperSTT] VAD speech ended")
				e.speakingMutex.Lock()
				e.isSpeaking = false
				e.speakingMutex.Unlock()

				// Trigger recognition on buffered audio
				e.recognizeBufferedAudio(ctx)
			}
		}
	}
}

// sendAudioToRecognizer sends audio data to the streaming recognizer.
func (e *WhisperSTTElement) sendAudioToRecognizer(ctx context.Context, audioData []byte) {
	e.recognizerLock.Lock()
	recognizer := e.recognizer
	e.recognizerLock.Unlock()

	if recognizer == nil {
		return
	}

	if err := recognizer.SendAudio(ctx, audioData); err != nil {
		log.Printf("[WhisperSTT] Error sending audio to recognizer: %v", err)
	}
}

// recognizeBufferedAudio processes all buffered audio through the recognizer.
func (e *WhisperSTTElement) recognizeBufferedAudio(ctx context.Context) {
	e.audioBufferLock.Lock()
	if len(e.audioBuffer) == 0 {
		e.audioBufferLock.Unlock()
		return
	}

	audioData := make([]byte, len(e.audioBuffer))
	copy(audioData, e.audioBuffer)
	e.audioBufferLock.Unlock()

	// Send to recognizer
	e.sendAudioToRecognizer(ctx, audioData)

	log.Printf("[WhisperSTT] Sent %d bytes of buffered audio for recognition", len(audioData))
}

// handleResults processes recognition results from the streaming recognizer.
func (e *WhisperSTTElement) handleResults(ctx context.Context) {
	defer e.wg.Done()

	e.recognizerLock.Lock()
	recognizer := e.recognizer
	e.recognizerLock.Unlock()

	if recognizer == nil {
		log.Printf("[WhisperSTT] No recognizer available for results")
		return
	}

	resultsChan := recognizer.Results()

	for {
		select {
		case <-ctx.Done():
			return

		case result, ok := <-resultsChan:
			if !ok {
				log.Printf("[WhisperSTT] Results channel closed")
				return
			}

			if result == nil {
				continue
			}

			// Skip empty results
			if result.Text == "" && !result.IsFinal {
				continue
			}

			// Determine text type
			textType := "text/partial"
			eventType := pipeline.EventPartialResult
			if result.IsFinal {
				textType = "text/final"
				eventType = pipeline.EventFinalResult
			}

			log.Printf("[WhisperSTT] Recognition result (%s): %s", textType, result.Text)

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
func (e *WhisperSTTElement) SetProperty(name string, value interface{}) error {
	switch name {
	case "language":
		if lang, ok := value.(string); ok {
			e.language = lang
			log.Printf("[WhisperSTT] Language set to: %s", lang)
			return nil
		}
	case "model":
		if model, ok := value.(string); ok {
			e.model = model
			log.Printf("[WhisperSTT] Model set to: %s", model)
			return nil
		}
	case "enable_partial_results":
		if enable, ok := value.(bool); ok {
			e.enablePartialResults = enable
			log.Printf("[WhisperSTT] Partial results: %v", enable)
			return nil
		}
	case "vad_enabled":
		if enable, ok := value.(bool); ok {
			e.vadEnabled = enable
			log.Printf("[WhisperSTT] VAD enabled: %v", enable)
			return nil
		}
	}

	return e.BaseElement.SetProperty(name, value)
}

// GetProperty gets a property value.
func (e *WhisperSTTElement) GetProperty(name string) (interface{}, error) {
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
