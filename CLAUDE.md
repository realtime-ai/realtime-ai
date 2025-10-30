# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Realtime AI is a real-time AI framework that uses WebRTC for audio/video transmission. The architecture consists of:

- **AI SDK (WebRTC)**: Client-side capture and processing of audio/video streams
- **WebRTC Gateway**: Handles signaling, NAT traversal, and media stream forwarding
- **AI Service**: Real-time inference including speech recognition, image recognition, STT, TTS, and LLM interactions

The project uses a **GStreamer-inspired pipeline architecture** where audio/video processing is handled through modular, composable elements.

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
go run examples/gemini-assis/main.go      # Gemini multimodal assistant
go run examples/local-assis/main.go       # Local connection example
go run examples/openai-realtime/main.go   # OpenAI Realtime API

# Open web client
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

# Debug options
export DUMP_GEMINI_INPUT=true        # Dump Gemini input audio
export DUMP_PLAYOUT_OUTPUT=true      # Dump playout output audio
```

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
- `GeminiElement`: Google Gemini multimodal integration
- `OpenAIRealtimeAPIElement`: OpenAI Realtime API integration
- `AzureSTTElement`: Azure speech-to-text
- `AzureTTSElement`: Azure text-to-speech
- `OpusDecodeElement`: Opus audio decoding
- `OpusEncodeElement`: Opus audio encoding
- `AudioResampleElement`: Audio resampling
- `PlayoutSinkElement`: Audio playback sink
- `SileroVADElement`: Voice activity detection (optional, requires `-tags vad`)

### Connection System

**RTCConnection Interface** (`pkg/connection/connection.go`):
- Abstracts WebRTC peer connections
- Provides `ConnectionEventHandler` for state changes, messages, and errors
- Implementations: `RTCConnection` (WebRTC), `LocalConnection` (local testing), `WSConnection` (WebSocket)

**Server** (`pkg/server/server.go`):
- `RTCServer`: Manages WebRTC peer connections
- `HandleNegotiate`: HTTP endpoint for WebRTC signaling at `/session`
- Uses callbacks `OnConnectionCreated` and `OnConnectionError` for connection lifecycle

### Typical Flow

1. Client connects to server via HTTP POST to `/session` with SDP offer
2. Server creates `RTCConnection` and registers `ConnectionEventHandler`
3. On connection, handler creates a `Pipeline` with elements (e.g., resample → gemini → playout)
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

- `pkg/pipeline`: Core pipeline infrastructure (Element, Pipeline, Bus, Message types)
- `pkg/elements`: All processing elements (AI integrations, codecs, sinks)
- `pkg/connection`: Connection abstractions (WebRTC, local, WebSocket)
- `pkg/server`: HTTP server and WebRTC session handling
- `pkg/audio`: Audio utilities (resampling, playout, dumping)
- `pkg/tokenizer`: Text tokenization for streaming responses

## WebRTC Protocol

See `docs/webrtc-protocol.md` for details on the WebRTC signaling protocol used between client and server.
