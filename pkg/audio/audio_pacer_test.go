package audio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAudioPacer(t *testing.T) {
	ap, err := NewAudioPacer()
	require.NoError(t, err)
	defer ap.Close()

	// 24kHz 下 20ms 的帧大小: 24000 * 20 / 1000 * 2 * 1 = 960 bytes
	expectedFrameSize := ap.BytesPerFrame()

	t.Run("Empty buffer returns silence", func(t *testing.T) {
		frame := ap.ReadFrame()
		assert.Equal(t, expectedFrameSize, len(frame))
		// 验证是否为静音（全0）
		for _, b := range frame {
			assert.Equal(t, byte(0), b)
		}
	})

	t.Run("Write and read exact frame", func(t *testing.T) {
		// 创建一帧测试数据
		testData := make([]byte, expectedFrameSize)
		for i := range testData {
			testData[i] = byte(i % 256)
		}

		err := ap.Write(testData)
		require.NoError(t, err)

		// 读取数据
		frame := ap.ReadFrame()
		assert.Equal(t, expectedFrameSize, len(frame))
		// 验证不是静音数据
		hasNonZero := false
		for _, b := range frame {
			if b != 0 {
				hasNonZero = true
				break
			}
		}
		assert.True(t, hasNonZero, "Data should not be all zeros")
	})

	t.Run("Write partial frame", func(t *testing.T) {
		// 写入半帧数据
		halfFrame := expectedFrameSize / 2
		testData := make([]byte, halfFrame)
		for i := range testData {
			testData[i] = byte(i % 256)
		}

		err := ap.Write(testData)
		require.NoError(t, err)

		frame := ap.ReadFrame()
		assert.Equal(t, expectedFrameSize, len(frame))
		// 验证输出不全是静音（部分数据被复制，其余为0）
		hasNonZero := false
		for _, b := range frame {
			if b != 0 {
				hasNonZero = true
				break
			}
		}
		assert.True(t, hasNonZero, "Data should not be all zeros")
	})

	t.Run("Write multiple frames", func(t *testing.T) {
		// 写入3帧数据
		testData := make([]byte, expectedFrameSize*3)
		for i := range testData {
			testData[i] = byte(i % 256)
		}

		err := ap.Write(testData)
		require.NoError(t, err)

		// 读取三帧数据
		for i := 0; i < 3; i++ {
			frame := ap.ReadFrame()
			assert.Equal(t, expectedFrameSize, len(frame))
			// 验证不是静音
			hasNonZero := false
			for _, b := range frame {
				if b != 0 {
					hasNonZero = true
					break
				}
			}
			assert.True(t, hasNonZero, "Frame %d should not be all zeros", i)
		}

		// 第四帧应该是静音
		frame := ap.ReadFrame()
		for _, b := range frame {
			assert.Equal(t, byte(0), b)
		}
	})

	t.Run("Clear buffer", func(t *testing.T) {
		// 写入一些数据
		testData := make([]byte, expectedFrameSize)
		for i := range testData {
			testData[i] = byte(i % 256)
		}
		err := ap.Write(testData)
		require.NoError(t, err)

		// 清空缓冲区
		ap.Clear()
		assert.Equal(t, 0, ap.Available())

		// 验证读取返回静音
		frame := ap.ReadFrame()
		assert.Equal(t, expectedFrameSize, len(frame))
		for _, b := range frame {
			assert.Equal(t, byte(0), b)
		}
	})

	t.Run("Write empty data", func(t *testing.T) {
		err := ap.Write([]byte{})
		assert.NoError(t, err)
	})
}

func TestAudioPacer_PauseResume(t *testing.T) {
	ap, err := NewAudioPacer()
	require.NoError(t, err)
	defer ap.Close()

	expectedFrameSize := ap.BytesPerFrame()

	t.Run("Pause returns silence", func(t *testing.T) {
		// Write some data
		testData := make([]byte, expectedFrameSize*5)
		for i := range testData {
			testData[i] = byte((i % 254) + 1) // Non-zero data
		}
		err := ap.Write(testData)
		require.NoError(t, err)

		// Verify data is available
		assert.True(t, ap.Available() > 0)
		assert.False(t, ap.IsPaused())

		// Pause
		ap.Pause()
		assert.True(t, ap.IsPaused())

		// Read should return silence while paused
		frame := ap.ReadFrame()
		assert.Equal(t, expectedFrameSize, len(frame))
		for _, b := range frame {
			assert.Equal(t, byte(0), b, "Paused frame should be silence")
		}

		// Data should still be in buffer
		assert.True(t, ap.Available() > 0)
	})

	t.Run("Resume continues playback", func(t *testing.T) {
		// Resume
		ap.Resume()
		assert.False(t, ap.IsPaused())

		// Read should return actual data now
		frame := ap.ReadFrame()
		assert.Equal(t, expectedFrameSize, len(frame))

		hasNonZero := false
		for _, b := range frame {
			if b != 0 {
				hasNonZero = true
				break
			}
		}
		assert.True(t, hasNonZero, "Resumed frame should have data")
	})
}

func TestAudioPacer_ClearWithFadeOut(t *testing.T) {
	ap, err := NewAudioPacerWithConfig(AudioPacerConfig{
		SampleRate: 48000,
		Channels:   1,
	})
	require.NoError(t, err)
	defer ap.Close()

	expectedFrameSize := ap.BytesPerFrame()

	t.Run("FadeOut applies to buffer", func(t *testing.T) {
		// Write constant amplitude data (simulating audio)
		testData := make([]byte, expectedFrameSize*10) // 200ms of data
		for i := 0; i < len(testData); i += 2 {
			// Write 16-bit samples with value 0x4000 (16384)
			testData[i] = 0x00
			testData[i+1] = 0x40
		}
		err := ap.Write(testData)
		require.NoError(t, err)

		// Clear with 50ms fadeout
		ap.ClearWithFadeOut(50)

		// Buffer should be mostly cleared, accumulating mode
		assert.True(t, ap.Available() <= expectedFrameSize*3) // At most ~50ms of fadeout data
	})

	t.Run("ClearWithFadeOut zero ms clears immediately", func(t *testing.T) {
		// Write some data
		testData := make([]byte, expectedFrameSize*5)
		for i := range testData {
			testData[i] = byte(i % 256)
		}
		err := ap.Write(testData)
		require.NoError(t, err)

		// Clear with 0ms fadeout
		ap.ClearWithFadeOut(0)

		// Buffer should be empty
		assert.Equal(t, 0, ap.Available())
	})
}

func TestAudioPacerWithConfig(t *testing.T) {
	t.Run("Custom sample rate 48kHz", func(t *testing.T) {
		ap, err := NewAudioPacerWithConfig(AudioPacerConfig{
			SampleRate: 48000,
			Channels:   1,
		})
		require.NoError(t, err)
		defer ap.Close()

		// 48kHz 下 20ms 的帧大小: 48000 * 20 / 1000 * 2 * 1 = 1920 bytes
		expectedFrameSize := 1920
		assert.Equal(t, expectedFrameSize, ap.BytesPerFrame())
		assert.Equal(t, 48000, ap.SampleRate())
	})

	t.Run("Custom sample rate 16kHz", func(t *testing.T) {
		ap, err := NewAudioPacerWithConfig(AudioPacerConfig{
			SampleRate: 16000,
			Channels:   1,
		})
		require.NoError(t, err)
		defer ap.Close()

		// 16kHz 下 20ms 的帧大小: 16000 * 20 / 1000 * 2 * 1 = 640 bytes
		expectedFrameSize := 640
		assert.Equal(t, expectedFrameSize, ap.BytesPerFrame())
		assert.Equal(t, 16000, ap.SampleRate())
	})
}
