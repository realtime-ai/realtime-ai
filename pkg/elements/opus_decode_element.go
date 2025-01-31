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
	"github.com/realtime-ai/realtime-ai/pkg/utils"
)

type OpusDecodeElement struct {
	*pipeline.BaseElement

	decoder    *opus.Decoder
	sampleRate int
	channels   int
	dumper     *audio.Dumper

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewOpusDecodeElement(sampleRate int, channels int) *OpusDecodeElement {
	decoder, err := opus.NewDecoder(sampleRate, channels)
	if err != nil {
		log.Fatalf("failed to create opus decoder: %v", err)
	}

	var dumper *audio.Dumper
	if os.Getenv("DUMP_OPUS_DECODED") == "true" {
		dumper, err = audio.NewDumper("opus_decoded", sampleRate, channels)
		if err != nil {
			log.Printf("create audio dumper error: %v", err)
		}
	}

	return &OpusDecodeElement{
		BaseElement: pipeline.NewBaseElement(100),
		decoder:     decoder,
		sampleRate:  sampleRate,
		channels:    channels,
		dumper:      dumper,
	}
}

func (e *OpusDecodeElement) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()

		pcmBuf := make([]int16, 1920) // stereo * 960
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-e.BaseElement.InChan:
				if msg.Type != pipeline.MsgTypeAudio {
					continue
				}

				if msg.AudioData.MediaType != "audio/x-opus" {
					continue
				}

				if len(msg.AudioData.Data) == 0 {
					continue
				}

				// 解码
				n, err := e.decoder.Decode(msg.AudioData.Data, pcmBuf)
				if err != nil {
					log.Println("Opus decode error:", err)
					continue
				}

				audioData := utils.Int16SliceToByteSlice(pcmBuf[:n])

				// dump 音频数据
				if e.dumper != nil {
					if err := e.dumper.Write(audioData); err != nil {
						log.Printf("Failed to dump audio: %v", err)
					}
				}

				// 创建输出消息
				outMsg := &pipeline.PipelineMessage{
					Type:      pipeline.MsgTypeAudio,
					SessionID: msg.SessionID,
					Timestamp: time.Now(),
					AudioData: &pipeline.AudioData{
						Data:       audioData,
						MediaType:  "audio/x-raw",
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

func (e *OpusDecodeElement) Stop() error {
	if e.cancel != nil {
		e.cancel()
		e.wg.Wait()
		e.cancel = nil
	}

	if e.dumper != nil {
		e.dumper.Close()
		e.dumper = nil
	}

	// 清空解码器引用
	e.decoder = nil
	return nil
}

// func (e *OpusDecodeElement) In() chan<- *pipeline.PipelineMessage {
// 	return e.BaseElement.InChan
// }

// func (e *OpusDecodeElement) Out() <-chan *pipeline.PipelineMessage {
// 	return e.BaseElement.OutChan
// }
