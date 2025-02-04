# Realtime AI

A real-time AI development framework leveraging WebRTC for audio and video transmission.

**Note that this project is under active development.**

## Project Overview

The project consists of three main components:

- **AI SDK (WebRTC)**: Captures and processes audio and video streams on the client side using the WebRTC protocol, including tasks such as audio/video encoding and preliminary inference.

- **WebRTC Gateway**: Manages signaling, handles NAT/firewall traversal, and forwards media streams. It also supports load balancing with the AI Service.

- **AI Service**: Provides real-time inference and data processing capabilities, including speech recognition, image recognition, real-time subtitle generation, speech synthesis, and interactive real-time large model interactions.

## Key Features

- **User-Friendly**: Designed for ease of use with straightforward integration.

- **WebRTC-Based Transmission**: Utilizes WebRTC for audio and video transmission and employs Data Channels for signaling.

- **Flexible AI Pipeline**: AI services are processed through a pipeline architecture, allowing for customizable and modular assembly.

- **Optimized for Real-Time Scenarios**: Specifically engineered to meet the demands of real-time applications, ensuring low latency and high performance.


##  Plans

```
- [x] WebRTC Server
- [x] Support Gemini Multimodal Live API
- [x] Support OpenAI Realtime API
- [x] Video support
- [x] Support Interruption
- [x] Split WebRTC Gateway and AI Service
- [x] Better Pipeline Design(like Gstreamer)
- [ ] Support ASR/LLM/TTS Pipeline
- [ ] Support JSON-RPC 
- [ ] Support Custom CMD
```


## Prerequisites

- Go 1.21 or higher
- FFmpeg libraries (for audio processing)
- Opus codec library
- Google API Key for Gemini AI
- OpenAI API Key for OpenAI Realtime API

## Installation

1. Install system dependencies:

```bash
# For Debian/Ubuntu
apt-get install pkg-config libopus-dev libavcodec-dev libavformat-dev libavutil-dev libswresample-dev

# For macOS
brew install opus ffmpeg
```

2. Clone the repository:

```bash
git clone https://github.com/realtime-ai/realtime-ai.git
cd realtime-ai
```

3. Install Go dependencies:

```bash
go mod download
```

## Configuration

1. Set up environment variables:

```bash
# Required
export GOOGLE_API_KEY=your_api_key_here

export OPENAI_API_KEY=your_api_key_here
```

## Running the Application


1、Start the server:

```bash
# use  gemini by default
go run examples/gemini-assis/main.go
```

2、 Open the WebRTC Client:

Chrome Browser Open `http://localhost:8080`






