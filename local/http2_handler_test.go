package local

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/superfly/tlstest"
	"github.com/superfly/wormhole/config"
	"github.com/superfly/wormhole/messages"
	wnet "github.com/superfly/wormhole/net"
	"golang.org/x/net/http2"

	"os"
)

var httpTestServer *httptest.Server
var testBody string

var testTLSServerConfig *tls.Config
var testTLSClientConfig *tls.Config

var testTLSCACert []byte

var testRemoteListener *net.TCPListener

func init() {
	testBody = "test"

	httpTestServer = httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, testBody)
		}))

	var testTLSServerCertPEM []byte
	var testTLSServerKeyPEM []byte
	var err error

	testTLSCACert, testTLSServerCertPEM, testTLSServerKeyPEM, err = tlstest.CreateServerCertKeyPEMPairWithRootCert()
	if err != nil {
		os.Exit(1)
	}

	servTLSCert, err := tls.X509KeyPair(testTLSServerCertPEM, testTLSServerKeyPEM)
	if err != nil {
		os.Exit(1)
	}

	testTLSServerConfig = &tls.Config{
		Certificates: []tls.Certificate{servTLSCert},
	}

	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(testTLSCACert)

	testTLSClientConfig = &tls.Config{
		RootCAs:    certPool,
		ServerName: "127.0.0.1",
	}

	tAddr, err := net.ResolveTCPAddr("tcp4", "127.0.0.1:8001")
	if err != nil {
		os.Exit(1)
	}

	testRemoteListener, err = net.ListenTCP("tcp4", tAddr)
	if err != nil {
		os.Exit(1)
	}
}

func TestNewHTTP2Handler(t *testing.T) {
	testCfg := &config.ClientConfig{
		Config: config.Config{
			Logger:  logrus.New(),
			Version: "test_version",
			TLSCert: testTLSCACert,
		},
		Token:          "test_token",
		LocalEndpoint:  httpTestServer.Listener.Addr().String(),
		RemoteEndpoint: testRemoteListener.Addr().String(),
	}

	testRelease := &messages.Release{
		ID: "test_id",
	}

	expHandler, err := NewHTTP2Handler(testCfg, testRelease)
	assert.NoError(t, err, "Should be no error creating new handler")

	controlHandler := &HTTP2Handler{
		RemoteEndpoint: testRemoteListener.Addr().String(),
		FlyToken:       "test_token",
		Version:        "test_version",
		LocalEndpoint:  httpTestServer.Listener.Addr().String(),
		tlsConfig:      testTLSClientConfig,
	}

	assert.Equal(t, expHandler.RemoteEndpoint, controlHandler.RemoteEndpoint, "Remote endpoints should match")
	assert.Equal(t, expHandler.LocalEndpoint, controlHandler.LocalEndpoint, "Local endpoints should match")
	assert.Equal(t, expHandler.Version, controlHandler.Version, "Versions should match")
	assert.EqualValues(t, expHandler.tlsConfig, controlHandler.tlsConfig, "TLS Configs should match")
}

func newTestHTTP2Handler() (*HTTP2Handler, error) {
	testCfg := &config.ClientConfig{
		Config: config.Config{
			Logger:  logrus.New(),
			Version: "test_version",
			TLSCert: testTLSCACert,
		},
		Token:          "test_token",
		LocalEndpoint:  httpTestServer.Listener.Addr().String(),
		RemoteEndpoint: testRemoteListener.Addr().String(),
	}

	testRelease := &messages.Release{
		ID: "test_id",
	}

	return NewHTTP2Handler(testCfg, testRelease)
}

func TestHTTP2Handler(t *testing.T) {
	h, err := newTestHTTP2Handler()
	assert.NoError(t, err, "Should be no error creating test handler")

	t.Run("Test_dial", func(t *testing.T) {
		sConnCh := make(chan *net.TCPConn)

		go func(ln *net.TCPListener) {
			s, err := ln.AcceptTCP()
			assert.NoError(t, err, "Should be no error in accept")
			sConnCh <- s
		}(testRemoteListener)

		_, err = h.dial()
		assert.NoError(t, err, "Should be no error getting connectio")

		_, ok := <-sConnCh
		assert.True(t, ok, "We should have no issue getting conn")
	})
	t.Run("Test_dial_control", func(t *testing.T) {
		sConnCh := make(chan *tls.Conn)

		go func(ln *net.TCPListener) {
			s, err := ln.AcceptTCP()
			assert.NoError(t, err, "Should be no error in accept")
			sTLS, err := wnet.GenericTLSWrap(s, testTLSServerConfig, tls.Server)
			assert.NoError(t, err, "Should be no error wrapping tls conn")
			sConnCh <- sTLS
		}(testRemoteListener)

		_, err = h.dialControl()
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
		controlConn, err := testRemoteListener.AcceptTCP()
		assert.NoError(t, err, "Should have no error accepting control conn from handler")

		controlCTLS, err := wnet.GenericTLSWrap(controlConn, testTLSServerConfig, tls.Server)
		assert.NoError(t, err, "Should have no error wrapping control conn with TLS")

		buf := make([]byte, 1024)

		nr, err := controlCTLS.Read(buf)
		assert.NoError(t, err, "Should be no error reading from handler conn")

		msg, err := messages.Unpack(buf[:nr])
		assert.NoError(t, err, "Should be no error unpacking msg")

		authMsg, ok := msg.(*messages.AuthControl)
		assert.True(t, ok, "Should be an AuthControl message")

		assert.Equal(t, authMsg.Token, h.FlyToken)

		t.Run("Test_open_tunnel", func(t *testing.T) {
			oMsg := &messages.OpenTunnel{
				ClientID: "test",
			}

			buf, err := messages.Pack(oMsg)
			assert.NoError(t, err, "Should have no error packing messages")

			_, err = controlCTLS.Write(buf)
			assert.NoError(t, err, "Should have no error writing message")

			tunConn, err := testRemoteListener.AcceptTCP()
			assert.NoError(t, err, "Should have no error accepting tunnel")

			tunTLSConn, err := wnet.GenericTLSWrap(tunConn, testTLSServerConfig, tls.Server)
			assert.NoError(t, err, "Should have no error wrapping tunnel")

			buf = make([]byte, 1024)

			nr, err := tunTLSConn.Read(buf)
			assert.True(t, err == nil || err == io.EOF, "Should have no error reading from tunnel conn")

			for err != io.EOF {
				_, err = tunTLSConn.Read(buf)
			}

			msg, err := messages.Unpack(buf[:nr])
			assert.NoError(t, err, "Should have no error unpacking message")

			authTunMsg, ok := msg.(*messages.AuthTunnel)
			assert.True(t, ok, "Should be of type authtunnel")

			assert.Equal(t, authTunMsg.ClientID, oMsg.ClientID, "Should have same clientID as openTunnel")
			assert.Equal(t, authTunMsg.Token, h.FlyToken, "Should have matching tokens")

			err = tunTLSConn.CloseWrite()
			assert.NoError(t, err, "Should be no error closing conn")

			alpnConn, err := wnet.HTTP2ALPNTLSWrap(tunConn, testTLSServerConfig, tls.Server)
			assert.NoError(t, err, "Should be no error wrapping alpnConn")

			tr := &http2.Transport{}
			http2Client, err := tr.NewClientConn(alpnConn)
			assert.NoError(t, err, "Should be no error creating new client conn")

			req, err := http.NewRequest("GET", "https://127.0.0.1:8000", nil)
			assert.NoError(t, err, "Should have no error making request")

			resp, err := http2Client.RoundTrip(req)
			assert.NoError(t, err, "Should have no error sending request")

			body, err := ioutil.ReadAll(resp.Body)
			assert.NoError(t, err, "Should have no error reading body")

			assert.Equal(t, testBody, string(body))
		})
	})
}
