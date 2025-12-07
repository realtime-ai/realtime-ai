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

// Ensure QwenRealtimeSTTElement implements pipeline.Element
var _ pipeline.Element = (*QwenRealtimeSTTElement)(nil)

// QwenRealtimeSTTElement implements speech-to-text using Alibaba Cloud DashScope Qwen Realtime ASR API.
// It provides true streaming ASR with WebSocket connection.
// Unlike Whisper which buffers audio, Qwen Realtime streams audio directly and provides real-time partial results.
type QwenRealtimeSTTElement struct {
	*pipeline.BaseElement

	// ASR provider
	provider asr.Provider

	// ASR configuration
	language             string
	model                string
	enablePartialResults bool

	// Audio configuration
	sampleRate    int
	channels      int
	bitsPerSample int

	// VAD integration
	vadEnabled   bool
	vadEventsSub chan pipeline.Event
	isSpeaking   bool
	speakingMu   sync.Mutex

	// Streaming recognizer
	recognizer     asr.StreamingRecognizer
	recognizerLock sync.Mutex

	// Audio packet counter for logging
	audioPacketCount int64

	// Lifecycle management
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// QwenRealtimeSTTConfig holds configuration for QwenRealtimeSTTElement.
type QwenRealtimeSTTConfig struct {
	// APIKey is the DashScope API key (if empty, will use DASHSCOPE_API_KEY env var)
	APIKey string

	// Language code (e.g., "zh", "en", "auto" for auto-detection)
	// Default: "zh"
	Language string

	// Model to use (default: "qwen3-asr-flash-realtime")
	Model string

	// EnablePartialResults enables interim results during recognition
	// Qwen Realtime natively supports partial results
	EnablePartialResults bool

	// VADEnabled determines if element should listen to VAD events
	// When true, audio is only streamed when speech is detected
	// When false, all audio is streamed continuously
	VADEnabled bool

	// SampleRate in Hz (default: 16000, recommended for Qwen)
	SampleRate int

	// Channels (default: 1 for mono, required by Qwen)
	Channels int

	// BitsPerSample (default: 16)
	BitsPerSample int
}

// NewQwenRealtimeSTTElement creates a new Qwen Realtime STT element.
func NewQwenRealtimeSTTElement(config QwenRealtimeSTTConfig) (*QwenRealtimeSTTElement, error) {
	// Get API key from config or environment
	apiKey := config.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("DASHSCOPE_API_KEY")
	}

	if apiKey == "" {
		return nil, fmt.Errorf("DashScope API key is required (set APIKey or DASHSCOPE_API_KEY env var)")
	}

	// Create Qwen Realtime provider
	provider, err := asr.NewQwenRealtimeProvider(asr.QwenRealtimeConfig{
		APIKey: apiKey,
		Model:  config.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Qwen Realtime provider: %w", err)
	}

	// Set defaults
	if config.Language == "" {
		config.Language = "zh"
	}
	if config.Model == "" {
		config.Model = "qwen3-asr-flash-realtime"
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

	elem := &QwenRealtimeSTTElement{
		BaseElement:          pipeline.NewBaseElement("qwen-realtime-stt", 100),
		provider:             provider,
		language:             config.Language,
		model:                config.Model,
		enablePartialResults: config.EnablePartialResults,
		vadEnabled:           config.VADEnabled,
		sampleRate:           config.SampleRate,
		channels:             config.Channels,
		bitsPerSample:        config.BitsPerSample,
	}

	// Register properties for runtime configuration
	elem.registerProperties()

	return elem, nil
}

// registerProperties sets up the property system for runtime configuration.
func (e *QwenRealtimeSTTElement) registerProperties() {
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

// Start starts the Qwen Realtime STT element.
func (e *QwenRealtimeSTTElement) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	log.Printf("[QwenRealtimeSTT] Starting element (VAD: %v, Language: %s, Model: %s)",
		e.vadEnabled, e.language, e.model)

	// Subscribe to VAD events if VAD is enabled
	if e.vadEnabled && e.BaseElement.Bus() != nil {
		e.vadEventsSub = make(chan pipeline.Event, 10)
		e.BaseElement.Bus().Subscribe(pipeline.EventVADSpeechStart, e.vadEventsSub)
		e.BaseElement.Bus().Subscribe(pipeline.EventVADSpeechEnd, e.vadEventsSub)

		log.Printf("[QwenRealtimeSTT] Subscribed to VAD events")
	}

	// Start streaming recognizer
	if err := e.startRecognizer(ctx); err != nil {
		cancel()
		return fmt.Errorf("failed to start recognizer: %w", err)
	}

	// Start audio processing goroutine
	e.wg.Add(1)
	go e.processAudio(ctx)

	// Start VAD event handler if enabled
	if e.vadEnabled {
		e.wg.Add(1)
		go e.handleVADEvents(ctx)
	}

	// Start result handler
	e.wg.Add(1)
	go e.handleResults(ctx)

	return nil
}

// Stop stops the Qwen Realtime STT element.
func (e *QwenRealtimeSTTElement) Stop() error {
	log.Printf("[QwenRealtimeSTT] Stopping element")

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

	log.Printf("[QwenRealtimeSTT] Stopped")
	return nil
}

// startRecognizer creates and starts a streaming recognizer.
func (e *QwenRealtimeSTTElement) startRecognizer(ctx context.Context) error {
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
	log.Printf("[QwenRealtimeSTT] Streaming recognizer started")
	return nil
}

// processAudio processes incoming audio messages.
func (e *QwenRealtimeSTTElement) processAudio(ctx context.Context) {
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
				log.Printf("[QwenRealtimeSTT] Warning: Audio sample rate mismatch (expected %d, got %d)",
					e.sampleRate, msg.AudioData.SampleRate)
				continue
			}

			// Check if we should send audio
			shouldSend := true
			if e.vadEnabled {
				e.speakingMu.Lock()
				shouldSend = e.isSpeaking
				e.speakingMu.Unlock()
			}

			if shouldSend {
				e.sendAudioToRecognizer(ctx, msg.AudioData.Data)
			}
		}
	}
}

// handleVADEvents processes VAD speech start/end events.
func (e *QwenRealtimeSTTElement) handleVADEvents(ctx context.Context) {
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
				log.Printf("[QwenRealtimeSTT] VAD speech started")
				e.speakingMu.Lock()
				e.isSpeaking = true
				e.speakingMu.Unlock()

			case pipeline.EventVADSpeechEnd:
				log.Printf("[QwenRealtimeSTT] VAD speech ended")
				e.speakingMu.Lock()
				e.isSpeaking = false
				e.speakingMu.Unlock()

				// Commit audio buffer to trigger final transcription
				e.commitAudioBuffer(ctx)
			}
		}
	}
}

// sendAudioToRecognizer sends audio data to the streaming recognizer.
func (e *QwenRealtimeSTTElement) sendAudioToRecognizer(ctx context.Context, audioData []byte) {
	e.recognizerLock.Lock()
	recognizer := e.recognizer
	e.recognizerLock.Unlock()

	if recognizer == nil {
		return
	}

	if err := recognizer.SendAudio(ctx, audioData); err != nil {
		log.Printf("[QwenRealtimeSTT] Error sending audio to recognizer: %v", err)
	}

	e.audioPacketCount++
	if e.audioPacketCount%100 == 0 {
		log.Printf("[QwenRealtimeSTT] Sent %d audio packets", e.audioPacketCount)
	}
}

// commitAudioBuffer commits the audio buffer to trigger final transcription.
func (e *QwenRealtimeSTTElement) commitAudioBuffer(ctx context.Context) {
	e.recognizerLock.Lock()
	recognizer := e.recognizer
	e.recognizerLock.Unlock()

	if recognizer == nil {
		return
	}

	// Check if recognizer supports Commit (Qwen Realtime specific)
	if qr, ok := asr.IsQwenRealtimeRecognizer(recognizer); ok {
		if err := qr.Commit(ctx); err != nil {
			log.Printf("[QwenRealtimeSTT] Error committing audio buffer: %v", err)
		} else {
			log.Printf("[QwenRealtimeSTT] Audio buffer committed")
		}
	}
}

// handleResults processes recognition results from the streaming recognizer.
func (e *QwenRealtimeSTTElement) handleResults(ctx context.Context) {
	defer e.wg.Done()

	e.recognizerLock.Lock()
	recognizer := e.recognizer
	e.recognizerLock.Unlock()

	if recognizer == nil {
		log.Printf("[QwenRealtimeSTT] No recognizer available for results")
		return
	}

	resultsChan := recognizer.Results()

	for {
		select {
		case <-ctx.Done():
			return

		case result, ok := <-resultsChan:
			if !ok {
				log.Printf("[QwenRealtimeSTT] Results channel closed")
				return
			}

			if result == nil {
				continue
			}

			// Skip empty results unless it's final
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

			log.Printf("[QwenRealtimeSTT] Recognition result (%s): %s", textType, result.Text)

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

// Commit manually commits the audio buffer to trigger final transcription.
// This is useful when VAD is disabled and you want explicit control over when
// to get final transcription results.
func (e *QwenRealtimeSTTElement) Commit(ctx context.Context) error {
	e.commitAudioBuffer(ctx)
	return nil
}

// SetProperty sets a property value at runtime.
func (e *QwenRealtimeSTTElement) SetProperty(name string, value interface{}) error {
	switch name {
	case "language":
		if lang, ok := value.(string); ok {
			e.language = lang
			log.Printf("[QwenRealtimeSTT] Language set to: %s", lang)
			return nil
		}
	case "model":
		if model, ok := value.(string); ok {
			e.model = model
			log.Printf("[QwenRealtimeSTT] Model set to: %s", model)
			return nil
		}
	case "enable_partial_results":
		if enable, ok := value.(bool); ok {
			e.enablePartialResults = enable
			log.Printf("[QwenRealtimeSTT] Partial results: %v", enable)
			return nil
		}
	case "vad_enabled":
		if enable, ok := value.(bool); ok {
			e.vadEnabled = enable
			log.Printf("[QwenRealtimeSTT] VAD enabled: %v", enable)
			return nil
		}
	}

	return e.BaseElement.SetProperty(name, value)
}

// GetProperty gets a property value.
func (e *QwenRealtimeSTTElement) GetProperty(name string) (interface{}, error) {
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
