package elements

import (
	"context"

	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

// Make sure STTBaseElement implements pipeline.Element
var _ pipeline.Element = (*STTBaseElement)(nil)

type STTBaseElement struct {
	pipeline.BaseElement
}

func (e *STTBaseElement) Start(ctx context.Context) error {
	return nil
}

func (e *STTBaseElement) Stop() error {
	return nil
}
