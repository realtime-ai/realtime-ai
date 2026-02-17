This PR introduces a comprehensive Agent-based architecture for realtime-ai, moving beyond LLM-centric processing to a multi-agent system.

## Changes

### Core Agent System (`pkg/agent/`)
- `core.go`: Agent interface, Coordinator, Router, Message types
- BaseAgent for easy agent implementation
- SimpleRouter for basic request routing

### Basic Agents (`pkg/agents/`)
- EchoAgent: Simple echo functionality
- TimeAgent: Time and date queries
- WeatherAgent: Weather information (mock)

### Pipeline Integration (`pkg/elements/`)
- AgentElement: Integrates Agent system into Pipeline
- Automatic message format conversion

### Example (`examples/agent-pipeline/`)
- Complete usage example
- Interactive testing mode

### Documentation
- `docs/agent-architecture-design.md`: Architecture design
- `docs/AGENT_TODO.md`: Development roadmap and test fix plan

## Architecture

```
User Input → Coordinator → Router → Agent(s) → Response
                ↓
           Session, Memory, Tools
```

## Usage

```go
coordinator := agent.NewCoordinator(nil)
coordinator.Register(agents.NewEchoAgent())
coordinator.Register(agents.NewTimeAgent())

response, err := coordinator.Process(ctx, sessionID, message)
```

## Next Steps
- LLM-based Router for intelligent routing
- Tool system for external API integration
- Memory system for conversation history
- Multi-agent collaboration

## Testing
- [ ] Unit tests for Agent system
- [ ] Integration tests
- [ ] CI/CD workflow updates

## Files Changed
- `pkg/agent/core.go` - Core Agent system
- `pkg/agents/basic.go` - Basic Agent implementations
- `pkg/elements/agent_element.go` - Pipeline integration
- `examples/agent-pipeline/main.go` - Usage example
- `docs/agent-architecture-design.md` - Architecture documentation
- `docs/AGENT_TODO.md` - Development roadmap
