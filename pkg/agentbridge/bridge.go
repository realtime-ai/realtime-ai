// Package agentbridge connects the Agent system with Realtime API
package agentbridge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/agent"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi/events"
)

// RealtimeAgentBridge connects Agent system with Realtime API
type RealtimeAgentBridge struct {
	session     *realtimeapi.Session
	coordinator *agent.Coordinator
	
	// Audio handling
	audioBuffer []byte
	bufferSize  int
	
	// State
	mu          sync.RWMutex
	currentMsg  *agent.Message
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewRealtimeAgentBridge creates a new bridge
func NewRealtimeAgentBridge(session *realtimeapi.Session, coordinator *agent.Coordinator) *RealtimeAgentBridge {
	ctx, cancel := context.WithCancel(session.Context())
	return &RealtimeAgentBridge{
		session:     session,
		coordinator: coordinator,
		bufferSize:  4800, // 100ms at 24kHz, 16bit, mono
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start starts the bridge
func (b *RealtimeAgentBridge) Start() error {
	// Listen for pipeline output
	if p := b.session.GetPipeline(); p != nil {
		go b.handlePipelineOutput(p)
	}
	
	return nil
}

// Stop stops the bridge
func (b *RealtimeAgentBridge) Stop() {
	b.cancel()
}

// HandleAudioInput handles incoming audio from Realtime API
func (b *RealtimeAgentBridge) HandleAudioInput(audioData []byte, sampleRate, channels int) {
	// Accumulate audio
	b.audioBuffer = append(b.audioBuffer, audioData...)
	
	// When buffer is full enough, process
	if len(b.audioBuffer) >= b.bufferSize {
		// Create message
		msg := &agent.Message{
			ID:        generateID(),
			SessionID: b.session.ID,
			Type:      agent.MessageTypeAudio,
			AudioData: b.audioBuffer,
			Timestamp: time.Now(),
			Source:    "user",
		}
		
		// Clear buffer
		b.audioBuffer = nil
		
		// Process through agent system
		go b.processMessage(msg)
	}
}

// HandleTextInput handles incoming text from Realtime API
func (b *RealtimeAgentBridge) HandleTextInput(text string) {
	msg := &agent.Message{
		ID:          generateID(),
		SessionID:   b.session.ID,
		Type:        agent.MessageTypeText,
		TextContent: text,
		Timestamp:   time.Now(),
		Source:      "user",
	}
	
	b.processMessage(msg)
}

// processMessage sends message to agent system and handles response
func (b *RealtimeAgentBridge) processMessage(msg *agent.Message) {
	// Send to coordinator
	response, err := b.coordinator.Process(b.ctx, b.session.ID, msg)
	if err != nil {
		log.Printf("[AgentBridge] Error processing message: %v", err)
		b.sendErrorResponse(err)
		return
	}
	
	// Convert agent response to Realtime API events
	b.sendAgentResponse(response)
}

// sendAgentResponse converts agent message to Realtime API events
func (b *RealtimeAgentBridge) sendAgentResponse(msg *agent.Message) {
	// Get or create ResponseManager
	rm := b.session.GetResponseManager()
	if rm == nil {
		rm = realtimeapi.NewResponseManager(b.session)
		// Note: In actual implementation, this should be stored in session
	}
	
	// Create response
	config := events.ResponseConfig{
		Modalities: []events.Modality{events.ModalityText, events.ModalityAudio},
	}
	if err := rm.CreateResponse(config); err != nil {
		log.Printf("[AgentBridge] Error creating response: %v", err)
		return
	}
	
	// Send content based on type
	switch msg.Type {
	case agent.MessageTypeText:
		b.sendTextResponse(rm, msg.TextContent)
		
	case agent.MessageTypeAudio:
		b.sendAudioResponse(rm, msg.AudioData)
		
	case agent.MessageTypeEvent:
		b.sendEventResponse(rm, msg)
	}
	
	// Complete response
	if err := rm.CompleteContentPart(); err != nil {
		log.Printf("[AgentBridge] Error completing content part: %v", err)
	}
	if err := rm.CompleteOutputItem(); err != nil {
		log.Printf("[AgentBridge] Error completing output item: %v", err)
	}
	if err := rm.CompleteResponse(); err != nil {
		log.Printf("[AgentBridge] Error completing response: %v", err)
	}
}

func (b *RealtimeAgentBridge) sendTextResponse(rm *realtimeapi.ResponseManager, text string) {
	// Send text as deltas (simulate streaming)
	chunkSize := 20 // characters
	for i := 0; i < len(text); i += chunkSize {
		end := i + chunkSize
		if end > len(text) {
			end = len(text)
		}
		chunk := text[i:end]
		
		if err := rm.SendTextDelta(chunk); err != nil {
			log.Printf("[AgentBridge] Error sending text delta: %v", err)
		}
		
		// Small delay to simulate streaming
		time.Sleep(10 * time.Millisecond)
	}
}

func (b *RealtimeAgentBridge) sendAudioResponse(rm *realtimeapi.ResponseManager, audioData []byte) {
	// Send audio in chunks
	chunkSize := 4800 // 100ms
	for i := 0; i < len(audioData); i += chunkSize {
		end := i + chunkSize
		if end > len(audioData) {
			end = len(audioData)
		}
		chunk := audioData[i:end]
		
		if err := rm.SendAudioDelta(chunk); err != nil {
			log.Printf("[AgentBridge] Error sending audio delta: %v", err)
		}
	}
}

func (b *RealtimeAgentBridge) sendEventResponse(rm *realtimeapi.ResponseManager, msg *agent.Message) {
	// Handle special events like tool calls
	// This would parse msg.JSONData and send appropriate events
}

func (b *RealtimeAgentBridge) sendErrorResponse(err error) {
	// Send error event through session
	b.session.SendEvent(events.NewErrorEvent(
		events.ErrorTypeServer,
		"agent_processing_error",
		err.Error(),
		"",
	))
}

// handlePipelineOutput listens to pipeline output and forwards to agent system
func (b *RealtimeAgentBridge) handlePipelineOutput(p *pipeline.Pipeline) {
	for {
		select {
		case <-b.ctx.Done():
			return
		default:
			msg := p.Pull()
			if msg == nil {
				return
			}
			
			// Convert pipeline message to agent message
			agentMsg := b.convertPipelineMessage(msg)
			if agentMsg != nil {
				b.processMessage(agentMsg)
			}
		}
	}
}

func (b *RealtimeAgentBridge) convertPipelineMessage(msg *pipeline.PipelineMessage) *agent.Message {
	switch msg.Type {
	case pipeline.MsgTypeAudio:
		return &agent.Message{
			ID:        generateID(),
			SessionID: msg.SessionID,
			Type:      agent.MessageTypeAudio,
			AudioData: msg.AudioData.Data,
			Timestamp: msg.Timestamp,
			Source:    "pipeline",
		}
		
	case pipeline.MsgTypeData:
		return &agent.Message{
			ID:          generateID(),
			SessionID:   msg.SessionID,
			Type:        agent.MessageTypeText,
			TextContent: string(msg.TextData.Data),
			Timestamp:   msg.Timestamp,
			Source:      "pipeline",
		}
		
	default:
		return nil
	}
}

// AgentRealtimeServer wraps WebRTCRealtimeServer with Agent support
type AgentRealtimeServer struct {
	*realtimeapi.WebRTCRealtimeServer
	coordinator *agent.Coordinator
	bridges     map[string]*RealtimeAgentBridge
	mu          sync.RWMutex
}

// NewAgentRealtimeServer creates a server with agent support
func NewAgentRealtimeServer(config *realtimeapi.WebRTCRealtimeConfig, coordinator *agent.Coordinator) *AgentRealtimeServer {
	server := realtimeapi.NewWebRTCRealtimeServer(config)
	
	s := &AgentRealtimeServer{
		WebRTCRealtimeServer: server,
		coordinator:          coordinator,
		bridges:              make(map[string]*RealtimeAgentBridge),
	}
	
	// Set up connection handler
	server.OnConnectionCreated(s.handleConnectionCreated)
	
	return s
}

func (s *AgentRealtimeServer) handleConnectionCreated(ctx context.Context, conn realtimeapi.WebRTCRealtimeConnection, session *realtimeapi.Session) {
	// Create bridge for this session
	bridge := NewRealtimeAgentBridge(session, s.coordinator)
	
	s.mu.Lock()
	s.bridges[session.ID] = bridge
	s.mu.Unlock()
	
	// Start bridge
	if err := bridge.Start(); err != nil {
		log.Printf("[AgentServer] Failed to start bridge: %v", err)
		return
	}
	
	// Set up event handler for this connection
	handler := &agentEventHandler{
		bridge:  bridge,
		session: session,
	}
	conn.RegisterEventHandler(handler)
	
	log.Printf("[AgentServer] Agent bridge started for session %s", session.ID)
}

// agentEventHandler handles WebRTC events and forwards to agent system
type agentEventHandler struct {
	bridge  *RealtimeAgentBridge
	session *realtimeapi.Session
}

func (h *agentEventHandler) OnConnectionStateChange(state webrtc.PeerConnectionState) {
	// Handle connection state changes
}

func (h *agentEventHandler) OnAudioReceived(data []byte, sampleRate, channels int, timestamp time.Time) {
	// Forward to agent bridge
	h.bridge.HandleAudioInput(data, sampleRate, channels)
}

func (h *agentEventHandler) OnImageReceived(data []byte, mimeType string, width, height int, timestamp time.Time) {
	// Handle image input - could be sent to vision-capable agents
}

func (h *agentEventHandler) OnClientEvent(event events.ClientEvent) {
	// Handle Realtime API events
	switch e := event.(type) {
	case *events.ResponseCreateEvent:
		// This triggers agent processing
		h.handleResponseCreate(e)
	case *events.ResponseCancelEvent:
		// Handle cancellation
		h.handleResponseCancel(e)
	}
}

func (h *agentEventHandler) OnError(err error) {
	log.Printf("[AgentHandler] Error: %v", err)
}

func (h *agentEventHandler) handleResponseCreate(e *events.ResponseCreateEvent) {
	// The response creation is handled by the agent system
	// when audio/text input is received
	// This method can be used for special handling
}

func (h *agentEventHandler) handleResponseCancel(e *events.ResponseCancelEvent) {
	// Cancel current agent processing
	rm := h.session.GetResponseManager()
	if rm != nil {
		rm.Interrupt(e.Reason)
	}
}

// Helper function
func generateID() string {
	return "msg_" + time.Now().Format("20060102150405")
}

// Note: This file needs proper imports and may require adjustments
// based on the actual implementation of realtimeapi package
