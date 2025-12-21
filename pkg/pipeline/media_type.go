package pipeline

// AudioMediaType represents the media type for audio data
type AudioMediaType string

const (
	// Raw PCM audio (default)
	AudioMediaTypeRaw AudioMediaType = "audio/x-raw"
	// Opus encoded audio
	AudioMediaTypeOpus AudioMediaType = "audio/x-opus"
	// PCM audio format
	AudioMediaTypePCM AudioMediaType = "audio/pcm"
	// MPEG audio format
	AudioMediaTypeMPEG AudioMediaType = "audio/mpeg"
	// WAV audio format
	AudioMediaTypeWAV AudioMediaType = "audio/wav"
	// Speech audio format
	AudioMediaTypeSpeech AudioMediaType = "audio/speech"
	// Opus with RFC header
	AudioMediaTypeOpusStandard AudioMediaType = "audio/opus"
)

// String returns the string representation of AudioMediaType
func (amt AudioMediaType) String() string {
	return string(amt)
}

// VideoMediaType represents the media type for video data
type VideoMediaType string

const (
	// Raw video format
	VideoMediaTypeRaw VideoMediaType = "video/x-raw"
	// VP8 encoded video
	VideoMediaTypeVP8 VideoMediaType = "video/vp8"
)

// String returns the string representation of VideoMediaType
func (vmt VideoMediaType) String() string {
	return string(vmt)
}
