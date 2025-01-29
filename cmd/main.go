package main

import (
	"log"
	"net/http"

	"github.com/joho/godotenv"
	"github.com/realtime-ai/gemini-realtime-webrtc/pkg/server"
)

// StartServer 启动 WebRTC 服务器
func StartServer(addr string) error {

	cfg := &server.ServerConfig{
		RTCUDPPort: 9000,
	}

	rtcServer := server.NewRTCServer(cfg, nil)
	if err := rtcServer.Start(); err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/session", rtcServer.HandleNegotiate)

	log.Printf("WebRTC server starting on %s", addr)
	return http.ListenAndServe(addr, nil)
}

func main() {
	godotenv.Load()
	// if err := gateway.StartServer(":8080"); err != nil {
	// 	log.Fatal(err)
	// }

	StartServer(":8080")
}
