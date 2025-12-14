// Package state provides state management for the Realtime API server.
package state

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi/events"
)

// ResponseState represents the state of a response.
type ResponseState int

const (
	ResponseStateIdle ResponseState = iota
	ResponseStateInProgress
	ResponseStateCompleted
	ResponseStateFailed
	ResponseStateCancelled
)

// String returns the string representation of ResponseState.
func (s ResponseState) String() string {
	switch s {
	case ResponseStateIdle:
		return "idle"
	case ResponseStateInProgress:
		return "in_progress"
	case ResponseStateFailed:
		return "failed"
	case ResponseStateCancelled:
		return "cancelled"
	default:
		return "completed"
	}
}

// Errors for response tracker operations.
var (
	ErrNoActiveResponse      = errors.New("no active response")
	ErrResponseAlreadyActive = errors.New("response already active")
	ErrInvalidStateTransition = errors.New("invalid state transition")
)

// ResponseContext holds the context for an active response.
type ResponseContext struct {
	ResponseID    string
	ItemID        string
	OutputIndex   int
	ContentIndex  int
	State         ResponseState
	StartTime     time.Time
	AudioData     []byte // Accumulated audio data
	TextData      string // Accumulated text data
	ContentType   events.ContentType
}

// ResponseTracker tracks the lifecycle of responses.
type ResponseTracker struct {
	mu              sync.RWMutex
	currentResponse *ResponseContext
}

// NewResponseTracker creates a new ResponseTracker.
func NewResponseTracker() *ResponseTracker {
	return &ResponseTracker{}
}

// StartResponse starts a new response and returns the response and item IDs.
func (rt *ResponseTracker) StartResponse() (responseID, itemID string, err error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.currentResponse != nil && rt.currentResponse.State == ResponseStateInProgress {
		return "", "", ErrResponseAlreadyActive
	}

	responseID = "resp_" + uuid.New().String()[:8]
	itemID = "item_" + uuid.New().String()[:8]

	rt.currentResponse = &ResponseContext{
		ResponseID:   responseID,
		ItemID:       itemID,
		OutputIndex:  0,
		ContentIndex: 0,
		State:        ResponseStateInProgress,
		StartTime:    time.Now(),
	}

	return responseID, itemID, nil
}

// StartResponseWithContentType starts a new response with a specific content type.
func (rt *ResponseTracker) StartResponseWithContentType(contentType events.ContentType) (responseID, itemID string, err error) {
	responseID, itemID, err = rt.StartResponse()
	if err != nil {
		return "", "", err
	}

	rt.mu.Lock()
	rt.currentResponse.ContentType = contentType
	rt.mu.Unlock()

	return responseID, itemID, nil
}

// GetCurrentResponse returns the current response context.
func (rt *ResponseTracker) GetCurrentResponse() (*ResponseContext, error) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	if rt.currentResponse == nil {
		return nil, ErrNoActiveResponse
	}

	// Return a copy to prevent external modification
	ctx := *rt.currentResponse
	return &ctx, nil
}

// HasActiveResponse returns true if there is an active response.
func (rt *ResponseTracker) HasActiveResponse() bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.currentResponse != nil && rt.currentResponse.State == ResponseStateInProgress
}

// AddAudioData adds audio data to the current response.
func (rt *ResponseTracker) AddAudioData(data []byte) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.currentResponse == nil || rt.currentResponse.State != ResponseStateInProgress {
		return ErrNoActiveResponse
	}

	rt.currentResponse.AudioData = append(rt.currentResponse.AudioData, data...)
	return nil
}

// AddTextData adds text data to the current response.
func (rt *ResponseTracker) AddTextData(text string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.currentResponse == nil || rt.currentResponse.State != ResponseStateInProgress {
		return ErrNoActiveResponse
	}

	rt.currentResponse.TextData += text
	return nil
}

// CompleteResponse marks the current response as completed.
func (rt *ResponseTracker) CompleteResponse(status events.ResponseStatus) (*ResponseContext, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.currentResponse == nil {
		return nil, ErrNoActiveResponse
	}

	if rt.currentResponse.State != ResponseStateInProgress {
		return nil, ErrInvalidStateTransition
	}

	switch status {
	case events.ResponseStatusCompleted:
		rt.currentResponse.State = ResponseStateCompleted
	case events.ResponseStatusCancelled:
		rt.currentResponse.State = ResponseStateCancelled
	case events.ResponseStatusFailed:
		rt.currentResponse.State = ResponseStateFailed
	default:
		rt.currentResponse.State = ResponseStateCompleted
	}

	// Return a copy
	ctx := *rt.currentResponse
	return &ctx, nil
}

// CancelResponse cancels the current response.
func (rt *ResponseTracker) CancelResponse() (*ResponseContext, error) {
	return rt.CompleteResponse(events.ResponseStatusCancelled)
}

// Reset clears the current response state.
func (rt *ResponseTracker) Reset() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.currentResponse = nil
}

// IncrementContentIndex increments the content index.
func (rt *ResponseTracker) IncrementContentIndex() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.currentResponse == nil {
		return 0
	}

	rt.currentResponse.ContentIndex++
	return rt.currentResponse.ContentIndex
}

// SetContentType sets the content type for the current response.
func (rt *ResponseTracker) SetContentType(contentType events.ContentType) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.currentResponse == nil {
		return ErrNoActiveResponse
	}

	rt.currentResponse.ContentType = contentType
	return nil
}
