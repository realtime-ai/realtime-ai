// Package server provides HTTP and WebSocket server implementations.
//
// TwilioMediaServer implements a WebSocket server for Twilio Media Streams.
// It handles bidirectional audio streaming for voice AI applications.
//
// Features:
//   - WebSocket endpoint for Twilio Media Streams
//   - TwiML webhook endpoint for call control
//   - Pipeline factory for per-call processing
//   - Session management with call lifecycle hooks
//
// Usage:
//   1. Configure TwiML to connect to /media endpoint
//   2. Implement PipelineFactory to create processing pipelines
//   3. Start server and handle incoming calls
//
// Reference: https://www.twilio.com/docs/voice/media-streams

package server

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/realtime-ai/realtime-ai/pkg/connection"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

// TwilioServerConfig holds configuration for TwilioMediaServer.
type TwilioServerConfig struct {
	// Address is the listen address (e.g., ":8080")
	Address string

	// WebSocketPath is the path for WebSocket connections (default: "/media")
	WebSocketPath string

	// TwiMLPath is the path for TwiML webhook (default: "/twiml")
	TwiMLPath string

	// StreamURL is the public URL for WebSocket connections
	// This is used in TwiML response for <Connect><Stream>
	// Example: "wss://your-domain.com/media"
	StreamURL string

	// ReadBufferSize for WebSocket (default: 1024)
	ReadBufferSize int

	// WriteBufferSize for WebSocket (default: 1024)
	WriteBufferSize int

	// CustomParameters to pass from TwiML to the stream
	CustomParameters map[string]string
}

// TwilioPipelineFactory creates pipelines for Twilio connections.
type TwilioPipelineFactory interface {
	// CreatePipeline creates a new pipeline for a Twilio call.
	// The connection provides audio I/O with the Twilio stream.
	CreatePipeline(ctx context.Context, conn *connection.TwilioConnection) (*pipeline.Pipeline, error)
}

// TwilioMediaServer handles Twilio Media Streams WebSocket connections.
type TwilioMediaServer struct {
	config          TwilioServerConfig
	pipelineFactory TwilioPipelineFactory

	upgrader websocket.Upgrader
	server   *http.Server

	// Active sessions
	sessions   map[string]*TwilioSession
	sessionsMu sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// TwilioSession represents an active Twilio call session.
type TwilioSession struct {
	Connection *connection.TwilioConnection
	Pipeline   *pipeline.Pipeline
	CallSid    string
	StreamSid  string
	StartTime  time.Time
}

// NewTwilioMediaServer creates a new Twilio Media Streams server.
func NewTwilioMediaServer(config TwilioServerConfig, factory TwilioPipelineFactory) *TwilioMediaServer {
	// Set defaults
	if config.WebSocketPath == "" {
		config.WebSocketPath = "/media"
	}
	if config.TwiMLPath == "" {
		config.TwiMLPath = "/twiml"
	}
	if config.ReadBufferSize == 0 {
		config.ReadBufferSize = 1024
	}
	if config.WriteBufferSize == 0 {
		config.WriteBufferSize = 1024
	}

	return &TwilioMediaServer{
		config:          config,
		pipelineFactory: factory,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  config.ReadBufferSize,
			WriteBufferSize: config.WriteBufferSize,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
		sessions: make(map[string]*TwilioSession),
	}
}

// Start starts the Twilio Media Streams server.
func (s *TwilioMediaServer) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc(s.config.WebSocketPath, s.handleWebSocket)
	mux.HandleFunc(s.config.TwiMLPath, s.handleTwiML)
	mux.HandleFunc("/health", s.handleHealth)

	s.server = &http.Server{
		Addr:    s.config.Address,
		Handler: mux,
	}

	log.Printf("[TwilioServer] Starting server on %s", s.config.Address)
	log.Printf("[TwilioServer] WebSocket endpoint: %s", s.config.WebSocketPath)
	log.Printf("[TwilioServer] TwiML endpoint: %s", s.config.TwiMLPath)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[TwilioServer] Server error: %v", err)
		}
	}()

	return nil
}

// Stop stops the server gracefully.
func (s *TwilioMediaServer) Stop() error {
	log.Printf("[TwilioServer] Stopping server...")

	if s.cancel != nil {
		s.cancel()
	}

	// Close all sessions
	s.sessionsMu.Lock()
	for _, session := range s.sessions {
		if session.Pipeline != nil {
			session.Pipeline.Stop()
		}
		if session.Connection != nil {
			session.Connection.Close()
		}
	}
	s.sessions = make(map[string]*TwilioSession)
	s.sessionsMu.Unlock()

	// Shutdown HTTP server
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(ctx)
	}

	s.wg.Wait()
	log.Printf("[TwilioServer] Server stopped")
	return nil
}

// handleWebSocket handles incoming Twilio WebSocket connections.
func (s *TwilioMediaServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	log.Printf("[TwilioServer] WebSocket connection from %s", r.RemoteAddr)

	// Upgrade HTTP to WebSocket
	wsConn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[TwilioServer] WebSocket upgrade failed: %v", err)
		return
	}

	// Create Twilio connection
	twilioConn, err := connection.NewTwilioConnection(wsConn)
	if err != nil {
		log.Printf("[TwilioServer] Failed to create Twilio connection: %v", err)
		wsConn.Close()
		return
	}

	// Register event handler to track session state
	twilioConn.RegisterEventHandler(&twilioSessionHandler{
		server:     s,
		connection: twilioConn,
	})

	// Start connection processing
	twilioConn.Start()

	// Wait for connection to receive start event and get callSid
	// Then create pipeline
	go s.waitAndCreatePipeline(twilioConn)
}

// waitAndCreatePipeline waits for connection to be established then creates pipeline.
func (s *TwilioMediaServer) waitAndCreatePipeline(conn *connection.TwilioConnection) {
	// Wait for connection to be connected (receive start event)
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			log.Printf("[TwilioServer] Timeout waiting for stream start")
			conn.Close()
			return
		case <-ticker.C:
			if conn.State() == connection.ConnectionStateConnected {
				goto createPipeline
			}
			if conn.State() == connection.ConnectionStateClosed ||
				conn.State() == connection.ConnectionStateFailed {
				return
			}
		case <-s.ctx.Done():
			conn.Close()
			return
		}
	}

createPipeline:
	callSid := conn.CallSid()
	streamSid := conn.StreamSid()

	log.Printf("[TwilioServer] Creating pipeline for call %s (stream %s)", callSid, streamSid)

	// Create pipeline using factory
	p, err := s.pipelineFactory.CreatePipeline(s.ctx, conn)
	if err != nil {
		log.Printf("[TwilioServer] Failed to create pipeline: %v", err)
		conn.Close()
		return
	}

	// Store session
	session := &TwilioSession{
		Connection: conn,
		Pipeline:   p,
		CallSid:    callSid,
		StreamSid:  streamSid,
		StartTime:  time.Now(),
	}

	s.sessionsMu.Lock()
	s.sessions[callSid] = session
	s.sessionsMu.Unlock()

	log.Printf("[TwilioServer] Session created for call %s", callSid)

	// Start pipeline
	if err := p.Start(s.ctx); err != nil {
		log.Printf("[TwilioServer] Failed to start pipeline: %v", err)
		s.removeSession(callSid)
		conn.Close()
		return
	}

	// Start audio forwarding from connection to pipeline
	go s.forwardAudioToPipeline(conn, p)
}

// forwardAudioToPipeline forwards audio from Twilio connection to pipeline.
func (s *TwilioMediaServer) forwardAudioToPipeline(conn *connection.TwilioConnection, p *pipeline.Pipeline) {
	for msg := range conn.In() {
		if msg.Type == pipeline.MsgTypeAudio {
			p.Push(msg)
		}
	}
	log.Printf("[TwilioServer] Audio forwarding stopped for call %s", conn.CallSid())
}

// handleTwiML serves TwiML for incoming calls.
func (s *TwilioMediaServer) handleTwiML(w http.ResponseWriter, r *http.Request) {
	log.Printf("[TwilioServer] TwiML request from %s", r.RemoteAddr)

	// Log request parameters
	if err := r.ParseForm(); err == nil {
		callSid := r.FormValue("CallSid")
		from := r.FormValue("From")
		to := r.FormValue("To")
		log.Printf("[TwilioServer] Incoming call: CallSid=%s, From=%s, To=%s", callSid, from, to)
	}

	// Generate TwiML with Stream
	twimlTemplate := `<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Connect>
        <Stream url="{{.StreamURL}}">
            {{range $key, $value := .Parameters}}
            <Parameter name="{{$key}}" value="{{$value}}" />
            {{end}}
        </Stream>
    </Connect>
</Response>`

	tmpl, err := template.New("twiml").Parse(twimlTemplate)
	if err != nil {
		log.Printf("[TwilioServer] Failed to parse TwiML template: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	data := struct {
		StreamURL  string
		Parameters map[string]string
	}{
		StreamURL:  s.config.StreamURL,
		Parameters: s.config.CustomParameters,
	}

	w.Header().Set("Content-Type", "application/xml")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[TwilioServer] Failed to execute TwiML template: %v", err)
	}
}

// handleHealth handles health check requests.
func (s *TwilioMediaServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.sessionsMu.RLock()
	sessionCount := len(s.sessions)
	s.sessionsMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","sessions":%d}`, sessionCount)
}

// removeSession removes a session from tracking.
func (s *TwilioMediaServer) removeSession(callSid string) {
	s.sessionsMu.Lock()
	if session, ok := s.sessions[callSid]; ok {
		if session.Pipeline != nil {
			session.Pipeline.Stop()
		}
		delete(s.sessions, callSid)
		log.Printf("[TwilioServer] Session removed for call %s (duration: %v)",
			callSid, time.Since(session.StartTime))
	}
	s.sessionsMu.Unlock()
}

// GetSession returns a session by call SID.
func (s *TwilioMediaServer) GetSession(callSid string) *TwilioSession {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	return s.sessions[callSid]
}

// GetActiveSessions returns all active sessions.
func (s *TwilioMediaServer) GetActiveSessions() []*TwilioSession {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()

	sessions := make([]*TwilioSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

// twilioSessionHandler handles connection state changes.
type twilioSessionHandler struct {
	server     *TwilioMediaServer
	connection *connection.TwilioConnection
}

func (h *twilioSessionHandler) OnConnectionStateChange(state connection.ConnectionState) {
	log.Printf("[TwilioServer] Connection state changed: %v", state)

	if state == connection.ConnectionStateClosed || state == connection.ConnectionStateDisconnected {
		callSid := h.connection.CallSid()
		if callSid != "" {
			h.server.removeSession(callSid)
		}
	}
}

func (h *twilioSessionHandler) OnMessage(msg *pipeline.PipelineMessage) {
	// Messages are handled by the pipeline, not here
}

func (h *twilioSessionHandler) OnError(err error) {
	log.Printf("[TwilioServer] Connection error: %v", err)
}
