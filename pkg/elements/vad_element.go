//go:build vad

package elements

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"reflect"
	"sync"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/streamer45/silero-vad-go/speech"
)

// VADMode defines the operating mode of the VAD element
type VADMode int

const (
	// VADModePassthrough passes all audio through and emits events
	VADModePassthrough VADMode = iota
	// VADModeFilter only passes audio segments containing speech
	VADModeFilter
)

// VADEventPayload contains information about VAD events
type VADEventPayload struct {
	SessionID  string
	Confidence float32
	Timestamp  time.Time
	// AudioMs is approximate audio position (ms) when the event is emitted.
	// It is derived from processed samples and has ~chunk-level resolution.
	AudioMs int
}

// SileroVADConfig holds configuration for Silero VAD
type SileroVADConfig struct {
	ModelPath       string
	Threshold       float32
	MinSilenceDurMs int
	SpeechPadMs     int
	Mode            VADMode
}

// SileroVADElement implements voice activity detection using Silero VAD
type SileroVADElement struct {
	*pipeline.BaseElement

	// Configuration
	modelPath       string
	threshold       float32
	minSilenceDurMs int
	speechPadMs     int
	mode            VADMode

	// VAD detector
	detector *speech.Detector

	// State management
	isSpeaking  bool
	audioBuffer []float32
	stateLock   sync.Mutex
	// Total processed samples (16kHz). Used to approximate audio position.
	processedSamples int64

	// Lifecycle management
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewSileroVADElement creates a new Silero VAD element
func NewSileroVADElement(config SileroVADConfig) (*SileroVADElement, error) {
	if config.ModelPath == "" {
		return nil, fmt.Errorf("model path is required")
	}

	if config.Threshold == 0 {
		config.Threshold = 0.5 // Default threshold
	}

	if config.MinSilenceDurMs == 0 {
		config.MinSilenceDurMs = 100 // Default 100ms
	}

	if config.SpeechPadMs == 0 {
		config.SpeechPadMs = 30 // Default 30ms
	}

	elem := &SileroVADElement{
		BaseElement:      pipeline.NewBaseElement("silero-vad-element", 100),
		modelPath:        config.ModelPath,
		threshold:        config.Threshold,
		minSilenceDurMs:  config.MinSilenceDurMs,
		speechPadMs:      config.SpeechPadMs,
		mode:             config.Mode,
		isSpeaking:       false,
		audioBuffer:      make([]float32, 0, 1024),
		processedSamples: 0,
	}

	// Register properties
	if err := elem.registerProperties(); err != nil {
		return nil, fmt.Errorf("failed to register properties: %w", err)
	}

	return elem, nil
}

// registerProperties registers configurable properties
func (e *SileroVADElement) registerProperties() error {
	props := []pipeline.PropertyDesc{
		{
			Name:     "threshold",
			Type:     reflect.TypeOf(float32(0)),
			Writable: true,
			Readable: true,
			Default:  e.threshold,
		},
		{
			Name:     "mode",
			Type:     reflect.TypeOf(int(0)),
			Writable: true,
			Readable: true,
			Default:  int(e.mode),
		},
		{
			Name:     "min-silence-ms",
			Type:     reflect.TypeOf(int(0)),
			Writable: true,
			Readable: true,
			Default:  e.minSilenceDurMs,
		},
		{
			Name:     "speech-pad-ms",
			Type:     reflect.TypeOf(int(0)),
			Writable: true,
			Readable: true,
			Default:  e.speechPadMs,
		},
	}

	for _, prop := range props {
		if err := e.RegisterProperty(prop); err != nil {
			return err
		}
	}

	return nil
}

// Init initializes the VAD detector
func (e *SileroVADElement) Init(ctx context.Context) error {
	detector, err := speech.NewDetector(speech.DetectorConfig{
		ModelPath:            e.modelPath,
		SampleRate:           16000, // Only support 16kHz
		Threshold:            e.threshold,
		MinSilenceDurationMs: e.minSilenceDurMs,
		SpeechPadMs:          e.speechPadMs,
	})
	if err != nil {
		return fmt.Errorf("failed to create VAD detector: %w", err)
	}

	e.detector = detector
	log.Printf("[SileroVAD] Initialized with threshold=%.2f, minSilence=%dms, speechPad=%dms, mode=%d",
		e.threshold, e.minSilenceDurMs, e.speechPadMs, e.mode)

	return nil
}

// Start starts the VAD element processing
func (e *SileroVADElement) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.processAudio(ctx)
	}()

	return nil
}

// Stop stops the VAD element and cleans up resources
func (e *SileroVADElement) Stop() error {
	if e.cancel != nil {
		e.cancel()
		e.wg.Wait()
		e.cancel = nil
	}

	if e.detector != nil {
		e.detector.Destroy()
		e.detector = nil
	}

	return nil
}

// processAudio is the main audio processing loop
func (e *SileroVADElement) processAudio(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-e.BaseElement.InChan:
			if msg.Type != pipeline.MsgTypeAudio {
				continue
			}

			if msg.AudioData == nil || len(msg.AudioData.Data) == 0 {
				continue
			}

			// Only process raw PCM audio
			if msg.AudioData.MediaType != pipeline.AudioMediaTypeRaw {
				log.Printf("[SileroVAD] Skipping non-raw audio: %s", msg.AudioData.MediaType)
				continue
			}

			// Verify sample rate is 16kHz
			if msg.AudioData.SampleRate != 16000 {
				log.Printf("[SileroVAD] Warning: Expected 16kHz audio, got %dHz. Please add AudioResampleElement before VAD.",
					msg.AudioData.SampleRate)
				continue
			}

			// Process the audio data
			e.handleAudioData(ctx, msg)
		}
	}
}

// handleAudioData processes a single audio message
func (e *SileroVADElement) handleAudioData(ctx context.Context, msg *pipeline.PipelineMessage) {
	// Convert byte data to normalized float32 samples in [-1, 1]
	samples := e.bytesToFloat32(msg.AudioData.Data)

	// Add samples to buffer
	e.stateLock.Lock()
	e.audioBuffer = append(e.audioBuffer, samples...)
	e.stateLock.Unlock()

	// Streaming adaptation for silero-vad-go Detect():
	// silero-vad-go is designed for batch processing. Its Detect() function loops:
	//   for i := 0; i < len(pcm)-windowSize; i += windowSize
	// and maintains internal state (currSample, triggered, tempEnd) across calls.
	//
	// For streaming, we accumulate ~0.5 seconds of audio (8000 samples @ 16kHz)
	// before calling Detect(). This reduces the frequency of "unexpected speech end"
	// errors that occur when speech end triggers without a matching start in the batch.
	const windowSize = 512
	const batchSize = 8000 // ~0.5 seconds at 16kHz

	for {
		e.stateLock.Lock()
		bufLen := len(e.audioBuffer)
		if bufLen < batchSize {
			e.stateLock.Unlock()
			break
		}

		// Process batchSize samples, keep windowSize overlap for continuity
		buf := make([]float32, batchSize)
		copy(buf, e.audioBuffer[:batchSize])
		e.audioBuffer = e.audioBuffer[batchSize-windowSize:]
		e.processedSamples += int64(batchSize - windowSize)
		e.stateLock.Unlock()

		segments, err := e.detector.Detect(buf)
		if err != nil {
			// "unexpected speech end" is expected in streaming mode when speech
			// end is detected without a matching start in the current batch.
			// This happens because internal state persists across Detect() calls.
			if err.Error() != "unexpected speech end" {
				log.Printf("[SileroVAD] Detection error: %v", err)
			}
			continue
		}
		if len(segments) > 0 {
			log.Printf("[SileroVAD] Detected %d segments in %d samples", len(segments), bufLen)
			for i, seg := range segments {
				log.Printf("[SileroVAD]   Segment %d: start=%.3fs, end=%.3fs", i, seg.SpeechStartAt, seg.SpeechEndAt)
			}
		}
		e.processSegments(ctx, msg, segments)
	}

	// Handle output based on mode
	switch e.mode {
	case VADModePassthrough:
		// Always pass through the audio
		select {
		case e.BaseElement.OutChan <- msg:
		case <-ctx.Done():
			return
		}

	case VADModeFilter:
		// Only pass through if speaking
		if e.isSpeaking {
			select {
			case e.BaseElement.OutChan <- msg:
			case <-ctx.Done():
				return
			}
		}
	}
}

// processSegments processes VAD detection segments
func (e *SileroVADElement) processSegments(ctx context.Context, msg *pipeline.PipelineMessage, segments []speech.Segment) {
	for _, segment := range segments {
		wasSpeaking := e.isSpeaking

		// A Segment is emitted when speech is triggered; startAt can be 0 due to padding clamp.
		if !wasSpeaking {
			// Speech started
			e.isSpeaking = true
			startMs := int(segment.SpeechStartAt * 1000)
			e.emitEvent(pipeline.EventVADSpeechStart, msg.SessionID, e.threshold, startMs)
			log.Printf("[SileroVAD] Speech started (startMs=%d)", startMs)
		}

		if segment.SpeechEndAt > 0 && e.isSpeaking {
			// Speech ended
			e.isSpeaking = false
			endMs := int(segment.SpeechEndAt * 1000)
			e.emitEvent(pipeline.EventVADSpeechEnd, msg.SessionID, e.threshold, endMs)
			log.Printf("[SileroVAD] Speech ended (endMs=%d)", endMs)
		}
	}
}

// emitEvent emits a VAD event to the bus
func (e *SileroVADElement) emitEvent(eventType pipeline.EventType, sessionID string, confidence float32, audioMs int) {
	if e.Bus() == nil {
		return
	}

	payload := VADEventPayload{
		SessionID:  sessionID,
		Confidence: confidence,
		Timestamp:  time.Now(),
		AudioMs:    audioMs,
	}

	event := pipeline.Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Payload:   payload,
	}

	e.Bus().Publish(event)
}

// bytesToFloat32 converts 16-bit PCM (little-endian) to normalized float32 in [-1, 1].
func (e *SileroVADElement) bytesToFloat32(data []byte) []float32 {
	n := len(data) / 2
	samples := make([]float32, n)
	for i := 0; i < n; i++ {
		v := int16(binary.LittleEndian.Uint16(data[i*2 : i*2+2]))
		samples[i] = float32(v) / 32768.0
	}
	return samples
}

// SetThreshold updates the VAD threshold
func (e *SileroVADElement) SetThreshold(threshold float32) error {
	if threshold < 0 || threshold > 1 {
		return fmt.Errorf("threshold must be between 0 and 1")
	}
	e.threshold = threshold
	if e.detector != nil {
		e.detector.Reset()
	}
	return nil
}

// GetIsSpeaking returns whether speech is currently detected
func (e *SileroVADElement) GetIsSpeaking() bool {
	e.stateLock.Lock()
	defer e.stateLock.Unlock()
	return e.isSpeaking
}
