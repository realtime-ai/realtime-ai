# Realtime API 同声传译设计方案

## 架构对比

### 传统方案 (当前实现)
```
Microphone → STT (3s) → Translation (2s) → TTS (2s) → Speaker
总延迟: 7秒
成本: $0.022/min
```

### Realtime API 方案 (新设计)
```
Microphone → Realtime API (1-2s) → Speaker
总延迟: 1-2秒 (降低 70-80%)
成本: ~$0.015/min (降低 30%)
```

## 技术选项

### 选项 1: Gemini Live API (推荐)

**优势**:
- ✅ 原生支持音频输入输出
- ✅ 内置 VAD
- ✅ 支持多模态 (音频+文本)
- ✅ 低延迟 (1-2秒)
- ✅ 支持中文等多语言
- ✅ 成本较低

**限制**:
- ⚠️ 需要通过 System Instructions 实现翻译
- ⚠️ 输出音频采样率固定 (24kHz)

**实现方式**:
```go
config := elements.GeminiLiveConfig{
    Model: "gemini-2.5-flash-native-audio-preview-12-2025",
    SystemInstruction: `You are a professional simultaneous interpreter.
Your task is to:
1. Listen to speech in {sourceLang}
2. Translate it to {targetLang}
3. Speak the translation naturally
4. Preserve the tone and meaning
5. Be concise and clear`,
}
gemini := elements.NewGeminiLiveElementWithConfig(config)
```

### 选项 2: OpenAI Realtime API

**优势**:
- ✅ 原生支持音频对话
- ✅ 内置 VAD
- ✅ 低延迟
- ✅ 高质量音频输出

**限制**:
- ⚠️ 需要通过 instructions 实现翻译
- ⚠️ 成本较高 (~$100/hour 输入 + ~$200/hour 输出)
- ⚠️ 输出采样率固定 (24kHz)

### 选项 3: 混合方案 (最佳性能)

对于不同语言对使用不同方案:
- 中英翻译: Gemini Live (更便宜，质量好)
- 其他语言: OpenAI Realtime (支持更多语言)

## 推荐架构: Gemini Live

### Pipeline 设计

```
┌────────────────────────────────────────────────────────────┐
│           REALTIME API 同声传译 PIPELINE                     │
├────────────────────────────────────────────────────────────┤
│                                                              │
│  Microphone (48kHz Opus)                                    │
│      ↓                                                       │
│  [1] Opus Decode Element                                   │
│      ↓                                                       │
│  [2] Audio Resample (48kHz → 16kHz)                       │
│      ↓                                                       │
│  ╔═══════════════════════════════════════════════════╗      │
│  ║  [3] Gemini Live Element                         ║      │
│  ║      - Input: 16kHz PCM audio                    ║      │
│  ║      - System: Translation instructions          ║      │
│  ║      - Output: 24kHz PCM audio                   ║      │
│  ║      - Latency: ~1-2s                            ║      │
│  ╚═══════════════════════════════════════════════════╝      │
│      ↓                                                       │
│  [4] Audio Resample (24kHz → 48kHz)                       │
│      ↓                                                       │
│  [5] Audio Pacer (200ms buffer for smooth playback)       │
│      ↓                                                       │
│  [6] Opus Encode Element                                   │
│      ↓                                                       │
│  Speaker (48kHz Opus)                                       │
│                                                              │
└────────────────────────────────────────────────────────────┘
```

### 关键改进

1. **元素数量**: 7 → 6 (减少 1 个)
2. **延迟**: 7s → 1-2s (降低 70-80%)
3. **成本**: $0.022/min → ~$0.015/min (降低 30%)
4. **代码复杂度**: 大幅简化

## System Instructions 设计

### 基础翻译指令

```go
func buildTranslationInstruction(sourceLang, targetLang string) string {
    return fmt.Sprintf(`You are a professional simultaneous interpreter.

TASK:
- Listen to speech in %s
- Immediately translate to %s while preserving:
  * Original meaning and context
  * Speaker's tone and emotion
  * Natural flow and pacing

RULES:
1. Start speaking as soon as you understand a complete thought
2. Keep translations concise but complete
3. Use natural, conversational language
4. Don't add explanations or meta-comments
5. If you hear silence, wait for more input

QUALITY STANDARDS:
- Accuracy: Translate precisely
- Fluency: Sound natural in %s
- Latency: Minimize delay between input and output
- Tone: Match the speaker's sentiment

Begin interpretation now.`, sourceLang, targetLang, targetLang)
}
```

### 领域专业化指令

```go
// 商务会议
businessInstruction := `... (基础指令) ...

DOMAIN: Business Meeting
- Use formal, professional language
- Preserve technical terms when appropriate
- Maintain business etiquette
`

// 日常对话
casualInstruction := `... (基础指令) ...

DOMAIN: Casual Conversation
- Use relaxed, natural language
- Include colloquialisms when suitable
- Keep it conversational
`

// 技术讨论
technicalInstruction := `... (基础指令) ...

DOMAIN: Technical Discussion
- Preserve technical terminology
- Maintain precision over brevity
- Use industry-standard translations
`
```

## 配置选项

### 环境变量

```env
# API Provider Selection
INTERPRETATION_PROVIDER=gemini  # Options: gemini, openai, hybrid

# Gemini Configuration
GOOGLE_API_KEY=your-key
GEMINI_MODEL=gemini-2.5-flash-native-audio-preview-12-2025

# OpenAI Configuration (if using OpenAI)
OPENAI_API_KEY=your-key
OPENAI_MODEL=gpt-4o-realtime-preview

# Language Configuration
SOURCE_LANG=Chinese
TARGET_LANG=English

# Domain/Context
INTERPRETATION_DOMAIN=casual  # casual, business, technical

# Audio Configuration
ENABLE_SUBTITLES=true
AUDIO_BUFFER_MS=200

# Performance
ENABLE_METRICS=true
LOG_LATENCY=true
```

## 代码实现

### main.go 结构

```go
func createRealtimeInterpretationPipeline(
    session *realtimeapi.Session,
    provider string,
    sourceLang, targetLang string,
    domain string,
) (*pipeline.Pipeline, error) {
    p := pipeline.NewPipeline("realtime-interpretation")

    // 1. Opus Decode (WebRTC input)
    opusDecode := elements.NewOpusDecodeElement(48000, 1)
    p.AddElement(opusDecode)

    // 2. Resample to 16kHz for Gemini
    resample16k := elements.NewAudioResampleElement(48000, 16000, 1, 1)
    p.AddElement(resample16k)

    // 3. Realtime API Element (CORE)
    var realtimeElement pipeline.Element
    switch provider {
    case "gemini":
        instruction := buildGeminiInstruction(sourceLang, targetLang, domain)
        geminiConfig := elements.GeminiLiveConfig{
            Model:  "gemini-2.5-flash-native-audio-preview-12-2025",
            APIKey: os.Getenv("GOOGLE_API_KEY"),
            SystemInstruction: instruction,
        }
        realtimeElement = elements.NewGeminiLiveElementWithConfig(geminiConfig)

    case "openai":
        // OpenAI Realtime API 配置
        realtimeElement = elements.NewOpenAIRealtimeAPIElement()
        // Configure with instructions
    }
    p.AddElement(realtimeElement)

    // 4. Resample to 48kHz for WebRTC
    resample48k := elements.NewAudioResampleElement(24000, 48000, 1, 1)
    p.AddElement(resample48k)

    // 5. Audio Pacer (CRITICAL for smooth playback)
    audioPacer := elements.NewAudioPacerSinkElementWithConfig(
        elements.AudioPacerSinkConfig{
            SampleRate: 48000,
            Channels:   1,
        },
    )
    p.AddElement(audioPacer)

    // 6. Opus Encode (WebRTC output)
    opusEncode := elements.NewOpusEncodeElement(960, 48000, 1)
    p.AddElement(opusEncode)

    // Link pipeline
    p.Link(opusDecode, resample16k)
    p.Link(resample16k, realtimeElement)
    p.Link(realtimeElement, resample48k)
    p.Link(resample48k, audioPacer)
    p.Link(audioPacer, opusEncode)

    return p, nil
}
```

### Gemini System Instruction 构建器

```go
func buildGeminiInstruction(sourceLang, targetLang, domain string) string {
    base := fmt.Sprintf(`You are a professional simultaneous interpreter.

Your task is to listen to speech in %s and immediately translate it to %s.

CRITICAL RULES:
1. Translate ONLY - do not add commentary
2. Speak naturally in %s as if you are the original speaker
3. Start translating as soon as you understand a complete thought
4. Preserve emotion, tone, and intent
5. Be concise but complete
6. If unclear, make your best interpretation

`, sourceLang, targetLang, targetLang)

    // Add domain-specific instructions
    switch domain {
    case "business":
        base += `CONTEXT: Business Meeting
- Use formal, professional language
- Preserve technical business terms
- Maintain professional tone
`
    case "technical":
        base += `CONTEXT: Technical Discussion
- Preserve technical terminology
- Use precise translations
- Maintain technical accuracy
`
    default: // casual
        base += `CONTEXT: Casual Conversation
- Use natural, relaxed language
- Include appropriate colloquialisms
- Keep it conversational
`
    }

    base += "\nBegin interpretation immediately when you hear speech."
    return base
}
```

### Session Integration with Realtime API

```go
func main() {
    // Create WebRTC Realtime Server
    config := server.DefaultWebRTCRealtimeConfig()
    config.RTCUDPPort = 9000
    config.ICELite = false

    srv := server.NewWebRTCRealtimeServer(config)

    // Set pipeline factory
    srv.SetPipelineFactory(func(ctx context.Context, session *realtimeapi.Session) (*pipeline.Pipeline, error) {
        provider := getEnv("INTERPRETATION_PROVIDER", "gemini")
        sourceLang := getEnv("SOURCE_LANG", "Chinese")
        targetLang := getEnv("TARGET_LANG", "English")
        domain := getEnv("INTERPRETATION_DOMAIN", "casual")

        return createRealtimeInterpretationPipeline(
            session,
            provider,
            sourceLang,
            targetLang,
            domain,
        )
    })

    // Start server
    if err := srv.Start(); err != nil {
        log.Fatalf("Failed to start server: %v", err)
    }

    // HTTP handlers
    http.HandleFunc("/session", srv.HandleNegotiate)
    http.Handle("/", http.FileServer(http.Dir("static")))

    log.Println("Realtime Interpretation Server started")
    log.Println("Open http://localhost:8080")

    http.ListenAndServe(":8080", nil)
}
```

## 性能优化

### 1. Audio Pacer 配置

```go
audioPacer := elements.NewAudioPacerSinkElementWithConfig(
    elements.AudioPacerSinkConfig{
        SampleRate: 48000,
        Channels:   1,
        // 关键: 200ms 缓冲确保平滑播放
        // Gemini 输出可能不均匀，需要平滑
    },
)
```

### 2. 监控和指标

```go
import "github.com/realtime-ai/realtime-ai/pkg/trace"

// 在 pipeline 创建时
ctx, span := trace.InstrumentPipelineStart(ctx, "realtime-interpretation")
defer span.End()

// 监控 Gemini 延迟
ctx, geminiSpan := trace.InstrumentElementStart(ctx, "gemini-live")
// ... Gemini processing ...
geminiSpan.SetAttributes(
    attribute.String("source_lang", sourceLang),
    attribute.String("target_lang", targetLang),
)
geminiSpan.End()
```

### 3. 错误恢复

```go
// 在 Gemini Element 中添加重连逻辑
type ResilientGeminiElement struct {
    *GeminiLiveElement
    reconnectAttempts int
    maxReconnects     int
}

func (e *ResilientGeminiElement) handleError(err error) {
    if e.reconnectAttempts < e.maxReconnects {
        log.Printf("Gemini connection lost, reconnecting... (attempt %d/%d)",
            e.reconnectAttempts+1, e.maxReconnects)

        time.Sleep(time.Second * time.Duration(1<<e.reconnectAttempts))
        e.reconnectAttempts++

        // Reconnect logic
        e.reconnect()
    }
}
```

## UI 更新

### 新功能

1. **Provider 选择器**
   - Gemini Live
   - OpenAI Realtime
   - Hybrid (自动选择)

2. **领域选择**
   - 日常对话
   - 商务会议
   - 技术讨论

3. **实时指标**
   - 当前延迟
   - 音频质量
   - API 响应时间

4. **现代设计**
   - 参考 gemini-assis 的渐变背景
   - 平滑动画
   - 实时波形显示

## 测试计划

### 延迟测试

```go
func TestRealtimeAPILatency(t *testing.T) {
    // 测试端到端延迟
    start := time.Now()

    // 发送音频
    pipeline.Push(audioMessage)

    // 接收翻译音频
    response := pipeline.Pull()

    latency := time.Since(start)
    assert.Less(t, latency, 2*time.Second)
}
```

### 质量测试

- 翻译准确性
- 音频流畅度
- 错误恢复
- 长时间运行稳定性

## 迁移路径

### Phase 1: 基础实现 (2-3小时)
1. 创建 Realtime API pipeline
2. 配置 Gemini Live
3. 添加 AudioPacer
4. 基础测试

### Phase 2: 优化 (2-3小时)
1. System instructions 优化
2. 错误处理
3. 性能监控
4. UI 更新

### Phase 3: 生产化 (2-3小时)
1. 多 provider 支持
2. 混合模式
3. 自适应质量
4. 完整测试

## 预期结果

| 指标 | 当前 | Realtime API | 改进 |
|------|------|--------------|------|
| 延迟 | 4-7s | 1-2s | **70-80%** |
| 成本 | $0.022/min | $0.015/min | **30%** |
| 代码复杂度 | 高 | 低 | **50%** |
| 音频质量 | 卡顿 | 流畅 | **显著** |
| 维护成本 | 高 | 低 | **60%** |

## 总结

使用 Realtime API 方案将带来:

✅ **70-80% 延迟降低** - 从 7秒降至 1-2秒
✅ **30% 成本降低** - 减少 API 调用次数
✅ **50% 代码简化** - 7 个元素减至 6 个
✅ **更好的用户体验** - 流畅的音频播放
✅ **更易维护** - 更少的组件和配置

**推荐立即实施此方案!**
