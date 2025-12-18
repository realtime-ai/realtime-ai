package main

import (
	"context"
	"log"
	"os"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/realtime-ai/realtime-ai/pkg/audio"
	"github.com/realtime-ai/realtime-ai/pkg/connection"
	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

type connectionEventHandler struct {
	connection.ConnectionEventHandler

	conn connection.Connection

	pipeline *pipeline.Pipeline
}

func (c *connectionEventHandler) OnConnectionStateChange(state connection.ConnectionState) {
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

	audioPacerSinkElement := elements.NewAudioPacerSinkElement()
	geminiElement := elements.NewGeminiElement()

	p.AddElement(geminiElement)
	p.AddElement(audioPacerSinkElement)

	p.Link(geminiElement, audioPacerSinkElement)

	eventHandler.pipeline = p

	p.Start(context.Background())

	// 创建音频 dumper
	var dumper *audio.Dumper
	if os.Getenv("DUMP_PACER_OUTPUT") == "true" {
		var err error
		dumper, err = audio.NewDumper("pacer_output", 48000, 1) // 使用48kHz采样率，单声道
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
