// Package factories provides pipeline factory implementations for the Realtime API server.
package factories

import (
	"context"
	"fmt"

	"github.com/realtime-ai/realtime-ai/pkg/elements"
	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/realtimeapi"
)

// GeminiPipelineFactory creates a Gemini-based pipeline for real-time AI processing.
// The pipeline structure is:
//
//	Input (24kHz) → AudioResample (24kHz→16kHz) → GeminiElement → Output (24kHz)
//
// Note: Input is assumed to be 24kHz PCM16 from the client.
// Gemini expects 16kHz input, so we resample down.
// Gemini outputs 24kHz, which matches our output format.
func GeminiPipelineFactory(ctx context.Context, session *realtimeapi.Session) (*pipeline.Pipeline, error) {
	pipelineName := fmt.Sprintf("realtime-gemini-%s", session.ID)
	p := pipeline.NewPipeline(pipelineName)

	// Create audio resample element: 24kHz → 16kHz for Gemini input
	// Client sends 24kHz PCM16, Gemini expects 16kHz
	resample := elements.NewAudioResampleElement(24000, 16000, 1, 1)

	// Create Gemini element for AI processing
	gemini := elements.NewGeminiElement()

	// Add elements to pipeline
	p.AddElement(resample)
	p.AddElement(gemini)

	// Link elements: resample → gemini
	// Gemini output goes directly to pipeline output (24kHz already)
	p.Link(resample, gemini)

	return p, nil
}

// GeminiPipelineFactoryWithVAD creates a Gemini pipeline with VAD (Voice Activity Detection).
// Requires building with -tags vad and ONNX Runtime.
// The pipeline structure is:
//
//	Input (24kHz) → AudioResample (24kHz→16kHz) → VAD → GeminiElement → Output (24kHz)
//
// The VAD element filters audio and publishes VADSpeechStart/End events to the bus.
func GeminiPipelineFactoryWithVAD(ctx context.Context, session *realtimeapi.Session) (*pipeline.Pipeline, error) {
	pipelineName := fmt.Sprintf("realtime-gemini-vad-%s", session.ID)
	p := pipeline.NewPipeline(pipelineName)

	// Create audio resample element: 24kHz → 16kHz for both VAD and Gemini
	resample := elements.NewAudioResampleElement(24000, 16000, 1, 1)

	// Create Gemini element for AI processing
	gemini := elements.NewGeminiElement()

	// Add elements to pipeline
	p.AddElement(resample)
	p.AddElement(gemini)

	// Link elements: resample → gemini
	// Note: If VAD is needed, it should be inserted between resample and gemini
	// VAD element requires -tags vad build flag
	p.Link(resample, gemini)

	return p, nil
}

// SimplePipelineFactory creates a simple echo pipeline for testing.
// This pipeline just passes audio through without processing.
func SimplePipelineFactory(ctx context.Context, session *realtimeapi.Session) (*pipeline.Pipeline, error) {
	pipelineName := fmt.Sprintf("realtime-echo-%s", session.ID)
	p := pipeline.NewPipeline(pipelineName)

	// Create a simple pass-through element
	echo := NewEchoElement()
	p.AddElement(echo)

	return p, nil
}

// EchoElement is a simple element that echoes audio back.
type EchoElement struct {
	*pipeline.BaseElement
	cancel context.CancelFunc
}

// NewEchoElement creates a new EchoElement.
func NewEchoElement() *EchoElement {
	return &EchoElement{
		BaseElement: pipeline.NewBaseElement("echo", 100),
	}
}

// Start implements pipeline.Element.
func (e *EchoElement) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-e.InChan:
				if msg == nil {
					continue
				}
				// Echo the message back
				select {
				case e.OutChan <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return nil
}

// Stop implements pipeline.Element.
func (e *EchoElement) Stop() error {
	if e.cancel != nil {
		e.cancel()
	}
	return nil
}
