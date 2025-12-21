package elements

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

type AzureTTSElement struct {
	*pipeline.BaseElement

	subscriptionKey string
	region          string
	language        string
	voice           string
	outputFormat    string

	// 音频格式配置
	sampleRate int
	channels   int

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewAzureTTSElement() *AzureTTSElement {
	return &AzureTTSElement{
		BaseElement:     pipeline.NewBaseElement("azure-tts-element", 100),
		subscriptionKey: os.Getenv("AZURE_SPEECH_KEY"),
		region:          os.Getenv("AZURE_SPEECH_REGION"),
		language:        "zh-CN",
		voice:           "zh-CN-XiaoxiaoNeural", // 默认使用晓晓的声音
		outputFormat:    "raw-24khz-16bit-mono-pcm",
		sampleRate:      24000, // 24kHz
		channels:        1,     // 单声道
	}
}

func (e *AzureTTSElement) Start(ctx context.Context) error {
	if e.subscriptionKey == "" || e.region == "" {
		return fmt.Errorf("Azure Speech credentials not set")
	}

	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	// 启动处理协程
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-e.BaseElement.InChan:
				if msg.Type == pipeline.MsgTypeData && msg.TextData != nil {
					if err := e.Synthesize(ctx, string(msg.TextData.Data)); err != nil {
						log.Printf("Failed to synthesize speech: %v", err)
						e.BaseElement.Bus().Publish(pipeline.Event{
							Type:      pipeline.EventError,
							Timestamp: time.Now(),
							Payload:   fmt.Sprintf("Failed to synthesize speech: %v", err),
						})
					}
				}
			}
		}
	}()

	return nil
}

func (e *AzureTTSElement) Stop() error {
	if e.cancel != nil {
		e.cancel()
		e.wg.Wait()
		e.cancel = nil
	}
	return nil
}

// Synthesize 合成语音
func (e *AzureTTSElement) Synthesize(ctx context.Context, text string) error {
	// 构建 SSML
	ssml := fmt.Sprintf(`<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="%s">
		<voice name="%s">%s</voice>
	</speak>`, e.language, e.voice, text)

	// 创建请求
	endpoint := fmt.Sprintf("https://%s.tts.speech.microsoft.com/cognitiveservices/v1", e.region)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(ssml))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/ssml+xml")
	req.Header.Set("X-Microsoft-OutputFormat", e.outputFormat)
	req.Header.Set("Ocp-Apim-Subscription-Key", e.subscriptionKey)
	req.Header.Set("User-Agent", "realtime-ai")

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// 读取音频数据
	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}

	// 创建音频消息
	msg := &pipeline.PipelineMessage{
		Type: pipeline.MsgTypeAudio,
		AudioData: &pipeline.AudioData{
			Data:       audioData,
			SampleRate: e.sampleRate,
			Channels:   e.channels,
			MediaType:  pipeline.AudioMediaTypeRaw,
			Timestamp:  time.Now(),
		},
	}

	// 发送到输出通道
	e.BaseElement.OutChan <- msg

	return nil
}

// SetVoice 设置语音合成的声音
func (e *AzureTTSElement) SetVoice(voice string) {
	e.voice = voice
}

// SetLanguage 设置语音合成的语言
func (e *AzureTTSElement) SetLanguage(language string) {
	e.language = language
}

func init() {
	// 注册属性
	element := NewAzureTTSElement()
	element.RegisterProperty(pipeline.PropertyDesc{
		Name:     "voice",
		Type:     reflect.TypeOf(""),
		Writable: true,
		Readable: true,
		Default:  "zh-CN-XiaoxiaoNeural",
	})
	element.RegisterProperty(pipeline.PropertyDesc{
		Name:     "language",
		Type:     reflect.TypeOf(""),
		Writable: true,
		Readable: true,
		Default:  "zh-CN",
	})
}
