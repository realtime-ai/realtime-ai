# Silero VAD Element

Voice Activity Detection (VAD) element using Silero VAD for real-time speech detection.

## Features

- **Dual Operating Modes**:
  - **Passthrough Mode**: Forwards all audio and emits speech start/end events via Bus
  - **Filter Mode**: Only forwards audio segments containing speech

- **Real-time Detection**: Low-latency speech activity detection (~30-100ms)
- **Configurable**: Adjustable threshold, silence duration, and speech padding
- **Event-driven**: Emits `EventVADSpeechStart` and `EventVADSpeechEnd` events
- **16kHz Audio**: Optimized for 16kHz PCM audio (mono, int16)

## Build Requirements

VAD uses the `vad` build tag and requires the ONNX Runtime shared library.

### System Dependencies

1. **ONNX Runtime v1.20.1** (or compatible version)
   ```bash
   # Ubuntu/Debian
   wget https://github.com/microsoft/onnxruntime/releases/download/v1.20.1/onnxruntime-linux-x64-1.20.1.tgz
   tar -xzf onnxruntime-linux-x64-1.20.1.tgz
   export ONNXRUNTIME_LIB=$(pwd)/onnxruntime-linux-x64-1.20.1/lib/libonnxruntime.so
   export LD_LIBRARY_PATH=$(pwd)/onnxruntime-linux-x64-1.20.1/lib:$LD_LIBRARY_PATH

   # macOS
   brew install onnxruntime
   # Library is auto-detected at /opt/homebrew/lib/libonnxruntime.dylib
   ```

2. **Silero VAD Model**
   ```bash
   mkdir -p models
   wget https://github.com/snakers4/silero-vad/raw/master/src/silero_vad/data/silero_vad.onnx -O models/silero_vad.onnx
   ```

### Building

```bash
# Build with VAD support
go build -tags vad ./...

# Run tests with VAD
go test -tags vad ./pkg/vad/...
go test -tags vad ./pkg/elements/...

# Build examples with VAD
go build -tags vad ./examples/...
```

### Implementation Notes

The VAD implementation uses [yalue/onnxruntime_go](https://github.com/yalue/onnxruntime_go) v1.17.0 for ONNX model inference. This provides a pure Go wrapper around the ONNX Runtime C API, eliminating the need for custom CGO code.

**Version Compatibility**:
| onnxruntime_go | ONNX Runtime |
|----------------|--------------|
| v1.17.0        | 1.20.x       |
| v1.18.0        | 1.21.x       |
| v1.19.0+       | 1.22.x+      |

## Usage

### Basic Example (Passthrough Mode)

```go
package main

import (
    "context"
    "log"

    "github.com/realtime-ai/realtime-ai/pkg/elements"
    "github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

func main() {
    // Create VAD element
    vadElement, err := elements.NewSileroVADElement(elements.SileroVADConfig{
        ModelPath:       "models/silero_vad.onnx",
        Threshold:       0.5,
        MinSilenceDurMs: 100,
        SpeechPadMs:     30,
        Mode:            elements.VADModePassthrough,
    })
    if err != nil {
        log.Fatal(err)
    }

    // Create pipeline
    p := pipeline.NewPipeline("vad-example")

    // Add elements
    resampleElement := elements.NewAudioResampleElement(48000, 16000, 1, 1)
    p.AddElements([]pipeline.Element{resampleElement, vadElement})

    // Link elements
    pipeline.Link(resampleElement, vadElement)

    // Subscribe to VAD events
    eventChan := make(chan pipeline.Event, 10)
    p.Bus().Subscribe(pipeline.EventVADSpeechStart, eventChan)
    p.Bus().Subscribe(pipeline.EventVADSpeechEnd, eventChan)

    go func() {
        for event := range eventChan {
            payload := event.Payload.(pipeline.VADPayload)
            log.Printf("[VAD] %s - Confidence: %.2f, AudioMs: %d",
                event.Type, payload.Confidence, payload.AudioMs)
        }
    }()

    // Initialize and start
    ctx := context.Background()
    vadElement.Init(ctx)
    p.Start(ctx)

    // ... send audio to pipeline ...

    p.Stop()
}
```

### Filter Mode Example

```go
// Only forward speech segments to downstream elements
vadElement, err := elements.NewSileroVADElement(elements.SileroVADConfig{
    ModelPath:       "models/silero_vad.onnx",
    Threshold:       0.5,
    MinSilenceDurMs: 500,  // 500ms of silence before cutting
    SpeechPadMs:     100,  // 100ms padding to avoid cutting
    Mode:            elements.VADModeFilter,
})

// Link: resample → vad (filter) → stt
// STT will only receive speech segments
```

## Configuration

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `ModelPath` | string | required | Path to Silero VAD ONNX model |
| `Threshold` | float32 | 0.5 | Speech detection threshold (0.0-1.0) |
| `MinSilenceDurMs` | int | 100 | Minimum silence duration in ms |
| `SpeechPadMs` | int | 30 | Speech padding in ms |
| `PreRollMs` | int | 300 | Pre-roll buffer duration in ms |
| `Mode` | VADMode | Passthrough | Operating mode |

### Runtime Configuration

Properties can be changed at runtime:

```go
// Change threshold
vadElement.SetProperty("threshold", float32(0.7))

// Change mode
vadElement.SetProperty("mode", int(elements.VADModeFilter))
```

## Events

The VAD element emits two event types via the pipeline Bus:

### EventVADSpeechStart

Emitted when speech begins. Includes pre-roll audio captured before speech detection.

**Payload**: `pipeline.VADPayload`
```go
type VADPayload struct {
    AudioMs      int     // Audio position in milliseconds
    ItemID       string  // Session/item ID
    Confidence   float32 // Speech probability at detection time
    PreRollAudio []byte  // Pre-roll audio data before speech start
    SampleRate   int     // Sample rate of PreRollAudio (16000)
    Channels     int     // Number of channels in PreRollAudio (1)
}
```

### EventVADSpeechEnd

Emitted when speech ends (after minimum silence duration).

**Payload**: `pipeline.VADPayload` (without PreRollAudio)

## Performance

- **Latency**: 30-100ms
- **CPU Usage**: < 5% on single core
- **Memory**: ~2-3MB (model) + ~100KB (runtime buffers)
- **Accuracy**: Excellent in various noise conditions

## Troubleshooting

### "onnxruntime_c_api.h: No such file or directory"

Install ONNX Runtime (see Build Requirements above).

### "Expected 16kHz audio, got XXXXHz"

Add an `AudioResampleElement` before the VAD element:
```go
resampleElement := elements.NewAudioResampleElement(inputRate, 16000, 1, 1)
pipeline.Link(resampleElement, vadElement)
```

### Model file not found

Download the Silero VAD model:
```bash
mkdir -p models
wget https://github.com/snakers4/silero-vad/raw/master/src/silero_vad/data/silero_vad.onnx -O models/silero_vad.onnx
```

## Testing

Tests require the VAD model and ONNX Runtime:

```bash
# Run VAD tests
go test -v ./pkg/elements/ -run TestVAD
```

## References

- [Silero VAD GitHub](https://github.com/snakers4/silero-vad)
- [silero-vad-go](https://github.com/streamer45/silero-vad-go)
- [ONNX Runtime](https://onnxruntime.ai/)
