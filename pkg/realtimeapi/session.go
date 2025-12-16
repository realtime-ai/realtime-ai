package realtimeapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi/bridge"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi/events"
)

// Session represents a realtime API session.
type Session struct {
	ID     string
	Config events.Session

	// Conversation management
	Conversation *Conversation

	// Audio buffer
	AudioBuffer *AudioBuffer

	// Pipeline for AI processing
	Pipeline *pipeline.Pipeline

	// EventBridge for pipeline-to-WebSocket event translation
	EventBridge *bridge.EventBridge

	// Transport abstraction (WebSocket or DataChannel)
	transport Transport

	// Legacy WebSocket connection (for backward compatibility)
	conn *websocket.Conn

	// Audio mode: true if audio is transmitted via RTP (WebRTC mode)
	audioViaRTP bool

	// Event channels
	eventChan chan events.ServerEvent

	// Context for lifecycle management
	ctx    context.Context
	cancel context.CancelFunc

	// Synchronization
	mu       sync.RWMutex
	wg       sync.WaitGroup
	closed   bool
	closedCh chan struct{}

	// Callbacks
	onClose func(session *Session)
}

// SessionConfig holds the configuration for creating a new session.
type SessionConfig struct {
	Model             string
	Modalities        []events.Modality
	Voice             string
	Instructions      string
	InputAudioFormat  events.AudioFormat
	OutputAudioFormat events.AudioFormat
	TurnDetection     *events.TurnDetection
	Temperature       float64
	MaxOutputTokens   int
}

// DefaultSessionConfig returns the default session configuration.
func DefaultSessionConfig() SessionConfig {
	createResponse := true
	return SessionConfig{
		Model:             "gemini-2.0-flash",
		Modalities:        []events.Modality{events.ModalityText, events.ModalityAudio},
		Voice:             "alloy",
		InputAudioFormat:  events.AudioFormatPCM16,
		OutputAudioFormat: events.AudioFormatPCM16,
		TurnDetection: &events.TurnDetection{
			Type:              events.TurnDetectionTypeServerVAD,
			Threshold:         0.5,
			PrefixPaddingMs:   300,
			SilenceDurationMs: 500,
			CreateResponse:    &createResponse,
		},
		Temperature:     0.8,
		MaxOutputTokens: 4096,
	}
}

// NewSession creates a new session with the given WebSocket connection.
// This is kept for backward compatibility. Use NewSessionWithTransport for new code.
func NewSession(ctx context.Context, conn *websocket.Conn, config SessionConfig) *Session {
	session := NewSessionWithTransport(ctx, NewWebSocketTransport(conn), config)
	session.conn = conn // Keep reference for backward compatibility
	return session
}

// NewSessionWithTransport creates a new session with the given transport.
// Use this for WebRTC-based connections where audio is transmitted via RTP.
func NewSessionWithTransport(ctx context.Context, transport Transport, config SessionConfig) *Session {
	sessionID := "sess_" + uuid.New().String()[:12]

	sessionCtx, cancel := context.WithCancel(ctx)

	// Check if transport supports RTP audio
	audioViaRTP := false
	if at, ok := transport.(AudioTransport); ok {
		audioViaRTP = at.SupportsRTPAudio()
	}

	session := &Session{
		ID: sessionID,
		Config: events.Session{
			ID:                sessionID,
			Object:            "realtime.session",
			Model:             config.Model,
			Modalities:        config.Modalities,
			Voice:             config.Voice,
			Instructions:      config.Instructions,
			InputAudioFormat:  config.InputAudioFormat,
			OutputAudioFormat: config.OutputAudioFormat,
			TurnDetection:     config.TurnDetection,
			Temperature:       config.Temperature,
			MaxOutputTokens:   config.MaxOutputTokens,
		},
		Conversation: NewConversation(),
		AudioBuffer: NewAudioBuffer(AudioBufferConfig{
			MaxSize:    10 * 1024 * 1024,
			SampleRate: 24000,
			Channels:   1,
			Format:     config.InputAudioFormat,
		}),
		transport:   transport,
		audioViaRTP: audioViaRTP,
		eventChan:   make(chan events.ServerEvent, 100),
		ctx:         sessionCtx,
		cancel:      cancel,
		closedCh:    make(chan struct{}),
	}

	// Start event writer goroutine
	session.wg.Add(1)
	go session.writeLoop()

	return session
}

// NewSessionWithID creates a new session with a specific session ID.
// Useful for WebRTC connections where session ID is known beforehand.
func NewSessionWithID(ctx context.Context, sessionID string, transport Transport, config SessionConfig) *Session {
	sessionCtx, cancel := context.WithCancel(ctx)

	// Check if transport supports RTP audio
	audioViaRTP := false
	if at, ok := transport.(AudioTransport); ok {
		audioViaRTP = at.SupportsRTPAudio()
	}

	session := &Session{
		ID: sessionID,
		Config: events.Session{
			ID:                sessionID,
			Object:            "realtime.session",
			Model:             config.Model,
			Modalities:        config.Modalities,
			Voice:             config.Voice,
			Instructions:      config.Instructions,
			InputAudioFormat:  config.InputAudioFormat,
			OutputAudioFormat: config.OutputAudioFormat,
			TurnDetection:     config.TurnDetection,
			Temperature:       config.Temperature,
			MaxOutputTokens:   config.MaxOutputTokens,
		},
		Conversation: NewConversation(),
		AudioBuffer: NewAudioBuffer(AudioBufferConfig{
			MaxSize:    10 * 1024 * 1024,
			SampleRate: 24000,
			Channels:   1,
			Format:     config.InputAudioFormat,
		}),
		transport:   transport,
		audioViaRTP: audioViaRTP,
		eventChan:   make(chan events.ServerEvent, 100),
		ctx:         sessionCtx,
		cancel:      cancel,
		closedCh:    make(chan struct{}),
	}

	// Start event writer goroutine
	session.wg.Add(1)
	go session.writeLoop()

	return session
}

// Start starts the session processing.
func (s *Session) Start() error {
	// Send session.created event
	return s.SendEvent(events.NewSessionCreatedEvent(s.Config))
}

// HandleClientEvent processes a client event.
func (s *Session) HandleClientEvent(event events.ClientEvent) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil
	}
	s.mu.RUnlock()

	switch e := event.(type) {
	case *events.SessionUpdateEvent:
		return s.handleSessionUpdate(e)

	case *events.InputAudioBufferAppendEvent:
		return s.handleInputAudioBufferAppend(e)

	case *events.InputAudioBufferCommitEvent:
		return s.handleInputAudioBufferCommit(e)

	case *events.InputAudioBufferClearEvent:
		return s.handleInputAudioBufferClear(e)

	case *events.ConversationItemCreateEvent:
		return s.handleConversationItemCreate(e)

	case *events.ConversationItemTruncateEvent:
		return s.handleConversationItemTruncate(e)

	case *events.ConversationItemDeleteEvent:
		return s.handleConversationItemDelete(e)

	case *events.ResponseCreateEvent:
		return s.handleResponseCreate(e)

	case *events.ResponseCancelEvent:
		return s.handleResponseCancel(e)

	default:
		return s.SendEvent(events.NewErrorEvent(
			events.ErrorTypeInvalidRequest,
			"invalid_event_type",
			"Unknown event type",
			"type",
		))
	}
}

// SendEvent sends a server event to the client.
func (s *Session) SendEvent(event events.ServerEvent) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil
	}
	s.mu.RUnlock()

	select {
	case s.eventChan <- event:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	default:
		// Channel full, log and drop
		log.Printf("[session %s] event channel full, dropping event: %s", s.ID, event.ServerEventType())
		return nil
	}
}

// Close closes the session.
func (s *Session) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	close(s.closedCh)
	s.mu.Unlock()

	// Cancel context
	s.cancel()

	// Close event channel
	close(s.eventChan)

	// Wait for goroutines
	s.wg.Wait()

	// Stop EventBridge if exists
	if s.EventBridge != nil {
		s.EventBridge.Stop()
	}

	// Stop pipeline if exists
	if s.Pipeline != nil {
		s.Pipeline.Stop()
	}

	// Call onClose callback
	if s.onClose != nil {
		s.onClose(s)
	}

	log.Printf("[session %s] closed", s.ID)
	return nil
}

// SetOnClose sets the callback to be called when the session is closed.
func (s *Session) SetOnClose(fn func(session *Session)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onClose = fn
}

// SetPipeline sets the pipeline for this session.
func (s *Session) SetPipeline(p *pipeline.Pipeline) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Pipeline = p
}

// GetPipeline returns the pipeline for this session.
func (s *Session) GetPipeline() *pipeline.Pipeline {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Pipeline
}

// SetEventBridge sets the event bridge for this session.
func (s *Session) SetEventBridge(eb *bridge.EventBridge) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.EventBridge = eb
}

// GetEventBridge returns the event bridge for this session.
func (s *Session) GetEventBridge() *bridge.EventBridge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.EventBridge
}

// Context returns the session context.
func (s *Session) Context() context.Context {
	return s.ctx
}

// IsAudioViaRTP returns true if audio is transmitted via RTP (WebRTC mode).
func (s *Session) IsAudioViaRTP() bool {
	return s.audioViaRTP
}

// GetTransport returns the transport for this session.
func (s *Session) GetTransport() Transport {
	return s.transport
}

// GetAudioTransport returns the audio transport if available.
func (s *Session) GetAudioTransport() AudioTransport {
	if at, ok := s.transport.(AudioTransport); ok {
		return at
	}
	return nil
}

// PushAudio pushes PCM audio data directly to the pipeline.
// This is used for WebRTC mode where audio comes via RTP, not base64-encoded events.
func (s *Session) PushAudio(data []byte, sampleRate, channels int) {
	if p := s.GetPipeline(); p != nil {
		p.Push(&pipeline.PipelineMessage{
			Type:      pipeline.MsgTypeAudio,
			SessionID: s.ID,
			Timestamp: time.Now(),
			AudioData: &pipeline.AudioData{
				Data:       data,
				SampleRate: sampleRate,
				Channels:   channels,
				MediaType:  "audio/x-raw",
				Timestamp:  time.Now(),
			},
		})
	}
}

// writeLoop writes events to the transport.
func (s *Session) writeLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		case event, ok := <-s.eventChan:
			if !ok {
				return
			}

			// Use transport if available, fallback to legacy conn
			if s.transport != nil {
				if err := s.transport.SendEvent(event); err != nil {
					log.Printf("[session %s] failed to send event via transport: %v", s.ID, err)
					return
				}
			} else if s.conn != nil {
				// Legacy WebSocket path
				data, err := json.Marshal(event)
				if err != nil {
					log.Printf("[session %s] failed to marshal event: %v", s.ID, err)
					continue
				}

				if err := s.conn.WriteMessage(websocket.TextMessage, data); err != nil {
					log.Printf("[session %s] failed to write event: %v", s.ID, err)
					return
				}
			}
		}
	}
}

// Event handlers

func (s *Session) handleSessionUpdate(e *events.SessionUpdateEvent) error {
	s.mu.Lock()

	// Update modalities if provided
	if len(e.Session.Modalities) > 0 {
		s.Config.Modalities = e.Session.Modalities
	}

	// Update model if provided
	if e.Session.Model != "" {
		s.Config.Model = e.Session.Model
	}

	// Update voice if provided
	if e.Session.Voice != "" {
		s.Config.Voice = e.Session.Voice
	}

	// Update instructions if provided
	if e.Session.Instructions != "" {
		s.Config.Instructions = e.Session.Instructions
	}

	// Update audio formats if provided
	if e.Session.InputAudioFormat != "" {
		s.Config.InputAudioFormat = e.Session.InputAudioFormat
	}
	if e.Session.OutputAudioFormat != "" {
		s.Config.OutputAudioFormat = e.Session.OutputAudioFormat
	}

	// Update turn detection if provided
	if e.Session.TurnDetection != nil {
		s.Config.TurnDetection = e.Session.TurnDetection
	}

	// Update temperature if provided
	if e.Session.Temperature > 0 {
		s.Config.Temperature = e.Session.Temperature
	}

	// Update max tokens if provided
	if e.Session.MaxOutputTokens > 0 {
		s.Config.MaxOutputTokens = e.Session.MaxOutputTokens
	}

	// Update input audio transcription if provided
	if e.Session.InputAudioTranscription != nil {
		s.Config.InputAudioTranscription = e.Session.InputAudioTranscription
	}

	// Update tools if provided
	if e.Session.Tools != nil {
		s.Config.Tools = e.Session.Tools
	}

	s.mu.Unlock()

	// Send session.updated event
	return s.SendEvent(events.NewSessionUpdatedEvent(s.Config))
}

func (s *Session) handleInputAudioBufferAppend(e *events.InputAudioBufferAppendEvent) error {
	// In RTP mode, audio comes via RTP track, not via this event.
	// Skip processing but don't return error for compatibility.
	if s.audioViaRTP {
		log.Printf("[session %s] ignoring input_audio_buffer.append in RTP mode", s.ID)
		return nil
	}

	if err := s.AudioBuffer.Append(e.Audio); err != nil {
		return s.SendEvent(events.NewErrorEvent(
			events.ErrorTypeInvalidRequest,
			"audio_buffer_overflow",
			err.Error(),
			"audio",
		))
	}

	// If pipeline exists, push audio data
	if p := s.GetPipeline(); p != nil {
		decoded, err := base64.StdEncoding.DecodeString(e.Audio)
		if err == nil && len(decoded) > 0 {
			p.Push(&pipeline.PipelineMessage{
				Type:      pipeline.MsgTypeAudio,
				SessionID: s.ID,
				Timestamp: time.Now(),
				AudioData: &pipeline.AudioData{
					Data:       decoded,
					SampleRate: s.AudioBuffer.Config().SampleRate,
					Channels:   s.AudioBuffer.Config().Channels,
					MediaType:  "audio/x-raw",
					Timestamp:  time.Now(),
				},
			})
		}
	}

	return nil
}

func (s *Session) handleInputAudioBufferCommit(_ *events.InputAudioBufferCommitEvent) error {
	audioData, _, err := s.AudioBuffer.Commit()
	if err != nil {
		return s.SendEvent(events.NewErrorEvent(
			events.ErrorTypeInvalidRequest,
			"buffer_commit_failed",
			err.Error(),
			"",
		))
	}

	if len(audioData) == 0 {
		return nil
	}

	// Create a conversation item for the committed audio
	itemID := "item_" + uuid.New().String()[:8]
	previousItemID := s.Conversation.GetLastItemID()

	item := events.ConversationItem{
		ID:     itemID,
		Object: "realtime.item",
		Type:   events.ItemTypeMessage,
		Status: events.ItemStatusCompleted,
		Role:   events.RoleUser,
		Content: []events.Content{
			{
				Type:  events.ContentTypeInputAudio,
				Audio: base64.StdEncoding.EncodeToString(audioData),
			},
		},
	}

	s.Conversation.AddItem(item)

	// Send events
	if err := s.SendEvent(events.NewInputAudioBufferCommittedEvent(itemID, previousItemID)); err != nil {
		return err
	}

	return s.SendEvent(events.NewConversationItemCreatedEvent(item, previousItemID))
}

func (s *Session) handleInputAudioBufferClear(_ *events.InputAudioBufferClearEvent) error {
	s.AudioBuffer.Clear()
	return s.SendEvent(events.NewInputAudioBufferClearedEvent())
}

func (s *Session) handleConversationItemCreate(e *events.ConversationItemCreateEvent) error {
	itemID := "item_" + uuid.New().String()[:8]

	item := events.ConversationItem{
		ID:      itemID,
		Object:  "realtime.item",
		Type:    e.Item.Type,
		Status:  events.ItemStatusCompleted,
		Role:    e.Item.Role,
		Content: e.Item.Content,
	}

	s.Conversation.AddItem(item)

	return s.SendEvent(events.NewConversationItemCreatedEvent(item, e.PreviousItemID))
}

func (s *Session) handleConversationItemTruncate(e *events.ConversationItemTruncateEvent) error {
	if err := s.Conversation.TruncateItem(e.ItemID, e.ContentIndex, e.AudioEndMs); err != nil {
		return s.SendEvent(events.NewErrorEvent(
			events.ErrorTypeInvalidRequest,
			"item_not_found",
			err.Error(),
			"item_id",
		))
	}

	return s.SendEvent(&events.ConversationItemTruncatedEvent{
		BaseServerEvent: events.NewBaseServerEvent(events.ServerEventTypeConversationItemTruncated),
		ItemID:          e.ItemID,
		ContentIndex:    e.ContentIndex,
		AudioEndMs:      e.AudioEndMs,
	})
}

func (s *Session) handleConversationItemDelete(e *events.ConversationItemDeleteEvent) error {
	if err := s.Conversation.DeleteItem(e.ItemID); err != nil {
		return s.SendEvent(events.NewErrorEvent(
			events.ErrorTypeInvalidRequest,
			"item_not_found",
			err.Error(),
			"item_id",
		))
	}

	return s.SendEvent(&events.ConversationItemDeletedEvent{
		BaseServerEvent: events.NewBaseServerEvent(events.ServerEventTypeConversationItemDeleted),
		ItemID:          e.ItemID,
	})
}

func (s *Session) handleResponseCreate(_ *events.ResponseCreateEvent) error {
	// Create a new response
	responseID := "resp_" + uuid.New().String()[:8]

	response := events.Response{
		ID:     responseID,
		Object: "realtime.response",
		Status: events.ResponseStatusInProgress,
		Output: []events.ConversationItem{},
	}

	// Send response.created event
	if err := s.SendEvent(events.NewResponseCreatedEvent(response)); err != nil {
		return err
	}

	// Trigger AI processing through the pipeline
	// This will be handled by the event handler that listens to pipeline output
	if p := s.GetPipeline(); p != nil {
		// The pipeline will process and send response events
		// through the session's event handler
	}

	return nil
}

func (s *Session) handleResponseCancel(_ *events.ResponseCancelEvent) error {
	// Cancel the current response generation
	// This would typically involve:
	// 1. Canceling any ongoing AI generation
	// 2. Sending a response.done event with cancelled status

	// For now, we just acknowledge
	// The actual implementation would need to track the current response
	// and send appropriate events
	return nil
}
