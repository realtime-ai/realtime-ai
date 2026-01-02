// ElevenLabs Scribe V2 Realtime ASR Provider
//
// This package implements real-time speech recognition using ElevenLabs Scribe V2 API.
// It uses WebSocket for true streaming speech recognition with support for partial
// and committed transcripts.
//
// Features:
// - Real-time streaming ASR via WebSocket
// - Partial (interim) and final transcript support
// - Manual commit strategy for VAD integration
// - Connection retry with exponential backoff
// - Only supports 16kHz mono audio

package asr

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// ElevenLabs Scribe V2 Realtime WebSocket endpoint
	elevenlabsRealtimeWSURL = "wss://api.elevenlabs.io/v1/speech-to-text/realtime"

	// Default model
	elevenlabsDefaultModel = "scribe_v2_realtime"

	// Required sample rate (ElevenLabs only supports 16kHz)
	elevenlabsRequiredSampleRate = 16000

	// Connection configuration
	elevenlabsMaxRetryAttempts  = 3
	elevenlabsInitialRetryDelay = 1 * time.Second
	elevenlabsMaxRetryDelay     = 4 * time.Second
	elevenlabsConnectionTimeout = 10 * time.Second
)

// ElevenLabsProvider implements the Provider interface using ElevenLabs Scribe V2 Realtime API.
// It uses WebSocket for true streaming speech recognition.
type ElevenLabsProvider struct {
	apiKey string
	model  string
	mu     sync.RWMutex
}

// ElevenLabsConfig holds configuration for ElevenLabsProvider.
type ElevenLabsConfig struct {
	// APIKey is the ElevenLabs API key (required)
	APIKey string

	// Model to use (default: "scribe_v2_realtime")
	Model string
}

// NewElevenLabsProvider creates a new ElevenLabs Realtime ASR provider.
func NewElevenLabsProvider(config ElevenLabsConfig) (*ElevenLabsProvider, error) {
	if config.APIKey == "" {
		return nil, &Error{
			Code:    ErrCodeInvalidConfig,
			Message: "ElevenLabs API key is required",
		}
	}

	model := config.Model
	if model == "" {
		model = elevenlabsDefaultModel
	}

	return &ElevenLabsProvider{
		apiKey: config.APIKey,
		model:  model,
	}, nil
}

// Name returns the provider name.
func (p *ElevenLabsProvider) Name() string {
	return "elevenlabs"
}

// Recognize performs speech recognition on a complete audio segment.
func (p *ElevenLabsProvider) Recognize(ctx context.Context, audio io.Reader, audioConfig AudioConfig, config RecognitionConfig) (*RecognitionResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Validate sample rate
	if audioConfig.SampleRate != elevenlabsRequiredSampleRate {
		return nil, &Error{
			Code:    ErrCodeInvalidConfig,
			Message: fmt.Sprintf("ElevenLabs ASR requires 16kHz sample rate, got %dHz", audioConfig.SampleRate),
		}
	}

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

	// Create a streaming recognizer
	recognizer, err := p.StreamingRecognize(ctx, audioConfig, config)
	if err != nil {
		return nil, err
	}
	defer recognizer.Close()

	// Send all audio
	if err := recognizer.SendAudio(ctx, audioData); err != nil {
		return nil, err
	}

	// Commit to trigger final transcription
	if er, ok := recognizer.(*elevenlabsStreamingRecognizer); ok {
		if err := er.Commit(ctx); err != nil {
			return nil, err
		}
	}

	// Wait for final result with timeout
	timeout := time.After(30 * time.Second)
	var finalResult *RecognitionResult
	for {
		select {
		case result, ok := <-recognizer.Results():
			if !ok {
				goto done
			}
			if result.IsFinal {
				finalResult = result
				goto done
			}
		case <-timeout:
			goto done
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

done:
	if finalResult == nil {
		return &RecognitionResult{
			Text:       "",
			IsFinal:    true,
			Confidence: -1,
			Timestamp:  time.Now(),
		}, nil
	}

	return finalResult, nil
}

// StreamingRecognize creates a streaming recognizer for continuous audio input.
func (p *ElevenLabsProvider) StreamingRecognize(ctx context.Context, audioConfig AudioConfig, config RecognitionConfig) (StreamingRecognizer, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Validate sample rate
	if audioConfig.SampleRate != elevenlabsRequiredSampleRate {
		return nil, &Error{
			Code:    ErrCodeInvalidConfig,
			Message: fmt.Sprintf("ElevenLabs ASR requires 16kHz sample rate, got %dHz", audioConfig.SampleRate),
		}
	}

	recognizer := &elevenlabsStreamingRecognizer{
		provider:    p,
		audioConfig: audioConfig,
		config:      config,
		resultsChan: make(chan *RecognitionResult, 10),
		sendChan:    make(chan []byte, 100),
		commitChan:  make(chan struct{}, 1),
	}

	// Start connection
	if err := recognizer.connect(ctx); err != nil {
		return nil, err
	}

	return recognizer, nil
}

// SupportsStreaming indicates if the provider supports streaming recognition.
func (p *ElevenLabsProvider) SupportsStreaming() bool {
	return true
}

// SupportedLanguages returns a list of supported language codes.
func (p *ElevenLabsProvider) SupportedLanguages() []string {
	// ElevenLabs Scribe supports many languages
	return []string{
		"en", "zh", "es", "fr", "de", "it", "pt", "ru", "ja", "ko",
		"ar", "hi", "nl", "pl", "tr", "vi", "th", "id", "auto",
	}
}

// Close releases any resources held by the provider.
func (p *ElevenLabsProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return nil
}

// elevenlabsStreamingRecognizer implements StreamingRecognizer for ElevenLabs.
type elevenlabsStreamingRecognizer struct {
	provider     *ElevenLabsProvider
	audioConfig  AudioConfig
	config       RecognitionConfig
	resultsChan  chan *RecognitionResult
	sendChan     chan []byte
	commitChan   chan struct{}
	conn         *websocket.Conn
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.Mutex
	closed       atomic.Bool
	sessionReady atomic.Bool
	startTime    time.Time
	sentenceID   string
}

// ElevenLabs message types
type elevenlabsMessage struct {
	MessageType string `json:"message_type"`
	Text        string `json:"text,omitempty"`
	StartTime   *int   `json:"start_time,omitempty"`
	EndTime     *int   `json:"end_time,omitempty"`
	Confidence  *float32 `json:"confidence,omitempty"`
	Words       []elevenlabsWord `json:"words,omitempty"`
	Error       *elevenlabsError `json:"error,omitempty"`
}

type elevenlabsWord struct {
	Text       string  `json:"text"`
	StartTime  int     `json:"start_time"`
	EndTime    int     `json:"end_time"`
	Confidence float32 `json:"confidence"`
}

type elevenlabsError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type elevenlabsAudioChunk struct {
	MessageType string `json:"message_type"`
	AudioBase64 string `json:"audio_base_64"`
	Commit      bool   `json:"commit"`
	SampleRate  int    `json:"sample_rate"`
}

// connect establishes WebSocket connection with retry logic.
func (r *elevenlabsStreamingRecognizer) connect(ctx context.Context) error {
	r.ctx, r.cancel = context.WithCancel(ctx)
	r.startTime = time.Now()
	r.sentenceID = fmt.Sprintf("s_%d", time.Now().UnixNano())

	var lastErr error
	retryDelay := elevenlabsInitialRetryDelay

	for attempt := 0; attempt < elevenlabsMaxRetryAttempts; attempt++ {
		if err := r.doConnect(); err != nil {
			lastErr = err
			log.Printf("[ElevenLabs] Connection attempt %d/%d failed: %v", attempt+1, elevenlabsMaxRetryAttempts, err)

			if attempt < elevenlabsMaxRetryAttempts-1 {
				select {
				case <-time.After(retryDelay):
					retryDelay *= 2
					if retryDelay > elevenlabsMaxRetryDelay {
						retryDelay = elevenlabsMaxRetryDelay
					}
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			continue
		}

		// Successfully connected
		return nil
	}

	return &Error{
		Code:    ErrCodeNetworkError,
		Message: fmt.Sprintf("failed to connect after %d attempts", elevenlabsMaxRetryAttempts),
		Err:     lastErr,
	}
}

// doConnect performs the actual WebSocket connection.
func (r *elevenlabsStreamingRecognizer) doConnect() error {
	// Build WebSocket URL with query parameters
	params := url.Values{}
	params.Set("model_id", r.provider.model)
	params.Set("commit_strategy", "manual")

	// Add language_code if specified
	if r.config.Language != "" && r.config.Language != "auto" {
		languageCode := r.normalizeLanguageCode(r.config.Language)
		params.Set("language_code", languageCode)
		log.Printf("[ElevenLabs] Using language_code: %s", languageCode)
	}

	wsURL := fmt.Sprintf("%s?%s", elevenlabsRealtimeWSURL, params.Encode())
	log.Printf("[ElevenLabs] Connecting to %s", wsURL)

	dialer := websocket.Dialer{
		HandshakeTimeout: elevenlabsConnectionTimeout,
	}

	headers := map[string][]string{
		"xi-api-key": {r.provider.apiKey},
	}

	conn, _, err := dialer.DialContext(r.ctx, wsURL, headers)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	r.conn = conn
	log.Printf("[ElevenLabs] WebSocket connected")

	// Start message handlers
	r.wg.Add(2)
	go r.readLoop()
	go r.writeLoop()

	// Wait for session_started message
	select {
	case <-time.After(elevenlabsConnectionTimeout):
		r.Close()
		return fmt.Errorf("session start timeout")
	case <-r.ctx.Done():
		return r.ctx.Err()
	default:
		// Give some time for session_started
		for i := 0; i < 100; i++ {
			if r.sessionReady.Load() {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	return nil
}

// readLoop handles incoming WebSocket messages.
func (r *elevenlabsStreamingRecognizer) readLoop() {
	defer r.wg.Done()

	for {
		select {
		case <-r.ctx.Done():
			return
		default:
		}

		_, message, err := r.conn.ReadMessage()
		if err != nil {
			if !r.closed.Load() && !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("[ElevenLabs] WebSocket read error: %v", err)
			}
			return
		}

		r.handleMessage(message)
	}
}

// writeLoop handles outgoing messages.
func (r *elevenlabsStreamingRecognizer) writeLoop() {
	defer r.wg.Done()

	for {
		select {
		case <-r.ctx.Done():
			return

		case audioData, ok := <-r.sendChan:
			if !ok {
				return
			}

			if !r.sessionReady.Load() {
				log.Printf("[ElevenLabs] Session not ready, queuing audio")
				continue
			}

			r.sendAudioChunk(audioData, false)

		case <-r.commitChan:
			if !r.sessionReady.Load() {
				continue
			}

			// Send empty audio with commit=true
			r.sendAudioChunk([]byte{}, true)
			log.Printf("[ElevenLabs] Sent commit")
		}
	}
}

// sendAudioChunk sends an audio chunk to the WebSocket.
func (r *elevenlabsStreamingRecognizer) sendAudioChunk(audioData []byte, commit bool) {
	chunk := elevenlabsAudioChunk{
		MessageType: "input_audio_chunk",
		AudioBase64: base64.StdEncoding.EncodeToString(audioData),
		Commit:      commit,
		SampleRate:  r.audioConfig.SampleRate,
	}

	data, err := json.Marshal(chunk)
	if err != nil {
		log.Printf("[ElevenLabs] Failed to marshal audio chunk: %v", err)
		return
	}

	r.mu.Lock()
	if r.conn != nil {
		if err := r.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[ElevenLabs] Failed to send audio: %v", err)
		}
	}
	r.mu.Unlock()
}

// handleMessage processes incoming WebSocket messages.
func (r *elevenlabsStreamingRecognizer) handleMessage(data []byte) {
	var msg elevenlabsMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("[ElevenLabs] Failed to parse message: %v", err)
		return
	}

	switch msg.MessageType {
	case "session_started":
		log.Printf("[ElevenLabs] Session started")
		r.sessionReady.Store(true)
		r.startTime = time.Now()

	case "partial_transcript":
		if msg.Text != "" {
			confidence := float32(0.8)
			if msg.Confidence != nil {
				confidence = *msg.Confidence
			}

			result := &RecognitionResult{
				Text:       msg.Text,
				IsFinal:    false,
				Confidence: confidence,
				Language:   r.config.Language,
				Duration:   time.Since(r.startTime),
				Timestamp:  time.Now(),
				Metadata: map[string]interface{}{
					"sentence_id": r.sentenceID,
				},
			}

			log.Printf("[ElevenLabs] Partial: %s", msg.Text)

			select {
			case r.resultsChan <- result:
			case <-r.ctx.Done():
			default:
				log.Printf("[ElevenLabs] Results channel full, dropping partial")
			}
		}

	case "committed_transcript", "committed_transcript_with_timestamps":
		if msg.Text != "" {
			confidence := float32(0.95)
			if msg.Confidence != nil {
				confidence = *msg.Confidence
			}

			result := &RecognitionResult{
				Text:       msg.Text,
				IsFinal:    true,
				Confidence: confidence,
				Language:   r.config.Language,
				Duration:   time.Since(r.startTime),
				Timestamp:  time.Now(),
				Metadata: map[string]interface{}{
					"sentence_id": r.sentenceID,
					"words":       msg.Words,
				},
			}

			log.Printf("[ElevenLabs] Final: %s", msg.Text)

			select {
			case r.resultsChan <- result:
			case <-r.ctx.Done():
			default:
				log.Printf("[ElevenLabs] Results channel full, dropping final")
			}

			// Generate new sentence ID for next utterance
			r.sentenceID = fmt.Sprintf("s_%d", time.Now().UnixNano())
			r.startTime = time.Now()
		}

	case "error":
		if msg.Error != nil {
			log.Printf("[ElevenLabs] Error: %s - %s", msg.Error.Code, msg.Error.Message)
		}

	default:
		log.Printf("[ElevenLabs] Unknown message type: %s", msg.MessageType)
	}
}

// SendAudio sends audio data to the recognizer.
func (r *elevenlabsStreamingRecognizer) SendAudio(ctx context.Context, audioData []byte) error {
	if r.closed.Load() {
		return &Error{
			Code:    ErrCodeProviderError,
			Message: "recognizer is closed",
		}
	}

	select {
	case r.sendChan <- audioData:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-r.ctx.Done():
		return r.ctx.Err()
	}
}

// Commit commits the audio buffer to trigger final transcription.
func (r *elevenlabsStreamingRecognizer) Commit(ctx context.Context) error {
	if r.closed.Load() {
		return &Error{
			Code:    ErrCodeProviderError,
			Message: "recognizer is closed",
		}
	}

	select {
	case r.commitChan <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-r.ctx.Done():
		return r.ctx.Err()
	}
}

// Results returns a channel that receives recognition results.
func (r *elevenlabsStreamingRecognizer) Results() <-chan *RecognitionResult {
	return r.resultsChan
}

// Close stops recognition and releases resources.
func (r *elevenlabsStreamingRecognizer) Close() error {
	if r.closed.Swap(true) {
		return nil // Already closed
	}

	log.Printf("[ElevenLabs] Closing recognizer")

	if r.cancel != nil {
		r.cancel()
	}

	r.mu.Lock()
	if r.conn != nil {
		r.conn.Close()
		r.conn = nil
	}
	r.mu.Unlock()

	// Close channels
	close(r.sendChan)
	close(r.commitChan)

	// Wait for goroutines
	r.wg.Wait()

	// Close results channel
	close(r.resultsChan)

	log.Printf("[ElevenLabs] Recognizer closed")
	return nil
}

// normalizeLanguageCode converts language code to ISO 639-1 format.
func (r *elevenlabsStreamingRecognizer) normalizeLanguageCode(language string) string {
	// Mapping of common language formats to ISO 639-1 codes
	languageMap := map[string]string{
		"zh-CN": "zh",
		"zh-TW": "zh",
		"en-US": "en",
		"en-GB": "en",
		"ja-JP": "ja",
		"ko-KR": "ko",
		"es-ES": "es",
		"fr-FR": "fr",
		"de-DE": "de",
		"it-IT": "it",
		"pt-BR": "pt",
		"pt-PT": "pt",
		"ru-RU": "ru",
		"ar-SA": "ar",
		"hi-IN": "hi",
	}

	if mapped, ok := languageMap[language]; ok {
		return mapped
	}

	// Extract ISO 639-1 code from formats like 'zh-CN' -> 'zh'
	if len(language) >= 2 && (len(language) == 2 || language[2] == '-' || language[2] == '_') {
		return language[:2]
	}

	return language
}

// ElevenLabsStreamingRecognizer interface for accessing Commit method
type ElevenLabsStreamingRecognizer interface {
	StreamingRecognizer
	// Commit commits the audio buffer to trigger final transcription.
	Commit(ctx context.Context) error
}

// Ensure elevenlabsStreamingRecognizer implements ElevenLabsStreamingRecognizer
var _ ElevenLabsStreamingRecognizer = (*elevenlabsStreamingRecognizer)(nil)

// IsElevenLabsRecognizer checks if a recognizer is an ElevenLabs recognizer.
func IsElevenLabsRecognizer(r StreamingRecognizer) (ElevenLabsStreamingRecognizer, bool) {
	er, ok := r.(*elevenlabsStreamingRecognizer)
	return er, ok
}
