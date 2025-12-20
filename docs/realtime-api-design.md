# Realtime AI API 设计方案

本文档描述了 realtime-ai 框架对外提供的 Realtime API 设计，该 API 类似于 OpenAI Realtime API，采用 WebSocket 协议和事件驱动的 JSON 消息格式。

## 1. 概述

Realtime API 允许外部客户端通过标准化的事件驱动协议与 realtime-ai 框架交互，支持实时语音/文本双向对话。

### 核心特性

- **WebSocket 协议**: 低延迟双向通信
- **事件驱动**: JSON 格式的事件消息
- **多模态支持**: 文本和音频输入/输出
- **服务端 VAD**: 自动语音活动检测
- **流式响应**: 增量返回音频和文本
- **多模型支持**: Gemini、OpenAI 等 AI 后端

## 2. 架构设计

```
┌─────────────────────────────────────────────────────────────────┐
│                        Realtime API Server                       │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────┐    ┌──────────────┐    ┌─────────────────────┐ │
│  │  WebSocket  │───▶│   Session    │───▶│      Pipeline       │ │
│  │   Server    │    │   Manager    │    │   (Configurable)    │ │
│  └─────────────┘    └──────────────┘    └─────────────────────┘ │
│        │                   │                      │             │
│        ▼                   ▼                      ▼             │
│  ┌─────────────┐    ┌──────────────┐    ┌─────────────────────┐ │
│  │   Event     │    │    Audio     │    │    LLM Elements     │ │
│  │  Protocol   │    │   Buffer     │    │ (Gemini/OpenAI/etc) │ │
│  │  (JSON)     │    │   Manager    │    └─────────────────────┘ │
│  └─────────────┘    └──────────────┘                            │
└─────────────────────────────────────────────────────────────────┘
```

### 组件职责

| 组件 | 职责 |
|-----|------|
| WebSocket Server | 处理 WebSocket 连接、认证、消息路由 |
| Session Manager | 管理会话生命周期、配置、状态 |
| Event Protocol | 事件序列化/反序列化、类型分发 |
| Audio Buffer | 音频数据缓冲、格式转换 |
| Pipeline | 可配置的 AI 处理流水线 |

## 3. 协议设计

### 3.1 消息格式

所有消息使用 JSON 格式：

```json
{
    "event_id": "evt_001",  // 可选，客户端生成的唯一ID
    "type": "event.type",   // 必需，事件类型
    // ... 事件特定字段
}
```

### 3.2 客户端事件 (Client Events)

| 事件类型 | 描述 |
|---------|------|
| `session.update` | 更新会话配置 |
| `input_audio_buffer.append` | 追加音频数据 |
| `input_audio_buffer.commit` | 提交音频缓冲区 |
| `input_audio_buffer.clear` | 清空音频缓冲区 |
| `conversation.item.create` | 创建对话项 |
| `conversation.item.truncate` | 截断对话项 |
| `conversation.item.delete` | 删除对话项 |
| `response.create` | 触发 AI 响应 |
| `response.cancel` | 取消当前响应 |

#### session.update

```json
{
    "type": "session.update",
    "session": {
        "modalities": ["text", "audio"],
        "model": "gemini-2.0-flash",
        "voice": "alloy",
        "input_audio_format": "pcm16",
        "output_audio_format": "pcm16",
        "input_audio_transcription": {
            "model": "whisper-1"
        },
        "turn_detection": {
            "type": "server_vad",
            "threshold": 0.5,
            "prefix_padding_ms": 300,
            "silence_duration_ms": 500
        },
        "instructions": "You are a helpful assistant.",
        "temperature": 0.8,
        "max_output_tokens": 4096
    }
}
```

#### input_audio_buffer.append

```json
{
    "type": "input_audio_buffer.append",
    "audio": "BASE64_ENCODED_AUDIO_DATA"
}
```

#### response.create

```json
{
    "type": "response.create",
    "response": {
        "modalities": ["text", "audio"],
        "instructions": "Please respond briefly."
    }
}
```

### 3.3 服务端事件 (Server Events)

| 事件类型 | 描述 |
|---------|------|
| `error` | 错误信息 |
| `session.created` | 会话已创建 |
| `session.updated` | 会话配置已更新 |
| `input_audio_buffer.committed` | 音频缓冲区已提交 |
| `input_audio_buffer.cleared` | 音频缓冲区已清空 |
| `input_audio_buffer.speech_started` | 检测到语音开始 (VAD) |
| `input_audio_buffer.speech_stopped` | 检测到语音结束 (VAD) |
| `conversation.item.created` | 对话项已创建 |
| `conversation.item.input_audio_transcription.completed` | 输入音频转录完成 |
| `response.created` | 响应已创建 |
| `response.output_item.added` | 输出项已添加 |
| `response.content_part.added` | 内容部分已添加 |
| `response.audio.delta` | 音频增量数据 |
| `response.audio.done` | 音频输出完成 |
| `response.audio_transcript.delta` | 音频转录增量 |
| `response.audio_transcript.done` | 音频转录完成 |
| `response.text.delta` | 文本增量 |
| `response.text.done` | 文本输出完成 |
| `response.done` | 响应完成 |
| `rate_limits.updated` | 速率限制更新 |

#### session.created

```json
{
    "type": "session.created",
    "event_id": "evt_abc123",
    "session": {
        "id": "sess_001",
        "object": "realtime.session",
        "model": "gemini-2.0-flash",
        "modalities": ["text", "audio"],
        "voice": "alloy",
        "input_audio_format": "pcm16",
        "output_audio_format": "pcm16",
        "turn_detection": {
            "type": "server_vad",
            "threshold": 0.5,
            "silence_duration_ms": 500
        },
        "max_output_tokens": 4096
    }
}
```

#### response.audio.delta

```json
{
    "type": "response.audio.delta",
    "event_id": "evt_audio_001",
    "response_id": "resp_001",
    "item_id": "item_001",
    "output_index": 0,
    "content_index": 0,
    "delta": "BASE64_ENCODED_AUDIO_CHUNK"
}
```

#### input_audio_buffer.speech_started

```json
{
    "type": "input_audio_buffer.speech_started",
    "event_id": "evt_vad_001",
    "audio_start_ms": 1500,
    "item_id": "item_002"
}
```

#### error

```json
{
    "type": "error",
    "event_id": "evt_err_001",
    "error": {
        "type": "invalid_request_error",
        "code": "invalid_audio_format",
        "message": "Audio format not supported",
        "param": "input_audio_format"
    }
}
```

## 4. 连接流程

```
                    Client                                     Server
                      │                                          │
                      │──── WebSocket Connect ───────────────────▶│
                      │     GET /v1/realtime?model=gemini-2.0    │
                      │     Authorization: Bearer <token>        │
                      │                                          │
                      │◀──── session.created ────────────────────│
                      │      {session: {id, model, ...}}         │
                      │                                          │
                      │──── session.update ──────────────────────▶│
                      │     {session: {voice, turn_detection}}   │
                      │                                          │
                      │◀──── session.updated ────────────────────│
                      │                                          │
                      │──── input_audio_buffer.append ───────────▶│  ┐
                      │     {audio: "base64..."}                 │  │ 重复
                      │                                          │  │ 发送
                      │◀──── input_audio_buffer.speech_started ──│  │ 音频
                      │                                          │  ┘
                      │◀──── response.created ───────────────────│
                      │                                          │
                      │◀──── response.audio.delta ───────────────│  ┐
                      │      {delta: "base64..."}                │  │ 流式
                      │                                          │  │ 返回
                      │◀──── response.audio_transcript.delta ────│  │ 音频
                      │      {delta: "Hello..."}                 │  ┘
                      │                                          │
                      │◀──── response.audio.done ────────────────│
                      │◀──── response.done ──────────────────────│
                      │                                          │
```

## 5. 目录结构

```
pkg/realtimeapi/
├── server.go           # WebSocket 服务器
├── session.go          # 会话管理
├── handler.go          # 事件处理器
├── events/
│   ├── client.go       # 客户端事件定义
│   ├── server.go       # 服务端事件定义
│   └── types.go        # 共享类型
├── audio_buffer.go     # 音频缓冲区管理
├── conversation.go     # 对话管理
└── response.go         # 响应生成
```

## 6. 数据类型定义

### Session

```go
type Session struct {
    ID                       string           `json:"id"`
    Object                   string           `json:"object"` // "realtime.session"
    Model                    string           `json:"model"`
    Modalities               []Modality       `json:"modalities"`
    Voice                    string           `json:"voice,omitempty"`
    Instructions             string           `json:"instructions,omitempty"`
    InputAudioFormat         AudioFormat      `json:"input_audio_format"`
    OutputAudioFormat        AudioFormat      `json:"output_audio_format"`
    InputAudioTranscription  *Transcription   `json:"input_audio_transcription,omitempty"`
    TurnDetection            *TurnDetection   `json:"turn_detection,omitempty"`
    Tools                    []Tool           `json:"tools,omitempty"`
    ToolChoice               string           `json:"tool_choice,omitempty"`
    Temperature              float64          `json:"temperature,omitempty"`
    MaxOutputTokens          int              `json:"max_output_tokens,omitempty"`
}
```

### Conversation Item

```go
type ConversationItem struct {
    ID        string    `json:"id"`
    Object    string    `json:"object"` // "realtime.item"
    Type      ItemType  `json:"type"`   // message, function_call, function_call_output
    Status    Status    `json:"status"` // in_progress, completed, incomplete
    Role      Role      `json:"role"`   // user, assistant, system
    Content   []Content `json:"content"`
}

type Content struct {
    Type       ContentType `json:"type"`       // input_text, input_audio, text, audio
    Text       string      `json:"text,omitempty"`
    Audio      string      `json:"audio,omitempty"`      // Base64
    Transcript string      `json:"transcript,omitempty"`
}
```

### Response

```go
type Response struct {
    ID            string             `json:"id"`
    Object        string             `json:"object"` // "realtime.response"
    Status        ResponseStatus     `json:"status"` // in_progress, completed, cancelled, failed
    StatusDetails *ResponseStatus    `json:"status_details,omitempty"`
    Output        []ConversationItem `json:"output"`
    Usage         *Usage             `json:"usage,omitempty"`
}

type Usage struct {
    TotalTokens       int `json:"total_tokens"`
    InputTokens       int `json:"input_tokens"`
    OutputTokens      int `json:"output_tokens"`
    InputTokenDetails struct {
        CachedTokens int `json:"cached_tokens"`
        TextTokens   int `json:"text_tokens"`
        AudioTokens  int `json:"audio_tokens"`
    } `json:"input_token_details"`
    OutputTokenDetails struct {
        TextTokens  int `json:"text_tokens"`
        AudioTokens int `json:"audio_tokens"`
    } `json:"output_token_details"`
}
```

## 7. 音频格式

### 支持的输入格式

| 格式 | 描述 | 采样率 | 位深 |
|-----|------|-------|-----|
| `pcm16` | 16-bit PCM, little-endian | 24000 Hz | 16-bit |
| `g711_ulaw` | G.711 μ-law | 8000 Hz | 8-bit |
| `g711_alaw` | G.711 A-law | 8000 Hz | 8-bit |

### 支持的输出格式

| 格式 | 描述 | 采样率 | 位深 |
|-----|------|-------|-----|
| `pcm16` | 16-bit PCM, little-endian | 24000 Hz | 16-bit |

## 8. VAD (Voice Activity Detection)

服务端 VAD 使用 Silero VAD 模型自动检测语音活动。

### 配置

```json
{
    "turn_detection": {
        "type": "server_vad",
        "threshold": 0.5,
        "prefix_padding_ms": 300,
        "silence_duration_ms": 500,
        "create_response": true
    }
}
```

| 参数 | 描述 | 默认值 |
|-----|------|-------|
| `type` | `server_vad` 或 `none` | `server_vad` |
| `threshold` | VAD 激活阈值 (0.0-1.0) | 0.5 |
| `prefix_padding_ms` | 语音开始前保留的毫秒数 | 300 |
| `silence_duration_ms` | 静音多久后认为语音结束 | 500 |
| `create_response` | 语音结束后是否自动创建响应 | true |

## 9. 错误处理

### 错误类型

| 类型 | 描述 |
|-----|------|
| `invalid_request_error` | 请求格式错误 |
| `authentication_error` | 认证失败 |
| `rate_limit_error` | 超出速率限制 |
| `server_error` | 服务器内部错误 |
| `session_error` | 会话相关错误 |

### 错误码

| 错误码 | 描述 |
|-------|------|
| `invalid_event_type` | 无效的事件类型 |
| `invalid_session_config` | 无效的会话配置 |
| `invalid_audio_format` | 不支持的音频格式 |
| `audio_buffer_overflow` | 音频缓冲区溢出 |
| `session_expired` | 会话已过期 |
| `model_not_available` | 模型不可用 |

## 10. 与现有 Pipeline 的集成

### Pipeline 工厂模式

```go
type PipelineFactory func(session *Session) (*pipeline.Pipeline, error)

// Gemini Pipeline 工厂
func GeminiPipelineFactory(session *Session) (*pipeline.Pipeline, error) {
    p := pipeline.NewPipeline(fmt.Sprintf("realtime-api-%s", session.ID))

    resample := elements.NewAudioResampleElement(24000, 16000, 1, 1)
    gemini := elements.NewGeminiElement()
    pacer := elements.NewAudioPacerSinkElement()

    p.AddElements([]pipeline.Element{resample, gemini, pacer})
    p.Link(resample, gemini)
    p.Link(gemini, pacer)

    return p, nil
}
```

### Bus 事件映射

| Pipeline Event | API Event |
|---------------|-----------|
| `EventInterrupted` | `input_audio_buffer.speech_started` |
| `EventVADSpeechEnd` | `input_audio_buffer.speech_stopped` |
| `EventPartialResult` | `response.audio.delta` / `response.text.delta` |
| `EventFinalResult` | `response.done` |

## 11. 使用示例

### 服务端

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/realtime-ai/realtime-ai/pkg/server"
)

func main() {
    config := server.DefaultWebSocketRealtimeConfig()
    config.Addr = ":8080"
    config.Path = "/v1/realtime"
    config.AuthToken = os.Getenv("API_TOKEN")
    config.DefaultModel = "gemini-2.0-flash"
    config.AllowedModels = []string{"gemini-2.0-flash", "gpt-4o-realtime"}

    srv := server.NewWebSocketRealtimeServer(config)

    // Set pipeline factory for AI processing
    srv.SetPipelineFactory(createPipeline)

    ctx := context.Background()
    if err := srv.Start(ctx); err != nil {
        log.Fatal(err)
    }
}
```

### JavaScript 客户端

```javascript
const ws = new WebSocket('wss://api.example.com/v1/realtime?model=gemini-2.0-flash', {
    headers: { 'Authorization': 'Bearer YOUR_TOKEN' }
});

ws.onopen = () => {
    // 配置会话
    ws.send(JSON.stringify({
        type: 'session.update',
        session: {
            modalities: ['text', 'audio'],
            voice: 'alloy',
            turn_detection: {
                type: 'server_vad',
                threshold: 0.5,
                silence_duration_ms: 500
            }
        }
    }));
};

ws.onmessage = (event) => {
    const data = JSON.parse(event.data);

    switch (data.type) {
        case 'session.created':
            console.log('Session created:', data.session.id);
            break;
        case 'response.audio.delta':
            playAudio(base64ToArrayBuffer(data.delta));
            break;
        case 'response.audio_transcript.delta':
            displayTranscript(data.delta);
            break;
        case 'error':
            console.error('Error:', data.error.message);
            break;
    }
};

// 发送音频
function sendAudio(audioData) {
    ws.send(JSON.stringify({
        type: 'input_audio_buffer.append',
        audio: arrayBufferToBase64(audioData)
    }));
}
```

### Go 客户端

```go
package main

import (
    "github.com/gorilla/websocket"
    "github.com/realtime-ai/realtime-ai/pkg/realtimeapi/events"
)

func main() {
    header := http.Header{}
    header.Add("Authorization", "Bearer YOUR_TOKEN")

    conn, _, err := websocket.DefaultDialer.Dial(
        "wss://api.example.com/v1/realtime?model=gemini-2.0-flash",
        header,
    )
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    // 读取 session.created
    _, msg, _ := conn.ReadMessage()
    var created events.SessionCreatedEvent
    json.Unmarshal(msg, &created)
    fmt.Println("Session ID:", created.Session.ID)

    // 更新会话
    conn.WriteJSON(events.SessionUpdateEvent{
        Event: events.Event{Type: "session.update"},
        Session: events.SessionConfig{
            Modalities: []string{"text", "audio"},
            Voice:      "alloy",
        },
    })

    // 发送音频...
}
```

## 12. 实现计划

### 阶段 1: 核心协议
- [x] 设计文档
- [ ] 事件类型定义 (`pkg/realtimeapi/events/`)
- [ ] WebSocket 服务器 (`pkg/realtimeapi/server.go`)
- [ ] 会话管理 (`pkg/realtimeapi/session.go`)
- [ ] 音频缓冲区 (`pkg/realtimeapi/audio_buffer.go`)

### 阶段 2: Pipeline 集成
- [ ] Pipeline 工厂模式
- [ ] 事件处理器（Pipeline Bus → API Events）
- [ ] VAD 集成

### 阶段 3: 高级功能
- [ ] 对话历史管理
- [ ] Function Calling 支持
- [ ] 多模型支持
- [ ] 速率限制

### 阶段 4: 生产就绪
- [ ] 认证和授权
- [ ] 分布式会话管理
- [ ] OpenTelemetry 追踪集成
- [ ] 客户端 SDK

## 13. 参考资料

- [OpenAI Realtime API Reference](https://platform.openai.com/docs/api-reference/realtime)
- [WebSocket Protocol RFC 6455](https://tools.ietf.org/html/rfc6455)
- [realtime-ai Pipeline Architecture](./grpc-architecture.md)
