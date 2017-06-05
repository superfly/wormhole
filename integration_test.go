// +build integration

package wormhole_test

import (
	"io/ioutil"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/alicebob/miniredis"
	"github.com/superfly/wormhole"
	"github.com/superfly/wormhole/config"

	"github.com/garyburd/redigo/redis"
)

var fakeRedis *miniredis.Miniredis
var redisURL string

func testServerConfig(t *testing.T, redisURL string) *config.ServerConfig {
	c, err := redis.Dial("tcp", redisURL)
	if err != nil {
		t.Fatal(err)
	}

	// make sure that token is registered in Redis for this backend ID
	c.Do("HSET", "backend_tokens", "fly_token", "1")

	sshKey, err := ioutil.ReadFile("testdata/id_rsa")
	if err != nil {
		t.Fatal(err)
	}
	sc := &config.ServerConfig{
		RedisURL:      "redis://" + redisURL,
		SSHPrivateKey: sshKey,
	}
	logger := logrus.New()
	sc.Logger = logger
	sc.Port = "13579"

	return sc
}

func testClientConfig(t *testing.T) *config.ClientConfig {
	cc := &config.ClientConfig{
		Token:          "fly_token",
		RemoteEndpoint: "127.0.0.1:13579",
	}
	logger := logrus.New()
	cc.Logger = logger
	return cc
}

func testServer(redisURL string, t *testing.T) {
	wormhole.StartRemote(testServerConfig(t, redisURL))
}

func testClient(t *testing.T) {
	wormhole.StartLocal(testClientConfig(t))
}

func TestClientConnect(t *testing.T) {
	fakeRedis, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}

	redisURL := fakeRedis.Addr()
	go testServer(redisURL, t)
	go testClient(t)

	// wait for SSH handshake
	time.Sleep(1 * time.Second)

	c, err := redis.Dial("tcp", redisURL)
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.Do("SMEMBERS", "backend:1:sessions")
	if err != nil {
		t.Fatalf("Expected backend to have started a new session: %s", err.Error())
	}
}
