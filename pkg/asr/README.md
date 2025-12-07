# ASR (Automatic Speech Recognition) Package

This package provides a unified interface for integrating various ASR (Automatic Speech Recognition) systems into the Realtime AI framework.

## Overview

The ASR package is designed with extensibility in mind, allowing easy integration of different speech recognition providers:

- **OpenAI Whisper** - Production-ready implementation (buffered streaming)
- **Qwen Realtime** - Alibaba Cloud DashScope real-time ASR (true streaming via WebSocket)
- **Azure Speech Services** - Already integrated (see `azure_stt_element.go`)
- **Google Cloud Speech** - Future implementation
- **Other providers** - Easy to add by implementing the `Provider` interface

## Architecture

### Core Interface

The package defines a `Provider` interface that all ASR implementations must satisfy:

```go
type Provider interface {
    Name() string
    Recognize(ctx context.Context, audio io.Reader, audioConfig AudioConfig, config RecognitionConfig) (*RecognitionResult, error)
    StreamingRecognize(ctx context.Context, audioConfig AudioConfig, config RecognitionConfig) (StreamingRecognizer, error)
    SupportsStreaming() bool
    SupportedLanguages() []string
    Close() error
}
```

### Key Components

#### 1. Provider
The main interface for ASR systems. Each provider (Whisper, Google Cloud, etc.) implements this interface.

#### 2. StreamingRecognizer
For continuous real-time recognition:
```go
type StreamingRecognizer interface {
    SendAudio(ctx context.Context, audioData []byte) error
    Results() <-chan *RecognitionResult
    Close() error
}
```

#### 3. RecognitionResult
Standardized result format:
```go
type RecognitionResult struct {
    Text       string                 // Recognized text
    IsFinal    bool                   // true for final, false for partial
    Confidence float32                // 0.0-1.0, or -1 if not available
    Language   string                 // Language code
    Duration   time.Duration          // Processing duration
    Timestamp  time.Time              // When recognition completed
    Metadata   map[string]interface{} // Provider-specific data
}
```

## OpenAI Whisper Integration

### Features

- ✅ High-quality transcription (99+ languages)
- ✅ Streaming recognition (buffered implementation)
- ✅ VAD integration for optimized processing
- ✅ Automatic audio format conversion (PCM to WAV)
- ✅ Configurable model, language, and parameters

### Quick Start

```go
import (
    "context"
    "github.com/realtime-ai/realtime-ai/pkg/asr"
)

// Create Whisper provider
provider, err := asr.NewWhisperProvider("your-openai-api-key")
if err != nil {
    log.Fatal(err)
}
defer provider.Close()

// Configure audio and recognition
audioConfig := asr.AudioConfig{
    SampleRate:    16000,
    Channels:      1,
    Encoding:      "pcm",
    BitsPerSample: 16,
}

recognitionConfig := asr.RecognitionConfig{
    Language: "en",  // or "zh", "auto", etc.
    Model:    "whisper-1",
}

// Batch recognition
result, err := provider.Recognize(ctx, audioReader, audioConfig, recognitionConfig)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Transcription: %s\n", result.Text)

// Streaming recognition
recognizer, err := provider.StreamingRecognize(ctx, audioConfig, recognitionConfig)
if err != nil {
    log.Fatal(err)
}
defer recognizer.Close()

// Send audio chunks
go func() {
    for {
        audioChunk := getNextAudioChunk()
        recognizer.SendAudio(ctx, audioChunk)
    }
}()

// Receive results
for result := range recognizer.Results() {
    if result.IsFinal {
        fmt.Printf("Final: %s\n", result.Text)
    } else {
        fmt.Printf("Partial: %s\n", result.Text)
    }
}
```

### Using WhisperSTTElement in Pipeline

```go
import (
    "github.com/realtime-ai/realtime-ai/pkg/elements"
    "github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

// Create Whisper STT element
whisperSTT, err := elements.NewWhisperSTTElement(elements.WhisperSTTConfig{
    APIKey:               "your-openai-api-key",
    Language:             "en",
    Model:                "whisper-1",
    EnablePartialResults: true,
    VADEnabled:           true,  // Integrate with VAD
    SampleRate:           16000,
    Channels:             1,
    BitsPerSample:        16,
})

// Create pipeline
p := pipeline.NewPipeline("speech-recognition-pipeline")

// Add elements (example with VAD)
vadElement := elements.NewSileroVADElement(vadConfig)
audioResample := elements.NewAudioResampleElement(16000, 1)

p.AddElement(audioResample)
p.AddElement(vadElement)
p.AddElement(whisperSTT)

// Link elements: audio -> resample -> VAD -> Whisper STT
pipeline.Link(audioResample, vadElement)
pipeline.Link(vadElement, whisperSTT)

// Start pipeline
p.Start(ctx)
```

## Qwen Realtime ASR Integration

Qwen Realtime provides true streaming ASR using WebSocket, similar to OpenAI's Realtime API.

### Features

- ✅ True real-time streaming via WebSocket
- ✅ Partial (interim) and final results
- ✅ Manual commit mode for VAD integration
- ✅ Multiple language support (Chinese, English, Japanese, Korean, Cantonese)
- ✅ Low latency transcription
- ✅ Connection retry with exponential backoff

### Quick Start

```go
import (
    "context"
    "github.com/realtime-ai/realtime-ai/pkg/asr"
)

// Create Qwen Realtime provider
provider, err := asr.NewQwenRealtimeProvider(asr.QwenRealtimeConfig{
    APIKey: "your-dashscope-api-key",
    Model:  "qwen3-asr-flash-realtime", // default model
})
if err != nil {
    log.Fatal(err)
}
defer provider.Close()

// Configure audio and recognition
audioConfig := asr.AudioConfig{
    SampleRate:    16000,
    Channels:      1,
    Encoding:      "pcm",
    BitsPerSample: 16,
}

recognitionConfig := asr.RecognitionConfig{
    Language:             "zh",  // or "en", "ja", "ko", "yue", "auto"
    EnablePartialResults: true,
}

// Streaming recognition
recognizer, err := provider.StreamingRecognize(ctx, audioConfig, recognitionConfig)
if err != nil {
    log.Fatal(err)
}
defer recognizer.Close()

// Send audio chunks
go func() {
    for {
        audioChunk := getNextAudioChunk()
        recognizer.SendAudio(ctx, audioChunk)
    }
}()

// Commit to trigger final transcription (when using manual mode)
if qr, ok := asr.IsQwenRealtimeRecognizer(recognizer); ok {
    qr.Commit(ctx)
}

// Receive results
for result := range recognizer.Results() {
    if result.IsFinal {
        fmt.Printf("Final: %s\n", result.Text)
    } else {
        fmt.Printf("Partial: %s\n", result.Text)
    }
}
```

### Using QwenRealtimeSTTElement in Pipeline

```go
import (
    "github.com/realtime-ai/realtime-ai/pkg/elements"
    "github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

// Create Qwen Realtime STT element
qwenSTT, err := elements.NewQwenRealtimeSTTElement(elements.QwenRealtimeSTTConfig{
    APIKey:               "your-dashscope-api-key",
    Language:             "zh",
    Model:                "qwen3-asr-flash-realtime",
    EnablePartialResults: true,
    VADEnabled:           true,  // Integrate with VAD
    SampleRate:           16000,
    Channels:             1,
    BitsPerSample:        16,
})

// Create pipeline
p := pipeline.NewPipeline("qwen-speech-recognition-pipeline")

// Add elements (example with VAD)
vadElement := elements.NewSileroVADElement(vadConfig)
audioResample := elements.NewAudioResampleElement(16000, 1)

p.AddElement(audioResample)
p.AddElement(vadElement)
p.AddElement(qwenSTT)

// Link elements: audio -> resample -> VAD -> Qwen STT
pipeline.Link(audioResample, vadElement)
pipeline.Link(vadElement, qwenSTT)

// Start pipeline
p.Start(ctx)
```

### Qwen vs Whisper Comparison

| Feature | Qwen Realtime | OpenAI Whisper |
|---------|---------------|----------------|
| Streaming | True WebSocket streaming | Buffered (simulated) |
| Latency | Lower (real-time) | Higher (batch processing) |
| Partial Results | Native support | Limited |
| Languages | zh, en, ja, ko, yue | 99+ languages |
| Commit Mode | Manual/VAD controlled | Timer-based |
| API Style | OpenAI Realtime-like | REST API |

### Environment Variables

```bash
# DashScope API Key
export DASHSCOPE_API_KEY=sk-...

# For debugging
export QWEN_DEBUG=true
```

### Supported Languages

- Chinese (`zh`) - Default
- English (`en`)
- Japanese (`ja`)
- Korean (`ko`)
- Cantonese (`yue`)
- Auto-detect (`auto`)

## VAD Integration

Both WhisperSTT and QwenRealtimeSTT integrate seamlessly with the SileroVAD element:

### How It Works

1. **VAD in Passthrough Mode**: VAD emits speech start/end events while forwarding all audio
2. **WhisperSTT listens**: Subscribes to `EventVADSpeechStart` and `EventVADSpeechEnd`
3. **Smart buffering**: Only recognizes audio segments where speech was detected
4. **Cost optimization**: Reduces API calls by only processing speech segments

### Pipeline Flow

```
┌──────────────────┐
│  Audio Input     │
│  (WebRTC/Local)  │
└────────┬─────────┘
         │
         ▼
┌──────────────────────┐
│ AudioResampleElement │
│  → 16kHz, mono       │
└────────┬─────────────┘
         │
         ▼
┌──────────────────────┐
│  SileroVADElement    │
│  Mode: Passthrough   │
│  Events: ─────┐      │
└────────┬──────┘      │
         │             │
         ▼             │ EventVADSpeechStart
┌──────────────────────┼──────┐
│  WhisperSTTElement   │      │
│  VADEnabled: true ◄──┘      │
│                             │
│  Events: ────────────────┐  │
└────────┬─────────────────┘  │
         │                    │ EventFinalResult
         ▼                    ▼
┌──────────────────────────────┐
│  TextData Output             │
│  → LLM Processing            │
└──────────────────────────────┘
```

### Configuration Example

```go
// VAD configuration (passthrough mode)
vadConfig := elements.SileroVADConfig{
    ModelPath:       "models/silero_vad.onnx",
    Threshold:       0.5,
    MinSilenceDurMs: 300,
    SpeechPadMs:     30,
    Mode:            elements.VADModePassthrough, // Important!
}

// Whisper configuration (VAD enabled)
whisperConfig := elements.WhisperSTTConfig{
    APIKey:               os.Getenv("OPENAI_API_KEY"),
    Language:             "auto",  // Auto-detect language
    Model:                "whisper-1",
    EnablePartialResults: false,   // Only final results
    VADEnabled:           true,    // Listen to VAD events
}
```

## Extending with New Providers

To add a new ASR provider:

1. **Implement the Provider interface**:
```go
type MyASRProvider struct {
    // Your provider fields
}

func (p *MyASRProvider) Name() string {
    return "my-asr-provider"
}

func (p *MyASRProvider) Recognize(ctx context.Context, audio io.Reader, audioConfig AudioConfig, config RecognitionConfig) (*RecognitionResult, error) {
    // Your implementation
}

// ... implement other interface methods
```

2. **Implement StreamingRecognizer** (if supported):
```go
type myStreamingRecognizer struct {
    // Your fields
}

func (r *myStreamingRecognizer) SendAudio(ctx context.Context, audioData []byte) error {
    // Your implementation
}

// ... implement other methods
```

3. **Create an Element** (optional, but recommended):
```go
type MySTTElement struct {
    *pipeline.BaseElement
    provider asr.Provider
    // ... other fields
}

// Implement Start(), Stop(), and processing logic
```

## Configuration

### Environment Variables

```bash
# OpenAI API Key
export OPENAI_API_KEY=sk-...

# For debugging
export OPENAI_DEBUG=true
```

### Audio Format Requirements

**Whisper API Requirements**:
- Supported formats: `flac`, `m4a`, `mp3`, `mp4`, `mpeg`, `mpga`, `oga`, `ogg`, `wav`, `webm`
- Maximum file size: 25 MB
- Recommended: 16 kHz, mono, 16-bit PCM

**WhisperSTT Element**:
- Automatically converts raw PCM to WAV format
- Supports custom sample rates (resampled internally if needed)
- Handles buffering for streaming recognition

## Performance Considerations

### API Latency
- Whisper API is **not real-time** - expect 1-5 second latency
- Use VAD integration to minimize unnecessary API calls
- Buffer audio strategically (5-10 second chunks work well)

### Cost Optimization
1. **Enable VAD**: Only recognize speech segments
2. **Batch audio**: Group small segments together
3. **Use prompts**: Improve accuracy, reduce retries
4. **Monitor usage**: Track API calls and costs

### Streaming vs Batch
- **Batch** (`Recognize`): Better for pre-recorded audio
- **Streaming** (`StreamingRecognize`): Better for real-time with VAD

## Error Handling

The package defines standard error codes:

```go
const (
    ErrCodeUnknown
    ErrCodeInvalidConfig
    ErrCodeInvalidAudio
    ErrCodeUnsupportedLanguage
    ErrCodeUnsupportedFeature
    ErrCodeAuthenticationFailed
    ErrCodeQuotaExceeded
    ErrCodeNetworkError
    ErrCodeProviderError
)
```

Example error handling:
```go
result, err := provider.Recognize(ctx, audio, audioConfig, config)
if err != nil {
    if asrErr, ok := err.(*asr.Error); ok {
        switch asrErr.Code {
        case asr.ErrCodeAuthenticationFailed:
            log.Fatal("Invalid API key")
        case asr.ErrCodeQuotaExceeded:
            log.Fatal("API quota exceeded")
        default:
            log.Printf("ASR error: %v", err)
        }
    }
}
```

## Supported Languages

Whisper supports 99+ languages including:

- English (`en`)
- Chinese (`zh`)
- Spanish (`es`)
- French (`fr`)
- German (`de`)
- Japanese (`ja`)
- Korean (`ko`)
- And many more...

Use `"auto"` or empty string for automatic language detection.

## Future Improvements

- [x] Add Qwen Realtime ASR provider (true streaming)
- [ ] Add Google Cloud Speech provider
- [ ] Add AWS Transcribe provider
- [ ] Support for speaker diarization
- [ ] Word-level timestamps
- [ ] Confidence scores (where available)
- [ ] Custom model fine-tuning support

## References

- [OpenAI Whisper API Documentation](https://platform.openai.com/docs/guides/speech-to-text)
- [Whisper Model Card](https://github.com/openai/whisper)
- [Supported Languages](https://github.com/openai/whisper#available-models-and-languages)
- [Alibaba Cloud DashScope Qwen ASR Realtime](https://help.aliyun.com/zh/model-studio/qwen-asr-realtime-interaction-process)

## License

See main project LICENSE file.
