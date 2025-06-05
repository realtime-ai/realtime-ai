package elements

import (
	"context"
	"fmt"
	"log"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/Microsoft/cognitive-services-speech-sdk-go/audio"
	"github.com/Microsoft/cognitive-services-speech-sdk-go/common"
	"github.com/Microsoft/cognitive-services-speech-sdk-go/speech"

	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

// Make sure AzureSTTElement implements pipeline.Element
var _ pipeline.Element = (*AzureSTTElement)(nil)

type AzureSTTElement struct {
	*pipeline.BaseElement

	subscriptionKey string
	region          string
	language        string

	// VAD 配置
	enableVAD               bool   // 是否启用 VAD
	silenceTimeoutMs        string // 静音超时时间（毫秒）
	initialSilenceTimeoutMs string // 初始静音超时时间（毫秒）

	// 音频格式配置
	sampleRate int
	channels   int

	recognizer  *speech.SpeechRecognizer
	pushStream  *audio.PushAudioInputStream
	audioConfig *audio.AudioConfig

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewAzureSTTElement() *AzureSTTElement {
	return &AzureSTTElement{
		BaseElement:     pipeline.NewBaseElement(100),
		subscriptionKey: os.Getenv("AZURE_SPEECH_KEY"),
		region:          os.Getenv("AZURE_SPEECH_REGION"),
		language:        "zh-CN", // 默认使用中文，可以通过 SetProperty 修改

		// VAD 默认配置
		enableVAD:               true,
		silenceTimeoutMs:        "500",  // 默认 500ms 静音超时
		initialSilenceTimeoutMs: "5000", // 默认 5s 初始静音超时

		// 音频格式配置
		sampleRate: 16000, // 16kHz
		channels:   1,     // 单声道
	}
}

func (e *AzureSTTElement) Start(ctx context.Context) error {
	if e.subscriptionKey == "" || e.region == "" {
		return fmt.Errorf("Azure Speech credentials not set")
	}

	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	// 创建推流，默认音频格式为 16kHz 单声道,
	var err error
	e.pushStream, err = audio.CreatePushAudioInputStream()
	if err != nil {
		return fmt.Errorf("failed to create push stream: %v", err)
	}

	// 创建音频配置
	e.audioConfig, err = audio.NewAudioConfigFromStreamInput(e.pushStream)
	if err != nil {
		return fmt.Errorf("failed to create audio config: %v", err)
	}

	// 创建语音配置
	speechConfig, err := speech.NewSpeechConfigFromSubscription(e.subscriptionKey, e.region)
	if err != nil {
		return fmt.Errorf("failed to create speech config: %v", err)
	}

	// 设置语言
	speechConfig.SetSpeechRecognitionLanguage(e.language)

	// 设置音频日志和 VAD 参数
	speechConfig.SetPropertyByString("SPEECH-EnableAudioLogging", "true")
	speechConfig.SetProperty(common.SegmentationSilenceTimeoutMs, e.silenceTimeoutMs)
	speechConfig.SetProperty(common.ConversationInitialSilenceTimeout, e.initialSilenceTimeoutMs)

	// 配置 VAD
	// if e.enableVAD {
	// 	// 设置 VAD 开启
	// 	speechConfig.SetPropertyByString("SPEECH-VoiceDetectionEnabled", "true")
	// 	// 设置静音超时
	// 	speechConfig.SetPropertyByString("SPEECH-EndSilenceTimeoutMs", e.silenceTimeoutMs)
	// 	// 设置初始静音超时
	// 	speechConfig.SetPropertyByString("SPEECH-InitialSilenceTimeoutMs", e.initialSilenceTimeoutMs)
	// } else {
	// 	speechConfig.SetPropertyByString("SPEECH-VoiceDetectionEnabled", "false")
	// }

	// 创建识别器
	e.recognizer, err = speech.NewSpeechRecognizerFromConfig(speechConfig, e.audioConfig)
	if err != nil {
		return fmt.Errorf("failed to create recognizer: %v", err)
	}

	// 处理会话开始事件
	e.recognizer.SessionStarted(func(evt speech.SessionEventArgs) {
		log.Printf("Speech recognition session started")
		// 发布会话开始事件到总线
	})

	// 处理会话结束事件
	e.recognizer.SessionStopped(func(evt speech.SessionEventArgs) {
		log.Printf("Speech recognition session stopped")
	})

	// 处理部分识别结果
	e.recognizer.Recognizing(func(evt speech.SpeechRecognitionEventArgs) {
		if evt.Result.Reason == common.RecognizingSpeech {
			// 创建部分识别结果消息
			msg := &pipeline.PipelineMessage{
				Type: pipeline.MsgTypeData,
				TextData: &pipeline.TextData{
					Data:      []byte(evt.Result.Text),
					TextType:  "text/partial", // 标记为部分结果
					Timestamp: time.Now(),
				},
			}
			// 发送到输出通道
			e.BaseElement.OutChan <- msg

			// 发布部分结果事件到总线
			e.BaseElement.Bus().Publish(pipeline.Event{
				Type:      pipeline.EventPartialResult,
				Timestamp: time.Now(),
				Payload:   evt.Result.Text,
			})
		}
	})

	// 处理最终识别结果
	e.recognizer.Recognized(func(evt speech.SpeechRecognitionEventArgs) {
		if evt.Result.Reason == common.RecognizedSpeech {
			// 创建最终识别结果消息
			msg := &pipeline.PipelineMessage{
				Type: pipeline.MsgTypeData,
				TextData: &pipeline.TextData{
					Data:      []byte(evt.Result.Text),
					TextType:  "text/final", // 标记为最终结果
					Timestamp: time.Now(),
				},
			}
			// 发送到输出通道
			e.BaseElement.OutChan <- msg
		}
	})

	// 处理错误事件
	e.recognizer.Canceled(func(evt speech.SpeechRecognitionCanceledEventArgs) {
		log.Printf("Speech recognition canceled: %v", evt.ErrorDetails)
		// 发布错误事件到总线
		e.BaseElement.Bus().Publish(pipeline.Event{
			Type:      pipeline.EventError,
			Timestamp: time.Now(),
			Payload:   fmt.Sprintf("Recognition canceled: %v", evt.ErrorDetails),
		})
	})

	// 启动音频处理协程
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-e.BaseElement.InChan:
				if msg.Type == pipeline.MsgTypeAudio && msg.AudioData != nil {
					// 写入音频数据到推流
					if err := e.pushStream.Write(msg.AudioData.Data); err != nil {
						log.Printf("Failed to write audio data: %v", err)
						e.BaseElement.Bus().Publish(pipeline.Event{
							Type:      pipeline.EventError,
							Timestamp: time.Now(),
							Payload:   fmt.Sprintf("Failed to write audio data: %v", err),
						})
					}
				}
			}
		}
	}()

	// 启动连续识别
	if err := e.recognizer.StartContinuousRecognitionAsync(); err != nil {
		return fmt.Errorf("failed to start continuous recognition: %v", err)
	}

	return nil
}

func (e *AzureSTTElement) Stop() error {
	if e.cancel != nil {
		e.cancel()
		e.wg.Wait()
		e.cancel = nil
	}

	if e.recognizer != nil {
		if err := e.recognizer.StopContinuousRecognitionAsync(); err != nil {
			log.Printf("Failed to stop recognition: %v", err)
		}
		e.recognizer.Close()
		e.recognizer = nil
	}

	if e.pushStream != nil {
		e.pushStream.Close()
		e.pushStream = nil
	}

	if e.audioConfig != nil {
		e.audioConfig.Close()
		e.audioConfig = nil
	}

	return nil
}

// SetVADConfig 设置 VAD 配置
func (e *AzureSTTElement) SetVADConfig(enable bool, silenceTimeout, initialSilenceTimeout int) {
	e.enableVAD = enable
	if enable {
		e.silenceTimeoutMs = fmt.Sprintf("%d", silenceTimeout)
		e.initialSilenceTimeoutMs = fmt.Sprintf("%d", initialSilenceTimeout)
	}
}

func init() {
	// 注册属性
	element := NewAzureSTTElement()
	element.RegisterProperty(pipeline.PropertyDesc{
		Name:     "enable_vad",
		Type:     reflect.TypeOf(true),
		Writable: true,
		Readable: true,
		Default:  true,
	})
	element.RegisterProperty(pipeline.PropertyDesc{
		Name:     "silence_timeout_ms",
		Type:     reflect.TypeOf(""),
		Writable: true,
		Readable: true,
		Default:  "500",
	})
	element.RegisterProperty(pipeline.PropertyDesc{
		Name:     "initial_silence_timeout_ms",
		Type:     reflect.TypeOf(""),
		Writable: true,
		Readable: true,
		Default:  "5000",
	})
}
