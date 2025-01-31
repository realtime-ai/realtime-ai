package elements

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/audio"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"google.golang.org/genai"
)

// Make sure GeminiElement implements pipeline.Element
var _ pipeline.Element = (*GeminiElement)(nil)

type GeminiElement struct {
	*pipeline.BaseElement

	session   *genai.Session
	sessionID string
	dumper    *audio.Dumper

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewGeminiElement() *GeminiElement {
	var dumper *audio.Dumper
	var err error

	if os.Getenv("DUMP_GEMINI_INPUT") == "true" {
		dumper, err = audio.NewDumper("gemini_input", 16000, 1)
		if err != nil {
			log.Printf("create audio dumper error: %v", err)
		}
	}

	return &GeminiElement{
		BaseElement: pipeline.NewBaseElement(100),
		dumper:      dumper,
	}
}

func (e *GeminiElement) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	apiKey := os.Getenv("GOOGLE_API_KEY")

	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey, Backend: genai.BackendGoogleAI})
	if err != nil {
		log.Fatal("create client error: ", err)
		return err
	}

	session, err := client.Live.Connect("gemini-2.0-flash-exp", &genai.LiveConnectConfig{
		ResponseModalities: []string{"AUDIO"},
	})
	if err != nil {
		log.Fatal("connect to model error: ", err)
		return err
	}

	e.session = session

	// 启动音频输入处理协程
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
				if e.session != nil {
					// 封装为 LiveClientMessage

					// dump 音频数据
					if e.dumper != nil {
						if err := e.dumper.Write(msg.AudioData.Data); err != nil {
							log.Printf("Failed to dump audio: %v", err)
						}
					}

					liveMsg := genai.LiveClientMessage{
						RealtimeInput: &genai.LiveClientRealtimeInput{
							MediaChunks: []*genai.Blob{
								{Data: msg.AudioData.Data, MIMEType: "audio/pcm"},
							},
						},
					}
					if err := e.session.Send(&liveMsg); err != nil {
						log.Println("AI session send error:", err)
						continue
					}
				}
			}
		}
	}()

	if e.session != nil {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					// 从 AI session 接收
					msg, err := e.session.Receive()
					if err != nil {
						log.Println("AI session receive error:", err)
						return
					}
					// 假设返回的 PCM 在 msg.ServerContent.ModelTurn.Parts 里
					if msg.ServerContent != nil && msg.ServerContent.ModelTurn != nil {
						log.Printf("gemini element receive %+v\n", msg.ServerContent)

						for _, part := range msg.ServerContent.ModelTurn.Parts {
							if part.InlineData != nil {
								// pcmData := part.InlineData.Data
								// 投递给下一环节

								log.Printf("gemini element receive data len %d\n", len(part.InlineData.Data))

								// todo: 将 AI 返回的 PCM 数据投递给下一环节
								e.BaseElement.OutChan <- &pipeline.PipelineMessage{
									Type:      pipeline.MsgTypeAudio,
									SessionID: e.sessionID,
									Timestamp: time.Now(),
									AudioData: &pipeline.AudioData{
										Data:       part.InlineData.Data,
										MediaType:  "audio/x-raw",
										SampleRate: 24000, // AI 返回的采样率
										Channels:   1,     // AI 返回的通道数
										Timestamp:  time.Now(),
									},
								}
							}
						}
					}
				}
			}
		}()
	}

	return nil
}

func (e *GeminiElement) Stop() error {
	if e.cancel != nil {
		e.cancel()
		e.wg.Wait()
		e.cancel = nil
	}

	if e.dumper != nil {
		e.dumper.Close()
		e.dumper = nil
	}

	// 清理 session
	e.session = nil
	e.sessionID = ""
	return nil
}
