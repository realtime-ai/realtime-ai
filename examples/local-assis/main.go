package main

import (
	"context"
	"log"
	"os"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/pion/webrtc/v4"
	"github.com/realtime-ai/realtime-ai/pkg/audio"
	"github.com/realtime-ai/realtime-ai/pkg/connection"
	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

type connectionEventHandler struct {
	connection.ConnectionEventHandler

	conn connection.RTCConnection

	pipeline *pipeline.Pipeline
}

func (c *connectionEventHandler) OnConnectionStateChange(state webrtc.PeerConnectionState) {
	log.Printf("OnConnectionStateChange: %v", state)
}

func (c *connectionEventHandler) OnMessage(msg *pipeline.PipelineMessage) {

	c.pipeline.Push(msg)

}

func (c *connectionEventHandler) OnError(err error) {
	log.Printf("OnError: %v", err)
}

func main() {
	godotenv.Load()

	peerId := uuid.New().String()

	conn, err := connection.NewLocalConnection(peerId)
	if err != nil {
		log.Fatalf("failed to create local connection: %v", err)
	}

	eventHandler := &connectionEventHandler{
		conn: conn,
	}

	conn.RegisterEventHandler(eventHandler)

	p := pipeline.NewPipeline("local-assis")

	playoutSinkElement := elements.NewPlayoutSinkElement()
	geminiElement := elements.NewGeminiElement()

	p.AddElement(geminiElement)
	p.AddElement(playoutSinkElement)

	p.Link(geminiElement, playoutSinkElement)

	eventHandler.pipeline = p

	p.Start(context.Background())

	// 创建音频 dumper
	var dumper *audio.Dumper
	if os.Getenv("DUMP_PLAYOUT_OUTPUT") == "true" {
		var err error
		dumper, err = audio.NewDumper("playout_output", 48000, 1) // 使用48kHz采样率，单声道
		if err != nil {
			log.Printf("create audio dumper error: %v", err)
		} else {
			defer dumper.Close()
		}
	}

	go func() {

		for {
			msg := p.Pull()

			// 如果是音频数据且dumper可用，则保存
			if msg != nil && msg.Type == pipeline.MsgTypeAudio && msg.AudioData != nil && dumper != nil {
				if err := dumper.Write(msg.AudioData.Data); err != nil {
					log.Printf("dump audio data error: %v", err)
				}
			}

			conn.SendMessage(msg)
		}
	}()

	select {}
}
