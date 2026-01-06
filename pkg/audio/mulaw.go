// Package audio provides audio processing utilities.
//
// mulaw.go implements μ-law (G.711) audio codec conversions.
// μ-law is the standard audio encoding for telephone systems in North America and Japan.
//
// Features:
//   - μ-law to Linear PCM (16-bit signed) conversion
//   - Linear PCM to μ-law conversion
//   - Optimized lookup tables for fast conversion
//
// Reference: ITU-T G.711 specification

package audio

// MuLaw codec constants
const (
	MuLawBias     = 0x84  // Bias for linear code
	MuLawMax      = 32635 // Maximum linear value
	MuLawClip     = 32635
	MuLawSegShift = 4
	MuLawSegMask  = 0x70
	MuLawQuantMask = 0x0f
)

// muLawDecompressTable is a pre-computed lookup table for μ-law to linear PCM conversion.
// Each μ-law byte maps to a 16-bit signed PCM value.
var muLawDecompressTable = [256]int16{
	-32124, -31100, -30076, -29052, -28028, -27004, -25980, -24956,
	-23932, -22908, -21884, -20860, -19836, -18812, -17788, -16764,
	-15996, -15484, -14972, -14460, -13948, -13436, -12924, -12412,
	-11900, -11388, -10876, -10364, -9852, -9340, -8828, -8316,
	-7932, -7676, -7420, -7164, -6908, -6652, -6396, -6140,
	-5884, -5628, -5372, -5116, -4860, -4604, -4348, -4092,
	-3900, -3772, -3644, -3516, -3388, -3260, -3132, -3004,
	-2876, -2748, -2620, -2492, -2364, -2236, -2108, -1980,
	-1884, -1820, -1756, -1692, -1628, -1564, -1500, -1436,
	-1372, -1308, -1244, -1180, -1116, -1052, -988, -924,
	-876, -844, -812, -780, -748, -716, -684, -652,
	-620, -588, -556, -524, -492, -460, -428, -396,
	-372, -356, -340, -324, -308, -292, -276, -260,
	-244, -228, -212, -196, -180, -164, -148, -132,
	-120, -112, -104, -96, -88, -80, -72, -64,
	-56, -48, -40, -32, -24, -16, -8, 0,
	32124, 31100, 30076, 29052, 28028, 27004, 25980, 24956,
	23932, 22908, 21884, 20860, 19836, 18812, 17788, 16764,
	15996, 15484, 14972, 14460, 13948, 13436, 12924, 12412,
	11900, 11388, 10876, 10364, 9852, 9340, 8828, 8316,
	7932, 7676, 7420, 7164, 6908, 6652, 6396, 6140,
	5884, 5628, 5372, 5116, 4860, 4604, 4348, 4092,
	3900, 3772, 3644, 3516, 3388, 3260, 3132, 3004,
	2876, 2748, 2620, 2492, 2364, 2236, 2108, 1980,
	1884, 1820, 1756, 1692, 1628, 1564, 1500, 1436,
	1372, 1308, 1244, 1180, 1116, 1052, 988, 924,
	876, 844, 812, 780, 748, 716, 684, 652,
	620, 588, 556, 524, 492, 460, 428, 396,
	372, 356, 340, 324, 308, 292, 276, 260,
	244, 228, 212, 196, 180, 164, 148, 132,
	120, 112, 104, 96, 88, 80, 72, 64,
	56, 48, 40, 32, 24, 16, 8, 0,
}

// muLawCompressTable is a segment end lookup for μ-law encoding
var muLawSegmentTable = [8]int16{0xFF, 0x1FF, 0x3FF, 0x7FF, 0xFFF, 0x1FFF, 0x3FFF, 0x7FFF}

// MuLawDecode converts a single μ-law byte to a 16-bit signed PCM sample.
func MuLawDecode(mulaw byte) int16 {
	return muLawDecompressTable[mulaw]
}

// MuLawEncode converts a 16-bit signed PCM sample to μ-law.
func MuLawEncode(pcm int16) byte {
	// Determine sign and get magnitude
	sign := (pcm >> 8) & 0x80
	if sign != 0 {
		pcm = -pcm
	}
	if pcm > MuLawClip {
		pcm = MuLawClip
	}
	pcm = pcm + MuLawBias

	// Find segment
	segment := 7
	for i := 0; i < 8; i++ {
		if pcm <= muLawSegmentTable[i] {
			segment = i
			break
		}
	}

	// Combine sign, segment, and quantization
	return byte(^(sign | (int16(segment) << MuLawSegShift) | ((pcm >> (segment + 3)) & MuLawQuantMask)))
}

// MuLawDecodeBuf converts μ-law encoded bytes to 16-bit signed PCM.
// Output buffer must be 2x the size of input (2 bytes per sample).
func MuLawDecodeBuf(mulaw []byte, pcm []byte) {
	for i, b := range mulaw {
		sample := muLawDecompressTable[b]
		pcm[i*2] = byte(sample)
		pcm[i*2+1] = byte(sample >> 8)
	}
}

// MuLawEncodeBuf converts 16-bit signed PCM to μ-law encoded bytes.
// Output buffer must be half the size of input.
func MuLawEncodeBuf(pcm []byte, mulaw []byte) {
	numSamples := len(pcm) / 2
	for i := 0; i < numSamples; i++ {
		sample := int16(pcm[i*2]) | (int16(pcm[i*2+1]) << 8)
		mulaw[i] = MuLawEncode(sample)
	}
}

// MuLawToPCM converts μ-law encoded audio to 16-bit signed PCM.
// Returns a new slice containing the PCM data.
func MuLawToPCM(mulaw []byte) []byte {
	pcm := make([]byte, len(mulaw)*2)
	MuLawDecodeBuf(mulaw, pcm)
	return pcm
}

// PCMToMuLaw converts 16-bit signed PCM audio to μ-law.
// Returns a new slice containing the μ-law data.
func PCMToMuLaw(pcm []byte) []byte {
	mulaw := make([]byte, len(pcm)/2)
	MuLawEncodeBuf(pcm, mulaw)
	return mulaw
}
