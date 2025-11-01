package elements

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"sync"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/tts"
)

// UniversalTTSElement is a TTS element that can use any TTSProvider
// This provides flexibility to switch between different TTS services
// (OpenAI, Azure, ElevenLabs, etc.) without changing the pipeline code
type UniversalTTSElement struct {
	*pipeline.BaseElement

	provider tts.TTSProvider
	voice    string
	language string
	options  map[string]interface{}

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewUniversalTTSElement creates a new universal TTS element with the given provider
func NewUniversalTTSElement(provider tts.TTSProvider) *UniversalTTSElement {
	elem := &UniversalTTSElement{
		BaseElement: pipeline.NewBaseElement(fmt.Sprintf("%s-tts-element", provider.Name()), 100),
		provider:    provider,
		voice:       provider.GetDefaultVoice(),
		language:    "en-US", // Default language
		options:     make(map[string]interface{}),
	}

	// Register properties
	elem.registerProperties()

	return elem
}

// registerProperties registers the element's configurable properties
func (e *UniversalTTSElement) registerProperties() {
	e.RegisterProperty(pipeline.PropertyDesc{
		Name:     "voice",
		Type:     reflect.TypeOf(""),
		Writable: true,
		Readable: true,
		Default:  e.provider.GetDefaultVoice(),
	})

	e.RegisterProperty(pipeline.PropertyDesc{
		Name:     "language",
		Type:     reflect.TypeOf(""),
		Writable: true,
		Readable: true,
		Default:  "en-US",
	})
}

// Start starts the TTS element
func (e *UniversalTTSElement) Start(ctx context.Context) error {
	// Validate provider configuration
	if err := e.provider.ValidateConfig(); err != nil {
		return fmt.Errorf("TTS provider validation failed: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	// Start processing goroutine
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.processMessages(ctx)
	}()

	log.Printf("[%s] TTS element started with voice: %s", e.provider.Name(), e.voice)
	return nil
}

// Stop stops the TTS element
func (e *UniversalTTSElement) Stop() error {
	if e.cancel != nil {
		e.cancel()
		e.wg.Wait()
		e.cancel = nil
	}
	log.Printf("[%s] TTS element stopped", e.provider.Name())
	return nil
}

// processMessages processes incoming text messages and synthesizes speech
func (e *UniversalTTSElement) processMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-e.BaseElement.InChan:
			if msg.Type == pipeline.MsgTypeData && msg.TextData != nil {
				text := string(msg.TextData.Data)
				if err := e.synthesizeAndOutput(ctx, text); err != nil {
					log.Printf("[%s] Failed to synthesize speech: %v", e.provider.Name(), err)
					e.publishError(fmt.Sprintf("Failed to synthesize speech: %v", err))
				}
			}
		}
	}
}

// synthesizeAndOutput synthesizes speech from text and outputs audio data
func (e *UniversalTTSElement) synthesizeAndOutput(ctx context.Context, text string) error {
	// Create synthesis request
	req := &tts.SynthesizeRequest{
		Text:     text,
		Voice:    e.voice,
		Language: e.language,
		Options:  e.options,
	}

	// Call the provider's synthesize method
	resp, err := e.provider.Synthesize(ctx, req)
	if err != nil {
		return err
	}

	// Create audio message for the pipeline
	msg := &pipeline.PipelineMessage{
		Type: pipeline.MsgTypeAudio,
		AudioData: &pipeline.AudioData{
			Data:       resp.AudioData,
			SampleRate: resp.AudioFormat.SampleRate,
			Channels:   resp.AudioFormat.Channels,
			MediaType:  resp.AudioFormat.MediaType,
			Timestamp:  time.Now(),
		},
	}

	// Send to output channel
	e.BaseElement.OutChan <- msg

	log.Printf("[%s] Synthesized %d bytes of audio (voice: %s)",
		e.provider.Name(), len(resp.AudioData), e.voice)

	return nil
}

// publishError publishes an error event to the pipeline bus
func (e *UniversalTTSElement) publishError(message string) {
	if e.BaseElement.Bus() != nil {
		e.BaseElement.Bus().Publish(pipeline.Event{
			Type:      pipeline.EventError,
			Timestamp: time.Now(),
			Payload:   message,
		})
	}
}

// SetVoice sets the voice to use for synthesis
func (e *UniversalTTSElement) SetVoice(voice string) {
	e.voice = voice
}

// SetLanguage sets the language for synthesis
func (e *UniversalTTSElement) SetLanguage(language string) {
	e.language = language
}

// SetOption sets a provider-specific option
func (e *UniversalTTSElement) SetOption(key string, value interface{}) {
	if e.options == nil {
		e.options = make(map[string]interface{})
	}
	e.options[key] = value
}

// GetProvider returns the underlying TTS provider
func (e *UniversalTTSElement) GetProvider() tts.TTSProvider {
	return e.provider
}

// GetSupportedVoices returns the list of supported voices
func (e *UniversalTTSElement) GetSupportedVoices() []string {
	return e.provider.GetSupportedVoices()
}
