package server

// BasicWebRTCConfig holds configuration for BasicWebRTCServer.
// This is a simple WebRTC server without Realtime API protocol support.
type BasicWebRTCConfig struct {
	// RTCUDPPort is the UDP port for WebRTC
	RTCUDPPort int

	// ICELite enables ICE lite mode (default: false)
	ICELite bool

	// Endpoint is the list of candidate addresses (default: []string{"0.0.0.0"})
	Endpoint []string
}

// Deprecated: ServerConfig is deprecated. Use BasicWebRTCConfig instead.
type ServerConfig = BasicWebRTCConfig
