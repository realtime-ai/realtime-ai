# Whisper STT Example

This example demonstrates real-time speech-to-text using OpenAI's Whisper API with optional Voice Activity Detection (VAD).

## Features

- 🎤 Real-time speech-to-text using OpenAI Whisper
- 🔊 Voice Activity Detection using Silero VAD (optional)
- 🌐 WebRTC-based audio streaming
- 📝 Event-driven architecture with live transcription updates
- 🔧 Modular pipeline design

## Architecture

```
┌─────────────────┐
│  Web Client     │ (Microphone)
└────────┬────────┘
         │ WebRTC Audio Stream
         ▼
┌─────────────────────────────┐
│  AudioResampleElement       │
│  Convert to 16kHz mono      │
└────────┬────────────────────┘
         │
         ▼
┌─────────────────────────────┐
│  SileroVADElement           │ (Optional)
│  - Detects speech presence  │
│  - Emits events             │
│  - Mode: Passthrough        │
└────────┬────────────────────┘
         │
         ▼
┌─────────────────────────────┐
│  WhisperSTTElement          │
│  - Receives audio           │
│  - Listens to VAD events    │
│  - Calls Whisper API        │
│  - Emits transcriptions     │
└────────┬────────────────────┘
         │
         ▼
┌─────────────────────────────┐
│  TextData Output            │
│  Sent back to client        │
└─────────────────────────────┘
```

## Prerequisites

1. **OpenAI API Key**
   ```bash
   export OPENAI_API_KEY=sk-...
   ```

2. **Go Dependencies**
   ```bash
   go mod download
   ```

3. **Optional: VAD Support**

   For Voice Activity Detection, you need:
   - ONNX Runtime library installed
   - Silero VAD model file

   See `pkg/elements/VAD_README.md` for setup instructions.

## Running the Example

### Without VAD (Simpler Setup)

```bash
# Set API key
export OPENAI_API_KEY=sk-...

# Run example
go run examples/whisper-stt/main.go

# Open browser
open http://localhost:8080
```

### With VAD (Recommended for Production)

```bash
# Set API key
export OPENAI_API_KEY=sk-...

# Build with VAD support
go run -tags vad examples/whisper-stt/main.go

# Open browser
open http://localhost:8080
```

## Configuration

You can modify the pipeline configuration in `main.go`:

### Whisper Configuration

```go
whisperConfig := elements.WhisperSTTConfig{
    APIKey:               os.Getenv("OPENAI_API_KEY"),
    Language:             "auto",  // or "en", "zh", "es", etc.
    Model:                "whisper-1",
    EnablePartialResults: false,   // true for interim results
    VADEnabled:           true,    // enable VAD integration
    SampleRate:           16000,
    Channels:             1,
    BitsPerSample:        16,
    Prompt:               "",      // optional context
}
```

### VAD Configuration

```go
vadConfig := elements.SileroVADConfig{
    ModelPath:       "models/silero_vad.onnx",
    Threshold:       0.5,    // 0.0-1.0, higher = more selective
    MinSilenceDurMs: 300,    // min silence before speech end
    SpeechPadMs:     30,     // padding around speech
    Mode:            elements.VADModePassthrough,
}
```

## How It Works

### Without VAD

1. Audio is continuously streamed from the browser via WebRTC
2. Audio is resampled to 16kHz mono
3. Audio is buffered in 5-10 second chunks
4. Each chunk is sent to Whisper API for transcription
5. Results are sent back to the client

### With VAD

1. Audio is continuously streamed from the browser via WebRTC
2. Audio is resampled to 16kHz mono
3. VAD analyzes audio and emits `EventVADSpeechStart` / `EventVADSpeechEnd`
4. WhisperSTT **only processes audio segments where speech is detected**
5. When speech ends, buffered audio is sent to Whisper API
6. Results are sent back to the client

**Benefits of VAD:**
- ✅ Reduces API calls (only process speech, not silence)
- ✅ Lower costs
- ✅ Better accuracy (focused on actual speech)
- ✅ Faster results (smaller audio chunks)

## Events

The example subscribes to pipeline events for monitoring:

### VAD Events
- `EventVADSpeechStart` - Speech detected, recording started
- `EventVADSpeechEnd` - Speech ended, processing audio

### STT Events
- `EventPartialResult` - Interim transcription (if enabled)
- `EventFinalResult` - Final transcription

## Expected Output

```
=== Whisper STT Example with VAD Integration ===
This example demonstrates:
  - Speech-to-Text using OpenAI Whisper
  - Voice Activity Detection (VAD) using Silero
  - Real-time audio processing pipeline

Starting HTTP server on :8080
Open http://localhost:8080 in your browser
Added: AudioResampleElement (16kHz, mono)
Added: SileroVADElement (Passthrough mode, emits events)
Added: WhisperSTTElement (Language: auto, VAD: true)
Pipeline configured successfully

New connection created: abc123...
Pipeline started successfully

🎤 Speech detected - recording...
🔇 Speech ended - processing...
✅ [Final] Hello, this is a test of the Whisper speech recognition system.
📨 Sending transcription to client: Hello, this is a test of the Whisper speech recognition system.

🎤 Speech detected - recording...
🔇 Speech ended - processing...
✅ [Final] It's working great!
📨 Sending transcription to client: It's working great!
```

## Supported Languages

Whisper supports 99+ languages. Use the `Language` config:

- `"auto"` - Auto-detect (default)
- `"en"` - English
- `"zh"` - Chinese
- `"es"` - Spanish
- `"fr"` - French
- `"de"` - German
- `"ja"` - Japanese
- `"ko"` - Korean
- And many more...

See [Whisper Language Support](https://github.com/openai/whisper#available-models-and-languages)

## Troubleshooting

### "OPENAI_API_KEY environment variable is required"
Set your API key:
```bash
export OPENAI_API_KEY=sk-...
```

### "VAD not available"
This is normal if you didn't build with `-tags vad`. The example will work without VAD, but won't have the optimization benefits.

To enable VAD:
1. Install ONNX Runtime (see `pkg/elements/VAD_README.md`)
2. Download Silero VAD model
3. Build with: `go run -tags vad examples/whisper-stt/main.go`

### "Failed to create Whisper STT element"
Check:
- API key is valid
- Internet connection is available
- No firewall blocking OpenAI API

### High latency
This is expected - Whisper API is not real-time. Typical latency:
- Without VAD: 3-10 seconds (depends on buffer size)
- With VAD: 1-5 seconds (smaller chunks)

For lower latency, consider:
- Reducing audio chunk size (in `whisper.go`)
- Using streaming STT providers (Azure, Google Cloud)

## Cost Considerations

OpenAI Whisper API pricing (as of 2024):
- $0.006 per minute of audio

**Cost optimization tips:**
1. Enable VAD - only transcribe speech, not silence
2. Use appropriate `MinSilenceDurMs` to avoid splitting words
3. Monitor usage with OpenAI dashboard

## Next Steps

- Integrate with LLM for conversational AI
- Add TTS for voice responses
- Implement custom wake word detection
- Add speaker diarization
- Create multi-language support

## See Also

- [ASR Package Documentation](../../pkg/asr/README.md)
- [VAD Setup Guide](../../pkg/elements/VAD_README.md)
- [OpenAI Whisper API](https://platform.openai.com/docs/guides/speech-to-text)
