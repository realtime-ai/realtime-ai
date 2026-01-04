package vad

// DetectorInterface defines the interface for VAD detection.
// This interface allows for mock implementations in testing.
type DetectorInterface interface {
	// Infer runs inference on audio samples and returns the speech probability.
	// samples should be normalized float32 values in the range [-1, 1].
	// Returns a probability value in [0, 1] where higher values indicate speech.
	Infer(samples []float32) (float32, error)

	// Reset resets the detector's internal state.
	// This should be called when starting a new audio stream.
	Reset() error

	// Destroy releases all resources held by the detector.
	// The detector should not be used after calling Destroy.
	Destroy() error
}

// Ensure Detector implements DetectorInterface at compile time.
// This is commented out because it requires CGO which may not be available.
// var _ DetectorInterface = (*Detector)(nil)
