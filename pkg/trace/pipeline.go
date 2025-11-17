package trace

import (
	"context"
	"fmt"

	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// InstrumentPipelineStart creates a span for pipeline start
func InstrumentPipelineStart(ctx context.Context, pipelineName string) (context.Context, trace.Span) {
	return StartSpan(ctx, fmt.Sprintf("pipeline.%s.start", pipelineName),
		trace.WithAttributes(
			attribute.String(AttrPipelineName, pipelineName),
		),
	)
}

// InstrumentPipelineStop creates a span for pipeline stop
func InstrumentPipelineStop(ctx context.Context, pipelineName string) (context.Context, trace.Span) {
	return StartSpan(ctx, fmt.Sprintf("pipeline.%s.stop", pipelineName),
		trace.WithAttributes(
			attribute.String(AttrPipelineName, pipelineName),
		),
	)
}

// InstrumentElementStart creates a span for element start
func InstrumentElementStart(ctx context.Context, elementName string) (context.Context, trace.Span) {
	return StartSpan(ctx, fmt.Sprintf("element.%s.start", elementName),
		trace.WithAttributes(
			attribute.String(AttrPipelineElement, elementName),
		),
	)
}

// InstrumentElementStop creates a span for element stop
func InstrumentElementStop(ctx context.Context, elementName string) (context.Context, trace.Span) {
	return StartSpan(ctx, fmt.Sprintf("element.%s.stop", elementName),
		trace.WithAttributes(
			attribute.String(AttrPipelineElement, elementName),
		),
	)
}

// InstrumentElementProcess creates a span for element message processing
func InstrumentElementProcess(ctx context.Context, elementName string, msg *pipeline.PipelineMessage) (context.Context, trace.Span) {
	spanName := fmt.Sprintf("element.%s.process", elementName)

	attrs := []attribute.KeyValue{
		attribute.String(AttrPipelineElement, elementName),
		attribute.String(AttrSessionID, msg.SessionID),
		attribute.Int(AttrMessageType, int(msg.Type)),
	}

	// Add message-specific attributes
	switch msg.Type {
	case pipeline.MsgTypeAudio:
		if msg.AudioData != nil {
			attrs = append(attrs, AudioAttrs(
				msg.AudioData.SampleRate,
				msg.AudioData.Channels,
				len(msg.AudioData.Data),
				msg.AudioData.MediaType,
				msg.AudioData.Codec,
			)...)
		}
	case pipeline.MsgTypeVideo:
		if msg.VideoData != nil {
			attrs = append(attrs, VideoAttrs(
				msg.VideoData.Width,
				msg.VideoData.Height,
				len(msg.VideoData.Data),
				msg.VideoData.MediaType,
				msg.VideoData.Codec,
			)...)
		}
	case pipeline.MsgTypeData:
		if msg.TextData != nil {
			attrs = append(attrs, attribute.Int("text.data_size", len(msg.TextData.Data)))
		}
	}

	return StartSpan(ctx, spanName, trace.WithAttributes(attrs...))
}

// InstrumentPipelinePush creates a span for pushing a message to the pipeline
func InstrumentPipelinePush(ctx context.Context, pipelineName string, msg *pipeline.PipelineMessage) (context.Context, trace.Span) {
	spanName := fmt.Sprintf("pipeline.%s.push", pipelineName)

	attrs := []attribute.KeyValue{
		attribute.String(AttrPipelineName, pipelineName),
		attribute.String(AttrSessionID, msg.SessionID),
		attribute.Int(AttrMessageType, int(msg.Type)),
	}

	return StartSpan(ctx, spanName, trace.WithAttributes(attrs...))
}

// InstrumentPipelinePull creates a span for pulling a message from the pipeline
func InstrumentPipelinePull(ctx context.Context, pipelineName string) (context.Context, trace.Span) {
	return StartSpan(ctx, fmt.Sprintf("pipeline.%s.pull", pipelineName),
		trace.WithAttributes(
			attribute.String(AttrPipelineName, pipelineName),
		),
	)
}
