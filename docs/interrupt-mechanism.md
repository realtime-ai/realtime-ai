# 打断机制设计文档

本文档描述了 realtime-ai 框架的打断机制（Interrupt/Barge-in），允许用户在 AI 响应过程中通过说话来打断当前输出。

## 1. 概述

**打断（Barge-in/Interruption）** 是指用户在 AI 正在输出响应（语音/文本）时开始说话，系统需要：

1. 立即停止 AI 当前的音频输出
2. 开始处理用户的新输入
3. 保持低延迟的用户体验

```
时间线: ────────────────────────────────────────────>

AI响应:  [████████████████░░░░░░░░░░]  (被打断)
用户:                    [██████████████████]  (新输入)
                         ↑
                    打断点 (需要立即响应)
```

### 核心特性

- **多种打断模式**: VAD 模式、API 模式、混合模式
- **低延迟响应**: 混合模式下 40-60ms 响应
- **平滑过渡**: 音频淡出避免爆音
- **误判恢复**: 混合模式支持短语音恢复
- **状态管理**: 完整的状态机追踪

## 2. 架构设计

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           打断机制架构                                    │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  用户语音输入                                                             │
│       │                                                                  │
│       ▼                                                                  │
│  ┌────────────────┐                                                      │
│  │ SileroVADElement│──► EventVADSpeechStart (预滚动300ms)                │
│  └────────┬───────┘                                                      │
│           │                                                              │
│           ▼                                                              │
│  ┌────────────────────┐                                                  │
│  │  InterruptManager  │ ◄── 打断决策中心                                  │
│  │  ┌──────────────┐  │                                                  │
│  │  │ 状态机        │  │    Idle ↔ UserSpeaking ↔ AIResponding           │
│  │  ├──────────────┤  │         ↘      ↓      ↙                          │
│  │  │ 模式选择      │  │           Interrupted                           │
│  │  │ • VAD模式     │  │                                                  │
│  │  │ • API模式     │  │                                                  │
│  │  │ • 混合模式    │  │                                                  │
│  │  ├──────────────┤  │                                                  │
│  │  │ 冷却控制      │  │    500ms 防止频繁打断                            │
│  │  └──────────────┘  │                                                  │
│  └────────┬───────────┘                                                  │
│           │                                                              │
│           │ EventInterrupted                                             │
│           │                                                              │
│   ┌───────┴───────────────────────────────────┐                          │
│   │                    │                       │                          │
│   ▼                    ▼                       ▼                          │
│ ┌──────────────┐ ┌───────────────┐ ┌─────────────────┐                   │
│ │AudioPacerSink│ │  LLM Element  │ │  EventBridge    │                   │
│ │ • 清空缓冲    │ │ • 取消请求    │ │ • 发送通知       │                   │
│ │ • 50ms淡出   │ │               │ │ • 完成响应       │                   │
│ └──────────────┘ └───────────────┘ └────────┬────────┘                   │
│                                              │                            │
│                                              ▼                            │
│                                    ┌─────────────────┐                   │
│                                    │  WebSocket/RTC  │                   │
│                                    │  • interrupted   │                   │
│                                    │  • speech_started│                   │
│                                    └─────────────────┘                   │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### 组件职责

| 组件 | 职责 |
|------|------|
| InterruptManager | 打断决策中心，管理状态机和打断策略 |
| AudioPacerSinkElement | 音频输出控制，处理清空和淡出 |
| EventBridge | 事件转换，通知客户端 |
| SileroVADElement | 语音活动检测，提供预滚动音频 |

## 3. 打断模式

### 3.1 VAD 模式 (最低延迟)

```
延迟: ~40-60ms
准确率: 85-90%

用户说话 → VAD检测 → 立即打断 → 停止输出
```

**优点**: 延迟最低
**缺点**: 可能误判（咳嗽、背景噪音等）

### 3.2 API 模式 (最高准确率)

```
延迟: ~200-500ms
准确率: 95-99%

用户说话 → 发送到LLM → API返回中断信号 → 停止输出
```

**优点**: 准确率最高
**缺点**: 延迟较高

### 3.3 混合模式 (推荐)

```
延迟: ~40-60ms (感知)
准确率: 95-99%

用户说话 → VAD检测 → 暂停输出 ─┬─► API确认 → 确认打断
                              │
                              └─► 语音<300ms → 恢复输出
```

**优点**: 兼顾低延迟和高准确率，支持误判恢复

## 4. 状态机

```
                    ┌─────────────────┐
                    │      Idle       │ ← 初始状态
                    │   (空闲等待)     │
                    └────────┬────────┘
                             │ VADSpeechStart
                             ▼
                    ┌─────────────────┐
                    │  UserSpeaking   │
                    │   (用户说话中)   │
                    └────────┬────────┘
                             │ VADSpeechEnd + 音频发送到 LLM
                             ▼
                    ┌─────────────────┐
                    │   Processing    │
                    │   (处理中)      │
                    └────────┬────────┘
                             │ ResponseStart
                             ▼
┌──────────┐        ┌─────────────────┐
│Interrupted│◄──────│  AIResponding   │
│ (被打断)  │        │  (AI响应中)     │
└────┬─────┘        └────────┬────────┘
     │                       │ ResponseEnd
     │                       ▼
     │              ┌─────────────────┐
     └─────────────►│      Idle       │
    (重新开始)       └─────────────────┘
```

## 5. 事件定义

### 5.1 Pipeline 事件

| 事件类型 | 说明 | 载荷 |
|---------|------|------|
| `EventInterrupted` | 打断触发 | `InterruptPayload` |
| `EventAudioPause` | 暂停音频输出 | - |
| `EventAudioResume` | 恢复音频输出 | - |
| `EventInterruptAcknowledged` | 组件确认打断 | `map[string]interface{}` |

### 5.2 InterruptPayload 结构

```go
type InterruptPayload struct {
    Source        InterruptSource // 打断来源: VAD/API/Client
    ResponseID    string          // 被打断的响应ID
    InterruptedAt int64           // 打断时间戳(毫秒)
    AudioMs       int             // 已播放音频时长
    Reason        string          // 打断原因
}
```

### 5.3 客户端发送的打断事件

客户端可以发送 `response.cancel` 事件来触发打断（`response.interrupt` 作为别名也被支持，两者功能完全相同）：

```json
{
  "type": "response.cancel",
  "event_id": "evt_client_001",
  "reason": "user_clicked_stop"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `type` | string | `response.cancel` 或 `response.interrupt`（两者等价） |
| `event_id` | string | 可选，客户端生成的事件 ID |
| `reason` | string | 可选，打断原因（如 `user_clicked_stop`、`timeout` 等） |

> **注意**: `response.cancel` 是 OpenAI Realtime API 的标准事件。`response.interrupt` 作为别名保留以保持向后兼容性。推荐使用 `response.cancel`。

### 5.4 服务端返回的打断事件

打断发生时客户端收到的事件序列：

```json
// 1. response.interrupted (自定义扩展)
{
  "type": "response.interrupted",
  "event_id": "evt_xxx",
  "response_id": "resp_xxx",
  "item_id": "item_xxx",
  "audio_ms": 1500,
  "reason": "user_speech_detected"
}

// 2. response.audio.done
// 3. response.content_part.done
// 4. response.output_item.done
// 5. response.done (status: "cancelled")

// 6. input_audio_buffer.speech_started
{
  "type": "input_audio_buffer.speech_started",
  "audio_start_ms": 0,
  "item_id": "item_new"
}
```

## 6. 配置选项

### 6.1 InterruptConfig

```go
type InterruptConfig struct {
    // 打断检测模式
    EnableVADInterrupt bool // 启用 VAD 本地打断
    EnableAPIInterrupt bool // 启用 LLM API 打断信号
    EnableHybridMode   bool // 启用混合模式

    // 敏感度配置
    MinSpeechDurationMs int // 最小语音时长(ms)，默认 100
    InterruptCooldownMs int // 打断冷却时间(ms)，默认 500

    // 混合模式配置
    APIConfirmTimeoutMs   int // API 确认超时(ms)，默认 500
    MinSpeechForConfirmMs int // 无确认时最小语音时长(ms)，默认 300
}
```

### 6.2 AudioPacerSinkConfig

```go
type AudioPacerSinkConfig struct {
    SampleRate int // 采样率
    Channels   int // 通道数
    FadeOutMs  int // 淡出时长(ms)，默认 50
}
```

## 7. 使用示例

### 7.1 启用混合模式打断

```go
// 创建 Pipeline
p := pipeline.NewPipeline("my-pipeline")

// 启用打断管理器（混合模式）
config := pipeline.DefaultInterruptConfig()
config.EnableHybridMode = true
config.MinSpeechForConfirmMs = 300
p.EnableInterruptManager(config)

// 添加 Elements
p.AddElements([]pipeline.Element{
    inputResample,
    gemini,
    outputResample,
    audioPacer,
})

// 启动 Pipeline（自动管理 InterruptManager 生命周期）
p.Start(ctx)
```

### 7.2 服务端手动触发打断

```go
// 获取打断管理器
im := p.GetInterruptManager()

// 手动触发打断（例如用户点击按钮）
im.TriggerManualInterrupt()

// 带自定义原因的打断
im.TriggerManualInterruptWithReason("timeout")
```

### 7.3 客户端信令打断

客户端可以通过 WebSocket/DataChannel 发送信令来打断：

```javascript
// JavaScript 客户端示例
const cancelEvent = {
  type: "response.cancel",  // 推荐使用 response.cancel（OpenAI 标准）
  event_id: "evt_" + Date.now(),
  reason: "user_clicked_stop"
};

websocket.send(JSON.stringify(cancelEvent));

// 注意: "response.interrupt" 也可用（作为别名保留）
```

服务端收到后会立即触发打断流程，客户端会收到 `response.interrupted` 事件。

### 7.4 监听打断事件

```go
// 订阅打断确认事件
ackCh := make(chan pipeline.Event, 10)
p.Bus().Subscribe(pipeline.EventInterruptAcknowledged, ackCh)

go func() {
    for evt := range ackCh {
        log.Printf("Interrupt acknowledged by: %v", evt.Payload)
    }
}()
```

## 8. 性能指标

| 模式 | 用户感知延迟 | 准确率 | 误判恢复 | 复杂度 |
|------|-------------|--------|---------|-------|
| VAD 模式 | 40-60ms | 85-90% | ❌ | 低 |
| API 模式 | 200-500ms | 95-99% | ✅ | 低 |
| **混合模式** | **40-60ms** | **95-99%** | **✅** | 中 |

## 9. 打断流程时序图

### 9.1 混合模式打断流程

```
时间 ─────────────────────────────────────────────────────────────────►

AI输出: ████████████████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
                        │
                        │ (用户开始说话)
用户:                   ▼ ███████████████████████████████████████████████
                        │
                        │
VAD:                    ├─► EventVADSpeechStart
                        │
                        │
InterruptManager:       ├─► 检测到 AI 响应中
                        ├─► 发布 EventAudioPause (暂停输出)
                        ├─► 等待 API 确认或语音时长
                        │
                        │        ┌─► API 确认到达
                        │        │   或
                        ├────────┴─► 语音 > 300ms
                        │
                        ├─► 发布 EventInterrupted
                        │
AudioPacerSink:         ├─► 收到 EventInterrupted
                        ├─► ClearWithFadeOut(50ms)
                        ├─► 发布 EventInterruptAcknowledged
                        │
EventBridge:            ├─► 发送 response.interrupted
                        ├─► 完成响应 (cancelled)
                        └─► 发送 input_audio_buffer.speech_started

总延迟: ~40-60ms (用户感知)
```

### 9.2 短语音恢复流程（混合模式）

```
时间 ─────────────────────────────────────────────────────────────────►

AI输出: ████████████████░░░░░░░░████████████████████████████████████████
                        │       │
                        │       │ (恢复输出)
用户:                   ▼ ██░░░░▼
                        │ │
                        │ │ (短语音 < 300ms)
                        │ │
VAD:                    ├─┤ EventVADSpeechStart / EventVADSpeechEnd
                        │ │
InterruptManager:       ├─┤ 检测到短语音
                        │ └─► 发布 EventAudioResume
                        │
AudioPacerSink:         └───► 收到 EventAudioResume, 恢复播放

结果: 避免误判，AI 继续输出
```

## 10. 关键代码位置

| 组件 | 文件路径 |
|------|---------|
| InterruptManager | `pkg/pipeline/interrupt_manager.go` |
| InterruptConfig | `pkg/pipeline/interrupt_manager.go` |
| InterruptPayload | `pkg/pipeline/bus.go` |
| AudioPacer | `pkg/audio/audio_pacer.go` |
| AudioPacerSinkElement | `pkg/elements/audio_pacer_sink_element.go` |
| EventBridge | `pkg/realtimeapi/bridge/event_bridge.go` |
| ResponseInterruptedEvent | `pkg/realtimeapi/events/server.go` |

## 11. 测试

运行打断机制相关测试：

```bash
# InterruptManager 测试
go test -v ./pkg/pipeline/ -run TestInterrupt

# AudioPacer 暂停/恢复/淡出测试
go test -v ./pkg/audio/ -run 'TestAudioPacer_Pause|TestAudioPacer_Clear'
```

## 12. 注意事项

1. **冷却时间**: 默认 500ms 冷却期防止频繁打断
2. **淡出效果**: 默认 50ms 淡出避免音频爆音
3. **预滚动缓冲**: VAD 提供 300ms 预滚动音频确保语音完整
4. **状态同步**: 打断后会自动重置 ResponseTracker 状态
5. **客户端兼容**: `response.interrupted` 是自定义扩展，标准 OpenAI 客户端可忽略
