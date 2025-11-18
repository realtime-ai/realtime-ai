# Tracing System Design Document

## 概述 (Overview)

本文档详细说明了为 realtime-ai 项目设计和实现的分布式追踪（Distributed Tracing）系统。该系统基于 OpenTelemetry，提供全面的可观测性能力。

This document provides a detailed explanation of the distributed tracing system designed and implemented for the realtime-ai project. The system is based on OpenTelemetry and provides comprehensive observability capabilities.

## 设计目标 (Design Goals)

1. **易于集成** - 最小化代码侵入，提供开箱即用的体验
2. **灵活配置** - 支持多种导出器和环境配置
3. **全面覆盖** - 覆盖所有关键组件：Pipeline、Element、Connection、AI Services
4. **高性能** - 最小化性能开销，支持可配置的采样
5. **生产就绪** - 与主流追踪后端集成（Jaeger、Zipkin、云服务商）

1. **Easy Integration** - Minimal code intrusion with out-of-the-box experience
2. **Flexible Configuration** - Support for multiple exporters and environment configurations
3. **Comprehensive Coverage** - Cover all key components: Pipeline, Element, Connection, AI Services
4. **High Performance** - Minimize performance overhead with configurable sampling
5. **Production Ready** - Integration with mainstream tracing backends (Jaeger, Zipkin, cloud providers)

## 架构设计 (Architecture Design)

### 组件结构 (Component Structure)

```
pkg/trace/
├── trace.go          # 核心追踪初始化和生命周期管理
├── attributes.go     # 预定义的属性键和辅助函数
├── helpers.go        # 通用的 Span 管理工具
├── pipeline.go       # Pipeline 专用的追踪辅助函数
├── connection.go     # Connection 追踪辅助函数
├── ai.go            # AI 服务追踪辅助函数
└── README.md        # 包文档
```

### 核心模块 (Core Modules)

#### 1. 追踪初始化模块 (Trace Initialization Module)

**文件**: `trace.go`

**功能**:
- 全局 TracerProvider 的初始化和管理
- 支持多种导出器（stdout, OTLP, none）
- 配置管理和环境变量支持
- 优雅的关闭和资源清理

**主要接口**:
```go
// 初始化追踪系统
func Initialize(ctx context.Context, cfg *Config) error

// 关闭追踪系统
func Shutdown(ctx context.Context) error

// 获取全局 Tracer
func GetTracer() trace.Tracer

// 启动新的 Span
func StartSpan(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span)
```

**配置选项**:
```go
type Config struct {
    ServiceName    string  // 服务名称
    ServiceVersion string  // 服务版本
    Environment    string  // 部署环境
    ExporterType   string  // 导出器类型: stdout, otlp, none
    OTLPEndpoint   string  // OTLP 端点地址
    SamplingRate   float64 // 采样率 (0.0-1.0)
}
```

#### 2. 属性管理模块 (Attributes Module)

**文件**: `attributes.go`

**功能**:
- 定义标准化的属性键
- 提供常用属性集的辅助函数
- 确保追踪数据的一致性

**属性分类**:

1. **Pipeline 属性**
   - `pipeline.name`: Pipeline 名称
   - `pipeline.element`: Element 名称
   - `session.id`: 会话 ID
   - `message.type`: 消息类型

2. **音频属性**
   - `audio.sample_rate`: 采样率
   - `audio.channels`: 声道数
   - `audio.media_type`: 媒体类型
   - `audio.codec`: 编解码器
   - `audio.data_size`: 数据大小

3. **视频属性**
   - `video.width`: 宽度
   - `video.height`: 高度
   - `video.media_type`: 媒体类型
   - `video.codec`: 编解码器
   - `video.data_size`: 数据大小

4. **连接属性**
   - `connection.id`: 连接 ID
   - `connection.type`: 连接类型
   - `connection.state`: 连接状态

5. **AI 服务属性**
   - `llm.provider`: LLM 提供商
   - `llm.model`: 模型名称
   - `stt.provider`: STT 提供商
   - `tts.provider`: TTS 提供商

#### 3. 辅助工具模块 (Helper Utilities Module)

**文件**: `helpers.go`

**功能**:
- 简化 Span 的创建和管理
- 错误记录和状态设置
- 事件和属性添加
- 追踪 ID 提取

**主要函数**:
```go
// 在 Span 内执行函数
func WithSpan(ctx context.Context, spanName string, fn func(context.Context) error) error

// 记录错误
func RecordError(span trace.Span, err error)

// 添加事件
func AddEvent(span trace.Span, name string, attrs ...attribute.KeyValue)

// 提取追踪 ID
func TraceID(ctx context.Context) string

// 日志格式化
func LogWithTrace(ctx context.Context, message string) string
```

#### 4. Pipeline 追踪模块 (Pipeline Tracing Module)

**文件**: `pipeline.go`

**功能**:
- Pipeline 生命周期追踪
- Element 操作追踪
- 消息处理追踪

**追踪点**:
```go
// Pipeline 启动
InstrumentPipelineStart(ctx, pipelineName)

// Pipeline 停止
InstrumentPipelineStop(ctx, pipelineName)

// Element 启动
InstrumentElementStart(ctx, elementName)

// Element 处理消息
InstrumentElementProcess(ctx, elementName, msg)

// Pipeline 推送消息
InstrumentPipelinePush(ctx, pipelineName, msg)

// Pipeline 拉取消息
InstrumentPipelinePull(ctx, pipelineName)
```

#### 5. Connection 追踪模块 (Connection Tracing Module)

**文件**: `connection.go`

**功能**:
- WebRTC 连接生命周期追踪
- 连接状态变化追踪
- 消息收发追踪

**追踪点**:
```go
// 连接创建
InstrumentConnectionCreated(ctx, connID, connType)

// 状态变化
InstrumentConnectionStateChange(ctx, connID, connType, oldState, newState)

// 消息收发
InstrumentConnectionMessage(ctx, connID, connType, direction, dataSize)

// 连接错误
InstrumentConnectionError(ctx, connID, connType, err)

// 连接关闭
InstrumentConnectionClosed(ctx, connID, connType)
```

#### 6. AI 服务追踪模块 (AI Service Tracing Module)

**文件**: `ai.go`

**功能**:
- LLM 请求/响应追踪
- STT 请求/响应追踪
- TTS 请求/响应追踪
- 音视频处理追踪

**追踪点**:
```go
// LLM 追踪
InstrumentLLMRequest(ctx, provider, model)
InstrumentLLMResponse(ctx, provider, model, responseType, dataSize)

// STT 追踪
InstrumentSTTRequest(ctx, provider, audioSize)
InstrumentSTTResponse(ctx, provider, text)

// TTS 追踪
InstrumentTTSRequest(ctx, provider, voice, text)
InstrumentTTSResponse(ctx, provider, audioSize)

// 音视频处理追踪
InstrumentAudioProcessing(ctx, operation, inputSize, outputSize)
InstrumentVideoProcessing(ctx, operation, inputSize, outputSize)
```

## 使用场景 (Use Cases)

### 场景 1: 调试 Pipeline 消息流

当需要调试消息在 Pipeline 中的流转时：

```go
// 在 Pipeline 启动时创建追踪
ctx, span := trace.InstrumentPipelineStart(ctx, "my-pipeline")
err := pipeline.Start(ctx)
span.End()

// 在每个 Element 处理消息时追踪
ctx, span := trace.InstrumentElementProcess(ctx, elementName, msg)
// 处理消息
span.End()
```

通过追踪可以看到：
- 消息在哪个 Element 耗时最长
- 消息的完整处理路径
- 每个 Element 的音频/视频参数

### 场景 2: 监控 AI 服务性能

追踪 AI 服务的延迟和错误：

```go
// LLM 请求追踪
ctx, span := trace.InstrumentLLMRequest(ctx, "gemini", "gemini-2.0-flash-exp")
response, err := callLLM(ctx, request)
if err != nil {
    trace.RecordError(span, err)
}
span.End()
```

可以分析：
- 不同 AI 服务的响应时间
- 错误率和错误类型
- 请求/响应数据大小

### 场景 3: 诊断 WebRTC 连接问题

追踪 WebRTC 连接的完整生命周期：

```go
// 连接创建
ctx, span := trace.InstrumentConnectionCreated(ctx, connID, "webrtc")
conn := createConnection(ctx)
span.End()

// 状态变化
ctx, span := trace.InstrumentConnectionStateChange(ctx, connID, "webrtc", "new", "connected")
span.End()
```

可以分析：
- 连接建立耗时
- 状态转换序列
- 连接失败原因

## 性能优化 (Performance Optimization)

### 1. 异步导出

使用批处理导出器，异步发送追踪数据，不阻塞主流程：

```go
tracerProvider = sdktrace.NewTracerProvider(
    sdktrace.WithBatcher(exporter),  // 批处理导出
    // ...
)
```

### 2. 可配置采样

在高吞吐场景下，可以降低采样率：

```go
cfg.SamplingRate = 0.1  // 只采样 10% 的追踪
```

### 3. No-op 模式

完全禁用追踪以消除开销：

```bash
export TRACE_EXPORTER=none
```

### 4. 上下文传播

使用 context 传播追踪信息，避免额外的查找开销。

## 与后端集成 (Backend Integration)

### Jaeger

```bash
# 启动 Jaeger
docker run -d --name jaeger \
  -p 16686:16686 \
  -p 4317:4317 \
  jaegertracing/all-in-one:latest

# 配置应用
export TRACE_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
```

访问 http://localhost:16686 查看追踪。

### Zipkin

```bash
# 启动 Zipkin
docker run -d -p 9411:9411 openzipkin/zipkin

# 配置 OpenTelemetry Collector 导出到 Zipkin
```

### 云服务商

- **Google Cloud Trace**: 配置 OTLP 端点到 cloudtrace.googleapis.com
- **AWS X-Ray**: 使用 AWS Distro for OpenTelemetry (ADOT) Collector
- **Azure Monitor**: 使用 Azure Monitor OpenTelemetry Exporter

## 最佳实践 (Best Practices)

### 1. 始终传播 Context

```go
func parent(ctx context.Context) {
    ctx, span := trace.StartSpan(ctx, "parent")
    defer span.End()

    // 传递 ctx 到子函数
    child(ctx)
}
```

### 2. 记录有意义的属性

```go
trace.SetAttributes(span,
    attribute.String("session.id", sessionID),
    attribute.Int("audio.sample_rate", sampleRate),
)
```

### 3. 正确处理错误

```go
if err != nil {
    trace.RecordError(span, err)
    return err
}
```

### 4. 使用事件标记里程碑

```go
trace.AddEvent(span, "audio.processing.completed")
```

### 5. 在日志中包含追踪 ID

```go
log.Printf(trace.LogWithTrace(ctx, "Processing message"))
```

## 实现清单 (Implementation Checklist)

- [x] 核心追踪基础设施
- [x] 多种导出器支持（stdout, OTLP, none）
- [x] Pipeline 追踪辅助函数
- [x] Connection 追踪辅助函数
- [x] AI 服务追踪辅助函数
- [x] 标准化属性定义
- [x] 辅助工具函数
- [x] 示例应用
- [x] 完整文档（中英文）
- [x] README 文件

## 示例代码 (Example Code)

完整的示例应用位于 `examples/tracing-demo/main.go`，演示了：

1. 追踪初始化
2. Pipeline 追踪
3. 消息处理追踪
4. 优雅关闭

运行示例：

```bash
# 使用 stdout 导出器
go run examples/tracing-demo/main.go

# 使用 OTLP 导出器
TRACE_EXPORTER=otlp go run examples/tracing-demo/main.go
```

## 文档资源 (Documentation Resources)

1. **用户指南**: `docs/tracing.md` - 完整的使用指南
2. **包文档**: `pkg/trace/README.md` - 包级别的文档
3. **设计文档**: `docs/tracing-design.md` - 本设计文档
4. **示例代码**: `examples/tracing-demo/main.go` - 可运行的示例

## 未来扩展 (Future Enhancements)

1. **Metrics 集成**: 添加 OpenTelemetry Metrics 支持
2. **Logs 集成**: 集成 OpenTelemetry Logs
3. **自动化仪表化**: 使用 Go instrumentation 自动添加追踪
4. **Dashboard 模板**: 提供 Grafana dashboard 模板
5. **性能分析**: 集成性能分析工具

## 总结 (Summary)

本追踪系统为 realtime-ai 项目提供了全面的可观测性支持，具有以下特点：

1. **易于使用**: 简单的初始化和丰富的辅助函数
2. **灵活配置**: 支持多种环境和导出器
3. **全面覆盖**: 涵盖所有关键组件
4. **高性能**: 最小化开销，支持采样
5. **生产就绪**: 与主流后端无缝集成

通过使用这个追踪系统，开发者可以：
- 快速定位性能瓶颈
- 诊断复杂的分布式问题
- 监控生产环境的健康状况
- 优化系统性能
