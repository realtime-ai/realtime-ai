package trace

import (
	"go.opentelemetry.io/otel/attribute"
)

// Common attribute keys used throughout the application
const (
	// Pipeline attributes
	AttrPipelineName     = "pipeline.name"
	AttrPipelineElement  = "pipeline.element"
	AttrSessionID        = "session.id"
	AttrMessageType      = "message.type"

	// Audio attributes
	AttrAudioSampleRate  = "audio.sample_rate"
	AttrAudioChannels    = "audio.channels"
	AttrAudioMediaType   = "audio.media_type"
	AttrAudioCodec       = "audio.codec"
	AttrAudioDataSize    = "audio.data_size"

	// Video attributes
	AttrVideoWidth       = "video.width"
	AttrVideoHeight      = "video.height"
	AttrVideoMediaType   = "video.media_type"
	AttrVideoCodec       = "video.codec"
	AttrVideoDataSize    = "video.data_size"

	// Connection attributes
	AttrConnectionID     = "connection.id"
	AttrConnectionType   = "connection.type"
	AttrConnectionState  = "connection.state"

	// AI/LLM attributes
	AttrLLMProvider      = "llm.provider"
	AttrLLMModel         = "llm.model"
	AttrLLMResponseType  = "llm.response_type"

	// STT/TTS attributes
	AttrSTTProvider      = "stt.provider"
	AttrTTSProvider      = "tts.provider"
	AttrTTSVoice         = "tts.voice"

	// Error attributes
	AttrErrorType        = "error.type"
	AttrErrorMessage     = "error.message"
)

// Helper functions to create common attributes

// PipelineAttrs creates attributes for pipeline operations
func PipelineAttrs(pipelineName, elementName string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(AttrPipelineName, pipelineName),
		attribute.String(AttrPipelineElement, elementName),
	}
}

// SessionAttrs creates attributes for session information
func SessionAttrs(sessionID string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(AttrSessionID, sessionID),
	}
}

// AudioAttrs creates attributes for audio data
func AudioAttrs(sampleRate, channels, dataSize int, mediaType, codec string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.Int(AttrAudioSampleRate, sampleRate),
		attribute.Int(AttrAudioChannels, channels),
		attribute.Int(AttrAudioDataSize, dataSize),
		attribute.String(AttrAudioMediaType, mediaType),
		attribute.String(AttrAudioCodec, codec),
	}
}

// VideoAttrs creates attributes for video data
func VideoAttrs(width, height, dataSize int, mediaType, codec string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.Int(AttrVideoWidth, width),
		attribute.Int(AttrVideoHeight, height),
		attribute.Int(AttrVideoDataSize, dataSize),
		attribute.String(AttrVideoMediaType, mediaType),
		attribute.String(AttrVideoCodec, codec),
	}
}

// ConnectionAttrs creates attributes for connection information
func ConnectionAttrs(connID, connType, state string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(AttrConnectionID, connID),
		attribute.String(AttrConnectionType, connType),
		attribute.String(AttrConnectionState, state),
	}
}

// LLMAttrs creates attributes for LLM operations
func LLMAttrs(provider, model string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(AttrLLMProvider, provider),
		attribute.String(AttrLLMModel, model),
	}
}

// ErrorAttrs creates attributes for errors
func ErrorAttrs(errType, errMsg string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(AttrErrorType, errType),
		attribute.String(AttrErrorMessage, errMsg),
	}
}
