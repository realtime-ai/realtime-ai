package elements

import (
	"context"

	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

// Make sure LLMBaseElement implements pipeline.Element
var _ pipeline.Element = (*LLMBaseElement)(nil)

type LLMBaseElement struct {
	pipeline.BaseElement
}

func (e *LLMBaseElement) Start(ctx context.Context) error {
	return nil
}

func (e *LLMBaseElement) Stop() error {
	return nil
}
