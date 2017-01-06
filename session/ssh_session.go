package session

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net"
	"strconv"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/superfly/wormhole/messages"
	"github.com/superfly/wormhole/utils"
	"golang.org/x/crypto/ssh"
)

const (
	sshRemoteForwardRequest      = "tcpip-forward"
	sshForwardedTCPReturnRequest = "forwarded-tcpip"
)

type SshSession struct {
	id           string `redis:"id,omitempty"`
	client       string `redis:"client,omitempty"`
	nodeID       string `redis:"node_id,omitempty"`
	backendID    string `redis:"backend_id,omitempty"`
	clientAddr   string `redis:"client_addr,omitempty"`
	EndpointAddr string `redis:"endpoint_addr,omitempty"`

	release *messages.Release

	sessions map[string]Session
	store    *RedisSessionStore
	config   *ssh.ServerConfig
	tcpConn  net.Conn
	conn     *ssh.ServerConn
	reqs     <-chan *ssh.Request
	chans    <-chan ssh.NewChannel
}

type tcpipForward struct {
	Host string
	Port uint32
}

type directForward struct {
	Host1 string
	Port1 uint32
	Host2 string
	Port2 uint32
}

func NewSshSession(nodeID string, redisPool *redis.Pool, sessions map[string]Session, tcpConn net.Conn, config *ssh.ServerConfig) *SshSession {
	s := &SshSession{
		nodeID:   nodeID,
		tcpConn:  tcpConn,
		store:    NewRedisSessionStore(redisPool),
		sessions: sessions,
	}
	config.PasswordCallback = s.authFromToken
	s.config = config
	return s
}

func (s *SshSession) ID() string {
	return s.id
}

func (s *SshSession) Key() string {
	return "session:" + s.id
}

func (s *SshSession) BackendID() string {
	return s.backendID
}

func (s *SshSession) NodeID() string {
	return s.nodeID
}

func (s *SshSession) Release() *messages.Release {
	return s.release
}

func (s *SshSession) RequireStream() error {
	// Before use, a handshake must be performed on the incoming net.Conn.
	sshConn, chans, reqs, err := ssh.NewServerConn(s.tcpConn, s.config)
	if err != nil {
		log.Printf("Failed to handshake (%s)", err)
		return err
	}
	s.conn = sshConn
	s.chans = chans
	s.reqs = reqs
	go handleChannels(chans)
	return nil
}

func (s *SshSession) HandleRequests(ln *net.TCPListener) {
	for req := range s.reqs {
		switch req.Type {
		case sshRemoteForwardRequest:
			go func() {
				s.handleRemoteForward(req, ln)
				go s.RegisterDisconnection()
			}()
		case "keepalive":
			go s.handleKeepalive(req)
		}
	}
}

func (s *SshSession) RequireAuthentication() error {
	// done as a hook to ssh handshake
	go s.RegisterConnection(time.Now())
	return nil
}

func (s *SshSession) Close() {
	s.RegisterDisconnection()
}

func (s *SshSession) authFromToken(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
	backendID, err := s.store.BackendIDFromToken(string(pass))
	if err != nil {
		return nil, err
	}
	if backendID == "" {
		return nil, errors.New("token rejected")
	}
	s.backendID = backendID
	s.client = string(c.ClientVersion())
	s.id = hex.EncodeToString(c.SessionID())
	s.clientAddr = c.RemoteAddr().String()

	return nil, nil
}

func (s *SshSession) setSshPort(req *ssh.Request, ln net.Listener) tcpipForward {
	t := tcpipForward{}
	ssh.Unmarshal(req.Payload, &t)

	reply := (t.Port == 0) && req.WantReply

	if reply { // Client sent port 0. let them know which port is actually being used
		_, port, _ := net.SplitHostPort(ln.Addr().String())
		portNum, _ := strconv.Atoi(port)
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, uint32(portNum))
		t.Port = uint32(portNum)
		req.Reply(true, b)
	} else {
		req.Reply(true, nil)
	}

	return t
}

func (s *SshSession) handleRemoteForward(req *ssh.Request, ln *net.TCPListener) {
	t := s.setSshPort(req, ln)
	p := directForward{}
	for {
		ln.SetDeadline(time.Now().Add(time.Second))
		tcpConn, err := ln.AcceptTCP()

		/*
			if sess.IsClosed() {
				log.Println("session is closed, breaking listen loop")
				return
			}
		*/

		if err != nil {
			netErr, ok := err.(net.Error)

			//If this is a timeout, then continue to wait for
			//new connections
			if ok && netErr.Timeout() && netErr.Temporary() {
				continue
			}
			log.Errorln("Could not accept tcp conn:", err)
			return
		}
		log.Debugln("Accepted tcp connection from:", tcpConn.RemoteAddr())

		host, port, err := net.SplitHostPort(tcpConn.RemoteAddr().String())
		if err != nil {
			return
		}
		portnum, err := strconv.Atoi(port)
		if err != nil {
			return
		}

		p.Host1 = t.Host
		p.Port1 = t.Port
		p.Host2 = host
		p.Port2 = uint32(portnum)

		ch, reqs, sshErr := s.conn.OpenChannel(sshForwardedTCPReturnRequest, ssh.Marshal(p))
		if sshErr != nil {
			log.WithFields(log.Fields{
				"err": err.Error(),
			}).Error("Open forwarded Channel error:")
			return
		}
		go ssh.DiscardRequests(reqs)
		go utils.CopyCloseIO(tcpConn, ch)
	}
}

// RegisterConnection ...
func (s *SshSession) RegisterConnection(t time.Time) error {
	s.sessions[s.id] = s
	return s.store.RegisterConnection(s)
}

// RegisterDisconnection ...
func (s *SshSession) RegisterDisconnection() error {
	return s.store.RegisterDisconnection(s)
}

// UpdateAttribute ...
func (s *SshSession) UpdateAttribute(name string, value interface{}) error {
	return s.store.UpdateAttribute(s, name, value)
}

func handleChannels(chans <-chan ssh.NewChannel) {
	for _ = range chans {
		// nothing for now.
	}
}

func (s *SshSession) handleKeepalive(req *ssh.Request) {
	if req.WantReply {
		req.Reply(true, nil)
	}
	// TODO: we should update redis with last_seen or something like that
	// go s.RegisterKeepalive(time.Now())
}
