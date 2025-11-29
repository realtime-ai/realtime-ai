package main

import (
	"context"
	_ "embed"
	"log"
	"net/http"

	"github.com/joho/godotenv"
	"github.com/pion/webrtc/v4"
	"github.com/realtime-ai/realtime-ai/pkg/connection"
	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/server"
)

//go:embed realtime_webrtc.html
var indexHTML []byte

type connectionEventHandler struct {
	connection.ConnectionEventHandler

	conn connection.RTCConnection

	pipeline *pipeline.Pipeline
}

func (c *connectionEventHandler) OnConnectionStateChange(state webrtc.PeerConnectionState) {

	log.Printf("OnConnectionStateChange: %v \n", state)

	if state == webrtc.PeerConnectionStateConnected {

		audioPacerSinkElement := elements.NewAudioPacerSinkElement()
		geminiElement := elements.NewGeminiElement()
		audioResampleElement := elements.NewAudioResampleElement(48000, 16000, 1, 1)

		// 如果使用 Gemini，则需要添加 Gemini Element
		elements := []pipeline.Element{
			audioResampleElement,
			geminiElement,
			audioPacerSinkElement,
		}

		pipeline := pipeline.NewPipeline("rtc_connection")
		pipeline.AddElements(elements)

		pipeline.Link(audioResampleElement, geminiElement)
		pipeline.Link(geminiElement, audioPacerSinkElement)

		c.pipeline = pipeline

		pipeline.Start(context.Background())

		go func() {

			for {
				msg := c.pipeline.Pull()

				if msg != nil {
					c.conn.SendMessage(msg)
				}
			}

		}()

	}
}

func (c *connectionEventHandler) OnMessage(msg *pipeline.PipelineMessage) {

	c.pipeline.Push(msg)
}

func (c *connectionEventHandler) OnError(err error) {

	log.Printf("OnError: %v \n", err)
}

// StartServer 启动 WebRTC 服务器
func StartServer(addr string) error {

	cfg := &server.ServerConfig{
		RTCUDPPort: 9000,
	}

	rtcServer := server.NewRTCServer(cfg)

	rtcServer.OnConnectionCreated(func(ctx context.Context, conn connection.RTCConnection) {
		conn.RegisterEventHandler(&connectionEventHandler{
			conn: conn,
		})

	})

	rtcServer.OnConnectionError(func(ctx context.Context, conn connection.RTCConnection, err error) {
		log.Printf("OnConnectionError: %s, %v \n", conn.PeerID(), err)
	})

	if err := rtcServer.Start(); err != nil {
		log.Fatal(err)
	}

	// Add handler for serving the embedded HTML file
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(indexHTML)
	})
	http.HandleFunc("/session", rtcServer.HandleNegotiate)

	log.Printf("WebRTC server starting on %s", addr)
	return http.ListenAndServe(addr, nil)
}

func main() {
	godotenv.Load()
	StartServer(":8080")
}
