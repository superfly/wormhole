package local

import (
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/oknoah/wormhole/config"
	"github.com/oknoah/wormhole/messages"
	"golang.org/x/crypto/ssh"

	"os"
)

var localTestServer *httptest.Server
var testRespBody string

const testSSHToken = "test_token"

var testSSHServerConfig *ssh.ServerConfig
var testSSHClientConfig *tls.Config

var testSSHRemoteListener *net.TCPListener

func init() {
	testRespBody = "test"

	localTestServer = httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, testBody)
		}))

	var err error

	tAddr, err := net.ResolveTCPAddr("tcp4", "127.0.0.1:8002")
	if err != nil {
		os.Exit(1)
	}

	testSSHRemoteListener, err = net.ListenTCP("tcp4", tAddr)
	if err != nil {
		os.Exit(1)
	}

	testSSHServerConfig = &ssh.ServerConfig{}
	pkey, err := ioutil.ReadFile("testdata/id_rsa")
	if err != nil {
		os.Exit(1)
	}

	if private, err := ssh.ParsePrivateKey(pkey); err == nil {
		testSSHServerConfig.AddHostKey(private)
	} else {
		os.Exit(1)
	}
	testSSHServerConfig.PasswordCallback = func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
		if string(pass) == testSSHToken {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to authenticate, got pass: '%s'", string(pass))
	}

}

func TestNewSSHHandler(t *testing.T) {
	testCfg := &config.ClientConfig{
		Config: config.Config{
			Logger:  logrus.New(),
			Version: "test_version",
		},
		Token:          testSSHToken,
		LocalEndpoint:  localTestServer.Listener.Addr().String(),
		RemoteEndpoint: testSSHRemoteListener.Addr().String(),
	}

	testRelease := &messages.Release{
		ID: "test_id",
	}

	expHandler, err := NewSSHHandler(testCfg, testRelease)
	assert.NoError(t, err, "Should be no error creating new handler")

	controlHandler := &SSHHandler{
		RemoteEndpoint: testSSHRemoteListener.Addr().String(),
		FlyToken:       testSSHToken,
		Version:        "test_version",
		LocalEndpoint:  localTestServer.Listener.Addr().String(),
	}

	assert.Equal(t, expHandler.RemoteEndpoint, controlHandler.RemoteEndpoint, "Remote endpoints should match")
	assert.Equal(t, expHandler.LocalEndpoint, controlHandler.LocalEndpoint, "Local endpoints should match")
	assert.Equal(t, expHandler.Version, controlHandler.Version, "Versions should match")
}

func newTestSSHHandler() (*SSHHandler, error) {
	testCfg := &config.ClientConfig{
		Config: config.Config{
			Logger:  logrus.New(),
			Version: "test_version",
		},
		Token:          testSSHToken,
		LocalEndpoint:  localTestServer.Listener.Addr().String(),
		RemoteEndpoint: testSSHRemoteListener.Addr().String(),
	}

	testRelease := &messages.Release{
		ID: "test_id",
	}

	return NewSSHHandler(testCfg, testRelease)
}

func newTestSSHServer(rawconn net.Conn, cfg *ssh.ServerConfig) *ssh.ServerConn {
	conn, chans, reqs, err := ssh.NewServerConn(rawconn, cfg)
	if err != nil {
		log.Fatal("failed to handshake: ", err)
		return nil
	}

	// The incoming Request channel must be serviced.
	go func() {
		for req := range reqs {
			switch req.Type {
			case "tcpip-forward":
				b := make([]byte, 4)
				// the local ssh handler expects to get the remote port number back
				binary.BigEndian.PutUint32(b, uint32(1024))
				req.Reply(true, b)
			}
		}
	}()

	// Service the incoming Channel channel.
	go func() {
		for newChannel := range chans {
			// Channels have a type, depending on the application level
			// protocol intended. In the case of a shell, the type is
			// "session" and ServerShell may be used to present a simple
			// terminal interface.
			if newChannel.ChannelType() != "session" {
				newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
				continue
			}
			_, _, err := newChannel.Accept()
			if err != nil {
				log.Fatalf("Could not accept channel: %v", err)
			}
		}
	}()
	return conn
}

func TestSSHHandler(t *testing.T) {
	h, err := newTestSSHHandler()
	assert.NoError(t, err, "Should be no error creating test handler")

	t.Run("Test_dial", func(t *testing.T) {
		sConnCh := make(chan *ssh.ServerConn)

		go func(ln *net.TCPListener) {
			conn, lnerr := ln.AcceptTCP()
			assert.NoError(t, lnerr, "Should be no error in accept")
			sshconn := newTestSSHServer(conn, testSSHServerConfig)
			sConnCh <- sshconn
		}(testSSHRemoteListener)

		_, _, err = h.dial()
		assert.NoError(t, err, "Should be no error getting connectio")

		_, ok := <-sConnCh
		assert.True(t, ok, "We should have no issue getting conn")
	})
	t.Run("Test_listen_and_serve", func(t *testing.T) {
		go func() {
			for {
				// We may have TCP errors to recover from
				_ = h.ListenAndServe()
			}
		}()
		conn, err := testSSHRemoteListener.AcceptTCP()
		assert.NoError(t, err, "Should have no error accepting control conn from handler")

		sshconn := newTestSSHServer(conn, testSSHServerConfig)
		assert.NotNil(t, sshconn, "Should have sshconn from initializing the SSH server")
	})
}
