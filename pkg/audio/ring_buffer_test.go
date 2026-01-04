package audio

import (
	"bytes"
	"testing"
)

func TestNewRingBuffer(t *testing.T) {
	// 300ms at 16kHz = 4800 samples = 9600 bytes
	rb := NewRingBuffer(16000, 300)
	if rb.Capacity() != 9600 {
		t.Errorf("Expected capacity 9600, got %d", rb.Capacity())
	}
	if rb.Size() != 0 {
		t.Errorf("Expected size 0, got %d", rb.Size())
	}
}

func TestRingBuffer_WriteAndReadAll(t *testing.T) {
	rb := NewRingBuffer(16000, 100) // 100ms = 3200 bytes capacity

	// Write some data
	data1 := make([]byte, 1000)
	for i := range data1 {
		data1[i] = byte(i % 256)
	}
	rb.Write(data1)

	if rb.Size() != 1000 {
		t.Errorf("Expected size 1000, got %d", rb.Size())
	}

	// Read should return the same data
	result := rb.ReadAll()
	if !bytes.Equal(result, data1) {
		t.Error("ReadAll did not return expected data")
	}

	// Size should remain unchanged after read
	if rb.Size() != 1000 {
		t.Errorf("Expected size 1000 after read, got %d", rb.Size())
	}
}

func TestRingBuffer_Wraparound(t *testing.T) {
	rb := NewRingBuffer(16000, 100) // 3200 bytes capacity

	// Write data that fills most of the buffer
	data1 := make([]byte, 2000)
	for i := range data1 {
		data1[i] = 1
	}
	rb.Write(data1)

	// Write more data that causes wraparound
	data2 := make([]byte, 2000)
	for i := range data2 {
		data2[i] = 2
	}
	rb.Write(data2)

	// Buffer should be at capacity
	if rb.Size() != rb.Capacity() {
		t.Errorf("Expected buffer to be full, got size %d", rb.Size())
	}

	// Read should return most recent data (data2 + part of nothing, since it wrapped)
	result := rb.ReadAll()
	if len(result) != rb.Capacity() {
		t.Errorf("Expected %d bytes, got %d", rb.Capacity(), len(result))
	}

	// Last 2000 bytes should be data2
	last2000 := result[len(result)-2000:]
	for i, b := range last2000 {
		if b != 2 {
			t.Errorf("Expected byte 2 at position %d, got %d", i, b)
			break
		}
	}
}

func TestRingBuffer_OverwriteLargeData(t *testing.T) {
	rb := NewRingBuffer(16000, 100) // 3200 bytes capacity

	// Write data larger than capacity
	data := make([]byte, 5000)
	for i := range data {
		data[i] = byte(i % 256)
	}
	rb.Write(data)

	// Should only keep last 3200 bytes
	if rb.Size() != rb.Capacity() {
		t.Errorf("Expected size %d, got %d", rb.Capacity(), rb.Size())
	}

	result := rb.ReadAll()
	expected := data[len(data)-rb.Capacity():]
	if !bytes.Equal(result, expected) {
		t.Error("ReadAll did not return expected tail of large data")
	}
}

func TestRingBuffer_Clear(t *testing.T) {
	rb := NewRingBuffer(16000, 100)

	rb.Write(make([]byte, 1000))
	rb.Clear()

	if rb.Size() != 0 {
		t.Errorf("Expected size 0 after clear, got %d", rb.Size())
	}

	result := rb.ReadAll()
	if result != nil {
		t.Error("Expected nil from ReadAll after clear")
	}
}

func TestRingBuffer_EmptyRead(t *testing.T) {
	rb := NewRingBuffer(16000, 100)

	result := rb.ReadAll()
	if result != nil {
		t.Error("Expected nil from ReadAll on empty buffer")
	}
}
