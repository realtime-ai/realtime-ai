package server

type ServerConfig struct {
	RTCUDPPort int
	// 你也可以在这里添加更多配置信息，比如 STUN/TURN server 列表、TLS 配置等
}
