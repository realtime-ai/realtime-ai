package vad

import "sync"

// MockDetector is a mock implementation of DetectorInterface for testing.
// It allows customizing the behavior of Infer through the InferFunc field.
type MockDetector struct {
	// InferFunc is called when Infer is invoked.
	// If nil, returns 0.0 (no speech detected).
	InferFunc func(samples []float32) (float32, error)

	// InferCalls records all calls to Infer for verification.
	InferCalls [][]float32

	// ResetCalled tracks if Reset was called.
	ResetCalled bool

	// DestroyCalled tracks if Destroy was called.
	DestroyCalled bool

	mu sync.Mutex
}

// NewMockDetector creates a new MockDetector with default behavior.
func NewMockDetector() *MockDetector {
	return &MockDetector{
		InferCalls: make([][]float32, 0),
	}
}

// NewMockDetectorWithProb creates a MockDetector that returns a fixed probability.
func NewMockDetectorWithProb(prob float32) *MockDetector {
	return &MockDetector{
		InferFunc: func(samples []float32) (float32, error) {
			return prob, nil
		},
		InferCalls: make([][]float32, 0),
	}
}

// NewMockDetectorWithSequence creates a MockDetector that returns probabilities in sequence.
// After all probabilities are returned, it cycles back to the beginning.
func NewMockDetectorWithSequence(probs []float32) *MockDetector {
	idx := 0
	return &MockDetector{
		InferFunc: func(samples []float32) (float32, error) {
			if len(probs) == 0 {
				return 0, nil
			}
			prob := probs[idx]
			idx = (idx + 1) % len(probs)
			return prob, nil
		},
		InferCalls: make([][]float32, 0),
	}
}

// Infer implements DetectorInterface.
func (m *MockDetector) Infer(samples []float32) (float32, error) {
	m.mu.Lock()
	// Make a copy to avoid issues with reused slices
	samplesCopy := make([]float32, len(samples))
	copy(samplesCopy, samples)
	m.InferCalls = append(m.InferCalls, samplesCopy)
	m.mu.Unlock()

	if m.InferFunc != nil {
		return m.InferFunc(samples)
	}
	return 0.0, nil
}

// Reset implements DetectorInterface.
func (m *MockDetector) Reset() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ResetCalled = true
	return nil
}

// Destroy implements DetectorInterface.
func (m *MockDetector) Destroy() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DestroyCalled = true
	return nil
}

// GetInferCallCount returns the number of times Infer was called.
func (m *MockDetector) GetInferCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.InferCalls)
}

// Ensure MockDetector implements DetectorInterface at compile time.
var _ DetectorInterface = (*MockDetector)(nil)
