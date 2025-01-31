package elements

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/hraban/opus"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/utils"
)

type OpusEncodeElement struct {
	*pipeline.BaseElement

	encoder    *opus.Encoder
	sampleRate int
	channels   int

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewOpusEncodeElement(bufferSize int, sampleRate int, channels int) *OpusEncodeElement {
	encoder, err := opus.NewEncoder(sampleRate, channels, opus.AppVoIP)
	if err != nil {
		log.Fatalf("failed to create opus encoder: %v", err)
	}

	// 设置编码参数
	encoder.SetBitrate(64000) // 64 kbps
	encoder.SetComplexity(10) // 最高质量

	return &OpusEncodeElement{
		BaseElement: pipeline.NewBaseElement(bufferSize),
		encoder:     encoder,
		sampleRate:  sampleRate,
		channels:    channels,
	}
}

func (e *OpusEncodeElement) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()

		// 创建编码缓冲区 (最大 Opus 帧大小)
		opusBuf := make([]byte, 1275) // 最大 Opus 帧大小

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

				pcmData := utils.ByteSliceToInt16Slice(msg.AudioData.Data)

				// 编码
				n, err := e.encoder.Encode(pcmData, opusBuf)
				if err != nil {
					log.Println("Opus encode error:", err)
					continue
				}

				log.Printf("Opus encode success, n: %d", n)

				// 创建输出消息
				outMsg := &pipeline.PipelineMessage{
					Type:      pipeline.MsgTypeAudio,
					SessionID: msg.SessionID,
					Timestamp: time.Now(),
					AudioData: &pipeline.AudioData{
						Data:       opusBuf[:n],
						MediaType:  "audio/x-opus",
						SampleRate: e.sampleRate,
						Channels:   e.channels,
						Timestamp:  time.Now(),
					},
				}

				// 输出
				select {
				case e.BaseElement.OutChan <- outMsg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return nil
}

func (e *OpusEncodeElement) Stop() error {
	if e.cancel != nil {
		e.cancel()
		e.wg.Wait()
		e.cancel = nil
	}

	// 清空编码器引用
	e.encoder = nil
	return nil
}

// func (e *OpusEncodeElement) In() chan<- *pipeline.PipelineMessage {
// 	return e.BaseElement.InChan
// }

// func (e *OpusEncodeElement) Out() <-chan *pipeline.PipelineMessage {
// 	return e.BaseElement.OutChan
// }
