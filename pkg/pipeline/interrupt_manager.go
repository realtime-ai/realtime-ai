// Package pipeline provides the core pipeline processing framework.
//
// InterruptManager 统一管理打断逻辑的核心组件。
// 负责打断检测、决策、执行的协调工作。
//
// 主要功能:
//   - 聚合多种打断信号源（VAD、LLM API、客户端）
//   - 判断是否应该触发打断（防抖、状态检查）
//   - 广播打断事件到所有相关组件
//   - 管理打断后的状态恢复
//
// 使用示例:
//
//	config := DefaultInterruptConfig()
//	im := NewInterruptManager(bus, config)
//	im.Start(ctx)
package pipeline

import (
	"context"
	"log"
	"sync"
	"time"
)

// InterruptState 表示打断管理器的状态
type InterruptState int

const (
	InterruptStateIdle         InterruptState = iota // 空闲等待
	InterruptStateUserSpeaking                       // 用户说话中
	InterruptStateProcessing                         // 处理中
	InterruptStateAIResponding                       // AI 响应中
	InterruptStateInterrupted                        // 被打断
)

// String 返回状态的字符串表示
func (s InterruptState) String() string {
	switch s {
	case InterruptStateIdle:
		return "Idle"
	case InterruptStateUserSpeaking:
		return "UserSpeaking"
	case InterruptStateProcessing:
		return "Processing"
	case InterruptStateAIResponding:
		return "AIResponding"
	case InterruptStateInterrupted:
		return "Interrupted"
	default:
		return "Unknown"
	}
}

// InterruptConfig 打断机制配置
type InterruptConfig struct {
	// 打断检测模式
	EnableVADInterrupt bool // 启用 VAD 本地打断（最低延迟）
	EnableAPIInterrupt bool // 启用 LLM API 打断信号
	EnableHybridMode   bool // 启用混合模式（VAD快速响应 + API确认）

	// 敏感度配置
	MinSpeechDurationMs int // 最小语音时长才触发打断（毫秒）
	InterruptCooldownMs int // 打断冷却时间（毫秒）

	// 混合模式配置
	APIConfirmTimeoutMs     int // API 确认超时时间（毫秒）
	MinSpeechForConfirmMs   int // 无 API 确认时的最小语音时长（毫秒）
}

// DefaultInterruptConfig 返回默认配置
func DefaultInterruptConfig() InterruptConfig {
	return InterruptConfig{
		EnableVADInterrupt:      false, // 默认不启用纯 VAD 打断
		EnableAPIInterrupt:      true,  // 默认使用 API 打断信号
		EnableHybridMode:        false, // 默认不启用混合模式
		MinSpeechDurationMs:     100,   // 最小 100ms 语音
		InterruptCooldownMs:     500,   // 500ms 冷却时间
		APIConfirmTimeoutMs:     500,   // API 确认超时 500ms
		MinSpeechForConfirmMs:   300,   // 无确认时需要 300ms 语音
	}
}

// InterruptManager 打断管理器
type InterruptManager struct {
	bus    Bus
	config InterruptConfig

	// 状态
	state            InterruptState
	currentResponseID string
	lastInterruptAt  time.Time

	// 混合模式状态
	pendingInterrupt   bool
	pendingInterruptAt time.Time
	speechStartAt      time.Time

	// 同步
	mu     sync.RWMutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewInterruptManager 创建打断管理器
func NewInterruptManager(bus Bus, config InterruptConfig) *InterruptManager {
	return &InterruptManager{
		bus:    bus,
		config: config,
		state:  InterruptStateIdle,
	}
}

// Start 启动打断管理器
func (im *InterruptManager) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	im.cancel = cancel

	im.wg.Add(1)
	go im.eventLoop(ctx)

	log.Printf("[InterruptManager] Started with config: VAD=%v, API=%v, Hybrid=%v",
		im.config.EnableVADInterrupt, im.config.EnableAPIInterrupt, im.config.EnableHybridMode)

	return nil
}

// Stop 停止打断管理器
func (im *InterruptManager) Stop() error {
	if im.cancel != nil {
		im.cancel()
		im.wg.Wait()
		im.cancel = nil
	}
	log.Printf("[InterruptManager] Stopped")
	return nil
}

// eventLoop 事件循环
func (im *InterruptManager) eventLoop(ctx context.Context) {
	defer im.wg.Done()

	// 订阅事件
	vadStartCh := make(chan Event, 10)
	vadEndCh := make(chan Event, 10)
	responseStartCh := make(chan Event, 10)
	responseEndCh := make(chan Event, 10)
	apiInterruptCh := make(chan Event, 10)

	im.bus.Subscribe(EventVADSpeechStart, vadStartCh)
	im.bus.Subscribe(EventVADSpeechEnd, vadEndCh)
	im.bus.Subscribe(EventResponseStart, responseStartCh)
	im.bus.Subscribe(EventResponseEnd, responseEndCh)
	im.bus.Subscribe(EventInterrupted, apiInterruptCh)

	defer func() {
		im.bus.Unsubscribe(EventVADSpeechStart, vadStartCh)
		im.bus.Unsubscribe(EventVADSpeechEnd, vadEndCh)
		im.bus.Unsubscribe(EventResponseStart, responseStartCh)
		im.bus.Unsubscribe(EventResponseEnd, responseEndCh)
		im.bus.Unsubscribe(EventInterrupted, apiInterruptCh)
	}()

	// 混合模式超时检查定时器
	var hybridTimer *time.Timer
	if im.config.EnableHybridMode {
		hybridTimer = time.NewTimer(time.Hour) // 初始化为很长时间
		hybridTimer.Stop()
		defer hybridTimer.Stop()
	}

	for {
		select {
		case <-ctx.Done():
			return

		case evt := <-vadStartCh:
			im.handleVADStart(evt, hybridTimer)

		case evt := <-vadEndCh:
			im.handleVADEnd(evt)

		case evt := <-responseStartCh:
			im.handleResponseStart(evt)

		case evt := <-responseEndCh:
			im.handleResponseEnd(evt)

		case evt := <-apiInterruptCh:
			im.handleAPIInterrupt(evt)

		case <-func() <-chan time.Time {
			if hybridTimer != nil {
				return hybridTimer.C
			}
			return nil
		}():
			im.handleHybridTimeout()
		}
	}
}

// handleVADStart 处理 VAD 语音开始事件
func (im *InterruptManager) handleVADStart(evt Event, hybridTimer *time.Timer) {
	im.mu.Lock()
	defer im.mu.Unlock()

	im.speechStartAt = time.Now()
	prevState := im.state

	log.Printf("[InterruptManager] VAD speech start, state: %s -> UserSpeaking", prevState)

	// 检查是否在 AI 响应中
	if prevState == InterruptStateAIResponding {
		if im.shouldInterrupt(InterruptSourceVAD) {
			if im.config.EnableHybridMode {
				// 混合模式：先暂停输出，等待确认
				im.pendingInterrupt = true
				im.pendingInterruptAt = time.Now()
				im.pauseAudioOutput()

				// 设置超时定时器
				if hybridTimer != nil {
					hybridTimer.Reset(time.Duration(im.config.APIConfirmTimeoutMs) * time.Millisecond)
				}

				log.Printf("[InterruptManager] Hybrid mode: paused audio, waiting for API confirm or timeout")
			} else if im.config.EnableVADInterrupt {
				// 纯 VAD 模式：直接打断
				im.triggerInterruptLocked(InterruptSourceVAD, evt.Payload)
			}
			// 如果只启用 API 打断，这里不做任何事，等待 API 信号
		}
	}

	im.state = InterruptStateUserSpeaking
}

// handleVADEnd 处理 VAD 语音结束事件
func (im *InterruptManager) handleVADEnd(evt Event) {
	im.mu.Lock()
	defer im.mu.Unlock()

	speechDuration := time.Since(im.speechStartAt)
	log.Printf("[InterruptManager] VAD speech end, duration: %v, pending: %v", speechDuration, im.pendingInterrupt)

	// 混合模式：检查是否需要恢复或确认打断
	if im.pendingInterrupt {
		if speechDuration < time.Duration(im.config.MinSpeechForConfirmMs)*time.Millisecond {
			// 语音太短，可能是误判，恢复输出
			log.Printf("[InterruptManager] Short speech (%v < %dms), resuming audio",
				speechDuration, im.config.MinSpeechForConfirmMs)
			im.resumeAudioOutput()
			im.pendingInterrupt = false
			im.state = InterruptStateAIResponding
		} else {
			// 语音足够长，确认打断
			log.Printf("[InterruptManager] Confirming interrupt after %v speech", speechDuration)
			im.confirmInterruptLocked()
		}
	}

	// 状态转换
	if im.state == InterruptStateUserSpeaking {
		im.state = InterruptStateProcessing
	}
}

// handleResponseStart 处理 AI 响应开始事件
func (im *InterruptManager) handleResponseStart(evt Event) {
	im.mu.Lock()
	defer im.mu.Unlock()

	if payload, ok := evt.Payload.(*ResponseStartPayload); ok {
		im.currentResponseID = payload.ResponseID
	}

	im.state = InterruptStateAIResponding
	log.Printf("[InterruptManager] AI response started, responseID: %s", im.currentResponseID)
}

// handleResponseEnd 处理 AI 响应结束事件
func (im *InterruptManager) handleResponseEnd(evt Event) {
	im.mu.Lock()
	defer im.mu.Unlock()

	log.Printf("[InterruptManager] AI response ended, responseID: %s", im.currentResponseID)

	im.state = InterruptStateIdle
	im.currentResponseID = ""
	im.pendingInterrupt = false
}

// handleAPIInterrupt 处理来自 LLM API 的打断信号
func (im *InterruptManager) handleAPIInterrupt(evt Event) {
	im.mu.Lock()
	defer im.mu.Unlock()

	log.Printf("[InterruptManager] API interrupt signal received, pending: %v", im.pendingInterrupt)

	if im.pendingInterrupt {
		// 混合模式：API 确认了打断
		im.confirmInterruptLocked()
	} else if im.config.EnableAPIInterrupt && im.state == InterruptStateAIResponding {
		// 纯 API 模式：触发打断
		// 注意：不重复发布 EventInterrupted，因为它已经由 LLM Element 发布
		im.state = InterruptStateInterrupted
		im.lastInterruptAt = time.Now()
		log.Printf("[InterruptManager] API interrupt confirmed, state -> Interrupted")
	}
}

// handleHybridTimeout 处理混合模式超时
func (im *InterruptManager) handleHybridTimeout() {
	im.mu.Lock()
	defer im.mu.Unlock()

	if !im.pendingInterrupt {
		return
	}

	speechDuration := time.Since(im.speechStartAt)
	log.Printf("[InterruptManager] Hybrid timeout, speech duration: %v", speechDuration)

	if speechDuration >= time.Duration(im.config.MinSpeechForConfirmMs)*time.Millisecond {
		// 语音足够长，确认打断
		im.confirmInterruptLocked()
	} else {
		// 语音不够长，恢复输出
		log.Printf("[InterruptManager] Speech too short at timeout, resuming")
		im.resumeAudioOutput()
		im.pendingInterrupt = false
	}
}

// shouldInterrupt 判断是否应该触发打断
func (im *InterruptManager) shouldInterrupt(source InterruptSource) bool {
	// 冷却时间检查
	if time.Since(im.lastInterruptAt) < time.Duration(im.config.InterruptCooldownMs)*time.Millisecond {
		log.Printf("[InterruptManager] In cooldown period (%v since last interrupt), ignoring",
			time.Since(im.lastInterruptAt))
		return false
	}

	// 根据配置检查是否启用该来源的打断
	switch source {
	case InterruptSourceVAD:
		return im.config.EnableVADInterrupt || im.config.EnableHybridMode
	case InterruptSourceLLMAPI:
		return im.config.EnableAPIInterrupt
	case InterruptSourceClient:
		return true // 客户端手动打断总是允许
	}

	return false
}

// triggerInterruptLocked 触发打断（必须持有锁）
func (im *InterruptManager) triggerInterruptLocked(source InterruptSource, payload interface{}) {
	im.triggerInterruptLockedWithReason(source, payload, "user_speech_detected")
}

// confirmInterruptLocked 确认打断（混合模式使用，必须持有锁）
func (im *InterruptManager) confirmInterruptLocked() {
	log.Printf("[InterruptManager] Confirming interrupt")
	im.triggerInterruptLocked(InterruptSourceVAD, nil)
}

// pauseAudioOutput 暂停音频输出
func (im *InterruptManager) pauseAudioOutput() {
	im.bus.Publish(Event{
		Type:      EventAudioPause,
		Timestamp: time.Now(),
	})
}

// resumeAudioOutput 恢复音频输出
func (im *InterruptManager) resumeAudioOutput() {
	im.bus.Publish(Event{
		Type:      EventAudioResume,
		Timestamp: time.Now(),
	})
}

// TriggerManualInterrupt 手动触发打断（供外部调用）
func (im *InterruptManager) TriggerManualInterrupt() {
	im.TriggerManualInterruptWithReason("client_request")
}

// TriggerManualInterruptWithReason 手动触发打断，支持自定义原因
func (im *InterruptManager) TriggerManualInterruptWithReason(reason string) {
	im.mu.Lock()
	defer im.mu.Unlock()

	if im.state != InterruptStateAIResponding {
		log.Printf("[InterruptManager] Manual interrupt ignored, not in AI responding state")
		return
	}

	log.Printf("[InterruptManager] Manual interrupt triggered, reason: %s", reason)
	im.triggerInterruptLockedWithReason(InterruptSourceClient, nil, reason)
}

// triggerInterruptLockedWithReason 触发打断，支持自定义原因（必须持有锁）
func (im *InterruptManager) triggerInterruptLockedWithReason(source InterruptSource, payload interface{}, reason string) {
	log.Printf("[InterruptManager] Triggering interrupt from source: %v, reason: %s", source, reason)

	im.state = InterruptStateInterrupted
	im.lastInterruptAt = time.Now()
	im.pendingInterrupt = false

	// 构建打断载荷
	interruptPayload := &InterruptPayload{
		Source:        source,
		ResponseID:    im.currentResponseID,
		InterruptedAt: time.Now().UnixMilli(),
		Reason:        reason,
	}

	// 如果原始载荷有音频时间信息，提取
	if vadPayload, ok := payload.(*VADPayload); ok {
		interruptPayload.AudioMs = vadPayload.AudioMs
	}

	// 广播打断事件
	im.bus.Publish(Event{
		Type:      EventInterrupted,
		Timestamp: time.Now(),
		Payload:   interruptPayload,
	})
}

// GetState 获取当前状态
func (im *InterruptManager) GetState() InterruptState {
	im.mu.RLock()
	defer im.mu.RUnlock()
	return im.state
}

// GetConfig 获取配置
func (im *InterruptManager) GetConfig() InterruptConfig {
	return im.config
}

// SetConfig 更新配置
func (im *InterruptManager) SetConfig(config InterruptConfig) {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.config = config
	log.Printf("[InterruptManager] Config updated: VAD=%v, API=%v, Hybrid=%v",
		config.EnableVADInterrupt, config.EnableAPIInterrupt, config.EnableHybridMode)
}
