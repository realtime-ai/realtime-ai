package pipeline

import (
	"context"
	"testing"
	"time"
)

// mockBus 用于测试的 mock Bus 实现
type mockBus struct {
	subscribers map[EventType][]chan<- Event
	published   []Event
}

func newMockBus() *mockBus {
	return &mockBus{
		subscribers: make(map[EventType][]chan<- Event),
		published:   make([]Event, 0),
	}
}

func (b *mockBus) Subscribe(eventType EventType, ch chan<- Event) {
	b.subscribers[eventType] = append(b.subscribers[eventType], ch)
}

func (b *mockBus) Unsubscribe(eventType EventType, ch chan<- Event) {
	chans := b.subscribers[eventType]
	for i, c := range chans {
		if c == ch {
			chans = append(chans[:i], chans[i+1:]...)
			break
		}
	}
	b.subscribers[eventType] = chans
}

func (b *mockBus) Publish(evt Event) bool {
	b.published = append(b.published, evt)
	// 分发给订阅者
	for _, ch := range b.subscribers[evt.Type] {
		select {
		case ch <- evt:
		default:
		}
	}
	return true
}

func (b *mockBus) Start(ctx context.Context) error {
	return nil
}

func (b *mockBus) Stop() {}

func (b *mockBus) getPublishedEvents(eventType EventType) []Event {
	var events []Event
	for _, evt := range b.published {
		if evt.Type == eventType {
			events = append(events, evt)
		}
	}
	return events
}

func TestInterruptManager_DefaultConfig(t *testing.T) {
	config := DefaultInterruptConfig()

	if config.EnableVADInterrupt {
		t.Error("EnableVADInterrupt should be false by default")
	}
	if !config.EnableAPIInterrupt {
		t.Error("EnableAPIInterrupt should be true by default")
	}
	if config.EnableHybridMode {
		t.Error("EnableHybridMode should be false by default")
	}
	if config.MinSpeechDurationMs != 100 {
		t.Errorf("MinSpeechDurationMs should be 100, got %d", config.MinSpeechDurationMs)
	}
	if config.InterruptCooldownMs != 500 {
		t.Errorf("InterruptCooldownMs should be 500, got %d", config.InterruptCooldownMs)
	}
}

func TestInterruptManager_StartStop(t *testing.T) {
	bus := newMockBus()
	config := DefaultInterruptConfig()
	im := NewInterruptManager(bus, config)

	ctx := context.Background()
	err := im.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 检查初始状态
	if im.GetState() != InterruptStateIdle {
		t.Errorf("Initial state should be Idle, got %v", im.GetState())
	}

	err = im.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestInterruptManager_StateTransitions(t *testing.T) {
	bus := newMockBus()
	config := DefaultInterruptConfig()
	im := NewInterruptManager(bus, config)

	ctx := context.Background()
	_ = im.Start(ctx)
	defer im.Stop()

	// 等待事件循环启动
	time.Sleep(10 * time.Millisecond)

	// 1. Idle -> AIResponding (收到 ResponseStart)
	bus.Publish(Event{
		Type:      EventResponseStart,
		Timestamp: time.Now(),
		Payload:   &ResponseStartPayload{ResponseID: "resp_001"},
	})
	time.Sleep(10 * time.Millisecond)

	if im.GetState() != InterruptStateAIResponding {
		t.Errorf("State should be AIResponding, got %v", im.GetState())
	}

	// 2. AIResponding -> Idle (收到 ResponseEnd)
	bus.Publish(Event{
		Type:      EventResponseEnd,
		Timestamp: time.Now(),
		Payload:   &ResponseEndPayload{ResponseID: "resp_001", Completed: true},
	})
	time.Sleep(10 * time.Millisecond)

	if im.GetState() != InterruptStateIdle {
		t.Errorf("State should be Idle, got %v", im.GetState())
	}
}

func TestInterruptManager_VADInterrupt(t *testing.T) {
	bus := newMockBus()
	config := DefaultInterruptConfig()
	config.EnableVADInterrupt = true
	config.EnableAPIInterrupt = false
	config.InterruptCooldownMs = 0 // 禁用冷却

	im := NewInterruptManager(bus, config)

	ctx := context.Background()
	_ = im.Start(ctx)
	defer im.Stop()

	time.Sleep(10 * time.Millisecond)

	// 进入 AI 响应状态
	bus.Publish(Event{
		Type:      EventResponseStart,
		Timestamp: time.Now(),
		Payload:   &ResponseStartPayload{ResponseID: "resp_001"},
	})
	time.Sleep(10 * time.Millisecond)

	// 清空已发布的事件
	bus.published = bus.published[:0]

	// VAD 检测到语音开始
	bus.Publish(Event{
		Type:      EventVADSpeechStart,
		Timestamp: time.Now(),
		Payload:   &VADPayload{AudioMs: 0},
	})
	time.Sleep(10 * time.Millisecond)

	// 应该发布了打断事件
	interruptEvents := bus.getPublishedEvents(EventInterrupted)
	if len(interruptEvents) == 0 {
		t.Error("Should have published EventInterrupted")
	}

	if im.GetState() != InterruptStateUserSpeaking {
		t.Errorf("State should be UserSpeaking, got %v", im.GetState())
	}
}

func TestInterruptManager_Cooldown(t *testing.T) {
	bus := newMockBus()
	config := DefaultInterruptConfig()
	config.EnableVADInterrupt = true
	config.InterruptCooldownMs = 1000 // 1秒冷却

	im := NewInterruptManager(bus, config)

	ctx := context.Background()
	_ = im.Start(ctx)
	defer im.Stop()

	time.Sleep(10 * time.Millisecond)

	// 进入 AI 响应状态
	bus.Publish(Event{
		Type:      EventResponseStart,
		Timestamp: time.Now(),
		Payload:   &ResponseStartPayload{ResponseID: "resp_001"},
	})
	time.Sleep(10 * time.Millisecond)

	// 设置上次打断时间为刚才
	im.mu.Lock()
	im.lastInterruptAt = time.Now()
	im.mu.Unlock()

	bus.published = bus.published[:0]

	// VAD 检测到语音开始（应该被冷却阻止）
	bus.Publish(Event{
		Type:      EventVADSpeechStart,
		Timestamp: time.Now(),
		Payload:   &VADPayload{AudioMs: 0},
	})
	time.Sleep(10 * time.Millisecond)

	// 不应该发布打断事件
	interruptEvents := bus.getPublishedEvents(EventInterrupted)
	if len(interruptEvents) > 0 {
		t.Error("Should not publish EventInterrupted during cooldown")
	}
}

func TestInterruptManager_ManualInterrupt(t *testing.T) {
	bus := newMockBus()
	config := DefaultInterruptConfig()
	config.InterruptCooldownMs = 0

	im := NewInterruptManager(bus, config)

	ctx := context.Background()
	_ = im.Start(ctx)
	defer im.Stop()

	time.Sleep(10 * time.Millisecond)

	// 不在 AI 响应状态时的手动打断应该被忽略
	bus.published = bus.published[:0]
	im.TriggerManualInterrupt()
	time.Sleep(10 * time.Millisecond)

	interruptEvents := bus.getPublishedEvents(EventInterrupted)
	if len(interruptEvents) > 0 {
		t.Error("Manual interrupt should be ignored when not in AI responding state")
	}

	// 进入 AI 响应状态
	bus.Publish(Event{
		Type:      EventResponseStart,
		Timestamp: time.Now(),
		Payload:   &ResponseStartPayload{ResponseID: "resp_001"},
	})
	time.Sleep(10 * time.Millisecond)

	bus.published = bus.published[:0]

	// 手动打断
	im.TriggerManualInterrupt()
	time.Sleep(10 * time.Millisecond)

	interruptEvents = bus.getPublishedEvents(EventInterrupted)
	if len(interruptEvents) == 0 {
		t.Error("Manual interrupt should publish EventInterrupted")
	}

	// 检查打断来源
	if payload, ok := interruptEvents[0].Payload.(*InterruptPayload); ok {
		if payload.Source != InterruptSourceClient {
			t.Errorf("Interrupt source should be Client, got %v", payload.Source)
		}
	}
}

func TestInterruptManager_HybridMode_ShortSpeech(t *testing.T) {
	bus := newMockBus()
	config := DefaultInterruptConfig()
	config.EnableHybridMode = true
	config.EnableVADInterrupt = false
	config.EnableAPIInterrupt = false
	config.MinSpeechForConfirmMs = 300
	config.InterruptCooldownMs = 0

	im := NewInterruptManager(bus, config)

	ctx := context.Background()
	_ = im.Start(ctx)
	defer im.Stop()

	time.Sleep(10 * time.Millisecond)

	// 进入 AI 响应状态
	bus.Publish(Event{
		Type:      EventResponseStart,
		Timestamp: time.Now(),
		Payload:   &ResponseStartPayload{ResponseID: "resp_001"},
	})
	time.Sleep(10 * time.Millisecond)

	bus.published = bus.published[:0]

	// VAD 检测到语音开始
	bus.Publish(Event{
		Type:      EventVADSpeechStart,
		Timestamp: time.Now(),
		Payload:   &VADPayload{AudioMs: 0},
	})
	time.Sleep(10 * time.Millisecond)

	// 应该发布暂停事件
	pauseEvents := bus.getPublishedEvents(EventAudioPause)
	if len(pauseEvents) == 0 {
		t.Error("Hybrid mode should publish EventAudioPause on VAD start")
	}

	// 短语音后 VAD 结束（<300ms）
	bus.Publish(Event{
		Type:      EventVADSpeechEnd,
		Timestamp: time.Now(),
		Payload:   &VADPayload{AudioMs: 100},
	})
	time.Sleep(10 * time.Millisecond)

	// 应该发布恢复事件
	resumeEvents := bus.getPublishedEvents(EventAudioResume)
	if len(resumeEvents) == 0 {
		t.Error("Short speech should trigger EventAudioResume")
	}

	// 不应该有打断事件
	interruptEvents := bus.getPublishedEvents(EventInterrupted)
	if len(interruptEvents) > 0 {
		t.Error("Short speech should not trigger EventInterrupted")
	}
}

func TestInterruptState_String(t *testing.T) {
	tests := []struct {
		state    InterruptState
		expected string
	}{
		{InterruptStateIdle, "Idle"},
		{InterruptStateUserSpeaking, "UserSpeaking"},
		{InterruptStateProcessing, "Processing"},
		{InterruptStateAIResponding, "AIResponding"},
		{InterruptStateInterrupted, "Interrupted"},
		{InterruptState(99), "Unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("InterruptState(%d).String() = %q, want %q", tt.state, got, tt.expected)
		}
	}
}
