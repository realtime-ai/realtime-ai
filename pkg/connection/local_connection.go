package connection

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gen2brain/malgo"
	"github.com/realtime-ai/realtime-ai/pkg/audio"
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
	audioContext   *malgo.AllocatedContext
	captureDevice  *malgo.Device
	playbackDevice *malgo.Device

	handler ConnectionEventHandler

	inChan  chan *pipeline.PipelineMessage
	outChan chan *pipeline.PipelineMessage

	// Control channels
	stopCapture  chan struct{}
	stopPlayback chan struct{}

	playAudioBuffer []byte

	// Audio dumper
	playbackDumper *audio.Dumper
}

func NewLocalConnection(peerID string) (RTCConnection, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize context: %v", err)
	}

	conn := &localConnectionImpl{
		peerID:       peerID,
		audioContext: ctx,
		handler:      &NoOpConnectionEventHandler{},
		inChan:       make(chan *pipeline.PipelineMessage, 50),
		outChan:      make(chan *pipeline.PipelineMessage, 50),
		stopCapture:  make(chan struct{}),
		stopPlayback: make(chan struct{}),
	}

	// 创建播放数据的dumper
	if os.Getenv("DUMP_LOCAL_PLAYBACK") == "true" {
		var err error
		conn.playbackDumper, err = audio.NewDumper("local_playback", PlaybackDeviceSampleRate, PlaybackDeviceChannels)
		if err != nil {
			log.Printf("create playback dumper error: %v", err)
		}
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
	l.captureDevice, err = malgo.InitDevice(l.audioContext.Context, deviceConfig, malgo.DeviceCallbacks{
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

	l.playAudioBuffer = make([]byte, 0)
	var audioBuffer []byte     // 用于缓存音频数据
	var isBuffering = true     // 是否正在缓冲
	var bufferMutex sync.Mutex // 保护 buffer 的互斥锁

	var err error
	l.playbackDevice, err = malgo.InitDevice(l.audioContext.Context, deviceConfig, malgo.DeviceCallbacks{
		Data: func(outputSamples, inputSamples []byte, framecount uint32) {
			// 检查缓冲状态要在锁外面
			if isBuffering {
				// 如果还在缓冲，输出静音
				for i := range outputSamples {
					outputSamples[i] = 0
				}
				return
			}

			bufferMutex.Lock()
			if len(l.playAudioBuffer) >= len(outputSamples) {
				// 复制数据到输出缓冲区
				copy(outputSamples, l.playAudioBuffer[:len(outputSamples)])
				// 移除已经播放的数据
				l.playAudioBuffer = l.playAudioBuffer[len(outputSamples):]

				//log.Printf("1111111sampleBuffer remaining: %v, outputSamples: %v", len(l.playAudioBuffer), len(outputSamples))
			} else if len(l.playAudioBuffer) > 0 {
				// 如果还有剩余数据但不足一帧,复制剩余数据
				copy(outputSamples, l.playAudioBuffer)
				// 将未使用的输出缓冲区填充0
				for i := len(l.playAudioBuffer); i < len(outputSamples); i++ {
					outputSamples[i] = 0
				}

				//log.Printf("2222222sampleBuffer remaining: %v, outputSamples: %v", len(l.playAudioBuffer), len(outputSamples))
			} else {
				// 如果没有数据,输出静音
				for i := range outputSamples {
					outputSamples[i] = 0
				}

				//log.Printf("3333333sampleBuffer remaining: %v, outputSamples: %v", len(l.playAudioBuffer), len(outputSamples))
			}
			bufferMutex.Unlock()

			if l.playbackDumper != nil {
				if err := l.playbackDumper.Write(outputSamples); err != nil {
					// 避免在回调中打印日志，可以考虑使用channel发送错误
					log.Printf("dump playback data error: %v", err)
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
							bufferMutex.Lock()
							l.playAudioBuffer = append(l.playAudioBuffer, audioBuffer...)
							bufferMutex.Unlock()
							audioBuffer = nil
							isBuffering = false
						}
					} else {
						// 正常播放模式，直接追加到播放缓冲区
						bufferMutex.Lock()
						l.playAudioBuffer = append(l.playAudioBuffer, msg.AudioData.Data...)
						bufferMutex.Unlock()
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

	// Close dumper
	if l.playbackDumper != nil {
		l.playbackDumper.Close()
		l.playbackDumper = nil
	}

	// Uninit context
	if l.audioContext != nil {
		l.audioContext.Uninit()
	}

	close(l.inChan)
	close(l.outChan)

	return nil
}

func (l *localConnectionImpl) SetAudioEncodeParam(sampleRate int, channels int, bitRate int) {}
func (l *localConnectionImpl) SetAudioOutputParam(sampleRate int, channels int)              {}
