package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
	"github.com/realtime-ai/realtime-ai/pkg/connection"
)

// BasicWebRTCServer is a simple WebRTC server without Realtime API protocol.
// For Realtime API support, use WebRTCRealtimeServer or WebSocketRealtimeServer.
type BasicWebRTCServer struct {
	sync.RWMutex

	config  *BasicWebRTCConfig
	peers   map[string]connection.Connection
	api     *webrtc.API
	handler ServerEventHandler

	onConnectionCreated func(ctx context.Context, conn connection.Connection)
	onConnectionError   func(ctx context.Context, conn connection.Connection, err error)
}

// NewBasicWebRTCServer creates a new BasicWebRTCServer.
func NewBasicWebRTCServer(cfg *BasicWebRTCConfig) *BasicWebRTCServer {
	return &BasicWebRTCServer{
		config:              cfg,
		onConnectionCreated: func(ctx context.Context, conn connection.Connection) {},
		onConnectionError:   func(ctx context.Context, conn connection.Connection, err error) {},
		peers:               make(map[string]connection.Connection),
	}
}

// Deprecated: NewRealtimeServer is deprecated. Use NewBasicWebRTCServer instead.
func NewRealtimeServer(cfg *BasicWebRTCConfig) *BasicWebRTCServer {
	return NewBasicWebRTCServer(cfg)
}

func (s *BasicWebRTCServer) RegisterEventHandler(handler ServerEventHandler) {
	s.handler = handler
}

func (s *BasicWebRTCServer) OnConnectionCreated(f func(ctx context.Context, conn connection.Connection)) {
	s.onConnectionCreated = f
}

func (s *BasicWebRTCServer) OnConnectionError(f func(ctx context.Context, conn connection.Connection, err error)) {
	s.onConnectionError = f
}

func (s *BasicWebRTCServer) Start() error {

	settingEngine := webrtc.SettingEngine{}
	if s.config.ICELite {
		settingEngine.SetLite(true)
	}

	if len(s.config.Endpoint) > 0 {
		settingEngine.SetNAT1To1IPs(s.config.Endpoint, webrtc.ICECandidateTypeHost)
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
		fmt.Printf("Failed to listen on UDP port: %v\n", err)
		return err
	}

	udpMux := webrtc.NewICEUDPMux(nil, udpListener)
	settingEngine.SetICEUDPMux(udpMux)

	api := webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))

	s.api = api

	return nil

}

// HandleNegotiate handles the /session WebRTC negotiation endpoint.
func (s *BasicWebRTCServer) HandleNegotiate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Handle OPTIONS request for CORS preflight
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

	peerID := uuid.New().String()
	webrtcConn := connection.NewWebRTCConnection(peerID, pc)

	s.Lock()
	s.peers[peerID] = webrtcConn
	s.Unlock()

	// Notify handler: connection created
	s.onConnectionCreated(ctx, webrtcConn)

	// Start negotiation
	if err := pc.SetRemoteDescription(offer); err != nil {
		s.onConnectionError(ctx, webrtcConn, err)
		http.Error(w, "Failed to set remote description", http.StatusInternalServerError)
		return
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		s.onConnectionError(ctx, webrtcConn, err)
		http.Error(w, "Failed to create answer", http.StatusInternalServerError)
		return
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		s.onConnectionError(ctx, webrtcConn, err)
		http.Error(w, "Failed to set local description", http.StatusInternalServerError)
		return
	}

	// Wait for ICE gathering to complete
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	<-gatherComplete

	// Return SDP answer to client
	w.Header().Set("Content-Type", "application/sdp")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(pc.LocalDescription())
}
