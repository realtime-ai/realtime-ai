# Distributed Tracing Guide

This document explains how to use distributed tracing in the realtime-ai framework to monitor and debug your real-time AI applications.

## Overview

The realtime-ai framework integrates [OpenTelemetry](https://opentelemetry.io/) for distributed tracing, providing observability into:

- Pipeline message flow and processing
- Element lifecycle and operations
- WebRTC connection states and data flow
- AI service interactions (LLM, STT, TTS)
- Audio/video processing operations

Tracing helps you:
- **Debug issues**: Understand the flow of data through your pipeline
- **Optimize performance**: Identify bottlenecks and slow operations
- **Monitor production**: Track errors and performance metrics in real-time
- **Understand behavior**: Visualize complex interactions between components

## Quick Start

### 1. Initialize Tracing

Add tracing initialization to your application:

```go
import (
    "context"
    "github.com/realtime-ai/realtime-ai/pkg/trace"
)

func main() {
    ctx := context.Background()

    // Initialize with default configuration (stdout exporter)
    cfg := trace.DefaultConfig()
    if err := trace.Initialize(ctx, cfg); err != nil {
        log.Fatal(err)
    }
    defer trace.Shutdown(ctx)

    // Your application code here
}
```

### 2. Configure via Environment Variables

Control tracing behavior without code changes:

```bash
# Exporter type: "stdout", "otlp", or "none"
export TRACE_EXPORTER=stdout

# OTLP endpoint (when using OTLP exporter)
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317

# Environment name
export ENVIRONMENT=production
```

### 3. Run the Example

```bash
# Run with stdout exporter (default)
go run examples/tracing-demo/main.go

# Run with OTLP exporter (requires OTLP collector)
TRACE_EXPORTER=otlp go run examples/tracing-demo/main.go

# Disable tracing
TRACE_EXPORTER=none go run examples/tracing-demo/main.go
```

## Configuration

### Trace Configuration Options

```go
cfg := &trace.Config{
    ServiceName:    "my-realtime-app",      // Service name in traces
    ServiceVersion: "1.0.0",                // Service version
    Environment:    "production",           // Deployment environment
    ExporterType:   "otlp",                // Exporter: stdout, otlp, none
    OTLPEndpoint:   "localhost:4317",      // OTLP collector endpoint
    SamplingRate:   1.0,                   // Sample rate (0.0-1.0)
}
```

### Exporter Types

#### 1. Stdout Exporter (Development)
Prints traces to console in human-readable format.

```bash
export TRACE_EXPORTER=stdout
```

**Use case**: Local development, debugging

#### 2. OTLP Exporter (Production)
Sends traces to an OpenTelemetry Collector or compatible backend.

```bash
export TRACE_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
```

**Use case**: Production monitoring, integration with Jaeger, Zipkin, etc.

#### 3. None (Disabled)
No-op exporter, disables tracing overhead.

```bash
export TRACE_EXPORTER=none
```

**Use case**: Performance-critical scenarios where tracing is not needed

## Instrumentation

### Pipeline Tracing

Track pipeline operations:

```go
import "github.com/realtime-ai/realtime-ai/pkg/trace"

// Start pipeline with tracing
ctx, span := trace.InstrumentPipelineStart(ctx, "my-pipeline")
err := pipeline.Start(ctx)
span.End()

// Push message with tracing
ctx, span := trace.InstrumentPipelinePush(ctx, "my-pipeline", msg)
pipeline.Push(msg)
span.End()

// Stop pipeline with tracing
ctx, span := trace.InstrumentPipelineStop(ctx, "my-pipeline")
pipeline.Stop()
span.End()
```

### Element Tracing

Instrument custom elements:

```go
func (e *MyElement) Start(ctx context.Context) error {
    ctx, span := trace.InstrumentElementStart(ctx, e.GetName())
    defer span.End()

    // Element start logic
    return nil
}

func (e *MyElement) processMessage(ctx context.Context, msg *pipeline.PipelineMessage) {
    ctx, span := trace.InstrumentElementProcess(ctx, e.GetName(), msg)
    defer span.End()

    // Process message
    // Errors are automatically recorded if the function returns an error
}
```

### Connection Tracing

Track WebRTC connections:

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

### AI Service Tracing

Instrument AI operations:

```go
// LLM request
ctx, span := trace.InstrumentLLMRequest(ctx, "gemini", "gemini-2.0-flash-exp")
// ... make request ...
span.End()

// STT request
ctx, span := trace.InstrumentSTTRequest(ctx, "azure", len(audioData))
// ... transcribe audio ...
span.End()

// TTS request
ctx, span := trace.InstrumentTTSRequest(ctx, "azure", "en-US-Neural", text)
// ... synthesize speech ...
span.End()
```

### Custom Spans

Create custom spans for specific operations:

```go
ctx, span := trace.StartSpan(ctx, "custom-operation")
defer span.End()

// Add attributes
trace.SetAttributes(span,
    attribute.String("user.id", userID),
    attribute.Int("batch.size", batchSize),
)

// Add events
trace.AddEvent(span, "processing.started")

// Record errors
if err != nil {
    trace.RecordError(span, err)
    return err
}
```

## Trace Attributes

### Standard Attributes

The framework provides predefined attribute keys:

**Pipeline Attributes:**
- `pipeline.name`: Pipeline name
- `pipeline.element`: Element name
- `session.id`: Session identifier
- `message.type`: Message type (audio/video/data)

**Audio Attributes:**
- `audio.sample_rate`: Sample rate in Hz
- `audio.channels`: Number of channels
- `audio.media_type`: Media type (e.g., "audio/x-raw")
- `audio.codec`: Codec name
- `audio.data_size`: Data size in bytes

**Connection Attributes:**
- `connection.id`: Connection identifier
- `connection.type`: Connection type (webrtc/ws/grpc)
- `connection.state`: Connection state

**AI Attributes:**
- `llm.provider`: LLM provider (gemini/openai)
- `llm.model`: Model name
- `stt.provider`: STT provider
- `tts.provider`: TTS provider

### Helper Functions

Use helper functions to create common attribute sets:

```go
// Pipeline attributes
attrs := trace.PipelineAttrs("my-pipeline", "gemini-element")

// Session attributes
attrs := trace.SessionAttrs("session-123")

// Audio attributes
attrs := trace.AudioAttrs(16000, 1, 3200, "audio/x-raw", "opus")

// Connection attributes
attrs := trace.ConnectionAttrs("conn-456", "webrtc", "connected")

// LLM attributes
attrs := trace.LLMAttrs("gemini", "gemini-2.0-flash-exp")
```

## Integration with Observability Backends

### Jaeger

1. Run Jaeger all-in-one:
```bash
docker run -d --name jaeger \
  -p 16686:16686 \
  -p 4317:4317 \
  jaegertracing/all-in-one:latest
```

2. Configure your app:
```bash
export TRACE_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
```

3. View traces at http://localhost:16686

### Zipkin

1. Run Zipkin:
```bash
docker run -d -p 9411:9411 openzipkin/zipkin
```

2. Run OpenTelemetry Collector with Zipkin exporter

3. Configure your app to send to the collector

### Cloud Providers

#### Google Cloud Trace
```bash
export TRACE_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_ENDPOINT=cloudtrace.googleapis.com:443
# Set up authentication via service account
```

#### AWS X-Ray
Use AWS Distro for OpenTelemetry (ADOT) Collector

#### Azure Monitor
Use Azure Monitor OpenTelemetry Exporter

## Best Practices

### 1. Use Context Propagation

Always pass context through your call chain:

```go
func handleRequest(ctx context.Context) {
    ctx, span := trace.StartSpan(ctx, "handle-request")
    defer span.End()

    // Pass ctx to subsequent calls
    processData(ctx, data)
}

func processData(ctx context.Context, data []byte) {
    // Create child span automatically via context
    ctx, span := trace.StartSpan(ctx, "process-data")
    defer span.End()

    // ...
}
```

### 2. Add Meaningful Attributes

Add context-specific attributes to help with debugging:

```go
trace.SetAttributes(span,
    attribute.String("user.id", userID),
    attribute.String("session.id", sessionID),
    attribute.Int("audio.sample_rate", sampleRate),
)
```

### 3. Record Errors Properly

Always record errors on spans:

```go
ctx, span := trace.StartSpan(ctx, "operation")
defer span.End()

if err := doSomething(); err != nil {
    trace.RecordError(span, err)
    return err
}
```

### 4. Use Events for Milestones

Add events to mark important milestones:

```go
trace.AddEvent(span, "connection.established")
trace.AddEvent(span, "audio.processing.started")
trace.AddEvent(span, "ai.response.received")
```

### 5. Control Sampling in Production

Reduce overhead by sampling traces:

```go
cfg := trace.DefaultConfig()
cfg.SamplingRate = 0.1  // Sample 10% of traces
```

### 6. Include Trace IDs in Logs

Correlate logs with traces:

```go
log.Printf(trace.LogWithTrace(ctx, "Processing audio data"))
// Output: [trace_id=abc123 span_id=def456] Processing audio data
```

## Performance Considerations

### Overhead

- **Stdout exporter**: Minimal overhead, good for development
- **OTLP exporter**: Low overhead with batching
- **None exporter**: Zero overhead, disabled

### Sampling

For high-throughput applications, use sampling:

```go
cfg.SamplingRate = 0.1  // 10% sampling
```

### Async Export

Traces are exported asynchronously via batching, minimizing impact on latency-sensitive operations.

## Troubleshooting

### Traces not appearing

1. Check exporter configuration:
```bash
echo $TRACE_EXPORTER
```

2. Verify collector is running (for OTLP):
```bash
curl http://localhost:4317
```

3. Check for initialization errors in logs

### High overhead

1. Reduce sampling rate:
```go
cfg.SamplingRate = 0.1
```

2. Use OTLP exporter instead of stdout in production

3. Disable tracing for performance testing:
```bash
TRACE_EXPORTER=none
```

### Missing trace context

Ensure context is propagated through all function calls. Avoid creating new background contexts mid-chain.

## Example Application

See `examples/tracing-demo/main.go` for a complete working example demonstrating:

- Tracing initialization
- Pipeline instrumentation
- Custom spans and attributes
- Error recording
- Graceful shutdown

Run it:
```bash
go run examples/tracing-demo/main.go
```

## Resources

- [OpenTelemetry Documentation](https://opentelemetry.io/docs/)
- [OpenTelemetry Go SDK](https://github.com/open-telemetry/opentelemetry-go)
- [Jaeger Documentation](https://www.jaegertracing.io/docs/)
- [Zipkin Documentation](https://zipkin.io/)

## Next Steps

1. Add tracing to your application
2. Experiment with different exporters
3. Set up a tracing backend (Jaeger/Zipkin)
4. Create custom dashboards for your traces
5. Set up alerts based on trace data
