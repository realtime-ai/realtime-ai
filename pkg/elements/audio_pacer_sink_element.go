package elements

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/audio"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

// AudioPacerSinkConfig 配置
type AudioPacerSinkConfig struct {
	SampleRate int // 采样率
	Channels   int // 通道数
}

// DefaultAudioPacerSinkConfig 返回默认配置
func DefaultAudioPacerSinkConfig() AudioPacerSinkConfig {
	return AudioPacerSinkConfig{
		SampleRate: audio.DefaultSampleRate,
		Channels:   audio.Channels,
	}
}

// AudioPacerSinkElement 将音频数据写入音频节奏控制器，实现固定20ms间隔的音频输出
// 只做缓冲和帧切分，不做编码
type AudioPacerSinkElement struct {
	*pipeline.BaseElement

	pacer  *audio.AudioPacer
	dumper *audio.Dumper

	sampleRate int
	channels   int

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewAudioPacerSinkElement 创建新的 AudioPacerSinkElement (使用默认配置)
func NewAudioPacerSinkElement() *AudioPacerSinkElement {
	return NewAudioPacerSinkElementWithConfig(DefaultAudioPacerSinkConfig())
}

// NewAudioPacerSinkElementWithConfig 创建新的 AudioPacerSinkElement (使用自定义配置)
func NewAudioPacerSinkElementWithConfig(cfg AudioPacerSinkConfig) *AudioPacerSinkElement {
	if cfg.SampleRate <= 0 {
		cfg.SampleRate = audio.DefaultSampleRate
	}
	if cfg.Channels <= 0 {
		cfg.Channels = audio.Channels
	}

	pacer, err := audio.NewAudioPacerWithConfig(audio.AudioPacerConfig{
		SampleRate: cfg.SampleRate,
		Channels:   cfg.Channels,
	})
	if err != nil {
		log.Fatal("create audio buffer error: ", err)
	}

	var dumper *audio.Dumper
	if os.Getenv("DUMP_LOCAL_AUDIO") == "true" {
		dumper, err = audio.NewDumper("local", cfg.SampleRate, cfg.Channels)
		if err != nil {
			log.Printf("create audio dumper error: %v", err)
		}
	}

	return &AudioPacerSinkElement{
		BaseElement: pipeline.NewBaseElement("audio-pacer-sink-element", 100),
		pacer:       pacer,
		dumper:      dumper,
		sampleRate:  cfg.SampleRate,
		channels:    cfg.Channels,
	}
}

func (e *AudioPacerSinkElement) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	e.wg.Add(2) // 两个协程
	go e.run(ctx)

	go e.listenEvent(ctx)

	return nil
}

func (e *AudioPacerSinkElement) Stop() error {
	if e.cancel != nil {
		e.cancel()
		e.wg.Wait()
		e.cancel = nil
	}

	if e.pacer != nil {
		e.pacer.Close()
		e.pacer = nil
	}

	if e.dumper != nil {
		e.dumper.Close()
		e.dumper = nil
	}

	return nil
}

func (e *AudioPacerSinkElement) run(ctx context.Context) {
	// 启动读取输入的协程
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

				if msg.AudioData.MediaType != pipeline.AudioMediaTypeRaw {
					continue
				}

				if len(msg.AudioData.Data) == 0 {
					continue
				}

				// dump 音频数据
				if e.dumper != nil {
					if err := e.dumper.Write(msg.AudioData.Data); err != nil {
						log.Printf("Failed to dump audio: %v", err)
					}
				}

				// 写入音频节奏控制器
				if err := e.pacer.Write(msg.AudioData.Data); err != nil {
					log.Printf("Failed to write to audio pacer: %v", err)
				}
			}
		}
	}()

	// 启动发送输出的协程
	go func() {
		defer e.wg.Done()

		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()

		lastSendTime := time.Now()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// 从音频节奏控制器读取一帧数据
				if time.Since(lastSendTime) >= 20*time.Millisecond {

					lastSendTime = lastSendTime.Add(20 * time.Millisecond)

					audioData := e.pacer.ReadFrame()

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

					select {
					case e.BaseElement.OutChan <- msg:
					default:
						log.Println("audio pacer sink element out chan is full")
					}

				}
			}
		}
	}()
}

// 监听打断事件
func (e *AudioPacerSinkElement) listenEvent(ctx context.Context) {

	ch := make(chan pipeline.Event, 5)

	e.Bus().Subscribe(pipeline.EventInterrupted, ch)

	defer e.Bus().Unsubscribe(pipeline.EventInterrupted, ch)

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-ch:
			log.Println("AudioPacerSinkElement listenEvent", event)
		}
	}
}
