package main

import (
	"context"
	"io"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/realtime-ai/realtime-ai/pkg/proto/streamingai/v1"
)

// SimpleGRPCClient demonstrates how to use the gRPC streaming API
type SimpleGRPCClient struct {
	client pb.StreamingAIServiceClient
	conn   *grpc.ClientConn
}

// NewSimpleGRPCClient creates a new gRPC client
func NewSimpleGRPCClient(serverAddr string) (*SimpleGRPCClient, error) {
	conn, err := grpc.NewClient(
		serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(16*1024*1024),
			grpc.MaxCallSendMsgSize(16*1024*1024),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %v", err)
	}

	client := pb.NewStreamingAIServiceClient(conn)

	return &SimpleGRPCClient{
		client: client,
		conn:   conn,
	}, nil
}

// Close closes the gRPC connection
func (c *SimpleGRPCClient) Close() error {
	return c.conn.Close()
}

// StartBidirectionalStream starts a bidirectional streaming session
func (c *SimpleGRPCClient) StartBidirectionalStream(ctx context.Context) error {
	stream, err := c.client.BiDirectionalStreaming(ctx)
	if err != nil {
		return fmt.Errorf("failed to start stream: %v", err)
	}

	// Start receiver goroutine
	go func() {
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				log.Println("[Client] Server closed the stream")
				return
			}
			if err != nil {
				log.Printf("[Client] Receive error: %v", err)
				return
			}

			c.handleReceivedMessage(msg)
		}
	}()

	// Send a test audio frame every 2 seconds
	sessionID := "test-session-001"
	for i := 0; i < 5; i++ {
		// Simulate sending audio data (PCM format, 48kHz, mono, 20ms)
		audioData := make([]byte, 1920) // 48000 Hz * 1 channel * 20ms * 2 bytes/sample
		for j := range audioData {
			audioData[j] = byte(i * j % 256) // dummy data
		}

		msg := &pb.StreamMessage{
			SessionId: sessionID,
			Type:      pb.MessageType_MESSAGE_TYPE_AUDIO,
			Timestamp: time.Now().UnixNano(),
			Payload: &pb.StreamMessage_Audio{
				Audio: &pb.AudioFrame{
					Data:       audioData,
					SampleRate: 48000,
					Channels:   1,
					MediaType:  "audio/x-raw",
					Codec:      "pcm",
					DurationMs: 20,
				},
			},
		}

		if err := stream.Send(msg); err != nil {
			log.Printf("[Client] Send error: %v", err)
			return err
		}

		log.Printf("[Client] Sent audio frame #%d", i+1)
		time.Sleep(2 * time.Second)
	}

	// Send a text message
	textMsg := &pb.StreamMessage{
		SessionId: sessionID,
		Type:      pb.MessageType_MESSAGE_TYPE_TEXT,
		Timestamp: time.Now().UnixNano(),
		Payload: &pb.StreamMessage_Text{
			Text: &pb.TextMessage{
				Data:     []byte("Hello, Gemini!"),
				TextType: "plain",
			},
		},
	}

	if err := stream.Send(textMsg); err != nil {
		log.Printf("[Client] Send text error: %v", err)
		return err
	}

	log.Println("[Client] Sent text message")

	// Close the send side
	if err := stream.CloseSend(); err != nil {
		log.Printf("[Client] CloseSend error: %v", err)
	}

	// Wait a bit for final responses
	time.Sleep(3 * time.Second)

	return nil
}

// handleReceivedMessage processes received messages
func (c *SimpleGRPCClient) handleReceivedMessage(msg *pb.StreamMessage) {
	switch msg.Type {
	case pb.MessageType_MESSAGE_TYPE_AUDIO:
		audio := msg.GetAudio()
		log.Printf("[Client] Received audio: %d bytes, %d Hz, %d channels",
			len(audio.Data), audio.SampleRate, audio.Channels)

	case pb.MessageType_MESSAGE_TYPE_TEXT:
		text := msg.GetText()
		log.Printf("[Client] Received text: %s", string(text.Data))

	case pb.MessageType_MESSAGE_TYPE_CONTROL:
		control := msg.GetControl()
		log.Printf("[Client] Received control: %v", control.ControlType)

	default:
		log.Printf("[Client] Received unknown message type: %v", msg.Type)
	}
}

func runClient() {
	serverAddr := "localhost:50051"

	client, err := NewSimpleGRPCClient(serverAddr)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	log.Printf("[Client] Connected to server: %s", serverAddr)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.StartBidirectionalStream(ctx); err != nil {
		log.Fatalf("Stream error: %v", err)
	}

	log.Println("[Client] Stream completed")
}

func main() {
	log.Println("[Client] Starting gRPC client example")
	runClient()
}
