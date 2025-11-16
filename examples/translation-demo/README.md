# Real-time Transcription + Translation Demo

This demo showcases a complete real-time speech transcription and translation system using the Realtime AI framework. Speak in one language and see instant transcription and translation to another language with live bilingual subtitles.

## Features

- 🎤 **Real-time Speech Recognition** - Using OpenAI Whisper API
- 🌐 **Instant Translation** - Powered by GPT-4o-mini or Gemini
- 🔊 **Voice Activity Detection** - Optimized with Silero VAD (optional)
- 💬 **Live Bilingual Subtitles** - See both original and translated text in real-time
- 🌍 **Multi-language Support** - Support for Chinese, English, Japanese, Korean, Spanish, French, German, Russian, and more
- ⚡ **Low Latency** - WebRTC-based audio streaming for minimal delay

## Architecture

The demo uses a modular pipeline architecture:

```
┌─────────────┐      ┌──────────┐      ┌───────────┐      ┌──────────┐      ┌─────────┐
│  WebRTC     │──>───│  Audio   │──>───│  Whisper  │──>───│ Translate│──>───│ Client  │
│  Client     │      │ Resample │      │    STT    │      │ Element  │      │ (Text)  │
│  (Browser)  │      │  16kHz   │      │           │      │          │      │         │
└─────────────┘      └──────────┘      └───────────┘      └──────────┘      └─────────┘
                                              │                   │
                                              │                   │
                                         [Event Bus]          [Event Bus]
                                      Transcription Events  Translation Events
```

### Pipeline Elements

1. **AudioResampleElement** - Resamples audio to 16kHz mono (required for Whisper)
2. **SileroVADElement** (optional) - Detects voice activity to optimize API calls
3. **WhisperSTTElement** - Transcribes speech to text using OpenAI Whisper
4. **TranslateElement** - Translates text using GPT-4o-mini or Gemini

## Prerequisites

- Go 1.21 or later
- OpenAI API key (for Whisper STT and GPT translation)
- OR Google API key (for Gemini translation)
- Microphone-enabled browser (Chrome, Firefox, Safari, Edge)

## Installation

1. **Clone the repository**
   ```bash
   cd realtime-ai/examples/translation-demo
   ```

2. **Install dependencies**
   ```bash
   go mod download
   ```

3. **Set up environment variables**
   ```bash
   # Required for Whisper STT
   export OPENAI_API_KEY=your_openai_api_key_here

   # Optional: For Gemini translation (if using TRANSLATE_PROVIDER=gemini)
   export GOOGLE_API_KEY=your_google_api_key_here

   # Optional: Configure languages (defaults shown)
   export SOURCE_LANG=zh           # Source language: zh, en, ja, ko, etc.
   export TARGET_LANG=en           # Target language: en, zh, ja, ko, etc.

   # Optional: Choose translation provider
   export TRANSLATE_PROVIDER=openai  # "openai" or "gemini"
   export TRANSLATE_MODEL=gpt-4o-mini # Or "gemini-2.0-flash-exp"
   ```

   Or create a `.env` file:
   ```env
   OPENAI_API_KEY=sk-...
   SOURCE_LANG=zh
   TARGET_LANG=en
   TRANSLATE_PROVIDER=openai
   ```

## Usage

### Running the Demo

1. **Start the server**
   ```bash
   go run main.go
   ```

   You should see output like:
   ```
   === Real-time Transcription + Translation Demo ===
   This demo demonstrates:
     - Speech-to-Text using OpenAI Whisper
     - Real-time translation using GPT/Gemini
     - Voice Activity Detection (VAD) using Silero
     - Live bilingual subtitles

   Configuration:
     Source Language: zh
     Target Language: en
     Translation Provider: openai

   Starting HTTP server on :8080
   Open http://localhost:8080 in your browser
   ```

2. **Open the web interface**
   - Navigate to `http://localhost:8080` in your browser
   - Click "Start Recording" to begin
   - Grant microphone permissions when prompted
   - Speak in your source language
   - Watch real-time transcription and translation appear!

### With VAD Support (Optional)

For better performance and reduced API costs, you can enable Voice Activity Detection:

1. **Download Silero VAD model**
   ```bash
   mkdir -p models
   curl -L https://github.com/snakers4/silero-vad/raw/master/files/silero_vad.onnx -o models/silero_vad.onnx
   ```

2. **Build with VAD support**
   ```bash
   go build -tags vad -o translation-demo
   ./translation-demo
   ```

See `pkg/elements/VAD_README.md` for detailed VAD setup instructions.

## Configuration Options

### Language Codes

| Language | Code |
|----------|------|
| Chinese  | `zh` |
| English  | `en` |
| Japanese | `ja` |
| Korean   | `ko` |
| Spanish  | `es` |
| French   | `fr` |
| German   | `de` |
| Russian  | `ru` |
| Arabic   | `ar` |

### Translation Providers

#### OpenAI (Recommended)
- **Provider**: `openai`
- **Models**: `gpt-4o-mini` (fast, cheap), `gpt-4o` (highest quality)
- **Pros**: Excellent translation quality, low latency, good context handling
- **Cons**: Requires API key, costs per request

#### Google Gemini
- **Provider**: `gemini`
- **Models**: `gemini-2.0-flash-exp`, `gemini-1.5-flash`
- **Pros**: High quality, competitive pricing, multimodal capabilities
- **Cons**: Requires Google API key

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `OPENAI_API_KEY` | - | Required for Whisper STT and OpenAI translation |
| `GOOGLE_API_KEY` | - | Required if using Gemini translation |
| `SOURCE_LANG` | `zh` | Source language code |
| `TARGET_LANG` | `en` | Target language code |
| `TRANSLATE_PROVIDER` | `openai` | Translation provider: `openai` or `gemini` |
| `TRANSLATE_MODEL` | `gpt-4o-mini` | Model to use for translation |

## How It Works

### 1. Audio Capture
The browser captures microphone audio using WebRTC's `getUserMedia()` API and streams it to the server via a WebRTC peer connection.

### 2. Audio Processing
The server receives raw audio data and:
- Resamples it to 16kHz mono (Whisper's required format)
- Optionally runs it through Silero VAD to detect speech segments
- Buffers audio chunks for transcription

### 3. Speech-to-Text
When sufficient audio is collected (or VAD detects speech end):
- Audio is sent to OpenAI Whisper API
- Transcription result is returned in the source language
- Original text is sent back to the browser and published to event bus

### 4. Translation
When a final transcription is received:
- Text is sent to the TranslateElement
- Translation API (OpenAI or Gemini) translates to target language
- Translated text is sent back to the browser

### 5. Display
The web interface receives two streams of text:
- **Original**: Real-time transcription in source language
- **Translation**: Translated text in target language

Both are displayed as live subtitles with visual indicators.

## Example Use Cases

### Language Learning
- **Setup**: English → Japanese
- **Use**: Practice pronunciation and see immediate translation feedback

### International Meetings
- **Setup**: Chinese → English
- **Use**: Real-time subtitles for cross-language communication

### Content Creation
- **Setup**: Spanish → English
- **Use**: Live translation for podcasts or streaming

### Accessibility
- **Setup**: Any language → Your language
- **Use**: Make foreign language content accessible

## Troubleshooting

### "Failed to connect to server"
- Ensure the server is running on port 8080
- Check firewall settings
- Verify WebRTC is supported in your browser

### "Microphone access denied"
- Grant microphone permissions in browser settings
- Use HTTPS or localhost (required for getUserMedia)

### "No transcription appearing"
- Verify `OPENAI_API_KEY` is set correctly
- Check server logs for API errors
- Ensure you're speaking in the configured source language
- Try speaking louder or closer to the microphone

### "Translation not working"
- For OpenAI: Verify `OPENAI_API_KEY` is valid
- For Gemini: Verify `GOOGLE_API_KEY` is set and valid
- Check server logs for API errors
- Verify `TRANSLATE_PROVIDER` matches your API key

### High latency
- Enable VAD to reduce unnecessary API calls
- Use `gpt-4o-mini` instead of `gpt-4o` for faster translation
- Check your internet connection speed
- Consider using streaming mode (set `Streaming: true` in TranslateConfig)

## Performance Tips

1. **Enable VAD** - Reduces API calls by only processing actual speech
2. **Use gpt-4o-mini** - 10x cheaper and faster than gpt-4o with comparable quality
3. **Optimize chunk size** - Adjust `WhisperSTTConfig` buffer settings
4. **Enable streaming** - Set `TranslateConfig.Streaming = true` for lower latency
5. **Cache common phrases** - Implement translation caching for repeated phrases

## API Costs (Approximate)

### Per Minute of Audio

| Service | Cost |
|---------|------|
| Whisper STT | $0.006 per minute |
| GPT-4o-mini Translation | ~$0.001 per translation |
| Gemini Translation | ~$0.0005 per translation |

**Total cost**: ~$0.01 per minute of real-time translation with OpenAI

## Development

### Project Structure
```
translation-demo/
├── main.go              # Server and pipeline setup
├── static/
│   └── index.html      # Web UI (HTML + CSS + JS)
├── README.md           # This file
└── .env               # Environment variables (optional)
```

### Extending the Demo

**Add TTS output**:
```go
// Add Azure TTS element to pipeline
ttsElement := elements.NewAzureTTSElement()
p.AddElement(ttsElement)
p.Link(translateElement, ttsElement)
```

**Add custom translation prompt**:
```go
translateConfig := elements.TranslateConfig{
    // ...
    SystemPrompt: "You are a professional interpreter. Translate the following text with cultural context and idioms preserved.",
}
```

**Support multiple language pairs simultaneously**:
Create multiple pipelines with different language configurations and route audio based on user selection.

## License

This demo is part of the Realtime AI framework. See the main repository for license information.

## Credits

- **Realtime AI Framework**: [github.com/realtime-ai/realtime-ai](https://github.com/realtime-ai/realtime-ai)
- **OpenAI Whisper**: Speech recognition
- **OpenAI GPT-4o-mini**: Translation
- **Google Gemini**: Alternative translation
- **Silero VAD**: Voice activity detection

## Support

For issues and questions:
- GitHub Issues: [realtime-ai/realtime-ai/issues](https://github.com/realtime-ai/realtime-ai/issues)
- Documentation: See main repository README and CLAUDE.md

---

**Enjoy real-time translation! 🌐**
