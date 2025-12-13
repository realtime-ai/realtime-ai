// Package events defines the event types for the Realtime API protocol.
package events

// Modality represents the supported modalities for the session.
type Modality string

const (
	ModalityText  Modality = "text"
	ModalityAudio Modality = "audio"
)

// AudioFormat represents the supported audio formats.
type AudioFormat string

const (
	AudioFormatPCM16    AudioFormat = "pcm16"
	AudioFormatG711ULaw AudioFormat = "g711_ulaw"
	AudioFormatG711ALaw AudioFormat = "g711_alaw"
)

// ItemType represents the type of conversation item.
type ItemType string

const (
	ItemTypeMessage            ItemType = "message"
	ItemTypeFunctionCall       ItemType = "function_call"
	ItemTypeFunctionCallOutput ItemType = "function_call_output"
)

// ItemStatus represents the status of a conversation item.
type ItemStatus string

const (
	ItemStatusInProgress ItemStatus = "in_progress"
	ItemStatusCompleted  ItemStatus = "completed"
	ItemStatusIncomplete ItemStatus = "incomplete"
)

// Role represents the role of a conversation participant.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
)

// ContentType represents the type of content in a conversation item.
type ContentType string

const (
	ContentTypeInputText  ContentType = "input_text"
	ContentTypeInputAudio ContentType = "input_audio"
	ContentTypeText       ContentType = "text"
	ContentTypeAudio      ContentType = "audio"
)

// ResponseStatus represents the status of a response.
type ResponseStatus string

const (
	ResponseStatusInProgress ResponseStatus = "in_progress"
	ResponseStatusCompleted  ResponseStatus = "completed"
	ResponseStatusCancelled  ResponseStatus = "cancelled"
	ResponseStatusFailed     ResponseStatus = "failed"
)

// TurnDetectionType represents the type of turn detection.
type TurnDetectionType string

const (
	TurnDetectionTypeServerVAD TurnDetectionType = "server_vad"
	TurnDetectionTypeNone      TurnDetectionType = "none"
)

// ErrorType represents the type of error.
type ErrorType string

const (
	ErrorTypeInvalidRequest   ErrorType = "invalid_request_error"
	ErrorTypeAuthentication   ErrorType = "authentication_error"
	ErrorTypeRateLimit        ErrorType = "rate_limit_error"
	ErrorTypeServer           ErrorType = "server_error"
	ErrorTypeSession          ErrorType = "session_error"
)

// Session represents a realtime session configuration.
type Session struct {
	ID                      string                `json:"id"`
	Object                  string                `json:"object"` // "realtime.session"
	Model                   string                `json:"model"`
	Modalities              []Modality            `json:"modalities"`
	Voice                   string                `json:"voice,omitempty"`
	Instructions            string                `json:"instructions,omitempty"`
	InputAudioFormat        AudioFormat           `json:"input_audio_format"`
	OutputAudioFormat       AudioFormat           `json:"output_audio_format"`
	InputAudioTranscription *TranscriptionConfig  `json:"input_audio_transcription,omitempty"`
	TurnDetection           *TurnDetection        `json:"turn_detection,omitempty"`
	Tools                   []Tool                `json:"tools,omitempty"`
	ToolChoice              string                `json:"tool_choice,omitempty"`
	Temperature             float64               `json:"temperature,omitempty"`
	MaxOutputTokens         int                   `json:"max_output_tokens,omitempty"`
}

// SessionConfig is used in session.update events.
type SessionConfig struct {
	Modalities              []Modality            `json:"modalities,omitempty"`
	Model                   string                `json:"model,omitempty"`
	Voice                   string                `json:"voice,omitempty"`
	Instructions            string                `json:"instructions,omitempty"`
	InputAudioFormat        AudioFormat           `json:"input_audio_format,omitempty"`
	OutputAudioFormat       AudioFormat           `json:"output_audio_format,omitempty"`
	InputAudioTranscription *TranscriptionConfig  `json:"input_audio_transcription,omitempty"`
	TurnDetection           *TurnDetection        `json:"turn_detection,omitempty"`
	Tools                   []Tool                `json:"tools,omitempty"`
	ToolChoice              string                `json:"tool_choice,omitempty"`
	Temperature             float64               `json:"temperature,omitempty"`
	MaxOutputTokens         int                   `json:"max_output_tokens,omitempty"`
}

// TranscriptionConfig represents the configuration for input audio transcription.
type TranscriptionConfig struct {
	Model string `json:"model,omitempty"`
}

// TurnDetection represents the configuration for turn detection.
type TurnDetection struct {
	Type              TurnDetectionType `json:"type"`
	Threshold         float64           `json:"threshold,omitempty"`
	PrefixPaddingMs   int               `json:"prefix_padding_ms,omitempty"`
	SilenceDurationMs int               `json:"silence_duration_ms,omitempty"`
	CreateResponse    *bool             `json:"create_response,omitempty"`
}

// Tool represents a tool that can be used by the assistant.
type Tool struct {
	Type        string      `json:"type"` // "function"
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

// ConversationItem represents an item in the conversation.
type ConversationItem struct {
	ID      string      `json:"id"`
	Object  string      `json:"object"` // "realtime.item"
	Type    ItemType    `json:"type"`
	Status  ItemStatus  `json:"status"`
	Role    Role        `json:"role"`
	Content []Content   `json:"content"`
}

// Content represents the content of a conversation item.
type Content struct {
	Type       ContentType `json:"type"`
	Text       string      `json:"text,omitempty"`
	Audio      string      `json:"audio,omitempty"`      // Base64 encoded
	Transcript string      `json:"transcript,omitempty"`
}

// Response represents a response from the assistant.
type Response struct {
	ID            string             `json:"id"`
	Object        string             `json:"object"` // "realtime.response"
	Status        ResponseStatus     `json:"status"`
	StatusDetails *StatusDetails     `json:"status_details,omitempty"`
	Output        []ConversationItem `json:"output"`
	Usage         *Usage             `json:"usage,omitempty"`
}

// StatusDetails provides additional details about the response status.
type StatusDetails struct {
	Type   string      `json:"type,omitempty"`
	Reason string      `json:"reason,omitempty"`
	Error  *ErrorDetail `json:"error,omitempty"`
}

// Usage represents token usage information.
type Usage struct {
	TotalTokens        int                 `json:"total_tokens"`
	InputTokens        int                 `json:"input_tokens"`
	OutputTokens       int                 `json:"output_tokens"`
	InputTokenDetails  *InputTokenDetails  `json:"input_token_details,omitempty"`
	OutputTokenDetails *OutputTokenDetails `json:"output_token_details,omitempty"`
}

// InputTokenDetails provides detailed input token breakdown.
type InputTokenDetails struct {
	CachedTokens int `json:"cached_tokens"`
	TextTokens   int `json:"text_tokens"`
	AudioTokens  int `json:"audio_tokens"`
}

// OutputTokenDetails provides detailed output token breakdown.
type OutputTokenDetails struct {
	TextTokens  int `json:"text_tokens"`
	AudioTokens int `json:"audio_tokens"`
}

// ErrorDetail represents detailed error information.
type ErrorDetail struct {
	Type    ErrorType `json:"type"`
	Code    string    `json:"code,omitempty"`
	Message string    `json:"message"`
	Param   string    `json:"param,omitempty"`
}

// RateLimit represents rate limit information.
type RateLimit struct {
	Name         string `json:"name"`
	Limit        int    `json:"limit"`
	Remaining    int    `json:"remaining"`
	ResetSeconds int    `json:"reset_seconds"`
}

// ResponseConfig is used in response.create events.
type ResponseConfig struct {
	Modalities      []Modality        `json:"modalities,omitempty"`
	Instructions    string            `json:"instructions,omitempty"`
	Voice           string            `json:"voice,omitempty"`
	OutputAudioFormat AudioFormat     `json:"output_audio_format,omitempty"`
	Tools           []Tool            `json:"tools,omitempty"`
	ToolChoice      string            `json:"tool_choice,omitempty"`
	Temperature     float64           `json:"temperature,omitempty"`
	MaxOutputTokens int               `json:"max_output_tokens,omitempty"`
	Conversation    string            `json:"conversation,omitempty"` // "auto" or "none"
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// ItemCreateConfig is used in conversation.item.create events.
type ItemCreateConfig struct {
	Type    ItemType  `json:"type"`
	Role    Role      `json:"role,omitempty"`
	Content []Content `json:"content,omitempty"`
}
