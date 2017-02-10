package wormhole

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestParseTunnelProto(t *testing.T) {
	equals(t, ParseTunnelProto("bla"), UNSUPPORTED)
	equals(t, ParseTunnelProto("ssh"), SSH)
	equals(t, ParseTunnelProto("tls"), TLS)
	equals(t, ParseTunnelProto("tcp"), TCP)
}

func TestDefaultServerConfig(t *testing.T) {
	os.Setenv("FLY_LOCALHOST", "localhost")
	os.Setenv("FLY_CLUSTER_URL", "127.0.0.1")
	os.Setenv("FLY_REDIS_URL", "redis://localhost:6379")
	os.Setenv("FLY_SSH_PRIVATE_KEY_FILE", "testdata/id_rsa")
	defer func() {
		os.Unsetenv("FLY_LOCALHOST")
		os.Unsetenv("FLY_CLUSTER_URL")
		os.Unsetenv("FLY_REDIS_URL")
		os.Unsetenv("FLY_SSH_PRIVATE_KEY_FILE")
	}()

	cfg, err := NewServerConfig()

	ok(t, err)
	equals(t, cfg.Protocol, SSH)
	equals(t, cfg.Port, "10000")
	equals(t, cfg.Localhost, "localhost")
	equals(t, cfg.ClusterURL, "127.0.0.1")
	equals(t, cfg.RedisURL, "redis://localhost:6379")
	equals(t, cfg.LogLevel, "info")

	bytes, err := ioutil.ReadFile("testdata/id_rsa")
	if err != nil {
		t.Fatal(err)
	}
	equals(t, cfg.SSHPrivateKey, bytes)

	nodeID, _ := os.Hostname()
	equals(t, cfg.NodeID, nodeID)
}

func TestDefaultClientConfig(t *testing.T) {
	os.Setenv("FLY_TOKEN", "bla")
	defer func() {
		os.Unsetenv("FLY_TOKEN")
	}()

	cfg, err := NewClientConfig()

	ok(t, err)
	equals(t, cfg.Protocol, SSH)
	equals(t, cfg.Port, "5000")
	equals(t, cfg.Localhost, "127.0.0.1")
	equals(t, cfg.LogLevel, "info")
	equals(t, cfg.LocalEndpoint, "127.0.0.1:5000")
	equals(t, cfg.RemoteEndpoint, "wormhole.fly.io:30000")
	equals(t, cfg.Token, "bla")
}
