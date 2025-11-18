# Trace Package

This package provides distributed tracing capabilities for the realtime-ai framework using OpenTelemetry.

## Overview

The trace package offers:

- **Easy initialization**: One-line setup with sensible defaults
- **Multiple exporters**: stdout (development), OTLP (production), or none (disabled)
- **Rich instrumentation**: Pre-built helpers for pipelines, elements, connections, and AI operations
- **Production-ready**: Configurable sampling, async batching, and integration with popular backends

## Quick Start

```go
import "github.com/realtime-ai/realtime-ai/pkg/trace"

func main() {
    ctx := context.Background()

    // Initialize tracing
    cfg := trace.DefaultConfig()
    if err := trace.Initialize(ctx, cfg); err != nil {
        log.Fatal(err)
    }
    defer trace.Shutdown(ctx)

    // Use tracing in your application
    ctx, span := trace.StartSpan(ctx, "my-operation")
    defer span.End()

    // ... your code ...
}
```

## Configuration

### Environment Variables

- `TRACE_EXPORTER`: Exporter type (`stdout`, `otlp`, `none`) - default: `stdout`
- `OTEL_EXPORTER_OTLP_ENDPOINT`: OTLP collector endpoint - default: `localhost:4317`
- `ENVIRONMENT`: Deployment environment - default: `development`

### Programmatic Configuration

```go
cfg := &trace.Config{
    ServiceName:    "my-service",
    ServiceVersion: "1.0.0",
    Environment:    "production",
    ExporterType:   "otlp",
    OTLPEndpoint:   "collector.example.com:4317",
    SamplingRate:   0.1,  // Sample 10% of traces
}

trace.Initialize(ctx, cfg)
```

## Instrumentation Helpers

### Pipeline Operations

```go
// Start pipeline
ctx, span := trace.InstrumentPipelineStart(ctx, "my-pipeline")
defer span.End()

// Process message
ctx, span := trace.InstrumentElementProcess(ctx, "element-name", msg)
defer span.End()

// Push/pull messages
ctx, span := trace.InstrumentPipelinePush(ctx, "pipeline-name", msg)
defer span.End()
```

### Connection Operations

```go
// Connection created
ctx, span := trace.InstrumentConnectionCreated(ctx, connID, "webrtc")
defer span.End()

// State change
ctx, span := trace.InstrumentConnectionStateChange(ctx, connID, "webrtc", "new", "connected")
defer span.End()

// Send/receive messages
ctx, span := trace.InstrumentConnectionMessage(ctx, connID, "webrtc", "send", len(data))
defer span.End()
```

### AI Operations

```go
// LLM request
ctx, span := trace.InstrumentLLMRequest(ctx, "gemini", "gemini-2.0-flash-exp")
defer span.End()

// STT request
ctx, span := trace.InstrumentSTTRequest(ctx, "azure", len(audioData))
defer span.End()

// TTS request
ctx, span := trace.InstrumentTTSRequest(ctx, "azure", "en-US-Neural", text)
defer span.End()
```

### Audio/Video Processing

```go
// Audio processing
ctx, span := trace.InstrumentAudioProcessing(ctx, "resample", inputSize, outputSize)
defer span.End()

// Video processing
ctx, span := trace.InstrumentVideoProcessing(ctx, "encode", inputSize, outputSize)
defer span.End()
```

## Attributes

### Predefined Attribute Keys

The package defines standard attribute keys for consistency:

```go
const (
    // Pipeline
    AttrPipelineName     = "pipeline.name"
    AttrPipelineElement  = "pipeline.element"
    AttrSessionID        = "session.id"

    // Audio
    AttrAudioSampleRate  = "audio.sample_rate"
    AttrAudioChannels    = "audio.channels"
    AttrAudioMediaType   = "audio.media_type"

    // Connection
    AttrConnectionID     = "connection.id"
    AttrConnectionType   = "connection.type"
    AttrConnectionState  = "connection.state"

    // AI
    AttrLLMProvider      = "llm.provider"
    AttrLLMModel         = "llm.model"
    // ... and more
)
```

### Helper Functions

```go
// Create attribute sets
attrs := trace.PipelineAttrs("my-pipeline", "element-name")
attrs := trace.AudioAttrs(16000, 1, 3200, "audio/x-raw", "opus")
attrs := trace.ConnectionAttrs("conn-123", "webrtc", "connected")
attrs := trace.LLMAttrs("gemini", "gemini-2.0-flash-exp")

// Use in spans
ctx, span := trace.StartSpan(ctx, "operation",
    trace.WithAttributes(attrs...),
)
```

## Utility Functions

### Error Recording

```go
if err != nil {
    trace.RecordError(span, err)
    return err
}
```

### Events

```go
trace.AddEvent(span, "processing.started",
    attribute.String("batch.id", batchID),
)
```

### Attributes

```go
trace.SetAttributes(span,
    attribute.String("user.id", userID),
    attribute.Int("batch.size", batchSize),
)
```

### Trace IDs

```go
// Get trace ID from context
traceID := trace.TraceID(ctx)

// Get span ID from context
spanID := trace.SpanID(ctx)

// Format log message with trace info
log.Println(trace.LogWithTrace(ctx, "Processing data"))
// Output: [trace_id=abc123 span_id=def456] Processing data
```

## Examples

See the complete example in `examples/tracing-demo/main.go` for a working demonstration.

## Documentation

For detailed documentation, see [`docs/tracing.md`](../../docs/tracing.md).

## Architecture

### Initialization Flow

1. Call `trace.Initialize(ctx, cfg)` at application startup
2. Package creates OpenTelemetry tracer provider with configured exporter
3. Sets global tracer provider and propagators
4. Returns tracer for use throughout the application

### Span Lifecycle

1. Start span: `ctx, span := trace.StartSpan(ctx, "operation")`
2. Add attributes: `trace.SetAttributes(span, ...)`
3. Add events: `trace.AddEvent(span, "event-name", ...)`
4. Record errors: `trace.RecordError(span, err)`
5. End span: `defer span.End()`

### Context Propagation

Traces are propagated through context:

```go
func parent(ctx context.Context) {
    ctx, span := trace.StartSpan(ctx, "parent")
    defer span.End()

    // Child span automatically linked via context
    child(ctx)
}

func child(ctx context.Context) {
    ctx, span := trace.StartSpan(ctx, "child")
    defer span.End()
    // ...
}
```

## Integration with Backends

### Jaeger

```bash
# Run Jaeger
docker run -d -p 16686:16686 -p 4317:4317 jaegertracing/all-in-one:latest

# Configure app
export TRACE_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317

# View traces at http://localhost:16686
```

### Zipkin

```bash
# Run Zipkin
docker run -d -p 9411:9411 openzipkin/zipkin

# Configure OpenTelemetry Collector with Zipkin exporter
# Point app to collector
```

### Cloud Providers

Set `TRACE_EXPORTER=otlp` and configure endpoint for:
- Google Cloud Trace
- AWS X-Ray (via ADOT Collector)
- Azure Monitor

## Performance

- **Overhead**: Minimal with batched async export
- **Sampling**: Configurable sampling rate (0.0-1.0)
- **No-op mode**: Set `TRACE_EXPORTER=none` for zero overhead

## Best Practices

1. **Always propagate context**: Pass `ctx` through your call chain
2. **Add meaningful attributes**: Help with debugging and filtering
3. **Record errors**: Use `trace.RecordError(span, err)`
4. **Use events for milestones**: Mark important points in processing
5. **Control sampling**: Reduce overhead in high-throughput scenarios
6. **Include trace IDs in logs**: Correlate logs with traces

## License

Same as the realtime-ai project.
