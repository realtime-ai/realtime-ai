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

// WebRTCSinkElement 将音频数据写入 WebRTC 轨道, todo 支持视频/文本
type WebRTCSinkElement struct {
	*pipeline.BaseElement

	playout *audio.PlayoutBuffer
	dumper  *audio.Dumper

	encoder *opus.Encoder

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewWebRTCSinkElement() *WebRTCSinkElement {
	playout, err := audio.NewPlayoutBuffer()
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

	return &WebRTCSinkElement{
		BaseElement: pipeline.NewBaseElement(100),
		playout:     playout,
		dumper:      dumper,
		encoder:     encoder,
	}
}

func (e *WebRTCSinkElement) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	e.wg.Add(2) // 两个协程
	go e.run(ctx)

	return nil
}

func (e *WebRTCSinkElement) Stop() error {
	if e.cancel != nil {
		e.cancel()
		e.wg.Wait()
		e.cancel = nil
	}

	if e.playout != nil {
		e.playout.Close()
		e.playout = nil
	}

	if e.dumper != nil {
		e.dumper.Close()
		e.dumper = nil
	}

	return nil
}

func (e *WebRTCSinkElement) In() chan<- *pipeline.PipelineMessage {
	return e.BaseElement.InChan
}

func (e *WebRTCSinkElement) Out() <-chan *pipeline.PipelineMessage {
	return e.BaseElement.OutChan
}

func (e *WebRTCSinkElement) run(ctx context.Context) {
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

				// 写入播放缓冲区
				if err := e.playout.Write(msg.AudioData.Data); err != nil {
					log.Printf("Failed to write to playout buffer: %v", err)
				}
			}
		}
	}()

	// 启动发送输出的协程
	go func() {
		defer e.wg.Done()

		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		lastSendTime := time.Now()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// 从播放缓冲区读取一帧数据
				if time.Since(lastSendTime) >= 20*time.Millisecond {

					audioData := e.playout.ReadFrame()

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

					e.BaseElement.OutChan <- msg

					lastSendTime = time.Now()
				}
			}
		}
	}()
}
