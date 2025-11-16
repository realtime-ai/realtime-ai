# ConversationRelay 架构设计文档 v2.1

## 1. 设计理念

### 1.1 核心目标

将系统拆分为三个独立的服务层：
- **Media Gateway**: 负责媒体接入（WebRTC）和协议处理
- **ConversationRelay**: 负责会话管理和业务控制平面（纯逻辑层）
- **AI Service**: 负责 AI Pipeline 执行（STT/VAD/LLM/TTS）

支持的媒体传输方式：
- **WebRTC**: 浏览器/移动端的实时音视频传输
- **gRPC Stream**: 服务端音频流传输

三者通过**清晰的接口**解耦，支持独立部署、扩缩容和演进。

### 1.2 设计原则

1. **关注点分离**:
   - Media Gateway 关注"传输"
   - ConversationRelay 关注"控制"
   - AI Service 关注"智能"
2. **独立演进**: 各服务可独立升级、部署
3. **水平扩展**: 各服务可独立扩缩容
4. **协议无关**: 通过标准接口通信（gRPC/HTTP）

---

## 2. 整体架构

### 2.1 宏观架构图

```
┌─────────────────────────────────────────────────────────────────────┐
│                        External Layer                                │
│                                                                       │
│  ┌──────────┐  ┌──────────┐  ┌──────────────┐  ┌──────────────┐   │
│  │ Browser  │  │  Mobile  │  │   Server     │  │  Business    │   │
│  │ (WebRTC) │  │ (WebRTC) │  │   (gRPC)     │  │   Backend    │   │
│  └────┬─────┘  └────┬─────┘  └──────┬───────┘  └──────┬───────┘   │
└───────┼─────────────┼────────────────┼──────────────────┼───────────┘
        │             │                │                  │
        │ WebRTC      │ WebRTC         │ gRPC Stream      │ gRPC Control
        │ Media       │ Media          │ (Audio)          │ (订阅/控制)
        │             │                │                  │
┌───────▼─────────────▼────────────────┼──────────────────┼───────────┐
│                                      │                  │            │
│           Media Gateway              │                  │            │
│           (媒体接入层)                 │                  │            │
│                                      │                  │            │
│  ┌──────────────────────┐           │                  │            │
│  │   WebRTC Handler     │           │                  │            │
│  │                      │           │                  │            │
│  │  - SDP 协商           │           │                  │            │
│  │  - ICE 处理           │           │                  │            │
│  │  - RTP/SRTP          │           │                  │            │
│  │  - 编解码 (Opus)      │           │                  │            │
│  └──────────┬───────────┘           │                  │            │
│             │                       │                  │            │
│    ┌────────▼────────┐              │                  │            │
│    │  Media Router   │  音频/视频流路由 │                  │            │
│    └────────┬────────┘              │                  │            │
└─────────────┼───────────────────────┼──────────────────┼────────────┘
              │ gRPC Stream (Audio)   │                  │
              │                       │                  │
┌─────────────────────▼──────────────────────────────▼───────────────┐
│                                                                      │
│                   ConversationRelay Service                          │
│                   (纯控制平面 - 无媒体处理)                           │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │                    Session Manager                            │  │
│  │  - Session 生命周期管理                                        │  │
│  │  - 状态机维护（INIT → ACTIVE → PAUSED → ENDED）              │  │
│  │  - 会话元数据存储（user_id, call_id, metadata）              │  │
│  │  - 多租户隔离                                                  │  │
│  └────────┬──────────────────────────────────────────┬───────────┘  │
│           │                                          │               │
│  ┌────────▼────────┐                       ┌────────▼────────┐     │
│  │     Event       │                       │    Control      │     │
│  │   Dispatcher    │                       │    Handler      │     │
│  │                 │                       │                 │     │
│  │ 转发 AI Service  │                       │ 接收 Backend     │     │
│  │ 事件到 Backend   │                       │ 控制指令         │     │
│  └────────┬────────┘                       └────────┬────────┘     │
│           │                                         │               │
│  ┌────────▼─────────────────────────────────────────▼────────┐    │
│  │                    TTS Manager                             │    │
│  │  - TTS 队列管理 (接收 Backend 推送的文本/音频)             │    │
│  │  - 播放控制 (控制 Media Gateway 播放)                      │    │
│  │  - 打断处理 (响应用户打断事件)                             │    │
│  └────────┬───────────────────────────────────────────────────┘    │
└───────────┼──────────────────────────────────────────────────────────┘
            │
            │ gRPC Stream (事件)             gRPC RPC (控制)
            ▼                                       ▲
┌───────────────────────────────────────────────────┼──────────────────┐
│                      Business Backend             │                  │
│                      (客户业务后台 - 控制平面)      │                  │
│                                                   │                  │
│  ┌────────────┐  ┌────────────┐  ┌──────────────▼─┐  ┌──────────┐ │
│  │   Event    │  │  Business  │  │      LLM       │  │   TTS    │ │
│  │  Listener  │─▶│   Logic    │─▶│   Integrator   │─▶│Generator │ │
│  │ (监听事件)  │  │  (决策引擎) │  │  (调用LLM)      │  │(生成回复) │ │
│  └────────────┘  └────────────┘  └────────────────┘  └─────┬────┘ │
│                                                              │       │
│  客户自定义业务逻辑:                                          │       │
│  - 根据 STT 结果查询知识库                                    │       │
│  - 调用自己的 LLM (OpenAI/Claude/...)                        │       │
│  - 生成个性化回复文本                                         │       │
│  - 记录对话日志到数据库                                       │       │
└──────────────────────────────────────────────────────────────┼──────┘
                                                               │
                                             (推送 TTS 文本/音频)
                                                               │
┌──────────────────────────────────────────────────────────────▼──────┐
│                                                                      │
│                      AI Service (Pipeline)                           │
│                      (AI 数据平面 - 可选被调用)                       │
│                                                                      │
│  ┌────────────────────────────────────────────────────────────┐    │
│  │                   Pipeline Engine                           │    │
│  │                                                             │    │
│  │  ┌──────┐   ┌──────┐   ┌──────┐   ┌──────┐   ┌──────┐   │    │
│  │  │ VAD  │──▶│ STT  │──▶│ LLM  │──▶│ TTS  │──▶│Codec │   │    │
│  │  └──────┘   └──────┘   └──────┘   └──────┘   └──────┘   │    │
│  │                                                             │    │
│  │  Elements 可插拔、可组合、可配置                             │    │
│  └────────────────────────────────────────────────────────────┘    │
│           ▲                                          │              │
│           │                                          │              │
│  ┌────────┴──────────────────────────────────────────▼─────────┐  │
│  │                    gRPC Service API                           │  │
│  │                                                               │  │
│  │  - ProcessAudio(stream) → stream Events (VAD/STT)           │  │
│  │  - SynthesizeSpeech(text) → stream Audio                    │  │
│  │  - GetCapabilities() → Capabilities                         │  │
│  └───────────────────────────────────────────────────────────────┘  │
└──────────────────────┬──────────────────────────────────────────────┘
                       │ gRPC Stream (VAD/STT Events)
                       ▼
┌──────────────────────────────────────────────────────────────────────┐
│                   ConversationRelay Service                          │
│                   (接收 AI 事件，转发给 Backend)                      │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 3. 服务职责划分

### 3.1 Media Gateway

#### 核心职责

**1. 媒体协议处理**:
   - WebRTC 信令处理（SDP offer/answer，ICE candidate 交换）
   - gRPC 流式音频接入

**2. 媒体流处理**:
   - 音视频编解码（Opus/VP8/H264）
   - 音频重采样
   - 媒体流路由和转发
   - RTP/SRTP 处理

**3. 连接管理**:
   - 用户连接的建立和维持
   - NAT穿透（STUN/TURN）
   - 连接质量监控
   - 断线重连

**4. 与其他服务交互**:
   - 将音频流推送给 AI Service（gRPC Stream）
   - 将音频流推送给 ConversationRelay（gRPC Stream）
   - 接收 ConversationRelay 的播放控制指令
   - 通知 ConversationRelay 连接状态变化

#### 不负责
- 会话业务逻辑
- AI 推理
- 业务控制决策

---

### 3.2 ConversationRelay Service

#### 核心职责

**1. 会话管理 (Session Management)**:
   - Session 生命周期管理（创建/激活/暂停/销毁）
   - 会话状态机维护（INIT → ACTIVE → PAUSED → ENDED）
   - 会话元数据存储和查询（user_id, call_id, metadata）
   - 会话超时和清理
   - 多租户隔离

**2. 控制平面 (Control Plane)**:
   - 向 Business Backend 推送事件（gRPC Server Stream）:
     - Session 生命周期事件（start/end）
     - VAD 事件（从 AI Service 转发）
     - STT 转录结果（从 AI Service 转发）
     - 打断事件（从 AI Service 转发）
     - 错误事件
   - 接收 Business Backend 控制指令（gRPC RPC）:
     - TTS 文本/音频推送
     - 会话控制（暂停/恢复/结束）
     - 配置更新
   - TTS 队列管理和播放控制
   - 事件路由和分发

#### 不负责
- 媒体协议处理（WebRTC/gRPC Stream）- 由独立的 Media Gateway 处理
- 音视频编解码 - 由 Media Gateway 或 AI Service 处理
- AI 推理（VAD/STT/LLM/TTS）- 由 AI Service 处理
- 业务逻辑 - 由 Business Backend 处理

---

### 3.3 AI Service

#### 核心职责
1. **Pipeline 执行**:
   - Element 管理和编排
   - 音频/视频/文本处理 Pipeline

2. **AI 推理**:
   - VAD（语音活动检测）
   - STT（语音转文本）
   - LLM（大语言模型）
   - TTS（文本转语音）
   - NLU（自然语言理解）

3. **对外 API**:
   - 提供 gRPC/HTTP API
   - 流式处理能力
   - 批量处理能力

#### 不负责
- 媒体传输协议处理
- 会话管理
- 业务逻辑

---

### 3.4 Business Backend（客户实现）

#### 核心职责
1. **业务逻辑**:
   - 对话策略
   - 知识库查询
   - 权限验证

2. **LLM 集成**:
   - 调用 OpenAI/Claude/自建 LLM
   - Prompt 工程
   - 上下文管理

3. **数据管理**:
   - 对话日志
   - 用户画像
   - 数据分析

---

## 4. 接口设计

### 4.1 ConversationRelay ↔ Business Backend (gRPC)

```protobuf
// ConversationRelay 提供给 Business Backend 的接口

service ConversationRelayService {
  // 1. 订阅会话事件 (Server Stream)
  rpc SubscribeSession(SubscribeRequest) returns (stream SessionEvent);

  // 2. 推送 TTS 内容 (Client Stream)
  rpc PushTTS(stream TTSRequest) returns (TTSResponse);

  // 3. 发送控制指令 (Unary)
  rpc SendControl(ControlRequest) returns (ControlResponse);

  // 4. 查询会话信息 (Unary)
  rpc GetSessionInfo(SessionInfoRequest) returns (SessionInfoResponse);
}

// 事件类型
enum EventType {
  SESSION_START = 0;    // 会话开始
  SESSION_END = 1;      // 会话结束
  VAD_EVENT = 2;        // 语音活动检测
  STT_PARTIAL = 3;      // 部分转录
  STT_FINAL = 4;        // 最终转录
  INTERRUPT = 5;        // 用户打断
  ERROR = 6;            // 错误
}

// 会话事件
message SessionEvent {
  string session_id = 1;
  int64 timestamp = 2;
  EventType event_type = 3;

  oneof payload {
    SessionStartPayload session_start = 10;
    SessionEndPayload session_end = 11;
    VADPayload vad = 12;
    STTPayload stt = 13;
    InterruptPayload interrupt = 14;
    ErrorPayload error = 15;
  }
}
```

### 4.2 ConversationRelay ↔ AI Service (gRPC)

```protobuf
// AI Service 提供给 ConversationRelay 的接口

service AIProcessingService {
  // 1. 处理音频流 (Bidirectional Stream)
  rpc ProcessAudioStream(stream AudioChunk) returns (stream ProcessingEvent);

  // 2. 语音合成 (Server Stream)
  rpc SynthesizeSpeech(SynthesisRequest) returns (stream AudioChunk);

  // 3. 查询能力 (Unary)
  rpc GetCapabilities(CapabilitiesRequest) returns (CapabilitiesResponse);

  // 4. 健康检查 (Unary)
  rpc HealthCheck(HealthCheckRequest) returns (HealthCheckResponse);
}

// 处理事件
message ProcessingEvent {
  string session_id = 1;
  int64 timestamp = 2;

  oneof event {
    VADResult vad = 10;
    STTResult stt = 11;
    LLMResult llm = 12;
    TTSResult tts = 13;
  }
}

// VAD 结果
message VADResult {
  bool speech_detected = 1;
  float confidence = 2;
}

// STT 结果
message STTResult {
  string text = 1;
  bool is_final = 2;
  float confidence = 3;
  repeated WordTiming words = 4;
}

// TTS 合成请求
message SynthesisRequest {
  string text = 1;
  string voice = 2;
  string language = 3;
  SynthesisConfig config = 4;
}

message SynthesisConfig {
  float speed = 1;      // 0.5 - 2.0
  float pitch = 2;      // 0.5 - 2.0
  int32 sample_rate = 3; // 16000, 24000, 48000
}
```

---

## 5. 核心交互流程

### 5.1 会话建立流程

```
End User         Media Gateway      ConversationRelay      Business Backend      AI Service
    │                    │                    │                      │                  │
    │ 1. WebRTC Offer    │                    │                      │                  │
    │───────────────────▶│                    │                      │                  │
    │                    │                    │                      │                  │
    │ 2. WebRTC Answer   │                    │                      │                  │
    │◀───────────────────│                    │                      │                  │
    │                    │                    │                      │                  │
    │                    │ 3. Notify Connect  │                      │                  │
    │                    │───────────────────▶│                      │                  │
    │                    │                    │                      │                  │
    │                    │                    │ 4. SessionStart      │                  │
    │                    │                    │─────────────────────▶│                  │
    │                    │                    │   (gRPC Stream)      │                  │
    │                    │                    │                      │                  │
    │                    │                    │ 5. Config Response   │                  │
    │                    │                    │◀─────────────────────│                  │
    │                    │                    │   (Pipeline配置)      │                  │
    │                    │                    │                      │                  │
    │                    │                    │ 6. Init Pipeline     │                  │
    │                    │                    │──────────────────────┼─────────────────▶│
    │                    │                    │   (gRPC: GetCapabilities)               │
    │                    │                    │                      │                  │
    │                    │                    │ 7. Capabilities      │                  │
    │                    │                    │◀─────────────────────┼──────────────────│
```

### 5.2 语音识别流程（可选调用 AI Service）

```
End User       Media Gateway    ConversationRelay      Business Backend      AI Service
    │                │                  │                      │                  │
    │ Audio Stream   │                  │                      │                  │
    │───────────────▶│                  │                      │                  │
    │                │                  │                      │                  │
    │                │ Forward Audio    │                      │                  │
    │                │─────────────────▶│                      │                  │
    │                │                  │                      │                  │
    │                │                  │ (可选) 调用 AI Service                   │
    │                │                  │──────────────────────┼─────────────────▶│
    │                │                  │   ProcessAudioStream │                  │
    │                │                  │                      │                  │
    │                │                  │ VAD Event            │                  │
    │                │                  │◀─────────────────────┼──────────────────│
    │                │                  │                      │                  │
    │                │                  │ 转发给 Backend        │                  │
    │                │                  │─────────────────────▶│                  │
    │                │                  │   VADPayload         │                  │
    │                │                  │                      │                  │
    │                │                  │ STT Result (partial) │                  │
    │                │                  │◀─────────────────────┼──────────────────│
    │                │                  │                      │                  │
    │                │                  │ 转发给 Backend        │                  │
    │                │                  │─────────────────────▶│                  │
    │                │                  │   STTPayload         │                  │
    │                │                  │                      │                  │
    │                │                  │ STT Result (final)   │                  │
    │                │                  │◀─────────────────────┼──────────────────│
    │                │                  │                      │                  │
    │                │                  │ 转发给 Backend        │                  │
    │                │                  │─────────────────────▶│                  │
    │                │                  │   STTPayload(final)  │                  │
```

### 5.3 业务后台控制 TTS 流程

```
End User     Media Gateway    ConversationRelay      Business Backend      AI Service
    │                │                  │                      │                  │
    │                │                  │                      │ 1. 业务处理       │
    │                │                  │                      │   + LLM 调用     │
    │                │                  │                      │   + 生成回复     │
    │                │                  │                      │                  │
    │                │                  │ 2. Push TTS Text     │                  │
    │                │                  │◀─────────────────────│                  │
    │                │                  │   "您好，请问..."     │                  │
    │                │                  │                      │                  │
    │                │                  │ 3. (可选) 调用 AI Service 合成          │
    │                │                  │──────────────────────┼─────────────────▶│
    │                │                  │   SynthesizeSpeech   │                  │
    │                │                  │                      │                  │
    │                │                  │ 4. Audio Chunks      │                  │
    │                │                  │◀─────────────────────┼──────────────────│
    │                │                  │                      │                  │
    │                │ 5. Send Audio    │                      │                  │
    │                │◀─────────────────│                      │                  │
    │                │                  │                      │                  │
    │ 6. Play Audio  │                  │                      │                  │
    │◀───────────────│                  │                      │                  │
```

### 5.4 打断流程

```
End User     Media Gateway    ConversationRelay      Business Backend      AI Service
    │                │                  │                      │                  │
    │ 正在播放 AI 回复... │                  │                      │                  │
    │◀───────────────│                  │                      │                  │
    │                │                  │                      │                  │
    │ 用户开始说话       │                  │                      │                  │
    │───────────────▶│                  │                      │                  │
    │                │                  │                      │                  │
    │                │ Forward Audio    │                      │                  │
    │                │─────────────────▶│                      │                  │
    │                │                  │                      │                  │
    │                │                  │ (内部 VAD 检测)       │                  │
    │                │                  │                      │                  │
    │                │                  │ 1. Interrupt Event   │                  │
    │                │                  │─────────────────────▶│                  │
    │                │                  │                      │                  │
    │                │ 2. Stop Command  │                      │                  │
    │                │◀─────────────────│                      │                  │
    │                │                  │                      │                  │
    │ 3. Stop Audio  │                  │                      │                  │
    │◀───────────────│                  │                      │                  │
    │                │                  │                      │                  │
    │                │                  │ 4. Clear TTS Queue   │                  │
    │                │                  │──────────────────────┼─────────────────▶│
    │                │                  │   (如果使用 AI Service)                  │
```

---

## 6. 部署架构

### 6.1 单体部署（初期）

```
┌─────────────────────────────────────────┐
│        Same Process / Container          │
│                                          │
│  ┌────────────────────────────────────┐ │
│  │   ConversationRelay Service        │ │
│  │   Port: 8080 (HTTP/WS)            │ │
│  │   Port: 9000 (WebRTC UDP)         │ │
│  │   Port: 50051 (gRPC to Backend)   │ │
│  └────────────┬───────────────────────┘ │
│               │ 内部函数调用              │
│  ┌────────────▼───────────────────────┐ │
│  │   AI Service (Pipeline)            │ │
│  │   - 同进程调用                      │ │
│  │   - 无网络开销                      │ │
│  └────────────────────────────────────┘ │
└─────────────────────────────────────────┘
                   │
                   │ gRPC
                   ▼
┌─────────────────────────────────────────┐
│        Business Backend                  │
│        (客户部署)                         │
└─────────────────────────────────────────┘
```

### 6.2 微服务部署（中期）

```
┌──────────────────────────┐
│  ConversationRelay       │
│  Service                 │
│  (Stateful)              │
│  - 处理媒体连接           │
│  - 会话管理              │
│  Replicas: 3-5           │
└────────┬─────────────────┘
         │
         │ gRPC / HTTP
         │
┌────────▼─────────────────┐
│  AI Service              │
│  (Stateless)             │
│  - Pipeline 执行         │
│  - AI 推理               │
│  Replicas: 5-10          │
│  (可独立扩容)             │
└──────────────────────────┘

         │ gRPC
         ▼
┌──────────────────────────┐
│  Business Backend        │
│  (Stateless)             │
│  Replicas: 2-5           │
└──────────────────────────┘
```

### 6.3 大规模部署（长期）

```
             Load Balancer
                  │
    ┌─────────────┼─────────────┐
    │             │             │
┌───▼────┐   ┌───▼────┐   ┌───▼────┐
│ Relay  │   │ Relay  │   │ Relay  │
│   1    │   │   2    │   │   3    │
└───┬────┘   └───┬────┘   └───┬────┘
    │            │            │
    └────────────┼────────────┘
                 │ Service Mesh (Istio)
    ┌────────────┼────────────┐
    │            │            │
┌───▼────┐   ┌──▼─────┐  ┌──▼─────┐
│   AI   │   │   AI   │  │   AI   │
│Service │   │Service │  │Service │
│   1    │   │   2    │  │   3    │
└────────┘   └────────┘  └────────┘

    │ gRPC to Backend
    ▼
┌──────────────────────────┐
│  Business Backend Pool   │
└──────────────────────────┘

    │ 存储
    ▼
┌──────────────────────────┐
│  Redis (Session Store)   │
│  PostgreSQL (Metadata)   │
└──────────────────────────┘
```

---

## 7. 代码组织

### 7.1 目录结构

```
realtime-ai/
├── pkg/
│   ├── gateway/                  # Media Gateway 服务
│   │   ├── service.go            # Gateway 主服务
│   │   ├── webrtc/               # WebRTC 处理
│   │   │   ├── handler.go
│   │   │   ├── peer_connection.go
│   │   │   └── ice_handler.go
│   │   ├── grpc/                 # gRPC 音频流处理
│   │   │   └── handler.go
│   │   ├── router/               # 媒体路由
│   │   │   └── media_router.go
│   │   ├── codec/                # 编解码
│   │   │   ├── opus.go
│   │   │   └── resample.go
│   │   └── proto/                # gRPC 协议
│   │       ├── gateway.proto     # Gateway ↔ Relay/AI
│   │       └── gateway.pb.go
│   │
│   ├── relay/                    # ConversationRelay 服务 (纯控制平面)
│   │   ├── service.go            # gRPC 服务实现
│   │   ├── session/              # Session 管理
│   │   │   ├── manager.go
│   │   │   ├── store.go
│   │   │   └── state_machine.go
│   │   ├── event/                # 事件分发
│   │   │   ├── dispatcher.go
│   │   │   └── router.go
│   │   ├── control/              # 控制处理
│   │   │   ├── tts_manager.go
│   │   │   ├── command_handler.go
│   │   │   └── playback_controller.go
│   │   └── proto/                # gRPC 协议
│   │       ├── relay.proto       # Relay ↔ Backend
│   │       └── relay.pb.go
│   │
│   ├── aiservice/                # AI Service (原 pipeline)
│   │   ├── service.go            # gRPC 服务实现
│   │   ├── pipeline/             # Pipeline 引擎
│   │   │   ├── pipeline.go
│   │   │   ├── element.go
│   │   │   └── bus.go
│   │   ├── elements/             # AI Elements
│   │   │   ├── vad_element.go
│   │   │   ├── stt_element.go
│   │   │   ├── llm_element.go
│   │   │   └── tts_element.go
│   │   └── proto/                # gRPC 协议
│   │       ├── aiservice.proto   # AI Service API
│   │       └── aiservice.pb.go
│   │
│   └── common/                   # 公共组件 (跨服务共享)
│       ├── audio/
│       ├── codec/
│       ├── proto/                # 公共消息定义
│       └── utils/
│
├── cmd/
│   ├── gateway/                  # Media Gateway 启动入口
│   │   └── main.go
│   ├── relay/                    # ConversationRelay 启动入口
│   │   └── main.go
│   ├── aiservice/                # AI Service 启动入口
│   │   └── main.go
│   └── standalone/               # 单体部署启动入口
│       └── main.go
│
├── examples/
│   ├── relay-backend/            # 业务后台示例
│   │   └── main.go
│   └── client/
│       └── webrtc-client.html
│
└── docs/
    ├── conversation-relay-architecture-v2.md
    ├── gateway-api.md            # Media Gateway API 文档
    ├── relay-api.md              # Relay API 文档
    └── aiservice-api.md          # AI Service API 文档
```

### 7.2 启动方式

**单体部署**:
```go
// cmd/standalone/main.go
package main

import (
    "github.com/realtime-ai/realtime-ai/pkg/relay"
    "github.com/realtime-ai/realtime-ai/pkg/aiservice"
)

func main() {
    // 启动 AI Service (同进程)
    aiSvc := aiservice.NewService()
    go aiSvc.Start(":50052")

    // 启动 ConversationRelay
    relaySvc := relay.NewService(&relay.Config{
        AIServiceAddr: "localhost:50052", // 本地调用
        ListenAddr: ":8080",
    })
    relaySvc.Start()
}
```

**微服务部署**:
```bash
# 分别启动两个服务
go run cmd/aiservice/main.go --port=50052
go run cmd/relay/main.go --ai-service=ai-service:50052 --port=8080
```

---

## 8. 配置管理

### 8.1 ConversationRelay 配置

```yaml
# relay-config.yaml
server:
  grpc_port: 50051           # gRPC 控制平面端口

ai_service:
  enabled: true              # 是否集成 AI Service
  mode: "embedded"           # embedded / remote
  grpc_addr: "localhost:50052"
  timeout: 30s

gateway:
  grpc_addr: "localhost:50053"  # Media Gateway 地址
  timeout: 5s

session:
  max_concurrent: 1000
  idle_timeout: 300s
  redis_url: "redis://localhost:6379"

features:
  builtin_vad: true          # 内置简单 VAD
```

### 8.3 Media Gateway 配置

```yaml
# gateway-config.yaml
server:
  http_addr: ":8080"         # HTTP/WebRTC 信令端口
  grpc_port: 50053           # gRPC 音频流端口

webrtc:
  ice_servers:
    - urls: ["stun:stun.l.google.com:19302"]
  udp_port_min: 10000
  udp_port_max: 20000

audio:
  sample_rate: 48000
  channels: 1
  codec: "opus"

relay:
  grpc_addr: "localhost:50051"  # ConversationRelay 地址
  timeout: 5s
```

### 8.2 AI Service 配置

```yaml
# aiservice-config.yaml
server:
  grpc_port: 50052

pipeline:
  default_elements:
    - vad
    - stt
    - llm
    - tts

elements:
  vad:
    provider: "silero"
    threshold: 0.5

  stt:
    provider: "azure"
    language: "zh-CN"
    api_key: "${AZURE_SPEECH_KEY}"

  llm:
    provider: "gemini"
    model: "gemini-2.0-flash"
    api_key: "${GOOGLE_API_KEY}"

  tts:
    provider: "azure"
    voice: "zh-CN-XiaoxiaoNeural"
    api_key: "${AZURE_SPEECH_KEY}"

performance:
  max_concurrent_pipelines: 100
  element_buffer_size: 100
```

---

## 9. 监控与可观测性

### 9.1 指标

**ConversationRelay 指标**:
- `relay_sessions_active`: 活跃会话数
- `relay_sessions_total`: 总会话数
- `relay_media_bytes_rx`: 接收字节数
- `relay_media_bytes_tx`: 发送字节数
- `relay_event_push_latency_ms`: 事件推送延迟
- `relay_grpc_backend_errors`: Backend 调用错误数

**AI Service 指标**:
- `aiservice_pipeline_active`: 活跃 Pipeline 数
- `aiservice_stt_latency_ms`: STT 延迟
- `aiservice_llm_latency_ms`: LLM 延迟
- `aiservice_tts_latency_ms`: TTS 延迟
- `aiservice_element_errors`: Element 错误数

### 9.2 日志

使用结构化日志，区分服务：

```json
{
  "timestamp": "2025-01-15T10:30:00Z",
  "service": "conversation-relay",
  "component": "session_manager",
  "level": "INFO",
  "session_id": "uuid",
  "event": "session_created",
  "metadata": {
    "user_id": "user123",
    "client_type": "webrtc"
  }
}
```

### 9.3 分布式追踪

```
Trace: conversation-flow
├─ Span: relay.handle_media [ConversationRelay]
│  ├─ Span: relay.dispatch_event [ConversationRelay]
│  └─ Span: aiservice.process_audio [AI Service]
│     ├─ Span: element.vad [AI Service]
│     └─ Span: element.stt [AI Service]
└─ Span: backend.handle_stt [Business Backend]
   └─ Span: llm.generate [Business Backend]
```

---

## 10. 迁移路径

### 10.1 Phase 1: 代码分离（1-2 周）
- 将 `pkg/pipeline` 重命名为 `pkg/aiservice`
- 创建 `pkg/relay` 目录
- 定义两个服务的 gRPC 接口
- 保持单体部署方式不变

### 10.2 Phase 2: 接口解耦（2-3 周）
- 实现 ConversationRelay ↔ AI Service gRPC 调用
- 实现 ConversationRelay ↔ Backend gRPC 接口
- 添加配置开关（embedded / remote）
- 编写集成测试

### 10.3 Phase 3: 独立部署（1-2 周）
- 提供独立的 Docker 镜像
- 编写 Kubernetes 部署文件
- 编写业务后台 SDK（Go/Python）
- 更新文档和示例

### 10.4 Phase 4: 生产优化（持续）
- 性能调优
- 监控完善
- 故障注入测试
- 弹性伸缩配置

---

## 11. 总结

### 11.1 架构优势

1. **清晰的职责边界**:
   - Media Gateway 专注媒体传输（WebRTC/gRPC Stream）
   - ConversationRelay 专注会话管理和控制
   - AI Service 专注 AI 能力
   - Business Backend 专注业务逻辑

2. **独立演进**:
   - 各服务可独立迭代
   - 降低耦合度
   - 减少相互影响

3. **灵活部署**:
   - 初期单体，降低运维成本
   - 后期微服务，支持大规模

4. **客户控制力强**:
   - Business Backend 可完全控制对话流程
   - 可自由选择 LLM
   - 可实现复杂业务逻辑

### 11.2 适用场景

- **客服系统**: 客户需要集成自己的知识库和业务系统
- **语音助手**: 需要个性化对话策略
- **教育培训**: 需要自定义教学逻辑
- **智能外呼**: 需要复杂的对话流程控制

---

**文档版本**: v2.1
**最后更新**: 2025-01-15
**状态**: Draft - Ready for Implementation

**更新日志**:
- v2.1: 移除 SIP 和 WebSocket 支持，仅保留 WebRTC 和 gRPC Stream 作为媒体传输方式
- v2.0: 初始版本，三层架构设计
