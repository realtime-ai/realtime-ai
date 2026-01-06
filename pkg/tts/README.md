# TTS Package

A flexible, provider-based Text-to-Speech system for Realtime AI.

## Quick Start

### OpenAI TTS (gpt-4o-mini-tts)

```go
import "github.com/realtime-ai/realtime-ai/pkg/tts"

// Create provider
provider := tts.NewOpenAITTSProvider("your-api-key")

// Set voice instructions (optional)
provider.SetInstructions("Speak in a cheerful and positive tone")

// Synthesize speech
req := &tts.SynthesizeRequest{
    Text:  "Hello, world!",
    Voice: "coral",
}
resp, err := provider.Synthesize(context.Background(), req)
if err != nil {
    log.Fatal(err)
}

// resp.AudioData contains the synthesized audio
// resp.AudioFormat contains format information
```

### With Pipeline Element

```go
import (
    "github.com/realtime-ai/realtime-ai/pkg/tts"
    "github.com/realtime-ai/realtime-ai/pkg/elements"
)

// Create provider and element
provider := tts.NewOpenAITTSProvider("")
provider.SetInstructions("Speak calmly and clearly")
ttsElement := elements.NewUniversalTTSElement(provider)

// Configure
ttsElement.SetVoice("marin")
ttsElement.SetOption("speed", 1.2)

// Use in pipeline
pipeline.AddElement(ttsElement)
```

### Streaming Mode

```go
// Use SSE streaming for lower latency
audioChan, errChan := provider.StreamSynthesize(ctx, req)

for chunk := range audioChan {
    // Process audio chunk in real-time
    processAudio(chunk)
}
```

## OpenAI TTS

### Model

- `gpt-4o-mini-tts`: High quality with instructions support (default)
- `gpt-4o-mini-tts-2025-12-15`: Latest snapshot with 35% lower WER

### Voices

| Voice | Description |
|-------|-------------|
| alloy | Neutral and balanced |
| ash | Clear and precise |
| ballad | Melodic and warm |
| coral | Natural and conversational (default) |
| echo | More expressive |
| fable | British accent |
| nova | Energetic and lively |
| onyx | Deep and authoritative |
| sage | Calm and thoughtful |
| shimmer | Soft and gentle |
| verse | Versatile and adaptive |
| **marin** | High quality, recommended |
| **cedar** | High quality, recommended |

### Instructions

Control the voice style with natural language instructions:

```go
provider.SetInstructions("Speak in a cheerful and enthusiastic tone")
provider.SetInstructions("Talk like a sympathetic customer service agent")
provider.SetInstructions("Use a calm, professional tone with clear enunciation")
```

Controllable aspects:
- Accent
- Emotional range
- Intonation
- Speed of speech
- Tone
- Whispering

### Formats

- `pcm`: Raw PCM audio (default, best for pipelines)
- `opus`: Opus codec
- `mp3`: MP3 format
- `wav`: WAV format
- `aac`: AAC format
- `flac`: FLAC lossless

### Options

```go
// Speech speed (0.25 to 4.0)
ttsElement.SetOption("speed", 1.5)

// Output format
ttsElement.SetOption("format", "opus")

// Voice instructions (per-request override)
ttsElement.SetOption("instructions", "Speak excitedly")
```

## Creating a Custom Provider

```go
package tts

type MyProvider struct {
    apiKey string
}

func NewMyProvider(apiKey string) *MyProvider {
    return &MyProvider{apiKey: apiKey}
}

func (p *MyProvider) Name() string {
    return "my-provider"
}

func (p *MyProvider) Synthesize(ctx context.Context, req *SynthesizeRequest) (*SynthesizeResponse, error) {
    // Call your TTS API
    audioData, err := callMyTTSAPI(req.Text, req.Voice)
    if err != nil {
        return nil, err
    }

    return &SynthesizeResponse{
        AudioData: audioData,
        AudioFormat: AudioFormat{
            SampleRate: 24000,
            Channels:   1,
            MediaType:  "audio/pcm",
            Encoding:   "pcm_s16le",
        },
    }, nil
}

func (p *MyProvider) GetSupportedVoices() []string {
    return []string{"voice1", "voice2"}
}

func (p *MyProvider) GetDefaultVoice() string {
    return "voice1"
}

func (p *MyProvider) ValidateConfig() error {
    if p.apiKey == "" {
        return fmt.Errorf("API key required")
    }
    return nil
}
```

## Environment Variables

```bash
# OpenAI
export OPENAI_API_KEY=sk-...

# Optional: Custom base URL
export OPENAI_BASE_URL=https://your-proxy.com/v1
```

## Testing

See `examples/openai-tts/main.go` for a complete working example.

```bash
export OPENAI_API_KEY=your-key
go run examples/openai-tts/main.go
```

## Architecture

```
TTSProvider (interface)
    ├── OpenAITTSProvider (gpt-4o-mini-tts, streaming)
    ├── ElevenLabsHTTPTTSProvider
    ├── ElevenLabsWSTTSProvider (WebSocket streaming)
    └── Your custom provider

StreamingTTSProvider (interface, extends TTSProvider)
    └── StreamSynthesize() for real-time audio

UniversalTTSElement
    └── Accepts any TTSProvider
```

## Documentation

See `docs/tts-design.md` for detailed design documentation.

## License

See project LICENSE file.
