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
	FadeOutMs  int // 打断时淡出时长（毫秒），0 表示不淡出
}

// DefaultAudioPacerSinkConfig 返回默认配置
func DefaultAudioPacerSinkConfig() AudioPacerSinkConfig {
	return AudioPacerSinkConfig{
		SampleRate: audio.DefaultSampleRate,
		Channels:   audio.Channels,
		FadeOutMs:  50, // 默认 50ms 淡出，避免爆音
	}
}

// AudioPacerSinkElement 将音频数据写入音频节奏控制器，实现固定20ms间隔的音频输出
// 只做缓冲和帧切分，不做编码
//
// 主要功能:
//   - 音频缓冲和 20ms 帧输出
//   - 打断时快速清空缓冲 (支持淡出)
//   - 暂停/恢复支持 (用于混合模式打断)
type AudioPacerSinkElement struct {
	*pipeline.BaseElement

	pacer  *audio.AudioPacer
	dumper *audio.Dumper

	sampleRate int
	channels   int

	// 打断配置
	fadeOutMs int // 淡出时长（毫秒），0 表示不淡出

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
		fadeOutMs:   cfg.FadeOutMs,
	}
}

func (e *AudioPacerSinkElement) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	e.wg.Add(3) // 3 个协程: run 中的 2 个 + listenEvent
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

// listenEvent 监听打断、暂停、恢复事件
func (e *AudioPacerSinkElement) listenEvent(ctx context.Context) {
	defer e.wg.Done()

	interruptCh := make(chan pipeline.Event, 5)
	pauseCh := make(chan pipeline.Event, 5)
	resumeCh := make(chan pipeline.Event, 5)

	// 订阅事件
	e.Bus().Subscribe(pipeline.EventInterrupted, interruptCh)
	e.Bus().Subscribe(pipeline.EventAudioPause, pauseCh)
	e.Bus().Subscribe(pipeline.EventAudioResume, resumeCh)

	// 退出时取消订阅
	defer func() {
		e.Bus().Unsubscribe(pipeline.EventInterrupted, interruptCh)
		e.Bus().Unsubscribe(pipeline.EventAudioPause, pauseCh)
		e.Bus().Unsubscribe(pipeline.EventAudioResume, resumeCh)
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case event := <-interruptCh:
			e.handleInterrupt(event)

		case event := <-pauseCh:
			e.handlePause(event)

		case event := <-resumeCh:
			e.handleResume(event)
		}
	}
}

// handleInterrupt 处理打断事件
func (e *AudioPacerSinkElement) handleInterrupt(event pipeline.Event) {
	log.Printf("[AudioPacerSink] Received interrupt event, clearing buffer with %dms fade-out", e.fadeOutMs)

	// 清空音频缓冲区（带淡出效果）
	if e.fadeOutMs > 0 {
		e.pacer.ClearWithFadeOut(e.fadeOutMs)
	} else {
		e.pacer.Clear()
	}

	// 发送确认事件
	e.Bus().Publish(pipeline.Event{
		Type:      pipeline.EventInterruptAcknowledged,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"source":     e.GetName(),
			"fadeOutMs":  e.fadeOutMs,
			"clearedAt":  time.Now().UnixMilli(),
		},
	})

	log.Printf("[AudioPacerSink] Interrupt handled, buffer cleared")
}

// handlePause 处理暂停事件（混合模式打断用）
func (e *AudioPacerSinkElement) handlePause(event pipeline.Event) {
	log.Printf("[AudioPacerSink] Received pause event")
	e.pacer.Pause()
}

// handleResume 处理恢复事件（混合模式打断用）
func (e *AudioPacerSinkElement) handleResume(event pipeline.Event) {
	log.Printf("[AudioPacerSink] Received resume event")
	e.pacer.Resume()
}
