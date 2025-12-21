package asr

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Qwen Realtime ASR WebSocket endpoint
	qwenRealtimeWSURL = "wss://dashscope.aliyuncs.com/api-ws/v1/realtime"

	// Default model
	qwenRealtimeDefaultModel = "qwen3-asr-flash-realtime"

	// Connection retry configuration
	maxRetryAttempts   = 3
	initialRetryDelay  = 1 * time.Second
	maxRetryDelay      = 10 * time.Second
	connectionTimeout  = 10 * time.Second
)

// QwenRealtimeProvider implements the Provider interface using Alibaba Cloud DashScope Qwen Realtime ASR API.
// It uses WebSocket for true streaming speech recognition.
type QwenRealtimeProvider struct {
	apiKey string
	model  string
	mu     sync.RWMutex
}

// QwenRealtimeConfig holds configuration for QwenRealtimeProvider.
type QwenRealtimeConfig struct {
	// APIKey is the DashScope API key (required)
	APIKey string

	// Model to use (default: "qwen3-asr-flash-realtime")
	Model string
}

// NewQwenRealtimeProvider creates a new Qwen Realtime ASR provider.
func NewQwenRealtimeProvider(config QwenRealtimeConfig) (*QwenRealtimeProvider, error) {
	if config.APIKey == "" {
		return nil, &Error{
			Code:    ErrCodeInvalidConfig,
			Message: "DashScope API key is required",
		}
	}

	model := config.Model
	if model == "" {
		model = qwenRealtimeDefaultModel
	}

	return &QwenRealtimeProvider{
		apiKey: config.APIKey,
		model:  model,
	}, nil
}

// Name returns the provider name.
func (p *QwenRealtimeProvider) Name() string {
	return "qwen-realtime"
}

// Recognize performs speech recognition on a complete audio segment.
// Note: For Qwen Realtime, we use streaming internally even for batch recognition.
func (p *QwenRealtimeProvider) Recognize(ctx context.Context, audio io.Reader, audioConfig AudioConfig, config RecognitionConfig) (*RecognitionResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

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
	if qr, ok := recognizer.(*qwenRealtimeStreamingRecognizer); ok {
		if err := qr.Commit(ctx); err != nil {
			return nil, err
		}
	}

	// Wait for final result
	var finalResult *RecognitionResult
	for result := range recognizer.Results() {
		if result.IsFinal {
			finalResult = result
			break
		}
	}

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
func (p *QwenRealtimeProvider) StreamingRecognize(ctx context.Context, audioConfig AudioConfig, config RecognitionConfig) (StreamingRecognizer, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	recognizer := &qwenRealtimeStreamingRecognizer{
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
func (p *QwenRealtimeProvider) SupportsStreaming() bool {
	return true
}

// SupportedLanguages returns a list of supported language codes.
func (p *QwenRealtimeProvider) SupportedLanguages() []string {
	// Qwen ASR supports Chinese and other languages
	return []string{"zh", "en", "ja", "ko", "yue", "auto"}
}

// Close releases any resources held by the provider.
func (p *QwenRealtimeProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return nil
}

// qwenRealtimeStreamingRecognizer implements StreamingRecognizer for Qwen Realtime.
type qwenRealtimeStreamingRecognizer struct {
	provider    *QwenRealtimeProvider
	audioConfig AudioConfig
	config      RecognitionConfig
	resultsChan chan *RecognitionResult
	sendChan    chan []byte
	commitChan  chan struct{}
	conn        *websocket.Conn
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	mu          sync.Mutex
	closed      atomic.Bool
	sessionReady atomic.Bool
	startTime   time.Time
}

// Qwen Realtime ASR event types
type qwenEvent struct {
	EventID string `json:"event_id,omitempty"`
	Type    string `json:"type"`
}

type qwenSessionUpdateEvent struct {
	EventID string       `json:"event_id,omitempty"`
	Type    string       `json:"type"`
	Session qwenSession  `json:"session"`
}

type qwenSession struct {
	Modalities              []string                    `json:"modalities"`
	InputAudioFormat        string                      `json:"input_audio_format"`
	SampleRate              int                         `json:"sample_rate"`
	InputAudioTranscription qwenAudioTranscription      `json:"input_audio_transcription"`
	TurnDetection           *qwenTurnDetection          `json:"turn_detection"`
}

type qwenAudioTranscription struct {
	Language string `json:"language"`
}

type qwenTurnDetection struct {
	Type              string  `json:"type"`
	Threshold         float64 `json:"threshold"`
	SilenceDurationMs int     `json:"silence_duration_ms"`
}

type qwenAudioAppendEvent struct {
	EventID string `json:"event_id,omitempty"`
	Type    string `json:"type"`
	Audio   string `json:"audio"` // Base64 encoded
}

type qwenAudioCommitEvent struct {
	EventID string `json:"event_id,omitempty"`
	Type    string `json:"type"`
}

// Response event types
type qwenSessionUpdatedEvent struct {
	Type    string `json:"type"`
	Session struct {
		ID string `json:"id"`
	} `json:"session"`
}

type qwenTranscriptionTextEvent struct {
	Type     string `json:"type"`
	Language string `json:"language"`
	Emotion  string `json:"emotion"`
	Text     string `json:"text"`
	Stash    string `json:"stash"`
	ItemID   string `json:"item_id,omitempty"`
}

type qwenTranscriptionCompletedEvent struct {
	Type       string `json:"type"`
	Transcript string `json:"transcript"`
	ItemID     string `json:"item_id,omitempty"`
}

type qwenErrorEvent struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// connect establishes WebSocket connection with retry logic.
func (r *qwenRealtimeStreamingRecognizer) connect(ctx context.Context) error {
	r.ctx, r.cancel = context.WithCancel(ctx)
	r.startTime = time.Now()

	var lastErr error
	retryDelay := initialRetryDelay

	for attempt := 0; attempt < maxRetryAttempts; attempt++ {
		if err := r.doConnect(); err != nil {
			lastErr = err
			log.Printf("[QwenRealtime] Connection attempt %d/%d failed: %v", attempt+1, maxRetryAttempts, err)

			if attempt < maxRetryAttempts-1 {
				select {
				case <-time.After(retryDelay):
					retryDelay *= 2
					if retryDelay > maxRetryDelay {
						retryDelay = maxRetryDelay
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
		Message: fmt.Sprintf("failed to connect after %d attempts", maxRetryAttempts),
		Err:     lastErr,
	}
}

// doConnect performs the actual WebSocket connection.
func (r *qwenRealtimeStreamingRecognizer) doConnect() error {
	url := fmt.Sprintf("%s?model=%s", qwenRealtimeWSURL, r.provider.model)
	log.Printf("[QwenRealtime] Connecting to %s", url)

	dialer := websocket.Dialer{
		HandshakeTimeout: connectionTimeout,
	}

	headers := map[string][]string{
		"Authorization": {fmt.Sprintf("Bearer %s", r.provider.apiKey)},
		"OpenAI-Beta":   {"realtime=v1"},
	}

	conn, _, err := dialer.DialContext(r.ctx, url, headers)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	r.conn = conn
	log.Printf("[QwenRealtime] WebSocket connected")

	// Start message handlers
	r.wg.Add(2)
	go r.readLoop()
	go r.writeLoop()

	// Send session update
	r.sendSessionUpdate()

	return nil
}

// sendSessionUpdate sends session.update event to configure the session.
func (r *qwenRealtimeStreamingRecognizer) sendSessionUpdate() {
	language := r.normalizeLanguage(r.config.Language)

	sampleRate := r.audioConfig.SampleRate
	if sampleRate == 0 {
		sampleRate = 16000
	}

	event := qwenSessionUpdateEvent{
		EventID: fmt.Sprintf("session_%d", time.Now().UnixNano()),
		Type:    "session.update",
		Session: qwenSession{
			Modalities:       []string{"text"},
			InputAudioFormat: "pcm",
			SampleRate:       sampleRate,
			InputAudioTranscription: qwenAudioTranscription{
				Language: language,
			},
			// Manual mode - VAD disabled for explicit control
			TurnDetection: nil,
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("[QwenRealtime] Failed to marshal session update: %v", err)
		return
	}

	r.mu.Lock()
	if r.conn != nil {
		if err := r.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[QwenRealtime] Failed to send session update: %v", err)
		} else {
			log.Printf("[QwenRealtime] Sent session.update")
		}
	}
	r.mu.Unlock()
}

// readLoop handles incoming WebSocket messages.
func (r *qwenRealtimeStreamingRecognizer) readLoop() {
	defer r.wg.Done()
	defer r.Close()

	for {
		select {
		case <-r.ctx.Done():
			return
		default:
		}

		_, message, err := r.conn.ReadMessage()
		if err != nil {
			if !r.closed.Load() && !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("[QwenRealtime] WebSocket read error: %v", err)
			}
			return
		}

		r.handleMessage(message)
	}
}

// writeLoop handles outgoing messages.
func (r *qwenRealtimeStreamingRecognizer) writeLoop() {
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
				// Wait for session to be ready
				continue
			}

			event := qwenAudioAppendEvent{
				EventID: fmt.Sprintf("audio_%d", time.Now().UnixNano()),
				Type:    "input_audio_buffer.append",
				Audio:   base64.StdEncoding.EncodeToString(audioData),
			}

			data, err := json.Marshal(event)
			if err != nil {
				log.Printf("[QwenRealtime] Failed to marshal audio append: %v", err)
				continue
			}

			r.mu.Lock()
			if r.conn != nil {
				if err := r.conn.WriteMessage(websocket.TextMessage, data); err != nil {
					log.Printf("[QwenRealtime] Failed to send audio: %v", err)
				}
			}
			r.mu.Unlock()

		case <-r.commitChan:
			if !r.sessionReady.Load() {
				continue
			}

			event := qwenAudioCommitEvent{
				EventID: fmt.Sprintf("commit_%d", time.Now().UnixNano()),
				Type:    "input_audio_buffer.commit",
			}

			data, err := json.Marshal(event)
			if err != nil {
				log.Printf("[QwenRealtime] Failed to marshal commit: %v", err)
				continue
			}

			r.mu.Lock()
			if r.conn != nil {
				if err := r.conn.WriteMessage(websocket.TextMessage, data); err != nil {
					log.Printf("[QwenRealtime] Failed to send commit: %v", err)
				} else {
					log.Printf("[QwenRealtime] Audio buffer committed")
				}
			}
			r.mu.Unlock()
		}
	}
}

// handleMessage processes incoming WebSocket messages.
func (r *qwenRealtimeStreamingRecognizer) handleMessage(data []byte) {
	var event qwenEvent
	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("[QwenRealtime] Failed to parse event: %v", err)
		return
	}

	switch event.Type {
	case "session.updated":
		r.handleSessionUpdated(data)

	case "session.created":
		log.Printf("[QwenRealtime] Session created")

	case "conversation.item.created":
		// This event is fired when a new item is added to the conversation
		// For STT-only use case, we can ignore it or log it at debug level
		log.Printf("[QwenRealtime] Conversation item created")

	case "input_audio_buffer.committed":
		log.Printf("[QwenRealtime] Audio buffer committed, waiting for transcription...")

	case "conversation.item.input_audio_transcription.text":
		r.handleTranscriptionText(data)

	case "conversation.item.input_audio_transcription.completed":
		r.handleTranscriptionCompleted(data)

	case "error":
		r.handleError(data)

	default:
		log.Printf("[QwenRealtime] Unhandled event type: %s", event.Type)
	}
}

// handleSessionUpdated handles session.updated event.
func (r *qwenRealtimeStreamingRecognizer) handleSessionUpdated(data []byte) {
	var event qwenSessionUpdatedEvent
	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("[QwenRealtime] Failed to parse session.updated: %v", err)
		return
	}

	log.Printf("[QwenRealtime] Session configured successfully (ID: %s)", event.Session.ID)
	r.sessionReady.Store(true)
	r.startTime = time.Now()
}

// handleTranscriptionText handles partial transcription text events.
func (r *qwenRealtimeStreamingRecognizer) handleTranscriptionText(data []byte) {
	var event qwenTranscriptionTextEvent
	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("[QwenRealtime] Failed to parse transcription text: %v", err)
		return
	}

	if event.Text == "" {
		return
	}

	log.Printf("[QwenRealtime] Partial: %s", event.Text)

	result := &RecognitionResult{
		Text:       event.Text,
		IsFinal:    false,
		Confidence: 0.8, // Partial results have lower confidence
		Language:   event.Language,
		Duration:   time.Since(r.startTime),
		Timestamp:  time.Now(),
		Metadata: map[string]interface{}{
			"emotion": event.Emotion,
			"stash":   event.Stash,
		},
	}

	select {
	case r.resultsChan <- result:
	case <-r.ctx.Done():
	default:
		log.Printf("[QwenRealtime] Results channel full, dropping partial result")
	}
}

// handleTranscriptionCompleted handles final transcription completed events.
func (r *qwenRealtimeStreamingRecognizer) handleTranscriptionCompleted(data []byte) {
	var event qwenTranscriptionCompletedEvent
	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("[QwenRealtime] Failed to parse transcription completed: %v", err)
		return
	}

	processingTime := time.Since(r.startTime)
	log.Printf("[QwenRealtime] Final: %s (processing time: %v)", event.Transcript, processingTime)

	result := &RecognitionResult{
		Text:       event.Transcript,
		IsFinal:    true,
		Confidence: 0.95,
		Language:   r.config.Language,
		Duration:   processingTime,
		Timestamp:  time.Now(),
		Metadata: map[string]interface{}{
			"item_id": event.ItemID,
			"model":   r.provider.model,
		},
	}

	select {
	case r.resultsChan <- result:
	case <-r.ctx.Done():
	default:
		log.Printf("[QwenRealtime] Results channel full, dropping final result")
	}

	// Reset start time for next utterance
	r.startTime = time.Now()
}

// handleError handles error events.
func (r *qwenRealtimeStreamingRecognizer) handleError(data []byte) {
	var event qwenErrorEvent
	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("[QwenRealtime] Failed to parse error event: %v", err)
		return
	}

	log.Printf("[QwenRealtime] Error: %s - %s", event.Error.Code, event.Error.Message)
}

// SendAudio sends audio data to the recognizer.
func (r *qwenRealtimeStreamingRecognizer) SendAudio(ctx context.Context, audioData []byte) error {
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
// This is specific to Qwen Realtime when using manual commit mode (VAD disabled).
func (r *qwenRealtimeStreamingRecognizer) Commit(ctx context.Context) error {
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
func (r *qwenRealtimeStreamingRecognizer) Results() <-chan *RecognitionResult {
	return r.resultsChan
}

// Close stops recognition and releases resources.
func (r *qwenRealtimeStreamingRecognizer) Close() error {
	if r.closed.Swap(true) {
		return nil // Already closed
	}

	log.Printf("[QwenRealtime] Closing recognizer")

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

	log.Printf("[QwenRealtime] Recognizer closed")
	return nil
}

// normalizeLanguage normalizes language code to Qwen format.
func (r *qwenRealtimeStreamingRecognizer) normalizeLanguage(language string) string {
	if language == "" {
		return "zh" // Default to Chinese
	}

	// Extract simple language code (e.g., 'zh-CN' -> 'zh')
	lang := language
	for i, c := range language {
		if c == '-' || c == '_' {
			lang = language[:i]
			break
		}
	}

	// Supported languages
	supported := map[string]bool{
		"zh":   true,
		"en":   true,
		"ja":   true,
		"ko":   true,
		"yue":  true, // Cantonese
		"auto": true,
	}

	if supported[lang] {
		return lang
	}

	log.Printf("[QwenRealtime] Language '%s' may not be supported, using 'zh'", language)
	return "zh"
}

// QwenRealtimeStreamingRecognizer interface for accessing Commit method
type QwenRealtimeStreamingRecognizer interface {
	StreamingRecognizer
	// Commit commits the audio buffer to trigger final transcription.
	// This is specific to Qwen Realtime when using manual commit mode.
	Commit(ctx context.Context) error
}

// Ensure qwenRealtimeStreamingRecognizer implements QwenRealtimeStreamingRecognizer
var _ QwenRealtimeStreamingRecognizer = (*qwenRealtimeStreamingRecognizer)(nil)

// IsQwenRealtimeRecognizer checks if a recognizer is a Qwen Realtime recognizer
// and returns it casted to QwenRealtimeStreamingRecognizer if so.
func IsQwenRealtimeRecognizer(r StreamingRecognizer) (QwenRealtimeStreamingRecognizer, bool) {
	qr, ok := r.(*qwenRealtimeStreamingRecognizer)
	return qr, ok
}

// ConvertPCMToWAVForQwen converts raw PCM audio to WAV format for Qwen.
// This is provided for compatibility but Qwen Realtime accepts raw PCM directly.
func ConvertPCMToWAVForQwen(pcmData []byte, sampleRate, channels, bitsPerSample int) ([]byte, error) {
	// For Qwen Realtime, we send raw PCM, but this helper is available if needed
	return convertPCMToWAV(pcmData, AudioConfig{
		SampleRate:    sampleRate,
		Channels:      channels,
		BitsPerSample: bitsPerSample,
	})
}
