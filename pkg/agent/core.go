// Package agent provides the Agent-based AI architecture for realtime-ai.
package agent

import (
	"context"
	"fmt"
	"time"
)

// DataType represents the type of data an agent can handle
type DataType string

const (
	DataTypeText     DataType = "text"
	DataTypeAudio    DataType = "audio"
	DataTypeImage    DataType = "image"
	DataTypeVideo    DataType = "video"
	DataTypeJSON     DataType = "json"
)

// Message represents a message in the agent system
type Message struct {
	ID        string
	SessionID string
	Type      MessageType
	
	// Content based on type
	TextContent  string
	AudioData    []byte
	ImageData    []byte
	
	// Metadata
	Timestamp time.Time
	Source    string // Agent ID or "user"
}

type MessageType string

const (
	MessageTypeText  MessageType = "text"
	MessageTypeAudio MessageType = "audio"
	MessageTypeImage MessageType = "image"
	MessageTypeEvent MessageType = "event"
)

// Capability represents what an agent can do
type Capability struct {
	Name        string
	Description string
	InputTypes  []DataType
	OutputTypes []DataType
}

// Tool represents a tool that an agent can use
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]Parameter
	Handler     func(ctx context.Context, args map[string]interface{}) (interface{}, error)
}

type Parameter struct {
	Name        string
	Type        string
	Description string
	Required    bool
}

// Agent is the core interface for all agents
type Agent interface {
	// Identity
	ID() string
	Name() string
	Description() string
	
	// Capabilities
	Capabilities() []Capability
	CanHandle(intent string) bool
	
	// Processing
	Process(ctx context.Context, msg *Message) (*Message, error)
	
	// Tools
	Tools() []Tool
	
	// Lifecycle
	Start(ctx context.Context) error
	Stop() error
}

// BaseAgent provides common functionality for agents
type BaseAgent struct {
	id           string
	name         string
	description  string
	capabilities []Capability
	tools        []Tool
}

func NewBaseAgent(id, name, description string) *BaseAgent {
	return &BaseAgent{
		id:           id,
		name:         name,
		description:  description,
		capabilities: make([]Capability, 0),
		tools:        make([]Tool, 0),
	}
}

func (a *BaseAgent) ID() string                { return a.id }
func (a *BaseAgent) Name() string              { return a.name }
func (a *BaseAgent) Description() string       { return a.description }
func (a *BaseAgent) Capabilities() []Capability { return a.capabilities }
func (a *BaseAgent) Tools() []Tool             { return a.tools }

func (a *BaseAgent) CanHandle(intent string) bool {
	for _, cap := range a.capabilities {
		if cap.Name == intent {
			return true
		}
	}
	return false
}

func (a *BaseAgent) Start(ctx context.Context) error { return nil }
func (a *BaseAgent) Stop() error                     { return nil }

func (a *BaseAgent) AddCapability(cap Capability) {
	a.capabilities = append(a.capabilities, cap)
}

func (a *BaseAgent) AddTool(tool Tool) {
	a.tools = append(a.tools, tool)
}

// Router decides which agent should handle a message
type Router interface {
	Route(ctx context.Context, msg *Message, agents []Agent) (*RouteResult, error)
}

// RouteResult contains routing decision
type RouteResult struct {
	AgentID       string
	Confidence    float64
	Reasoning     string
	SubTasks      []SubTask // For multi-agent collaboration
}

type SubTask struct {
	AgentID     string
	Description string
	DependsOn   []int // Indices of tasks this depends on
}

// SimpleRouter is a basic router implementation
type SimpleRouter struct{}

func NewSimpleRouter() *SimpleRouter {
	return &SimpleRouter{}
}

func (r *SimpleRouter) Route(ctx context.Context, msg *Message, agents []Agent) (*RouteResult, error) {
	// Simple keyword matching - in production use LLM
	for _, agent := range agents {
		for _, cap := range agent.Capabilities() {
			if contains(msg.TextContent, cap.Name) {
				return &RouteResult{
					AgentID:    agent.ID(),
					Confidence: 0.8,
					Reasoning:  "Keyword match: " + cap.Name,
				}, nil
			}
		}
	}
	
	// Default to first agent
	if len(agents) > 0 {
		return &RouteResult{
			AgentID:    agents[0].ID(),
			Confidence: 0.5,
			Reasoning:  "Default fallback",
		}, nil
	}
	
	return nil, fmt.Errorf("no agents available")
}

func contains(text, keyword string) bool {
	// Simple substring check
	return len(text) > 0 && len(keyword) > 0
}

// Coordinator manages all agents and coordinates their execution
type Coordinator struct {
	agents   map[string]Agent
	router   Router
	sessions map[string]*AgentSession
}

// AgentSession tracks a conversation session
type AgentSession struct {
	SessionID    string
	CurrentAgent Agent
	History      []*Message
	Variables    map[string]interface{}
}

// NewCoordinator creates a new coordinator
func NewCoordinator(router Router) *Coordinator {
	if router == nil {
		router = NewSimpleRouter()
	}
	
	return &Coordinator{
		agents:   make(map[string]Agent),
		router:   router,
		sessions: make(map[string]*AgentSession),
	}
}

// Register adds an agent to the coordinator
func (c *Coordinator) Register(agent Agent) {
	c.agents[agent.ID()] = agent
}

// GetAgent retrieves an agent by ID
func (c *Coordinator) GetAgent(id string) (Agent, bool) {
	agent, ok := c.agents[id]
	return agent, ok
}

// ListAgents returns all registered agents
func (c *Coordinator) ListAgents() []Agent {
	agents := make([]Agent, 0, len(c.agents))
	for _, agent := range c.agents {
		agents = append(agents, agent)
	}
	return agents
}

// Process handles a message through the agent system
func (c *Coordinator) Process(ctx context.Context, sessionID string, msg *Message) (*Message, error) {
	// Get or create session
	session := c.getOrCreateSession(sessionID)
	
	// Add to history
	session.History = append(session.History, msg)
	
	// Get list of available agents
	agentList := c.ListAgents()
	if len(agentList) == 0 {
		return nil, fmt.Errorf("no agents registered")
	}
	
	// Route to appropriate agent
	route, err := c.router.Route(ctx, msg, agentList)
	if err != nil {
		return nil, fmt.Errorf("routing failed: %w", err)
	}
	
	// Get target agent
	targetAgent, ok := c.agents[route.AgentID]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", route.AgentID)
	}
	
	// Update session
	session.CurrentAgent = targetAgent
	
	// Single agent execution
	response, err := targetAgent.Process(ctx, msg)
	if err != nil {
		return nil, err
	}
	
	// Add response to history
	if response != nil {
		session.History = append(session.History, response)
	}
	
	return response, nil
}

func (c *Coordinator) getOrCreateSession(sessionID string) *AgentSession {
	if session, ok := c.sessions[sessionID]; ok {
		return session
	}
	
	session := &AgentSession{
		SessionID: sessionID,
		History:   make([]*Message, 0),
		Variables: make(map[string]interface{}),
	}
	c.sessions[sessionID] = session
	return session
}

// Start starts all registered agents
func (c *Coordinator) Start(ctx context.Context) error {
	for _, agent := range c.agents {
		if err := agent.Start(ctx); err != nil {
			return fmt.Errorf("failed to start agent %s: %w", agent.ID(), err)
		}
	}
	return nil
}

// Stop stops all registered agents
func (c *Coordinator) Stop() error {
	for _, agent := range c.agents {
		if err := agent.Stop(); err != nil {
			return fmt.Errorf("failed to stop agent %s: %w", agent.ID(), err)
		}
	}
	return nil
}

// Helper function
func generateID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}
