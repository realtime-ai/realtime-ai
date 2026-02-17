// Package elements provides pipeline elements for the agent system.
package elements

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/agent"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

// AgentElement integrates the Agent system into the Pipeline
type AgentElement struct {
	*pipeline.BaseElement
	
	coordinator *agent.Coordinator
	
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewAgentElement creates a new agent element
func NewAgentElement(name string, coordinator *agent.Coordinator) *AgentElement {
	return &AgentElement{
		BaseElement: pipeline.NewBaseElement(name, 100),
		coordinator: coordinator,
	}
}

func (e *AgentElement) Start(ctx context.Context) error {
	e.ctx, e.cancel = context.WithCancel(ctx)
	
	// Start coordinator
	if err := e.coordinator.Start(e.ctx); err != nil {
		return fmt.Errorf("failed to start coordinator: %w", err)
	}
	
	// Start processing goroutine
	e.wg.Add(1)
	go e.processLoop()
	
	log.Printf("[AgentElement] Started with %d agents", len(e.coordinator.ListAgents()))
	return nil
}

func (e *AgentElement) Stop() error {
	if e.cancel != nil {
		e.cancel()
	}
	
	e.wg.Wait()
	
	// Stop coordinator
	if err := e.coordinator.Stop(); err != nil {
		log.Printf("[AgentElement] Error stopping coordinator: %v", err)
	}
	
	return nil
}

func (e *AgentElement) processLoop() {
	defer e.wg.Done()
	
	for {
		select {
		case <-e.ctx.Done():
			return
			
		case msg := <-e.InChan:
			if msg == nil {
				continue
			}
			
			// Convert pipeline message to agent message
			agentMsg := e.toAgentMessage(msg)
			if agentMsg == nil {
				// Pass through if not convertible
				e.OutChan <- msg
				continue
			}
			
			// Process through agent system
			response, err := e.coordinator.Process(e.ctx, msg.SessionID, agentMsg)
			if err != nil {
				log.Printf("[AgentElement] Error processing message: %v", err)
				// Send error response
				e.sendErrorResponse(msg.SessionID, err)
				continue
			}
			
			// Convert agent response back to pipeline message
			if response != nil {
				pipelineMsg := e.toPipelineMessage(response, msg)
				e.OutChan <- pipelineMsg
			}
		}
	}
}

func (e *AgentElement) toAgentMessage(msg *pipeline.PipelineMessage) *agent.Message {
	switch msg.Type {
	case pipeline.MsgTypeAudio:
		if msg.AudioData == nil {
			return nil
		}
		return &agent.Message{
			ID:        msg.SessionID + "_audio",
			SessionID: msg.SessionID,
			Type:      agent.MessageTypeAudio,
			AudioData: msg.AudioData.Data,
			Timestamp: msg.Timestamp,
			Source:    "user",
		}
		
	case pipeline.MsgTypeData:
		if msg.TextData == nil {
			return nil
		}
		return &agent.Message{
			ID:          msg.SessionID + "_text",
			SessionID:   msg.SessionID,
			Type:        agent.MessageTypeText,
			TextContent: string(msg.TextData.Data),
			Timestamp:   msg.Timestamp,
			Source:      "user",
		}
		
	default:
		return nil
	}
}

func (e *AgentElement) toPipelineMessage(agentMsg *agent.Message, original *pipeline.PipelineMessage) *pipeline.PipelineMessage {
	switch agentMsg.Type {
	case agent.MessageTypeText:
		return &pipeline.PipelineMessage{
			Type:      pipeline.MsgTypeData,
			SessionID: agentMsg.SessionID,
			Timestamp: agentMsg.Timestamp,
			TextData: &pipeline.TextData{
				Data:      []byte(agentMsg.TextContent),
				TextType:  "agent_response",
				Timestamp: agentMsg.Timestamp,
			},
		}
		
	case agent.MessageTypeAudio:
		return &pipeline.PipelineMessage{
			Type:      pipeline.MsgTypeAudio,
			SessionID: agentMsg.SessionID,
			Timestamp: agentMsg.Timestamp,
			AudioData: &pipeline.AudioData{
				Data:       agentMsg.AudioData,
				SampleRate: 24000, // Default
				Channels:   1,
				MediaType:  pipeline.AudioMediaTypeRaw,
				Timestamp:  agentMsg.Timestamp,
			},
		}
		
	default:
		// Pass through original if unknown type
		return original
	}
}

func (e *AgentElement) sendErrorResponse(sessionID string, err error) {
	e.OutChan <- &pipeline.PipelineMessage{
		Type:      pipeline.MsgTypeData,
		SessionID: sessionID,
		Timestamp: time.Now(),
		TextData: &pipeline.TextData{
			Data:      []byte(fmt.Sprintf("Error: %v", err)),
			TextType:  "error",
			Timestamp: time.Now(),
		},
	}
}

// RegisterAgent registers an agent with the coordinator
func (e *AgentElement) RegisterAgent(a agent.Agent) {
	e.coordinator.Register(a)
}

// AgentCoordinatorElement is an alias for AgentElement
type AgentCoordinatorElement = AgentElement
