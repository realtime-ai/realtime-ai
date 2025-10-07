package audio

import (
	"testing"

	"github.com/asticode/go-astiav"
	"github.com/stretchr/testify/assert"
)

func TestResample(t *testing.T) {
	tests := []struct {
		name          string
		inRate        int
		outRate       int
		inLayout      astiav.ChannelLayout
		outLayout     astiav.ChannelLayout
		inputSamples  int
		expectedError bool
	}{
		{
			name:          "48kHz to 16kHz mono",
			inRate:        48000,
			outRate:       16000,
			inLayout:      astiav.ChannelLayoutMono,
			outLayout:     astiav.ChannelLayoutMono,
			inputSamples:  960,
			expectedError: false,
		},
		{
			name:          "16kHz to 48kHz mono",
			inRate:        16000,
			outRate:       48000,
			inLayout:      astiav.ChannelLayoutMono,
			outLayout:     astiav.ChannelLayoutMono,
			inputSamples:  480, // 使用更大的输入以避免重采样误差
			expectedError: false,
		},
		{
			name:          "48kHz mono to stereo",
			inRate:        48000,
			outRate:       48000,
			inLayout:      astiav.ChannelLayoutMono,
			outLayout:     astiav.ChannelLayoutStereo,
			inputSamples:  960,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create resampler
			r, err := NewResample(tt.inRate, tt.outRate, tt.inLayout, tt.outLayout)
			if tt.expectedError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			defer r.Free()

			// Create test input data (16-bit samples)
			inputData := make([]byte, tt.inputSamples*2) // 2 bytes per sample for S16
			// Fill with a simple sine wave or test pattern
			for i := 0; i < len(inputData); i += 2 {
				// Simple ascending values for testing
				inputData[i] = byte(i % 256)
				inputData[i+1] = byte((i / 256) % 256)
			}

			// Perform resampling
			outputData, err := r.Resample(inputData)
			assert.NoError(t, err)
			assert.NotNil(t, outputData)

			// Verify output length is reasonable (allow some tolerance due to resampling)
			expectedSamples := (tt.inputSamples * tt.outRate) / tt.inRate
			expectedBytes := expectedSamples * 2 // 2 bytes per sample for S16
			if tt.outLayout == astiav.ChannelLayoutStereo {
				expectedBytes *= 2 // double for stereo
			}
			// Allow 10% tolerance for resampling artifacts
			tolerance := expectedBytes / 10
			assert.InDelta(t, expectedBytes, len(outputData), float64(tolerance),
				"Output size should be close to expected")
		})
	}
}

func TestResampleResourceCleanup(t *testing.T) {
	r, err := NewResample(48000, 16000, astiav.ChannelLayoutMono, astiav.ChannelLayoutMono)
	assert.NoError(t, err)
	assert.NotNil(t, r)

	// Verify resources are allocated
	assert.NotNil(t, r.ctx)
	assert.NotNil(t, r.inFrame)
	assert.NotNil(t, r.outFrame)

	// Free resources
	r.Free()

	// Verify resources are freed
	assert.Nil(t, r.ctx)
	assert.Nil(t, r.inFrame)
	assert.Nil(t, r.outFrame)
}

func TestResampleInvalidParams(t *testing.T) {
	tests := []struct {
		name      string
		inRate    int
		outRate   int
		inLayout  astiav.ChannelLayout
		outLayout astiav.ChannelLayout
	}{
		{
			name:      "Zero input rate",
			inRate:    0,
			outRate:   16000,
			inLayout:  astiav.ChannelLayoutMono,
			outLayout: astiav.ChannelLayoutMono,
		},
		{
			name:      "Zero output rate",
			inRate:    48000,
			outRate:   0,
			inLayout:  astiav.ChannelLayoutMono,
			outLayout: astiav.ChannelLayoutMono,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewResample(tt.inRate, tt.outRate, tt.inLayout, tt.outLayout)
			if r != nil {
				r.Free()
			}
			assert.Error(t, err)
		})
	}
}

func TestResampleEmptyInput(t *testing.T) {
	r, err := NewResample(48000, 16000, astiav.ChannelLayoutMono, astiav.ChannelLayoutMono)
	assert.NoError(t, err)
	defer r.Free()

	// Test with empty input
	output, err := r.Resample([]byte{})
	assert.Error(t, err)
	assert.Nil(t, output)

	// Test with nil input
	output, err = r.Resample(nil)
	assert.Error(t, err)
	assert.Nil(t, output)
}
