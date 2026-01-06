package audio

import (
	"testing"
)

func TestMuLawEncodeDecode(t *testing.T) {
	// Test round-trip encoding/decoding
	testSamples := []int16{0, 100, 1000, 10000, 32000, -100, -1000, -10000, -32000}

	for _, original := range testSamples {
		encoded := MuLawEncode(original)
		decoded := MuLawDecode(encoded)

		// μ-law is lossy, so we check if decoded is close to original
		// The error should be within the quantization step for the segment
		diff := original - decoded
		if diff < 0 {
			diff = -diff
		}

		// Allow up to 5% error or 200 absolute for μ-law lossy compression
		// Large values have larger quantization steps
		absOriginal := original
		if absOriginal < 0 {
			absOriginal = -absOriginal
		}
		maxError := int16(float64(absOriginal) * 0.05)
		if maxError < 200 {
			maxError = 200 // Minimum error threshold
		}

		if diff > maxError && original != 0 {
			t.Errorf("MuLaw round-trip for %d: encoded=%02x, decoded=%d, diff=%d (max allowed: %d)", original, encoded, decoded, diff, maxError)
		}
	}
}

func TestMuLawToPCM(t *testing.T) {
	// Test buffer conversion
	mulaw := []byte{0x7F, 0xFF, 0x00, 0x80} // Some μ-law samples
	pcm := MuLawToPCM(mulaw)

	if len(pcm) != len(mulaw)*2 {
		t.Errorf("Expected PCM length %d, got %d", len(mulaw)*2, len(pcm))
	}

	// Verify individual conversions
	for i, b := range mulaw {
		expected := MuLawDecode(b)
		got := int16(pcm[i*2]) | (int16(pcm[i*2+1]) << 8)
		if got != expected {
			t.Errorf("Sample %d: expected %d, got %d", i, expected, got)
		}
	}
}

func TestPCMToMuLaw(t *testing.T) {
	// Create PCM samples
	samples := []int16{0, 1000, -1000, 10000, -10000}
	pcm := make([]byte, len(samples)*2)
	for i, s := range samples {
		pcm[i*2] = byte(s)
		pcm[i*2+1] = byte(s >> 8)
	}

	mulaw := PCMToMuLaw(pcm)

	if len(mulaw) != len(samples) {
		t.Errorf("Expected μ-law length %d, got %d", len(samples), len(mulaw))
	}

	// Verify individual conversions
	for i, s := range samples {
		expected := MuLawEncode(s)
		if mulaw[i] != expected {
			t.Errorf("Sample %d (%d): expected %02x, got %02x", i, s, expected, mulaw[i])
		}
	}
}

func TestMuLawDecodeLookupTable(t *testing.T) {
	// Verify a few known μ-law values
	// 0x7F (127) should decode to near 0
	decoded := MuLawDecode(0x7F)
	if decoded != 0 {
		t.Errorf("μ-law 0x7F should decode to 0, got %d", decoded)
	}

	// 0xFF (255) should decode to near 0
	decoded = MuLawDecode(0xFF)
	if decoded != 0 {
		t.Errorf("μ-law 0xFF should decode to 0, got %d", decoded)
	}

	// 0x00 should decode to max negative
	decoded = MuLawDecode(0x00)
	if decoded >= 0 {
		t.Errorf("μ-law 0x00 should decode to negative value, got %d", decoded)
	}

	// 0x80 should decode to max positive
	decoded = MuLawDecode(0x80)
	if decoded <= 0 {
		t.Errorf("μ-law 0x80 should decode to positive value, got %d", decoded)
	}
}

func BenchmarkMuLawDecode(b *testing.B) {
	mulaw := make([]byte, 8000) // 1 second at 8kHz
	for i := range mulaw {
		mulaw[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MuLawToPCM(mulaw)
	}
}

func BenchmarkMuLawEncode(b *testing.B) {
	pcm := make([]byte, 16000) // 1 second at 8kHz, 16-bit
	for i := 0; i < len(pcm); i += 2 {
		sample := int16((i / 2) * 10)
		pcm[i] = byte(sample)
		pcm[i+1] = byte(sample >> 8)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = PCMToMuLaw(pcm)
	}
}
