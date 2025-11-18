package trace

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// InstrumentLLMRequest creates a span for LLM requests
func InstrumentLLMRequest(ctx context.Context, provider, model string) (context.Context, trace.Span) {
	return StartSpan(ctx, "llm.request",
		trace.WithAttributes(
			LLMAttrs(provider, model)...,
		),
	)
}

// InstrumentLLMResponse creates a span for LLM responses
func InstrumentLLMResponse(ctx context.Context, provider, model, responseType string, dataSize int) (context.Context, trace.Span) {
	attrs := LLMAttrs(provider, model)
	attrs = append(attrs,
		attribute.String(AttrLLMResponseType, responseType),
		attribute.Int("response.data_size", dataSize),
	)

	return StartSpan(ctx, "llm.response",
		trace.WithAttributes(attrs...),
	)
}

// InstrumentSTTRequest creates a span for STT (Speech-to-Text) requests
func InstrumentSTTRequest(ctx context.Context, provider string, audioSize int) (context.Context, trace.Span) {
	return StartSpan(ctx, "stt.request",
		trace.WithAttributes(
			attribute.String(AttrSTTProvider, provider),
			attribute.Int("audio.size", audioSize),
		),
	)
}

// InstrumentSTTResponse creates a span for STT responses
func InstrumentSTTResponse(ctx context.Context, provider, text string) (context.Context, trace.Span) {
	return StartSpan(ctx, "stt.response",
		trace.WithAttributes(
			attribute.String(AttrSTTProvider, provider),
			attribute.Int("text.length", len(text)),
		),
	)
}

// InstrumentTTSRequest creates a span for TTS (Text-to-Speech) requests
func InstrumentTTSRequest(ctx context.Context, provider, voice, text string) (context.Context, trace.Span) {
	return StartSpan(ctx, "tts.request",
		trace.WithAttributes(
			attribute.String(AttrTTSProvider, provider),
			attribute.String(AttrTTSVoice, voice),
			attribute.Int("text.length", len(text)),
		),
	)
}

// InstrumentTTSResponse creates a span for TTS responses
func InstrumentTTSResponse(ctx context.Context, provider string, audioSize int) (context.Context, trace.Span) {
	return StartSpan(ctx, "tts.response",
		trace.WithAttributes(
			attribute.String(AttrTTSProvider, provider),
			attribute.Int("audio.size", audioSize),
		),
	)
}

// InstrumentAudioProcessing creates a span for audio processing operations
func InstrumentAudioProcessing(ctx context.Context, operation string, inputSize, outputSize int) (context.Context, trace.Span) {
	return StartSpan(ctx, fmt.Sprintf("audio.%s", operation),
		trace.WithAttributes(
			attribute.String("audio.operation", operation),
			attribute.Int("audio.input_size", inputSize),
			attribute.Int("audio.output_size", outputSize),
		),
	)
}

// InstrumentVideoProcessing creates a span for video processing operations
func InstrumentVideoProcessing(ctx context.Context, operation string, inputSize, outputSize int) (context.Context, trace.Span) {
	return StartSpan(ctx, fmt.Sprintf("video.%s", operation),
		trace.WithAttributes(
			attribute.String("video.operation", operation),
			attribute.Int("video.input_size", inputSize),
			attribute.Int("video.output_size", outputSize),
		),
	)
}
