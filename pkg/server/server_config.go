package server

type ServerConfig struct {
	// UDP 端口
	RTCUDPPort int

	// ICE 是否开启 lite 模式, 默认是 false
	ICELite bool

	// Candidate 地址, 默认是 []string{"0.0.0.0"}
	Endpoint []string

	// 你也可以在这里添加更多配置信息，比如 STUN/TURN server 列表、TLS 配置等
}
