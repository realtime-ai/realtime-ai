package connection

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/hraban/opus"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/utils"
)

const (
	DefaultInSampleRate = 48000
	DefaultInChannels   = 1
	DefaultInBitRate    = 50000

	DefaultOutSampleRate = 48000
	DefaultOutChannels   = 1
)

type rtcConnectionImpl struct {
	peerID string

	// 底层 Pion WebRTC 对象
	pc *webrtc.PeerConnection

	// DataChannel 管理
	dataChannel *webrtc.DataChannel

	// 远端音频流
	remoteAudioTrack *webrtc.TrackRemote

	// 本地音频流
	localAudioTrack *webrtc.TrackLocalStaticSample

	handler ConnectionEventHandler

	inChan  chan *pipeline.PipelineMessage
	outChan chan *pipeline.PipelineMessage

	audioEncoder *opus.Encoder
	audioDecoder *opus.Decoder

	// 音频参数
	inSampleRate  int
	inChannels    int
	inBitRate     int
	outSampleRate int
	outChannels   int

	once sync.Once
}

var _ RTCConnection = (*rtcConnectionImpl)(nil)

func NewRTCConnection(peerID string, pc *webrtc.PeerConnection) RTCConnection {

	audioEncoder, err := opus.NewEncoder(DefaultInSampleRate, DefaultInChannels, opus.AppVoIP)
	if err != nil {
		log.Fatalf("failed to create encoder: %v", err)
	}
	audioEncoder.SetBitrate(DefaultInBitRate)
	audioEncoder.SetComplexity(10)
	audioEncoder.SetDTX(true)

	audioDecoder, err := opus.NewDecoder(48000, DefaultOutChannels)
	if err != nil {
		log.Fatalf("failed to create decoder: %v", err)
	}

	conn := &rtcConnectionImpl{
		peerID:  peerID,
		pc:      pc,
		handler: &NoOpConnectionEventHandler{},
		inChan:  make(chan *pipeline.PipelineMessage, 50),
		outChan: make(chan *pipeline.PipelineMessage, 50),

		// 编码
		inSampleRate: DefaultInSampleRate,
		inChannels:   DefaultInChannels,
		inBitRate:    DefaultInBitRate,

		// 回调出来
		outSampleRate: DefaultOutSampleRate,
		outChannels:   DefaultOutChannels,

		// // 音频重采样
		// inResample:  inResample,
		// outResample: outResample,

		// 音频编码
		audioEncoder: audioEncoder,
		audioDecoder: audioDecoder,
	}

	conn.Start(context.Background())

	return conn
}

func (c *rtcConnectionImpl) RegisterEventHandler(handler ConnectionEventHandler) {
	c.handler = handler
}

func (c *rtcConnectionImpl) PeerID() string {
	return c.peerID
}

func (c *rtcConnectionImpl) In() chan<- *pipeline.PipelineMessage {
	return c.inChan
}

func (c *rtcConnectionImpl) Out() <-chan *pipeline.PipelineMessage {
	return c.outChan
}

func (c *rtcConnectionImpl) SetAudioEncodeParam(sampleRate int, channels int, bitRate int) {

	// // 增加判断，如果参数和之前一样，则不重新创建
	// if c.inSampleRate == sampleRate && c.inChannels == channels && c.inBitRate == bitRate {
	// 	return
	// }

	// c.inSampleRate = sampleRate
	// c.inChannels = channels
	// c.inBitRate = bitRate

	// inLayout := astiav.ChannelLayoutMono
	// if channels == 1 {
	// 	inLayout = astiav.ChannelLayoutMono
	// } else if channels == 2 {
	// 	inLayout = astiav.ChannelLayoutStereo
	// }

	// // todo 这里需要根据 sampleRate 和 channels 来创建
	// inResample, err := audio.NewResample(48000, sampleRate, astiav.ChannelLayoutMono, inLayout)
	// if err != nil {
	// 	log.Fatalf("failed to create resample: %v", err)
	// }

	// c.inResample = inResample

	// encoder, err := opus.NewEncoder(sampleRate, channels, opus.AppVoIP)
	// if err != nil {
	// 	log.Fatalf("failed to create encoder: %v", err)
	// }
	// encoder.SetBitrate(bitRate)
	// encoder.SetComplexity(10)
	// encoder.SetDTX(true)

	// c.audioEncoder = encoder
}

func (c *rtcConnectionImpl) SetAudioOutputParam(sampleRate int, channels int) {

	// 增加判断，如果参数和之前一样，则不重新创建
	// if c.outSampleRate == sampleRate && c.outChannels == channels {
	// 	return
	// }

	// c.outSampleRate = sampleRate
	// c.outChannels = channels

	// outLayout := astiav.ChannelLayoutMono
	// if channels == 1 {
	// 	outLayout = astiav.ChannelLayoutMono
	// } else if channels == 2 {
	// 	outLayout = astiav.ChannelLayoutStereo
	// }

	// outResample, err := audio.NewResample(48000, sampleRate, astiav.ChannelLayoutMono, outLayout)
	// if err != nil {
	// 	log.Fatalf("failed to create resample: %v", err)
	// }

	// c.outResample = outResample

	// decoder, err := opus.NewDecoder(sampleRate, channels)
	// if err != nil {
	// 	log.Fatalf("failed to create decoder: %v", err)
	// }

	// c.audioDecoder = decoder
}

func (c *rtcConnectionImpl) Start(ctx context.Context) error {

	c.pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		c.handler.OnConnectionStateChange(state)
	})

	// 如果你想在 wrapper 里设置 pc.OnDataChannel，也行；也可以在这里：
	c.pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		// 这里可能需要把 channel 存到 wrapper 里
		//wrapper.AddDataChannel(dc)

		c.dataChannel = dc

		c.readDataChannel()
	})

	transceiver, err := c.pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendrecv,
	})

	c.localAudioTrack = transceiver.Sender().Track().(*webrtc.TrackLocalStaticSample)

	if err != nil {
		log.Println("add transceiver error:", err)
		return err
	}

	c.pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("OnTrack: %v, codec: %v", track.ID(), track.Codec().MimeType)
		if track.Kind() == webrtc.RTPCodecTypeAudio {
			c.remoteAudioTrack = track
			go c.readRemoteAudio(ctx)
		}
	})

	return nil
}

// 发送消息,
func (c *rtcConnectionImpl) SendMessage(msg *pipeline.PipelineMessage) {

	if msg.Type == pipeline.MsgTypeData {

		if c.dataChannel != nil && c.dataChannel.ReadyState() == webrtc.DataChannelStateOpen {
			c.dataChannel.Send(msg.TextData.Data)
		} else {
			log.Println("data channel does not open")
		}

	} else if msg.Type == pipeline.MsgTypeAudio {

		if msg.AudioData.MediaType == "audio/x-raw" {
			audioData := msg.AudioData.Data

			// if msg.AudioData.SampleRate != c.inSampleRate || msg.AudioData.Channels != c.inChannels {
			// 	var err error
			// 	audioData, err = c.inResample.Resample(audioData)
			// 	if err != nil {
			// 		log.Println("resample error:", err)
			// 		return
			// 	}
			// }

			opusBuf := make([]byte, 1275)

			pcm := utils.ByteSliceToInt16Slice(audioData)
			n, err := c.audioEncoder.Encode(pcm, opusBuf)
			if err != nil {
				log.Println("encode error:", err)
				return
			}

			sample := media.Sample{
				Data:     opusBuf[:n],
				Duration: 20 * time.Millisecond,
			}
			// 写入音频轨道
			if err := c.localAudioTrack.WriteSample(sample); err != nil {
				log.Printf("Failed to write audio sample: %v", err)
				return
			}
		}
	}
}

func (c *rtcConnectionImpl) Close() error {

	c.once.Do(func() {
		c.pc.Close()
	})

	return nil
}

func (c *rtcConnectionImpl) readRemoteAudio(ctx context.Context) {

	pcmBuf := make([]int16, 1920) // stereo * 960

	for {
		select {
		case <-ctx.Done():
			return
		default:
			rtpPacket, _, err := c.remoteAudioTrack.ReadRTP()
			if err != nil {
				log.Println("read RTP error:", err)
				continue
			}

			// 解码
			n, err := c.audioDecoder.Decode(rtpPacket.Payload, pcmBuf)
			if err != nil {
				log.Println("Opus decode error:", err)
				continue
			}

			audioData := utils.Int16SliceToByteSlice(pcmBuf[:n])

			// 重采样
			// if c.outSampleRate != 48000 || c.outChannels != 1 {

			// 	audioData, err = c.outResample.Resample(audioData)
			// 	if err != nil {
			// 		log.Println("resample error:", err)
			// 		continue
			// 	}
			// }
			// 将拿到的 payload 投递给 pipeline 的“输入 element”
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

			c.handler.OnMessage(msg)
		}
	}
}

func (c *rtcConnectionImpl) readDataChannel() {

	c.dataChannel.OnMessage(func(data webrtc.DataChannelMessage) {

		message := data.Data

		msg := &pipeline.PipelineMessage{
			Type: pipeline.MsgTypeData,
			TextData: &pipeline.TextData{
				Data:      message,
				TextType:  "text",
				Timestamp: time.Now(),
			},
		}

		c.handler.OnMessage(msg)
	})
}
