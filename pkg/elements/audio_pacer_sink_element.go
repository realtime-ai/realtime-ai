package elements

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"github.com/hraban/opus"
	"github.com/realtime-ai/realtime-ai/pkg/audio"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

// AudioPacerSinkElement 将音频数据写入音频节奏控制器，实现固定20ms间隔的音频输出
type AudioPacerSinkElement struct {
	*pipeline.BaseElement

	pacer  *audio.AudioPacer
	dumper *audio.Dumper

	encoder *opus.Encoder

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewAudioPacerSinkElement() *AudioPacerSinkElement {
	pacer, err := audio.NewAudioPacer()
	if err != nil {
		log.Fatal("create audio buffer error: ", err)
	}

	var dumper *audio.Dumper
	if os.Getenv("DUMP_LOCAL_AUDIO") == "true" {
		dumper, err = audio.NewDumper("local", 24000, 1)
		if err != nil {
			log.Printf("create audio dumper error: %v", err)
		}
	}

	encoder, err := opus.NewEncoder(48000, 1, opus.AppVoIP)
	if err != nil {
		log.Fatal("create opus encoder error: ", err)
	}

	// // 设置编码参数
	encoder.SetBitrate(50000) // 64 kbps
	encoder.SetComplexity(10) // 最高质量

	return &AudioPacerSinkElement{
		BaseElement: pipeline.NewBaseElement("audio-pacer-sink-element", 100),
		pacer:       pacer,
		dumper:      dumper,
		encoder:     encoder,
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

				if msg.AudioData.MediaType != "audio/x-raw" {
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
							SampleRate: 48000,
							Channels:   1,
							MediaType:  "audio/x-raw",
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
