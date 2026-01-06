# Realtime AI

<div align="center">

**A high-performance real-time AI framework for audio and video processing**

[![Go Version](https://img.shields.io/badge/Go-1.23%2B-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

</div>

---

## Overview

Realtime AI is a WebRTC-based framework for building low-latency AI applications with audio and video. It features a **modular pipeline architecture** inspired by GStreamer, enabling you to compose processing elements for speech recognition, LLM interactions, and text-to-speech.

**Architecture:**
```
Client (Browser) → WebRTC Gateway → AI Pipeline
                                    (Decode → STT → LLM → TTS → Encode)
```

## Features

- 🎯 **Low Latency** - WebRTC for real-time audio/video streaming
- 🔌 **Modular Pipelines** - Composable processing elements
- 🤖 **AI Integrations** - Gemini, OpenAI Realtime API, Azure STT/TTS
- ⚡ **Interruption Support** - Natural conversation flow

## Quick Start

### Installation

**macOS:**
```bash
brew install opus ffmpeg go
```

**Ubuntu/Debian (推荐使用安装脚本):**
```bash
# 使用预编译 FFmpeg (更稳定)
./scripts/setup-ffmpeg.sh
eval "$(./scripts/setup-ffmpeg.sh --env)"

# 安装其他依赖
apt-get install pkg-config libopus-dev libopusfile-dev
```

**Ubuntu/Debian (手动安装):**
```bash
apt-get install pkg-config libopus-dev libavcodec-dev libavformat-dev libavutil-dev libswresample-dev
```

**Setup:**
```bash
git clone https://github.com/realtime-ai/realtime-ai.git
cd realtime-ai
go mod download
```

### Run Example

```bash
# Set API key
export GOOGLE_API_KEY="your_api_key"

# Run Gemini assistant
go run examples/gemini-assis/main.go

# Open browser
open http://localhost:8080
```

## Basic Usage

```go
// Create pipeline
pipeline := pipeline.NewPipeline("assistant")

// Add and link elements
resample := elements.NewAudioResampleElement("resample")
gemini := elements.NewGeminiElement("gemini", apiKey)
audioPacer := elements.NewAudioPacerSinkElement("audioPacer")

pipeline.Link(resample, gemini)
pipeline.Link(gemini, audioPacer)

// Start processing
pipeline.Start(ctx)
```

## Documentation

- [CLAUDE.md](CLAUDE.md) - Development guide and architecture details
- [WebRTC Protocol](docs/webrtc-protocol.md) - Signaling protocol specification

## Project Structure

```
pkg/
├── pipeline/      # Core pipeline system
├── elements/      # AI, codecs, and processing elements
├── connection/    # WebRTC abstractions
├── server/        # HTTP/WebRTC server
└── audio/         # Audio utilities

examples/
├── gemini-assis/  # Gemini multimodal assistant
├── local-assis/   # Local connection example
└── openai-realtime/ # OpenAI Realtime API
```

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.

## Status

⚠️ **Active Development** - APIs may change without notice.

---

<div align="center">
Made with ❤️ by the Realtime AI Team
</div>
