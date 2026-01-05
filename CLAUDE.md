# CLAUDE.md

Claude Code 开发指南。

## 项目概述

Realtime AI 是一个实时 AI 框架，采用 **GStreamer 风格的 Pipeline 架构** 处理音视频流。

**传输协议**: WebRTC (浏览器) | gRPC (服务端) | WebSocket

**核心组件**: Pipeline → Elements → Connection → Server

## 常用命令

```bash
# 依赖安装 (macOS)
brew install opus ffmpeg go
go mod download

# 依赖安装 (Linux) - 推荐使用预编译 FFmpeg
./scripts/setup-ffmpeg.sh
eval "$(./scripts/setup-ffmpeg.sh --env)"
sudo apt-get install pkg-config libopus-dev

# 构建
go build ./...                    # 标准构建
go build -tags vad ./...          # 启用 VAD

# 运行示例
go run examples/gemini-assis/main.go                    # Gemini 助手
go run examples/simultaneous-interpretation-gemini/main.go  # 同声传译 (推荐)
go run examples/grpc-assis/server/main.go               # gRPC 服务端

# 测试
go test ./...
go test -tags vad ./pkg/elements  # VAD 测试

# 环境变量
export GOOGLE_API_KEY=xxx
export OPENAI_API_KEY=xxx
```

## 架构

### Pipeline 系统

```
Pipeline → Element1 → Element2 → Element3 → Output
              ↓          ↓          ↓
           AudioData  TextData   AudioData
```

- **Pipeline**: 管理 Elements 连接和生命周期
- **Element**: 处理单元 (STT/LLM/TTS/Codec 等)
- **Bus**: 跨 Element 事件通信
- **Message**: 包含 AudioData/VideoData/TextData

### 关键 Elements

| 类型 | Element | 说明 |
|------|---------|------|
| LLM | GeminiLiveElement | Gemini 多模态 |
| STT | WhisperSTTElement | OpenAI Whisper |
| TTS | UniversalTTSElement | 通用 TTS |
| Audio | AudioResampleElement | 采样率转换 |
| Audio | AudioPacerSinkElement | 音频平滑输出 |
| VAD | SileroVADElement | 语音活动检测 |

### 连接系统

- `RTCConnection`: WebRTC (浏览器)
- `GRPCConnection`: gRPC (服务端)
- `LocalConnection`: 本地测试

## 编码规范

### 文件头部注释 (必需)

每个新代码文件必须在头部添加功能说明，功能变更时及时更新：

```go
// Package elements provides pipeline processing elements.
//
// AudioResampleElement 实现音频采样率转换。
// 支持任意采样率之间的转换，使用 libsamplerate 实现高质量重采样。
//
// 主要功能:
//   - 输入/输出采样率配置
//   - 单声道/立体声支持
//   - 实时流式处理
//
// 使用示例:
//   resample := NewAudioResampleElement(48000, 16000, 1, 1)
package elements
```

### Element 开发规范

1. **必须** 使用 `BaseElement` 作为基础结构
2. **必须** 实现 `Start(ctx)` 和 `Stop()` 方法
3. 使用 `context.WithCancel` 管理生命周期
4. goroutine 使用 `wg.Add(1)` / `wg.Done()` / `wg.Wait()` 模式

```go
func (e *MyElement) Start(ctx context.Context) error {
    e.ctx, e.cancel = context.WithCancel(ctx)
    e.wg.Add(1)
    go func() {
        defer e.wg.Done()
        for {
            select {
            case <-e.ctx.Done():
                return
            case msg := <-e.In():
                // 处理消息
                e.Out() <- result
            }
        }
    }()
    return nil
}

func (e *MyElement) Stop() error {
    e.cancel()
    e.wg.Wait()
    return nil
}
```

### 通用规范

- 优先编辑现有文件，避免创建新文件
- 不主动创建文档文件 (README.md 等)
- 保持代码简洁，避免过度工程

## 关键包

| 包 | 说明 |
|----|------|
| `pkg/pipeline` | Pipeline/Element/Bus 核心 |
| `pkg/elements` | 所有处理 Elements |
| `pkg/connection` | 连接抽象 (RTC/gRPC/WS) |
| `pkg/server` | HTTP/WebRTC/gRPC 服务器 |
| `pkg/asr` | ASR Provider 接口 |
| `pkg/tts` | TTS Provider 接口 |
| `pkg/audio` | 音频工具 |
| `pkg/trace` | OpenTelemetry 追踪 |

## 测试资源

### 测试音频文件 (`tests/audiofiles/`)

| 文件 | 格式 | 说明 |
|------|------|------|
| `vad_test_en.wav` | 16kHz mono | VAD/ASR/同传测试用英语语音 |
| `test_speech.wav` | 16kHz mono | 快速测试用短语音 |

生成测试音频：
```bash
# 生成 5 秒 440Hz 正弦波测试音频
ffmpeg -f lavfi -i "sine=frequency=440:duration=5" -ar 16000 -ac 1 tests/audiofiles/test.wav
```

## 文档索引

- `docs/grpc-architecture.md` - gRPC 架构
- `docs/tracing.md` - 分布式追踪
- `pkg/asr/README.md` - ASR 接口
- `pkg/tts/README.md` - TTS 接口
- `pkg/elements/VAD_README.md` - VAD 配置
- `tests/audiofiles/README.md` - 测试音频说明
