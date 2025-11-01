# TTS Package

A flexible, provider-based Text-to-Speech system for Realtime AI.

## Quick Start

### OpenAI TTS

```go
import "github.com/realtime-ai/realtime-ai/pkg/tts"

// Create provider
provider := tts.NewOpenAITTSProvider("your-api-key")

// Synthesize speech
req := &tts.SynthesizeRequest{
    Text:  "Hello, world!",
    Voice: "nova",
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
ttsElement := elements.NewUniversalTTSElement(provider)

// Configure
ttsElement.SetVoice("alloy")
ttsElement.SetOption("speed", 1.2)

// Use in pipeline
pipeline.AddElement(ttsElement)
```

## OpenAI TTS

### Voices

| Voice | Description |
|-------|-------------|
| alloy | Neutral and balanced |
| echo | More expressive |
| fable | British accent |
| onyx | Deep and authoritative |
| nova | Energetic and lively |
| shimmer | Soft and gentle |

### Models

- `tts-1`: Standard quality (default)
- `tts-1-hd`: High definition quality

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

# Azure (for existing AzureTTSElement)
export AZURE_SPEECH_KEY=...
export AZURE_SPEECH_REGION=...
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
    ├── OpenAITTSProvider
    ├── AzureTTSProvider (future)
    ├── ElevenLabsProvider (future)
    └── Your custom provider

UniversalTTSElement
    └── Accepts any TTSProvider
```

## Documentation

See `docs/tts-design.md` for detailed design documentation.

## License

See project LICENSE file.
