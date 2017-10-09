package local

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/superfly/wormhole/config"
	"github.com/superfly/wormhole/messages"
	wnet "github.com/superfly/wormhole/net"
)

const (
	pingInterval   = 2 * time.Second
	maxPongLatency = 5 * time.Second
)

// TCPHandler type represents the handler that opens a TCP conn to wormhole server and serves
// incoming requests
type TCPHandler struct {
	RemoteEndpoint string
	LocalEndpoint  string
	FlyToken       string
	Release        *messages.Release
	Version        string
	ln             net.Listener
	control        net.Conn
	conns          []net.Conn
	encrypted      bool
	tlsConfig      *tls.Config
	lastPongAt     int64
	logger         *logrus.Entry
}

// NewTCPHandler returns a TCPHandler struct
// WARNING: TCPHandler is insecure and shouldn't be used in production
func NewTCPHandler(cfg *config.ClientConfig, release *messages.Release) (*TCPHandler, error) {
	h := &TCPHandler{
		FlyToken:       cfg.Token,
		RemoteEndpoint: cfg.RemoteEndpoint,
		LocalEndpoint:  cfg.LocalEndpoint,
		Release:        release,
		Version:        cfg.Version,
		logger:         cfg.Logger.WithFields(logrus.Fields{"prefix": "TCPHandler"}),
	}
	if !cfg.Insecure {
		rootCAs := x509.NewCertPool()
		ok := rootCAs.AppendCertsFromPEM(cfg.TLSCert)
		if !ok {
			return nil, fmt.Errorf("couln't append a root CA: ")
		}
		h.tlsConfig = &tls.Config{RootCAs: rootCAs}
	}
	return h, nil
}

// ListenAndServe accepts requests coming from wormhole server
// and forwards them to the local server
func (s *TCPHandler) ListenAndServe() error {
	control, err := s.dial()
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
			conn, err := s.dial()
			if err != nil {
				return err
			}
			authMsg := &messages.AuthTunnel{ClientID: m.ClientID, Token: s.FlyToken}
			b, _ := messages.Pack(authMsg)
			_, err = conn.Write(b)
			if err != nil {
				return fmt.Errorf("Failed to auth tunnel: %s", err.Error())
			}

			s.logger.Infof("Established TCP Tunnel connection for Session: %s", m.ClientID)
			s.conns = append(s.conns, conn)
			go s.forwardConnection(conn, s.LocalEndpoint)
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

// Close closes the listener and TCP connection
func (s *TCPHandler) Close() error {
	err := s.control.Close()
	if err != nil {
		s.logger.Errorf("Control TCP conn close: %s", err)
	}
	for _, c := range s.conns {
		err = c.Close()
		if err != nil {
			s.logger.Errorf("Proxy TCP conn close: %s", err)
		}
	}
	return err
}

// connects to wormhole server
func (s *TCPHandler) dial() (conn net.Conn, err error) {
	// TCP into wormhole server

	if s.tlsConfig != nil {
		conn, err = tls.Dial("tcp", s.RemoteEndpoint, s.tlsConfig)
	} else {
		conn, err = net.Dial("tcp", s.RemoteEndpoint)
	}
	if err != nil {
		return nil, fmt.Errorf("Failed to establish TCP connection: %s", err.Error())
	}
	s.logger.Info("Established TCP connection.")

	return conn, nil
}

func (s *TCPHandler) heartbeat() {
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

func (s *TCPHandler) forwardConnection(tunnel net.Conn, local string) {
	s.logger.Debugf("Accepted TCP session on %s", tunnel.RemoteAddr())

	localConn, err := net.DialTimeout("tcp", local, localConnTimeout)
	if err != nil {
		s.logger.Errorf("Failed to reach local server: %s", err.Error())
	}

	s.logger.Debugf("Dialed local server on %s", local)

	_, _, err = wnet.CopyCloseIO(localConn, tunnel)
	if err != nil && err != io.EOF {
		s.logger.Error(err)
	}
}
