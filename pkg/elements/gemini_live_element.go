package elements

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/audio"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"google.golang.org/genai"
)

// Make sure GeminiLiveElement implements pipeline.Element
var _ pipeline.Element = (*GeminiLiveElement)(nil)

// Deprecated: Use GeminiLiveElement, GeminiLiveConfig, etc. instead
type GeminiElement = GeminiLiveElement
type GeminiConfig = GeminiLiveConfig

const DefaultGeminiModel = DefaultGeminiLiveModel

// Deprecated: Use NewGeminiLiveElement instead
func NewGeminiElement() *GeminiLiveElement {
	return NewGeminiLiveElement()
}

// Deprecated: Use NewGeminiLiveElementWithConfig instead
func NewGeminiElementWithConfig(cfg GeminiLiveConfig) *GeminiLiveElement {
	return NewGeminiLiveElementWithConfig(cfg)
}

// Deprecated: Use DefaultGeminiLiveConfig instead
func DefaultGeminiConfig() GeminiLiveConfig {
	return DefaultGeminiLiveConfig()
}

// DefaultGeminiLiveModel is the default model used by GeminiLiveElement
const DefaultGeminiLiveModel = "gemini-2.5-flash-native-audio-preview-12-2025"

// GeminiLiveConfig holds configuration for GeminiLiveElement
type GeminiLiveConfig struct {
	// Model is the Gemini model to use (default: gemini-2.5-flash-native-audio-preview-12-2025)
	Model string
	// APIKey is the Google API key (default: from GOOGLE_API_KEY env)
	APIKey string
}

// DefaultGeminiLiveConfig returns the default configuration
func DefaultGeminiLiveConfig() GeminiLiveConfig {
	return GeminiLiveConfig{
		Model:  DefaultGeminiLiveModel,
		APIKey: os.Getenv("GOOGLE_API_KEY"),
	}
}

type GeminiLiveElement struct {
	*pipeline.BaseElement

	model     string
	apiKey    string
	session   *genai.Session
	sessionID string
	dumper    *audio.Dumper

	// Response tracking
	inResponse        bool
	currentResponseID string

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewGeminiLiveElement creates a new GeminiLiveElement with default configuration
func NewGeminiLiveElement() *GeminiLiveElement {
	return NewGeminiLiveElementWithConfig(DefaultGeminiLiveConfig())
}

// NewGeminiLiveElementWithConfig creates a new GeminiLiveElement with custom configuration
func NewGeminiLiveElementWithConfig(cfg GeminiLiveConfig) *GeminiLiveElement {
	var dumper *audio.Dumper
	var err error

	if os.Getenv("DUMP_GEMINI_INPUT") == "true" {
		dumper, err = audio.NewDumper("gemini_live_input", 16000, 1)
		if err != nil {
			log.Printf("create audio dumper error: %v", err)
		}
	}

	model := cfg.Model
	if model == "" {
		model = DefaultGeminiLiveModel
	}

	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("GOOGLE_API_KEY")
	}

	return &GeminiLiveElement{
		BaseElement: pipeline.NewBaseElement("gemini-live-element", 100),
		model:       model,
		apiKey:      apiKey,
		dumper:      dumper,
	}
}

// Implement Element interface

func (e *GeminiLiveElement) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: e.apiKey, Backend: genai.BackendGoogleAI})
	if err != nil {
		log.Printf("[GEMINI] create client error: %v", err)
		return err
	}

	log.Printf("[GEMINI] 正在连接模型: %s", e.model)
	session, err := client.Live.Connect(e.model, &genai.LiveConnectConfig{
		ResponseModalities: []string{"AUDIO"},
	})
	if err != nil {
		log.Printf("[GEMINI] connect to model error: %v", err)
		return err
	}

	e.session = session
	log.Printf("[GEMINI] 成功连接到 Gemini Live API (模型: %s)", e.model)

	// 启动输入处理协程（音频、图像、文本）
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-e.BaseElement.InChan:
				if msg.Type == pipeline.MsgTypeAudio {
					if msg.AudioData.MediaType != pipeline.AudioMediaTypeRaw || len(msg.AudioData.Data) == 0 {
						continue
					}

					if e.session != nil {
						// 封装为 LiveClientMessage
						liveMsg := genai.LiveClientMessage{
							RealtimeInput: &genai.LiveClientRealtimeInput{
								MediaChunks: []*genai.Blob{
									{Data: msg.AudioData.Data, MIMEType: string(pipeline.AudioMediaTypePCM)},
								},
							},
						}

						if err := e.session.Send(&liveMsg); err != nil {
							log.Println("[GEMINI] AI session send error:", err)
							continue
						}
					} else {
						log.Println("[GEMINI] session 为空，无法发送音频")
					}
				} else if msg.Type == pipeline.MsgTypeImage {
					// 处理图像消息
					if msg.ImageData == nil || len(msg.ImageData.Data) == 0 {
						continue
					}

					if e.session != nil {
						log.Printf("[GEMINI] 发送图像到 Gemini: %s, %d bytes", msg.ImageData.MIMEType, len(msg.ImageData.Data))

						// 将图像作为 ClientContent 发送
						liveMsg := genai.LiveClientMessage{
							ClientContent: &genai.LiveClientContent{
								Turns: []*genai.Content{
									{
										Role: "user",
										Parts: []*genai.Part{
											{
												InlineData: &genai.Blob{
													MIMEType: msg.ImageData.MIMEType,
													Data:     msg.ImageData.Data,
												},
											},
										},
									},
								},
								TurnComplete: true,
							},
						}

						if err := e.session.Send(&liveMsg); err != nil {
							log.Println("[GEMINI] AI session send image error:", err)
							continue
						}
						log.Printf("[GEMINI] 图像发送成功")
					} else {
						log.Println("[GEMINI] session 为空，无法发送图像")
					}
				} else if msg.Type == pipeline.MsgTypeData {
					liveMsg := genai.LiveClientMessage{}
					err := json.Unmarshal([]byte(msg.TextData.Data), &liveMsg)
					if err != nil {
						log.Println("AI session send error:", err)
						continue
					}

					if liveMsg.ClientContent != nil || liveMsg.RealtimeInput != nil {
						if err := e.session.Send(&liveMsg); err != nil {
							log.Println("AI session send error:", err)
							continue
						}
					}
				} else {
					// 投递给下一环节
					e.BaseElement.OutChan <- msg
				}
			}
		}
	}()

	if e.session != nil {
		go func() {
			log.Println("[GEMINI] 开始监听 Gemini 响应...")
			for {
				select {
				case <-ctx.Done():
					// If we're in a response, end it
					if e.inResponse {
						e.endCurrentResponse("cancelled")
					}
					return
				default:
					// 从 AI session 接收
					msg, err := e.session.Receive()
					if err != nil {
						log.Println("AI session receive error:", err)
						// End any active response on error
						if e.inResponse {
							e.endCurrentResponse("error")
						}
						return
					}

					// Handle interruption first
					if msg.ServerContent != nil && msg.ServerContent.Interrupted {
						log.Println("AI session interrupted")
						// End current response if any
						if e.inResponse {
							e.endCurrentResponse("interrupted")
						}
						// Publish interrupt event with proper payload
						e.BaseElement.Bus().Publish(pipeline.Event{
							Type:      pipeline.EventInterrupted,
							Timestamp: time.Now(),
							Payload: &pipeline.VADPayload{
								AudioMs: 0,
								ItemID:  "",
							},
						})
						continue
					}

					// 假设返回的 PCM 在 msg.ServerContent.ModelTurn.Parts 里
					if msg.ServerContent != nil && msg.ServerContent.ModelTurn != nil {
						for _, part := range msg.ServerContent.ModelTurn.Parts {
							if part.InlineData != nil && len(part.InlineData.Data) > 0 {
								log.Printf("[GEMINI] 收到 Gemini 音频响应: %d bytes", len(part.InlineData.Data))
								// Start response if not already started
								if !e.inResponse {
									e.startNewResponse()
								}

								// Publish audio delta event to bus
								// e.BaseElement.Bus().Publish(pipeline.Event{
								// 	Type:      pipeline.EventAudioDelta,
								// 	Timestamp: time.Now(),
								// 	Payload: &pipeline.AudioDeltaPayload{
								// 		ResponseID: e.currentResponseID,
								// 		Data:       part.InlineData.Data,
								// 		SampleRate: 24000,
								// 		Channels:   1,
								// 	},
								// })

								// 将 AI 返回的 PCM 数据投递给下一环节
								e.BaseElement.OutChan <- &pipeline.PipelineMessage{
									Type:      pipeline.MsgTypeAudio,
									SessionID: e.sessionID,
									Timestamp: time.Now(),
									AudioData: &pipeline.AudioData{
										Data:       part.InlineData.Data,
										MediaType:  pipeline.AudioMediaTypeRaw,
										SampleRate: 24000, // AI 返回的采样率
										Channels:   1,     // AI 返回的通道数
										Timestamp:  time.Now(),
									},
								}
							}
						}
					}

					// Check if turn is complete
					if msg.ServerContent != nil && msg.ServerContent.TurnComplete {
						if e.inResponse {
							e.endCurrentResponse("completed")
						}
					}
				}
			}
		}()
	}

	return nil
}

func (e *GeminiLiveElement) Stop() error {
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

// startNewResponse starts tracking a new response.
func (e *GeminiLiveElement) startNewResponse() {
	e.currentResponseID = generateResponseID()
	e.inResponse = true

	// Publish response start event
	e.BaseElement.Bus().Publish(pipeline.Event{
		Type:      pipeline.EventResponseStart,
		Timestamp: time.Now(),
		Payload: &pipeline.ResponseStartPayload{
			ResponseID: e.currentResponseID,
		},
	})
}

// endCurrentResponse ends the current response.
func (e *GeminiLiveElement) endCurrentResponse(reason string) {
	if !e.inResponse {
		return
	}

	completed := reason == "completed"

	// Publish response end event
	e.BaseElement.Bus().Publish(pipeline.Event{
		Type:      pipeline.EventResponseEnd,
		Timestamp: time.Now(),
		Payload: &pipeline.ResponseEndPayload{
			ResponseID: e.currentResponseID,
			Completed:  completed,
			Reason:     reason,
		},
	})

	e.inResponse = false
	e.currentResponseID = ""
}

// generateResponseID generates a unique response ID.
func generateResponseID() string {
	return "resp_" + time.Now().Format("20060102150405") + "_" + randomString(4)
}

// randomString generates a random string of specified length.
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
		time.Sleep(time.Nanosecond)
	}
	return string(b)
}
