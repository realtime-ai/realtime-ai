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

type RTCServer struct {
	sync.RWMutex

	config  *ServerConfig
	peers   map[string]connection.RTCConnection
	api     *webrtc.API
	handler ServerEventHandler
}

func NewRTCServer(cfg *ServerConfig, handler ServerEventHandler) *RTCServer {
	if handler == nil {
		handler = &NoOpServerEventHandler{}
	}
	return &RTCServer{
		config:  cfg,
		handler: handler,
		peers:   make(map[string]connection.RTCConnection),
	}
}

func (s *RTCServer) Start() error {

	settingEngine := webrtc.SettingEngine{}
	// settingEngine.SetLite(true)

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
		fmt.Printf("监听 UDP 端口失败: %v\n", err)
		return err
	}

	udpMux := webrtc.NewICEUDPMux(nil, udpListener)
	settingEngine.SetICEUDPMux(udpMux)

	api := webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))

	s.api = api

	return nil

}

// HandleNegotiate 处理 /session 路由
func (s *RTCServer) HandleNegotiate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// 处理 OPTIONS 请求
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

	// 创建 PeerConnection
	pc, err := s.api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{},
	})

	if err != nil {
		s.handler.OnConnectionError(ctx, "", err)
		http.Error(w, "Failed to create peer connection", http.StatusInternalServerError)
		return
	}

	peerID := uuid.New().String()
	rtcConnection := connection.NewRTCConnection(peerID, pc)

	s.Lock()
	s.peers[peerID] = rtcConnection
	s.Unlock()

	// 通知 Handler： PeerConnection 已经创建
	s.handler.OnConnectionCreated(ctx, rtcConnection)

	// 开始协商
	if err := pc.SetRemoteDescription(offer); err != nil {
		s.handler.OnConnectionError(ctx, peerID, err)
		http.Error(w, "Failed to set remote description", http.StatusInternalServerError)
		return
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		s.handler.OnConnectionError(ctx, peerID, err)
		http.Error(w, "Failed to create answer", http.StatusInternalServerError)
		return
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		s.handler.OnConnectionError(ctx, peerID, err)
		http.Error(w, "Failed to set local description", http.StatusInternalServerError)
		return
	}

	// 等待 ICE gathering 完成
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	<-gatherComplete

	// 返回给前端
	w.Header().Set("Content-Type", "application/sdp")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(pc.LocalDescription())
}
