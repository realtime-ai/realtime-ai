# TTS System Design

## Overview

The TTS (Text-to-Speech) system in Realtime AI uses a **provider pattern** for extensibility. This design allows you to easily integrate multiple TTS services (OpenAI, Azure, ElevenLabs, etc.) and switch between them without changing your pipeline code.

## Architecture

### Components

```
┌─────────────────────────────────────────┐
│         Pipeline Architecture           │
├─────────────────────────────────────────┤
│                                         │
│  TextData → UniversalTTSElement → Audio│
│                    │                    │
│                    ↓                    │
│              TTSProvider                │
│                    │                    │
│         ┌──────────┴──────────┐        │
│         │          │          │        │
│   OpenAIProvider AzureProvider ...     │
└─────────────────────────────────────────┘
```

### Core Interfaces

#### 1. TTSProvider Interface (`pkg/tts/provider.go`)

The `TTSProvider` interface defines the contract that all TTS services must implement:

```go
type TTSProvider interface {
    Name() string
    Synthesize(ctx context.Context, req *SynthesizeRequest) (*SynthesizeResponse, error)
    GetSupportedVoices() []string
    GetDefaultVoice() string
    ValidateConfig() error
}
```

#### 2. UniversalTTSElement (`pkg/elements/universal_tts_element.go`)

A pipeline element that accepts any `TTSProvider`, allowing you to use different TTS services interchangeably.

#### 3. Provider Implementations

- **OpenAITTSProvider** (`pkg/tts/openai_provider.go`): OpenAI TTS API integration
- **AzureTTSProvider**: Can be refactored from existing `AzureTTSElement`
- Future providers: ElevenLabs, Google Cloud TTS, AWS Polly, etc.

## OpenAI TTS Integration

### Features

- **Models**:
  - `tts-1`: Standard quality, lower latency
  - `tts-1-hd`: High definition quality

- **Voices**: alloy, echo, fable, onyx, nova, shimmer

- **Formats**: PCM (raw), Opus, MP3, WAV, AAC, FLAC

- **Options**:
  - Speed control (0.25x to 4.0x)
  - Multiple output formats
  - Streaming support (future enhancement)

### Usage Examples

#### Basic Usage

```go
import (
    "github.com/realtime-ai/realtime-ai/pkg/elements"
    "github.com/realtime-ai/realtime-ai/pkg/tts"
)

// Create OpenAI provider
provider := tts.NewOpenAITTSProvider("your-api-key")

// Create TTS element
ttsElement := elements.NewUniversalTTSElement(provider)

// Configure
ttsElement.SetVoice("nova")
ttsElement.SetOption("speed", 1.2)

// Use in pipeline
pipeline.AddElement(ttsElement)
```

#### Using HD Model

```go
// Use HD model for higher quality
provider := tts.NewOpenAITTSProviderHD("your-api-key")
ttsElement := elements.NewUniversalTTSElement(provider)
```

#### In a Complete Pipeline

```go
// Create pipeline with OpenAI TTS
pipeline := pipeline.NewPipeline("my-pipeline")

// Add TTS element
openaiProvider := tts.NewOpenAITTSProvider("")
ttsElement := elements.NewUniversalTTSElement(openaiProvider)
ttsElement.SetVoice("alloy")

// Add other elements
playoutElement := elements.NewPlayoutSinkElement()

// Link elements
pipeline.AddElements([]Element{ttsElement, playoutElement})
pipeline.Link(ttsElement, playoutElement)

// Start pipeline
ttsElement.Start(ctx)
playoutElement.Start(ctx)

// Send text
msg := &pipeline.PipelineMessage{
    Type: pipeline.MsgTypeData,
    TextData: &pipeline.TextData{
        Data: []byte("Hello, world!"),
    },
}
pipeline.Push(msg)
```

#### Switching Between Providers

```go
// Easy to switch between different TTS providers
var provider tts.TTSProvider

switch config.TTSProvider {
case "openai":
    provider = tts.NewOpenAITTSProvider(apiKey)
case "azure":
    provider = tts.NewAzureTTSProvider(subscriptionKey, region)
case "elevenlabs":
    provider = tts.NewElevenLabsProvider(apiKey)
}

ttsElement := elements.NewUniversalTTSElement(provider)
```

## Extending with New Providers

To add a new TTS provider:

### Step 1: Implement TTSProvider Interface

```go
package tts

type MyTTSProvider struct {
    apiKey string
    // ... other fields
}

func NewMyTTSProvider(apiKey string) *MyTTSProvider {
    return &MyTTSProvider{apiKey: apiKey}
}

func (p *MyTTSProvider) Name() string {
    return "my-tts"
}

func (p *MyTTSProvider) Synthesize(ctx context.Context, req *SynthesizeRequest) (*SynthesizeResponse, error) {
    // Implementation here
    // 1. Call your TTS API
    // 2. Convert response to SynthesizeResponse format
    // 3. Return audio data with format info
}

func (p *MyTTSProvider) GetSupportedVoices() []string {
    return []string{"voice1", "voice2", "voice3"}
}

func (p *MyTTSProvider) GetDefaultVoice() string {
    return "voice1"
}

func (p *MyTTSProvider) ValidateConfig() error {
    if p.apiKey == "" {
        return fmt.Errorf("API key is required")
    }
    return nil
}
```

### Step 2: Use with UniversalTTSElement

```go
provider := tts.NewMyTTSProvider(apiKey)
ttsElement := elements.NewUniversalTTSElement(provider)
// Use in pipeline as normal
```

That's it! No need to create a new element type.

## Advanced Features

### Streaming Support (Future)

For providers that support streaming TTS, you can implement the `StreamingTTSProvider` interface:

```go
type StreamingTTSProvider interface {
    TTSProvider
    StreamSynthesize(ctx context.Context, req *SynthesizeRequest) (<-chan []byte, <-chan error)
}
```

### Configuration via Properties

The `UniversalTTSElement` supports the pipeline property system:

```go
// Set properties programmatically
ttsElement.SetProperty("voice", "nova")
ttsElement.SetProperty("language", "en-US")

// Get property values
voice, _ := ttsElement.GetProperty("voice")
```

## Comparison with Legacy Approach

### Old Approach (AzureTTSElement)

- Each TTS service requires a separate element class
- Tight coupling between element and TTS API
- Difficult to test and extend
- Code duplication

```go
// Need separate classes for each provider
azureTTS := elements.NewAzureTTSElement()
// Can't easily switch providers
```

### New Approach (Provider Pattern)

- Single `UniversalTTSElement` works with all providers
- Loose coupling via interface
- Easy to test with mock providers
- Reusable code

```go
// One element class, multiple providers
provider := tts.NewOpenAITTSProvider(apiKey)
ttsElement := elements.NewUniversalTTSElement(provider)

// Easy to swap providers
provider2 := tts.NewAzureTTSProvider(key, region)
ttsElement2 := elements.NewUniversalTTSElement(provider2)
```

## Migration Guide

If you're using the old `AzureTTSElement`, you have two options:

### Option 1: Continue Using Legacy Element

The old `AzureTTSElement` continues to work:

```go
azureTTS := elements.NewAzureTTSElement()
```

### Option 2: Migrate to Provider Pattern

1. Refactor Azure code into `AzureTTSProvider`
2. Use `UniversalTTSElement`:

```go
provider := tts.NewAzureTTSProvider(key, region)
ttsElement := elements.NewUniversalTTSElement(provider)
```

Benefits: Better testability, easier to add features, consistent API

## Environment Variables

### OpenAI TTS

```bash
export OPENAI_API_KEY=sk-...
```

### Azure TTS (existing)

```bash
export AZURE_SPEECH_KEY=your-key
export AZURE_SPEECH_REGION=your-region
```

## Testing

### Unit Testing with Mock Provider

```go
type MockTTSProvider struct{}

func (m *MockTTSProvider) Name() string { return "mock" }

func (m *MockTTSProvider) Synthesize(ctx context.Context, req *SynthesizeRequest) (*SynthesizeResponse, error) {
    // Return mock audio data
    return &SynthesizeResponse{
        AudioData: []byte{0, 1, 2, 3},
        AudioFormat: AudioFormat{
            SampleRate: 24000,
            Channels: 1,
        },
    }, nil
}

// ... implement other methods

// In test
func TestTTSElement(t *testing.T) {
    provider := &MockTTSProvider{}
    element := elements.NewUniversalTTSElement(provider)
    // Test element behavior
}
```

## Best Practices

1. **Always validate configuration** before starting the element
2. **Use context for cancellation** in long-running synthesis operations
3. **Handle errors gracefully** and publish to the event bus
4. **Set appropriate buffer sizes** for the element channels
5. **Clean up resources** in the Stop() method
6. **Use environment variables** for API keys (never hardcode)

## API References

### OpenAI TTS API

- Endpoint: `https://api.openai.com/v1/audio/speech`
- Docs: https://platform.openai.com/docs/guides/text-to-speech
- Rate limits: Depends on your tier
- Pricing: $15 per 1M characters (standard), $30 per 1M (HD)

## Example Projects

- `examples/openai-tts/main.go`: Basic OpenAI TTS demo
- More examples coming soon

## Future Enhancements

1. **Streaming TTS**: Real-time audio generation for lower latency
2. **Voice cloning**: Support for custom voice models
3. **SSML support**: Advanced speech markup
4. **Batch synthesis**: Process multiple texts efficiently
5. **Caching**: Cache frequently used phrases
6. **More providers**: ElevenLabs, Google Cloud TTS, AWS Polly, etc.
