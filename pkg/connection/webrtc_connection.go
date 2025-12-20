package connection

import (
	"context"
	"io"
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
	DefaultWebRTCSampleRate = 48000
	DefaultWebRTCChannels   = 1
	DefaultWebRTCBitRate    = 50000
)

// WebRTCConfig holds configuration for WebRTC connection.
type WebRTCConfig struct {
	SampleRate int
	Channels   int
	BitRate    int
}

// DefaultWebRTCConfig returns the default WebRTC configuration.
func DefaultWebRTCConfig() WebRTCConfig {
	return WebRTCConfig{
		SampleRate: DefaultWebRTCSampleRate,
		Channels:   DefaultWebRTCChannels,
		BitRate:    DefaultWebRTCBitRate,
	}
}

type webrtcConnection struct {
	peerID string
	pc     *webrtc.PeerConnection

	// WebRTC components
	dataChannel      *webrtc.DataChannel
	remoteAudioTrack *webrtc.TrackRemote
	localAudioTrack  *webrtc.TrackLocalStaticSample

	// Event handler
	handler ConnectionEventHandler

	// Audio codec
	audioEncoder *opus.Encoder
	audioDecoder *opus.Decoder

	// Audio parameters
	sampleRate int
	channels   int
	bitRate    int

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once
	mu     sync.RWMutex
}

var _ Connection = (*webrtcConnection)(nil)

// NewWebRTCConnection creates a new WebRTC connection with default config.
func NewWebRTCConnection(peerID string, pc *webrtc.PeerConnection) Connection {
	return NewWebRTCConnectionWithConfig(peerID, pc, DefaultWebRTCConfig())
}

// NewWebRTCConnectionWithConfig creates a new WebRTC connection with custom config.
func NewWebRTCConnectionWithConfig(peerID string, pc *webrtc.PeerConnection, cfg WebRTCConfig) Connection {
	audioEncoder, err := opus.NewEncoder(cfg.SampleRate, cfg.Channels, opus.AppVoIP)
	if err != nil {
		log.Fatalf("failed to create opus encoder: %v", err)
	}
	audioEncoder.SetBitrate(cfg.BitRate)
	audioEncoder.SetComplexity(10)
	audioEncoder.SetDTX(true)

	audioDecoder, err := opus.NewDecoder(cfg.SampleRate, cfg.Channels)
	if err != nil {
		log.Fatalf("failed to create opus decoder: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	conn := &webrtcConnection{
		peerID:       peerID,
		pc:           pc,
		handler:      &NoOpConnectionEventHandler{},
		audioEncoder: audioEncoder,
		audioDecoder: audioDecoder,
		sampleRate:   cfg.SampleRate,
		channels:     cfg.Channels,
		bitRate:      cfg.BitRate,
		ctx:          ctx,
		cancel:       cancel,
	}

	conn.start()

	return conn
}

func (c *webrtcConnection) PeerID() string {
	return c.peerID
}

func (c *webrtcConnection) RegisterEventHandler(handler ConnectionEventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = handler
}

func (c *webrtcConnection) start() {
	// Map WebRTC states to ConnectionState
	c.pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		c.mu.RLock()
		handler := c.handler
		c.mu.RUnlock()

		connState := mapWebRTCState(state)
		handler.OnConnectionStateChange(connState)
	})

	// Handle incoming DataChannel for text messages
	c.pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		c.mu.Lock()
		c.dataChannel = dc
		c.mu.Unlock()

		c.setupDataChannel(dc)
	})

	// Setup audio transceiver
	transceiver, err := c.pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendrecv,
	})
	if err != nil {
		log.Printf("[webrtc %s] failed to add transceiver: %v", c.peerID, err)
		return
	}

	// Get local audio track from transceiver
	if sender := transceiver.Sender(); sender != nil {
		if track := sender.Track(); track != nil {
			c.localAudioTrack = track.(*webrtc.TrackLocalStaticSample)
		}
	}

	// Handle incoming audio track
	c.pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("[webrtc %s] OnTrack: %v, codec: %v", c.peerID, track.ID(), track.Codec().MimeType)
		if track.Kind() == webrtc.RTPCodecTypeAudio {
			c.mu.Lock()
			c.remoteAudioTrack = track
			c.mu.Unlock()

			c.wg.Add(1)
			go c.readRemoteAudio()
		}
	})
}

func (c *webrtcConnection) setupDataChannel(dc *webrtc.DataChannel) {
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		pipelineMsg := &pipeline.PipelineMessage{
			Type: pipeline.MsgTypeData,
			TextData: &pipeline.TextData{
				Data:      msg.Data,
				TextType:  "text",
				Timestamp: time.Now(),
			},
		}

		c.mu.RLock()
		handler := c.handler
		c.mu.RUnlock()

		handler.OnMessage(pipelineMsg)
	})

	dc.OnOpen(func() {
		log.Printf("[webrtc %s] DataChannel opened", c.peerID)
	})
}

func (c *webrtcConnection) readRemoteAudio() {
	defer c.wg.Done()

	log.Printf("[webrtc %s] 开始读取远程音频...", c.peerID)

	pcmBuf := make([]int16, 1920) // 20ms at 48kHz stereo
	frameCount := 0

	for {
		select {
		case <-c.ctx.Done():
			log.Printf("[webrtc %s] 音频读取被取消", c.peerID)
			return
		default:
			c.mu.RLock()
			track := c.remoteAudioTrack
			c.mu.RUnlock()

			if track == nil {
				log.Printf("[webrtc %s] 远程音频轨道为空，退出", c.peerID)
				return
			}

			rtpPacket, _, err := track.ReadRTP()
			if err != nil {
				// EOF 是正常的连接关闭
				if err == io.EOF {
					log.Printf("[webrtc %s] 远程音频轨道已关闭", c.peerID)
					return
				}
				log.Printf("[webrtc %s] RTP read error: %v", c.peerID, err)
				continue
			}

			// 跳过空的 RTP 包
			if len(rtpPacket.Payload) == 0 {
				continue
			}

			// Decode Opus to PCM
			n, err := c.audioDecoder.Decode(rtpPacket.Payload, pcmBuf)
			if err != nil {
				log.Printf("[webrtc %s] Opus decode error: %v", c.peerID, err)
				continue
			}

			audioData := utils.Int16SliceToByteSlice(pcmBuf[:n])

			frameCount++
			if frameCount%100 == 1 { // 每 100 帧打印一次（约 2 秒）
				log.Printf("[webrtc %s] 收到音频帧 #%d: %d samples, %d bytes",
					c.peerID, frameCount, n, len(audioData))
			}

			msg := &pipeline.PipelineMessage{
				Type: pipeline.MsgTypeAudio,
				AudioData: &pipeline.AudioData{
					Data:       audioData,
					SampleRate: c.sampleRate,
					Channels:   c.channels,
					MediaType:  "audio/x-raw",
					Timestamp:  time.Now(),
				},
			}

			c.mu.RLock()
			handler := c.handler
			c.mu.RUnlock()

			handler.OnMessage(msg)
		}
	}
}

func (c *webrtcConnection) SendMessage(msg *pipeline.PipelineMessage) {
	switch msg.Type {
	case pipeline.MsgTypeData:
		c.sendTextMessage(msg)
	case pipeline.MsgTypeAudio:
		c.sendAudioMessage(msg)
	}
}

func (c *webrtcConnection) sendTextMessage(msg *pipeline.PipelineMessage) {
	c.mu.RLock()
	dc := c.dataChannel
	c.mu.RUnlock()

	if dc == nil || dc.ReadyState() != webrtc.DataChannelStateOpen {
		log.Printf("[webrtc %s] DataChannel not open", c.peerID)
		return
	}

	if err := dc.Send(msg.TextData.Data); err != nil {
		log.Printf("[webrtc %s] failed to send text: %v", c.peerID, err)
	}
}

func (c *webrtcConnection) sendAudioMessage(msg *pipeline.PipelineMessage) {
	if msg.AudioData == nil || msg.AudioData.MediaType != "audio/x-raw" {
		return
	}

	c.mu.RLock()
	track := c.localAudioTrack
	c.mu.RUnlock()

	if track == nil {
		return
	}

	// Encode PCM to Opus
	opusBuf := make([]byte, 1275)
	pcm := utils.ByteSliceToInt16Slice(msg.AudioData.Data)

	n, err := c.audioEncoder.Encode(pcm, opusBuf)
	if err != nil {
		log.Printf("[webrtc %s] Opus encode error: %v", c.peerID, err)
		return
	}

	// Write to RTP track
	sample := media.Sample{
		Data:     opusBuf[:n],
		Duration: 20 * time.Millisecond,
	}

	if err := track.WriteSample(sample); err != nil {
		log.Printf("[webrtc %s] failed to write audio sample: %v", c.peerID, err)
	}
}

func (c *webrtcConnection) Close() error {
	c.once.Do(func() {
		c.cancel()
		c.wg.Wait()
		if c.pc != nil {
			c.pc.Close()
		}
	})
	return nil
}

// mapWebRTCState maps WebRTC PeerConnectionState to ConnectionState.
func mapWebRTCState(state webrtc.PeerConnectionState) ConnectionState {
	switch state {
	case webrtc.PeerConnectionStateNew:
		return ConnectionStateNew
	case webrtc.PeerConnectionStateConnecting:
		return ConnectionStateConnecting
	case webrtc.PeerConnectionStateConnected:
		return ConnectionStateConnected
	case webrtc.PeerConnectionStateDisconnected:
		return ConnectionStateDisconnected
	case webrtc.PeerConnectionStateFailed:
		return ConnectionStateFailed
	case webrtc.PeerConnectionStateClosed:
		return ConnectionStateClosed
	default:
		return ConnectionStateFailed
	}
}
