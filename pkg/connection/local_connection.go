package connection

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gen2brain/malgo"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

const (
	CaptureDeviceSampleRate = 16000
	CaptureDeviceChannels   = 1

	PlaybackDeviceSampleRate = 48000
	PlaybackDeviceChannels   = 1
)

var _ RTCConnection = (*localConnectionImpl)(nil)

// LocalConnection 本地连接
type localConnectionImpl struct {
	peerID string

	// malgo context and devices
	context        *malgo.AllocatedContext
	captureDevice  *malgo.Device
	playbackDevice *malgo.Device

	handler ConnectionEventHandler

	inChan  chan *pipeline.PipelineMessage
	outChan chan *pipeline.PipelineMessage

	// Control channels
	stopCapture  chan struct{}
	stopPlayback chan struct{}
}

func NewLocalConnection(peerID string) (RTCConnection, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize context: %v", err)
	}

	conn := &localConnectionImpl{
		peerID:       peerID,
		context:      ctx,
		handler:      &NoOpConnectionEventHandler{},
		inChan:       make(chan *pipeline.PipelineMessage, 50),
		outChan:      make(chan *pipeline.PipelineMessage, 50),
		stopCapture:  make(chan struct{}),
		stopPlayback: make(chan struct{}),
	}

	err = conn.Start(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to start local connection: %v", err)
	}

	return conn, nil
}

func (l *localConnectionImpl) PeerID() string {
	return l.peerID
}

func (l *localConnectionImpl) RegisterEventHandler(handler ConnectionEventHandler) {
	l.handler = handler
}

func (l *localConnectionImpl) In() chan<- *pipeline.PipelineMessage {
	return l.inChan
}

func (l *localConnectionImpl) Out() <-chan *pipeline.PipelineMessage {
	return l.outChan
}

func (l *localConnectionImpl) Start(ctx context.Context) error {
	if err := l.startAudioCapture(); err != nil {
		return fmt.Errorf("failed to start audio capture: %v", err)
	}

	if err := l.startAudioPlayback(); err != nil {
		return fmt.Errorf("failed to start audio playback: %v", err)
	}

	return nil
}

func (l *localConnectionImpl) startAudioCapture() error {
	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.PeriodSizeInMilliseconds = 20 // 20ms
	deviceConfig.Capture.Format = malgo.FormatS16
	deviceConfig.Capture.Channels = CaptureDeviceChannels
	deviceConfig.SampleRate = CaptureDeviceSampleRate
	deviceConfig.Alsa.NoMMap = 1

	var err error
	l.captureDevice, err = malgo.InitDevice(l.context.Context, deviceConfig, malgo.DeviceCallbacks{
		Data: func(outputSamples, inputSamples []byte, framecount uint32) {

			// copy inputSamples 到 tempSamples
			tempSamples := make([]byte, len(inputSamples))
			copy(tempSamples, inputSamples)

			// Create audio message
			msg := &pipeline.PipelineMessage{
				Type: pipeline.MsgTypeAudio,
				AudioData: &pipeline.AudioData{
					Data:       tempSamples,
					SampleRate: CaptureDeviceSampleRate,
					Channels:   CaptureDeviceChannels,
					MediaType:  "audio/x-raw",
					Timestamp:  time.Now(),
				},
			}
			// Send to handler
			l.handler.OnMessage(msg)
		},
	})

	if err != nil {
		return fmt.Errorf("failed to initialize capture device: %v", err)
	}

	if err := l.captureDevice.Start(); err != nil {
		return fmt.Errorf("failed to start capture device: %v", err)
	}

	return nil
}

func (l *localConnectionImpl) startAudioPlayback() error {
	deviceConfig := malgo.DefaultDeviceConfig(malgo.Playback)
	deviceConfig.PeriodSizeInMilliseconds = 20 // 20ms
	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = PlaybackDeviceChannels
	deviceConfig.SampleRate = PlaybackDeviceSampleRate
	deviceConfig.Alsa.NoMMap = 1

	// 计算100ms对应的字节数
	bytesPerSample := 2                                      // 16-bit = 2 bytes
	samplesFor100ms := PlaybackDeviceSampleRate * 100 / 1000 // 采样率 * 0.1秒
	bufferSize := samplesFor100ms * bytesPerSample * PlaybackDeviceChannels

	var sampleBuffer []byte
	var audioBuffer []byte // 用于缓存音频数据
	var isBuffering = true // 是否正在缓冲

	var err error
	l.playbackDevice, err = malgo.InitDevice(l.context.Context, deviceConfig, malgo.DeviceCallbacks{
		Data: func(outputSamples, inputSamples []byte, framecount uint32) {
			if isBuffering {
				// 如果还在缓冲，输出静音
				for i := range outputSamples {
					outputSamples[i] = 0
				}
				return
			}

			if len(sampleBuffer) > 0 {
				copyLen := len(outputSamples)
				if len(sampleBuffer) < copyLen {
					copyLen = len(sampleBuffer)
				}
				copy(outputSamples[:copyLen], sampleBuffer[:copyLen])
				// Keep remaining data in buffer
				sampleBuffer = sampleBuffer[copyLen:]

				// Fill remaining output with silence if needed
				for i := copyLen; i < len(outputSamples); i++ {
					outputSamples[i] = 0

					log.Printf("outputSamples: %d, framecount: %d  time: %d\n", len(outputSamples), framecount, int64(time.Now().UnixNano()/1000))
				}
			} else {
				// Fill with silence if no data
				for i := range outputSamples {
					outputSamples[i] = 0
				}
			}
		},
	})

	if err != nil {
		return fmt.Errorf("failed to initialize playback device: %v", err)
	}

	if err := l.playbackDevice.Start(); err != nil {
		return fmt.Errorf("failed to start playback device: %v", err)
	}

	// Start goroutine to handle audio playback
	go func() {
		for {
			select {
			case msg := <-l.inChan:
				if msg.Type == pipeline.MsgTypeAudio && msg.AudioData != nil {
					if msg.AudioData.SampleRate != PlaybackDeviceSampleRate {
						continue
					}

					if isBuffering {
						// 累积数据到audioBuffer
						audioBuffer = append(audioBuffer, msg.AudioData.Data...)

						// 检查是否已经累积了足够的数据
						if len(audioBuffer) >= bufferSize {
							log.Printf("Audio buffer full (%d bytes), starting playback", len(audioBuffer))
							sampleBuffer = audioBuffer
							audioBuffer = nil
							isBuffering = false
						}
					} else {
						// 正常播放模式，直接追加到播放缓冲区
						sampleBuffer = append(sampleBuffer, msg.AudioData.Data...)
					}
				}
			case <-l.stopPlayback:
				return
			}
		}
	}()

	return nil
}

func (l *localConnectionImpl) SendMessage(msg *pipeline.PipelineMessage) {

	// 判断是否是音频消息，如果是，则直接发送给播放 inChan
	if msg.Type == pipeline.MsgTypeAudio {
		l.inChan <- msg
	}
}

func (l *localConnectionImpl) Close() error {
	// Stop audio devices
	if l.captureDevice != nil {
		l.captureDevice.Stop()
		l.captureDevice.Uninit()
	}

	if l.playbackDevice != nil {
		close(l.stopPlayback)
		l.playbackDevice.Stop()
		l.playbackDevice.Uninit()
	}

	// Uninit context
	if l.context != nil {
		l.context.Uninit()
	}

	close(l.inChan)
	close(l.outChan)

	return nil
}

func (l *localConnectionImpl) SetAudioEncodeParam(sampleRate int, channels int, bitRate int) {}
func (l *localConnectionImpl) SetAudioOutputParam(sampleRate int, channels int)              {}
