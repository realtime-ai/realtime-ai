// Package agents provides concrete agent implementations.
package agents

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/agent"
)

// EchoAgent is a simple agent that echoes back the input
type EchoAgent struct {
	*agent.BaseAgent
}

// NewEchoAgent creates a new echo agent
func NewEchoAgent() *EchoAgent {
	base := agent.NewBaseAgent(
		"echo",
		"Echo Agent",
		"Echoes back what you say",
	)
	
	base.AddCapability(agent.Capability{
		Name:        "echo",
		Description: "Echo back user input",
		InputTypes:  []agent.DataType{agent.DataTypeText},
		OutputTypes: []agent.DataType{agent.DataTypeText},
	})
	
	return &EchoAgent{BaseAgent: base}
}

func (a *EchoAgent) Process(ctx context.Context, msg *agent.Message) (*agent.Message, error) {
	var response string
	
	switch msg.Type {
	case agent.MessageTypeText:
		response = fmt.Sprintf("Echo: %s", msg.TextContent)
	case agent.MessageTypeAudio:
		response = "Echo: [Audio received]"
	default:
		response = "Echo: [Unknown message type]"
	}
	
	return &agent.Message{
		ID:          generateMessageID(),
		SessionID:   msg.SessionID,
		Type:        agent.MessageTypeText,
		TextContent: response,
		Timestamp:   time.Now(),
		Source:      a.ID(),
	}, nil
}

// TimeAgent provides time-related information
type TimeAgent struct {
	*agent.BaseAgent
}

// NewTimeAgent creates a new time agent
func NewTimeAgent() *TimeAgent {
	base := agent.NewBaseAgent(
		"time",
		"Time Agent",
		"Provides current time and date information",
	)
	
	base.AddCapability(agent.Capability{
		Name:        "current_time",
		Description: "Get current time",
		InputTypes:  []agent.DataType{agent.DataTypeText},
		OutputTypes: []agent.DataType{agent.DataTypeText},
	})
	
	return &TimeAgent{BaseAgent: base}
}

func (a *TimeAgent) Process(ctx context.Context, msg *agent.Message) (*agent.Message, error) {
	now := time.Now()
	
	var response string
	text := strings.ToLower(msg.TextContent)
	
	if strings.Contains(text, "date") {
		response = fmt.Sprintf("Today's date is %s.", now.Format("Monday, January 2, 2006"))
	} else if strings.Contains(text, "time") {
		response = fmt.Sprintf("Current time is %s.", now.Format("3:04 PM"))
	} else {
		response = fmt.Sprintf("It is now %s on %s.", 
			now.Format("3:04 PM"), 
			now.Format("Monday, January 2, 2006"))
	}
	
	return &agent.Message{
		ID:          generateMessageID(),
		SessionID:   msg.SessionID,
		Type:        agent.MessageTypeText,
		TextContent: response,
		Timestamp:   time.Now(),
		Source:      a.ID(),
	}, nil
}

// WeatherAgent provides weather information (mock implementation)
type WeatherAgent struct {
	*agent.BaseAgent
}

// NewWeatherAgent creates a new weather agent
func NewWeatherAgent() *WeatherAgent {
	base := agent.NewBaseAgent(
		"weather",
		"Weather Agent",
		"Provides weather information",
	)
	
	base.AddCapability(agent.Capability{
		Name:        "weather_query",
		Description: "Get weather information for a location",
		InputTypes:  []agent.DataType{agent.DataTypeText},
		OutputTypes: []agent.DataType{agent.DataTypeText},
	})
	
	return &WeatherAgent{BaseAgent: base}
}

func (a *WeatherAgent) Process(ctx context.Context, msg *agent.Message) (*agent.Message, error) {
	text := strings.ToLower(msg.TextContent)
	
	// Simple city extraction
	city := extractCity(text)
	if city == "" {
		city = "your location"
	}
	
	// Mock weather data
	conditions := []string{"sunny", "cloudy", "rainy", "partly cloudy"}
	condition := conditions[time.Now().Second()%len(conditions)]
	temp := 15 + (time.Now().Second() % 15)
	
	var response string
	if strings.Contains(text, "tomorrow") {
		response = fmt.Sprintf("Tomorrow in %s: expected to be %s with a high of %d°C.", 
			city, condition, temp+2)
	} else {
		response = fmt.Sprintf("Current weather in %s: %s, %d°C.", 
			city, condition, temp)
	}
	
	return &agent.Message{
		ID:          generateMessageID(),
		SessionID:   msg.SessionID,
		Type:        agent.MessageTypeText,
		TextContent: response,
		Timestamp:   time.Now(),
		Source:      a.ID(),
	}, nil
}

func extractCity(text string) string {
	// Simple extraction - look for common patterns
	patterns := []string{"in ", "at ", "for "}
	for _, pattern := range patterns {
		if idx := strings.Index(text, pattern); idx != -1 {
			end := idx + len(pattern)
			// Find end of city name (next space or end)
			if end < len(text) {
				rest := text[end:]
				if spaceIdx := strings.Index(rest, " "); spaceIdx != -1 {
					return rest[:spaceIdx]
				}
				return rest
			}
		}
	}
	return ""
}

// Helper function
func generateMessageID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}
