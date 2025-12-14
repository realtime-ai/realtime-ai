package realtimeapi

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi/bridge"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi/events"
)

// PipelineFactory creates a pipeline for a session.
type PipelineFactory func(ctx context.Context, session *Session) (*pipeline.Pipeline, error)

// ServerConfig holds the configuration for the Realtime API server.
type ServerConfig struct {
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
	DefaultSessionConfig SessionConfig

	// ReadBufferSize is the WebSocket read buffer size.
	ReadBufferSize int

	// WriteBufferSize is the WebSocket write buffer size.
	WriteBufferSize int
}

// DefaultServerConfig returns the default server configuration.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Addr:                 ":8080",
		Path:                 "/v1/realtime",
		DefaultModel:         "gemini-2.0-flash",
		AllowedModels:        []string{"gemini-2.0-flash", "gpt-4o-realtime"},
		MaxSessionsPerIP:     10,
		SessionTimeout:       30 * time.Minute,
		DefaultSessionConfig: DefaultSessionConfig(),
		ReadBufferSize:       4096,
		WriteBufferSize:      4096,
	}
}

// Server is the Realtime API WebSocket server.
type Server struct {
	config          ServerConfig
	pipelineFactory PipelineFactory

	// Session management
	sessions   map[string]*Session
	sessionsMu sync.RWMutex

	// IP-based session counting
	ipSessions   map[string]int
	ipSessionsMu sync.RWMutex

	// HTTP server
	httpServer *http.Server

	// WebSocket upgrader
	upgrader websocket.Upgrader

	// Context for shutdown
	ctx    context.Context
	cancel context.CancelFunc
}

// NewServer creates a new Realtime API server.
func NewServer(config ServerConfig) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		config:     config,
		sessions:   make(map[string]*Session),
		ipSessions: make(map[string]int),
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
func (s *Server) SetPipelineFactory(factory PipelineFactory) {
	s.pipelineFactory = factory
}

// Start starts the server.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc(s.config.Path, s.handleWebSocket)

	s.httpServer = &http.Server{
		Addr:    s.config.Addr,
		Handler: mux,
	}

	log.Printf("Realtime API server starting on %s%s", s.config.Addr, s.config.Path)

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
func (s *Server) Stop(ctx context.Context) error {
	s.cancel()

	// Close all sessions
	s.sessionsMu.Lock()
	for _, session := range s.sessions {
		session.Close()
	}
	s.sessions = make(map[string]*Session)
	s.sessionsMu.Unlock()

	// Shutdown HTTP server
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// handleWebSocket handles WebSocket connections.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	// Create session
	sessionConfig := s.config.DefaultSessionConfig
	sessionConfig.Model = model

	session := NewSession(s.ctx, conn, sessionConfig)

	// Register session
	s.registerSession(session, clientIP)

	// Set cleanup callback
	session.SetOnClose(func(sess *Session) {
		s.unregisterSession(sess, clientIP)
	})

	// Create pipeline if factory is set
	if s.pipelineFactory != nil {
		p, err := s.pipelineFactory(session.Context(), session)
		if err != nil {
			log.Printf("Failed to create pipeline: %v", err)
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
			log.Printf("Failed to start event bridge: %v", err)
		}

		// Start pipeline
		if err := p.Start(session.Context()); err != nil {
			log.Printf("Failed to start pipeline: %v", err)
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
		log.Printf("Failed to start session: %v", err)
		session.Close()
		return
	}

	// Handle incoming messages
	s.handleSession(session)
}

// handleSession handles messages for a session.
func (s *Server) handleSession(session *Session) {
	defer session.Close()

	for {
		select {
		case <-session.Context().Done():
			return
		default:
		}

		// Read message
		_, data, err := session.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[session %s] WebSocket read error: %v", session.ID, err)
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
			log.Printf("[session %s] Event handling error: %v", session.ID, err)
		}
	}
}

// handlePipelineOutput handles output from the pipeline.
func (s *Server) handlePipelineOutput(session *Session) {
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
func (s *Server) registerSession(session *Session, clientIP string) {
	s.sessionsMu.Lock()
	s.sessions[session.ID] = session
	s.sessionsMu.Unlock()

	s.ipSessionsMu.Lock()
	s.ipSessions[clientIP]++
	s.ipSessionsMu.Unlock()

	log.Printf("[session %s] registered from %s", session.ID, clientIP)
}

// unregisterSession removes a session from the server.
func (s *Server) unregisterSession(session *Session, clientIP string) {
	s.sessionsMu.Lock()
	delete(s.sessions, session.ID)
	s.sessionsMu.Unlock()

	s.ipSessionsMu.Lock()
	s.ipSessions[clientIP]--
	if s.ipSessions[clientIP] <= 0 {
		delete(s.ipSessions, clientIP)
	}
	s.ipSessionsMu.Unlock()

	log.Printf("[session %s] unregistered", session.ID)
}

// isModelAllowed checks if a model is in the allowed list.
func (s *Server) isModelAllowed(model string) bool {
	if len(s.config.AllowedModels) == 0 {
		return true
	}
	for _, m := range s.config.AllowedModels {
		if m == model {
			return true
		}
	}
	return false
}

// GetSession returns a session by ID.
func (s *Server) GetSession(sessionID string) *Session {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	return s.sessions[sessionID]
}

// SessionCount returns the number of active sessions.
func (s *Server) SessionCount() int {
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
