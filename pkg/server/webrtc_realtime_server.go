// Package server provides WebRTC server implementations for Realtime API.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/realtime-ai/realtime-ai/pkg/connection"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi/bridge"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi/events"
)

// PipelineFactory creates a pipeline for a session.
type PipelineFactory func(ctx context.Context, session *realtimeapi.Session) (*pipeline.Pipeline, error)

// WebRTCRealtimeConfig holds configuration for WebRTCRealtimeServer.
type WebRTCRealtimeConfig struct {
	// WebRTC configuration
	RTCUDPPort int
	ICELite    bool
	Endpoint   []string

	// Realtime API configuration
	DefaultModel  string
	AllowedModels []string

	// Authentication (optional)
	AuthValidator func(token string) bool
}

// DefaultWebRTCRealtimeConfig returns default configuration.
func DefaultWebRTCRealtimeConfig() *WebRTCRealtimeConfig {
	return &WebRTCRealtimeConfig{
		RTCUDPPort:    9000,
		ICELite:       true,
		DefaultModel:  "gemini-2.0-flash",
		AllowedModels: []string{"gemini-2.0-flash", "gemini-1.5-pro"},
	}
}

// WebRTCRealtimeServer handles WebRTC connections with Realtime API protocol.
// Audio is transmitted via RTP tracks, signaling via DataChannel.
type WebRTCRealtimeServer struct {
	sync.RWMutex

	config *WebRTCRealtimeConfig
	api    *webrtc.API

	// Pipeline factory
	pipelineFactory PipelineFactory

	// Session management
	sessions map[string]*realtimeapi.Session

	// Connection callbacks
	onConnectionCreated func(ctx context.Context, conn connection.WebRTCRealtimeConnection, session *realtimeapi.Session)
	onConnectionError   func(ctx context.Context, conn connection.WebRTCRealtimeConnection, err error)
}

// NewWebRTCRealtimeServer creates a new WebRTC Realtime server.
func NewWebRTCRealtimeServer(config *WebRTCRealtimeConfig) *WebRTCRealtimeServer {
	if config == nil {
		config = DefaultWebRTCRealtimeConfig()
	}

	return &WebRTCRealtimeServer{
		config:   config,
		sessions: make(map[string]*realtimeapi.Session),
		onConnectionCreated: func(ctx context.Context, conn connection.WebRTCRealtimeConnection, session *realtimeapi.Session) {
		},
		onConnectionError: func(ctx context.Context, conn connection.WebRTCRealtimeConnection, err error) {},
	}
}

// SetPipelineFactory sets the pipeline factory function.
func (s *WebRTCRealtimeServer) SetPipelineFactory(factory PipelineFactory) {
	s.pipelineFactory = factory
}

// OnConnectionCreated sets the callback for new connections.
func (s *WebRTCRealtimeServer) OnConnectionCreated(f func(ctx context.Context, conn connection.WebRTCRealtimeConnection, session *realtimeapi.Session)) {
	s.onConnectionCreated = f
}

// OnConnectionError sets the callback for connection errors.
func (s *WebRTCRealtimeServer) OnConnectionError(f func(ctx context.Context, conn connection.WebRTCRealtimeConnection, err error)) {
	s.onConnectionError = f
}

// Start initializes the WebRTC API.
func (s *WebRTCRealtimeServer) Start() error {
	settingEngine := webrtc.SettingEngine{}

	if s.config.ICELite {
		settingEngine.SetLite(true)
	}

	// Set NAT1To1IPs for ICE candidates
	endpoints := s.config.Endpoint
	if len(endpoints) == 0 && s.config.ICELite {
		// Auto-detect local IP for ICE Lite mode
		if localIP := getLocalIP(); localIP != "" {
			endpoints = []string{localIP}
			log.Printf("[WebRTCRealtimeServer] auto-detected local IP: %s", localIP)
		}
	}

	if len(endpoints) > 0 {
		settingEngine.SetNAT1To1IPs(endpoints, webrtc.ICECandidateTypeHost)
	}

	settingEngine.SetFireOnTrackBeforeFirstRTP(true)
	settingEngine.SetNetworkTypes([]webrtc.NetworkType{
		webrtc.NetworkTypeUDP4,
		webrtc.NetworkTypeTCP4,
	})

	udpListener, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.ParseIP("0.0.0.0"),
		Port: s.config.RTCUDPPort,
	})
	if err != nil {
		return fmt.Errorf("failed to listen UDP port %d: %w", s.config.RTCUDPPort, err)
	}

	udpMux := webrtc.NewICEUDPMux(nil, udpListener)
	settingEngine.SetICEUDPMux(udpMux)

	s.api = webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))

	log.Printf("[WebRTCRealtimeServer] started on UDP port %d", s.config.RTCUDPPort)
	return nil
}

// HandleNegotiate handles WebRTC signaling at /session endpoint.
func (s *WebRTCRealtimeServer) HandleNegotiate(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Optional authentication
	if s.config.AuthValidator != nil {
		token := r.Header.Get("Authorization")
		if !s.config.AuthValidator(token) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Parse SDP offer
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var offer webrtc.SessionDescription
	if err := json.Unmarshal(body, &offer); err != nil {
		http.Error(w, "Failed to parse offer", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	// Create PeerConnection
	pc, err := s.api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{},
	})
	if err != nil {
		s.onConnectionError(ctx, nil, err)
		http.Error(w, "Failed to create peer connection", http.StatusInternalServerError)
		return
	}

	// Create WebRTCRealtimeConnection
	conn, err := connection.NewWebRTCRealtimeConnection(pc)
	if err != nil {
		pc.Close()
		s.onConnectionError(ctx, nil, err)
		http.Error(w, "Failed to create connection", http.StatusInternalServerError)
		return
	}

	// Create DataChannel transport for the session
	// Note: DataChannel is created by client, we handle it in connection.Start()

	// Create session configuration
	sessionConfig := realtimeapi.DefaultSessionConfig()
	sessionConfig.Model = s.config.DefaultModel

	// Create transport adapter that wraps the connection
	transport := &webrtcConnectionTransport{conn: conn}

	// Create session with transport
	session := realtimeapi.NewSessionWithID(ctx, conn.SessionID(), transport, sessionConfig)

	// Register session
	s.Lock()
	s.sessions[session.ID] = session
	s.Unlock()

	// Set up cleanup on session close
	session.SetOnClose(func(sess *realtimeapi.Session) {
		s.Lock()
		delete(s.sessions, sess.ID)
		s.Unlock()
		conn.Close()
	})

	// Create event handler that bridges connection events to session
	handler := &webrtcRealtimeEventHandler{
		conn:    conn,
		session: session,
		server:  s,
	}
	conn.RegisterEventHandler(handler)

	// Start connection (sets up DataChannel and audio track handlers)
	if err := conn.Start(ctx); err != nil {
		session.Close()
		http.Error(w, "Failed to start connection", http.StatusInternalServerError)
		return
	}

	// Notify callback
	s.onConnectionCreated(ctx, conn, session)

	// WebRTC negotiation
	if err := pc.SetRemoteDescription(offer); err != nil {
		s.onConnectionError(ctx, conn, err)
		session.Close()
		http.Error(w, "Failed to set remote description", http.StatusInternalServerError)
		return
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		s.onConnectionError(ctx, conn, err)
		session.Close()
		http.Error(w, "Failed to create answer", http.StatusInternalServerError)
		return
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		s.onConnectionError(ctx, conn, err)
		session.Close()
		http.Error(w, "Failed to set local description", http.StatusInternalServerError)
		return
	}

	// Wait for ICE gathering
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	<-gatherComplete

	// Return answer
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	response := struct {
		SessionID string                     `json:"session_id"`
		SDP       *webrtc.SessionDescription `json:"sdp"`
	}{
		SessionID: session.ID,
		SDP:       pc.LocalDescription(),
	}
	json.NewEncoder(w).Encode(response)

	log.Printf("[WebRTCRealtimeServer] session %s created", session.ID)
}

// GetSession returns a session by ID.
func (s *WebRTCRealtimeServer) GetSession(sessionID string) *realtimeapi.Session {
	s.RLock()
	defer s.RUnlock()
	return s.sessions[sessionID]
}

// webrtcConnectionTransport adapts WebRTCRealtimeConnection to realtimeapi.AudioTransport.
type webrtcConnectionTransport struct {
	conn connection.WebRTCRealtimeConnection
}

func (t *webrtcConnectionTransport) SendEvent(event events.ServerEvent) error {
	return t.conn.SendEvent(event)
}

func (t *webrtcConnectionTransport) SendAudio(data []byte, sampleRate, channels int) error {
	return t.conn.SendAudio(data, sampleRate, channels)
}

func (t *webrtcConnectionTransport) SupportsRTPAudio() bool {
	return t.conn.SupportsRTPAudio()
}

func (t *webrtcConnectionTransport) Close() error {
	return t.conn.Close()
}

// webrtcRealtimeEventHandler handles events from WebRTCRealtimeConnection.
type webrtcRealtimeEventHandler struct {
	conn    connection.WebRTCRealtimeConnection
	session *realtimeapi.Session
	server  *WebRTCRealtimeServer

	pipelineCreated bool
}

func (h *webrtcRealtimeEventHandler) OnConnectionStateChange(state webrtc.PeerConnectionState) {
	log.Printf("[WebRTCRealtimeServer] session %s connection state: %s", h.session.ID, state.String())

	switch state {
	case webrtc.PeerConnectionStateConnected:
		// Start session and create pipeline
		if err := h.session.Start(); err != nil {
			log.Printf("[WebRTCRealtimeServer] session %s failed to start: %v", h.session.ID, err)
		}

		// Create pipeline if factory is set
		if h.server.pipelineFactory != nil && !h.pipelineCreated {
			h.pipelineCreated = true
			go h.setupPipeline()
		}

	case webrtc.PeerConnectionStateDisconnected, webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
		h.session.Close()
	}
}

func (h *webrtcRealtimeEventHandler) setupPipeline() {
	ctx := h.session.Context()

	// Create pipeline
	p, err := h.server.pipelineFactory(ctx, h.session)
	if err != nil {
		log.Printf("[WebRTCRealtimeServer] session %s failed to create pipeline: %v", h.session.ID, err)
		return
	}

	h.session.SetPipeline(p)

	// Create EventBridge with AudioSink for RTP audio output
	audioTransport := h.session.GetAudioTransport()
	var eb *bridge.EventBridge
	if audioTransport != nil {
		// Create adapter to bridge.AudioSink
		audioSink := &audioTransportSink{transport: audioTransport}
		eb = bridge.NewEventBridgeWithAudioSink(p.Bus(), h.session, h.session.ID, audioSink)
	} else {
		eb = bridge.NewEventBridge(p.Bus(), h.session, h.session.ID)
	}

	h.session.SetEventBridge(eb)

	// Start pipeline and event bridge
	if err := p.Start(ctx); err != nil {
		log.Printf("[WebRTCRealtimeServer] session %s failed to start pipeline: %v", h.session.ID, err)
		return
	}

	if err := eb.Start(ctx); err != nil {
		log.Printf("[WebRTCRealtimeServer] session %s failed to start event bridge: %v", h.session.ID, err)
		return
	}

	// Start pipeline output handler
	go h.handlePipelineOutput(ctx, p)

	log.Printf("[WebRTCRealtimeServer] session %s pipeline started", h.session.ID)
}

func (h *webrtcRealtimeEventHandler) handlePipelineOutput(ctx context.Context, p *pipeline.Pipeline) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg := p.Pull()
			if msg == nil {
				// Pipeline closed
				if eb := h.session.GetEventBridge(); eb != nil {
					eb.ForceCompleteResponse()
				}
				return
			}

			// In RTP mode, audio is handled by EventBridge via AudioSink
			// This handler is for any additional processing if needed
		}
	}
}

func (h *webrtcRealtimeEventHandler) OnAudioReceived(data []byte, sampleRate, channels int, timestamp time.Time) {
	// Push audio to session's pipeline
	h.session.PushAudio(data, sampleRate, channels)
}

func (h *webrtcRealtimeEventHandler) OnClientEvent(event events.ClientEvent) {
	// Handle Realtime API events
	if err := h.session.HandleClientEvent(event); err != nil {
		log.Printf("[WebRTCRealtimeServer] session %s failed to handle event: %v", h.session.ID, err)
	}
}

func (h *webrtcRealtimeEventHandler) OnError(err error) {
	log.Printf("[WebRTCRealtimeServer] session %s error: %v", h.session.ID, err)
	h.server.onConnectionError(h.session.Context(), h.conn, err)
}

// audioTransportSink adapts realtimeapi.AudioTransport to bridge.AudioSink.
type audioTransportSink struct {
	transport realtimeapi.AudioTransport
}

func (s *audioTransportSink) SendAudio(data []byte, sampleRate, channels int) error {
	return s.transport.SendAudio(data, sampleRate, channels)
}

func (s *audioTransportSink) SupportsRTPAudio() bool {
	return s.transport.SupportsRTPAudio()
}

// getLocalIP returns the preferred outbound IP of this machine.
func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}
