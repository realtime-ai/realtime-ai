// Example: Agent-based Pipeline
//
// This example demonstrates how to use the Agent system with the Pipeline.
//
// Run with:
//   go run examples/agent-pipeline/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/agent"
	"github.com/realtime-ai/realtime-ai/pkg/agents"
	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

func main() {
	log.Println("=== Agent Pipeline Example ===")

	// Create coordinator
	coordinator := agent.NewCoordinator(nil)

	// Register agents
	coordinator.Register(agents.NewEchoAgent())
	coordinator.Register(agents.NewTimeAgent())
	coordinator.Register(agents.NewWeatherAgent())

	log.Printf("Registered %d agents:", len(coordinator.ListAgents()))
	for _, a := range coordinator.ListAgents() {
		log.Printf("  - %s: %s", a.Name(), a.Description())
	}

	// Create pipeline with Agent element
	p := pipeline.NewPipeline("agent-pipeline")

	// Create agent element
	agentElement := elements.NewAgentElement("agent-coordinator", coordinator)
	p.AddElement(agentElement)

	// Start pipeline
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := p.Start(ctx); err != nil {
		log.Fatalf("Failed to start pipeline: %v", err)
	}

	log.Println("Pipeline started. Testing agents...")

	// Test messages
	testMessages := []string{
		"Hello, can you echo this?",
		"What time is it?",
		"What's the weather like in Beijing?",
		"What's the date today?",
	}

	sessionID := "test-session-001"

	for _, text := range testMessages {
		log.Printf("\n[User] %s", text)

		// Send message to pipeline
		p.Push(&pipeline.PipelineMessage{
			Type:      pipeline.MsgTypeData,
			SessionID: sessionID,
			Timestamp: time.Now(),
			TextData: &pipeline.TextData{
				Data:      []byte(text),
				TextType:  "user_input",
				Timestamp: time.Now(),
			},
		})

		// Wait for response (with timeout)
		select {
		case response := <-p.Pull():
			if response != nil && response.TextData != nil {
				log.Printf("[Agent] %s", string(response.TextData.Data))
			}
		case <-time.After(2 * time.Second):
			log.Println("[Agent] Timeout waiting for response")
		}

		time.Sleep(500 * time.Millisecond)
	}

	// Interactive mode
	log.Println("\n=== Interactive Mode ===")
	log.Println("Type messages and press Enter (or 'quit' to exit):")

	go func() {
		for {
			var input string
			fmt.Print("\n> ")
			fmt.Scanln(&input)

			if input == "quit" {
				cancel()
				return
			}

			if input == "" {
				continue
			}

			p.Push(&pipeline.PipelineMessage{
				Type:      pipeline.MsgTypeData,
				SessionID: sessionID,
				Timestamp: time.Now(),
				TextData: &pipeline.TextData{
					Data:      []byte(input),
					TextType:  "user_input",
					Timestamp: time.Now(),
				},
			})

			select {
			case response := <-p.Pull():
				if response != nil && response.TextData != nil {
					log.Printf("[Agent] %s", string(response.TextData.Data))
				}
			case <-time.After(2 * time.Second):
				log.Println("[Agent] Timeout")
			}
		}
	}()

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigChan:
		log.Println("\nShutting down...")
	case <-ctx.Done():
		log.Println("\nExiting...")
	}

	// Stop pipeline
	if err := p.Stop(); err != nil {
		log.Printf("Error stopping pipeline: %v", err)
	}

	log.Println("Done!")
}
