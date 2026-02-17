// Package agents provides concrete agent implementations.
package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/agent"
)

// WeatherAgent handles weather-related queries
type WeatherAgent struct {
	*agent.BaseAgent
	weatherService WeatherService
	llm            agent.LLMClient
}

// WeatherService interface for weather data
type WeatherService interface {
	GetCurrentWeather(ctx context.Context, city string) (*WeatherData, error)
	GetForecast(ctx context.Context, city string, days int) ([]*WeatherData, error)
}

// WeatherData represents weather information
type WeatherData struct {
	City        string
	Temperature float64
	Condition   string
	Humidity    int
	WindSpeed   float64
	Date        time.Time
}

// NewWeatherAgent creates a new weather agent
func NewWeatherAgent(weatherService WeatherService, llm agent.LLMClient) *WeatherAgent {
	base := agent.NewAgentBuilder(
		"weather",
		"Weather Agent",
		"Handles weather queries and forecasts",
	).
	WithCapability(agent.Capability{
		Name:        "current_weather",
		Description: "Get current weather for a city",
		InputTypes:  []agent.DataType{agent.DataTypeText, agent.DataTypeLocation},
		OutputTypes: []agent.DataType{agent.DataTypeText, agent.DataTypeAudio},
		Examples: []string{
			"What's the weather like in Beijing?",
			"北京今天天气怎么样？",
			"Is it raining in New York?",
		},
	}).
	WithCapability(agent.Capability{
		Name:        "weather_forecast",
		Description: "Get weather forecast for upcoming days",
		InputTypes:  []agent.DataType{agent.DataTypeText},
		OutputTypes: []agent.DataType{agent.DataTypeText},
		Examples: []string{
			"What's the forecast for tomorrow?",
			"Will it rain this weekend?",
		},
	}).
	WithTool(agent.Tool{
		Name:        "get_weather",
		Description: "Get current weather data for a city",
		Parameters: []agent.Parameter{
			{Name: "city", Type: "string", Description: "City name", Required: true},
			{Name: "date", Type: "string", Description: "Date (today, tomorrow, or YYYY-MM-DD)", Required: false},
		},
		Handler: nil, // Will be set in constructor
	}).
	Build()

	a := &WeatherAgent{
		BaseAgent:      base,
		weatherService: weatherService,
		llm:            llm,
	}

	// Set tool handler
	a.tools[0].Handler = a.handleGetWeather

	return a
}

func (a *WeatherAgent) Process(ctx context.Context, msg *agent.Message) (*agent.Message, error) {
	// Extract city and intent from message
	city, intent := a.extractIntent(msg)

	switch intent {
	case "current_weather":
		return a.handleCurrentWeather(ctx, city, msg)
	case "weather_forecast":
		return a.handleForecast(ctx, city, msg)
	default:
		return a.handleGeneralWeather(ctx, city, msg)
	}
}

func (a *WeatherAgent) extractIntent(msg *agent.Message) (string, string) {
	// Use LLM to extract city and intent
	prompt := fmt.Sprintf(`Extract the city and intent from this weather query.
Query: %s

Respond in JSON:
{
  "city": "city name",
  "intent": "current_weather" | "weather_forecast" | "general"
}`, msg.TextContent)

	response, err := a.llm.Complete(ctx, prompt)
	if err != nil {
		return "", "general"
	}

	var result struct {
		City   string `json:"city"`
		Intent string `json:"intent"`
	}
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return "", "general"
	}

	return result.City, result.Intent
}

func (a *WeatherAgent) handleCurrentWeather(ctx context.Context, city string, msg *agent.Message) (*agent.Message, error) {
	// Get weather data
	weather, err := a.weatherService.GetCurrentWeather(ctx, city)
	if err != nil {
		return nil, err
	}

	// Generate natural language response
	response := fmt.Sprintf("The weather in %s is currently %s with a temperature of %.1f°C. "+
		"Humidity is at %d%% and wind speed is %.1f m/s.",
		weather.City, weather.Condition, weather.Temperature,
		weather.Humidity, weather.WindSpeed)

	return &agent.Message{
		ID:          generateID(),
		SessionID:   msg.SessionID,
		Type:        agent.MessageTypeText,
		TextContent: response,
		Timestamp:   time.Now(),
		Source:      a.ID(),
	}, nil
}

func (a *WeatherAgent) handleForecast(ctx context.Context, city string, msg *agent.Message) (*agent.Message, error) {
	// Get 3-day forecast
	forecast, err := a.weatherService.GetForecast(ctx, city, 3)
	if err != nil {
		return nil, err
	}

	// Generate response
	var response string
	for i, day := range forecast {
		if i == 0 {
			response += fmt.Sprintf("Tomorrow in %s: %s, %.1f°C. ",
				day.City, day.Condition, day.Temperature)
		} else {
			response += fmt.Sprintf("Day %d: %s, %.1f°C. ",
				i+1, day.Condition, day.Temperature)
		}
	}

	return &agent.Message{
		ID:          generateID(),
		SessionID:   msg.SessionID,
		Type:        agent.MessageTypeText,
		TextContent: response,
		Timestamp:   time.Now(),
		Source:      a.ID(),
	}, nil
}

func (a *WeatherAgent) handleGeneralWeather(ctx context.Context, city string, msg *agent.Message) (*agent.Message, error) {
	// Default to current weather
	return a.handleCurrentWeather(ctx, city, msg)
}

func (a *WeatherAgent) handleGetWeather(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	city, ok := args["city"].(string)
	if !ok {
		return nil, fmt.Errorf("city parameter required")
	}

	return a.weatherService.GetCurrentWeather(ctx, city)
}

// CalendarAgent handles calendar and scheduling tasks
type CalendarAgent struct {
	*agent.BaseAgent
	calendarService CalendarService
	llm             agent.LLMClient
}

type CalendarService interface {
	CreateEvent(ctx context.Context, event *CalendarEvent) error
	GetEvents(ctx context.Context, start, end time.Time) ([]*CalendarEvent, error)
	DeleteEvent(ctx context.Context, eventID string) error
}

type CalendarEvent struct {
	ID          string
	Title       string
	Description string
	StartTime   time.Time
	EndTime     time.Time
	Location    string
}

// NewCalendarAgent creates a new calendar agent
func NewCalendarAgent(calendarService CalendarService, llm agent.LLMClient) *CalendarAgent {
	base := agent.NewAgentBuilder(
		"calendar",
		"Calendar Agent",
		"Manages calendar events and reminders",
	).
	WithCapability(agent.Capability{
		Name:        "create_event",
		Description: "Create a calendar event or reminder",
		InputTypes:  []agent.DataType{agent.DataTypeText},
		OutputTypes: []agent.DataType{agent.DataTypeText},
		Examples: []string{
			"Remind me to take an umbrella tomorrow at 8am",
			"Schedule a meeting for 3pm tomorrow",
			"Add a reminder to call mom",
		},
	}).
	WithCapability(agent.Capability{
		Name:        "query_events",
		Description: "Query calendar events",
		InputTypes:  []agent.DataType{agent.DataTypeText},
		OutputTypes: []agent.DataType{agent.DataTypeText},
		Examples: []string{
			"What's on my calendar today?",
			"Do I have any meetings tomorrow?",
		},
	}).
	WithTool(agent.Tool{
		Name:        "create_reminder",
		Description: "Create a reminder",
		Parameters: []agent.Parameter{
			{Name: "title", Type: "string", Required: true},
			{Name: "datetime", Type: "string", Required: true},
			{Name: "description", Type: "string", Required: false},
		},
	}).
	Build()

	return &CalendarAgent{
		BaseAgent:       base,
		calendarService: calendarService,
		llm:             llm,
	}
}

func (a *CalendarAgent) Process(ctx context.Context, msg *agent.Message) (*agent.Message, error) {
	// Parse intent
	intent, params := a.parseIntent(msg)

	switch intent {
	case "create_event":
		return a.handleCreateEvent(ctx, params, msg)
	case "query_events":
		return a.handleQueryEvents(ctx, params, msg)
	default:
		return &agent.Message{
			ID:          generateID(),
			SessionID:   msg.SessionID,
			Type:        agent.MessageTypeText,
			TextContent: "I can help you create calendar events or check your schedule. What would you like to do?",
			Timestamp:   time.Now(),
			Source:      a.ID(),
		}, nil
	}
}

func (a *CalendarAgent) parseIntent(msg *agent.Message) (string, map[string]interface{}) {
	prompt := fmt.Sprintf(`Parse this calendar request into intent and parameters.
Request: %s

Respond in JSON:
{
  "intent": "create_event" | "query_events" | "unknown",
  "parameters": {
    "title": "event title",
    "datetime": "ISO datetime or relative time",
    "description": "additional details"
  }
}`, msg.TextContent)

	response, _ := a.llm.Complete(ctx, prompt)

	var result struct {
		Intent     string                 `json:"intent"`
		Parameters map[string]interface{} `json:"parameters"`
	}
	json.Unmarshal([]byte(response), &result)

	return result.Intent, result.Parameters
}

func (a *CalendarAgent) handleCreateEvent(ctx context.Context, params map[string]interface{}, msg *agent.Message) (*agent.Message, error) {
	title, _ := params["title"].(string)
	datetimeStr, _ := params["datetime"].(string)
	description, _ := params["description"].(string)

	// Parse datetime
	datetime := parseRelativeTime(datetimeStr)

	// Create event
	event := &CalendarEvent{
		ID:          generateID(),
		Title:       title,
		Description: description,
		StartTime:   datetime,
		EndTime:     datetime.Add(30 * time.Minute),
	}

	if err := a.calendarService.CreateEvent(ctx, event); err != nil {
		return nil, err
	}

	response := fmt.Sprintf("I've added '%s' to your calendar for %s.",
		title, datetime.Format("Monday, January 2 at 3:04 PM"))

	return &agent.Message{
		ID:          generateID(),
		SessionID:   msg.SessionID,
		Type:        agent.MessageTypeText,
		TextContent: response,
		Timestamp:   time.Now(),
		Source:      a.ID(),
	}, nil
}

func (a *CalendarAgent) handleQueryEvents(ctx context.Context, params map[string]interface{}, msg *agent.Message) (*agent.Message, error) {
	// Query today's events
	start := time.Now().Truncate(24 * time.Hour)
	end := start.Add(24 * time.Hour)

	events, err := a.calendarService.GetEvents(ctx, start, end)
	if err != nil {
		return nil, err
	}

	var response string
	if len(events) == 0 {
		response = "You have no events scheduled for today."
	} else {
		response = "Here are your events for today:\n"
		for _, event := range events {
			response += fmt.Sprintf("- %s at %s\n",
				event.Title, event.StartTime.Format("3:04 PM"))
		}
	}

	return &agent.Message{
		ID:          generateID(),
		SessionID:   msg.SessionID,
		Type:        agent.MessageTypeText,
		TextContent: response,
		Timestamp:   time.Now(),
		Source:      a.ID(),
	}, nil
}

// RouterAgent is a special agent that routes to other agents
// It can be used as the entry point for all requests
type RouterAgent struct {
	*agent.BaseAgent
	coordinator *agent.Coordinator
	llm         agent.LLMClient
}

// NewRouterAgent creates a router agent
func NewRouterAgent(coordinator *agent.Coordinator, llm agent.LLMClient) *RouterAgent {
	base := agent.NewAgentBuilder(
		"router",
		"Router Agent",
		"Routes requests to appropriate specialized agents",
	).
	WithCapability(agent.Capability{
		Name:        "route_request",
		Description: "Route user request to the best agent",
		InputTypes:  []agent.DataType{agent.DataTypeText, agent.DataTypeAudio},
		OutputTypes: []agent.DataType{agent.DataTypeText},
	}).
	Build()

	return &RouterAgent{
		BaseAgent:   base,
		coordinator: coordinator,
		llm:         llm,
	}
}

func (a *RouterAgent) Process(ctx context.Context, msg *agent.Message) (*agent.Message, error) {
	// Get list of available agents
	agents := a.getAvailableAgents()

	// Build routing prompt
	prompt := a.buildRoutingPrompt(msg, agents)

	// Get routing decision from LLM
	response, err := a.llm.Complete(ctx, prompt)
	if err != nil {
		return nil, err
	}

	// Parse decision
	var decision struct {
		AgentID   string `json:"agent_id"`
		Reasoning string `json:"reasoning"`
	}
	if err := json.Unmarshal([]byte(response), &decision); err != nil {
		return nil, err
	}

	// Route to selected agent
	targetAgent, ok := a.coordinator.GetAgent(decision.AgentID)
	if !ok {
		return nil, fmt.Errorf("selected agent not found: %s", decision.AgentID)
	}

	// Process with target agent
	return targetAgent.Process(ctx, msg)
}

func (a *RouterAgent) getAvailableAgents() []agent.Agent {
	// Get all registered agents from coordinator
	// This is a simplified version
	return []agent.Agent{}
}

func (a *RouterAgent) buildRoutingPrompt(msg *agent.Message, agents []agent.Agent) string {
	prompt := "You are a routing system. Select the best agent for this request.\n\n"
	prompt += fmt.Sprintf("User Request: %s\n\n", msg.TextContent)
	prompt += "Available Agents:\n"

	for _, ag := range agents {
		prompt += fmt.Sprintf("- %s: %s\n", ag.Name(), ag.Description())
		for _, cap := range ag.Capabilities() {
			prompt += fmt.Sprintf("  - %s: %s\n", cap.Name, cap.Description)
		}
	}

	prompt += "\nRespond in JSON:\n"
	prompt += `{"agent_id": "agent_name", "reasoning": "why this agent"}`

	return prompt
}

// Helper functions
func generateID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}

func parseRelativeTime(input string) time.Time {
	// Simplified parsing - in production use proper NLP
	now := time.Now()

	switch input {
	case "tomorrow", "明天":
		return now.Add(24 * time.Hour)
	case "today", "今天":
		return now
	default:
		// Try to parse as ISO format
		t, _ := time.Parse(time.RFC3339, input)
		if t.IsZero() {
			return now.Add(1 * time.Hour) // Default to 1 hour from now
		}
		return t
	}
}

var ctx = context.Background()
