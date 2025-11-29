# gRPC Implementation Summary

## 概述

已成功实现基于 gRPC 的实时 AI 通信架构，作为 WebRTC 的替代方案。该实现保持了原有的 Pipeline 架构不变，仅替换了传输层。

## 已完成的工作

### 1. Protocol Buffers 定义
- **文件**: [`pkg/proto/streamingai/v1/streaming_ai.proto`](../pkg/proto/streamingai/v1/streaming_ai.proto)
- **内容**:
  - `StreamingAIService`: 双向流式 RPC 服务
  - `StreamMessage`: 统一消息容器（音频/视频/文本/控制）
  - `AudioFrame`, `VideoFrame`, `TextMessage`: 具体数据类型
  - `ControlMessage`: 连接状态、错误、配置控制
  - 会话管理: `CreateSession`, `CloseSession`

### 2. 核心实现

#### GRPCConnection
- **文件**: [`pkg/connection/grpc_connection.go`](../pkg/connection/grpc_connection.go)
- **功能**:
  - 实现 `RTCConnection` 接口
  - 使用 gRPC 双向流替代 WebRTC
  - 消息转换: `PipelineMessage` ↔ `StreamMessage`
  - 事件处理: `OnMessage`, `OnError`, `OnStateChange`
  - 生命周期管理: `Start`, `Stop`, 优雅关闭

#### GRPCServer
- **文件**: [`pkg/server/grpc_server.go`](../pkg/server/grpc_server.go)
- **功能**:
  - 实现 `StreamingAIService` gRPC 服务
  - 连接管理: 创建、跟踪、清理连接
  - 回调机制: `OnConnectionCreated`, `OnConnectionError`
  - 会话管理: 可选的会话创建和关闭
  - 并发安全: 使用 `sync.RWMutex` 保护共享状态

### 3. 示例代码

#### 服务端示例
- **文件**: [`examples/grpc-assis/server.go`](../examples/grpc-assis/server.go)
- **功能**:
  - 启动 gRPC 服务器（端口 50051）
  - 创建 Pipeline: `AudioResample → Gemini → AudioPacer`
  - 处理客户端连接和消息
  - 优雅关闭支持

#### 客户端示例
- **文件**: [`examples/grpc-assis/client.go`](../examples/grpc-assis/client.go)
- **功能**:
  - 连接到 gRPC 服务器
  - 发送音频帧（模拟数据）
  - 发送文本消息
  - 接收和处理服务器响应
  - 演示双向流通信

### 4. 文档

#### 使用指南
- **文件**: [`examples/grpc-assis/README.md`](../examples/grpc-assis/README.md)
- **内容**:
  - 快速开始指南
  - 环境配置说明
  - 运行示例步骤
  - 消息流程说明
  - 自定义配置方法
  - 故障排查

#### 架构文档
- **文件**: [`docs/grpc-architecture.md`](../docs/grpc-architecture.md)
- **内容**:
  - 完整架构图
  - Protocol Buffers 消息设计
  - 5 个详细信令流程图
  - 实现细节说明
  - gRPC vs WebRTC 对比
  - 使用建议和最佳实践

## 架构对比

### WebRTC 架构（原有）
```
Browser → [SDP/ICE] → WebRTC Server → Pipeline → AI Service
         UDP/RTP         (复杂)
```

### gRPC 架构（新增）
```
Client → [gRPC BiDi] → gRPC Server → Pipeline → AI Service
         HTTP/2/TCP      (简单)
```

## 关键特性

### ✅ 优势
1. **更简单部署**: 无需 STUN/TURN 服务器
2. **更好防火墙兼容**: TCP 可穿透大多数防火墙
3. **多语言支持**: Go, Python, Java, C++, JavaScript 等
4. **类型安全**: Protocol Buffers 强类型
5. **易于测试**: 简单的客户端/服务端测试
6. **保持架构**: Pipeline 系统完全不变
7. **更好调试**: HTTP/2 工具可检查流量

### ⚠️ 限制
1. **延迟稍高**: TCP 重传增加 ~30-50ms（vs UDP）
2. **浏览器支持**: 需要 gRPC-Web + Envoy 代理
3. **带宽开销**: HTTP/2 头部开销 vs 裸 RTP
4. **不适合**: 超低延迟场景（<50ms 要求）

## 文件清单

```
新增文件:
├── pkg/proto/streamingai/v1/
│   ├── streaming_ai.proto              ✨ Protocol Buffers 定义
│   ├── streaming_ai.pb.go              ✨ 生成的 Go 代码
│   └── streaming_ai_grpc.pb.go         ✨ 生成的 gRPC 代码
│
├── pkg/connection/
│   └── grpc_connection.go              ✨ gRPC 连接实现
│
├── pkg/server/
│   └── grpc_server.go                  ✨ gRPC 服务器实现
│
├── examples/grpc-assis/
│   ├── server.go                       ✨ 服务端示例
│   ├── client.go                       ✨ 客户端示例
│   └── README.md                       ✨ 使用指南
│
└── docs/
    ├── grpc-architecture.md            ✨ 架构文档
    └── grpc-implementation-summary.md  ✨ 本文档

保持不变:
├── pkg/pipeline/                       ✓ 完全不变
├── pkg/elements/                       ✓ 完全不变
└── pkg/connection/
    ├── connection.go                   ✓ 接口不变
    ├── rtc_connection.go               ✓ WebRTC 实现保留
    └── local_connection.go             ✓ 本地测试保留
```

## 编译验证

所有核心包已通过编译验证：

```bash
✓ go build ./pkg/proto/streamingai/v1
✓ go build ./pkg/connection
✓ go build ./pkg/server
```

## 使用方式

### 快速开始

1. **设置环境变量**:
   ```bash
   export GOOGLE_API_KEY="your_api_key"
   ```

2. **启动服务器** (Terminal 1):
   ```bash
   go run examples/grpc-assis/server.go
   ```

3. **运行客户端** (Terminal 2):
   ```bash
   go run examples/grpc-assis/client.go client
   ```

### 集成到现有项目

```go
// 替换 WebRTC 服务器
// 之前:
// rtcServer := server.NewRTCServer(cfg)

// 现在:
grpcServer := server.NewGRPCServer(&server.GRPCServerConfig{Port: 50051})

grpcServer.OnConnectionCreated(func(ctx context.Context, conn connection.RTCConnection) {
    // 使用完全相同的 Pipeline 代码
    pipeline := createPipeline()
    conn.RegisterEventHandler(&myHandler{pipeline: pipeline})
    pipeline.Start(ctx)
})

grpcServer.Start()
```

## 消息流程示例

### 音频流
```
Client                    Server                   Pipeline
  │ AudioFrame(48kHz)        │                         │
  │────────────────────────>│                         │
  │                         │ PipelineMessage         │
  │                         │────────────────────────>│
  │                         │     (Resample→Gemini)   │
  │                         │<────────────────────────│
  │ AudioFrame(AI reply)    │                         │
  │<────────────────────────│                         │
```

### 文本消息
```
Client                    Server                   Pipeline
  │ TextMessage("Hi")       │                         │
  │────────────────────────>│                         │
  │                         │ PipelineMessage         │
  │                         │────────────────────────>│
  │                         │     (Gemini LLM)        │
  │ TextMessage("Hello...") │                         │
  │<────────────────────────│                         │
```

## gRPC vs WebRTC 选择指南

### 使用 gRPC 当:
- ✅ 构建服务器到服务器集成
- ✅ 移动应用开发（iOS/Android）
- ✅ 桌面应用
- ✅ 测试和开发环境
- ✅ 简化部署是优先考虑

### 使用 WebRTC 当:
- ✅ 浏览器是主要客户端
- ✅ 需要超低延迟（<50ms）
- ✅ 点对点通信
- ✅ 已有 WebRTC 基础设施

## 依赖更新

在 `go.mod` 中已添加:
```go
require (
    google.golang.org/grpc v1.68.0
    google.golang.org/protobuf v1.36.0
)
```

## 下一步建议

### 短期优化
1. **身份验证**: 添加 JWT/OAuth2 拦截器
2. **TLS**: 启用安全连接
3. **测试**: 添加单元测试和集成测试

### 中期增强
4. **gRPC-Web**: 添加 Envoy 代理支持浏览器
5. **监控**: 集成 Prometheus 指标
6. **压缩**: 启用 gzip 压缩
7. **限流**: 实现客户端限流

### 长期规划
8. **负载均衡**: 使用 gRPC 负载均衡策略
9. **服务网格**: 集成 Istio/Linkerd
10. **多区域部署**: 跨区域负载均衡

## 性能特征

| 指标 | gRPC | WebRTC |
|------|------|--------|
| 连接建立时间 | ~100ms | ~500-1000ms (ICE) |
| 端到端延迟 | 50-100ms | 20-50ms |
| 吞吐量 | 高（TCP 流控）| 中（UDP 无拥塞控制）|
| CPU 使用 | 中 | 高（加密/解密）|
| 可靠性 | 极高（TCP）| 中（丢包）|

## 总结

gRPC 实现提供了一个**更简单、更可靠、更易部署**的实时 AI 通信方案，同时完全保持了 Pipeline 架构的灵活性。对于非浏览器客户端和服务器间通信，gRPC 是比 WebRTC 更好的选择。

两种实现可以**共存**，根据具体场景选择最合适的传输方式：
- 浏览器 → WebRTC
- 移动/服务器 → gRPC

## 参考资源

- 📖 [架构详细文档](./grpc-architecture.md)
- 📘 [使用指南](../examples/grpc-assis/README.md)
- 🔧 [Protocol Buffers 定义](../pkg/proto/streamingai/v1/streaming_ai.proto)
- 💻 [服务端示例](../examples/grpc-assis/server.go)
- 💻 [客户端示例](../examples/grpc-assis/client.go)
- 🌐 [gRPC 官方文档](https://grpc.io/docs/languages/go/)
- 🌐 [Protocol Buffers 指南](https://protobuf.dev/)
