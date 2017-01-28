package session

import (
	"fmt"
	"io"
	"net"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/rs/xid"
	"github.com/superfly/wormhole/messages"
	"github.com/superfly/wormhole/utils"
	"golang.org/x/crypto/ssh"
)

const (
	pingTimeoutInterval = 60 * time.Second
)

type TCPSession struct {
	baseSession

	config  *ssh.ServerConfig
	control net.Conn
	reqs    <-chan *ssh.Request
	chans   <-chan ssh.NewChannel
	conns   chan net.Conn
}

// NewTCPSession creates new TCPSession struct
func NewTCPSession(nodeID string, redisPool *redis.Pool, conn net.Conn) *TCPSession {
	base := baseSession{
		id:     xid.New().String(),
		nodeID: nodeID,
		store:  NewRedisStore(redisPool),
	}
	s := &TCPSession{
		control:     conn,
		baseSession: base,
		conns:       make(chan net.Conn, 10),
	}
	return s
}

func (s *TCPSession) AddTunnel(conn net.Conn) {
	select {
	case s.conns <- conn:
		log.Info("Added Tunnel")
	default:
		log.Info("Tunnels buffer is full, discarding.")
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
		log.Debug("No tunnels in pool, requesting tunnel from control . . .")

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

		case <-time.After(pingTimeoutInterval):
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
	s.handleRemoteForward(ln)
}

func (s *TCPSession) RequireAuthentication() error {
	// we can skip this right now, eventually needs conn to be authed
	go s.RegisterConnection(time.Now())
	return nil
}

func (s *TCPSession) Close() {
	s.RegisterDisconnection()
	log.Infof("Closed session %s for %s (%s).", s.ID(), s.NodeID(), s.Client())
	s.control.Close()
}

func (s *TCPSession) handleRemoteForward(ln *net.TCPListener) {
	defer func() {
		err := ln.Close()
		if err != nil {
			log.Debugf("Couldn't close ingress conn: %s", err)
			return
		}
		log.Debugf("Closed ingress conn: %s", ln.Addr().String())
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
			log.Errorln("Could not accept Ingress TCP conn:", err)
			return
		}
		log.Debugln("Accepted Ingress TCP conn from:", tcpConn.RemoteAddr())

		tunnel, err := s.GetTunnel()
		if err != nil {
			log.Errorf("Could not get a tunnel conn: %s", err.Error())
			return
		}

		// request a new tunnel
		go func() {
			if err = s.openTunnel(); err != nil {
				log.Error(err)
			}
		}()

		err = utils.CopyCloseIO(tunnel, tcpConn)
		if err != nil && err != io.EOF {
			log.Error(err)
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
