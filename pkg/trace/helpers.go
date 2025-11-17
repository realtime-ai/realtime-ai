package trace

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// WithSpan executes a function within a new span
func WithSpan(ctx context.Context, spanName string, fn func(context.Context) error, opts ...trace.SpanStartOption) error {
	ctx, span := StartSpan(ctx, spanName, opts...)
	defer span.End()

	if err := fn(ctx); err != nil {
		RecordError(span, err)
		return err
	}

	return nil
}

// RecordError records an error on a span
func RecordError(span trace.Span, err error) {
	if err == nil {
		return
	}

	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// AddEvent adds an event to a span
func AddEvent(span trace.Span, name string, attrs ...attribute.KeyValue) {
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// SetAttributes sets multiple attributes on a span
func SetAttributes(span trace.Span, attrs ...attribute.KeyValue) {
	span.SetAttributes(attrs...)
}

// SpanFromContext returns the current span from context
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// ContextWithSpan returns a new context with the given span
func ContextWithSpan(ctx context.Context, span trace.Span) context.Context {
	return trace.ContextWithSpan(ctx, span)
}

// TraceID returns the trace ID from the current span in context
func TraceID(ctx context.Context) string {
	span := SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return ""
	}
	return span.SpanContext().TraceID().String()
}

// SpanID returns the span ID from the current span in context
func SpanID(ctx context.Context) string {
	span := SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return ""
	}
	return span.SpanContext().SpanID().String()
}

// LogWithTrace returns a formatted string with trace information
func LogWithTrace(ctx context.Context, message string) string {
	traceID := TraceID(ctx)
	spanID := SpanID(ctx)

	if traceID == "" {
		return message
	}

	return fmt.Sprintf("[trace_id=%s span_id=%s] %s", traceID, spanID, message)
}
