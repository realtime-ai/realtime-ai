package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/joho/godotenv"
	"github.com/pion/webrtc/v4"
	"github.com/realtime-ai/realtime-ai/pkg/connection"
	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/server"
)

type GeminiAssistantHandler struct {
	server.ServerEventHandler
}

func (g *GeminiAssistantHandler) OnConnectionCreated(ctx context.Context, conn connection.RTCConnection) {

	conn.PeerConnection().OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("OnTrack: %s, %v \n", conn.PeerID(), track)

		webrtcSinkElement := elements.NewWebRTCSinkElement(conn.LocalAudioTrack())
		geminiElement := elements.NewGeminiElement()

		opusDecodeElement := elements.NewOpusDecodeElement(48000, 1)
		inAudioResampleElement := elements.NewAudioResampleElement(48000, 16000, 1, 1)

		// 如果使用 Gemini，则需要添加 Gemini Element
		elements := []pipeline.Element{
			opusDecodeElement,
			inAudioResampleElement,
			geminiElement,
			webrtcSinkElement,
		}

		pipeline := pipeline.NewPipeline("rtc_connection", nil)
		pipeline.AddElements(elements)

		pipeline.Link(opusDecodeElement, inAudioResampleElement)
		pipeline.Link(inAudioResampleElement, geminiElement)
		pipeline.Link(geminiElement, webrtcSinkElement)

		pipeline.Start(ctx)

		go g.readRemoteTrack(conn, opusDecodeElement, track)

	})

}

func (g *GeminiAssistantHandler) OnPeerConnectionError(ctx context.Context, peerID string, err error) {

	log.Printf("OnPeerConnectionError: %s, %v \n", peerID, err)
}

func (g *GeminiAssistantHandler) readRemoteTrack(conn connection.RTCConnection, opusDecodeElement *elements.OpusDecodeElement, track *webrtc.TrackRemote) {

	log.Printf("readRemoteTrack: %s, %v \n", conn.PeerID(), track)
	for {

		rtpPacket, _, err := track.ReadRTP()
		if err != nil {
			log.Println("read RTP error:", err)
			continue
		}

		// 将拿到的 payload 投递给 pipeline 的“输入 element”
		msg := &pipeline.PipelineMessage{
			Type: pipeline.MsgTypeAudio,
			AudioData: &pipeline.AudioData{
				Data:       rtpPacket.Payload,
				SampleRate: 48000,
				Channels:   1,
				MediaType:  "audio/x-opus",
				Codec:      "opus",
				Timestamp:  time.Now(),
			},
		}

		opusDecodeElement.In() <- msg

	}

}

// StartServer 启动 WebRTC 服务器
func StartServer(addr string) error {

	cfg := &server.ServerConfig{
		RTCUDPPort: 9000,
	}

	handler := &GeminiAssistantHandler{}
	rtcServer := server.NewRTCServer(cfg, handler)
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
