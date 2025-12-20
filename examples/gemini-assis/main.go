package main

import (
	"context"
	_ "embed"
	"log"
	"net/http"

	"github.com/joho/godotenv"
	"github.com/realtime-ai/realtime-ai/pkg/connection"
	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/server"
)

//go:embed realtime_webrtc.html
var indexHTML []byte

type connectionEventHandler struct {
	connection.ConnectionEventHandler

	conn connection.Connection

	pipeline *pipeline.Pipeline
}

func (c *connectionEventHandler) OnConnectionStateChange(state connection.ConnectionState) {

	log.Printf("OnConnectionStateChange: %v \n", state)

	if state == connection.ConnectionStateConnected {

		audioPacerSinkElement := elements.NewAudioPacerSinkElement()
		geminiElement := elements.NewGeminiElementWithConfig(elements.GeminiConfig{
			Model: elements.DefaultGeminiModel, // gemini-2.5-flash-native-audio-preview-12-2025
		})
		audioResampleElement := elements.NewAudioResampleElement(48000, 16000, 1, 1)

		// 如果使用 Gemini，则需要添加 Gemini Element
		pipelineElements := []pipeline.Element{
			audioResampleElement,
			geminiElement,
			audioPacerSinkElement,
		}

		p := pipeline.NewPipeline("rtc_connection")
		p.AddElements(pipelineElements)

		p.Link(audioResampleElement, geminiElement)
		p.Link(geminiElement, audioPacerSinkElement)

		c.pipeline = p

		p.Start(context.Background())

		go func() {
			log.Println("[OUTPUT] 开始监听 Pipeline 输出...")
			outputFrameCount := 0
			for {
				msg := c.pipeline.Pull()

				if msg != nil {
					if msg.Type == pipeline.MsgTypeAudio && msg.AudioData != nil {
						outputFrameCount++
						// 只有非 24000Hz（非 Gemini 响应）才减少日志，Gemini 响应全部打印
						if msg.AudioData.SampleRate == 24000 || outputFrameCount%100 == 1 {
							log.Printf("[OUTPUT] 发送音频 #%d: %d bytes, 采样率: %d",
								outputFrameCount, len(msg.AudioData.Data), msg.AudioData.SampleRate)
						}
					}
					c.conn.SendMessage(msg)
				}
			}
		}()

	}
}

var inputFrameCount int

func (c *connectionEventHandler) OnMessage(msg *pipeline.PipelineMessage) {
	if msg.Type == pipeline.MsgTypeAudio && msg.AudioData != nil {
		inputFrameCount++
		if inputFrameCount%100 == 1 { // 每 100 帧打印一次
			log.Printf("[INPUT] 收到音频 #%d: %d bytes, 采样率: %d",
				inputFrameCount, len(msg.AudioData.Data), msg.AudioData.SampleRate)
		}
	}
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

	rtcServer := server.NewRealtimeServer(cfg)

	rtcServer.OnConnectionCreated(func(ctx context.Context, conn connection.Connection) {
		conn.RegisterEventHandler(&connectionEventHandler{
			conn: conn,
		})

	})

	rtcServer.OnConnectionError(func(ctx context.Context, conn connection.Connection, err error) {
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
	if err := StartServer(":8080"); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
