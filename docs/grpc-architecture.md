# gRPC Architecture for Realtime AI

This document describes the gRPC-based architecture as an alternative to WebRTC for the Realtime AI framework.

## Overview

The gRPC implementation provides a simpler, more portable alternative to WebRTC for real-time AI communication. It uses HTTP/2 bidirectional streaming over TCP instead of WebRTC's UDP-based RTP protocol.

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                         Client Layer                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │   Go Client  │  │ Python Client│  │  Web Client  │          │
│  │              │  │              │  │  (gRPC-Web)  │          │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘          │
│         │                 │                  │                   │
└─────────┼─────────────────┼──────────────────┼───────────────────┘
          │                 │                  │
          └─────────────────┼──────────────────┘
                            │
                    gRPC BiDi Stream (HTTP/2 over TCP)
                            │
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│                    gRPC Server Layer                             │
│                                                                   │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  StreamingAIService (gRPC Service)                         │ │
│  │  ┌──────────────────────────────────────────────────────┐ │ │
│  │  │  BiDirectionalStreaming(stream) → stream             │ │ │
│  │  │  - Session Management                                │ │ │
│  │  │  - Connection Lifecycle                              │ │ │
│  │  └──────────────────────────────────────────────────────┘ │ │
│  └────────────┬───────────────────────────────────────────────┘ │
│               │                                                   │
│  ┌────────────▼───────────────────────────────────────────────┐ │
│  │  GRPCConnection (implements RTCConnection interface)      │ │
│  │  - 双向流管理 (Bidirectional stream management)           │ │
│  │  - 消息序列化/反序列化 (Message serialization)            │ │
│  │  - 事件处理 (Event handling)                              │ │
│  │    • OnMessage()                                          │ │
│  │    • OnError()                                            │ │
│  │    • OnStateChange()                                      │ │
│  └────────────┬───────────────────────────────────────────────┘ │
└───────────────┼─────────────────────────────────────────────────┘
                │
                ↓
┌─────────────────────────────────────────────────────────────────┐
│              Pipeline Layer (Unchanged from WebRTC)              │
│                                                                   │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  Pipeline                                                 │  │
│  │                                                           │  │
│  │  AudioResampleElement → GeminiElement → PlayoutSinkElement  │
│  │         ↓                    ↓                  ↓         │  │
│  │    48kHz→16kHz          AI Processing      Audio Output   │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                   │
│  处理流程: PipelineMessage 在 Elements 之间流转                 │
└─────────────────────────────────────────────────────────────────┘
                │
                ↓
┌─────────────────────────────────────────────────────────────────┐
│                    AI Service Layer                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │ Google Gemini│  │ OpenAI API   │  │  Azure STT   │          │
│  │ Multimodal   │  │ Realtime API │  │  Azure TTS   │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
└─────────────────────────────────────────────────────────────────┘
```

## Protocol Buffers Message Design

### Core Message Types

```protobuf
message StreamMessage {
  string session_id = 1;
  MessageType type = 2;
  int64 timestamp = 3;

  oneof payload {
    AudioFrame audio = 10;
    VideoFrame video = 11;
    TextMessage text = 12;
    ControlMessage control = 13;
  }

  map<string, string> metadata = 20;
}

enum MessageType {
  MESSAGE_TYPE_AUDIO = 1;
  MESSAGE_TYPE_VIDEO = 2;
  MESSAGE_TYPE_TEXT = 3;
  MESSAGE_TYPE_CONTROL = 4;
}
```

### Audio Frame

```protobuf
message AudioFrame {
  bytes data = 1;              // PCM or encoded audio data
  int32 sample_rate = 2;       // 16000, 48000, etc.
  int32 channels = 3;          // 1=mono, 2=stereo
  string media_type = 4;       // "audio/x-raw", "audio/opus"
  string codec = 5;            // "pcm", "opus"
  int32 duration_ms = 6;       // Frame duration
}
```

### Text Message

```protobuf
message TextMessage {
  bytes data = 1;              // Text content
  string text_type = 2;        // "plain", "json", "markdown"
}
```

### Control Message

```protobuf
message ControlMessage {
  ControlType control_type = 1;

  oneof control_data {
    ConnectionStateChange state_change = 10;
    ErrorInfo error = 11;
    ConfigUpdate config = 12;
  }
}

enum ControlType {
  CONTROL_TYPE_STATE_CHANGE = 1;
  CONTROL_TYPE_ERROR = 2;
  CONTROL_TYPE_CONFIG = 3;
  CONTROL_TYPE_PING = 4;
  CONTROL_TYPE_PONG = 5;
}
```

## Signaling Flows

### 1. Session Establishment

```
Client                          gRPC Server                    Pipeline
  │                                  │                             │
  │ BiDirectionalStreaming()         │                             │
  │─────────────────────────────────>│                             │
  │     (establish bidirectional)    │                             │
  │                                  │ Create GRPCConnection       │
  │                                  │────────────────────────────>│
  │                                  │                             │
  │                                  │ Initialize Pipeline         │
  │                                  │ (Resample→Gemini→Playout)  │
  │                                  │────────────────────────────>│
  │                                  │                             │
  │ ControlMessage(CONNECTED)        │                             │
  │<─────────────────────────────────│                             │
  │                                  │                             │
```

### 2. Audio Streaming

```
Client                          gRPC Server                    Pipeline
  │                                  │                             │
  │ StreamMessage(AUDIO)             │                             │
  │ AudioFrame(48kHz, mono, PCM)     │                             │
  │─────────────────────────────────>│                             │
  │                                  │ Convert to PipelineMessage  │
  │                                  │────────────────────────────>│
  │                                  │                             │
  │                                  │    Process Audio            │
  │                                  │    (Resample → Gemini)      │
  │                                  │                             │
  │                                  │ PipelineMessage Output      │
  │                                  │<────────────────────────────│
  │                                  │                             │
  │ StreamMessage(AUDIO) Response    │                             │
  │<─────────────────────────────────│                             │
  │ AudioFrame(AI voice reply)       │                             │
  │                                  │                             │
```

### 3. Text Messaging

```
Client                          gRPC Server                    Pipeline
  │                                  │                             │
  │ StreamMessage(TEXT)              │                             │
  │ TextMessage("Hello AI")          │                             │
  │─────────────────────────────────>│                             │
  │                                  │ Convert to PipelineMessage  │
  │                                  │────────────────────────────>│
  │                                  │                             │
  │                                  │    Gemini LLM Processing    │
  │                                  │                             │
  │ StreamMessage(TEXT) [streaming]  │                             │
  │<─────────────────────────────────│                             │
  │ "Hello! How..."                  │                             │
  │<─────────────────────────────────│                             │
  │ "can I help..."                  │                             │
  │<─────────────────────────────────│                             │
```

### 4. Error Handling

```
Client                          gRPC Server                    Pipeline
  │                                  │                             │
  │ StreamMessage(AUDIO)             │                             │
  │─────────────────────────────────>│                             │
  │                                  │ Pipeline processing fails   │
  │                                  │<────────────────────────────│
  │                                  │ Error: "Gemini timeout"     │
  │                                  │                             │
  │ ControlMessage(ERROR)            │                             │
  │<─────────────────────────────────│                             │
  │ ErrorInfo {                      │                             │
  │   code: "AI_TIMEOUT"             │                             │
  │   message: "Gemini timeout"      │                             │
  │ }                                │                             │
```

### 5. Heartbeat/Keepalive

```
Client                          gRPC Server
  │                                  │
  │ ControlMessage(PING)             │
  │─────────────────────────────────>│
  │     (every 30 seconds)           │
  │                                  │
  │ ControlMessage(PONG)             │
  │<─────────────────────────────────│
  │                                  │
```

## Implementation Details

### File Structure

```
pkg/
├── proto/
│   └── streamingai/v1/
│       ├── streaming_ai.proto          # Protocol definition
│       ├── streaming_ai.pb.go          # Generated Go code
│       └── streaming_ai_grpc.pb.go     # Generated gRPC code
│
├── connection/
│   ├── connection.go                   # RTCConnection interface
│   ├── rtc_connection.go               # WebRTC implementation
│   └── grpc_connection.go              # gRPC implementation ✨ NEW
│
├── server/
│   ├── server.go                       # WebRTC server
│   └── grpc_server.go                  # gRPC server ✨ NEW
│
└── pipeline/                           # Unchanged
    ├── pipeline.go
    ├── element.go
    └── bus.go

examples/
├── gemini-assis/                       # WebRTC example
│   └── main.go
│
└── grpc-assis/                         # gRPC example ✨ NEW
    ├── server.go                       # gRPC server example
    ├── client.go                       # gRPC client example
    └── README.md                       # Usage guide
```

### Key Components

#### 1. GRPCConnection

Implements the `RTCConnection` interface but uses gRPC streams instead of WebRTC:

```go
type grpcConnectionImpl struct {
    peerID  string
    stream  pb.StreamingAIService_BiDirectionalStreamingServer
    handler ConnectionEventHandler
    // ...
}

func (c *grpcConnectionImpl) SendMessage(msg *pipeline.PipelineMessage) {
    pbMsg := c.pipelineMessageToProto(msg)
    c.stream.Send(pbMsg)
}
```

#### 2. GRPCServer

Implements the gRPC service and manages connections:

```go
func (s *GRPCServer) BiDirectionalStreaming(
    stream pb.StreamingAIService_BiDirectionalStreamingServer,
) error {
    peerID := uuid.New().String()
    grpcConn := connection.NewGRPCConnection(peerID, stream)
    s.onConnectionCreated(ctx, grpcConn)
    // ...
}
```

#### 3. Pipeline Integration

The Pipeline layer remains unchanged - GRPCConnection simply converts between:
- `StreamMessage` (protobuf) ↔ `PipelineMessage` (internal format)

## Comparison: gRPC vs WebRTC

| Feature | gRPC | WebRTC |
|---------|------|--------|
| **Transport** | HTTP/2 over TCP | UDP (RTP), SCTP (DataChannel) |
| **NAT Traversal** | Not needed (TCP through firewalls) | Required (STUN/TURN servers) |
| **Latency** | ~50-100ms (TCP retransmission) | ~20-50ms (UDP, no retransmission) |
| **Setup Complexity** | Simple (direct connection) | Complex (SDP negotiation, ICE) |
| **Browser Support** | Requires gRPC-Web proxy | Native browser support |
| **Client Types** | Go, Python, Java, C++, etc. | JavaScript (browsers) |
| **Deployment** | Simple (single port, HTTP/2) | Complex (UDP ports, TURN) |
| **Reliability** | High (TCP guarantees delivery) | Medium (UDP may drop packets) |
| **Best For** | Server-to-server, mobile apps | Browser real-time communication |

## Usage Example

### Server

```go
grpcServer := server.NewGRPCServer(&server.GRPCServerConfig{Port: 50051})

grpcServer.OnConnectionCreated(func(ctx context.Context, conn connection.RTCConnection) {
    // Create pipeline
    pipeline := pipeline.NewPipeline("grpc_conn")
    pipeline.AddElements([]pipeline.Element{
        elements.NewAudioResampleElement(48000, 16000, 1, 1),
        elements.NewGeminiElement(),
        elements.NewPlayoutSinkElement(),
    })

    // Register handler
    conn.RegisterEventHandler(&handler{pipeline: pipeline})
    pipeline.Start(ctx)
})

grpcServer.Start() // Blocks
```

### Client

```go
conn, _ := grpc.Dial("localhost:50051", grpc.WithInsecure())
client := pb.NewStreamingAIServiceClient(conn)

stream, _ := client.BiDirectionalStreaming(context.Background())

// Send audio
stream.Send(&pb.StreamMessage{
    Type: pb.MessageType_MESSAGE_TYPE_AUDIO,
    Payload: &pb.StreamMessage_Audio{
        Audio: &pb.AudioFrame{
            Data:       audioData,
            SampleRate: 48000,
            Channels:   1,
            MediaType:  "audio/x-raw",
        },
    },
})

// Receive responses
for {
    msg, err := stream.Recv()
    // Handle received message
}
```

## Advantages of gRPC Approach

1. **Simpler Deployment**: No need for STUN/TURN servers
2. **Better Firewall Compatibility**: TCP works through most firewalls
3. **Multi-Language Support**: gRPC clients in many languages
4. **Type Safety**: Protocol Buffers provide strong typing
5. **Easier Testing**: Can test with simple Go/Python clients
6. **Better Debugging**: HTTP/2 tools can inspect traffic
7. **Maintains Architecture**: Pipeline system unchanged

## Limitations

1. **Higher Latency**: TCP retransmission adds ~30-50ms vs UDP
2. **Browser Support**: Requires gRPC-Web with proxy (Envoy)
3. **Bandwidth**: HTTP/2 headers add overhead vs raw RTP
4. **Not Ideal for**: Ultra-low latency use cases (<50ms requirement)

## When to Use Which?

### Use gRPC when:
- Building server-to-server integrations
- Mobile app development (iOS/Android)
- Desktop applications
- Testing and development
- Simpler deployment is priority

### Use WebRTC when:
- Web browser is the primary client
- Ultra-low latency is critical (<50ms)
- Peer-to-peer communication needed
- Already have WebRTC infrastructure

## Future Enhancements

1. **Authentication**: Add JWT/OAuth2 to gRPC interceptors
2. **TLS**: Enable secure connections with certificates
3. **Load Balancing**: Use gRPC load balancing strategies
4. **Compression**: Enable gzip compression for large messages
5. **Metrics**: Add Prometheus metrics for monitoring
6. **Rate Limiting**: Implement per-client rate limiting
7. **gRPC-Web**: Add Envoy proxy for browser clients

## References

- Protocol Buffers: [streaming_ai.proto](../pkg/proto/streamingai/v1/streaming_ai.proto)
- Implementation: [grpc_server.go](../pkg/server/grpc_server.go), [grpc_connection.go](../pkg/connection/grpc_connection.go)
- Example: [examples/grpc-assis/](../examples/grpc-assis/)
- gRPC Documentation: https://grpc.io/docs/languages/go/
