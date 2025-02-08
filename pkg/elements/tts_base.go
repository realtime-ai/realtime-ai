package elements

import (
	"context"

	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

// Make sure TTSBaseElement implements pipeline.Element
var _ pipeline.Element = (*TTSBaseElement)(nil)

type TTSBaseElement struct {
	pipeline.BaseElement
}

func (e *TTSBaseElement) Start(ctx context.Context) error {
	return nil
}

func (e *TTSBaseElement) Stop() error {
	return nil
}
