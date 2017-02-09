package wormhole

import "testing"

func TestUnsupportedProtocol(t *testing.T) {
	equals(t, ParseTunnelProto("bla"), UNSUPPORTED)
}
