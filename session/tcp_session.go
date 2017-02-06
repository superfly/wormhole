package session

import (
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/rs/xid"
	"github.com/superfly/wormhole/messages"
	"github.com/superfly/wormhole/utils"
)

const (
	connCheckInterval     = 1 * time.Second
	pingTimeoutInterval   = 10 * time.Second
	tunnelTimeoutInterval = 10 * time.Second
)

type TCPSession struct {
	baseSession

	control    net.Conn
	conns      chan net.Conn
	lastPingAt int64
	logger     *logrus.Entry
}

// NewTCPSession creates new TCPSession struct
func NewTCPSession(logger *logrus.Logger, nodeID string, redisPool *redis.Pool, conn net.Conn) *TCPSession {
	base := baseSession{
		id:     xid.New().String(),
		nodeID: nodeID,
		store:  NewRedisStore(redisPool),
	}
	s := &TCPSession{
		control:     conn,
		baseSession: base,
		conns:       make(chan net.Conn, 10),
		lastPingAt:  time.Now().UnixNano(),
		logger:      logger.WithFields(logrus.Fields{"prefix": "TCPSession"}),
	}
	return s
}

func (s *TCPSession) AddTunnel(conn net.Conn) {
	select {
	case s.conns <- conn:
		s.logger.Info("Added Tunnel")
	default:
		s.logger.Info("Tunnels buffer is full, discarding.")
		conn.Close()
	}
}

func (s *TCPSession) GetTunnel() (conn net.Conn, err error) {
	var ok bool

	// get a tunnel connection from the pool
	select {
	case conn, ok = <-s.conns:
		if !ok {
			err = fmt.Errorf("No tunnels available, control is closing")
			return
		}
	default:
		// no tunnels available in the pool, ask for one over the control channel
		s.logger.Debug("No tunnels in pool, requesting tunnel from control . . .")

		err = s.openTunnel()
		if err != nil {
			return
		}

		select {
		case conn, ok = <-s.conns:
			if !ok {
				err = fmt.Errorf("No tunnel connections available, control is closing")
				return
			}

		case <-time.After(tunnelTimeoutInterval):
			err = fmt.Errorf("Timeout trying to get tunnel connection")
			return
		}
	}
	return
}

func (s *TCPSession) RequireStream() error {
	return s.openTunnel()
}

func (s *TCPSession) HandleRequests(ln *net.TCPListener) {
	go s.controlLoop()
	go s.heartbeat()
	s.handleRemoteForward(ln)
}

func (s *TCPSession) RequireAuthentication() error {
	// we can skip this right now, eventually needs conn to be authed
	go s.RegisterConnection(time.Now())
	return nil
}

func (s *TCPSession) Close() {
	s.RegisterDisconnection()
	s.logger.Infof("Closed session %s for %s (%s).", s.ID(), s.NodeID(), s.Client())
	s.control.Close()
}

func (s *TCPSession) handleRemoteForward(ln *net.TCPListener) {
	defer func() {
		err := ln.Close()
		if err != nil {
			s.logger.Debugf("Couldn't close ingress conn: %s", err)
			return
		}
		s.logger.Debugf("Closed ingress conn: %s", ln.Addr().String())
	}()

	for {
		ln.SetDeadline(time.Now().Add(time.Second))
		tcpConn, err := ln.AcceptTCP()

		if err != nil {
			netErr, ok := err.(net.Error)

			//If this is a timeout, then continue to wait for
			//new connections
			if ok && netErr.Timeout() && netErr.Temporary() {
				continue
			}
			s.logger.Errorln("Could not accept Ingress TCP conn:", err)
			return
		}
		s.logger.Debugln("Accepted Ingress TCP conn from:", tcpConn.RemoteAddr())

		tunnel, err := s.GetTunnel()
		if err != nil {
			s.logger.Errorf("Could not get a tunnel conn: %s", err.Error())
			return
		}

		// request a new tunnel
		go func() {
			if err = s.openTunnel(); err != nil {
				s.logger.Error(err)
			}
		}()

		err = utils.CopyCloseIO(tunnel, tcpConn)
		if err != nil && err != io.EOF {
			s.logger.Error(err)
		}
	}
}

func (s *TCPSession) openTunnel() error {
	msg := &messages.OpenTunnel{ClientID: s.id}
	b, err := messages.Pack(msg)
	if err != nil {
		return fmt.Errorf("Couldn't create a request to open new tunnel: %s", err.Error())
	}
	_, err = s.control.Write(b)
	if err != nil {
		return fmt.Errorf("Failed to send request to open new tunnel: %s", err.Error())
	}
	return nil
}

func (s *TCPSession) heartbeat() {
	// timer for detecting heartbeat failure
	connCheck := time.NewTicker(connCheckInterval)
	defer connCheck.Stop()

	for {
		select {
		case <-connCheck.C:
			lastPing := time.Unix(0, atomic.LoadInt64(&s.lastPingAt))
			if time.Since(lastPing) > pingTimeoutInterval {
				s.logger.Info("Lost heartbeat")
				s.Close()
				return
			}
		}
	}
}

func (s *TCPSession) controlLoop() {
	b := make([]byte, 1024)

	for {
		nr, err := s.control.Read(b)
		if err == io.EOF {
			continue
		}
		if err != nil {
			s.logger.Errorf("error reading from control: " + err.Error())
			s.Close()
			return
		}
		msg, err := messages.Unpack(b[:nr])
		if err != nil {
			s.logger.Errorf("error parsing message from stream: " + err.Error())
			s.Close()
			return
		}
		switch m := msg.(type) {
		case *messages.Shutdown:
			s.logger.Debugf("Received Shutdown message: %s", m.Error)
			s.Close()
			return
		case *messages.Ping:
			s.logger.Debug("Received Ping message.")
			atomic.StoreInt64(&s.lastPingAt, time.Now().UnixNano())
			bw, err := messages.Pack(&messages.Pong{})
			if err != nil {
				s.logger.Errorf("Couldn't create a Pong message: %s", err.Error())
			}
			_, err = s.control.Write(bw)
			if err != nil {
				s.logger.Errorf("Failed to send Pong message: %s", err.Error())
			}
		default:
			s.logger.Warn("Unrecognized command. Ignoring.")
		}
	}
}

// RegisterConnection ...
func (s *TCPSession) RegisterConnection(t time.Time) error {
	return s.store.RegisterConnection(s)
}

// RegisterDisconnection ...
func (s *TCPSession) RegisterDisconnection() error {
	return s.store.RegisterDisconnection(s)
}

// RegisterEndpoint ...
func (s *TCPSession) RegisterEndpoint() error {
	return s.store.RegisterEndpoint(s)
}

// UpdateAttribute ...
func (s *TCPSession) UpdateAttribute(name string, value interface{}) error {
	return s.store.UpdateAttribute(s, name, value)
}
