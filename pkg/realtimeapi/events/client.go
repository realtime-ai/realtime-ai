package events

import (
	"encoding/json"
	"fmt"
)

// ClientEventType represents the type of client event.
type ClientEventType string

const (
	ClientEventTypeSessionUpdate            ClientEventType = "session.update"
	ClientEventTypeInputAudioBufferAppend   ClientEventType = "input_audio_buffer.append"
	ClientEventTypeInputAudioBufferCommit   ClientEventType = "input_audio_buffer.commit"
	ClientEventTypeInputAudioBufferClear    ClientEventType = "input_audio_buffer.clear"
	ClientEventTypeConversationItemCreate   ClientEventType = "conversation.item.create"
	ClientEventTypeConversationItemTruncate ClientEventType = "conversation.item.truncate"
	ClientEventTypeConversationItemDelete   ClientEventType = "conversation.item.delete"
	ClientEventTypeResponseCreate           ClientEventType = "response.create"
	ClientEventTypeResponseCancel           ClientEventType = "response.cancel"
	ClientEventTypeResponseInterrupt        ClientEventType = "response.interrupt" // Custom: interrupt current response
)

// ClientEvent is the interface for all client events.
type ClientEvent interface {
	ClientEventType() ClientEventType
	GetEventID() string
}

// BaseClientEvent contains common fields for all client events.
type BaseClientEvent struct {
	EventID string          `json:"event_id,omitempty"`
	Type    ClientEventType `json:"type"`
}

func (e BaseClientEvent) ClientEventType() ClientEventType {
	return e.Type
}

func (e BaseClientEvent) GetEventID() string {
	return e.EventID
}

// SessionUpdateEvent updates the session configuration.
type SessionUpdateEvent struct {
	BaseClientEvent
	Session SessionConfig `json:"session"`
}

// InputAudioBufferAppendEvent appends audio data to the input buffer.
type InputAudioBufferAppendEvent struct {
	BaseClientEvent
	Audio string `json:"audio"` // Base64 encoded audio data
}

// InputAudioBufferCommitEvent commits the audio buffer.
type InputAudioBufferCommitEvent struct {
	BaseClientEvent
}

// InputAudioBufferClearEvent clears the audio buffer.
type InputAudioBufferClearEvent struct {
	BaseClientEvent
}

// ConversationItemCreateEvent creates a new conversation item.
type ConversationItemCreateEvent struct {
	BaseClientEvent
	PreviousItemID string           `json:"previous_item_id,omitempty"`
	Item           ItemCreateConfig `json:"item"`
}

// ConversationItemTruncateEvent truncates a conversation item.
type ConversationItemTruncateEvent struct {
	BaseClientEvent
	ItemID       string `json:"item_id"`
	ContentIndex int    `json:"content_index"`
	AudioEndMs   int    `json:"audio_end_ms"`
}

// ConversationItemDeleteEvent deletes a conversation item.
type ConversationItemDeleteEvent struct {
	BaseClientEvent
	ItemID string `json:"item_id"`
}

// ResponseCreateEvent triggers the creation of a response.
type ResponseCreateEvent struct {
	BaseClientEvent
	Response *ResponseConfig `json:"response,omitempty"`
}

// ResponseCancelEvent cancels/interrupts the current response.
// This immediately stops AI output and is compatible with OpenAI Realtime API.
// Both "response.cancel" and "response.interrupt" event types are handled by this struct.
type ResponseCancelEvent struct {
	BaseClientEvent
	Reason string `json:"reason,omitempty"` // Optional reason for cancel/interrupt
}

// ResponseInterruptEvent is an alias for ResponseCancelEvent for backward compatibility.
// Both event types ("response.cancel" and "response.interrupt") trigger the same behavior.
// Recommended: Use ResponseCancelEvent / "response.cancel" for OpenAI API compatibility.
type ResponseInterruptEvent = ResponseCancelEvent

// ParseClientEvent parses a JSON message into a ClientEvent.
func ParseClientEvent(data []byte) (ClientEvent, error) {
	var base BaseClientEvent
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, fmt.Errorf("failed to parse event type: %w", err)
	}

	var event ClientEvent
	var err error

	switch base.Type {
	case ClientEventTypeSessionUpdate:
		var e SessionUpdateEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ClientEventTypeInputAudioBufferAppend:
		var e InputAudioBufferAppendEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ClientEventTypeInputAudioBufferCommit:
		var e InputAudioBufferCommitEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ClientEventTypeInputAudioBufferClear:
		var e InputAudioBufferClearEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ClientEventTypeConversationItemCreate:
		var e ConversationItemCreateEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ClientEventTypeConversationItemTruncate:
		var e ConversationItemTruncateEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ClientEventTypeConversationItemDelete:
		var e ConversationItemDeleteEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ClientEventTypeResponseCreate:
		var e ResponseCreateEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ClientEventTypeResponseCancel:
		var e ResponseCancelEvent
		err = json.Unmarshal(data, &e)
		event = &e

	case ClientEventTypeResponseInterrupt:
		var e ResponseInterruptEvent
		err = json.Unmarshal(data, &e)
		event = &e

	default:
		return nil, fmt.Errorf("unknown client event type: %s", base.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse %s event: %w", base.Type, err)
	}

	return event, nil
}
