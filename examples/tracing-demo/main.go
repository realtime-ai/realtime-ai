package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/trace"
)

func main() {
	// Load .env file if it exists
	_ = godotenv.Load()

	ctx := context.Background()

	// Initialize tracing with configuration
	// You can control tracing behavior via environment variables:
	// - TRACE_EXPORTER: "stdout", "otlp", or "none" (default: "stdout")
	// - OTEL_EXPORTER_OTLP_ENDPOINT: endpoint for OTLP exporter (default: "localhost:4317")
	// - ENVIRONMENT: deployment environment (default: "development")
	traceCfg := trace.DefaultConfig()
	if err := trace.Initialize(ctx, traceCfg); err != nil {
		log.Fatalf("Failed to initialize tracing: %v", err)
	}
	defer func() {
		if err := trace.Shutdown(ctx); err != nil {
			log.Printf("Failed to shutdown tracing: %v", err)
		}
	}()

	log.Println("Tracing initialized successfully")

	// Create a traced pipeline
	if err := runTracedPipeline(ctx); err != nil {
		log.Fatalf("Pipeline error: %v", err)
	}
}

func runTracedPipeline(ctx context.Context) error {
	// Create root span for the entire demo
	ctx, rootSpan := trace.StartSpan(ctx, "tracing-demo")
	defer rootSpan.End()

	log.Println("Starting traced pipeline demo...")

	// Create pipeline
	p := pipeline.NewPipeline("gemini-tracing-demo")

	// Create and add elements
	// AudioResampleElement(inputRate, outputRate, inputChannels, outputChannels)
	resampleElement := elements.NewAudioResampleElement(48000, 16000, 1, 1)
	geminiElement := elements.NewGeminiElement()
	audioPacerElement := elements.NewAudioPacerSinkElement()

	p.AddElements([]pipeline.Element{
		resampleElement,
		geminiElement,
		audioPacerElement,
	})

	// Link elements
	unlink1 := p.Link(resampleElement, geminiElement)
	defer unlink1()
	unlink2 := p.Link(geminiElement, audioPacerElement)
	defer unlink2()

	// Start pipeline with tracing
	startCtx, startSpan := trace.InstrumentPipelineStart(ctx, "gemini-tracing-demo")
	if err := p.Start(startCtx); err != nil {
		trace.RecordError(startSpan, err)
		startSpan.End()
		return fmt.Errorf("failed to start pipeline: %w", err)
	}
	startSpan.End()

	log.Println("Pipeline started successfully")

	// Simulate sending audio messages with tracing
	go func() {
		sessionID := "demo-session-001"

		for i := 0; i < 5; i++ {
			// Create a span for each message
			msgCtx, msgSpan := trace.InstrumentPipelinePush(ctx, "gemini-tracing-demo", &pipeline.PipelineMessage{
				Type:      pipeline.MsgTypeAudio,
				SessionID: sessionID,
				Timestamp: time.Now(),
			})

			// Create dummy audio data
			dummyAudio := make([]byte, 3200) // 100ms at 16kHz mono
			msg := &pipeline.PipelineMessage{
				Type:      pipeline.MsgTypeAudio,
				SessionID: sessionID,
				Timestamp: time.Now(),
				AudioData: &pipeline.AudioData{
					Data:       dummyAudio,
					SampleRate: 16000,
					Channels:   1,
					MediaType:  "audio/x-raw",
					Timestamp:  time.Now(),
				},
			}

			// Add trace information to span
			trace.SetAttributes(msgSpan,
				trace.AudioAttrs(16000, 1, len(dummyAudio), "audio/x-raw", "")...,
			)

			p.Push(msg)
			msgSpan.End()

			log.Printf("Pushed message %d (trace_id=%s)", i+1, trace.TraceID(msgCtx))
			time.Sleep(200 * time.Millisecond)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("Tracing demo running. Press Ctrl+C to stop...")
	log.Println("Trace information is being exported. Check your configured exporter output.")

	<-sigChan
	log.Println("\nShutting down...")

	// Stop pipeline with tracing
	_, stopSpan := trace.InstrumentPipelineStop(ctx, "gemini-tracing-demo")
	if err := p.Stop(); err != nil {
		trace.RecordError(stopSpan, err)
		stopSpan.End()
		return fmt.Errorf("failed to stop pipeline: %w", err)
	}
	stopSpan.End()

	log.Println("Pipeline stopped successfully")

	// Give time for traces to be exported
	time.Sleep(2 * time.Second)

	return nil
}
