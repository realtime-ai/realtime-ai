//go:build !vad

package elements

import (
	"context"
	"fmt"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
)

// VADMode defines the operating mode of the VAD element
type VADMode int

const (
	// VADModePassthrough passes all audio through and emits events
	VADModePassthrough VADMode = iota
	// VADModeFilter only passes audio segments containing speech
	VADModeFilter
)

// VADEventPayload contains information about VAD events
type VADEventPayload struct {
	SessionID  string
	Confidence float32
	Timestamp  time.Time
}

// SileroVADConfig holds configuration for Silero VAD
type SileroVADConfig struct {
	ModelPath       string
	Threshold       float32
	MinSilenceDurMs int
	SpeechPadMs     int
	Mode            VADMode
}

// SileroVADElement is a stub implementation when built without the 'vad' build tag
type SileroVADElement struct {
	*pipeline.BaseElement
}

// NewSileroVADElement returns an error indicating that VAD support is not built in
func NewSileroVADElement(config SileroVADConfig) (*SileroVADElement, error) {
	return nil, fmt.Errorf("VAD support is not enabled. Rebuild with '-tags vad' and ensure ONNX Runtime is installed")
}

// Init returns an error for stub implementation
func (e *SileroVADElement) Init(ctx context.Context) error {
	return fmt.Errorf("VAD support is not enabled")
}

// Start returns an error for stub implementation
func (e *SileroVADElement) Start(ctx context.Context) error {
	return fmt.Errorf("VAD support is not enabled")
}

// Stop returns an error for stub implementation
func (e *SileroVADElement) Stop() error {
	return fmt.Errorf("VAD support is not enabled")
}

// SetThreshold returns an error for stub implementation
func (e *SileroVADElement) SetThreshold(threshold float32) error {
	return fmt.Errorf("VAD support is not enabled")
}

// GetIsSpeaking returns false for stub implementation
func (e *SileroVADElement) GetIsSpeaking() bool {
	return false
}
