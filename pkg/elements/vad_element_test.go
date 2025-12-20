//go:build vad

package elements

import (
	"context"
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateSilence generates silent audio samples
func generateSilence(numSamples int) []byte {
	data := make([]byte, numSamples*2)
	return data
}

// generateTone generates a simple sine wave tone
func generateTone(numSamples int, frequency float64, sampleRate int) []byte {
	data := make([]byte, numSamples*2)
	amplitude := float64(10000) // Loud enough to be detected as speech

	for i := 0; i < numSamples; i++ {
		sample := int16(amplitude * math.Sin(2*math.Pi*frequency*float64(i)/float64(sampleRate)))
		binary.LittleEndian.PutUint16(data[i*2:i*2+2], uint16(sample))
	}

	return data
}

// TestNewSileroVADElement tests element creation
func TestNewSileroVADElement(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := SileroVADConfig{
			ModelPath:       "test_model.onnx",
			Threshold:       0.5,
			MinSilenceDurMs: 100,
			SpeechPadMs:     30,
			Mode:            VADModePassthrough,
		}

		elem, err := NewSileroVADElement(config)
		require.NoError(t, err)
		assert.NotNil(t, elem)
		assert.Equal(t, float32(0.5), elem.threshold)
		assert.Equal(t, 100, elem.minSilenceDurMs)
		assert.Equal(t, 30, elem.speechPadMs)
		assert.Equal(t, VADModePassthrough, elem.mode)
	})

	t.Run("missing model path", func(t *testing.T) {
		config := SileroVADConfig{
			Threshold: 0.5,
		}

		elem, err := NewSileroVADElement(config)
		assert.Error(t, err)
		assert.Nil(t, elem)
		assert.Contains(t, err.Error(), "model path is required")
	})

	t.Run("default values", func(t *testing.T) {
		config := SileroVADConfig{
			ModelPath: "test_model.onnx",
		}

		elem, err := NewSileroVADElement(config)
		require.NoError(t, err)
		assert.Equal(t, float32(0.5), elem.threshold)
		assert.Equal(t, 100, elem.minSilenceDurMs)
		assert.Equal(t, 30, elem.speechPadMs)
	})
}

// TestVADElementProperties tests property registration and access
func TestVADElementProperties(t *testing.T) {
	config := SileroVADConfig{
		ModelPath: "test_model.onnx",
		Threshold: 0.5,
	}

	elem, err := NewSileroVADElement(config)
	require.NoError(t, err)

	t.Run("read threshold property", func(t *testing.T) {
		val, err := elem.GetProperty("threshold")
		require.NoError(t, err)
		assert.Equal(t, float32(0.5), val)
	})

	t.Run("read mode property", func(t *testing.T) {
		val, err := elem.GetProperty("mode")
		require.NoError(t, err)
		assert.Equal(t, int(VADModePassthrough), val)
	})

	t.Run("write threshold property", func(t *testing.T) {
		err := elem.SetProperty("threshold", float32(0.7))
		require.NoError(t, err)

		val, err := elem.GetProperty("threshold")
		require.NoError(t, err)
		assert.Equal(t, float32(0.7), val)
	})

	t.Run("invalid property", func(t *testing.T) {
		_, err := elem.GetProperty("invalid-property")
		assert.Error(t, err)
	})
}

// TestVADElementLifecycle tests Start and Stop
func TestVADElementLifecycle(t *testing.T) {
	t.Skip("Skipping test that requires VAD model file")

	config := SileroVADConfig{
		ModelPath: "../../models/silero_vad.onnx",
		Threshold: 0.5,
		Mode:      VADModePassthrough,
	}

	elem, err := NewSileroVADElement(config)
	require.NoError(t, err)

	ctx := context.Background()

	// Initialize
	err = elem.Init(ctx)
	require.NoError(t, err)

	// Start
	err = elem.Start(ctx)
	require.NoError(t, err)

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	// Stop
	err = elem.Stop()
	require.NoError(t, err)

	// Should be able to stop multiple times
	err = elem.Stop()
	require.NoError(t, err)
}

// TestVADElementPassthroughMode tests passthrough mode
func TestVADElementPassthroughMode(t *testing.T) {
	t.Skip("Skipping test that requires VAD model file")

	config := SileroVADConfig{
		ModelPath: "../../models/silero_vad.onnx",
		Threshold: 0.5,
		Mode:      VADModePassthrough,
	}

	elem, err := NewSileroVADElement(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = elem.Init(ctx)
	require.NoError(t, err)

	err = elem.Start(ctx)
	require.NoError(t, err)
	defer elem.Stop()

	// Send audio message
	audioData := generateTone(16000, 440, 16000) // 1 second of 440Hz tone
	msg := &pipeline.PipelineMessage{
		Type:      pipeline.MsgTypeAudio,
		SessionID: "test-session",
		Timestamp: time.Now(),
		AudioData: &pipeline.AudioData{
			Data:       audioData,
			SampleRate: 16000,
			Channels:   1,
			MediaType:  pipeline.AudioMediaTypeRaw,
			Timestamp:  time.Now(),
		},
	}

	// Send message
	elem.In() <- msg

	// In passthrough mode, should receive output
	select {
	case outMsg := <-elem.Out():
		assert.NotNil(t, outMsg)
		assert.Equal(t, msg.SessionID, outMsg.SessionID)
	case <-time.After(2 * time.Second):
		t.Fatal("Expected output message in passthrough mode")
	}
}

// TestVADElementFilterMode tests filter mode
func TestVADElementFilterMode(t *testing.T) {
	t.Skip("Skipping test that requires VAD model file")

	config := SileroVADConfig{
		ModelPath: "../../models/silero_vad.onnx",
		Threshold: 0.5,
		Mode:      VADModeFilter,
	}

	elem, err := NewSileroVADElement(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = elem.Init(ctx)
	require.NoError(t, err)

	err = elem.Start(ctx)
	require.NoError(t, err)
	defer elem.Stop()

	// Send silence - should be filtered out
	silenceData := generateSilence(8000) // 0.5 seconds of silence
	silenceMsg := &pipeline.PipelineMessage{
		Type:      pipeline.MsgTypeAudio,
		SessionID: "test-session",
		Timestamp: time.Now(),
		AudioData: &pipeline.AudioData{
			Data:       silenceData,
			SampleRate: 16000,
			Channels:   1,
			MediaType:  pipeline.AudioMediaTypeRaw,
			Timestamp:  time.Now(),
		},
	}

	elem.In() <- silenceMsg

	// Should not receive output for silence in filter mode
	select {
	case <-elem.Out():
		t.Fatal("Should not receive output for silence in filter mode")
	case <-time.After(1 * time.Second):
		// Expected timeout
	}
}

// TestBytesToInt16 tests the byte conversion function
func TestBytesToInt16(t *testing.T) {
	config := SileroVADConfig{
		ModelPath: "test_model.onnx",
	}

	elem, err := NewSileroVADElement(config)
	require.NoError(t, err)

	t.Run("convert bytes to int16", func(t *testing.T) {
		data := []byte{0x00, 0x10, 0xFF, 0x7F, 0x00, 0x80}
		samples := elem.bytesToInt16(data)

		assert.Equal(t, 3, len(samples))
		assert.Equal(t, int16(0x1000), samples[0])
		assert.Equal(t, int16(0x7FFF), samples[1])
		assert.Equal(t, int16(-32768), samples[2])
	})

	t.Run("empty data", func(t *testing.T) {
		data := []byte{}
		samples := elem.bytesToInt16(data)
		assert.Equal(t, 0, len(samples))
	})
}

// TestSetThreshold tests threshold setting
func TestSetThreshold(t *testing.T) {
	config := SileroVADConfig{
		ModelPath: "test_model.onnx",
		Threshold: 0.5,
	}

	elem, err := NewSileroVADElement(config)
	require.NoError(t, err)

	t.Run("valid threshold", func(t *testing.T) {
		err := elem.SetThreshold(0.7)
		require.NoError(t, err)
		assert.Equal(t, float32(0.7), elem.threshold)
	})

	t.Run("invalid threshold - too low", func(t *testing.T) {
		err := elem.SetThreshold(-0.1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "threshold must be between 0 and 1")
	})

	t.Run("invalid threshold - too high", func(t *testing.T) {
		err := elem.SetThreshold(1.5)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "threshold must be between 0 and 1")
	})

	t.Run("boundary values", func(t *testing.T) {
		err := elem.SetThreshold(0.0)
		require.NoError(t, err)
		assert.Equal(t, float32(0.0), elem.threshold)

		err = elem.SetThreshold(1.0)
		require.NoError(t, err)
		assert.Equal(t, float32(1.0), elem.threshold)
	})
}

// TestGetIsSpeaking tests the speech state getter
func TestGetIsSpeaking(t *testing.T) {
	config := SileroVADConfig{
		ModelPath: "test_model.onnx",
	}

	elem, err := NewSileroVADElement(config)
	require.NoError(t, err)

	// Initially should not be speaking
	assert.False(t, elem.GetIsSpeaking())

	// Manually set speaking state for testing
	elem.isSpeaking = true
	assert.True(t, elem.GetIsSpeaking())

	elem.isSpeaking = false
	assert.False(t, elem.GetIsSpeaking())
}

// TestVADElementSampleRateValidation tests sample rate validation
func TestVADElementSampleRateValidation(t *testing.T) {
	t.Skip("Skipping test that requires VAD model file")

	config := SileroVADConfig{
		ModelPath: "../../models/silero_vad.onnx",
		Threshold: 0.5,
		Mode:      VADModePassthrough,
	}

	elem, err := NewSileroVADElement(config)
	require.NoError(t, err)

	ctx := context.Background()
	err = elem.Init(ctx)
	require.NoError(t, err)

	err = elem.Start(ctx)
	require.NoError(t, err)
	defer elem.Stop()

	// Send audio with wrong sample rate
	audioData := generateTone(8000, 440, 8000)
	msg := &pipeline.PipelineMessage{
		Type:      pipeline.MsgTypeAudio,
		SessionID: "test-session",
		Timestamp: time.Now(),
		AudioData: &pipeline.AudioData{
			Data:       audioData,
			SampleRate: 8000, // Wrong sample rate
			Channels:   1,
			MediaType:  pipeline.AudioMediaTypeRaw,
			Timestamp:  time.Now(),
		},
	}

	elem.In() <- msg

	// Should not receive output for wrong sample rate
	select {
	case <-elem.Out():
		t.Fatal("Should not receive output for wrong sample rate")
	case <-time.After(1 * time.Second):
		// Expected - audio was rejected
	}
}
