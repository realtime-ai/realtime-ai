package pipeline

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestEventBusBasicPublishSubscribe(t *testing.T) {
	bus := NewEventBus()
	ch := make(chan Event, 1)

	// Subscribe to an event type
	bus.Subscribe(EventError, ch)

	// Create and publish an event
	evt := Event{
		Type:      EventError,
		Timestamp: time.Now(),
		Payload:   "test error",
	}
	bus.Publish(evt)

	// Receive the event
	received := <-ch
	if received.Type != EventError {
		t.Errorf("Expected event type %v, got %v", EventError, received.Type)
	}
	if received.Payload.(string) != "test error" {
		t.Errorf("Expected payload 'test error', got %v", received.Payload)
	}
}

func TestEventBusUnsubscribe(t *testing.T) {
	bus := NewEventBus()
	ch := make(chan Event, 1)

	// Subscribe and then unsubscribe
	bus.Subscribe(EventWarning, ch)
	bus.Unsubscribe(EventWarning, ch)

	// Publish an event
	evt := Event{
		Type:      EventWarning,
		Timestamp: time.Now(),
		Payload:   "test warning",
	}
	bus.Publish(evt)

	// Verify no event is received
	select {
	case <-ch:
		t.Error("Should not receive event after unsubscribe")
	case <-time.After(100 * time.Millisecond):
		// Test passed - no event received
	}
}

func TestEventBusMultipleSubscribers(t *testing.T) {
	bus := NewEventBus()
	ch1 := make(chan Event, 1)
	ch2 := make(chan Event, 1)

	// Subscribe multiple channels
	bus.Subscribe(EventPartialResult, ch1)
	bus.Subscribe(EventPartialResult, ch2)

	evt := Event{
		Type:      EventPartialResult,
		Timestamp: time.Now(),
		Payload:   "test partial result",
	}
	bus.Publish(evt)

	// Both channels should receive the event
	for _, ch := range []chan Event{ch1, ch2} {
		select {
		case received := <-ch:
			if received.Type != EventPartialResult {
				t.Errorf("Expected event type %v, got %v", EventPartialResult, received.Type)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("Timeout waiting for event")
		}
	}
}

func TestEventBusAsyncOperation(t *testing.T) {
	bus := NewEventBus()
	ch := make(chan Event, 1)
	ctx := context.Background()

	// Start the bus
	if err := bus.Start(ctx); err != nil {
		t.Fatalf("Failed to start event bus: %v", err)
	}

	bus.Subscribe(EventFinalResult, ch)

	// Create and publish events
	evt := Event{
		Type:      EventFinalResult,
		Timestamp: time.Now(),
		Payload:   "test final result",
	}
	bus.Publish(evt)

	// Verify event is received
	select {
	case received := <-ch:
		if received.Type != EventFinalResult {
			t.Errorf("Expected event type %v, got %v", EventFinalResult, received.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for event")
	}

	// Stop the bus
	bus.Stop()
}

func TestEventBusChannelBlocking(t *testing.T) {
	bus := NewEventBus()
	// Create a channel with buffer size 1
	ch := make(chan Event, 1)

	bus.Subscribe(EventBargeIn, ch)

	// Fill the channel
	evt1 := Event{
		Type:      EventBargeIn,
		Timestamp: time.Now(),
		Payload:   "first event",
	}
	delivered := bus.Publish(evt1)
	if !delivered {
		t.Error("First event should be delivered successfully")
	}

	// Try to publish another event (should not block but will be dropped)
	evt2 := Event{
		Type:      EventBargeIn,
		Timestamp: time.Now(),
		Payload:   "second event",
	}

	// Use WaitGroup to ensure the publish operation completes
	var wg sync.WaitGroup
	var secondDelivered bool
	wg.Add(1)
	go func() {
		defer wg.Done()
		secondDelivered = bus.Publish(evt2)
	}()

	// Wait for a short time to ensure the publish operation completes
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Test passed - publish did not block
		if secondDelivered {
			t.Error("Second event should be dropped when channel is full")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Publish operation blocked when channel was full")
	}
}

func TestEventBusStartStop(t *testing.T) {
	bus := NewEventBus()
	ctx := context.Background()

	// Test multiple starts
	if err := bus.Start(ctx); err != nil {
		t.Fatalf("First start failed: %v", err)
	}
	if err := bus.Start(ctx); err != nil {
		t.Fatalf("Second start should not fail: %v", err)
	}

	// Test stop
	bus.Stop()
	bus.Stop() // Multiple stops should be safe

	// Test start after stop
	if err := bus.Start(ctx); err != nil {
		t.Fatalf("Start after stop failed: %v", err)
	}
}
