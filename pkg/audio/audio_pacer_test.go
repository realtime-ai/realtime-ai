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

	t.Run("Empty buffer returns silence", func(t *testing.T) {
		frame := ap.ReadFrame()
		assert.Equal(t, BytesPerFrame48kHz, len(frame))
		// 验证是否为静音（全0）
		for _, b := range frame {
			assert.Equal(t, byte(0), b)
		}
	})

	t.Run("Write and read exact frame", func(t *testing.T) {
		// 创建一帧24kHz的测试数据
		testData := make([]byte, BytesPerFrame24kHz)
		for i := range testData {
			testData[i] = byte(i % 256)
		}

		err := ap.Write(testData)
		require.NoError(t, err)

		// 读取重采样后的48kHz数据
		frame := ap.ReadFrame()
		assert.Equal(t, BytesPerFrame48kHz, len(frame))
		// 由于经过重采样，我们不能直接比较数据内容
		// 但可以验证不是静音数据
		hasNonZero := false
		for _, b := range frame {
			if b != 0 {
				hasNonZero = true
				break
			}
		}
		assert.True(t, hasNonZero, "Resampled data should not be all zeros")
	})

	t.Run("Write partial frame", func(t *testing.T) {
		// 写入半帧24kHz数据
		halfFrame := BytesPerFrame24kHz / 2
		testData := make([]byte, halfFrame)
		for i := range testData {
			testData[i] = byte(i % 256)
		}

		err := ap.Write(testData)
		require.NoError(t, err)

		frame := ap.ReadFrame()
		assert.Equal(t, BytesPerFrame48kHz, len(frame))
		// 验证输出不全是静音
		hasNonZero := false
		for _, b := range frame {
			if b != 0 {
				hasNonZero = true
				break
			}
		}
		assert.True(t, hasNonZero, "Resampled data should not be all zeros")
	})

	t.Run("Write multiple frames", func(t *testing.T) {
		// 写入3帧24kHz数据
		testData := make([]byte, BytesPerFrame24kHz*3)
		for i := range testData {
			testData[i] = byte(i % 256)
		}

		err := ap.Write(testData)
		require.NoError(t, err)

		// 读取三帧48kHz数据
		for i := 0; i < 3; i++ {
			frame := ap.ReadFrame()
			assert.Equal(t, BytesPerFrame48kHz, len(frame))
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
		testData := make([]byte, BytesPerFrame24kHz)
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
		assert.Equal(t, BytesPerFrame48kHz, len(frame))
		for _, b := range frame {
			assert.Equal(t, byte(0), b)
		}
	})

	t.Run("Write empty data", func(t *testing.T) {
		err := ap.Write([]byte{})
		assert.NoError(t, err)
	})
}
