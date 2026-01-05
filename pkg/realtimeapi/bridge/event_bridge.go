// Package bridge provides event translation between Pipeline Bus and WebSocket events.
package bridge

import (
	"context"
	"encoding/base64"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi/events"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi/state"
)

// EventSender is an interface for sending server events.
type EventSender interface {
	SendEvent(event events.ServerEvent) error
}

// AudioSink is an interface for sending audio data via RTP.
// Used in WebRTC mode to send audio directly instead of base64-encoding in events.
type AudioSink interface {
	// SendAudio sends PCM audio data via RTP track.
	SendAudio(data []byte, sampleRate, channels int) error

	// SupportsRTPAudio returns true if audio should be sent via RTP.
	SupportsRTPAudio() bool
}

// EventBridge bridges Pipeline Bus events to WebSocket server events.
type EventBridge struct {
	bus       pipeline.Bus
	sender    EventSender
	tracker   *state.ResponseTracker
	sessionID string

	// AudioSink for RTP-based audio output (WebRTC mode)
	audioSink AudioSink

	// Event channels for subscriptions
	vadStartCh      chan pipeline.Event
	vadEndCh        chan pipeline.Event
	interruptedCh   chan pipeline.Event
	responseStartCh chan pipeline.Event
	responseEndCh   chan pipeline.Event
	audioDeltaCh    chan pipeline.Event
	textDeltaCh     chan pipeline.Event

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewEventBridge creates a new EventBridge.
func NewEventBridge(bus pipeline.Bus, sender EventSender, sessionID string) *EventBridge {
	return &EventBridge{
		bus:             bus,
		sender:          sender,
		tracker:         state.NewResponseTracker(),
		sessionID:       sessionID,
		vadStartCh:      make(chan pipeline.Event, 10),
		vadEndCh:        make(chan pipeline.Event, 10),
		interruptedCh:   make(chan pipeline.Event, 10),
		responseStartCh: make(chan pipeline.Event, 10),
		responseEndCh:   make(chan pipeline.Event, 10),
		audioDeltaCh:    make(chan pipeline.Event, 100),
		textDeltaCh:     make(chan pipeline.Event, 100),
	}
}

// NewEventBridgeWithAudioSink creates a new EventBridge with an AudioSink for RTP audio output.
// Use this for WebRTC mode where audio should be sent via RTP instead of base64-encoded events.
func NewEventBridgeWithAudioSink(bus pipeline.Bus, sender EventSender, sessionID string, audioSink AudioSink) *EventBridge {
	eb := NewEventBridge(bus, sender, sessionID)
	eb.audioSink = audioSink
	return eb
}

// SetAudioSink sets the AudioSink for RTP audio output.
func (eb *EventBridge) SetAudioSink(sink AudioSink) {
	eb.audioSink = sink
}

// Start starts the event bridge.
func (eb *EventBridge) Start(ctx context.Context) error {
	eb.ctx, eb.cancel = context.WithCancel(ctx)

	// Subscribe to pipeline events
	eb.bus.Subscribe(pipeline.EventVADSpeechStart, eb.vadStartCh)
	eb.bus.Subscribe(pipeline.EventVADSpeechEnd, eb.vadEndCh)
	eb.bus.Subscribe(pipeline.EventInterrupted, eb.interruptedCh)
	eb.bus.Subscribe(pipeline.EventResponseStart, eb.responseStartCh)
	eb.bus.Subscribe(pipeline.EventResponseEnd, eb.responseEndCh)
	eb.bus.Subscribe(pipeline.EventAudioDelta, eb.audioDeltaCh)
	eb.bus.Subscribe(pipeline.EventTextDelta, eb.textDeltaCh)

	// Start event handlers
	eb.wg.Add(1)
	go eb.handleEvents()

	return nil
}

// Stop stops the event bridge.
func (eb *EventBridge) Stop() {
	if eb.cancel != nil {
		eb.cancel()
	}

	// Unsubscribe from pipeline events
	eb.bus.Unsubscribe(pipeline.EventVADSpeechStart, eb.vadStartCh)
	eb.bus.Unsubscribe(pipeline.EventVADSpeechEnd, eb.vadEndCh)
	eb.bus.Unsubscribe(pipeline.EventInterrupted, eb.interruptedCh)
	eb.bus.Unsubscribe(pipeline.EventResponseStart, eb.responseStartCh)
	eb.bus.Unsubscribe(pipeline.EventResponseEnd, eb.responseEndCh)
	eb.bus.Unsubscribe(pipeline.EventAudioDelta, eb.audioDeltaCh)
	eb.bus.Unsubscribe(pipeline.EventTextDelta, eb.textDeltaCh)

	eb.wg.Wait()
}

// GetResponseTracker returns the response tracker.
func (eb *EventBridge) GetResponseTracker() *state.ResponseTracker {
	return eb.tracker
}

// handleEvents handles events from the pipeline bus.
func (eb *EventBridge) handleEvents() {
	defer eb.wg.Done()

	for {
		select {
		case <-eb.ctx.Done():
			return

		case evt := <-eb.vadStartCh:
			eb.handleVADSpeechStart(evt)

		case evt := <-eb.vadEndCh:
			eb.handleVADSpeechEnd(evt)

		case evt := <-eb.interruptedCh:
			eb.handleInterrupted(evt)

		case evt := <-eb.responseStartCh:
			eb.handleResponseStart(evt)

		case evt := <-eb.responseEndCh:
			eb.handleResponseEnd(evt)

		case evt := <-eb.audioDeltaCh:
			eb.handleAudioDelta(evt)

		case evt := <-eb.textDeltaCh:
			eb.handleTextDelta(evt)
		}
	}
}

// handleVADSpeechStart handles VAD speech start events.
func (eb *EventBridge) handleVADSpeechStart(evt pipeline.Event) {
	payload, ok := evt.Payload.(*pipeline.VADPayload)
	if !ok {
		log.Printf("[EventBridge] invalid VADSpeechStart payload")
		return
	}

	itemID := payload.ItemID
	if itemID == "" {
		itemID = "item_" + uuid.New().String()[:8]
	}

	eb.sender.SendEvent(events.NewInputAudioBufferSpeechStartedEvent(payload.AudioMs, itemID))
}

// handleVADSpeechEnd handles VAD speech end events.
func (eb *EventBridge) handleVADSpeechEnd(evt pipeline.Event) {
	payload, ok := evt.Payload.(*pipeline.VADPayload)
	if !ok {
		log.Printf("[EventBridge] invalid VADSpeechEnd payload")
		return
	}

	itemID := payload.ItemID
	if itemID == "" {
		itemID = "item_" + uuid.New().String()[:8]
	}

	eb.sender.SendEvent(events.NewInputAudioBufferSpeechStoppedEvent(payload.AudioMs, itemID))
}

// handleInterrupted handles interruption events.
func (eb *EventBridge) handleInterrupted(evt pipeline.Event) {
	log.Printf("[EventBridge] Handling interrupt event")

	// Extract interrupt information
	var audioMs int
	var itemID string
	var reason string
	var responseID string

	// Try to get InterruptPayload first (from InterruptManager)
	if payload, ok := evt.Payload.(*pipeline.InterruptPayload); ok {
		audioMs = payload.AudioMs
		reason = payload.Reason
		responseID = payload.ResponseID
		if responseID != "" {
			log.Printf("[EventBridge] Interrupt for response: %s, reason: %s", responseID, reason)
		}
	} else if payload, ok := evt.Payload.(*pipeline.VADPayload); ok {
		// Fallback to VADPayload (legacy)
		audioMs = payload.AudioMs
		if payload.ItemID != "" {
			itemID = payload.ItemID
		}
		reason = "user_speech_detected"
	}

	// If there's an active response, send interrupted event and complete it
	if eb.tracker.HasActiveResponse() {
		ctx, err := eb.tracker.GetCurrentResponse()
		if err == nil {
			// Send response.interrupted event (custom extension)
			eb.sender.SendEvent(events.NewResponseInterruptedEvent(
				ctx.ResponseID,
				ctx.ItemID,
				audioMs,
				reason,
			))
		}

		log.Printf("[EventBridge] Completing active response due to interrupt")
		eb.completeCurrentResponse(events.ResponseStatusCancelled)
	}

	// Generate item ID if not provided
	if itemID == "" {
		itemID = "item_" + uuid.New().String()[:8]
	}

	// Send speech started event to indicate user is now speaking
	eb.sender.SendEvent(events.NewInputAudioBufferSpeechStartedEvent(audioMs, itemID))

	log.Printf("[EventBridge] Interrupt handled, new input item: %s, reason: %s", itemID, reason)
}

// handleResponseStart handles response start events.
func (eb *EventBridge) handleResponseStart(evt pipeline.Event) {
	// If already have an active response, complete it first
	if eb.tracker.HasActiveResponse() {
		eb.completeCurrentResponse(events.ResponseStatusCompleted)
	}

	// Start new response
	responseID, itemID, err := eb.tracker.StartResponseWithContentType(events.ContentTypeAudio)
	if err != nil {
		log.Printf("[EventBridge] failed to start response: %v", err)
		return
	}

	// Send response.created
	eb.sender.SendEvent(events.NewResponseCreatedEvent(events.Response{
		ID:     responseID,
		Object: "realtime.response",
		Status: events.ResponseStatusInProgress,
		Output: []events.ConversationItem{},
	}))

	// Send output_item.added
	eb.sender.SendEvent(events.NewResponseOutputItemAddedEvent(
		responseID,
		0,
		events.ConversationItem{
			ID:     itemID,
			Object: "realtime.item",
			Type:   events.ItemTypeMessage,
			Status: events.ItemStatusInProgress,
			Role:   events.RoleAssistant,
			Content: []events.Content{
				{Type: events.ContentTypeAudio},
			},
		},
	))

	// Send content_part.added
	eb.sender.SendEvent(events.NewResponseContentPartAddedEvent(
		responseID,
		itemID,
		0,
		0,
		events.Content{Type: events.ContentTypeAudio},
	))
}

// handleResponseEnd handles response end events.
func (eb *EventBridge) handleResponseEnd(evt pipeline.Event) {
	payload, ok := evt.Payload.(*pipeline.ResponseEndPayload)
	status := events.ResponseStatusCompleted
	if ok && !payload.Completed {
		if payload.Reason == "interrupted" || payload.Reason == "cancelled" {
			status = events.ResponseStatusCancelled
		} else if payload.Reason == "error" {
			status = events.ResponseStatusFailed
		}
	}

	eb.completeCurrentResponse(status)
}

// handleAudioDelta handles audio delta events.
func (eb *EventBridge) handleAudioDelta(evt pipeline.Event) {
	payload, ok := evt.Payload.(*pipeline.AudioDeltaPayload)
	if !ok {
		log.Printf("[EventBridge] invalid AudioDelta payload")
		return
	}

	// If no active response, start one
	if !eb.tracker.HasActiveResponse() {
		eb.handleResponseStart(pipeline.Event{
			Type:      pipeline.EventResponseStart,
			Timestamp: time.Now(),
		})
	}

	ctx, err := eb.tracker.GetCurrentResponse()
	if err != nil {
		log.Printf("[EventBridge] failed to get current response: %v", err)
		return
	}

	// Track audio data
	eb.tracker.AddAudioData(payload.Data)

	// Route audio based on transport capability
	if eb.audioSink != nil && eb.audioSink.SupportsRTPAudio() {
		// RTP mode: send audio directly via RTP track
		if err := eb.audioSink.SendAudio(payload.Data, payload.SampleRate, payload.Channels); err != nil {
			log.Printf("[EventBridge] failed to send audio via RTP: %v", err)
		}
		// Note: In RTP mode, we still need to track response state for proper event sequencing
		// but we don't send response.audio.delta events (audio goes via RTP)
		return
	}

	// WebSocket mode: send audio delta event with base64-encoded audio
	audioBase64 := base64.StdEncoding.EncodeToString(payload.Data)
	eb.sender.SendEvent(events.NewResponseAudioDeltaEvent(
		ctx.ResponseID,
		ctx.ItemID,
		ctx.OutputIndex,
		ctx.ContentIndex,
		audioBase64,
	))
}

// handleTextDelta handles text delta events.
func (eb *EventBridge) handleTextDelta(evt pipeline.Event) {
	payload, ok := evt.Payload.(*pipeline.TextDeltaPayload)
	if !ok {
		log.Printf("[EventBridge] invalid TextDelta payload")
		return
	}

	// If no active response, start one
	if !eb.tracker.HasActiveResponse() {
		eb.handleResponseStart(pipeline.Event{
			Type:      pipeline.EventResponseStart,
			Timestamp: time.Now(),
		})
	}

	ctx, err := eb.tracker.GetCurrentResponse()
	if err != nil {
		log.Printf("[EventBridge] failed to get current response: %v", err)
		return
	}

	// Track text data
	eb.tracker.AddTextData(payload.Text)

	// Send text delta event
	eb.sender.SendEvent(events.NewResponseTextDeltaEvent(
		ctx.ResponseID,
		ctx.ItemID,
		ctx.OutputIndex,
		ctx.ContentIndex,
		payload.Text,
	))

	// If this is the final text chunk, send text.done
	if payload.IsFinal {
		eb.sender.SendEvent(events.NewResponseTextDoneEvent(
			ctx.ResponseID,
			ctx.ItemID,
			ctx.OutputIndex,
			ctx.ContentIndex,
			ctx.TextData+payload.Text,
		))
	}
}

// completeCurrentResponse completes the current response with the given status.
func (eb *EventBridge) completeCurrentResponse(status events.ResponseStatus) {
	ctx, err := eb.tracker.CompleteResponse(status)
	if err != nil {
		log.Printf("[EventBridge] failed to complete response: %v", err)
		return
	}

	// Send audio.done if there was audio
	if len(ctx.AudioData) > 0 || ctx.ContentType == events.ContentTypeAudio {
		eb.sender.SendEvent(events.NewResponseAudioDoneEvent(
			ctx.ResponseID,
			ctx.ItemID,
			ctx.OutputIndex,
			ctx.ContentIndex,
		))
	}

	// Send content_part.done
	eb.sender.SendEvent(events.NewResponseContentPartDoneEvent(
		ctx.ResponseID,
		ctx.ItemID,
		ctx.OutputIndex,
		ctx.ContentIndex,
		events.Content{
			Type:  ctx.ContentType,
			Audio: base64.StdEncoding.EncodeToString(ctx.AudioData),
			Text:  ctx.TextData,
		},
	))

	// Send output_item.done
	eb.sender.SendEvent(events.NewResponseOutputItemDoneEvent(
		ctx.ResponseID,
		ctx.OutputIndex,
		events.ConversationItem{
			ID:     ctx.ItemID,
			Object: "realtime.item",
			Type:   events.ItemTypeMessage,
			Status: events.ItemStatusCompleted,
			Role:   events.RoleAssistant,
			Content: []events.Content{
				{
					Type:  ctx.ContentType,
					Audio: base64.StdEncoding.EncodeToString(ctx.AudioData),
					Text:  ctx.TextData,
				},
			},
		},
	))

	// Send response.done
	eb.sender.SendEvent(events.NewResponseDoneEvent(events.Response{
		ID:     ctx.ResponseID,
		Object: "realtime.response",
		Status: status,
		Output: []events.ConversationItem{
			{
				ID:     ctx.ItemID,
				Object: "realtime.item",
				Type:   events.ItemTypeMessage,
				Status: events.ItemStatusCompleted,
				Role:   events.RoleAssistant,
			},
		},
	}))

	// Reset tracker for next response
	eb.tracker.Reset()
}

// ForceCompleteResponse forces completion of any active response.
// This is useful when the pipeline indicates completion via Pull() returning nil.
func (eb *EventBridge) ForceCompleteResponse() {
	if eb.tracker.HasActiveResponse() {
		eb.completeCurrentResponse(events.ResponseStatusCompleted)
	}
}
