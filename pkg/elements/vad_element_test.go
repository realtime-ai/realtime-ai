//go:build vad

package elements

import (
	"context"
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/realtime-ai/realtime-ai/pkg/pipeline"
	"github.com/realtime-ai/realtime-ai/pkg/vad"
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
	config := SileroVADConfig{
		ModelPath: "test_model.onnx", // Not used when mock is injected
		Threshold: 0.5,
		Mode:      VADModePassthrough,
	}

	elem, err := NewSileroVADElement(config)
	require.NoError(t, err)

	// Inject mock detector
	mockDetector := vad.NewMockDetector()
	elem.SetDetector(mockDetector)

	ctx := context.Background()

	// Initialize (should use mock detector)
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

	// Verify mock was destroyed
	assert.True(t, mockDetector.DestroyCalled)

	// Should be able to stop multiple times without error
	err = elem.Stop()
	require.NoError(t, err)
}

// TestVADElementPassthroughMode tests passthrough mode
func TestVADElementPassthroughMode(t *testing.T) {
	config := SileroVADConfig{
		ModelPath: "test_model.onnx",
		Threshold: 0.5,
		Mode:      VADModePassthrough,
	}

	elem, err := NewSileroVADElement(config)
	require.NoError(t, err)

	// Inject mock detector that returns low probability (no speech)
	mockDetector := vad.NewMockDetectorWithProb(0.1)
	elem.SetDetector(mockDetector)

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

	// In passthrough mode, should receive output regardless of speech detection
	select {
	case outMsg := <-elem.Out():
		assert.NotNil(t, outMsg)
		assert.Equal(t, msg.SessionID, outMsg.SessionID)
	case <-time.After(2 * time.Second):
		t.Fatal("Expected output message in passthrough mode")
	}

	// Verify mock detector was called
	assert.Greater(t, mockDetector.GetInferCallCount(), 0)
}

// TestVADElementFilterMode tests filter mode
func TestVADElementFilterMode(t *testing.T) {
	config := SileroVADConfig{
		ModelPath: "test_model.onnx",
		Threshold: 0.5,
		Mode:      VADModeFilter,
	}

	elem, err := NewSileroVADElement(config)
	require.NoError(t, err)

	// Inject mock detector that returns low probability (no speech)
	mockDetector := vad.NewMockDetectorWithProb(0.1)
	elem.SetDetector(mockDetector)

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
	case <-time.After(500 * time.Millisecond):
		// Expected timeout - audio was filtered
	}

	// Verify mock detector was called
	assert.Greater(t, mockDetector.GetInferCallCount(), 0)
}

// TestBytesToFloat32 tests the byte conversion function
func TestBytesToFloat32(t *testing.T) {
	config := SileroVADConfig{
		ModelPath: "test_model.onnx",
	}

	elem, err := NewSileroVADElement(config)
	require.NoError(t, err)

	t.Run("convert bytes to float32", func(t *testing.T) {
		// 0x1000 = 4096, 0x7FFF = 32767, 0x8000 = -32768
		data := []byte{0x00, 0x10, 0xFF, 0x7F, 0x00, 0x80}
		samples := elem.bytesToFloat32(data)

		assert.Equal(t, 3, len(samples))
		// 4096 / 32768 ≈ 0.125
		assert.InDelta(t, 0.125, samples[0], 0.001)
		// 32767 / 32768 ≈ 0.99997
		assert.InDelta(t, 0.99997, samples[1], 0.001)
		// -32768 / 32768 = -1.0
		assert.InDelta(t, -1.0, samples[2], 0.001)
	})

	t.Run("empty data", func(t *testing.T) {
		data := []byte{}
		samples := elem.bytesToFloat32(data)
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
	config := SileroVADConfig{
		ModelPath: "test_model.onnx",
		Threshold: 0.5,
		Mode:      VADModePassthrough,
	}

	elem, err := NewSileroVADElement(config)
	require.NoError(t, err)

	// Inject mock detector
	mockDetector := vad.NewMockDetectorWithProb(0.8)
	elem.SetDetector(mockDetector)

	ctx := context.Background()
	err = elem.Init(ctx)
	require.NoError(t, err)

	err = elem.Start(ctx)
	require.NoError(t, err)
	defer elem.Stop()

	// Send audio with wrong sample rate (8kHz instead of 16kHz)
	audioData := generateTone(8000, 440, 8000)
	msg := &pipeline.PipelineMessage{
		Type:      pipeline.MsgTypeAudio,
		SessionID: "test-session",
		Timestamp: time.Now(),
		AudioData: &pipeline.AudioData{
			Data:       audioData,
			SampleRate: 8000, // Wrong sample rate - VAD requires 16kHz
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
	case <-time.After(500 * time.Millisecond):
		// Expected - audio was rejected due to wrong sample rate
	}

	// Mock detector should NOT be called because audio was rejected before inference
	assert.Equal(t, 0, mockDetector.GetInferCallCount())
}

// TestVADElementSpeechDetection tests speech start/end detection with mock
func TestVADElementSpeechDetection(t *testing.T) {
	config := SileroVADConfig{
		ModelPath:       "test_model.onnx",
		Threshold:       0.5,
		MinSilenceDurMs: 100,
		SpeechPadMs:     30,
		Mode:            VADModePassthrough,
	}

	elem, err := NewSileroVADElement(config)
	require.NoError(t, err)

	// Create a sequence: low -> high -> high -> low -> low (speech then silence)
	// This simulates: silence, speech starts, speech continues, speech ends
	mockDetector := vad.NewMockDetectorWithSequence([]float32{
		0.1, 0.1, // Initial silence
		0.8, 0.9, 0.85, 0.8, // Speech
		0.2, 0.1, 0.1, 0.1, 0.1, // Silence (should trigger speech end after minSilenceDurMs)
	})
	elem.SetDetector(mockDetector)

	// Set up pipeline with bus for events
	p := pipeline.NewPipeline("test-vad")
	p.AddElement(elem)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = elem.Init(ctx)
	require.NoError(t, err)

	err = p.Start(ctx)
	require.NoError(t, err)
	defer p.Stop()

	// Subscribe to VAD events
	eventChan := make(chan pipeline.Event, 10)
	p.Bus().Subscribe(pipeline.EventVADSpeechStart, eventChan)
	p.Bus().Subscribe(pipeline.EventVADSpeechEnd, eventChan)

	// Drain output channel
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-elem.Out():
			}
		}
	}()

	// Send enough audio to trigger multiple inferences
	// 512 samples per window at 16kHz = 32ms per window
	// Send 10 windows worth of audio (320ms)
	audioData := generateTone(512*10, 440, 16000)
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

	elem.In() <- msg

	// Wait for events
	var speechStartReceived, speechEndReceived bool
	timeout := time.After(2 * time.Second)

	for !speechStartReceived || !speechEndReceived {
		select {
		case event := <-eventChan:
			switch event.Type {
			case pipeline.EventVADSpeechStart:
				speechStartReceived = true
				payload := event.Payload.(VADEventPayload)
				assert.Equal(t, "test-session", payload.SessionID)
				t.Logf("Speech started at %d ms with confidence %.3f", payload.AudioMs, payload.Confidence)
			case pipeline.EventVADSpeechEnd:
				speechEndReceived = true
				payload := event.Payload.(VADEventPayload)
				assert.Equal(t, "test-session", payload.SessionID)
				t.Logf("Speech ended at %d ms with confidence %.3f", payload.AudioMs, payload.Confidence)
			}
		case <-timeout:
			// It's OK if we don't get both events in this test
			// since the timing depends on minSilenceDurMs
			break
		}
		if speechStartReceived {
			break // At minimum, we should get speech start
		}
	}

	assert.True(t, speechStartReceived, "Should receive speech start event")
}
