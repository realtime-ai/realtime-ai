package asr

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/sashabaranov/go-openai"
)

// WhisperProvider implements the Provider interface using OpenAI's Whisper API.
type WhisperProvider struct {
	client *openai.Client
	mu     sync.RWMutex
}

// NewWhisperProvider creates a new OpenAI Whisper ASR provider.
// apiKey is the OpenAI API key. If empty, it will use OPENAI_API_KEY from environment.
func NewWhisperProvider(apiKey string) (*WhisperProvider, error) {
	if apiKey == "" {
		return nil, &Error{
			Code:    ErrCodeInvalidConfig,
			Message: "OpenAI API key is required",
		}
	}

	clientConfig := openai.DefaultConfig(apiKey)
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		clientConfig.BaseURL = baseURL
		log.Printf("[Whisper STT] Using BaseURL: %s", clientConfig.BaseURL)
	}
	client := openai.NewClientWithConfig(clientConfig)

	return &WhisperProvider{
		client: client,
	}, nil
}

// Name returns the provider name.
func (w *WhisperProvider) Name() string {
	return "openai-whisper"
}

// Recognize performs speech recognition on a complete audio segment.
func (w *WhisperProvider) Recognize(ctx context.Context, audio io.Reader, audioConfig AudioConfig, config RecognitionConfig) (*RecognitionResult, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	// Read all audio data
	audioData, err := io.ReadAll(audio)
	if err != nil {
		return nil, &Error{
			Code:    ErrCodeInvalidAudio,
			Message: "failed to read audio data",
			Err:     err,
		}
	}

	if len(audioData) == 0 {
		return nil, &Error{
			Code:    ErrCodeInvalidAudio,
			Message: "audio data is empty",
		}
	}

	// Convert audio to WAV format if it's raw PCM
	var fileBytes []byte
	if audioConfig.Encoding == "pcm" || audioConfig.Encoding == "" {
		fileBytes, err = convertPCMToWAV(audioData, audioConfig)
		if err != nil {
			return nil, &Error{
				Code:    ErrCodeInvalidAudio,
				Message: "failed to convert PCM to WAV",
				Err:     err,
			}
		}
	} else {
		fileBytes = audioData
	}

	// Prepare Whisper API request
	req := openai.AudioRequest{
		Model:    config.Model,
		FilePath: "audio.wav", // Filename hint for API
		Reader:   bytes.NewReader(fileBytes),
		Prompt:   config.Prompt,
		Language: config.Language,
	}

	if req.Model == "" {
		req.Model = openai.Whisper1 // Default to whisper-1
	}

	// Set temperature if specified
	if config.Temperature > 0 {
		req.Temperature = config.Temperature
	}

	// Call Whisper API
	startTime := time.Now()
	resp, err := w.client.CreateTranscription(ctx, req)
	if err != nil {
		return nil, &Error{
			Code:    ErrCodeProviderError,
			Message: "Whisper API request failed",
			Err:     err,
		}
	}

	duration := time.Since(startTime)

	result := &RecognitionResult{
		Text:       resp.Text,
		IsFinal:    true, // Whisper always returns final results
		Confidence: -1,   // Whisper API doesn't provide confidence scores
		Language:   config.Language,
		Duration:   duration,
		Timestamp:  time.Now(),
		Metadata: map[string]interface{}{
			"model": req.Model,
		},
	}

	// If language was auto-detected, include it in metadata
	if config.Language == "" || config.Language == "auto" {
		// Whisper doesn't return detected language in basic transcription
		// For language detection, would need to use verbose_json format
		result.Metadata["language_detection"] = "not available in basic mode"
	}

	return result, nil
}

// StreamingRecognize creates a streaming recognizer for continuous audio input.
func (w *WhisperProvider) StreamingRecognize(ctx context.Context, audioConfig AudioConfig, config RecognitionConfig) (StreamingRecognizer, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	recognizer := &whisperStreamingRecognizer{
		provider:    w,
		audioConfig: audioConfig,
		config:      config,
		resultsChan: make(chan *RecognitionResult, 10),
		audioChan:   make(chan []byte, 100),
		ctx:         ctx,
	}

	// Start processing goroutine
	go recognizer.processAudio()

	return recognizer, nil
}

// SupportsStreaming indicates if the provider supports streaming recognition.
func (w *WhisperProvider) SupportsStreaming() bool {
	// Whisper API doesn't natively support streaming, but we can simulate it
	// by buffering audio and sending chunks periodically
	return true
}

// SupportedLanguages returns a list of supported language codes.
func (w *WhisperProvider) SupportedLanguages() []string {
	// Whisper supports 99+ languages, returning empty to indicate all are supported
	// See: https://github.com/openai/whisper#available-models-and-languages
	return []string{}
}

// Close releases any resources held by the provider.
func (w *WhisperProvider) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	// No resources to clean up for Whisper provider
	return nil
}

// whisperStreamingRecognizer implements StreamingRecognizer for Whisper.
// Since Whisper doesn't support true streaming, this buffers audio and
// processes it in chunks, optionally triggered by VAD events.
type whisperStreamingRecognizer struct {
	provider    *WhisperProvider
	audioConfig AudioConfig
	config      RecognitionConfig
	resultsChan chan *RecognitionResult
	audioChan   chan []byte
	audioBuffer []byte
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
	closed      bool
}

// SendAudio sends audio data to the recognizer.
func (r *whisperStreamingRecognizer) SendAudio(ctx context.Context, audioData []byte) error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return &Error{
			Code:    ErrCodeProviderError,
			Message: "recognizer is closed",
		}
	}
	r.mu.Unlock()

	select {
	case r.audioChan <- audioData:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-r.ctx.Done():
		return r.ctx.Err()
	}
}

// Results returns a channel that receives recognition results.
func (r *whisperStreamingRecognizer) Results() <-chan *RecognitionResult {
	return r.resultsChan
}

// Close stops recognition and releases resources.
func (r *whisperStreamingRecognizer) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	r.closed = true
	close(r.audioChan)
	// resultsChan will be closed by processAudio goroutine
	if r.cancel != nil {
		r.cancel()
	}

	return nil
}

// processAudio continuously processes incoming audio data.
func (r *whisperStreamingRecognizer) processAudio() {
	defer close(r.resultsChan)

	ctx, cancel := context.WithCancel(r.ctx)
	r.cancel = cancel
	defer cancel()

	// Buffer audio until we have enough for recognition
	// Whisper works best with 1-30 second chunks
	const maxBufferSize = 16000 * 2 * 10 // 10 seconds at 16kHz, 16-bit PCM

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Process any remaining audio
			r.processBufferedAudio(ctx)
			return

		case audioData, ok := <-r.audioChan:
			if !ok {
				// Channel closed, process remaining audio
				r.processBufferedAudio(ctx)
				return
			}

			r.mu.Lock()
			r.audioBuffer = append(r.audioBuffer, audioData...)
			bufferSize := len(r.audioBuffer)
			r.mu.Unlock()

			// If buffer is large enough, process it
			if bufferSize >= maxBufferSize {
				r.processBufferedAudio(ctx)
			}

		case <-ticker.C:
			// Periodically process buffered audio if we have any
			r.mu.Lock()
			hasAudio := len(r.audioBuffer) > 0
			r.mu.Unlock()

			if hasAudio {
				r.processBufferedAudio(ctx)
			}
		}
	}
}

// processBufferedAudio sends buffered audio to Whisper API for recognition.
func (r *whisperStreamingRecognizer) processBufferedAudio(ctx context.Context) {
	r.mu.Lock()
	if len(r.audioBuffer) == 0 {
		r.mu.Unlock()
		return
	}

	// Minimum audio length check (e.g., 0.1 seconds)
	minSamples := r.audioConfig.SampleRate * r.audioConfig.Channels * (r.audioConfig.BitsPerSample / 8) / 10
	if len(r.audioBuffer) < minSamples {
		r.mu.Unlock()
		return
	}

	audioData := make([]byte, len(r.audioBuffer))
	copy(audioData, r.audioBuffer)
	r.audioBuffer = r.audioBuffer[:0] // Clear buffer
	r.mu.Unlock()

	// Send partial result if enabled
	if r.config.EnablePartialResults && len(audioData) > 0 {
		// For partial results, we could send an empty partial result
		// to indicate processing is happening
		select {
		case r.resultsChan <- &RecognitionResult{
			Text:       "",
			IsFinal:    false,
			Confidence: -1,
			Language:   r.config.Language,
			Timestamp:  time.Now(),
			Metadata: map[string]interface{}{
				"processing": true,
			},
		}:
		case <-ctx.Done():
			return
		}
	}

	// Recognize the audio
	reader := bytes.NewReader(audioData)
	result, err := r.provider.Recognize(ctx, reader, r.audioConfig, r.config)
	if err != nil {
		log.Printf("Whisper recognition error: %v", err)
		return
	}

	// Send result
	select {
	case r.resultsChan <- result:
	case <-ctx.Done():
		return
	}
}

// convertPCMToWAV converts raw PCM audio data to WAV format.
// This is needed because Whisper API expects audio files in standard formats.
func convertPCMToWAV(pcmData []byte, config AudioConfig) ([]byte, error) {
	var buf bytes.Buffer

	// WAV header
	// RIFF header
	buf.WriteString("RIFF")
	// File size (will be updated later)
	fileSize := uint32(36 + len(pcmData))
	binary.Write(&buf, binary.LittleEndian, fileSize)
	buf.WriteString("WAVE")

	// fmt sub-chunk
	buf.WriteString("fmt ")
	subChunk1Size := uint32(16)
	binary.Write(&buf, binary.LittleEndian, subChunk1Size)
	audioFormat := uint16(1) // PCM
	binary.Write(&buf, binary.LittleEndian, audioFormat)
	binary.Write(&buf, binary.LittleEndian, uint16(config.Channels))
	binary.Write(&buf, binary.LittleEndian, uint32(config.SampleRate))

	bitsPerSample := config.BitsPerSample
	if bitsPerSample == 0 {
		bitsPerSample = 16 // Default to 16-bit
	}

	byteRate := uint32(config.SampleRate * config.Channels * bitsPerSample / 8)
	binary.Write(&buf, binary.LittleEndian, byteRate)

	blockAlign := uint16(config.Channels * bitsPerSample / 8)
	binary.Write(&buf, binary.LittleEndian, blockAlign)
	binary.Write(&buf, binary.LittleEndian, uint16(bitsPerSample))

	// data sub-chunk
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(len(pcmData)))
	buf.Write(pcmData)

	return buf.Bytes(), nil
}
