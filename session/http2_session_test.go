package session

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"testing"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	_ "github.com/superfly/wormhole/config"
	"github.com/superfly/wormhole/messages"
	wnet "github.com/superfly/wormhole/net"
	"golang.org/x/net/http2"
)

func newServerClientTLSConns(alpn bool) (serverTLSConn *tls.Conn, clientTLSConn *tls.Conn, err error) {
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

	cConn, err := net.DialTCP("tcp", nil, lnAddr)
	if err != nil {
		log.Errorf("Error dialing TCP conn")
		return
	}

	sConn, ok := <-sConnCh
	if !ok {
		return nil, nil, errors.New("Error creating server conn")
	}

	sTLSConnCh := make(chan *tls.Conn)

	var wrapFunc func(*net.TCPConn, *tls.Config, wnet.TLSWrapperFunc) (*tls.Conn, error)
	if alpn {
		wrapFunc = wnet.HTTP2ALPNTLSWrap
	} else {
		wrapFunc = wnet.GenericTLSWrap
	}

	go func(sConn *net.TCPConn) {
		sTLSConn, err := wrapFunc(sConn, serverTLSConfig, tls.Server)
		if err != nil {
			log.Errorf("Error creating tls wrap server")
			close(sConnCh)
		}
		sTLSConnCh <- sTLSConn
	}(sConn)

	clientTLSConn, err = wrapFunc(cConn, clientTLSConfig, tls.Client)
	if err != nil {
		return nil, nil, err
	}

	serverTLSConn, ok = <-sTLSConnCh
	if !ok {
		return nil, nil, errors.New("Error creating server tls wrap")
	}

	return
}

func TestHTTP2Session(t *testing.T) {
	sConn, cConn, err := newServerClientTLSConns(false)
	assert.NoError(t, err, "Should be no error creating conns")

	args := &HTTP2SessionArgs{
		Logger:    log.New(),
		NodeID:    "test_id",
		TLSConfig: serverTLSConfig,
		RedisPool: redisPool,
		Conn:      sConn,
	}

	s, err := NewHTTP2Session(args)
	assert.NoError(t, err, "Should be no error creating http2 session")

	t.Run("Test_open_tunnel", func(t *testing.T) {
		err = s.openTunnel()
		assert.NoError(t, err, "Should be no error opening tunnel")

		b := make([]byte, 1024)
		nr, err := cConn.Read(b)
		assert.NoError(t, err, "Should be no error reading data")

		msg, err := messages.Unpack(b[:nr])
		assert.NoError(t, err, "Should be no error unpacking")

		oTunMsg, ok := msg.(*messages.OpenTunnel)
		assert.True(t, ok, "Should be an opentunnel message")

		assert.Equal(t, oTunMsg.ClientID, s.id)
	})

	t.Run("Test_round_trip", func(t *testing.T) {
		sHTTPConn, cHTTPConn, err := newServerClientTLSConns(true)
		assert.NoError(t, err, "Should be no error getting new conns")

		http2Server := &http2.Server{}
		http2ServerConnOpts := &http2.ServeConnOpts{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, "test")
			}),
		}

		go func() {
			http2Server.ServeConn(cHTTPConn, http2ServerConnOpts)
			assert.False(t, true, "Should never stop serving conn during this test")
		}()

		err = s.AddTunnel(sHTTPConn)
		assert.NoError(t, err, "Should be no error adding tunnel")

		ln, err := net.Listen("tcp4", ":0")
		assert.NoError(t, err, "Should have no error listening")

		go func() {
			s.handleRemoteForward(ln)
			assert.False(t, true, "Should never stop handling forward during this test")
		}()

		resp, err := http.Get(fmt.Sprintf("http://%s", ln.Addr().String()))
		assert.NoError(t, err, "Should not have error requesting")

		body, err := ioutil.ReadAll(resp.Body)
		assert.NoError(t, err, "Should have no error parsing body")
		assert.Equal(t, "test", string(body), "Should have matching request body")

	})
}
