# Real-time Simultaneous Interpretation

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
│  [3] Whisper STT (Speech-to-Text)                                        │
│      ↓                                                                     │
│  [4] Translation Element (GPT/Gemini)                                    │
│      ↓                                                                     │
│  [5] OpenAI TTS (Text-to-Speech)                                         │
│      ↓                                                                     │
│  [6] Audio Resample (24kHz → 48kHz)                                      │
│      ↓                                                                     │
│  [7] Opus Encode (Audio Compression)                                     │
│      ↓                                                                     │
│  Audio Output (Speakers/Headphones)                                       │
│                                                                            │
└──────────────────────────────────────────────────────────────────────────┘
```

### Pipeline Components

1. **AudioResampleElement** - Converts audio to 16kHz mono (Whisper's required format)
2. **SileroVADElement** (optional) - Detects voice activity to optimize API calls
3. **WhisperSTTElement** - Transcribes speech to text using OpenAI Whisper
4. **TranslateElement** - Translates text using GPT-4o-mini or Gemini
5. **UniversalTTSElement** - Synthesizes translated text to natural speech
6. **AudioResampleElement** - Converts TTS output to 48kHz (WebRTC standard)
7. **OpusEncodeElement** - Compresses audio for efficient transmission

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

## 📖 Usage Guide

### Basic Usage

1. **Start the server** - Run `go run main.go`
2. **Open the web interface** - Go to `http://localhost:8080`
3. **Click "Start Interpretation"** - Grant microphone permissions when prompted
4. **Speak** - Talk naturally in your source language
5. **Listen** - Hear the interpretation in real-time through your speakers

### Tips for Best Results

- **Use headphones** to prevent echo and feedback
- **Speak clearly** and at a moderate pace
- **Minimize background noise** for better recognition
- **Wait for interpretation** - There's a small delay while processing
- **Enable VAD** for better performance and lower API costs

## ⚙️ Configuration Options

### Language Codes

Common language codes (99+ languages supported):

| Language | Code | Example Use Case |
|----------|------|------------------|
| Chinese (Mandarin) | `zh` | Chinese → English business meetings |
| English | `en` | English → Spanish customer support |
| Japanese | `ja` | Japanese → English anime/content |
| Korean | `ko` | Korean → English K-pop/entertainment |
| Spanish | `es` | Spanish → English international calls |
| French | `fr` | French → English conferences |
| German | `de` | German → English technical docs |
| Russian | `ru` | Russian → English news/media |
| Arabic | `ar` | Arabic → English translation |

### Translation Providers

#### OpenAI (Recommended)
```env
TRANSLATE_PROVIDER=openai
TRANSLATE_MODEL=gpt-4o-mini  # Fast and economical
```

**Pros**: Excellent quality, low latency, good context handling
**Cons**: Requires API key, pay-per-use pricing

#### Google Gemini
```env
TRANSLATE_PROVIDER=gemini
GOOGLE_API_KEY=your-key-here
TRANSLATE_MODEL=gemini-2.0-flash-exp
```

**Pros**: High quality, competitive pricing, multimodal
**Cons**: Requires separate API key

### TTS Voice Selection

Choose from 6 different OpenAI voices:

```env
TTS_VOICE=alloy    # Neutral, balanced (default)
TTS_VOICE=echo     # Expressive, engaging
TTS_VOICE=fable    # British accent, storytelling
TTS_VOICE=onyx     # Deep, authoritative
TTS_VOICE=nova     # Energetic, lively
TTS_VOICE=shimmer  # Soft, gentle
```

### Environment Variables Reference

| Variable | Default | Description |
|----------|---------|-------------|
| `OPENAI_API_KEY` | - | **Required** - Your OpenAI API key |
| `GOOGLE_API_KEY` | - | Optional - For Gemini translation |
| `SOURCE_LANG` | `zh` | Source language code |
| `TARGET_LANG` | `en` | Target language code |
| `TRANSLATE_PROVIDER` | `openai` | Translation provider: `openai` or `gemini` |
| `TRANSLATE_MODEL` | `gpt-4o-mini` | Translation model |
| `TTS_VOICE` | `alloy` | TTS voice selection |
| `TTS_SPEED` | `1.0` | Speech speed (0.25-4.0) |
| `ENABLE_SUBTITLES` | `true` | Show text subtitles |

## 🎬 Use Cases

### International Business Meetings
```env
SOURCE_LANG=zh
TARGET_LANG=en
TTS_VOICE=onyx  # Professional, authoritative voice
```
Real-time interpretation for cross-border business communications.

### Language Learning
```env
SOURCE_LANG=en
TARGET_LANG=ja
TTS_VOICE=nova  # Clear, energetic voice
ENABLE_SUBTITLES=true  # See both languages
```
Practice speaking and hear native pronunciation instantly.

### Customer Support
```env
SOURCE_LANG=es
TARGET_LANG=en
TTS_VOICE=alloy  # Neutral, friendly voice
```
Provide multilingual customer support in real-time.

### Content Consumption
```env
SOURCE_LANG=ja
TARGET_LANG=en
TTS_VOICE=fable  # Engaging storytelling voice
```
Watch foreign language content with live audio interpretation.

### Accessibility
```env
SOURCE_LANG=auto  # Auto-detect
TARGET_LANG=en
ENABLE_SUBTITLES=true
```
Make any language content accessible.

## 🔧 Troubleshooting

### "Failed to connect to server"
- Ensure the server is running on port 8080
- Check that no other service is using port 8080
- Verify firewall settings allow local connections

### "Microphone access denied"
- Grant microphone permissions in browser settings
- Use HTTPS or localhost (required for `getUserMedia`)
- Try a different browser (Chrome/Firefox recommended)

### "No audio output"
- Check speaker/headphone volume
- Verify audio output device in system settings
- Try refreshing the page and restarting
- Check browser console for errors

### "OPENAI_API_KEY environment variable is required"
- Ensure `.env` file exists in the correct directory
- Verify the API key is correctly formatted
- Check that the `.env` file is loaded (run `go run main.go` from the example directory)

### "Translation not working"
- For OpenAI: Verify `OPENAI_API_KEY` is valid and has credit
- For Gemini: Verify `GOOGLE_API_KEY` is set and valid
- Check server logs for API error messages
- Ensure `TRANSLATE_PROVIDER` matches your API key

### High latency / Slow interpretation
- **Enable VAD** - Reduces unnecessary processing
- **Use gpt-4o-mini** - 10x faster than gpt-4o
- **Check internet speed** - Requires stable connection
- **Reduce TTS speed** - Set `TTS_SPEED=0.9` for slight speedup
- **Disable subtitles** - Set `ENABLE_SUBTITLES=false`

### Poor recognition quality
- **Speak clearly** and at moderate pace
- **Reduce background noise**
- **Use a quality microphone**
- **Check source language** - Ensure `SOURCE_LANG` is correct
- **Enable VAD** - Helps filter out noise

### "VAD not available"
This is normal if you haven't built with VAD support. To enable:
```bash
# Download VAD model
mkdir -p models
curl -L https://github.com/snakers4/silero-vad/raw/master/files/silero_vad.onnx -o models/silero_vad.onnx

# Build with VAD
go build -tags vad -o interpretation
./interpretation
```

See `pkg/elements/VAD_README.md` for detailed VAD setup instructions.

## 💰 API Costs (Approximate)

### Per Minute of Real-time Interpretation

| Service | Cost per Minute |
|---------|----------------|
| Whisper STT | $0.006 |
| GPT-4o-mini Translation | ~$0.001 |
| OpenAI TTS | ~$0.015 |
| **Total** | **~$0.022/min** |

**Example costs:**
- 10 minutes: ~$0.22
- 1 hour: ~$1.32
- Daily 1-hour meeting: ~$1.32/day

### Cost Optimization Tips

1. **Enable VAD** - Reduces API calls by 30-50%
2. **Use gpt-4o-mini** - Much cheaper than gpt-4o
3. **Adjust TTS speed** - Faster speech = less audio = lower cost
4. **Batch processing** - If not real-time, process in batches

## 🛠️ Development

### Project Structure

```
simultaneous-interpretation/
├── main.go              # Server and pipeline implementation
├── static/
│   └── index.html      # Web UI (HTML + CSS + JS)
├── .env.example        # Environment variable template
├── .env                # Your configuration (git-ignored)
└── README.md           # This file
```

### Extending the Application

#### Add Azure TTS Support

```go
// In createInterpretationPipeline(), replace OpenAI TTS with Azure
azureTTSElement := elements.NewAzureTTSElement()
p.AddElement(azureTTSElement)
p.Link(translateElement, azureTTSElement)
```

#### Custom Translation Prompt

```go
translateConfig := elements.TranslateConfig{
    // ...
    SystemPrompt: "You are a professional interpreter specializing in business meetings. Translate accurately while preserving tone and formality.",
}
```

#### Multiple Language Pairs

Create multiple pipelines for different language pairs and use a selector in the UI to switch between them.

#### Recording/Playback

Add file sink elements to record both input and output audio for later review.

## 📚 Related Documentation

- **Main Framework**: See root `README.md` for Realtime AI framework overview
- **CLAUDE.md**: Developer guidance for working with this codebase
- **VAD Setup**: `pkg/elements/VAD_README.md` for detailed VAD configuration
- **Translation Demo**: `examples/translation-demo/` for text-only translation
- **Whisper STT**: `examples/whisper-stt/` for speech recognition only

## 🔗 Credits

- **Realtime AI Framework**: [github.com/realtime-ai/realtime-ai](https://github.com/realtime-ai/realtime-ai)
- **OpenAI Whisper**: Speech recognition
- **OpenAI GPT-4o-mini**: Translation
- **OpenAI TTS**: Text-to-speech synthesis
- **Google Gemini**: Alternative translation provider
- **Silero VAD**: Voice activity detection
- **WebRTC**: Real-time audio streaming

## 📄 License

This example is part of the Realtime AI framework. See the main repository for license information.

## 🆘 Support

For issues and questions:
- **GitHub Issues**: [realtime-ai/realtime-ai/issues](https://github.com/realtime-ai/realtime-ai/issues)
- **Documentation**: See main repository README and CLAUDE.md
- **Examples**: Check other examples in `examples/` directory

---

**Enjoy real-time interpretation! 🌐🎧**

*Break down language barriers and communicate with anyone, anywhere, in real-time.*
