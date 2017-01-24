package session

import (
	"io"
	"net"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/superfly/wormhole/utils"
	"golang.org/x/crypto/ssh"
)

type TCPSession struct {
	baseSession

	config *ssh.ServerConfig
	conn   net.Conn
	reqs   <-chan *ssh.Request
	chans  <-chan ssh.NewChannel
}

// NewTCPSession creates new TCPSession struct
func NewTCPSession(nodeID string, redisPool *redis.Pool, sessions map[string]Session, conn net.Conn) *TCPSession {
	base := baseSession{
		nodeID:   nodeID,
		store:    NewRedisStore(redisPool),
		sessions: sessions,
	}
	s := &TCPSession{
		conn:        conn,
		baseSession: base,
	}
	return s
}

func (s *TCPSession) RequireStream() error {
	// do nothing right now, eventually need some handshake here
	return nil
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
	s.conn.Close()
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

		err = utils.CopyCloseIO(s.conn, tcpConn)
		if err != nil && err != io.EOF {
			log.Error(err)
		}
	}
}

// RegisterConnection ...
func (s *TCPSession) RegisterConnection(t time.Time) error {
	s.sessions[s.id] = s
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
