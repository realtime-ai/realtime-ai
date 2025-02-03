package main

import (
	"context"
	"log"
	"net/http"

	"github.com/joho/godotenv"
	"github.com/pion/webrtc/v4"
	"github.com/realtime-ai/realtime-ai/pkg/connection"
	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/server"
)

type connectionEventHandler struct {
	connection.ConnectionEventHandler

	conn connection.RTCConnection

	pipeline             *pipeline.Pipeline
	geminiElement        *elements.GeminiElement
	audioResampleElement *elements.AudioResampleElement
	playoutSinkElement   *elements.PlayoutSinkElement
}

func (c *connectionEventHandler) OnConnectionStateChange(state webrtc.PeerConnectionState) {

	log.Printf("OnConnectionStateChange: %v \n", state)

	if state == webrtc.PeerConnectionStateConnected {

		playoutSinkElement := elements.NewPlayoutSinkElement()
		geminiElement := elements.NewGeminiElement()
		audioResampleElement := elements.NewAudioResampleElement(48000, 16000, 1, 1)

		// 如果使用 Gemini，则需要添加 Gemini Element
		elements := []pipeline.Element{
			audioResampleElement,
			geminiElement,
			playoutSinkElement,
		}

		pipeline := pipeline.NewPipeline("rtc_connection")
		pipeline.AddElements(elements)

		pipeline.Link(audioResampleElement, geminiElement)
		pipeline.Link(geminiElement, playoutSinkElement)

		c.pipeline = pipeline
		c.geminiElement = geminiElement
		c.playoutSinkElement = playoutSinkElement
		c.audioResampleElement = audioResampleElement
		pipeline.Start(context.Background())

		go func() {
			for msg := range c.playoutSinkElement.Out() {
				c.conn.SendMessage(msg)
			}
		}()

	}
}

func (c *connectionEventHandler) OnMessage(msg *pipeline.PipelineMessage) {

	select {
	case c.audioResampleElement.In() <- msg:
	default:
		log.Println("audio resample element in chan is full")
	}
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

	http.HandleFunc("/session", rtcServer.HandleNegotiate)

	log.Printf("WebRTC server starting on %s", addr)
	return http.ListenAndServe(addr, nil)
}

func main() {
	godotenv.Load()
	StartServer(":8080")
}
