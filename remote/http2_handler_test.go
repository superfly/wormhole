package remote

import (
	"crypto/tls"
	"crypto/x509"

	"fmt"
	"io"
	"net"
	_ "net/http"
	_ "net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/superfly/tlstest"
	"github.com/superfly/wormhole/config"
	"github.com/superfly/wormhole/messages"
	"github.com/superfly/wormhole/session"
	"golang.org/x/net/http2"
	"gopkg.in/ory-am/dockertest.v3"
)

var redisPool *redis.Pool
var serverTLSConfig *tls.Config
var clientTLSConfig *tls.Config

var serverTLSCert tls.Certificate
var serverCrtPEM []byte
var serverKeyPEM []byte

func TestMain(m *testing.M) {
	var rootCrtPEM []byte
	var err error
	rootCrtPEM, serverCrtPEM, serverKeyPEM, err = tlstest.CreateServerCertKeyPEMPairWithRootCert()
	if err != nil {
		log.Fatalf("tlstest could not generate x509 certs %v+", err)
	}

	serverTLSCert, err = tls.X509KeyPair(serverCrtPEM, serverKeyPEM)
	if err != nil {
		log.Fatalf("Couldn't create tls cert from keypair %v+", err)
	}

	serverTLSConfig = &tls.Config{
		Certificates: []tls.Certificate{serverTLSCert},
	}

	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(rootCrtPEM)

	clientTLSConfig = &tls.Config{
		RootCAs:    certPool,
		ServerName: "127.0.0.1",
	}

	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Dockertest could not connect to docker: %s", err)
	}

	redisResource, err := pool.Run("redis", "4.0.1", []string{})
	if err != nil {
		log.Fatalf("Could not create redis container")
	}

	if err := pool.Retry(func() error {
		var err error
		c, err := redis.DialURL(fmt.Sprintf("redis://localhost:%s", redisResource.GetPort("6379/tcp")))
		if err != nil {
			return err
		}
		_, err = c.Do("PING")
		return err
	}); err != nil {
		log.Fatalf("Could not connect to redis container: %s", err)
	}

	redisPool = newRedisPool(fmt.Sprintf("redis://localhost:%s", redisResource.GetPort("6379/tcp")))

	code := m.Run()

	if err := pool.Purge(redisResource); err != nil {
		log.Fatalf("Could not purge redis: %s", err)
	}

	os.Exit(code)
}

func newRedisPool(redisURL string) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			conn, err := redis.DialURL(redisURL)
			if err != nil {
				return nil, err
			}

			parsedURL, err := url.Parse(redisURL)
			if err != nil {
				return nil, err
			}
			if parsedURL.User != nil {
				if password, hasPassword := parsedURL.User.Password(); hasPassword == true {
					if _, authErr := conn.Do("AUTH", password); authErr != nil {
						conn.Close()
						return nil, authErr
					}
				}
			}
			return conn, nil
		},
		TestOnBorrow: func(conn redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := conn.Do("PING")
			return err
		},
	}
}

func TestNewHTTP2Handler(t *testing.T) {
	cfg := &config.ServerConfig{
		Config: config.Config{
			Logger:    log.New(),
			TLSCert:   serverCrtPEM,
			Localhost: "localhost",
		},
		TLSPrivateKey: serverKeyPEM,
		NodeID:        "1",
		ClusterURL:    "localhost",
	}

	h, err := NewHTTP2Handler(cfg, redisPool)
	assert.NoError(t, err, "Should be no error creating http2 handler")

	hControl := &HTTP2Handler{
		tlsConfig:  serverTLSConfig,
		logger:     cfg.Logger.WithFields(log.Fields{"prefix": "HTTP2Handler"}),
		pool:       redisPool,
		localhost:  "localhost",
		clusterURL: "localhost",
		sessions:   make(map[string]session.Session),
		nodeID:     "1",
	}

	assert.EqualValues(t, hControl, h, "Control and test HTTP2Handlers should match values")
}

func newTestHTTP2Handler() (*HTTP2Handler, error) {
	h := &HTTP2Handler{
		tlsConfig:  serverTLSConfig,
		logger:     log.New().WithFields(log.Fields{"prefix": "TestHTTP2Handler"}),
		pool:       redisPool,
		localhost:  "localhost",
		clusterURL: "localhost",
		sessions:   make(map[string]session.Session),
		nodeID:     "1",
	}

	return h, nil
}

func newServerClientTCPConns() (serverConn *net.TCPConn, clientConn *net.TCPConn, err error) {
	sConnCh := make(chan *net.TCPConn)
	lnAddr := &net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 8085,
	}

	ln, err := net.ListenTCP("tcp", lnAddr)
	if err != nil {
		log.Errorf("Error creating TCP listener: %+v", err)
		return
	}

	go func(ln *net.TCPListener) {
		s, err := ln.AcceptTCP()
		if err != nil {
			log.Errorf("Error accepting TCP listener: %+v", err)
			close(sConnCh)
		}
		if err := ln.Close(); err != nil {
			log.Errorf("Error closing listener: %+v", err)
			close(sConnCh)
		}
		sConnCh <- s
	}(ln)

	clientConn, err = net.DialTCP("tcp", nil, lnAddr)
	if err != nil {
		log.Errorf("Error dialing TCP conn")
		return
	}

	serverConn, ok := <-sConnCh
	if !ok {
		return nil, nil, errors.New("Error creating server conn")
	}

	return
}

func TestWrapsTLSOnServe(t *testing.T) {
	h, err := newTestHTTP2Handler()
	assert.NoError(t, err, "Should be no error creating new HTTP2Handler")
	assert.NotNil(t, h, "Handler shouldn't be nil")

	sConn, cConn, err := newServerClientTCPConns()
	assert.NoError(t, err, "Should be no error creating server/client TCP conns")
	assert.NotNil(t, sConn, "Server conn should not be nil")
	assert.NotNil(t, cConn, "Client conn should not be nil")

	go h.Serve(sConn)

	tlsClientConn := tls.Client(cConn, clientTLSConfig)

	err = tlsClientConn.Handshake()
	assert.NoError(t, err, "Should be no error completing tls handshake")
}

func wrapClientConn(cConn *net.TCPConn, tlsConf *tls.Config, alpn bool) (*tls.Conn, error) {

	tlsConf = tlsConf.Clone()
	// TODO: append here
	tlsConf.NextProtos = []string{http2.NextProtoTLS}
	tlsConf.MinVersion = tls.VersionTLS12

	var tlsClientConn *tls.Conn

	for {
		if err := cConn.SetDeadline(time.Now().Add(time.Second * 1)); err != nil {
			return nil, err
		}
		tlsClientConn = tls.Client(cConn, tlsConf)

		if err := tlsClientConn.Handshake(); err != nil {
			if netErr, ok := err.(net.Error); ok {
				if netErr.Timeout() {
					continue
				}
				if netErr.Temporary() {
					continue
				}
			}
			return nil, err
		}

		if err := cConn.SetDeadline(time.Time{}); err != nil {
			return nil, err
		}
		break
	}
	if alpn {
		if err := tlsClientConn.VerifyHostname(tlsConf.ServerName); err != nil {
			return &tls.Conn{}, err
		}
		state := tlsClientConn.ConnectionState()
		if p := state.NegotiatedProtocol; p != http2.NextProtoTLS {
			return &tls.Conn{}, fmt.Errorf("http2: unexpected ALPN protocol %q; want %q", p, http2.NextProtoTLS)
		}

		if !state.NegotiatedProtocolIsMutual {
			return &tls.Conn{}, fmt.Errorf("http2: could not negotiate protocol mutually")
		}
	}

	return tlsClientConn, nil
}

func TestCreatesFullSession(t *testing.T) {
	h, err := newTestHTTP2Handler()
	assert.NoError(t, err, "Should be no error creating new HTTP2Handler")
	assert.NotNil(t, h, "Handler shouldn't be nil")

	sControlConn, cControlConn, err := newServerClientTCPConns()
	assert.NoError(t, err, "Should be no error creating server/client TCP conns")
	assert.NotNil(t, sControlConn, "Server conn should not be nil")
	assert.NotNil(t, cControlConn, "Client conn should not be nil")

	go h.Serve(sControlConn)

	// Establish TLS connection
	tlsCControlConn, err := wrapClientConn(cControlConn, clientTLSConfig, false)
	assert.NoError(t, err, "Should have no error wrapping client in tls")

	authMessage := &messages.AuthControl{
		Token: "test",
	}

	authData, err := messages.Pack(authMessage)
	assert.NoError(t, err, "Should be no error packing message")

	_, err = tlsCControlConn.Write(authData)
	assert.NoError(t, err, "Should be no error writing data")

	tunData := make([]byte, 1024)
	nr, err := tlsCControlConn.Read(tunData)
	assert.NoError(t, err, "Should be no error reading tunnel message")

	msg, err := messages.Unpack(tunData[:nr])
	assert.NoError(t, err, "Should be no error unpacking message")

	tunMessage, ok := msg.(*messages.OpenTunnel)
	assert.True(t, ok, "Should be an opentunnel message")

	// establish new connections for tunnel socket
	sTunnelConn, cTunnelConn, err := newServerClientTCPConns()
	assert.NoError(t, err, "no error for conns")

	go h.Serve(sTunnelConn)

	tlsCTunnelConn, err := wrapClientConn(cTunnelConn, clientTLSConfig, false)
	assert.NoError(t, err, "Should be no error wrapping client")

	authTunMessage := &messages.AuthTunnel{
		ClientID: tunMessage.ClientID,
	}

	authTunData, err := messages.Pack(authTunMessage)
	assert.NoError(t, err, "Should be no error packing authTunMessage")

	_, err = tlsCTunnelConn.Write(authTunData)
	assert.NoError(t, err, "Should have no error writing to tunnel conn")

	err = tlsCTunnelConn.CloseWrite()
	assert.NoError(t, err, "Should have no error closing tunnel conn")

	for err == nil {
		_, err = tlsCTunnelConn.Read(tunData)
	}
	assert.Equal(t, err, io.EOF, "Read closing should establish EOF close-notify")

	alpnTunnelConn, err := wrapClientConn(cTunnelConn, clientTLSConfig, true)

	assert.NoError(t, err, "Should have no error establishing http2 conn tls")

	_ = alpnTunnelConn

	// TODO: Test throughput
	//	 This is dependent on registering backend IDs with token upon creation like the SSH handler currently does
}
