// Example Realtime API Server
//
// This example demonstrates how to create a Realtime API server
// similar to OpenAI's Realtime API using the realtime-ai framework.
//
// Usage:
//
//	go run examples/realtime-api-server/main.go
//
// Then connect via WebSocket:
//
//	wscat -c "ws://localhost:8080/v1/realtime?model=echo"
package main

import (
	"context"
	_ "embed"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi"
)

//go:embed index.html
var indexHTML []byte

func main() {
	// Load environment variables from .env file
	godotenv.Load()

	// Create server configuration
	config := realtimeapi.DefaultServerConfig()
	config.Addr = ":8080"
	config.Path = "/v1/realtime"
	config.DefaultModel = "echo"
	config.AllowedModels = []string{"echo", "gemini-2.0-flash", "gemini-1.5-pro"}

	// Optional: Set authentication token
	if token := os.Getenv("API_TOKEN"); token != "" {
		config.AuthToken = token
	}

	// Create server
	server := realtimeapi.NewServer(config)

	// Set pipeline factory - using echo pipeline for demo
	server.SetPipelineFactory(createEchoPipeline)

	// Serve frontend page at root
	server.RegisterHandler("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})

	// Start server
	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	log.Printf("Realtime API server running on %s%s", config.Addr, config.Path)
	log.Printf("Web UI: http://localhost%s", config.Addr)
	log.Println("Connect with: wscat -c 'ws://localhost:8080/v1/realtime?model=echo'")
	log.Println("Press Ctrl+C to stop")

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Stop(shutdownCtx); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}
	log.Println("Server stopped")
}

// createEchoPipeline creates a simple echo pipeline for testing.
// This pipeline just passes audio through without processing.
func createEchoPipeline(ctx context.Context, session *realtimeapi.Session) (*pipeline.Pipeline, error) {
	// Create pipeline
	p := pipeline.NewPipeline("realtime-api-" + session.ID)

	// Create a simple pass-through element
	echo := NewEchoElement()
	p.AddElement(echo)

	return p, nil
}

// EchoElement is a simple element that echoes audio back.
type EchoElement struct {
	*pipeline.BaseElement
}

// NewEchoElement creates a new EchoElement.
func NewEchoElement() *EchoElement {
	return &EchoElement{
		BaseElement: pipeline.NewBaseElement("echo", 100),
	}
}

// Start implements pipeline.Element.
func (e *EchoElement) Start(ctx context.Context) error {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-e.InChan:
				if msg == nil {
					continue
				}
				// Echo the message back
				select {
				case e.OutChan <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return nil
}

// Stop implements pipeline.Element.
func (e *EchoElement) Stop() error {
	return nil
}
