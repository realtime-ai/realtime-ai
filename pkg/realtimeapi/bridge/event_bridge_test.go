package bridge

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi/events"
)

// mockEventSender is a mock implementation of EventSender for testing.
type mockEventSender struct {
	mu     sync.Mutex
	events []events.ServerEvent
}

func newMockEventSender() *mockEventSender {
	return &mockEventSender{
		events: make([]events.ServerEvent, 0),
	}
}

func (m *mockEventSender) SendEvent(event events.ServerEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventSender) getEvents() []events.ServerEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]events.ServerEvent, len(m.events))
	copy(result, m.events)
	return result
}

func (m *mockEventSender) getEventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func (m *mockEventSender) getLastEvent() events.ServerEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.events) == 0 {
		return nil
	}
	return m.events[len(m.events)-1]
}

func (m *mockEventSender) hasEventType(eventType events.ServerEventType) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.events {
		if e.ServerEventType() == eventType {
			return true
		}
	}
	return false
}

func TestEventBridge_StartStop(t *testing.T) {
	bus := pipeline.NewEventBus()
	sender := newMockEventSender()

	eb := NewEventBridge(bus, sender, "test-session")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := eb.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Stop should not panic
	eb.Stop()
}

func TestEventBridge_VADSpeechStartEvent(t *testing.T) {
	bus := pipeline.NewEventBus()
	sender := newMockEventSender()

	eb := NewEventBridge(bus, sender, "test-session")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus.Start(ctx)
	eb.Start(ctx)

	// Publish VAD speech start event
	bus.Publish(pipeline.Event{
		Type:      pipeline.EventVADSpeechStart,
		Timestamp: time.Now(),
		Payload: &pipeline.VADPayload{
			AudioMs: 1000,
			ItemID:  "test-item",
		},
	})

	// Wait for event processing
	time.Sleep(100 * time.Millisecond)

	// Check that speech started event was sent
	if !sender.hasEventType(events.ServerEventTypeInputAudioBufferSpeechStarted) {
		t.Error("expected InputAudioBufferSpeechStarted event")
	}

	eb.Stop()
	bus.Stop()
}

func TestEventBridge_VADSpeechEndEvent(t *testing.T) {
	bus := pipeline.NewEventBus()
	sender := newMockEventSender()

	eb := NewEventBridge(bus, sender, "test-session")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus.Start(ctx)
	eb.Start(ctx)

	// Publish VAD speech end event
	bus.Publish(pipeline.Event{
		Type:      pipeline.EventVADSpeechEnd,
		Timestamp: time.Now(),
		Payload: &pipeline.VADPayload{
			AudioMs: 2000,
			ItemID:  "test-item",
		},
	})

	// Wait for event processing
	time.Sleep(100 * time.Millisecond)

	// Check that speech stopped event was sent
	if !sender.hasEventType(events.ServerEventTypeInputAudioBufferSpeechStopped) {
		t.Error("expected InputAudioBufferSpeechStopped event")
	}

	eb.Stop()
	bus.Stop()
}

func TestEventBridge_ResponseStartEvent(t *testing.T) {
	bus := pipeline.NewEventBus()
	sender := newMockEventSender()

	eb := NewEventBridge(bus, sender, "test-session")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus.Start(ctx)
	eb.Start(ctx)

	// Publish response start event
	bus.Publish(pipeline.Event{
		Type:      pipeline.EventResponseStart,
		Timestamp: time.Now(),
		Payload: &pipeline.ResponseStartPayload{
			ResponseID: "resp_123",
		},
	})

	// Wait for event processing
	time.Sleep(100 * time.Millisecond)

	// Check that response created event was sent
	if !sender.hasEventType(events.ServerEventTypeResponseCreated) {
		t.Error("expected ResponseCreated event")
	}

	// Check that output item added event was sent
	if !sender.hasEventType(events.ServerEventTypeResponseOutputItemAdded) {
		t.Error("expected ResponseOutputItemAdded event")
	}

	// Check that content part added event was sent
	if !sender.hasEventType(events.ServerEventTypeResponseContentPartAdded) {
		t.Error("expected ResponseContentPartAdded event")
	}

	eb.Stop()
	bus.Stop()
}

func TestEventBridge_ResponseEndEvent(t *testing.T) {
	bus := pipeline.NewEventBus()
	sender := newMockEventSender()

	eb := NewEventBridge(bus, sender, "test-session")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus.Start(ctx)
	eb.Start(ctx)

	// First start a response
	bus.Publish(pipeline.Event{
		Type:      pipeline.EventResponseStart,
		Timestamp: time.Now(),
		Payload: &pipeline.ResponseStartPayload{
			ResponseID: "resp_123",
		},
	})

	time.Sleep(50 * time.Millisecond)

	// Then end the response
	bus.Publish(pipeline.Event{
		Type:      pipeline.EventResponseEnd,
		Timestamp: time.Now(),
		Payload: &pipeline.ResponseEndPayload{
			ResponseID: "resp_123",
			Completed:  true,
			Reason:     "completed",
		},
	})

	// Wait for event processing
	time.Sleep(100 * time.Millisecond)

	// Check that response done event was sent
	if !sender.hasEventType(events.ServerEventTypeResponseDone) {
		t.Error("expected ResponseDone event")
	}

	eb.Stop()
	bus.Stop()
}

func TestEventBridge_AudioDeltaEvent(t *testing.T) {
	bus := pipeline.NewEventBus()
	sender := newMockEventSender()

	eb := NewEventBridge(bus, sender, "test-session")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus.Start(ctx)
	eb.Start(ctx)

	// Publish audio delta event (should auto-start response)
	bus.Publish(pipeline.Event{
		Type:      pipeline.EventAudioDelta,
		Timestamp: time.Now(),
		Payload: &pipeline.AudioDeltaPayload{
			ResponseID: "resp_123",
			Data:       []byte{1, 2, 3, 4},
			SampleRate: 24000,
			Channels:   1,
		},
	})

	// Wait for event processing
	time.Sleep(100 * time.Millisecond)

	// Check that audio delta event was sent
	if !sender.hasEventType(events.ServerEventTypeResponseAudioDelta) {
		t.Error("expected ResponseAudioDelta event")
	}

	// Check that response was auto-started
	if !sender.hasEventType(events.ServerEventTypeResponseCreated) {
		t.Error("expected ResponseCreated event (auto-started)")
	}

	eb.Stop()
	bus.Stop()
}

func TestEventBridge_TextDeltaEvent(t *testing.T) {
	bus := pipeline.NewEventBus()
	sender := newMockEventSender()

	eb := NewEventBridge(bus, sender, "test-session")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus.Start(ctx)
	eb.Start(ctx)

	// Publish text delta event (should auto-start response)
	bus.Publish(pipeline.Event{
		Type:      pipeline.EventTextDelta,
		Timestamp: time.Now(),
		Payload: &pipeline.TextDeltaPayload{
			ResponseID: "resp_123",
			Text:       "Hello",
			IsFinal:    false,
		},
	})

	// Wait for event processing
	time.Sleep(100 * time.Millisecond)

	// Check that text delta event was sent
	if !sender.hasEventType(events.ServerEventTypeResponseTextDelta) {
		t.Error("expected ResponseTextDelta event")
	}

	eb.Stop()
	bus.Stop()
}

func TestEventBridge_InterruptedEvent(t *testing.T) {
	bus := pipeline.NewEventBus()
	sender := newMockEventSender()

	eb := NewEventBridge(bus, sender, "test-session")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus.Start(ctx)
	eb.Start(ctx)

	// First start a response
	bus.Publish(pipeline.Event{
		Type:      pipeline.EventResponseStart,
		Timestamp: time.Now(),
		Payload: &pipeline.ResponseStartPayload{
			ResponseID: "resp_123",
		},
	})

	time.Sleep(50 * time.Millisecond)

	// Then send interrupted event
	bus.Publish(pipeline.Event{
		Type:      pipeline.EventInterrupted,
		Timestamp: time.Now(),
		Payload: &pipeline.VADPayload{
			AudioMs: 1000,
			ItemID:  "test-item",
		},
	})

	// Wait for event processing
	time.Sleep(100 * time.Millisecond)

	// Check that response was completed (cancelled)
	if !sender.hasEventType(events.ServerEventTypeResponseDone) {
		t.Error("expected ResponseDone event after interruption")
	}

	// Check that speech started event was sent (interruption triggers speech start)
	if !sender.hasEventType(events.ServerEventTypeInputAudioBufferSpeechStarted) {
		t.Error("expected InputAudioBufferSpeechStarted event after interruption")
	}

	eb.Stop()
	bus.Stop()
}

func TestEventBridge_GetResponseTracker(t *testing.T) {
	bus := pipeline.NewEventBus()
	sender := newMockEventSender()

	eb := NewEventBridge(bus, sender, "test-session")

	tracker := eb.GetResponseTracker()
	if tracker == nil {
		t.Error("GetResponseTracker should not return nil")
	}
}

func TestEventBridge_ForceCompleteResponse(t *testing.T) {
	bus := pipeline.NewEventBus()
	sender := newMockEventSender()

	eb := NewEventBridge(bus, sender, "test-session")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus.Start(ctx)
	eb.Start(ctx)

	// Start a response via audio delta
	bus.Publish(pipeline.Event{
		Type:      pipeline.EventAudioDelta,
		Timestamp: time.Now(),
		Payload: &pipeline.AudioDeltaPayload{
			ResponseID: "resp_123",
			Data:       []byte{1, 2, 3, 4},
			SampleRate: 24000,
			Channels:   1,
		},
	})

	time.Sleep(50 * time.Millisecond)

	// Verify response is active
	if !eb.GetResponseTracker().HasActiveResponse() {
		t.Error("expected active response")
	}

	// Force complete
	eb.ForceCompleteResponse()

	// Verify response is no longer active
	if eb.GetResponseTracker().HasActiveResponse() {
		t.Error("expected no active response after ForceCompleteResponse")
	}

	eb.Stop()
	bus.Stop()
}
