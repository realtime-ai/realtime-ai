package vad

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockDetector(t *testing.T) {
	t.Run("default returns zero probability", func(t *testing.T) {
		mock := NewMockDetector()

		prob, err := mock.Infer([]float32{0.1, 0.2, 0.3})
		require.NoError(t, err)
		assert.Equal(t, float32(0.0), prob)
	})

	t.Run("records infer calls", func(t *testing.T) {
		mock := NewMockDetector()

		mock.Infer([]float32{0.1, 0.2})
		mock.Infer([]float32{0.3, 0.4, 0.5})

		assert.Equal(t, 2, mock.GetInferCallCount())
		assert.Equal(t, []float32{0.1, 0.2}, mock.InferCalls[0])
		assert.Equal(t, []float32{0.3, 0.4, 0.5}, mock.InferCalls[1])
	})

	t.Run("reset and destroy tracking", func(t *testing.T) {
		mock := NewMockDetector()

		assert.False(t, mock.ResetCalled)
		assert.False(t, mock.DestroyCalled)

		mock.Reset()
		assert.True(t, mock.ResetCalled)

		mock.Destroy()
		assert.True(t, mock.DestroyCalled)
	})
}

func TestMockDetectorWithProb(t *testing.T) {
	mock := NewMockDetectorWithProb(0.75)

	prob1, err := mock.Infer([]float32{0.1})
	require.NoError(t, err)
	assert.Equal(t, float32(0.75), prob1)

	prob2, err := mock.Infer([]float32{0.2})
	require.NoError(t, err)
	assert.Equal(t, float32(0.75), prob2)
}

func TestMockDetectorWithSequence(t *testing.T) {
	probs := []float32{0.1, 0.5, 0.9}
	mock := NewMockDetectorWithSequence(probs)

	// First pass through sequence
	prob, _ := mock.Infer(nil)
	assert.Equal(t, float32(0.1), prob)

	prob, _ = mock.Infer(nil)
	assert.Equal(t, float32(0.5), prob)

	prob, _ = mock.Infer(nil)
	assert.Equal(t, float32(0.9), prob)

	// Should cycle back to beginning
	prob, _ = mock.Infer(nil)
	assert.Equal(t, float32(0.1), prob)
}

func TestMockDetectorWithSequenceEmpty(t *testing.T) {
	mock := NewMockDetectorWithSequence([]float32{})

	prob, err := mock.Infer(nil)
	require.NoError(t, err)
	assert.Equal(t, float32(0), prob)
}

func TestMockDetectorCustomInferFunc(t *testing.T) {
	callCount := 0
	mock := &MockDetector{
		InferFunc: func(samples []float32) (float32, error) {
			callCount++
			return float32(len(samples)) / 100.0, nil
		},
		InferCalls: make([][]float32, 0),
	}

	prob, err := mock.Infer(make([]float32, 50))
	require.NoError(t, err)
	assert.Equal(t, float32(0.5), prob)
	assert.Equal(t, 1, callCount)

	prob, err = mock.Infer(make([]float32, 100))
	require.NoError(t, err)
	assert.Equal(t, float32(1.0), prob)
	assert.Equal(t, 2, callCount)
}

func TestMockDetectorImplementsInterface(t *testing.T) {
	var _ DetectorInterface = (*MockDetector)(nil)
}
