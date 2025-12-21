// Package server provides WebSocket server implementations for Realtime API.
package server

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi/bridge"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi/events"
)

// WebSocketPipelineFactory creates a pipeline for a WebSocket session.
type WebSocketPipelineFactory func(ctx context.Context, session *realtimeapi.Session) (*pipeline.Pipeline, error)

// WebSocketRealtimeConfig holds the configuration for the WebSocket Realtime API server.
type WebSocketRealtimeConfig struct {
	// Addr is the address to listen on (e.g., ":8080").
	Addr string

	// Path is the WebSocket endpoint path (e.g., "/v1/realtime").
	Path string

	// AuthToken is the bearer token for authentication.
	// If empty, authentication is disabled.
	AuthToken string

	// DefaultModel is the default AI model to use.
	DefaultModel string

	// AllowedModels is a list of allowed model names.
	AllowedModels []string

	// MaxSessionsPerIP limits sessions per IP address.
	// 0 means no limit.
	MaxSessionsPerIP int

	// SessionTimeout is the maximum session duration.
	// 0 means no timeout.
	SessionTimeout time.Duration

	// DefaultSessionConfig is the default session configuration.
	DefaultSessionConfig realtimeapi.SessionConfig

	// ReadBufferSize is the WebSocket read buffer size.
	ReadBufferSize int

	// WriteBufferSize is the WebSocket write buffer size.
	WriteBufferSize int
}

// DefaultWebSocketRealtimeConfig returns the default server configuration.
func DefaultWebSocketRealtimeConfig() *WebSocketRealtimeConfig {
	return &WebSocketRealtimeConfig{
		Addr:                 ":8080",
		Path:                 "/v1/realtime",
		DefaultModel:         "gemini-2.0-flash",
		AllowedModels:        []string{"gemini-2.0-flash", "gpt-4o-realtime"},
		MaxSessionsPerIP:     10,
		SessionTimeout:       30 * time.Minute,
		DefaultSessionConfig: realtimeapi.DefaultSessionConfig(),
		ReadBufferSize:       4096,
		WriteBufferSize:      4096,
	}
}

// WebSocketRealtimeServer is the Realtime API WebSocket server.
type WebSocketRealtimeServer struct {
	config          *WebSocketRealtimeConfig
	pipelineFactory WebSocketPipelineFactory

	// Session management
	sessions   map[string]*realtimeapi.Session
	sessionsMu sync.RWMutex

	// IP-based session counting
	ipSessions   map[string]int
	ipSessionsMu sync.RWMutex

	// HTTP server
	httpServer *http.Server
	mux        *http.ServeMux

	// WebSocket upgrader
	upgrader websocket.Upgrader

	// Context for shutdown
	ctx    context.Context
	cancel context.CancelFunc
}

// NewWebSocketRealtimeServer creates a new WebSocket Realtime API server.
func NewWebSocketRealtimeServer(config *WebSocketRealtimeConfig) *WebSocketRealtimeServer {
	if config == nil {
		config = DefaultWebSocketRealtimeConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &WebSocketRealtimeServer{
		config:     config,
		sessions:   make(map[string]*realtimeapi.Session),
		ipSessions: make(map[string]int),
		mux:        http.NewServeMux(),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  config.ReadBufferSize,
			WriteBufferSize: config.WriteBufferSize,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins; customize for production
			},
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// SetPipelineFactory sets the pipeline factory for creating session pipelines.
func (s *WebSocketRealtimeServer) SetPipelineFactory(factory WebSocketPipelineFactory) {
	s.pipelineFactory = factory
}

// RegisterHandler registers an HTTP handler on the server's mux.
// Must be called before Start().
func (s *WebSocketRealtimeServer) RegisterHandler(pattern string, handler http.HandlerFunc) {
	s.mux.HandleFunc(pattern, handler)
}

// Start starts the server.
func (s *WebSocketRealtimeServer) Start(ctx context.Context) error {
	// Register WebSocket handler
	s.mux.HandleFunc(s.config.Path, s.handleWebSocket)

	s.httpServer = &http.Server{
		Addr:    s.config.Addr,
		Handler: s.mux,
	}

	log.Printf("[WebSocketRealtimeServer] starting on %s%s", s.config.Addr, s.config.Path)

	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-time.After(100 * time.Millisecond):
		// Server started successfully
		return nil
	}
}

// Stop stops the server gracefully.
func (s *WebSocketRealtimeServer) Stop(ctx context.Context) error {
	s.cancel()

	// Close all sessions
	s.sessionsMu.Lock()
	for _, session := range s.sessions {
		session.Close()
	}
	s.sessions = make(map[string]*realtimeapi.Session)
	s.sessionsMu.Unlock()

	// Shutdown HTTP server
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// handleWebSocket handles WebSocket connections.
func (s *WebSocketRealtimeServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Authentication
	if s.config.AuthToken != "" {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token != s.config.AuthToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Get model from query parameter
	model := r.URL.Query().Get("model")
	if model == "" {
		model = s.config.DefaultModel
	}

	// Validate model
	if !s.isModelAllowed(model) {
		http.Error(w, fmt.Sprintf("Model not allowed: %s", model), http.StatusBadRequest)
		return
	}

	// Check IP session limit
	clientIP := getClientIP(r)
	if s.config.MaxSessionsPerIP > 0 {
		s.ipSessionsMu.RLock()
		count := s.ipSessions[clientIP]
		s.ipSessionsMu.RUnlock()

		if count >= s.config.MaxSessionsPerIP {
			http.Error(w, "Too many sessions from this IP", http.StatusTooManyRequests)
			return
		}
	}

	// Upgrade to WebSocket
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WebSocketRealtimeServer] WebSocket upgrade failed: %v", err)
		return
	}

	// Create session
	sessionConfig := s.config.DefaultSessionConfig
	sessionConfig.Model = model

	session := realtimeapi.NewSession(s.ctx, conn, sessionConfig)

	// Register session
	s.registerSession(session, clientIP)

	// Set cleanup callback
	session.SetOnClose(func(sess *realtimeapi.Session) {
		s.unregisterSession(sess, clientIP)
	})

	// Create pipeline if factory is set
	if s.pipelineFactory != nil {
		p, err := s.pipelineFactory(session.Context(), session)
		if err != nil {
			log.Printf("[WebSocketRealtimeServer] Failed to create pipeline: %v", err)
			session.SendEvent(events.NewErrorEvent(
				events.ErrorTypeServer,
				"pipeline_creation_failed",
				err.Error(),
				"",
			))
			session.Close()
			return
		}
		session.SetPipeline(p)

		// Create and start EventBridge for pipeline-to-WebSocket event translation
		eb := bridge.NewEventBridge(p.Bus(), session, session.ID)
		session.SetEventBridge(eb)
		if err := eb.Start(session.Context()); err != nil {
			log.Printf("[WebSocketRealtimeServer] Failed to start event bridge: %v", err)
		}

		// Start pipeline
		if err := p.Start(session.Context()); err != nil {
			log.Printf("[WebSocketRealtimeServer] Failed to start pipeline: %v", err)
			session.SendEvent(events.NewErrorEvent(
				events.ErrorTypeServer,
				"pipeline_start_failed",
				err.Error(),
				"",
			))
			session.Close()
			return
		}

		// Start pipeline output handler (fallback for elements that don't use bus events)
		go s.handlePipelineOutput(session)
	}

	// Start session
	if err := session.Start(); err != nil {
		log.Printf("[WebSocketRealtimeServer] Failed to start session: %v", err)
		session.Close()
		return
	}

	// Handle incoming messages
	s.handleSession(session, conn)
}

// handleSession handles messages for a session.
func (s *WebSocketRealtimeServer) handleSession(session *realtimeapi.Session, conn *websocket.Conn) {
	defer session.Close()

	for {
		select {
		case <-session.Context().Done():
			return
		default:
		}

		// Read message from WebSocket connection
		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WebSocketRealtimeServer] [session %s] WebSocket read error: %v", session.ID, err)
			}
			return
		}

		// Parse event
		event, err := events.ParseClientEvent(data)
		if err != nil {
			session.SendEvent(events.NewErrorEvent(
				events.ErrorTypeInvalidRequest,
				"invalid_event",
				err.Error(),
				"",
			))
			continue
		}

		// Handle event
		if err := session.HandleClientEvent(event); err != nil {
			log.Printf("[WebSocketRealtimeServer] [session %s] Event handling error: %v", session.ID, err)
		}
	}
}

// handlePipelineOutput handles output from the pipeline.
func (s *WebSocketRealtimeServer) handlePipelineOutput(session *realtimeapi.Session) {
	p := session.GetPipeline()
	if p == nil {
		return
	}

	// Track current response state
	var currentResponseID string
	var currentItemID string
	var outputIndex int
	var contentIndex int

	for {
		select {
		case <-session.Context().Done():
			return
		default:
		}

		msg := p.Pull()
		if msg == nil {
			continue
		}

		switch msg.Type {
		case pipeline.MsgTypeAudio:
			if msg.AudioData == nil || len(msg.AudioData.Data) == 0 {
				continue
			}

			// If no current response, create one
			if currentResponseID == "" {
				currentResponseID = "resp_" + generateShortID()
				currentItemID = "item_" + generateShortID()
				outputIndex = 0
				contentIndex = 0

				// Send response.created
				session.SendEvent(events.NewResponseCreatedEvent(events.Response{
					ID:     currentResponseID,
					Object: "realtime.response",
					Status: events.ResponseStatusInProgress,
					Output: []events.ConversationItem{},
				}))

				// Send output_item.added
				session.SendEvent(events.NewResponseOutputItemAddedEvent(
					currentResponseID,
					outputIndex,
					events.ConversationItem{
						ID:     currentItemID,
						Object: "realtime.item",
						Type:   events.ItemTypeMessage,
						Status: events.ItemStatusInProgress,
						Role:   events.RoleAssistant,
						Content: []events.Content{
							{Type: events.ContentTypeAudio},
						},
					},
				))

				// Send content_part.added
				session.SendEvent(events.NewResponseContentPartAddedEvent(
					currentResponseID,
					currentItemID,
					outputIndex,
					contentIndex,
					events.Content{Type: events.ContentTypeAudio},
				))
			}

			// Send audio delta
			audioBase64 := base64.StdEncoding.EncodeToString(msg.AudioData.Data)
			session.SendEvent(events.NewResponseAudioDeltaEvent(
				currentResponseID,
				currentItemID,
				outputIndex,
				contentIndex,
				audioBase64,
			))

		case pipeline.MsgTypeData:
			if msg.TextData == nil || len(msg.TextData.Data) == 0 {
				continue
			}

			// If no current response, create one
			if currentResponseID == "" {
				currentResponseID = "resp_" + generateShortID()
				currentItemID = "item_" + generateShortID()
				outputIndex = 0
				contentIndex = 0

				// Send response.created
				session.SendEvent(events.NewResponseCreatedEvent(events.Response{
					ID:     currentResponseID,
					Object: "realtime.response",
					Status: events.ResponseStatusInProgress,
					Output: []events.ConversationItem{},
				}))
			}

			// Send text delta
			session.SendEvent(events.NewResponseTextDeltaEvent(
				currentResponseID,
				currentItemID,
				outputIndex,
				contentIndex,
				string(msg.TextData.Data),
			))
		}
	}
}

// registerSession adds a session to the server.
func (s *WebSocketRealtimeServer) registerSession(session *realtimeapi.Session, clientIP string) {
	s.sessionsMu.Lock()
	s.sessions[session.ID] = session
	s.sessionsMu.Unlock()

	s.ipSessionsMu.Lock()
	s.ipSessions[clientIP]++
	s.ipSessionsMu.Unlock()

	log.Printf("[WebSocketRealtimeServer] [session %s] registered from %s", session.ID, clientIP)
}

// unregisterSession removes a session from the server.
func (s *WebSocketRealtimeServer) unregisterSession(session *realtimeapi.Session, clientIP string) {
	s.sessionsMu.Lock()
	delete(s.sessions, session.ID)
	s.sessionsMu.Unlock()

	s.ipSessionsMu.Lock()
	s.ipSessions[clientIP]--
	if s.ipSessions[clientIP] <= 0 {
		delete(s.ipSessions, clientIP)
	}
	s.ipSessionsMu.Unlock()

	log.Printf("[WebSocketRealtimeServer] [session %s] unregistered", session.ID)
}

// isModelAllowed checks if a model is in the allowed list.
func (s *WebSocketRealtimeServer) isModelAllowed(model string) bool {
	if len(s.config.AllowedModels) == 0 {
		return true
	}
	return slices.Contains(s.config.AllowedModels, model)
}

// GetSession returns a session by ID.
func (s *WebSocketRealtimeServer) GetSession(sessionID string) *realtimeapi.Session {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	return s.sessions[sessionID]
}

// SessionCount returns the number of active sessions.
func (s *WebSocketRealtimeServer) SessionCount() int {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	return len(s.sessions)
}

// Helper functions

func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}

	// Check X-Real-IP header
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return strings.Split(r.RemoteAddr, ":")[0]
}

func generateShortID() string {
	return uuid.New().String()[:8]
}
