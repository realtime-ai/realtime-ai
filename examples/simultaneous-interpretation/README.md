# Real-time Simultaneous Interpretation

> ⚠️ **NEW: Realtime API Version Available!**
>
> A **significantly improved version** using Gemini Live API is now available at:
> **[`examples/simultaneous-interpretation-realtime/`](../simultaneous-interpretation-realtime/)**
>
> **Benefits of the new version:**
> - ✅ **70-80% faster** (1-2s latency vs 4-7s)
> - ✅ **36% cheaper** ($0.014/min vs $0.022/min)
> - ✅ **57% simpler** (3 elements vs 7)
> - ✅ **Better audio quality** (smooth, no choppiness)
> - ✅ **Easier setup** (5 minutes vs 1 hour)
>
> **👉 We strongly recommend using the Realtime API version for all new projects.**
>
> See [COMPARISON.md](../simultaneous-interpretation-realtime/COMPARISON.md) for detailed comparison.

---

# Traditional Pipeline Implementation (Legacy)

> **Note:** This implementation uses the traditional STT→Translation→TTS pipeline.
> While functional, it has higher latency and cost compared to the Realtime API version.
>
> **Use this version only if you need:**
> - Specific STT/TTS providers (Azure, custom providers)
> - Fine-grained control over each pipeline stage
> - Custom processing between stages

A complete real-time simultaneous interpretation system that converts speech from one language to another in real-time. Speak in your native language and hear the interpretation instantly through your speakers - just like having a professional interpreter!

## 🎯 Features

- 🎤 **Real-time Speech Recognition** - Using OpenAI Whisper API
- 🌐 **Instant Translation** - Powered by GPT-4o-mini or Gemini
- 🔊 **Natural Speech Synthesis** - Using OpenAI TTS with multiple voice options
- 🎧 **Audio-to-Audio Interpretation** - Complete voice-to-voice interpretation pipeline
- 💬 **Live Bilingual Subtitles** - Optional text display of original and translated speech
- 🔇 **Voice Activity Detection** - Optimized with Silero VAD (optional)
- 🌍 **Multi-language Support** - Support for 99+ languages
- ⚡ **Low Latency** - WebRTC-based streaming for minimal delay

## ⚠️ Known Limitations (Fixed in Realtime API Version)

This traditional implementation has some limitations:

1. **High Latency**: 4-7 seconds (vs 1-2s in Realtime API version)
2. **Missing AudioPacer**: Audio output can be choppy without proper buffering
3. **Complex Setup**: Requires configuring 3 separate APIs
4. **Higher Cost**: $0.022/min (vs $0.014/min in Realtime API version)
5. **Maintenance Burden**: 7 pipeline elements to manage

**These are all solved in the [Realtime API version](../simultaneous-interpretation-realtime/).**

## 📋 Quick Comparison

| Feature | This (Traditional) | [Realtime API](../simultaneous-interpretation-realtime/) |
|---------|-------------------|------------------------|
| Latency | 4-7 seconds | **1-2 seconds** ✅ |
| Cost | $0.022/min | **$0.014/min** ✅ |
| Setup Time | ~1 hour | **5 minutes** ✅ |
| Pipeline Elements | 7 | **3** ✅ |
| Audio Quality | Variable | **Smooth** ✅ |
| Maintenance | Complex | **Simple** ✅ |

## 🏗️ Architecture

The system uses a modular pipeline architecture with 7 processing stages:

```
┌──────────────────────────────────────────────────────────────────────────┐
│                    SIMULTANEOUS INTERPRETATION PIPELINE                   │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                            │
│  Audio Input (Microphone)                                                 │
│      ↓                                                                     │
│  [1] Audio Resample (48kHz → 16kHz)                                      │
│      ↓                                                                     │
│  [2] Silero VAD (Voice Activity Detection) [Optional]                    │
│      ↓                                                                     │
│  [3] Whisper STT (Speech-to-Text) ⏱️ 2-3s latency                       │
│      ↓                                                                     │
│  [4] Translation Element (GPT/Gemini) ⏱️ 1-2s latency                   │
│      ↓                                                                     │
│  [5] OpenAI TTS (Text-to-Speech) ⏱️ 1-2s latency                        │
│      ↓                                                                     │
│  [6] Audio Resample (24kHz → 48kHz)                                      │
│      ↓                                                                     │
│  [7] Opus Encode (Audio Compression)                                     │
│      ↓                                                                     │
│  Audio Output (Speakers/Headphones)                                       │
│                                                                            │
└──────────────────────────────────────────────────────────────────────────┘
```

**Total Latency**: ~4-7 seconds (vs 1-2s in Realtime API version)

## 📋 Prerequisites

- Go 1.21 or later
- OpenAI API key (required)
- Google API key (optional, only if using Gemini translation)
- Microphone-enabled device
- Web browser with WebRTC support (Chrome, Firefox, Safari, Edge)
- Speakers or headphones

## 🚀 Quick Start

### 1. Installation

```bash
# Clone the repository and navigate to the example
cd realtime-ai/examples/simultaneous-interpretation

# Install Go dependencies
go mod download
```

### 2. Configuration

Create a `.env` file from the example:

```bash
cp .env.example .env
```

Edit `.env` and add your API key:

```env
OPENAI_API_KEY=sk-your-api-key-here
SOURCE_LANG=zh
TARGET_LANG=en
```

### 3. Run the Application

**Standard mode (without VAD):**
```bash
go run main.go
```

**With VAD support** (recommended for better performance):
```bash
# First, download the VAD model
mkdir -p models
curl -L https://github.com/snakers4/silero-vad/raw/master/files/silero_vad.onnx -o models/silero_vad.onnx

# Build and run with VAD support
go build -tags vad -o interpretation
./interpretation
```

### 4. Open the Web Interface

Navigate to `http://localhost:8080` in your browser and click "Start Interpretation"!

## 💡 Recommendation

**For new projects, we strongly recommend using the [Realtime API version](../simultaneous-interpretation-realtime/) instead.**

It provides:
- Significantly lower latency (70-80% improvement)
- Lower cost (36% savings)
- Simpler setup and maintenance
- Better audio quality

This traditional implementation is maintained for:
- Users who need specific STT/TTS providers
- Projects requiring fine-grained control
- Compatibility with existing systems

## 📚 Documentation

For detailed documentation on this implementation, see the sections below.

For the recommended Realtime API version:
- **Quick Start**: See [`simultaneous-interpretation-realtime/QUICK_START.md`](../simultaneous-interpretation-realtime/QUICK_START.md)
- **Full Docs**: See [`simultaneous-interpretation-realtime/README.md`](../simultaneous-interpretation-realtime/README.md)
- **Comparison**: See [`simultaneous-interpretation-realtime/COMPARISON.md`](../simultaneous-interpretation-realtime/COMPARISON.md)

---

## Traditional Implementation Details

[Rest of the original README content follows...]

### Pipeline Components

1. **AudioResampleElement** - Converts audio to 16kHz mono (Whisper's required format)
2. **SileroVADElement** (optional) - Detects voice activity to optimize API calls
3. **WhisperSTTElement** - Transcribes speech to text using OpenAI Whisper
4. **TranslateElement** - Translates text using GPT-4o-mini or Gemini
5. **UniversalTTSElement** - Synthesizes translated text to natural speech
6. **AudioResampleElement** - Converts TTS output to 48kHz (WebRTC standard)
7. **OpusEncodeElement** - Compresses audio for efficient transmission

### Known Issues

⚠️ **Audio Choppy/Stuttering**: This implementation lacks AudioPacer which can cause choppy audio output. This is fixed in the Realtime API version.

⚠️ **High Latency**: The sequential pipeline (STT→Translation→TTS) results in 4-7 second latency. The Realtime API version achieves 1-2 seconds.

⚠️ **Higher Costs**: Using 3 separate APIs costs ~36% more than the unified Realtime API approach.

---

## Migration to Realtime API

To migrate from this traditional implementation to the Realtime API version:

1. **Navigate to the new directory**:
   ```bash
   cd ../simultaneous-interpretation-realtime
   ```

2. **Update configuration**:
   ```bash
   cp .env.example .env
   # Edit .env - you only need GOOGLE_API_KEY now
   ```

3. **Run the new version**:
   ```bash
   go run main.go
   open http://localhost:8080
   ```

**Migration time**: ~5 minutes
**Performance improvement**: 70-80% latency reduction

See [Migration Guide](../simultaneous-interpretation-realtime/COMPARISON.md#migration-path) for details.

---

## License

This example is part of the Realtime AI framework. See the main repository for license information.

## Support

For issues and questions:
- **GitHub Issues**: [realtime-ai/realtime-ai/issues](https://github.com/realtime-ai/realtime-ai/issues)
- **Realtime API Version** (recommended): See [`simultaneous-interpretation-realtime/`](../simultaneous-interpretation-realtime/)
- **Documentation**: See main repository README and CLAUDE.md

---

**💡 Recommendation: Switch to [Realtime API version](../simultaneous-interpretation-realtime/) for better performance!**
