# Response 完整事件生命周期实现方案

## 概述

本方案实现了 OpenAI Realtime API 规范的完整 Response 生命周期管理，包括所有标准事件的生成和发送。

## 实现文件

### 1. `pkg/realtimeapi/response_manager.go` (新增)

核心文件，包含 ResponseManager 和 PipelineResponseHandler 的实现。

#### 主要组件

**ResponseManager**
- 管理单个 Response 的完整生命周期
- 状态机：Idle → Creating → InProgress → Completing → Completed/Interrupted/Failed
- 自动发送所有标准事件

**PipelineResponseAdapter**
- 将 Pipeline 音频输出适配到 ResponseManager
- 支持音频缓冲和流式发送

**PipelineResponseHandler**
- 监听 Pipeline 输出
- 驱动 ResponseManager 发送事件

#### 关键方法

```go
// 创建响应，发送事件序列：
// response.created → response.output_item.added → conversation.item.created → response.content_part.added
func (rm *ResponseManager) CreateResponse(config events.ResponseConfig) error

// 发送音频增量，触发 response.audio.delta
func (rm *ResponseManager) SendAudioDelta(audioData []byte) error

// 发送文本增量，触发 response.text.delta
func (rm *ResponseManager) SendTextDelta(text string) error

// 完成内容部分，触发 response.audio.done → response.content_part.done
func (rm *ResponseManager) CompleteContentPart() error

// 完成输出项，触发 response.output_item.done
func (rm *ResponseManager) CompleteOutputItem() error

// 完成响应，触发 response.done
func (rm *ResponseManager) CompleteResponse() error

// 中断响应，触发 response.interrupted → response.done (cancelled)
func (rm *ResponseManager) Interrupt(reason string) error
```

### 2. `pkg/realtimeapi/session.go` (修改)

集成 ResponseManager 到 Session。

#### 修改内容

1. **Session 结构体添加字段**
```go
type Session struct {
    // ... 现有字段 ...
    
    // ResponseManager manages the complete response lifecycle
    responseManager *ResponseManager
}
```

2. **新增方法**
```go
// GetResponseManager 获取响应管理器
func (s *Session) GetResponseManager() *ResponseManager
```

3. **修改 handleResponseCreate**
```go
func (s *Session) handleResponseCreate(e *events.ResponseCreateEvent) error {
    // 1. 创建/获取 ResponseManager
    // 2. 合并 ResponseConfig（使用请求参数，缺失值用 Session 配置填充）
    // 3. 调用 rm.CreateResponse(config) 创建响应
    // 4. 启动 PipelineResponseHandler 监听输出
}
```

4. **修改 handleResponseCancel**
```go
func (s *Session) handleResponseCancel(e *events.ResponseCancelEvent) error {
    // 1. 优先通过 ResponseManager 中断
    // 2. 回退到 Pipeline 的 InterruptManager
}
```

### 3. `pkg/realtimeapi/events/server.go` (已存在)

已包含所有需要的事件类型：

- `response.created` / `response.done`
- `response.output_item.added` / `response.output_item.done`
- `response.content_part.added` / `response.content_part.done`
- `response.audio.delta` / `response.audio.done`
- `response.text.delta` / `response.text.done`
- `response.audio_transcript.delta` / `response.audio_transcript.done`
- `response.interrupted` (自定义扩展)

## Response 生命周期流程

```
Client                              Server
  │                                   │
  │─── response.create ──────────────►│
  │                                   │──┐
  │◄── response.created ──────────────│  │
  │                                   │  │ CreateResponse()
  │◄── response.output_item.added ────│  │
  │                                   │  │
  │◄── conversation.item.created ─────│  │
  │                                   │  │
  │◄── response.content_part.added ───│──┘
  │                                   │
  │                                   │──┐
  │◄── response.audio.delta ──────────│  │
  │◄── response.audio.delta ──────────│  │ SendAudioDelta()
  │◄── response.audio.delta ──────────│  │ (stream)
  │                                   │──┘
  │                                   │
  │                                   │──┐
  │◄── response.audio.done ───────────│  │
  │◄── response.content_part.done ────│  │ CompleteContentPart()
  │◄── response.output_item.done ─────│  │ CompleteOutputItem()
  │◄── response.done ─────────────────│──┘ CompleteResponse()
  │                                   │
```

## 中断处理流程

```
Client                              Server
  │                                   │
  │─── response.cancel ──────────────►│
  │                                   │──┐
  │◄── response.interrupted ──────────│  │ Interrupt()
  │◄── response.done (cancelled) ─────│──┘
  │                                   │
```

## ResponseConfig 处理

当客户端发送 `response.create` 时，配置按以下优先级合并：

1. 请求中的 `ResponseConfig` 参数
2. `Session.Config` 中的配置
3. 默认值

```go
config := events.ResponseConfig{}
if e.Response != nil {
    config = *e.Response
}

// 使用默认配置填充缺失值
if len(config.Modalities) == 0 {
    config.Modalities = s.Config.Modalities
}
if config.Voice == "" {
    config.Voice = s.Config.Voice
}
if config.Instructions == "" {
    config.Instructions = s.Config.Instructions
}
if config.OutputAudioFormat == "" {
    config.OutputAudioFormat = s.Config.OutputAudioFormat
}
if config.Temperature == 0 {
    config.Temperature = s.Config.Temperature
}
if config.MaxOutputTokens == 0 {
    config.MaxOutputTokens = s.Config.MaxOutputTokens
}
```

## 使用示例

### 基本用法

```go
// 创建 Session
session := realtimeapi.NewSessionWithTransport(ctx, transport, config)

// 设置 Pipeline
session.SetPipeline(pipeline)

// 启动 Session
session.Start()

// 处理 response.create 事件（自动完成）
// ResponseManager 会自动：
// 1. 发送 response.created
// 2. 创建 output item
// 3. 监听 Pipeline 输出
// 4. 发送 audio/text delta 事件
// 5. 发送完成事件序列
```

### 手动控制（高级用法）

```go
// 获取 ResponseManager
rm := session.GetResponseManager()

// 创建响应
err := rm.CreateResponse(events.ResponseConfig{
    Modalities: []events.Modality{events.ModalityAudio},
    Voice:      "alloy",
})

// 流式发送音频
for audioChunk := range audioStream {
    err := rm.SendAudioDelta(audioChunk)
}

// 完成内容部分
err = rm.CompleteContentPart()

// 完成输出项
err = rm.CompleteOutputItem()

// 完成响应
err = rm.CompleteResponse()
```

### 中断响应

```go
// 通过 response.cancel 事件（推荐）
session.HandleClientEvent(&events.ResponseCancelEvent{
    Reason: "user_interrupt",
})

// 或直接调用 ResponseManager
rm.Interrupt("user_interrupt")
```

## 与现有架构的集成

### Pipeline 集成

```go
// PipelineResponseHandler 自动监听 Pipeline 输出
handler := NewPipelineResponseHandler(session, responseManager)
handler.Start(pipeline)

// 音频输出自动转换为 response.audio.delta
// 文本输出自动转换为 response.text.delta
```

### WebRTC 集成

```go
// WebRTC 模式下，音频通过 RTP 传输
// ResponseManager 仍然发送事件，但音频数据通过 SendAudio() 直接发送
```

### 打断管理器集成

```go
// ResponseManager.Interrupt() 发送 response.interrupted 事件
// 同时更新 Response 状态为 cancelled

// 如果 ResponseManager 不存在，回退到 Pipeline 的 InterruptManager
```

## 测试建议

1. **单元测试**
   - 测试 ResponseManager 状态机转换
   - 测试事件发送顺序
   - 测试中断处理

2. **集成测试**
   - 测试完整 Response 生命周期
   - 测试 Pipeline 集成
   - 测试 WebSocket/WebRTC 传输

3. **兼容性测试**
   - 与 OpenAI 官方客户端对比事件序列
   - 验证所有必需事件已发送

## 后续优化

1. **工具调用支持**
   - 扩展 ResponseManager 支持 function_call 事件
   - 实现 `response.function_call_arguments.delta`

2. **速率限制**
   - 实现 `rate_limits.updated` 事件
   - 添加令牌桶限流

3. **多模态支持**
   - 支持图像输入的事件序列
   - 实现 `conversation.item.create` (image)
