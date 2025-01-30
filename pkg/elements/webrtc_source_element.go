package elements

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

const (
	sampleRate = 48000
	channels   = 1
)

type WebRTCSourceElement struct {
	*pipeline.BaseElement

	track *webrtc.TrackRemote

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewWebRTCSourceElement(bufferSize int, track *webrtc.TrackRemote) *WebRTCSourceElement {
	return &WebRTCSourceElement{
		BaseElement: pipeline.NewBaseElement(bufferSize),
		track:       track,
	}
}

func (e *WebRTCSourceElement) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				// 从 WebRTC track 读取 RTP 包
				rtp, _, err := e.track.ReadRTP()
				if err != nil {
					log.Printf("Failed to read RTP packet: %v", err)
					continue
				}

				// 创建输出消息
				outMsg := pipeline.PipelineMessage{
					Type:      pipeline.MsgTypeAudio,
					Timestamp: time.Now(),
					AudioData: &pipeline.AudioData{
						Data:       rtp.Payload,
						MediaType:  "audio/x-raw",
						SampleRate: 48000, // WebRTC 默认采样率
						Channels:   1,     // WebRTC 默认单声道
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

func (e *WebRTCSourceElement) Stop() error {
	if e.cancel != nil {
		e.cancel()
		e.wg.Wait()
		e.cancel = nil
	}

	// 清空 track 引用
	e.track = nil
	return nil
}

func (e *WebRTCSourceElement) In() chan<- pipeline.PipelineMessage {
	return e.BaseElement.InChan
}

func (e *WebRTCSourceElement) Out() <-chan pipeline.PipelineMessage {
	return e.BaseElement.OutChan
}
