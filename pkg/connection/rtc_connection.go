package connection

import (
	"context"
	"log"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/hraban/opus"
	"github.com/pion/webrtc/v4"
	"github.com/realtime-ai/realtime-ai/pkg/audio"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

type RTCConnection interface {
	// PeerID 返回此连接对应的唯一标识
	PeerID() string
	// PeerConnection 返回底层的 *webrtc.PeerConnection
	PeerConnection() *webrtc.PeerConnection

	// AddDataChannel 记录/管理新的 DataChannel（本地或远端创建）
	DataChannel() *webrtc.DataChannel

	// LocalAudioTrack 返回本地音频流
	LocalAudioTrack() *webrtc.TrackLocalStaticSample

	// RegisterEventHandler 注册事件处理器
	RegisterEventHandler(handler ConnectionEventHandler)

	// Close 关闭底层的 PeerConnection (并执行相应清理)
	Close() error

	// SetAudioEncodeParam 设置音频编码参数
	SetAudioEncodeParam(sampleRate int, channels int, bitRate int)

	// SetAudioOutputParam 设置音频输出参数
	SetAudioOutputParam(sampleRate int, channels int)

	// new API
	// In 返回此连接的输入 channel
	In() chan<- *pipeline.PipelineMessage

	// Out 返回此连接的输出 channel
	Out() <-chan *pipeline.PipelineMessage
}

// todo add

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

	inResample  *audio.Resample
	outResample *audio.Resample

	audioEncoder *opus.Encoder
	audioDecoder *opus.Decoder

	inSampleRate  int
	inChannels    int
	inBitRate     int
	outSampleRate int
	outChannels   int
}

var _ RTCConnection = (*rtcConnectionImpl)(nil)

func NewRTCConnection(peerID string, pc *webrtc.PeerConnection) RTCConnection {

	conn := &rtcConnectionImpl{
		peerID:  peerID,
		pc:      pc,
		handler: &NoOpConnectionEventHandler{},
		inChan:  make(chan *pipeline.PipelineMessage, 50),
		outChan: make(chan *pipeline.PipelineMessage, 50),
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

func (c *rtcConnectionImpl) PeerConnection() *webrtc.PeerConnection {
	return c.pc
}

func (c *rtcConnectionImpl) DataChannel() *webrtc.DataChannel {
	return c.dataChannel
}

func (c *rtcConnectionImpl) RemoteAudioTrack() *webrtc.TrackRemote {
	return c.remoteAudioTrack
}

func (c *rtcConnectionImpl) LocalAudioTrack() *webrtc.TrackLocalStaticSample {
	return c.localAudioTrack
}

func (c *rtcConnectionImpl) In() chan<- *pipeline.PipelineMessage {
	return c.inChan
}

func (c *rtcConnectionImpl) Out() <-chan *pipeline.PipelineMessage {
	return c.outChan
}

func (c *rtcConnectionImpl) SetAudioEncodeParam(sampleRate int, channels int, bitRate int) {

	// 增加判断，如果参数和之前一样，则不重新创建
	if c.inSampleRate == sampleRate && c.inChannels == channels && c.inBitRate == bitRate {
		return
	}

	c.inSampleRate = sampleRate
	c.inChannels = channels
	c.inBitRate = bitRate

	inLayout := astiav.ChannelLayoutMono
	if channels == 1 {
		inLayout = astiav.ChannelLayoutMono
	} else if channels == 2 {
		inLayout = astiav.ChannelLayoutStereo
	}

	inResample, err := audio.NewResample(24000, sampleRate, astiav.ChannelLayoutMono, inLayout)
	if err != nil {
		log.Fatalf("failed to create resample: %v", err)
	}

	c.inResample = inResample

	encoder, err := opus.NewEncoder(sampleRate, channels, opus.AppVoIP)
	if err != nil {
		log.Fatalf("failed to create encoder: %v", err)
	}
	encoder.SetBitrate(bitRate)
	encoder.SetComplexity(10)
	encoder.SetDTX(true)

	c.audioEncoder = encoder
}

func (c *rtcConnectionImpl) SetAudioOutputParam(sampleRate int, channels int) {

	// 增加判断，如果参数和之前一样，则不重新创建
	if c.outSampleRate == sampleRate && c.outChannels == channels {
		return
	}

	c.outSampleRate = sampleRate
	c.outChannels = channels

	outLayout := astiav.ChannelLayoutMono
	if channels == 1 {
		outLayout = astiav.ChannelLayoutMono
	} else if channels == 2 {
		outLayout = astiav.ChannelLayoutStereo
	}

	outResample, err := audio.NewResample(48000, sampleRate, outLayout, outLayout)
	if err != nil {
		log.Fatalf("failed to create resample: %v", err)
	}

	c.outResample = outResample

	decoder, err := opus.NewDecoder(sampleRate, channels)
	if err != nil {
		log.Fatalf("failed to create decoder: %v", err)
	}

	c.audioDecoder = decoder
}

func (c *rtcConnectionImpl) Start(ctx context.Context) error {

	c.pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("OnConnectionStateChange: %v", state)
	})

	// 如果你想在 wrapper 里设置 pc.OnDataChannel，也行；也可以在这里：
	c.pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		// 这里可能需要把 channel 存到 wrapper 里
		//wrapper.AddDataChannel(dc)
		log.Printf("OnDataChannel: %v", dc.Label())
	})

	dc, err := c.pc.CreateDataChannel("realtime-ai", nil)
	if err != nil {
		log.Println("create data channel error:", err)
		return err
	}

	c.dataChannel = dc

	transceiver, err := c.pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendrecv,
	})

	c.localAudioTrack = transceiver.Sender().Track().(*webrtc.TrackLocalStaticSample)

	log.Printf("localAudioTrack: %v, remoteAudioTrack: %v\n", c.localAudioTrack, c.remoteAudioTrack)

	if err != nil {
		log.Println("add transceiver error:", err)
		return err
	}

	// 将 wrapper 加入 server 管理

	c.pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("OnTrack: %v, codec: %v", track.ID(), track.Codec().MimeType)
		if track.Kind() == webrtc.RTPCodecTypeAudio {
			c.remoteAudioTrack = track
			go c.readRemoteAudio(ctx)
		}
	})

	return nil
}

func (c *rtcConnectionImpl) Close() error {
	return c.pc.Close()
}

func (c *rtcConnectionImpl) readRemoteAudio(ctx context.Context) {

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

			// 将拿到的 payload 投递给 pipeline 的“输入 element”
			msg := pipeline.PipelineMessage{
				Type: pipeline.MsgTypeAudio,
				AudioData: &pipeline.AudioData{
					Data:       rtpPacket.Payload,
					SampleRate: 48000,
					Channels:   1,
					MediaType:  "audio/x-opus",
					Codec:      "opus",
					Timestamp:  time.Now(),
				},
			}

			log.Printf("readRemoteAudio: %v", msg)
		}
	}
}
