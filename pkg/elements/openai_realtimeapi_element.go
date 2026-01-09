package elements

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"reflect"
	"sync"
	"time"

	openairt "github.com/WqyJh/go-openai-realtime/v2"
	"github.com/realtime-ai/realtime-ai/pkg/audio"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

// Make sure OpenAIRealtimeAPIElement implements pipeline.Element
var _ pipeline.Element = (*OpenAIRealtimeAPIElement)(nil)

type OpenAIRealtimeAPIElement struct {
	*pipeline.BaseElement

	model      string
	disableVad bool
	prompt     string

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

	elem := &OpenAIRealtimeAPIElement{
		BaseElement: pipeline.NewBaseElement("openai-realtime-element", 100),
		dumper:      dumper,
		model:       "gpt-realtime",
	}
	elem.registerProperties()
	return elem
}

func (e *OpenAIRealtimeAPIElement) registerProperties() {
	e.RegisterProperty(pipeline.PropertyDesc{
		Name:     "prompt",
		Type:     reflect.TypeOf(""),
		Writable: true,
		Readable: true,
		Default:  e.prompt,
	})
	// 绑定其他变量
}

func (e *OpenAIRealtimeAPIElement) Start(ctx context.Context) error {

	apiKey := os.Getenv("OPENAI_API_KEY")
	// baseURL := os.Getenv("OPENAI_BASE_URL")

	client := openairt.NewClient(apiKey)
	// if baseURL != "" {
	// 	client.BaseURL = baseURL
	// }
	conn, err := client.Connect(context.Background(), openairt.WithModel(e.model))
	if err != nil {
		log.Printf("Failed to connect to OpenAI: %v", err)
		return err
	}
	e.conn = conn
	log.Println("openai connect success")

	responseDeltaHandler := func(ctx context.Context, event openairt.ServerEvent) {
		switch event.ServerEventType() {
		case openairt.ServerEventTypeResponseOutputAudioTranscriptDelta:
			rsp := event.(openairt.ResponseOutputAudioTranscriptDeltaEvent)
			payload := &pipeline.TextDeltaPayload{
				Text: rsp.Delta,
			}
			e.Bus().Publish(pipeline.Event{
				Type:      pipeline.EventTextDelta,
				Timestamp: time.Now(),
				Payload:   payload,
			})
			// ignore
		case openairt.ServerEventTypeResponseOutputAudioTranscriptDone:
			log.Printf("[response] %s\n", event.(openairt.ResponseOutputAudioTranscriptDoneEvent).Transcript)
			e.Bus().Publish(pipeline.Event{
				Type:      pipeline.EventTextDelta,
				Timestamp: time.Now(),
				Payload: &pipeline.TextDeltaPayload{
					IsFinal: true,
				},
			})
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
		default:
			// data, err := json.Marshal(event)
			// if err != nil {
			// 	log.Fatal(err)
			// }
			// if len(data) > 200 {
			// 	data = data[:200]
			// }
			// fmt.Printf("[%s] %s\n", event.ServerEventType(), string(data))
		}
	}

	audioResponseHandler := func(ctx context.Context, event openairt.ServerEvent) {
		switch event.ServerEventType() {
		case openairt.ServerEventTypeResponseOutputAudioDelta:
			msg := event.(openairt.ResponseOutputAudioDeltaEvent)
			data, err := base64.StdEncoding.DecodeString(msg.Delta)
			if err != nil {
				log.Fatal(err)
			}

			// dump 音频数据
			if e.dumper != nil {
				if err := e.dumper.Write(data); err != nil {
					log.Printf("Failed to dump OpenAI audio: %v", err)
				}
			}

			e.BaseElement.OutChan <- &pipeline.PipelineMessage{
				Type:      pipeline.MsgTypeAudio,
				SessionID: e.sessionID,
				Timestamp: time.Now(),
				AudioData: &pipeline.AudioData{
					Data:       data,
					MediaType:  pipeline.AudioMediaTypeRaw,
					SampleRate: 24000, // AI 返回的采样率
					Channels:   1,     // AI 返回的通道数
					Timestamp:  time.Now(),
				},
			}

		case openairt.ServerEventTypeResponseOutputAudioDone:
			log.Printf("[OpenAI] 收到 OpenAI 音频结束")

		case openairt.ServerEventTypeInputAudioBufferSpeechStarted:
			// Interrupt the current response
			log.Println("[OpenAI] Input audio buffer speech started")
			// msg := event.(openairt.InputAudioBufferSpeechStartedEvent)
			// e.Bus().Publish(pipeline.Event{
			// 	Type:      pipeline.EventInterrupted,
			// 	Timestamp: time.Now(),
			// 	Payload:   msg,
			// })
		case openairt.ServerEventTypeInputAudioBufferSpeechStopped:
			log.Println("[OpenAI] Input audio buffer speech stopped")
		}
	}

	connHandler := openairt.NewConnHandler(ctx, conn, logHandler, responseHandler, responseDeltaHandler, audioResponseHandler)
	connHandler.Start()

	updateEvent := openairt.SessionUpdateEvent{
		Session: openairt.SessionUnion{
			Realtime: &openairt.RealtimeSession{
				Model:            e.model,
				OutputModalities: []openairt.Modality{openairt.ModalityAudio},
				Audio: &openairt.RealtimeSessionAudio{
					Input: &openairt.SessionAudioInput{
						Format: &openairt.AudioFormatUnion{
							PCM: &openairt.AudioFormatPCM{
								Rate: 24000,
							},
						},
						Transcription: &openairt.AudioTranscription{
							Language: "en",
							Model:    "gpt-4o-mini-transcribe",
						},
					},
					Output: &openairt.SessionAudioOutput{
						Format: &openairt.AudioFormatUnion{
							PCM: &openairt.AudioFormatPCM{
								Rate: 24000,
							},
						},
						Voice: openairt.VoiceShimmer,
					},
				},
				MaxOutputTokens: 4000,
				Instructions:    "You are a helpful, witty, and friendly AI. Your voice and personality should be warm and engaging, with a lively and playful tone. If interacting in a non-English language, start by using the user's language to response.",
			},
		},
	}

	if !e.disableVad {
		updateEvent.Session.Realtime.Audio.Input.TurnDetection = &openairt.TurnDetectionUnion{
			ServerVad: &openairt.ServerVad{
				Threshold:         0.8,
				SilenceDurationMs: 200,
			},
		}
		updateEvent.Session.Realtime.Audio.Input.NoiseReduction = &openairt.AudioNoiseReduction{
			Type: "near_field", // 耳机模式：near_field，远场模式：far_field
		}
	}

	if e.prompt != "" {
		updateEvent.Session.Realtime.Instructions = e.prompt
	}

	conn.SendMessage(ctx, updateEvent)

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()

		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-e.BaseElement.InChan:

				if msg.Type == pipeline.MsgTypeAudio {

					if msg.AudioData.MediaType != pipeline.AudioMediaTypeRaw {
						continue
					}

					// 将 PCM data 发送给 AI
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
				} else if msg.Type == pipeline.MsgTypeData {
					clientEvent, err := UnmarshalClientEvent(msg.TextData.Data)
					if err != nil {
						log.Println("AI session send error:", err)
						continue
					}
					// 发送消息给 OpenAI
					if err := e.conn.SendMessage(ctx, clientEvent); err != nil {
						log.Println("AI session send error:", err)
						continue
					}
				} else {
					// 投递给下一环节
					e.BaseElement.OutChan <- msg
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

// UnmarshalClientEvent unmarshals the client event from the given JSON data.
func UnmarshalClientEvent(data []byte) (openairt.ClientEvent, error) {
	var eventType struct {
		Type openairt.ClientEventType `json:"type"`
	}
	err := json.Unmarshal(data, &eventType)
	if err != nil {
		return nil, err
	}

	switch eventType.Type {
	case openairt.ClientEventTypeSessionUpdate:
		return unmarshalClientEvent[openairt.SessionUpdateEvent](data)
	case openairt.ClientEventTypeInputAudioBufferAppend:
		return unmarshalClientEvent[openairt.InputAudioBufferAppendEvent](data)
	case openairt.ClientEventTypeInputAudioBufferCommit:
		return unmarshalClientEvent[openairt.InputAudioBufferCommitEvent](data)
	case openairt.ClientEventTypeInputAudioBufferClear:
		return unmarshalClientEvent[openairt.InputAudioBufferClearEvent](data)
	case openairt.ClientEventTypeConversationItemCreate:
		return unmarshalClientEvent[openairt.ConversationItemCreateEvent](data)
	case openairt.ClientEventTypeConversationItemTruncate:
		return unmarshalClientEvent[openairt.ConversationItemTruncateEvent](data)
	case openairt.ClientEventTypeConversationItemDelete:
		return unmarshalClientEvent[openairt.ConversationItemDeleteEvent](data)
	case openairt.ClientEventTypeResponseCreate:
		return unmarshalClientEvent[openairt.ResponseCreateEvent](data)
	case openairt.ClientEventTypeResponseCancel:
		return unmarshalClientEvent[openairt.ResponseCancelEvent](data)
	}

	return nil, fmt.Errorf("unknown client event type: %s", eventType.Type)
}

// unmarshalClientEvent unmarshals the client event from the given JSON data.
func unmarshalClientEvent[T openairt.ClientEvent](data []byte) (T, error) {
	var t T
	err := json.Unmarshal(data, &t)
	if err != nil {
		return t, err
	}
	return t, nil
}

// SetProperty sets a property value at runtime.
func (e *OpenAIRealtimeAPIElement) SetProperty(name string, value interface{}) error {
	switch name {
	case "prompt":
		if prompt, ok := value.(string); ok {
			e.prompt = prompt
		}
	case "model":
		if model, ok := value.(string); ok {
			e.model = model
		}
	}

	return e.BaseElement.SetProperty(name, value)
}
