package elements

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	openairt "github.com/WqyJh/go-openai-realtime"
	"github.com/realtime-ai/realtime-ai/pkg/audio"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/sashabaranov/go-openai"
)

// Make sure OpenAIRealtimeAPIElement implements pipeline.Element
var _ pipeline.Element = (*OpenAIRealtimeAPIElement)(nil)

type OpenAIRealtimeAPIElement struct {
	*pipeline.BaseElement

	conn      *openairt.Conn
	sessionID string
	dumper    *audio.Dumper

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewOpenAIRealtimeAPIElement() *OpenAIRealtimeAPIElement {
	var dumper *audio.Dumper
	var err error

	// 如果环境变量设置了，就创建dumper
	if os.Getenv("DUMP_OPENAI_AUDIO") == "true" {
		dumper, err = audio.NewDumper("openai_response", 24000, 1)
		if err != nil {
			log.Printf("create audio dumper error: %v", err)
		} else {
			log.Printf("Created audio dumper for OpenAI response")
		}
	}

	return &OpenAIRealtimeAPIElement{
		BaseElement: pipeline.NewBaseElement(100),
		dumper:      dumper,
	}
}

func (e *OpenAIRealtimeAPIElement) Start(ctx context.Context) error {

	client := openairt.NewClient(os.Getenv("OPENAI_API_KEY"))
	conn, err := client.Connect(context.Background())
	if err != nil {
		return err
	}
	e.conn = conn

	responseDeltaHandler := func(ctx context.Context, event openairt.ServerEvent) {
		switch event.ServerEventType() {
		case openairt.ServerEventTypeResponseAudioTranscriptDelta:
			// ignore
		case openairt.ServerEventTypeResponseAudioTranscriptDone:
			fmt.Printf("[response] %s\n", event.(openairt.ResponseAudioTranscriptDoneEvent).Transcript)
		}
	}

	// Full response
	responseHandler := func(ctx context.Context, event openairt.ServerEvent) {
		switch event.ServerEventType() {
		case openairt.ServerEventTypeConversationItemInputAudioTranscriptionCompleted:
			question := event.(openairt.ConversationItemInputAudioTranscriptionCompletedEvent).Transcript
			fmt.Printf("\n[question] %s\n", question)
		case openairt.ServerEventTypeResponseDone:
			fmt.Print("\n> ")
		}
	}

	// Log handler
	logHandler := func(ctx context.Context, event openairt.ServerEvent) {

		switch event.ServerEventType() {
		case openairt.ServerEventTypeError,
			openairt.ServerEventTypeSessionUpdated:
			data, err := json.Marshal(event)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("[%s] %s\n", event.ServerEventType(), string(data))
		}
	}

	audiobuffer := make([]byte, 0)

	audioResponseHandler := func(ctx context.Context, event openairt.ServerEvent) {
		switch event.ServerEventType() {
		case openairt.ServerEventTypeResponseAudioDelta:
			msg := event.(openairt.ResponseAudioDeltaEvent)
			// log.Printf("audioResponseHandler: %v", delta)
			data, err := base64.StdEncoding.DecodeString(msg.Delta)
			if err != nil {
				log.Fatal(err)
			}
			audiobuffer = append(audiobuffer, data...)

			// dump 音频数据
			if e.dumper != nil {
				if err := e.dumper.Write(data); err != nil {
					log.Printf("Failed to dump OpenAI audio: %v", err)
				}
			}

		case openairt.ServerEventTypeResponseAudioDone:

			data := audiobuffer[:]
			audiobuffer = make([]byte, 0)

			e.BaseElement.OutChan <- pipeline.PipelineMessage{
				Type:      pipeline.MsgTypeAudio,
				SessionID: e.sessionID,
				Timestamp: time.Now(),
				AudioData: &pipeline.AudioData{
					Data:       data,
					MediaType:  "audio/x-raw",
					SampleRate: 24000, // AI 返回的采样率
					Channels:   1,     // AI 返回的通道数
					Timestamp:  time.Now(),
				},
			}
		}
	}

	connHandler := openairt.NewConnHandler(ctx, conn, logHandler, responseHandler, responseDeltaHandler, audioResponseHandler)
	connHandler.Start()

	conn.SendMessage(ctx, openairt.SessionUpdateEvent{
		Session: openairt.ClientSession{
			Modalities:        []openairt.Modality{openairt.ModalityText, openairt.ModalityAudio},
			Voice:             openairt.VoiceShimmer,
			OutputAudioFormat: openairt.AudioFormatPcm16,
			InputAudioTranscription: &openairt.InputAudioTranscription{
				Model: openai.Whisper1,
			},
			TurnDetection: &openairt.ClientTurnDetection{
				Type: openairt.ClientTurnDetectionTypeServerVad,
				TurnDetectionParams: openairt.TurnDetectionParams{
					Threshold:         0.7,
					SilenceDurationMs: 800,
				},
			},
			MaxOutputTokens: 4000,
		},
	})

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()

		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-e.BaseElement.InChan:
				if msg.Type != pipeline.MsgTypeAudio {
					continue
				}

				if msg.AudioData.MediaType != "audio/x-raw" {
					continue
				}

				if len(msg.AudioData.Data) == 0 {
					continue
				}

				// 保存会话ID
				e.sessionID = msg.SessionID

				// 将 PCM data 发送给 AI
				if e.conn != nil {
					base64Audio := base64.StdEncoding.EncodeToString(msg.AudioData.Data)
					conn.SendMessage(ctx, openairt.InputAudioBufferAppendEvent{
						Audio: base64Audio,
					})
				}
			}
		}

	}()

	return nil
}

func (e *OpenAIRealtimeAPIElement) Stop() error {
	if e.cancel != nil {
		e.cancel()
		e.wg.Wait()
		e.cancel = nil
	}

	// 关闭 dumper
	if e.dumper != nil {
		e.dumper.Close()
		e.dumper = nil
	}

	return nil
}
