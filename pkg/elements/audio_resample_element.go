package elements

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/realtime-ai/realtime-ai/pkg/audio"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

type AudioResampleElement struct {
	*pipeline.BaseElement

	inRate      int
	outRate     int
	inChannels  int
	outChannels int

	resample *audio.Resample

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewAudioResampleElement(inRate, outRate int, inChannels, outChannels int) *AudioResampleElement {
	inLayout := astiav.ChannelLayoutMono
	outLayout := astiav.ChannelLayoutMono
	if inChannels == 1 {
		inLayout = astiav.ChannelLayoutMono
	} else if inChannels == 2 {
		inLayout = astiav.ChannelLayoutStereo
	}

	if outChannels == 1 {
		outLayout = astiav.ChannelLayoutMono
	} else if outChannels == 2 {
		outLayout = astiav.ChannelLayoutStereo
	}

	resample, err := audio.NewResample(inRate, outRate, inLayout, outLayout)
	if err != nil {
		log.Fatalf("failed to create resample: %v", err)
	}

	return &AudioResampleElement{
		BaseElement: pipeline.NewBaseElement(100),
		inRate:      inRate,
		outRate:     outRate,
		inChannels:  inChannels,
		outChannels: outChannels,
		resample:    resample,
	}
}

func (e *AudioResampleElement) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

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

				// 重采样
				outData, err := e.resample.Resample(msg.AudioData.Data)
				if err != nil {
					log.Printf("Resample error: %v", err)
					continue
				}

				// 创建输出消息
				outMsg := &pipeline.PipelineMessage{
					Type:      pipeline.MsgTypeAudio,
					SessionID: msg.SessionID,
					Timestamp: time.Now(),
					AudioData: &pipeline.AudioData{
						Data:       outData,
						SampleRate: e.outRate,
						Channels:   e.outChannels,
						MediaType:  "audio/x-raw",
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

func (e *AudioResampleElement) Stop() error {
	if e.cancel != nil {
		e.cancel()
		e.wg.Wait()
		e.cancel = nil
	}

	if e.resample != nil {
		e.resample.Free()
		e.resample = nil
	}
	return nil
}
