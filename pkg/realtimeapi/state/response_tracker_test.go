package state

import (
	"testing"

	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi/events"
)

func TestResponseTracker_StartResponse(t *testing.T) {
	tracker := NewResponseTracker()

	// Test starting a new response
	responseID, itemID, err := tracker.StartResponse()
	if err != nil {
		t.Fatalf("StartResponse failed: %v", err)
	}

	if responseID == "" {
		t.Error("responseID should not be empty")
	}
	if itemID == "" {
		t.Error("itemID should not be empty")
	}

	// Verify active response
	if !tracker.HasActiveResponse() {
		t.Error("HasActiveResponse should return true after StartResponse")
	}

	// Test that starting another response fails
	_, _, err = tracker.StartResponse()
	if err != ErrResponseAlreadyActive {
		t.Errorf("expected ErrResponseAlreadyActive, got %v", err)
	}
}

func TestResponseTracker_GetCurrentResponse(t *testing.T) {
	tracker := NewResponseTracker()

	// Test getting response when none exists
	_, err := tracker.GetCurrentResponse()
	if err != ErrNoActiveResponse {
		t.Errorf("expected ErrNoActiveResponse, got %v", err)
	}

	// Start a response
	responseID, itemID, _ := tracker.StartResponse()

	// Get current response
	ctx, err := tracker.GetCurrentResponse()
	if err != nil {
		t.Fatalf("GetCurrentResponse failed: %v", err)
	}

	if ctx.ResponseID != responseID {
		t.Errorf("ResponseID mismatch: expected %s, got %s", responseID, ctx.ResponseID)
	}
	if ctx.ItemID != itemID {
		t.Errorf("ItemID mismatch: expected %s, got %s", itemID, ctx.ItemID)
	}
	if ctx.State != ResponseStateInProgress {
		t.Errorf("State should be InProgress, got %v", ctx.State)
	}
}

func TestResponseTracker_AddAudioData(t *testing.T) {
	tracker := NewResponseTracker()

	// Test adding audio data when no response exists
	err := tracker.AddAudioData([]byte{1, 2, 3})
	if err != ErrNoActiveResponse {
		t.Errorf("expected ErrNoActiveResponse, got %v", err)
	}

	// Start a response
	tracker.StartResponse()

	// Add audio data
	err = tracker.AddAudioData([]byte{1, 2, 3})
	if err != nil {
		t.Fatalf("AddAudioData failed: %v", err)
	}

	// Add more audio data
	err = tracker.AddAudioData([]byte{4, 5, 6})
	if err != nil {
		t.Fatalf("AddAudioData failed: %v", err)
	}

	// Verify accumulated data
	ctx, _ := tracker.GetCurrentResponse()
	if len(ctx.AudioData) != 6 {
		t.Errorf("expected 6 bytes of audio data, got %d", len(ctx.AudioData))
	}
}

func TestResponseTracker_AddTextData(t *testing.T) {
	tracker := NewResponseTracker()

	// Test adding text data when no response exists
	err := tracker.AddTextData("hello")
	if err != ErrNoActiveResponse {
		t.Errorf("expected ErrNoActiveResponse, got %v", err)
	}

	// Start a response
	tracker.StartResponse()

	// Add text data
	err = tracker.AddTextData("hello")
	if err != nil {
		t.Fatalf("AddTextData failed: %v", err)
	}

	// Add more text data
	err = tracker.AddTextData(" world")
	if err != nil {
		t.Fatalf("AddTextData failed: %v", err)
	}

	// Verify accumulated data
	ctx, _ := tracker.GetCurrentResponse()
	if ctx.TextData != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", ctx.TextData)
	}
}

func TestResponseTracker_CompleteResponse(t *testing.T) {
	tracker := NewResponseTracker()

	// Test completing when no response exists
	_, err := tracker.CompleteResponse(events.ResponseStatusCompleted)
	if err != ErrNoActiveResponse {
		t.Errorf("expected ErrNoActiveResponse, got %v", err)
	}

	// Start a response
	tracker.StartResponse()

	// Complete the response
	ctx, err := tracker.CompleteResponse(events.ResponseStatusCompleted)
	if err != nil {
		t.Fatalf("CompleteResponse failed: %v", err)
	}

	if ctx.State != ResponseStateCompleted {
		t.Errorf("expected state Completed, got %v", ctx.State)
	}

	// Verify response is no longer active
	if tracker.HasActiveResponse() {
		t.Error("HasActiveResponse should return false after CompleteResponse")
	}

	// Test completing again should fail (state no longer InProgress)
	_, err = tracker.CompleteResponse(events.ResponseStatusCompleted)
	if err != ErrInvalidStateTransition {
		t.Errorf("expected ErrInvalidStateTransition, got %v", err)
	}
}

func TestResponseTracker_CancelResponse(t *testing.T) {
	tracker := NewResponseTracker()

	// Start a response
	tracker.StartResponse()

	// Cancel the response
	ctx, err := tracker.CancelResponse()
	if err != nil {
		t.Fatalf("CancelResponse failed: %v", err)
	}

	if ctx.State != ResponseStateCancelled {
		t.Errorf("expected state Cancelled, got %v", ctx.State)
	}
}

func TestResponseTracker_Reset(t *testing.T) {
	tracker := NewResponseTracker()

	// Start and complete a response
	tracker.StartResponse()
	tracker.CompleteResponse(events.ResponseStatusCompleted)

	// Reset
	tracker.Reset()

	// Should be able to start a new response
	_, _, err := tracker.StartResponse()
	if err != nil {
		t.Fatalf("StartResponse after Reset failed: %v", err)
	}
}

func TestResponseTracker_StartResponseWithContentType(t *testing.T) {
	tracker := NewResponseTracker()

	responseID, itemID, err := tracker.StartResponseWithContentType(events.ContentTypeAudio)
	if err != nil {
		t.Fatalf("StartResponseWithContentType failed: %v", err)
	}

	if responseID == "" || itemID == "" {
		t.Error("IDs should not be empty")
	}

	ctx, _ := tracker.GetCurrentResponse()
	if ctx.ContentType != events.ContentTypeAudio {
		t.Errorf("expected ContentTypeAudio, got %v", ctx.ContentType)
	}
}

func TestResponseState_String(t *testing.T) {
	tests := []struct {
		state    ResponseState
		expected string
	}{
		{ResponseStateIdle, "idle"},
		{ResponseStateInProgress, "in_progress"},
		{ResponseStateCompleted, "completed"},
		{ResponseStateFailed, "failed"},
		{ResponseStateCancelled, "cancelled"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("ResponseState.String() = %s, want %s", got, tt.expected)
		}
	}
}
