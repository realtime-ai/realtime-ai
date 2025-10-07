package connection

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	pb "github.com/realtime-ai/realtime-ai/pkg/proto/streamingai/v1"

	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

type grpcConnectionImpl struct {
	peerID string

	// gRPC stream
	stream pb.StreamingAIService_BiDirectionalStreamingServer

	// Event handler
	handler ConnectionEventHandler

	// Message queues (not used for direct streaming, kept for interface compatibility)
	inChan  chan *pipeline.PipelineMessage
	outChan chan *pipeline.PipelineMessage

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once

	// Connection state
	state webrtc.PeerConnectionState
}

var _ RTCConnection = (*grpcConnectionImpl)(nil)

// NewGRPCConnection creates a new gRPC-based connection that implements RTCConnection interface
func NewGRPCConnection(
	peerID string,
	stream pb.StreamingAIService_BiDirectionalStreamingServer,
) RTCConnection {

	ctx, cancel := context.WithCancel(stream.Context())

	conn := &grpcConnectionImpl{
		peerID:  peerID,
		stream:  stream,
		handler: &NoOpConnectionEventHandler{},
		inChan:  make(chan *pipeline.PipelineMessage, 50),
		outChan: make(chan *pipeline.PipelineMessage, 50),
		ctx:     ctx,
		cancel:  cancel,
		state:   webrtc.PeerConnectionStateNew,
	}

	return conn
}

func (c *grpcConnectionImpl) PeerID() string {
	return c.peerID
}

func (c *grpcConnectionImpl) RegisterEventHandler(handler ConnectionEventHandler) {
	c.handler = handler

	// Start processing after handler is registered
	c.wg.Add(1)
	go c.receiveLoop()

	// Notify connection is ready
	c.state = webrtc.PeerConnectionStateConnected
	c.handler.OnConnectionStateChange(c.state)
}

func (c *grpcConnectionImpl) SendMessage(msg *pipeline.PipelineMessage) {
	// Convert PipelineMessage to protobuf StreamMessage
	pbMsg := c.pipelineMessageToProto(msg)

	if err := c.stream.Send(pbMsg); err != nil {
		log.Printf("[GRPCConnection] Failed to send message: %v", err)
		c.handler.OnError(err)
	}
}

func (c *grpcConnectionImpl) Close() error {
	c.once.Do(func() {
		log.Printf("[GRPCConnection] Closing connection: %s", c.peerID)

		c.state = webrtc.PeerConnectionStateClosed
		c.handler.OnConnectionStateChange(c.state)

		c.cancel()
		c.wg.Wait()
		close(c.inChan)
		close(c.outChan)
	})
	return nil
}

// receiveLoop continuously receives messages from gRPC stream
func (c *grpcConnectionImpl) receiveLoop() {
	defer c.wg.Done()

	log.Printf("[GRPCConnection] Starting receive loop for peer: %s", c.peerID)

	for {
		select {
		case <-c.ctx.Done():
			log.Printf("[GRPCConnection] Context done, stopping receive loop")
			return
		default:
			pbMsg, err := c.stream.Recv()
			if err != nil {
				log.Printf("[GRPCConnection] Stream receive error: %v", err)
				c.state = webrtc.PeerConnectionStateFailed
				c.handler.OnConnectionStateChange(c.state)
				c.handler.OnError(err)
				return
			}

			// Convert protobuf message to PipelineMessage
			pipelineMsg := c.protoToPipelineMessage(pbMsg)
			if pipelineMsg != nil {
				c.handler.OnMessage(pipelineMsg)
			}
		}
	}
}

// pipelineMessageToProto converts PipelineMessage to protobuf StreamMessage
func (c *grpcConnectionImpl) pipelineMessageToProto(msg *pipeline.PipelineMessage) *pb.StreamMessage {
	pbMsg := &pb.StreamMessage{
		SessionId: msg.SessionID,
		Timestamp: msg.Timestamp.UnixNano(),
	}

	switch msg.Type {
	case pipeline.MsgTypeAudio:
		if msg.AudioData != nil {
			pbMsg.Type = pb.MessageType_MESSAGE_TYPE_AUDIO
			pbMsg.Payload = &pb.StreamMessage_Audio{
				Audio: &pb.AudioFrame{
					Data:       msg.AudioData.Data,
					SampleRate: int32(msg.AudioData.SampleRate),
					Channels:   int32(msg.AudioData.Channels),
					MediaType:  msg.AudioData.MediaType,
					Codec:      msg.AudioData.Codec,
				},
			}
		}

	case pipeline.MsgTypeVideo:
		if msg.VideoData != nil {
			pbMsg.Type = pb.MessageType_MESSAGE_TYPE_VIDEO
			pbMsg.Payload = &pb.StreamMessage_Video{
				Video: &pb.VideoFrame{
					Data:            msg.VideoData.Data,
					Width:           int32(msg.VideoData.Width),
					Height:          int32(msg.VideoData.Height),
					MediaType:       msg.VideoData.MediaType,
					Format:          msg.VideoData.Format,
					Codec:           msg.VideoData.Codec,
					FramerateNum:    int32(msg.VideoData.FramerateNum),
					FramerateDenom:  int32(msg.VideoData.FramerateDenom),
				},
			}
		}

	case pipeline.MsgTypeData:
		if msg.TextData != nil {
			pbMsg.Type = pb.MessageType_MESSAGE_TYPE_TEXT
			pbMsg.Payload = &pb.StreamMessage_Text{
				Text: &pb.TextMessage{
					Data:     msg.TextData.Data,
					TextType: msg.TextData.TextType,
				},
			}
		}

	case pipeline.MsgTypeCommand:
		pbMsg.Type = pb.MessageType_MESSAGE_TYPE_CONTROL
		// Control messages can be extended as needed
	}

	return pbMsg
}

// protoToPipelineMessage converts protobuf StreamMessage to PipelineMessage
func (c *grpcConnectionImpl) protoToPipelineMessage(pbMsg *pb.StreamMessage) *pipeline.PipelineMessage {
	if pbMsg == nil {
		return nil
	}

	msg := &pipeline.PipelineMessage{
		SessionID: pbMsg.SessionId,
		Timestamp: time.Unix(0, pbMsg.Timestamp),
	}

	switch pbMsg.Type {
	case pb.MessageType_MESSAGE_TYPE_AUDIO:
		if audio := pbMsg.GetAudio(); audio != nil {
			msg.Type = pipeline.MsgTypeAudio
			msg.AudioData = &pipeline.AudioData{
				Data:       audio.Data,
				SampleRate: int(audio.SampleRate),
				Channels:   int(audio.Channels),
				MediaType:  audio.MediaType,
				Codec:      audio.Codec,
				Timestamp:  msg.Timestamp,
			}
		}

	case pb.MessageType_MESSAGE_TYPE_VIDEO:
		if video := pbMsg.GetVideo(); video != nil {
			msg.Type = pipeline.MsgTypeVideo
			msg.VideoData = &pipeline.VideoData{
				Data:           video.Data,
				Width:          int(video.Width),
				Height:         int(video.Height),
				MediaType:      video.MediaType,
				Format:         video.Format,
				Codec:          video.Codec,
				FramerateNum:   int(video.FramerateNum),
				FramerateDenom: int(video.FramerateDenom),
				Timestamp:      msg.Timestamp,
			}
		}

	case pb.MessageType_MESSAGE_TYPE_TEXT:
		if text := pbMsg.GetText(); text != nil {
			msg.Type = pipeline.MsgTypeData
			msg.TextData = &pipeline.TextData{
				Data:      text.Data,
				TextType:  text.TextType,
				Timestamp: msg.Timestamp,
			}
		}

	case pb.MessageType_MESSAGE_TYPE_CONTROL:
		msg.Type = pipeline.MsgTypeCommand
		// Handle control messages as needed
		if control := pbMsg.GetControl(); control != nil {
			switch control.ControlType {
			case pb.ControlType_CONTROL_TYPE_STATE_CHANGE:
				if stateChange := control.GetStateChange(); stateChange != nil {
					c.handleStateChange(stateChange.State)
				}
			case pb.ControlType_CONTROL_TYPE_ERROR:
				if errInfo := control.GetError(); errInfo != nil {
					log.Printf("[GRPCConnection] Received error: %s - %s", errInfo.Code, errInfo.Message)
				}
			}
		}
	}

	return msg
}

// handleStateChange processes connection state change messages
func (c *grpcConnectionImpl) handleStateChange(state pb.ConnectionState) {
	switch state {
	case pb.ConnectionState_CONNECTION_STATE_CONNECTING:
		c.state = webrtc.PeerConnectionStateConnecting
	case pb.ConnectionState_CONNECTION_STATE_CONNECTED:
		c.state = webrtc.PeerConnectionStateConnected
	case pb.ConnectionState_CONNECTION_STATE_DISCONNECTED:
		c.state = webrtc.PeerConnectionStateDisconnected
	case pb.ConnectionState_CONNECTION_STATE_FAILED:
		c.state = webrtc.PeerConnectionStateFailed
	}
	c.handler.OnConnectionStateChange(c.state)
}
