package local

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/superfly/wormhole/config"
	"github.com/superfly/wormhole/messages"
	wnet "github.com/superfly/wormhole/net"
	"golang.org/x/net/http2"
)

// HTTP2Handler type represents the handler that opens a TCP conn to wormhole server and serves
// incoming requests
type HTTP2Handler struct {
	RemoteEndpoint string
	LocalEndpoint  string
	FlyToken       string
	Release        *messages.Release
	Version        string
	ln             net.Listener
	control        net.Conn
	conns          []net.Conn
	server         *http2.Server
	transport      *http2.Transport
	fClient        *http.Client
	tlsConfig      *tls.Config
	lastPongAt     int64
	logger         *logrus.Entry
}

// NewHTTP2Handler returns a HTTP2Handler struct with TLS encryption
func NewHTTP2Handler(cfg *config.ClientConfig, release *messages.Release) (*HTTP2Handler, error) {
	rootCAs := x509.NewCertPool()
	ok := rootCAs.AppendCertsFromPEM(cfg.TLSCert)
	if !ok {
		return nil, fmt.Errorf("couln't append a root CA")
	}

	tlsHost, _, err := net.SplitHostPort(cfg.RemoteEndpoint)
	if err != nil {
		return nil, err
	}

	h := &HTTP2Handler{
		FlyToken:       cfg.Token,
		RemoteEndpoint: cfg.RemoteEndpoint,
		LocalEndpoint:  cfg.LocalEndpoint,
		Release:        release,
		Version:        cfg.Version,
		tlsConfig:      &tls.Config{RootCAs: rootCAs, ServerName: tlsHost},
		server:         &http2.Server{},
		transport:      &http2.Transport{},
		fClient:        &http.Client{},
		logger:         cfg.Logger.WithFields(logrus.Fields{"prefix": "HTTP2Handler"}),
	}

	return h, nil
}

// ListenAndServe accepts requests coming from wormhole server
// and forwards them to the local server
func (s *HTTP2Handler) ListenAndServe() error {
	control, err := s.dialControl()
	if err != nil {
		return err
	}
	defer control.Close()

	s.control = control
	ctlAuthMsg := &messages.AuthControl{
		Token: s.FlyToken,
	}
	buf, err := messages.Pack(ctlAuthMsg)
	if err != nil {
		return fmt.Errorf("error packing message to control: " + err.Error())
	}

	_, err = s.control.Write(buf)
	if err != nil {
		return fmt.Errorf("error writing to control: " + err.Error())
	}

	s.lastPongAt = time.Now().UnixNano()
	go s.heartbeat()

	b := make([]byte, 1024)
	for {
		nr, err := s.control.Read(b)
		if err == io.EOF {
			continue
		}
		if err != nil {
			return fmt.Errorf("error reading from control: " + err.Error())
		}
		msg, err := messages.Unpack(b[:nr])
		if err != nil {
			return fmt.Errorf("error parsing message from stream: " + err.Error())
		}
		switch m := msg.(type) {
		case *messages.OpenTunnel:
			s.logger.Debug("Received Open Tunnel message.")
			tcpConn, err := s.dial()
			if err != nil {
				return err
			}
			genericTLSConn, err := s.genericTLSWrap(tcpConn)
			if err != nil {
				return err
			}
			authMsg := &messages.AuthTunnel{ClientID: m.ClientID, Token: s.FlyToken}
			b, _ := messages.Pack(authMsg)
			_, err = genericTLSConn.Write(b)
			if err != nil {
				return fmt.Errorf("Failed to auth tunnel: %s", err.Error())
			}
			if err := genericTLSConn.CloseWrite(); err != nil {
				return fmt.Errorf("Failed to close tls: %s", err.Error())
			}

			for err == nil {
				_, err = genericTLSConn.Read(b)
			}
			if err != io.EOF {
				return fmt.Errorf("Failed to close tls: %s", err.Error())
			}

			// TODO: Listen for auth ACK

			http2TLSConn, err := s.http2ALPNTLSWrap(tcpConn)
			if err != nil {
				return err
			}

			s.logger.Infof("Established TLS connection for Session: %s", m.ClientID)
			s.conns = append(s.conns, http2TLSConn)

			opts := &http2.ServeConnOpts{
				// We are our own handler
				Handler: s,
			}
			s.logger.Info("Serving http2 Connection")
			go s.server.ServeConn(http2TLSConn, opts)
		case *messages.Shutdown:
			s.logger.Debugf("Received Shutdown message: %s", m.Error)
			return s.Close()
		case *messages.Pong:
			atomic.StoreInt64(&s.lastPongAt, time.Now().UnixNano())
		default:
			s.logger.Warn("Unrecognized command. Ignoring.")
		}
	}
}

func (s *HTTP2Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.URL.Host = s.LocalEndpoint
	// TODO: Figure out support for https
	r.URL.Scheme = "http"
	r.Host = s.LocalEndpoint
	r.RequestURI = ""

	// We ignore the error ONLY because it will be forwarded
	// to the end user
	// TODO: Handle this error
	resp, err := s.fClient.Do(r)
	if err != nil {
		s.logger.Error(err)
	}

	// Delete this so we don't copy it over
	// Will be handled by http.ResponseWriter
	resp.Header.Del("Content-Length")

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)

	defer resp.Body.Close()

	nr, err := io.Copy(w, resp.Body)
	if err != nil {
		s.logger.Errorf("Could not copy response body")
		return
	}
	s.logger.Infof("Copied %d bytes between connection bodies", nr)
}

// Close closes the listener and TCP connection
func (s *HTTP2Handler) Close() error {
	err := s.control.Close()
	if err != nil {
		s.logger.Errorf("Control TCP conn close: %s", err)
	}
	for _, c := range s.conns {
		err = c.Close()
		if err != nil {
			s.logger.Errorf("Proxy http2 conn close: %s", err)
		}
	}
	return err
}

// dial opens an unencrypted TCP connection to a server
func (s *HTTP2Handler) dial() (*net.TCPConn, error) {
	conn, err := net.Dial("tcp", s.RemoteEndpoint)
	if err != nil {
		return nil, err
	}
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil, errors.New("Error: could not cast tcp connection")
	}

	return tcpConn, nil
}

func (s *HTTP2Handler) dialControl() (net.Conn, error) {
	conn, err := s.dial()
	if err != nil {
		return nil, err
	}

	cConn, err := s.genericTLSWrap(conn)
	if err != nil {
		return nil, err
	}

	return cConn, nil
}

func (s *HTTP2Handler) genericTLSWrap(conn *net.TCPConn) (*tls.Conn, error) {
	return wnet.GenericTLSWrap(conn, s.tlsConfig, tls.Client)
}

// This wrapper fulfills the requirement for specifying the 'h2' ALPN TLS negotiation for
// TLS enabled http2 connections
//
// NOTE: The ALPN is a requirement of the spec for HTTP/2 capability discovery
// While technically the golang implementation will allow us not to perform ALPN,
// this breaks the http/2 spec. The goal here is to follow the RFC to the letter
// as documented in http://httpwg.org/specs/rfc7540.html#starting
func (s *HTTP2Handler) http2ALPNTLSWrap(conn *net.TCPConn) (*tls.Conn, error) {
	return wnet.HTTP2ALPNTLSWrap(conn, s.tlsConfig, tls.Client)
}

func (s *HTTP2Handler) heartbeat() {
	// set lastPing to something sane
	lastPing := time.Unix(atomic.LoadInt64(&s.lastPongAt)-1, 0)
	ping := time.NewTicker(pingInterval)
	pongCheck := time.NewTicker(time.Second)

	defer func() {
		s.control.Close()
		ping.Stop()
		pongCheck.Stop()
	}()

	for {
		select {
		case <-pongCheck.C:
			lastPong := time.Unix(0, atomic.LoadInt64(&s.lastPongAt))
			needPong := lastPong.Sub(lastPing) < 0
			pongLatency := time.Since(lastPing)

			if needPong && pongLatency > maxPongLatency {
				s.logger.Infof("Last ping: %v, Last pong: %v", lastPing, lastPong)
				s.logger.Infof("Connection stale, haven't gotten PongMsg in %d seconds", int(pongLatency.Seconds()))
				return
			}

		case <-ping.C:
			b, err := messages.Pack(&messages.Ping{})
			if err != nil {
				s.logger.Errorf("Got error %v when creating PingMsg", err)
				return
			}
			_, err = s.control.Write(b)
			if err != nil {
				s.logger.Errorf("Got error %v when writing PingMsg", err)
				return
			}
			s.logger.Debug("Sent Ping message")
			lastPing = time.Now()
		}
	}
}
