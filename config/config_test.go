package config

import (
	"io/ioutil"
	"os"
	"testing"

	. "github.com/oknoah/wormhole/testing"
)

func TestParseTunnelProto(t *testing.T) {
	Equals(t, ParseTunnelProto("bla"), UNSUPPORTED)
	Equals(t, ParseTunnelProto("ssh"), SSH)
	Equals(t, ParseTunnelProto("tcp"), TCP)
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

	Ok(t, err)
	Equals(t, cfg.Protocol, SSH)
	Equals(t, cfg.Port, "10000")
	Equals(t, cfg.Localhost, "localhost")
	Equals(t, cfg.ClusterURL, "127.0.0.1")
	// not with --link baby
	Equals(t, cfg.RedisURL, "redis://localhost:6379")
	Equals(t, cfg.LogLevel, "info")

	bytes, err := ioutil.ReadFile("testdata/id_rsa")
	if err != nil {
		t.Fatal(err)
	}
	Equals(t, cfg.SSHPrivateKey, bytes)

	nodeID, _ := os.Hostname()
	Equals(t, cfg.NodeID, nodeID)
}

func TestDefaultClientConfig(t *testing.T) {
	os.Setenv("FLY_TOKEN", "bla")
	defer func() {
		os.Unsetenv("FLY_TOKEN")
	}()

	cfg, err := NewClientConfig()

	Ok(t, err)
	Equals(t, cfg.Protocol, SSH)
	Equals(t, cfg.Port, "5000")
	Equals(t, cfg.Localhost, "127.0.0.1")
	Equals(t, cfg.LogLevel, "info")
	Equals(t, cfg.LocalEndpoint, "127.0.0.1:5000")
	Equals(t, cfg.RemoteEndpoint, "wormhole.fly.io:30000")
	Equals(t, cfg.Token, "bla")
}
