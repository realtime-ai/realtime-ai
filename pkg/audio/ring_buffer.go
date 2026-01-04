// Package audio provides audio processing utilities.
//
// RingBuffer implements a fixed-size circular buffer for audio data.
// Used for pre-roll buffering in VAD to capture audio before speech detection.
//
// Main features:
//   - Fixed capacity based on sample rate and duration
//   - Thread-safe read/write operations
//   - Efficient circular buffer without memory allocation on write
//
// Usage:
//
//	rb := NewRingBuffer(16000, 300) // 300ms at 16kHz
//	rb.Write(audioData)
//	preRoll := rb.ReadAll()
package audio

import (
	"sync"
)

// RingBuffer is a fixed-size circular buffer for audio data (PCM bytes).
type RingBuffer struct {
	data     []byte
	capacity int // total capacity in bytes
	writePos int // next write position
	size     int // current data size (may be less than capacity initially)
	mu       sync.Mutex
}

// NewRingBuffer creates a new ring buffer for the specified duration.
// sampleRate: audio sample rate in Hz (e.g., 16000)
// durationMs: buffer duration in milliseconds (e.g., 300 for 300ms)
// Assumes 16-bit mono PCM (2 bytes per sample).
func NewRingBuffer(sampleRate, durationMs int) *RingBuffer {
	// Calculate capacity: sampleRate * durationMs / 1000 * 2 bytes per sample
	samples := sampleRate * durationMs / 1000
	capacity := samples * 2 // 16-bit = 2 bytes per sample

	return &RingBuffer{
		data:     make([]byte, capacity),
		capacity: capacity,
		writePos: 0,
		size:     0,
	}
}

// Write appends data to the ring buffer.
// If the buffer is full, oldest data is overwritten.
func (rb *RingBuffer) Write(data []byte) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	dataLen := len(data)
	if dataLen == 0 {
		return
	}

	// If incoming data is larger than capacity, only keep the last 'capacity' bytes
	if dataLen >= rb.capacity {
		copy(rb.data, data[dataLen-rb.capacity:])
		rb.writePos = 0
		rb.size = rb.capacity
		return
	}

	// Calculate how much space is available before wrap
	spaceToEnd := rb.capacity - rb.writePos

	if dataLen <= spaceToEnd {
		// Fits without wrapping
		copy(rb.data[rb.writePos:], data)
		rb.writePos += dataLen
		if rb.writePos == rb.capacity {
			rb.writePos = 0
		}
	} else {
		// Need to wrap around
		copy(rb.data[rb.writePos:], data[:spaceToEnd])
		copy(rb.data[0:], data[spaceToEnd:])
		rb.writePos = dataLen - spaceToEnd
	}

	// Update size (capped at capacity)
	rb.size += dataLen
	if rb.size > rb.capacity {
		rb.size = rb.capacity
	}
}

// ReadAll returns all buffered data in chronological order.
// Does not modify the buffer state.
func (rb *RingBuffer) ReadAll() []byte {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.size == 0 {
		return nil
	}

	result := make([]byte, rb.size)

	if rb.size < rb.capacity {
		// Buffer not yet full, data starts at 0
		copy(result, rb.data[:rb.size])
	} else {
		// Buffer is full, oldest data starts at writePos
		firstPartLen := rb.capacity - rb.writePos
		copy(result[:firstPartLen], rb.data[rb.writePos:])
		copy(result[firstPartLen:], rb.data[:rb.writePos])
	}

	return result
}

// Clear resets the buffer to empty state.
func (rb *RingBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.writePos = 0
	rb.size = 0
}

// Size returns the current amount of data in the buffer.
func (rb *RingBuffer) Size() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.size
}

// Capacity returns the total capacity of the buffer.
func (rb *RingBuffer) Capacity() int {
	return rb.capacity
}
