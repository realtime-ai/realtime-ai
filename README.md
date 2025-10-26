# Realtime AI

<div align="center">

**A high-performance real-time AI framework for audio and video processing**

[![Go Version](https://img.shields.io/badge/Go-1.23%2B-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Status](https://img.shields.io/badge/Status-Active%20Development-orange)](https://github.com/realtime-ai/realtime-ai)

</div>

---

## 📖 Overview

**Realtime AI** is a real-time AI framework built on WebRTC for low-latency audio and video processing. It features a **GStreamer-inspired pipeline architecture** that enables modular, composable processing elements for building sophisticated AI-powered real-time applications.

### Architecture

The framework is organized into three main layers:

```
┌─────────────────────────────────────────────────────────────┐
│                        Client (Browser)                      │
│                   WebRTC Audio/Video Streams                 │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│                    WebRTC Gateway                            │
│         Signaling • NAT Traversal • Stream Routing          │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│                      AI Service                              │
│    Pipeline: Decode → STT → LLM → TTS → Encode → Playout   │
│  Elements: Gemini • OpenAI • Azure • Opus • Resample • VAD  │
└─────────────────────────────────────────────────────────────┘
```

**Components:**

- **WebRTC Gateway**: Handles WebRTC signaling, ICE negotiation, and media stream forwarding
- **AI Service**: Processes audio/video through a modular pipeline of AI elements
- **Pipeline System**: GStreamer-like architecture for connecting and managing processing elements

## ✨ Key Features

- **🎯 Low Latency**: Optimized for real-time interactions with minimal delay
- **🔌 Modular Pipeline**: GStreamer-inspired design with composable processing elements
- **🌐 WebRTC Native**: Built-in support for WebRTC signaling and media streaming
- **🤖 AI Integrations**: Ready-to-use elements for Gemini, OpenAI Realtime API, Azure STT/TTS
- **🎙️ Audio Processing**: Opus codec, resampling, VAD, and playout capabilities
- **🎥 Video Support**: Process video streams alongside audio
- **⚡ Interruption Handling**: Support for real-time interruptions in conversations
- **🧩 Extensible**: Easy to add custom processing elements

## 🚀 Quick Start

### Prerequisites

- **Go 1.23+**
- **FFmpeg** and **Opus** libraries
- **API Keys**: Google API Key (for Gemini) or OpenAI API Key

### Installation

**macOS:**
```bash
brew install opus ffmpeg
```

**Ubuntu/Debian:**
```bash
apt-get install pkg-config libopus-dev libavcodec-dev libavformat-dev libavutil-dev libswresample-dev
```

**Clone and setup:**
```bash
git clone https://github.com/realtime-ai/realtime-ai.git
cd realtime-ai
go mod download
```

### Configuration

Set your API keys:

```bash
export GOOGLE_API_KEY="your_google_api_key_here"
export OPENAI_API_KEY="your_openai_api_key_here"
```

### Run Examples

**Gemini Multimodal Assistant:**
```bash
go run examples/gemini-assis/main.go
```

**Local Connection Test:**
```bash
go run examples/local-assis/main.go
```

**Access the web client:**
```bash
open http://localhost:8080
```

## 📚 Core Concepts

### Pipeline System

The framework uses a **Pipeline** that connects multiple **Elements** together, inspired by GStreamer:

```go
// Create pipeline
pipeline := pipeline.NewPipeline("my-pipeline")

// Add elements
resample := elements.NewAudioResampleElement("resample")
gemini := elements.NewGeminiElement("gemini", apiKey)
playout := elements.NewPlayoutSinkElement("playout")

// Link elements: resample → gemini → playout
pipeline.Link(resample, gemini)
pipeline.Link(gemini, playout)

// Start processing
pipeline.Start(ctx)

// Push audio data
pipeline.Push(audioMessage)

// Pull processed results
result := pipeline.Pull()
```

### Elements

Elements are modular processing units. Available elements include:

**AI Models:**
- `GeminiElement` - Google Gemini multimodal API
- `OpenAIRealtimeAPIElement` - OpenAI Realtime API

**Audio Processing:**
- `OpusDecodeElement` / `OpusEncodeElement` - Opus codec
- `AudioResampleElement` - Audio resampling
- `PlayoutSinkElement` - Audio playback
- `VADElement` - Voice activity detection

**Speech Services:**
- `AzureSTTElement` - Azure Speech-to-Text
- `AzureTTSElement` - Azure Text-to-Speech

### Creating Custom Elements

Extend `BaseElement` and implement the `Element` interface:

```go
type MyElement struct {
    *pipeline.BaseElement
}

func (e *MyElement) Start(ctx context.Context) error {
    e.Context, e.CancelFunc = context.WithCancel(ctx)

    go func() {
        for {
            select {
            case msg := <-e.In():
                // Process message
                e.Out() <- processedMsg
            case <-e.Context.Done():
                return
            }
        }
    }()

    return nil
}
```

## 📂 Project Structure

```
realtime-ai/
├── pkg/
│   ├── pipeline/      # Core pipeline system (Pipeline, Element, Bus)
│   ├── elements/      # Processing elements (AI, codecs, sinks)
│   ├── connection/    # Connection abstractions (WebRTC, local, WebSocket)
│   ├── server/        # HTTP/WebRTC server
│   ├── audio/         # Audio utilities (resample, playout)
│   └── tokenizer/     # Text tokenization for streaming
├── examples/
│   ├── gemini-assis/  # Gemini multimodal assistant
│   ├── local-assis/   # Local connection example
│   └── grpc-assis/    # gRPC-based assistant
├── docs/              # Documentation
└── tests/             # Test files
```

## 🧪 Testing

```bash
# Run all tests
go test ./...

# Test specific package
go test ./pkg/pipeline
go test ./pkg/audio

# Run specific test
go test ./pkg/pipeline -run TestBus
```

## 🛣️ Roadmap

- [x] WebRTC Server
- [x] Gemini Multimodal Live API
- [x] OpenAI Realtime API
- [x] Video support
- [x] Interruption handling
- [x] Split WebRTC Gateway and AI Service
- [x] GStreamer-like Pipeline Design
- [ ] ASR/LLM/TTS Pipeline
- [ ] JSON-RPC support
- [ ] Custom CMD protocol
- [ ] More AI service integrations
- [ ] Performance benchmarks

## 🐛 Debug Options

Enable debugging with environment variables:

```bash
export DUMP_GEMINI_INPUT=true      # Dump Gemini input audio
export DUMP_PLAYOUT_OUTPUT=true    # Dump playout output audio
```

## 📖 Documentation

- [CLAUDE.md](CLAUDE.md) - Development guidelines and architecture details
- [WebRTC Protocol](docs/webrtc-protocol.md) - WebRTC signaling protocol specification

## 🤝 Contributing

Contributions are welcome! Please feel free to submit issues and pull requests.

## 📄 License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## ⚠️ Status

**This project is under active development.** APIs may change without notice. Use in production at your own risk.

---

<div align="center">
Made with ❤️ by the Realtime AI Team
</div>






