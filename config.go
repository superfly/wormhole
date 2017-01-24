package wormhole

// Config stores wormole server and client parameters
type Config struct {
	Protocol TunnelProto
	Version  string
}

// TunnelProto specifies the type of transport protocol used by wormhole instance
type TunnelProto int

const (
	// SSH tunnel with remote port forwarding
	SSH TunnelProto = iota
	// TCP connection pool
	TCP
	_
	_
	_
	UNSUPPORTED
)

func ParseTunnelProto(proto string) TunnelProto {
	switch proto {
	case "ssh":
		return SSH
	case "tcp":
		return TCP
	default:
		return UNSUPPORTED
	}
}
