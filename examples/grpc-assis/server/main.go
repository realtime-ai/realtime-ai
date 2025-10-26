package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/pion/webrtc/v4"
	"github.com/realtime-ai/realtime-ai/pkg/connection"
	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/server"
)

type connectionEventHandler struct {
	connection.ConnectionEventHandler

	conn     connection.RTCConnection
	pipeline *pipeline.Pipeline
}

func (c *connectionEventHandler) OnConnectionStateChange(state webrtc.PeerConnectionState) {
	log.Printf("[Handler] OnConnectionStateChange: %v", state)

	if state == webrtc.PeerConnectionStateConnected {
		// Create pipeline elements
		playoutSinkElement := elements.NewPlayoutSinkElement()
		geminiElement := elements.NewGeminiElement()
		audioResampleElement := elements.NewAudioResampleElement(48000, 16000, 1, 1)

		// Create pipeline
		elements := []pipeline.Element{
			audioResampleElement,
			geminiElement,
			playoutSinkElement,
		}

		pipeline := pipeline.NewPipeline("grpc_connection")
		pipeline.AddElements(elements)

		// Link elements
		pipeline.Link(audioResampleElement, geminiElement)
		pipeline.Link(geminiElement, playoutSinkElement)

		c.pipeline = pipeline

		// Start pipeline
		pipeline.Start(context.Background())

		// Start goroutine to pull messages from pipeline and send to client
		go func() {
			for {
				msg := c.pipeline.Pull()
				if msg != nil {
					c.conn.SendMessage(msg)
				}
			}
		}()

		log.Println("[Handler] Pipeline started and connected")
	}
}

func (c *connectionEventHandler) OnMessage(msg *pipeline.PipelineMessage) {
	if c.pipeline != nil {
		c.pipeline.Push(msg)
	}
}

func (c *connectionEventHandler) OnError(err error) {
	log.Printf("[Handler] OnError: %v", err)
}

// StartGRPCServer starts the gRPC server
func StartGRPCServer(port int) error {
	cfg := &server.GRPCServerConfig{
		Port: port,
	}

	grpcServer := server.NewGRPCServer(cfg)

	// Register callback for new connections
	grpcServer.OnConnectionCreated(func(ctx context.Context, conn connection.RTCConnection) {
		log.Printf("[Server] New connection created: %s", conn.PeerID())
		conn.RegisterEventHandler(&connectionEventHandler{
			conn: conn,
		})
	})

	// Register callback for connection errors
	grpcServer.OnConnectionError(func(ctx context.Context, conn connection.RTCConnection, err error) {
		log.Printf("[Server] Connection error: %s, error: %v", conn.PeerID(), err)
	})

	// Start server (blocking)
	return grpcServer.Start()
}

func main() {
	// Load environment variables
	godotenv.Load()

	// Check for required API keys
	if os.Getenv("GOOGLE_API_KEY") == "" {
		log.Fatal("GOOGLE_API_KEY environment variable is required")
	}

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		log.Println("[Main] Starting gRPC Gemini Assistant Server on port 50051")
		if err := StartGRPCServer(50051); err != nil {
			log.Fatalf("[Main] Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal
	<-sigChan
	log.Println("[Main] Shutting down server...")
}
