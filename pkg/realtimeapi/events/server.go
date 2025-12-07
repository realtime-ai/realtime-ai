package events

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// ServerEventType represents the type of server event.
type ServerEventType string

const (
	ServerEventTypeError                                        ServerEventType = "error"
	ServerEventTypeSessionCreated                               ServerEventType = "session.created"
	ServerEventTypeSessionUpdated                               ServerEventType = "session.updated"
	ServerEventTypeInputAudioBufferCommitted                    ServerEventType = "input_audio_buffer.committed"
	ServerEventTypeInputAudioBufferCleared                      ServerEventType = "input_audio_buffer.cleared"
	ServerEventTypeInputAudioBufferSpeechStarted                ServerEventType = "input_audio_buffer.speech_started"
	ServerEventTypeInputAudioBufferSpeechStopped                ServerEventType = "input_audio_buffer.speech_stopped"
	ServerEventTypeConversationCreated                          ServerEventType = "conversation.created"
	ServerEventTypeConversationItemCreated                      ServerEventType = "conversation.item.created"
	ServerEventTypeConversationItemInputAudioTranscriptionCompleted ServerEventType = "conversation.item.input_audio_transcription.completed"
	ServerEventTypeConversationItemInputAudioTranscriptionFailed    ServerEventType = "conversation.item.input_audio_transcription.failed"
	ServerEventTypeConversationItemTruncated                    ServerEventType = "conversation.item.truncated"
	ServerEventTypeConversationItemDeleted                      ServerEventType = "conversation.item.deleted"
	ServerEventTypeResponseCreated                              ServerEventType = "response.created"
	ServerEventTypeResponseDone                                 ServerEventType = "response.done"
	ServerEventTypeResponseOutputItemAdded                      ServerEventType = "response.output_item.added"
	ServerEventTypeResponseOutputItemDone                       ServerEventType = "response.output_item.done"
	ServerEventTypeResponseContentPartAdded                     ServerEventType = "response.content_part.added"
	ServerEventTypeResponseContentPartDone                      ServerEventType = "response.content_part.done"
	ServerEventTypeResponseTextDelta                            ServerEventType = "response.text.delta"
	ServerEventTypeResponseTextDone                             ServerEventType = "response.text.done"
	ServerEventTypeResponseAudioDelta                           ServerEventType = "response.audio.delta"
	ServerEventTypeResponseAudioDone                            ServerEventType = "response.audio.done"
	ServerEventTypeResponseAudioTranscriptDelta                 ServerEventType = "response.audio_transcript.delta"
	ServerEventTypeResponseAudioTranscriptDone                  ServerEventType = "response.audio_transcript.done"
	ServerEventTypeResponseFunctionCallArgumentsDelta           ServerEventType = "response.function_call_arguments.delta"
	ServerEventTypeResponseFunctionCallArgumentsDone            ServerEventType = "response.function_call_arguments.done"
	ServerEventTypeRateLimitsUpdated                            ServerEventType = "rate_limits.updated"
)

// ServerEvent is the interface for all server events.
type ServerEvent interface {
	ServerEventType() ServerEventType
	GetEventID() string
}

// BaseServerEvent contains common fields for all server events.
type BaseServerEvent struct {
	EventID string          `json:"event_id"`
	Type    ServerEventType `json:"type"`
}

func (e BaseServerEvent) ServerEventType() ServerEventType {
	return e.Type
}

func (e BaseServerEvent) GetEventID() string {
	return e.EventID
}

// NewBaseServerEvent creates a new base server event with a generated event ID.
func NewBaseServerEvent(eventType ServerEventType) BaseServerEvent {
	return BaseServerEvent{
		EventID: "evt_" + uuid.New().String()[:8],
		Type:    eventType,
	}
}

// ErrorEvent represents an error from the server.
type ErrorEvent struct {
	BaseServerEvent
	Error ErrorDetail `json:"error"`
}

func NewErrorEvent(errType ErrorType, code, message, param string) *ErrorEvent {
	return &ErrorEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeError),
		Error: ErrorDetail{
			Type:    errType,
			Code:    code,
			Message: message,
			Param:   param,
		},
	}
}

// SessionCreatedEvent is sent when a session is created.
type SessionCreatedEvent struct {
	BaseServerEvent
	Session Session `json:"session"`
}

func NewSessionCreatedEvent(session Session) *SessionCreatedEvent {
	return &SessionCreatedEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeSessionCreated),
		Session:         session,
	}
}

// SessionUpdatedEvent is sent when a session is updated.
type SessionUpdatedEvent struct {
	BaseServerEvent
	Session Session `json:"session"`
}

func NewSessionUpdatedEvent(session Session) *SessionUpdatedEvent {
	return &SessionUpdatedEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeSessionUpdated),
		Session:         session,
	}
}

// InputAudioBufferCommittedEvent is sent when the audio buffer is committed.
type InputAudioBufferCommittedEvent struct {
	BaseServerEvent
	PreviousItemID string `json:"previous_item_id,omitempty"`
	ItemID         string `json:"item_id"`
}

func NewInputAudioBufferCommittedEvent(itemID, previousItemID string) *InputAudioBufferCommittedEvent {
	return &InputAudioBufferCommittedEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeInputAudioBufferCommitted),
		ItemID:          itemID,
		PreviousItemID:  previousItemID,
	}
}

// InputAudioBufferClearedEvent is sent when the audio buffer is cleared.
type InputAudioBufferClearedEvent struct {
	BaseServerEvent
}

func NewInputAudioBufferClearedEvent() *InputAudioBufferClearedEvent {
	return &InputAudioBufferClearedEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeInputAudioBufferCleared),
	}
}

// InputAudioBufferSpeechStartedEvent is sent when speech is detected.
type InputAudioBufferSpeechStartedEvent struct {
	BaseServerEvent
	AudioStartMs int    `json:"audio_start_ms"`
	ItemID       string `json:"item_id"`
}

func NewInputAudioBufferSpeechStartedEvent(audioStartMs int, itemID string) *InputAudioBufferSpeechStartedEvent {
	return &InputAudioBufferSpeechStartedEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeInputAudioBufferSpeechStarted),
		AudioStartMs:    audioStartMs,
		ItemID:          itemID,
	}
}

// InputAudioBufferSpeechStoppedEvent is sent when speech ends.
type InputAudioBufferSpeechStoppedEvent struct {
	BaseServerEvent
	AudioEndMs int    `json:"audio_end_ms"`
	ItemID     string `json:"item_id"`
}

func NewInputAudioBufferSpeechStoppedEvent(audioEndMs int, itemID string) *InputAudioBufferSpeechStoppedEvent {
	return &InputAudioBufferSpeechStoppedEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeInputAudioBufferSpeechStopped),
		AudioEndMs:      audioEndMs,
		ItemID:          itemID,
	}
}

// ConversationCreatedEvent is sent when a conversation is created.
type ConversationCreatedEvent struct {
	BaseServerEvent
	Conversation struct {
		ID     string `json:"id"`
		Object string `json:"object"` // "realtime.conversation"
	} `json:"conversation"`
}

func NewConversationCreatedEvent(conversationID string) *ConversationCreatedEvent {
	e := &ConversationCreatedEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeConversationCreated),
	}
	e.Conversation.ID = conversationID
	e.Conversation.Object = "realtime.conversation"
	return e
}

// ConversationItemCreatedEvent is sent when a conversation item is created.
type ConversationItemCreatedEvent struct {
	BaseServerEvent
	PreviousItemID string           `json:"previous_item_id,omitempty"`
	Item           ConversationItem `json:"item"`
}

func NewConversationItemCreatedEvent(item ConversationItem, previousItemID string) *ConversationItemCreatedEvent {
	return &ConversationItemCreatedEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeConversationItemCreated),
		PreviousItemID:  previousItemID,
		Item:            item,
	}
}

// ConversationItemInputAudioTranscriptionCompletedEvent is sent when transcription is complete.
type ConversationItemInputAudioTranscriptionCompletedEvent struct {
	BaseServerEvent
	ItemID       string `json:"item_id"`
	ContentIndex int    `json:"content_index"`
	Transcript   string `json:"transcript"`
}

func NewConversationItemInputAudioTranscriptionCompletedEvent(itemID string, contentIndex int, transcript string) *ConversationItemInputAudioTranscriptionCompletedEvent {
	return &ConversationItemInputAudioTranscriptionCompletedEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeConversationItemInputAudioTranscriptionCompleted),
		ItemID:          itemID,
		ContentIndex:    contentIndex,
		Transcript:      transcript,
	}
}

// ConversationItemTruncatedEvent is sent when a conversation item is truncated.
type ConversationItemTruncatedEvent struct {
	BaseServerEvent
	ItemID       string `json:"item_id"`
	ContentIndex int    `json:"content_index"`
	AudioEndMs   int    `json:"audio_end_ms"`
}

// ConversationItemDeletedEvent is sent when a conversation item is deleted.
type ConversationItemDeletedEvent struct {
	BaseServerEvent
	ItemID string `json:"item_id"`
}

// ResponseCreatedEvent is sent when a response is created.
type ResponseCreatedEvent struct {
	BaseServerEvent
	Response Response `json:"response"`
}

func NewResponseCreatedEvent(response Response) *ResponseCreatedEvent {
	return &ResponseCreatedEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeResponseCreated),
		Response:        response,
	}
}

// ResponseDoneEvent is sent when a response is complete.
type ResponseDoneEvent struct {
	BaseServerEvent
	Response Response `json:"response"`
}

func NewResponseDoneEvent(response Response) *ResponseDoneEvent {
	return &ResponseDoneEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeResponseDone),
		Response:        response,
	}
}

// ResponseOutputItemAddedEvent is sent when an output item is added.
type ResponseOutputItemAddedEvent struct {
	BaseServerEvent
	ResponseID  string           `json:"response_id"`
	OutputIndex int              `json:"output_index"`
	Item        ConversationItem `json:"item"`
}

func NewResponseOutputItemAddedEvent(responseID string, outputIndex int, item ConversationItem) *ResponseOutputItemAddedEvent {
	return &ResponseOutputItemAddedEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeResponseOutputItemAdded),
		ResponseID:      responseID,
		OutputIndex:     outputIndex,
		Item:            item,
	}
}

// ResponseOutputItemDoneEvent is sent when an output item is complete.
type ResponseOutputItemDoneEvent struct {
	BaseServerEvent
	ResponseID  string           `json:"response_id"`
	OutputIndex int              `json:"output_index"`
	Item        ConversationItem `json:"item"`
}

func NewResponseOutputItemDoneEvent(responseID string, outputIndex int, item ConversationItem) *ResponseOutputItemDoneEvent {
	return &ResponseOutputItemDoneEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeResponseOutputItemDone),
		ResponseID:      responseID,
		OutputIndex:     outputIndex,
		Item:            item,
	}
}

// ResponseContentPartAddedEvent is sent when a content part is added.
type ResponseContentPartAddedEvent struct {
	BaseServerEvent
	ResponseID   string  `json:"response_id"`
	ItemID       string  `json:"item_id"`
	OutputIndex  int     `json:"output_index"`
	ContentIndex int     `json:"content_index"`
	Part         Content `json:"part"`
}

func NewResponseContentPartAddedEvent(responseID, itemID string, outputIndex, contentIndex int, part Content) *ResponseContentPartAddedEvent {
	return &ResponseContentPartAddedEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeResponseContentPartAdded),
		ResponseID:      responseID,
		ItemID:          itemID,
		OutputIndex:     outputIndex,
		ContentIndex:    contentIndex,
		Part:            part,
	}
}

// ResponseContentPartDoneEvent is sent when a content part is complete.
type ResponseContentPartDoneEvent struct {
	BaseServerEvent
	ResponseID   string  `json:"response_id"`
	ItemID       string  `json:"item_id"`
	OutputIndex  int     `json:"output_index"`
	ContentIndex int     `json:"content_index"`
	Part         Content `json:"part"`
}

func NewResponseContentPartDoneEvent(responseID, itemID string, outputIndex, contentIndex int, part Content) *ResponseContentPartDoneEvent {
	return &ResponseContentPartDoneEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeResponseContentPartDone),
		ResponseID:      responseID,
		ItemID:          itemID,
		OutputIndex:     outputIndex,
		ContentIndex:    contentIndex,
		Part:            part,
	}
}

// ResponseTextDeltaEvent is sent for text deltas.
type ResponseTextDeltaEvent struct {
	BaseServerEvent
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

func NewResponseTextDeltaEvent(responseID, itemID string, outputIndex, contentIndex int, delta string) *ResponseTextDeltaEvent {
	return &ResponseTextDeltaEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeResponseTextDelta),
		ResponseID:      responseID,
		ItemID:          itemID,
		OutputIndex:     outputIndex,
		ContentIndex:    contentIndex,
		Delta:           delta,
	}
}

// ResponseTextDoneEvent is sent when text is complete.
type ResponseTextDoneEvent struct {
	BaseServerEvent
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Text         string `json:"text"`
}

func NewResponseTextDoneEvent(responseID, itemID string, outputIndex, contentIndex int, text string) *ResponseTextDoneEvent {
	return &ResponseTextDoneEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeResponseTextDone),
		ResponseID:      responseID,
		ItemID:          itemID,
		OutputIndex:     outputIndex,
		ContentIndex:    contentIndex,
		Text:            text,
	}
}

// ResponseAudioDeltaEvent is sent for audio deltas.
type ResponseAudioDeltaEvent struct {
	BaseServerEvent
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"` // Base64 encoded audio
}

func NewResponseAudioDeltaEvent(responseID, itemID string, outputIndex, contentIndex int, delta string) *ResponseAudioDeltaEvent {
	return &ResponseAudioDeltaEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeResponseAudioDelta),
		ResponseID:      responseID,
		ItemID:          itemID,
		OutputIndex:     outputIndex,
		ContentIndex:    contentIndex,
		Delta:           delta,
	}
}

// ResponseAudioDoneEvent is sent when audio is complete.
type ResponseAudioDoneEvent struct {
	BaseServerEvent
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
}

func NewResponseAudioDoneEvent(responseID, itemID string, outputIndex, contentIndex int) *ResponseAudioDoneEvent {
	return &ResponseAudioDoneEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeResponseAudioDone),
		ResponseID:      responseID,
		ItemID:          itemID,
		OutputIndex:     outputIndex,
		ContentIndex:    contentIndex,
	}
}

// ResponseAudioTranscriptDeltaEvent is sent for audio transcript deltas.
type ResponseAudioTranscriptDeltaEvent struct {
	BaseServerEvent
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

func NewResponseAudioTranscriptDeltaEvent(responseID, itemID string, outputIndex, contentIndex int, delta string) *ResponseAudioTranscriptDeltaEvent {
	return &ResponseAudioTranscriptDeltaEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeResponseAudioTranscriptDelta),
		ResponseID:      responseID,
		ItemID:          itemID,
		OutputIndex:     outputIndex,
		ContentIndex:    contentIndex,
		Delta:           delta,
	}
}

// ResponseAudioTranscriptDoneEvent is sent when audio transcript is complete.
type ResponseAudioTranscriptDoneEvent struct {
	BaseServerEvent
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Transcript   string `json:"transcript"`
}

func NewResponseAudioTranscriptDoneEvent(responseID, itemID string, outputIndex, contentIndex int, transcript string) *ResponseAudioTranscriptDoneEvent {
	return &ResponseAudioTranscriptDoneEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeResponseAudioTranscriptDone),
		ResponseID:      responseID,
		ItemID:          itemID,
		OutputIndex:     outputIndex,
		ContentIndex:    contentIndex,
		Transcript:      transcript,
	}
}

// ResponseFunctionCallArgumentsDeltaEvent is sent for function call argument deltas.
type ResponseFunctionCallArgumentsDeltaEvent struct {
	BaseServerEvent
	ResponseID  string `json:"response_id"`
	ItemID      string `json:"item_id"`
	OutputIndex int    `json:"output_index"`
	CallID      string `json:"call_id"`
	Delta       string `json:"delta"`
}

// ResponseFunctionCallArgumentsDoneEvent is sent when function call arguments are complete.
type ResponseFunctionCallArgumentsDoneEvent struct {
	BaseServerEvent
	ResponseID  string `json:"response_id"`
	ItemID      string `json:"item_id"`
	OutputIndex int    `json:"output_index"`
	CallID      string `json:"call_id"`
	Arguments   string `json:"arguments"`
}

// RateLimitsUpdatedEvent is sent when rate limits are updated.
type RateLimitsUpdatedEvent struct {
	BaseServerEvent
	RateLimits []RateLimit `json:"rate_limits"`
}

func NewRateLimitsUpdatedEvent(rateLimits []RateLimit) *RateLimitsUpdatedEvent {
	return &RateLimitsUpdatedEvent{
		BaseServerEvent: NewBaseServerEvent(ServerEventTypeRateLimitsUpdated),
		RateLimits:      rateLimits,
	}
}

// ParseServerEvent parses a JSON message into a ServerEvent.
func ParseServerEvent(data []byte) (ServerEvent, error) {
	var base BaseServerEvent
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, fmt.Errorf("failed to parse event type: %w", err)
	}

	var event ServerEvent
	var err error

	switch base.Type {
	case ServerEventTypeError:
		var e ErrorEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ServerEventTypeSessionCreated:
		var e SessionCreatedEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ServerEventTypeSessionUpdated:
		var e SessionUpdatedEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ServerEventTypeInputAudioBufferCommitted:
		var e InputAudioBufferCommittedEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ServerEventTypeInputAudioBufferCleared:
		var e InputAudioBufferClearedEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ServerEventTypeInputAudioBufferSpeechStarted:
		var e InputAudioBufferSpeechStartedEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ServerEventTypeInputAudioBufferSpeechStopped:
		var e InputAudioBufferSpeechStoppedEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ServerEventTypeResponseCreated:
		var e ResponseCreatedEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ServerEventTypeResponseDone:
		var e ResponseDoneEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ServerEventTypeResponseAudioDelta:
		var e ResponseAudioDeltaEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ServerEventTypeResponseAudioDone:
		var e ResponseAudioDoneEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ServerEventTypeResponseTextDelta:
		var e ResponseTextDeltaEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ServerEventTypeResponseTextDone:
		var e ResponseTextDoneEvent
		err = json.Unmarshal(data, &e)
		event = &e

	default:
		// For unhandled types, return the base event
		return &base, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse %s event: %w", base.Type, err)
	}

	return event, nil
}
