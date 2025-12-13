# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Realtime AI is a real-time AI framework for building multimodal AI applications with audio/video processing. The architecture supports multiple transport protocols:

- **WebRTC Architecture**: Browser-based real-time communication with NAT traversal
- **gRPC Architecture**: Server-to-server bidirectional streaming (see `docs/grpc-architecture.md`)
- **WebSocket Architecture**: Lightweight alternative for specific use cases

**Core Components:**
- **Client SDK**: Captures and processes audio/video streams
- **Gateway/Server**: Handles signaling, NAT traversal, media forwarding, and session management
- **AI Service**: Real-time inference (speech recognition, translation, TTS, LLM interactions)

The project uses a **GStreamer-inspired pipeline architecture** where audio/video processing is handled through modular, composable elements. This design enables flexible composition of AI services, codecs, and audio processing into complete real-time applications.

## Common Commands

### Building and Running
```bash
# Install system dependencies (macOS)
brew install opus ffmpeg

# Install Go dependencies
go mod download

# Build (standard - VAD disabled)
go build ./...

# Build with VAD support (requires ONNX Runtime)
go build -tags vad ./...

# Run examples
go run examples/gemini-assis/main.go                # Gemini multimodal assistant (WebRTC)
go run examples/local-assis/main.go                 # Local connection testing
go run examples/openai-realtime/main.go             # OpenAI Realtime API
go run examples/grpc-assis/server/main.go           # gRPC server
go run examples/grpc-assis/client/main.go           # gRPC client
go run examples/whisper-stt/main.go                 # Whisper STT with VAD
go run examples/qwen-realtime-stt/main.go           # Qwen Realtime STT (true streaming)
go run examples/translation-demo/main.go            # Real-time transcription + translation
go run examples/simultaneous-interpretation/main.go # Voice-to-voice interpretation
go run examples/tracing-demo/main.go                # Distributed tracing demo

# Open web client (for WebRTC examples)
open http://localhost:8080
```

### Optional Features

**VAD (Voice Activity Detection)**: The Silero VAD element is optional and requires ONNX Runtime. See `pkg/elements/VAD_README.md` for setup instructions. Build with `-tags vad` to enable.

### Testing
```bash
# Run all tests
go test ./...

# Run tests in a specific package
go test ./pkg/pipeline
go test ./pkg/audio

# Run a specific test
go test ./pkg/pipeline -run TestBus
```

### Environment Variables
```bash
# Required API keys
export GOOGLE_API_KEY=your_api_key_here
export OPENAI_API_KEY=your_api_key_here

# Optional: Alibaba Cloud DashScope (for Qwen Realtime ASR)
export DASHSCOPE_API_KEY=your_dashscope_key

# Optional: Azure services
export AZURE_SPEECH_KEY=your_azure_key
export AZURE_SPEECH_REGION=your_region

# Debug options
export DUMP_GEMINI_INPUT=true        # Dump Gemini input audio
export DUMP_PACER_OUTPUT=true        # Dump audio pacer output audio

# Tracing options
export TRACE_EXPORTER=stdout         # Tracing exporter: stdout, otlp, none
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317  # OTLP collector endpoint
export ENVIRONMENT=development       # Environment name for traces
```

## Quick Start Guide

### First-Time Setup

1. **Install dependencies** (macOS):
   ```bash
   brew install opus ffmpeg go
   ```

2. **Clone and setup**:
   ```bash
   git clone https://github.com/realtime-ai/realtime-ai.git
   cd realtime-ai
   go mod download
   ```

3. **Set up API keys**:
   ```bash
   export GOOGLE_API_KEY=your_key_here
   # OR
   export OPENAI_API_KEY=your_key_here
   ```

4. **Run your first example** (choose one):
   ```bash
   # Gemini multimodal assistant (requires GOOGLE_API_KEY)
   go run examples/gemini-assis/main.go
   open http://localhost:8080

   # OpenAI Whisper STT demo (requires OPENAI_API_KEY)
   go run examples/whisper-stt/main.go

   # gRPC server/client (requires GOOGLE_API_KEY)
   go run examples/grpc-assis/server/main.go  # Terminal 1
   go run examples/grpc-assis/client/main.go  # Terminal 2
   ```

### Building Your First Pipeline

```go
package main

import (
    "context"
    "github.com/realtime-ai/realtime-ai/pkg/pipeline"
    "github.com/realtime-ai/realtime-ai/pkg/elements"
)

func main() {
    ctx := context.Background()

    // Create pipeline
    p := pipeline.NewPipeline("my-pipeline")

    // Add elements
    resample := elements.NewAudioResampleElement(16000, 1)
    gemini := elements.NewGeminiElement(os.Getenv("GOOGLE_API_KEY"))
    audioPacer := elements.NewAudioPacerSinkElement()

    // Link elements
    p.Link(resample, gemini)
    p.Link(gemini, audioPacer)

    // Start pipeline
    p.Start(ctx)
    defer p.Stop()

    // Push audio data
    p.Push(pipeline.PipelineMessage{Audio: audioData})

    // Pull responses
    response := p.Pull()
}
```

### Common Development Patterns

**Pattern 1: WebRTC Voice Assistant**
```
Browser → WebRTC → AudioResample → Gemini/OpenAI → AudioPacer
```

**Pattern 2: Translation with Transcription**
```
Audio → Whisper STT → Translate → Display/TTS
```

**Pattern 3: gRPC AI Service**
```
gRPC Client → gRPC Server → Pipeline → AI Service → Response
```

## Documentation Resources

### Core Documentation
- **`README.md`**: Main project overview and getting started
- **`CLAUDE.md`** (this file): Comprehensive guide for AI assistants and developers

### Architecture Documents
- **`docs/tracing.md`**: Distributed tracing system (OpenTelemetry)
- **`docs/grpc-architecture.md`**: gRPC architecture and use cases
- **`docs/conversation-relay-architecture-v2.md`**: Multi-service deployment patterns
- **`docs/grpc-implementation-summary.md`**: gRPC implementation details
- **`docs/tracing-design.md`**: Tracing system design decisions
- **`docs/tts-design.md`**: TTS provider system design
- **`docs/webrtc-protocol.md`**: WebRTC signaling protocol specification

### Package Documentation
- **`pkg/trace/README.md`**: Trace package API and usage
- **`pkg/asr/README.md`**: ASR interface and Whisper implementation guide
- **`pkg/tts/README.md`**: TTS provider system and customization
- **`pkg/elements/VAD_README.md`**: Silero VAD setup and usage

### Example-Specific Docs
- **`examples/grpc-assis/README.md`**: gRPC example detailed guide
- Each example directory may contain additional README files

### Development Rules
- **`.cursor/rules/element-rule.mdc`**: Element development standards

## Architecture

### Pipeline System

The core of the framework is a **Pipeline** that connects multiple **Elements** together. This is modeled after GStreamer's design:

- **Pipeline** (`pkg/pipeline/pipeline.go`): Container that manages elements and their connections
- **Element** (`pkg/pipeline/element.go`): Base interface for all processing units
- **BaseElement**: Base implementation providing channels and property management
- **Bus** (`pkg/pipeline/bus.go`): Event system for cross-element communication

#### Key Pipeline Concepts

1. **Message Flow**: Elements communicate via `PipelineMessage` containing `AudioData`, `VideoData`, or `TextData`
2. **Linking**: Elements are connected using `Pipeline.Link(a, b)` which pipes `a.Out()` to `b.In()`. Link returns an unlink function for safe disconnection:
   ```go
   unlink := pipeline.Link(elem1, elem2)
   defer unlink() // Safe cleanup
   ```
3. **Push/Pull**: Data enters via `Pipeline.Push()` and exits via `Pipeline.Pull()`
4. **Lifecycle**: Elements implement `Start(ctx)` and `Stop()` for resource management

#### Element Categories

**Base Elements** (in `pkg/elements/`):
- `LLMBaseElement`: Base for large language model elements
- `STTBaseElement`: Base for speech-to-text elements
- `TTSBaseElement`: Base for text-to-speech elements

**Concrete Elements**:
- **LLM Integration**:
  - `GeminiElement`: Google Gemini multimodal integration
  - `OpenAIRealtimeAPIElement`: OpenAI Realtime API integration
- **Speech Recognition (STT)**:
  - `WhisperSTTElement`: OpenAI Whisper speech-to-text (see `pkg/asr/`)
  - `AzureSTTElement`: Azure speech-to-text
- **Text-to-Speech (TTS)**:
  - `UniversalTTSElement`: Provider-agnostic TTS (see `pkg/tts/`)
  - `AzureTTSElement`: Azure text-to-speech
- **Translation**:
  - `TranslateElement`: Real-time text translation (OpenAI/Gemini)
- **Audio Processing**:
  - `OpusDecodeElement`: Opus audio decoding
  - `OpusEncodeElement`: Opus audio encoding
  - `AudioResampleElement`: Audio resampling and format conversion
  - `AudioPacerSinkElement`: Audio pacing sink
- **Voice Activity Detection**:
  - `SileroVADElement`: Silero VAD with passthrough/filter modes (optional, requires `-tags vad`)

### Connection System

**Connection Interface** (`pkg/connection/connection.go`):
- Abstracts real-time bidirectional communication
- Provides `ConnectionEventHandler` for state changes, messages, and errors
- **Implementations**:
  - `RTCConnection`: WebRTC peer connections for browser clients
  - `GRPCConnection`: gRPC bidirectional streaming for server-to-server
  - `WSConnection`: WebSocket connections for lightweight clients
  - `LocalConnection`: In-process connections for testing

**Server Implementations**:
- **`pkg/server/server.go`**: HTTP/WebRTC server
  - `RTCServer`: Manages WebRTC peer connections
  - `HandleNegotiate`: HTTP endpoint for WebRTC signaling at `/session`
  - Uses callbacks `OnConnectionCreated` and `OnConnectionError` for connection lifecycle
- **`pkg/server/grpc_server.go`**: gRPC server
  - `GRPCServer`: Manages gRPC bidirectional streaming connections
  - Implements `StreamingAI` service from `pkg/proto/streamingai/v1/`
  - Supports server-to-server and programmatic client integrations

### Typical Flow

1. Client connects to server via HTTP POST to `/session` with SDP offer
2. Server creates `RTCConnection` and registers `ConnectionEventHandler`
3. On connection, handler creates a `Pipeline` with elements (e.g., resample → gemini → audiopacer)
4. Elements are linked: `Pipeline.Link(element1, element2)`
5. Pipeline started: `Pipeline.Start(ctx)`
6. Audio flows: `conn` → `Pipeline.Push()` → elements → `Pipeline.Pull()` → `conn.SendMessage()`

## Element Development Rules

From `.cursor/rules/element-rule.mdc`:

- Elements **must** use `BaseElement` as the base struct
- Elements **must** implement `Start(ctx)` and `Stop()` methods following the pattern in `gemini_element.go`:
  - Use `context.WithCancel` to manage lifecycle
  - Start goroutines with `wg.Add(1)` and defer `wg.Done()`
  - Handle cancellation via `ctx.Done()`
  - Wait for goroutines in `Stop()` using `wg.Wait()`

## Key Packages

### Core Infrastructure
- `pkg/pipeline`: Core pipeline infrastructure (Element, Pipeline, Bus, Message types)
- `pkg/elements`: All processing elements (AI integrations, codecs, sinks, 17 files)
- `pkg/connection`: Connection abstractions (WebRTC, gRPC, WebSocket, Local)
- `pkg/server`: HTTP/WebRTC and gRPC server implementations

### AI Service Abstractions
- `pkg/asr`: Automatic Speech Recognition provider interface (see `pkg/asr/README.md`)
  - Extensible provider system for STT services
  - OpenAI Whisper implementation with VAD integration
  - Easy to add custom ASR providers
- `pkg/tts`: Text-to-Speech provider system (see `pkg/tts/README.md`)
  - Provider-agnostic TTS interface
  - OpenAI TTS implementation
  - Supports custom provider implementations

### Utilities
- `pkg/audio`: Audio utilities (resampling, audio pacing, dumping)
- `pkg/tokenizer`: Text tokenization for streaming responses
- `pkg/utils`: Common utilities (audio format conversions)
- `pkg/proto`: Protocol Buffer definitions for gRPC services

### Observability
- `pkg/trace`: Distributed tracing with OpenTelemetry (see `docs/tracing.md` and `pkg/trace/README.md`)
  - Multiple exporters (stdout, OTLP, none)
  - Pre-built instrumentation helpers
  - Production-ready tracing for pipelines, elements, connections, and AI operations

## Distributed Tracing

The framework includes OpenTelemetry-based distributed tracing for monitoring and debugging. See `docs/tracing.md` for comprehensive documentation.

### Quick Start

```go
import "github.com/realtime-ai/realtime-ai/pkg/trace"

func main() {
    ctx := context.Background()

    // Initialize tracing
    cfg := trace.DefaultConfig()
    if err := trace.Initialize(ctx, cfg); err != nil {
        log.Fatal(err)
    }
    defer trace.Shutdown(ctx)

    // Use instrumentation helpers
    ctx, span := trace.InstrumentPipelineStart(ctx, "my-pipeline")
    defer span.End()
}
```

### Tracing Features

- **Multiple exporters**: stdout (development), OTLP (production), none (disabled)
- **Pre-built instrumentation**: Helpers for pipelines, elements, connections, AI operations
- **Rich context**: Automatic attribute collection for audio/video/connection data
- **Performance-conscious**: Configurable sampling, async batching, minimal overhead
- **Production-ready**: Integration with Jaeger, Zipkin, cloud providers

### Example

```bash
# Run tracing demo
go run examples/tracing-demo/main.go

# With OTLP exporter (requires collector)
TRACE_EXPORTER=otlp go run examples/tracing-demo/main.go
```

See `pkg/trace/README.md` and `docs/tracing.md` for detailed documentation.

## ASR (Automatic Speech Recognition) System

The framework provides an extensible ASR system in `pkg/asr/` that abstracts speech-to-text providers:

### Provider Interface
```go
type Provider interface {
    Transcribe(ctx context.Context, audio AudioData) (string, error)
    TranscribeStream(ctx context.Context) (Stream, error)
}
```

### Available Providers
- **Whisper**: OpenAI Whisper API implementation (`pkg/asr/whisper.go`)
  - Supports multiple models (whisper-1, etc.)
  - Configurable language detection
  - VAD integration for optimized API usage
  - Buffered streaming (simulated)

- **Qwen Realtime**: Alibaba Cloud DashScope real-time ASR (`pkg/asr/qwen_realtime.go`)
  - True streaming via WebSocket (similar to OpenAI Realtime API)
  - Real-time partial and final transcription results
  - Manual commit mode for VAD integration
  - Supports Chinese, English, Japanese, Korean, Cantonese

### Pipeline Integration
- **`WhisperSTTElement`**: Pipeline element wrapping Whisper provider
  - Consumes `AudioData` messages
  - Emits `TextData` messages with transcriptions
  - Optionally integrates with `SileroVADElement` to reduce API costs

- **`QwenRealtimeSTTElement`**: Pipeline element wrapping Qwen Realtime provider
  - True streaming ASR with WebSocket connection
  - Emits partial and final `TextData` messages
  - VAD integration with manual commit mode

### Example Usage
```go
// Create Whisper provider
provider := asr.NewWhisperProvider(apiKey, asr.WhisperConfig{
    Model:    "whisper-1",
    Language: "en",
})

// Use in pipeline element
sttElement := elements.NewWhisperSTTElement(provider)
pipeline.AddElement(sttElement)

// Or create Qwen Realtime provider for true streaming
qwenProvider, _ := asr.NewQwenRealtimeProvider(asr.QwenRealtimeConfig{
    APIKey: os.Getenv("DASHSCOPE_API_KEY"),
    Model:  "qwen3-asr-flash-realtime",
})
qwenSTTElement, _ := elements.NewQwenRealtimeSTTElement(elements.QwenRealtimeSTTConfig{
    APIKey:               os.Getenv("DASHSCOPE_API_KEY"),
    Language:             "zh",
    EnablePartialResults: true,
})
```

See `pkg/asr/README.md` for comprehensive documentation and examples.

## TTS (Text-to-Speech) Provider System

The framework includes a provider-based TTS system in `pkg/tts/` for flexible text-to-speech integration:

### Provider Interface
```go
type TTSProvider interface {
    Synthesize(ctx context.Context, text string) ([]byte, error)
    GetAudioFormat() AudioFormat
}
```

### Available Providers
- **OpenAI TTS**: Implementation using OpenAI's TTS API
  - Supports multiple voices (alloy, echo, fable, onyx, nova, shimmer)
  - Multiple models (tts-1, tts-1-hd)
  - Configurable speed (0.25x - 4.0x)
  - Multiple output formats (mp3, opus, aac, flac, wav, pcm)

### Pipeline Integration
- **`UniversalTTSElement`**: Provider-agnostic TTS element
  - Accepts any `TTSProvider` implementation
  - Consumes `TextData` messages
  - Emits `AudioData` messages
  - Automatic audio format handling

### Example Usage
```go
// Create OpenAI TTS provider
provider := tts.NewOpenAIProvider(apiKey, tts.OpenAIConfig{
    Model: "tts-1",
    Voice: "alloy",
    Speed: 1.0,
})

// Use in pipeline
ttsElement := elements.NewUniversalTTSElement(provider)
pipeline.AddElement(ttsElement)
```

### Custom Providers
Easily create custom TTS providers by implementing the `TTSProvider` interface:
```go
type MyCustomTTS struct {}

func (t *MyCustomTTS) Synthesize(ctx context.Context, text string) ([]byte, error) {
    // Your TTS implementation
    return audioData, nil
}

func (t *MyCustomTTS) GetAudioFormat() tts.AudioFormat {
    return tts.AudioFormat{SampleRate: 24000, Format: "pcm"}
}
```

See `pkg/tts/README.md` for detailed documentation and customization guide.

## Translation

The `TranslateElement` provides real-time text translation in pipelines:

### Features
- **Multiple LLM backends**: OpenAI (GPT-4, GPT-3.5) or Google Gemini
- **Flexible language pairs**: Any supported source/target language
- **Pipeline integration**: Consumes and emits `TextData` messages
- **Streaming support**: Works with streaming text inputs

### Example Usage
```go
translateElement := elements.NewTranslateElement(elements.TranslateConfig{
    APIKey:       os.Getenv("OPENAI_API_KEY"),
    Provider:     "openai",
    Model:        "gpt-4",
    SourceLang:   "English",
    TargetLang:   "Spanish",
})
```

### Use Cases
- Real-time transcription with translation (see `examples/translation-demo/`)
- Simultaneous interpretation systems (see `examples/simultaneous-interpretation/`)
- Multilingual chat applications
- Live subtitle translation

## gRPC Architecture

The framework supports gRPC as an alternative to WebRTC for server-to-server and programmatic integrations:

### When to Use gRPC
- Server-to-server communication
- Programmatic client integrations (Go, Python, Java, etc.)
- Microservices architecture
- When browser support is not required
- Lower latency requirements in controlled networks

### Components
- **Protocol Buffers**: Service definitions in `pkg/proto/streamingai/v1/streaming_ai.proto`
- **gRPC Server**: `pkg/server/grpc_server.go`
- **gRPC Connection**: `pkg/connection/grpc_connection.go`
- **Example**: Complete server/client implementation in `examples/grpc-assis/`

### Service Definition
```protobuf
service StreamingAI {
  rpc BidirectionalStream(stream StreamRequest) returns (stream StreamResponse);
}
```

### Key Differences from WebRTC
| Feature | WebRTC | gRPC |
|---------|--------|------|
| Browser Support | ✅ Native | ❌ Requires proxy |
| NAT Traversal | ✅ Built-in (ICE/STUN/TURN) | ❌ Direct connection |
| Use Case | Browser clients | Server-to-server |
| Latency | Higher (NAT traversal) | Lower (direct) |
| Protocol | UDP/SRTP | HTTP/2 |

### Advanced Architecture
See `docs/conversation-relay-architecture-v2.md` for multi-service deployment patterns:
- **ConversationRelay**: Orchestrates multiple AI services
- **Media Gateway**: Handles WebRTC/gRPC protocol translation
- **AI Service**: Stateless processing service
- Enables independent scaling and multi-provider support

## GitHub Actions CI/CD

The repository includes comprehensive GitHub Actions workflows:

### Workflows

**`.github/workflows/test.yml`** - Main testing pipeline:
- Runs on every push/PR to main/develop
- Builds FFmpeg 7.0 from source (with caching for performance)
- Sets up Go 1.23
- Runs `go vet` and `go test` with coverage
- Uploads coverage to Codecov

**`.github/workflows/claude.yml`** - Claude Code integration:
- Triggered by issue comments, PR reviews, issue opens
- Activates when `@claude` is mentioned
- Runs Claude Code for automated development tasks

**`.github/workflows/claude-code-review.yml`** - Automated PR reviews:
- Triggered by PR open/sync events
- Provides automated code review feedback
- Uses Claude Code for intelligent analysis

### Development Workflow
1. Create feature branch
2. Make changes and commit
3. Push to GitHub → CI tests run automatically
4. Create PR → Automated review triggers
5. Mention `@claude` in comments for AI assistance

## Advanced Deployment Architectures

### ConversationRelay Architecture (v2)

For production deployments requiring scalability and multi-provider support:

**Architecture Layers:**
1. **Media Gateway**: Handles WebRTC/gRPC connections, protocol translation
2. **ConversationRelay**: Orchestrates AI service requests, manages sessions
3. **AI Service Pool**: Multiple stateless AI processing services

**Benefits:**
- Independent scaling of gateway vs. AI services
- Support for multiple AI providers simultaneously
- Hot-swap AI services without client disconnection
- Regional deployment for latency optimization

**Documentation**: See `docs/conversation-relay-architecture-v2.md` for detailed design

### Single-Server Deployment

For simpler deployments or development:
- WebRTC or gRPC server with embedded pipeline
- All processing in single process
- Examples: `gemini-assis`, `grpc-assis`

## Examples and Demos

The repository includes comprehensive examples demonstrating different use cases:

### WebRTC Examples
- **`examples/gemini-assis/`**: Full-featured Gemini multimodal assistant
  - WebRTC client with HTML5 UI
  - Real-time audio/video processing
  - Bidirectional conversation

- **`examples/openai-realtime/`**: OpenAI Realtime API integration
  - Demonstrates OpenAI's streaming API
  - WebRTC-based voice interface

### gRPC Examples
- **`examples/grpc-assis/`**: Complete gRPC implementation
  - Server and client components
  - Bidirectional streaming
  - Programmatic API integration
  - See `examples/grpc-assis/README.md` for details

### Speech Processing Examples
- **`examples/whisper-stt/`**: Whisper speech-to-text with VAD
  - OpenAI Whisper integration
  - Optional Silero VAD for optimized API usage
  - Real-time transcription

- **`examples/openai-tts/`**: OpenAI Text-to-Speech demo
  - Standalone TTS demonstration
  - Multiple voice options
  - Audio format handling

### Translation and Interpretation
- **`examples/translation-demo/`**: Real-time transcription + translation
  - Whisper STT → Translation → Display
  - Bilingual subtitle display
  - Shows ASR + Translation pipeline

- **`examples/simultaneous-interpretation/`**: Voice-to-voice interpretation
  - Complete audio-to-audio pipeline
  - Whisper STT → Translate → OpenAI TTS
  - Real-time multilingual conversation

### Development Tools
- **`examples/local-assis/`**: Local connection testing
  - In-process testing without WebRTC
  - Useful for pipeline development
  - No network setup required

- **`examples/tracing-demo/`**: Distributed tracing demonstration
  - OpenTelemetry integration showcase
  - Multiple exporter examples
  - Performance monitoring

## Testing

### Test Organization
The project uses multiple testing approaches:

**Unit Tests**: In-package tests alongside source code
```bash
go test ./pkg/pipeline        # Pipeline tests
go test ./pkg/audio          # Audio utility tests
go test ./pkg/asr            # ASR provider tests
go test ./pkg/tts            # TTS provider tests
```

**Integration Tests**: Separate test module in `tests/`
- `tests/audio_capture_play/`: Audio capture and playback tests
- `tests/gemini_live_api/`: Gemini Live API integration tests
- `tests/translation/`: Translation functionality tests
- `tests/whisper/`: Whisper STT integration tests
- `tests/openai_realtime_text2audio.go`: OpenAI Realtime API tests

**Build Tag Tests**: Tests requiring optional dependencies
```bash
go test -tags vad ./pkg/elements  # VAD element tests (requires ONNX Runtime)
```

### CI/CD Testing
- All tests run automatically on push/PR via GitHub Actions
- FFmpeg built from source for consistent testing environment
- Coverage reporting to Codecov
- Go vet checking for code quality

### Manual Testing
```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test -v ./pkg/pipeline

# Run with build tags
go test -tags vad ./pkg/elements
```

## WebRTC Protocol

See `docs/webrtc-protocol.md` for details on the WebRTC signaling protocol used between client and server.
