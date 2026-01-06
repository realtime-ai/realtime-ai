# TTS System Design

## Overview

The TTS (Text-to-Speech) system in Realtime AI uses a **provider pattern** for extensibility. This design allows you to easily integrate multiple TTS services (OpenAI, ElevenLabs, etc.) and switch between them without changing your pipeline code.

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
│         TTSProvider / StreamingTTSProvider
│                    │                    │
│         ┌──────────┴──────────┐        │
│         │          │          │        │
│   OpenAIProvider ElevenLabs  ...       │
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

#### 2. StreamingTTSProvider Interface

For providers that support streaming audio:

```go
type StreamingTTSProvider interface {
    TTSProvider
    StreamSynthesize(ctx context.Context, req *SynthesizeRequest) (<-chan []byte, <-chan error)
}
```

#### 3. UniversalTTSElement (`pkg/elements/universal_tts_element.go`)

A pipeline element that accepts any `TTSProvider`, allowing you to use different TTS services interchangeably.

#### 4. Provider Implementations

- **OpenAITTSProvider** (`pkg/tts/openai_provider.go`): OpenAI gpt-4o-mini-tts with SSE streaming
- **ElevenLabsHTTPTTSProvider**: ElevenLabs HTTP streaming
- **ElevenLabsWSTTSProvider**: ElevenLabs WebSocket low-latency streaming

## OpenAI TTS Integration

### Features

- **Model**: `gpt-4o-mini-tts` (default), `gpt-4o-mini-tts-2025-12-15` (latest)

- **Voices**: alloy, ash, ballad, coral, echo, fable, nova, onyx, sage, shimmer, verse, marin, cedar
  - Recommended: marin, cedar (highest quality)

- **Formats**: PCM (raw), Opus, MP3, WAV, AAC, FLAC

- **Options**:
  - Speed control (0.25x to 4.0x)
  - Voice instructions for tone/style control
  - SSE streaming support

### Voice Instructions

The gpt-4o-mini-tts model supports voice instructions to control:
- Accent
- Emotional range
- Intonation
- Speed of speech
- Tone
- Whispering

```go
provider.SetInstructions("Speak in a cheerful and enthusiastic tone")
provider.SetInstructions("Talk like a sympathetic customer service agent")
```

### Usage Examples

#### Basic Usage

```go
import (
    "github.com/realtime-ai/realtime-ai/pkg/elements"
    "github.com/realtime-ai/realtime-ai/pkg/tts"
)

// Create OpenAI provider
provider := tts.NewOpenAITTSProvider("your-api-key")

// Set voice instructions
provider.SetInstructions("Speak calmly and clearly")

// Create TTS element
ttsElement := elements.NewUniversalTTSElement(provider)

// Configure
ttsElement.SetVoice("coral")
ttsElement.SetOption("speed", 1.2)

// Use in pipeline
pipeline.AddElement(ttsElement)
```

#### Streaming Mode

```go
// Use SSE streaming for real-time audio
provider := tts.NewOpenAITTSProvider(apiKey)

req := &tts.SynthesizeRequest{
    Text:  "Hello, this is streaming audio!",
    Voice: "marin",
}

audioChan, errChan := provider.StreamSynthesize(ctx, req)

for {
    select {
    case chunk, ok := <-audioChan:
        if !ok {
            return // Stream complete
        }
        processAudioChunk(chunk)
    case err := <-errChan:
        if err != nil {
            log.Printf("Error: %v", err)
        }
    }
}
```

#### In a Complete Pipeline

```go
// Create pipeline with OpenAI TTS
pipeline := pipeline.NewPipeline("my-pipeline")

// Add TTS element
openaiProvider := tts.NewOpenAITTSProvider("")
openaiProvider.SetInstructions("Speak in a friendly tone")
ttsElement := elements.NewUniversalTTSElement(openaiProvider)
ttsElement.SetVoice("coral")

// Add other elements
audioPacerElement := elements.NewAudioPacerSinkElement()

// Link elements
pipeline.AddElements([]Element{ttsElement, audioPacerElement})
pipeline.Link(ttsElement, audioPacerElement)

// Start pipeline
ttsElement.Start(ctx)
audioPacerElement.Start(ctx)

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
    p := tts.NewOpenAITTSProvider(apiKey)
    p.SetInstructions("Speak naturally")
    provider = p
case "elevenlabs-http":
    provider = tts.NewElevenLabsHTTPTTSProvider(config)
case "elevenlabs-ws":
    provider, _ = tts.NewElevenLabsWSTTSProvider(config)
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

### Step 2: (Optional) Implement StreamingTTSProvider

```go
func (p *MyTTSProvider) StreamSynthesize(ctx context.Context, req *SynthesizeRequest) (<-chan []byte, <-chan error) {
    audioChan := make(chan []byte, 100)
    errChan := make(chan error, 1)

    go func() {
        defer close(audioChan)
        defer close(errChan)
        // Streaming implementation
    }()

    return audioChan, errChan
}

// Verify interface implementation
var _ StreamingTTSProvider = (*MyTTSProvider)(nil)
```

### Step 3: Use with UniversalTTSElement

```go
provider := tts.NewMyTTSProvider(apiKey)
ttsElement := elements.NewUniversalTTSElement(provider)
// Use in pipeline as normal
```

That's it! No need to create a new element type.

## Advanced Features

### Configuration via Properties

The `UniversalTTSElement` supports the pipeline property system:

```go
// Set properties programmatically
ttsElement.SetProperty("voice", "coral")
ttsElement.SetProperty("language", "en-US")

// Get property values
voice, _ := ttsElement.GetProperty("voice")
```

## Provider Comparison

| Provider | Streaming | Latency | Quality | Instructions |
|----------|-----------|---------|---------|--------------|
| OpenAI gpt-4o-mini-tts | SSE | Medium | High | Yes |
| ElevenLabs HTTP | HTTP Chunked | Medium | High | No |
| ElevenLabs WS | WebSocket | Low | High | No |

## Environment Variables

### OpenAI TTS

```bash
export OPENAI_API_KEY=sk-...
export OPENAI_BASE_URL=https://your-proxy.com/v1  # Optional
```

### ElevenLabs

```bash
export ELEVENLABS_API_KEY=...
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
7. **Use streaming** for lower latency in real-time applications

## API References

### OpenAI TTS API

- Endpoint: `https://api.openai.com/v1/audio/speech`
- Docs: https://platform.openai.com/docs/guides/text-to-speech
- Model: gpt-4o-mini-tts
- Streaming: SSE (stream_format: "sse")

## Example Projects

- `examples/openai-tts/main.go`: Basic OpenAI TTS demo with instructions
