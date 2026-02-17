// Package agent provides the Agent-based AI architecture for realtime-ai.
//
// This package implements a multi-agent system where LLM is just one capability
// among many. Agents are autonomous entities that can process messages, use tools,
// and collaborate with other agents.
//
// Basic usage:
//
//	coordinator := agent.NewCoordinator()
//	
//	// Register agents
//	coordinator.Register(agent.NewWeatherAgent())
//	coordinator.Register(agent.NewCalendarAgent())
//	
//	// Process message
//	response, err := coordinator.Process(ctx, sessionID, message)
//
// Architecture:
//
//	User Input → Router → Coordinator → Agent(s) → Response
//	                      ↓
//	                Memory, Tools, LLM
package agent

import (
	"context"
	"encoding/json"
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
	DataTypeLocation DataType = "location"
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
	JSONData     json.RawMessage
	
	// Metadata
	Timestamp   time.Time
	Source      string  // Agent ID or "user"
	Intent      Intent
	Entities    []Entity
	
	// Context
	Context     *Context
}

type MessageType string

const (
	MessageTypeText  MessageType = "text"
	MessageTypeAudio MessageType = "audio"
	MessageTypeImage MessageType = "image"
	MessageTypeEvent MessageType = "event"
	MessageTypeTool  MessageType = "tool"
)

// Intent represents the user's intent
type Intent struct {
	Name        string
	Confidence  float64
	Parameters  map[string]interface{}
}

// Entity represents an extracted entity
type Entity struct {
	Type  string
	Value string
	Start int
	End   int
}

// Context holds conversation context
type Context struct {
	SessionID       string
	UserID          string
	History         []*Message
	CurrentAgent    string
	Variables       map[string]interface{}
	Preferences     map[string]interface{}
}

// Capability represents what an agent can do
type Capability struct {
	Name        string
	Description string
	InputTypes  []DataType
	OutputTypes []DataType
	Examples    []string  // Example queries this agent handles
}

// Tool represents a tool that an agent can use
type Tool struct {
	Name        string
	Description string
	Parameters  []Parameter
	Handler     ToolHandler
}

type Parameter struct {
	Name        string
	Type        string
	Description string
	Required    bool
	Enum        []string
}

type ToolHandler func(ctx context.Context, args map[string]interface{}) (interface{}, error)

// Agent is the core interface for all agents
type Agent interface {
	// Identity
	ID() string
	Name() string
	Description() string
	
	// Capabilities
	Capabilities() []Capability
	CanHandle(intent Intent) bool
	
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
	id          string
	name        string
	description string
	capabilities []Capability
	tools       []Tool
}

func (a *BaseAgent) ID() string { return a.id }
func (a *BaseAgent) Name() string { return a.name }
func (a *BaseAgent) Description() string { return a.description }
func (a *BaseAgent) Capabilities() []Capability { return a.capabilities }
func (a *BaseAgent) Tools() []Tool { return a.tools }

func (a *BaseAgent) CanHandle(intent Intent) bool {
	for _, cap := range a.capabilities {
		if cap.Name == intent.Name {
			return true
		}
	}
	return false
}

func (a *BaseAgent) Start(ctx context.Context) error { return nil }
func (a *BaseAgent) Stop() error { return nil }

// Router decides which agent should handle a message
type Router interface {
	Route(ctx context.Context, msg *Message, agents []Agent) (*RouteResult, error)
}

// RouteResult contains routing decision
type RouteResult struct {
	AgentID       string
	Confidence    float64
	Reasoning     string
	SubTasks      []SubTask  // For multi-agent collaboration
}

type SubTask struct {
	AgentID     string
	Description string
	Input       *Message
	DependsOn   []int  // Indices of tasks this depends on
}

// LLMRouter uses LLM to make routing decisions
type LLMRouter struct {
	llm LLMClient
}

func NewLLMRouter(llm LLMClient) *LLMRouter {
	return &LLMRouter{llm: llm}
}

func (r *LLMRouter) Route(ctx context.Context, msg *Message, agents []Agent) (*RouteResult, error) {
	// Build routing prompt
	prompt := r.buildRoutingPrompt(msg, agents)
	
	// Call LLM
	response, err := r.llm.Complete(ctx, prompt)
	if err != nil {
		return nil, err
	}
	
	// Parse response
	return r.parseRoutingResponse(response)
}

func (r *LLMRouter) buildRoutingPrompt(msg *Message, agents []Agent) string {
	prompt := fmt.Sprintf(`You are a routing system. Decide which agent should handle this user message.

User Message: %s

Available Agents:
`, msg.TextContent)

	for _, agent := range agents {
		prompt += fmt.Sprintf("\n- %s: %s\n", agent.Name(), agent.Description())
		for _, cap := range agent.Capabilities() {
			prompt += fmt.Sprintf("  - %s: %s\n", cap.Name, cap.Description)
		}
	}

	prompt += `
Respond in JSON format:
{
  "agent_id": "agent name",
  "confidence": 0.95,
  "reasoning": "why this agent",
  "sub_tasks": [] // if multiple agents needed
}
`
	return prompt
}

func (r *LLMRouter) parseRoutingResponse(response string) (*RouteResult, error) {
	var result RouteResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Memory provides long-term memory for agents
type Memory interface {
	// Store message in session history
	Store(ctx context.Context, sessionID string, msg *Message) error
	
	// Retrieve relevant messages
	Retrieve(ctx context.Context, sessionID string, query string, limit int) ([]*MemoryItem, error)
	
	// Store user preference
	SetPreference(ctx context.Context, userID, key string, value interface{}) error
	
	// Get user preference
	GetPreference(ctx context.Context, userID, key string) (interface{}, error)
}

// MemoryItem represents a stored memory
type MemoryItem struct {
	ID        string
	Timestamp time.Time
	Content   string
	Type      string
	Relevance float64
}

// ToolRegistry manages available tools
type ToolRegistry interface {
	Register(tool Tool) error
	Get(name string) (Tool, bool)
	Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error)
	List() []Tool
}

// Coordinator manages all agents and coordinates their execution
type Coordinator struct {
	agents    map[string]Agent
	router    Router
	memory    Memory
	tools     ToolRegistry
	llm       LLMClient
	
	// Sessions
	sessions  map[string]*AgentSession
}

// AgentSession tracks a conversation session
type AgentSession struct {
	SessionID    string
	UserID       string
	CurrentAgent Agent
	AgentStack   []Agent  // For nested agent calls
	Variables    map[string]interface{}
}

// NewCoordinator creates a new coordinator
func NewCoordinator(router Router, memory Memory, tools ToolRegistry, llm LLMClient) *Coordinator {
	return &Coordinator{
		agents:   make(map[string]Agent),
		router:   router,
		memory:   memory,
		tools:    tools,
		llm:      llm,
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

// Process handles a message through the agent system
func (c *Coordinator) Process(ctx context.Context, sessionID string, msg *Message) (*Message, error) {
	// Get or create session
	session := c.getOrCreateSession(sessionID, msg)
	
	// Update message context
	msg.Context = c.buildContext(session)
	
	// Route to appropriate agent
	agentList := c.getAgentList()
	route, err := c.router.Route(ctx, msg, agentList)
	if err != nil {
		return nil, fmt.Errorf("routing failed: %w", err)
	}
	
	// Get target agent
	agent, ok := c.agents[route.AgentID]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", route.AgentID)
	}
	
	// Update session
	session.CurrentAgent = agent
	
	// Handle multi-agent collaboration
	if len(route.SubTasks) > 0 {
		return c.executeCollaboration(ctx, session, msg, route.SubTasks)
	}
	
	// Single agent execution
	response, err := agent.Process(ctx, msg)
	if err != nil {
		return nil, err
	}
	
	// Store in memory
	if c.memory != nil {
		c.memory.Store(ctx, sessionID, msg)
		c.memory.Store(ctx, sessionID, response)
	}
	
	return response, nil
}

func (c *Coordinator) getOrCreateSession(sessionID string, msg *Message) *AgentSession {
	if session, ok := c.sessions[sessionID]; ok {
		return session
	}
	
	session := &AgentSession{
		SessionID:  sessionID,
		UserID:     msg.Context.UserID,
		Variables:  make(map[string]interface{}),
		AgentStack: make([]Agent, 0),
	}
	c.sessions[sessionID] = session
	return session
}

func (c *Coordinator) buildContext(session *AgentSession) *Context {
	var history []*Message
	if c.memory != nil {
		history, _ = c.memory.Retrieve(context.Background(), session.SessionID, "", 10)
	}
	
	return &Context{
		SessionID:    session.SessionID,
		UserID:       session.UserID,
		History:      history,
		CurrentAgent: session.CurrentAgent.ID(),
		Variables:    session.Variables,
	}
}

func (c *Coordinator) getAgentList() []Agent {
	agents := make([]Agent, 0, len(c.agents))
	for _, agent := range c.agents {
		agents = append(agents, agent)
	}
	return agents
}

// executeCollaboration handles multi-agent collaboration
func (c *Coordinator) executeCollaboration(ctx context.Context, session *AgentSession, msg *Message, subTasks []SubTask) (*Message, error) {
	results := make(map[int]*Message)
	
	for i, task := range subTasks {
		agent, ok := c.agents[task.AgentID]
		if !ok {
			return nil, fmt.Errorf("agent not found: %s", task.AgentID)
		}
		
		// Wait for dependencies
		for _, depIdx := range task.DependsOn {
			if _, ok := results[depIdx]; !ok {
				return nil, fmt.Errorf("dependency not satisfied: %d", depIdx)
			}
		}
		
		// Execute task
		result, err := agent.Process(ctx, task.Input)
		if err != nil {
			return nil, err
		}
		
		results[i] = result
	}
	
	// Combine results (last task's result is the final response)
	return results[len(subTasks)-1], nil
}

// LLMClient interface for LLM interactions
type LLMClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
	CompleteStream(ctx context.Context, prompt string) (<-chan string, error)
	Chat(ctx context.Context, messages []ChatMessage) (string, error)
}

type ChatMessage struct {
	Role    string
	Content string
}

// AgentBuilder helps build agents
type AgentBuilder struct {
	agent *BaseAgent
}

func NewAgentBuilder(id, name, description string) *AgentBuilder {
	return &AgentBuilder{
		agent: &BaseAgent{
			id:          id,
			name:        name,
			description: description,
			capabilities: make([]Capability, 0),
			tools:       make([]Tool, 0),
		},
	}
}

func (b *AgentBuilder) WithCapability(cap Capability) *AgentBuilder {
	b.agent.capabilities = append(b.agent.capabilities, cap)
	return b
}

func (b *AgentBuilder) WithTool(tool Tool) *AgentBuilder {
	b.agent.tools = append(b.agent.tools, tool)
	return b
}

func (b *AgentBuilder) Build() *BaseAgent {
	return b.agent
}
