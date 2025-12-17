package trace

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// InstrumentConnectionCreated creates a span for connection creation
func InstrumentConnectionCreated(ctx context.Context, connID, connType string) (context.Context, trace.Span) {
	return StartSpan(ctx, "connection.created",
		trace.WithAttributes(
			ConnectionAttrs(connID, connType, "created")...,
		),
	)
}

// InstrumentConnectionStateChange creates a span for connection state changes
func InstrumentConnectionStateChange(ctx context.Context, connID, connType, oldState, newState string) (context.Context, trace.Span) {
	attrs := ConnectionAttrs(connID, connType, newState)
	attrs = append(attrs,
		attribute.String("connection.old_state", oldState),
	)

	return StartSpan(ctx, "connection.state_change",
		trace.WithAttributes(attrs...),
	)
}

// InstrumentConnectionMessage creates a span for sending/receiving messages over a connection
func InstrumentConnectionMessage(ctx context.Context, connID, connType, direction string, dataSize int) (context.Context, trace.Span) {
	attrs := ConnectionAttrs(connID, connType, "active")
	attrs = append(attrs,
		attribute.String("message.direction", direction),
		attribute.Int("message.size", dataSize),
	)

	return StartSpan(ctx, fmt.Sprintf("connection.message.%s", direction),
		trace.WithAttributes(attrs...),
	)
}

// InstrumentConnectionError creates a span for connection errors
func InstrumentConnectionError(ctx context.Context, connID, connType string, err error) (context.Context, trace.Span) {
	ctx, span := StartSpan(ctx, "connection.error",
		trace.WithAttributes(
			ConnectionAttrs(connID, connType, "error")...,
		),
	)

	RecordError(span, err)
	return ctx, span
}

// InstrumentConnectionClosed creates a span for connection closure
func InstrumentConnectionClosed(ctx context.Context, connID, connType string) (context.Context, trace.Span) {
	return StartSpan(ctx, "connection.closed",
		trace.WithAttributes(
			ConnectionAttrs(connID, connType, "closed")...,
		),
	)
}
