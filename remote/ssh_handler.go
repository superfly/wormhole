package remote

import (
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/superfly/wormhole/session"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
	"golang.org/x/crypto/ssh"
)

const (
	sshRemoteForwardRequest      = "tcpip-forward"
	sshForwardedTCPReturnRequest = "forwarded-tcpip"
)

var logger = logrus.New()
var log *logrus.Entry

func init() {
	logger.Formatter = new(prefixed.TextFormatter)
	if os.Getenv("LOG_LEVEL") == "debug" {
		logger.Level = logrus.DebugLevel
	}
}

// SSHHandler type represents the handler that accepts incoming wormhole connections
type SSHHandler struct {
	config     *ssh.ServerConfig
	nodeID     string
	localhost  string
	clusterURL string
	sessions   map[string]session.Session
	pool       *redis.Pool
	logger     *logrus.Entry
}

// NewSSHHandler ...
func NewSSHHandler(key []byte, localhost, clusterURL, nodeID string, pool *redis.Pool) (*SSHHandler, error) {
	config, err := makeConfig(key)
	if err != nil {
		return nil, fmt.Errorf("Couldn't create SSH Server Config: %s", err.Error())
	}

	s := SSHHandler{
		nodeID:     nodeID,
		sessions:   make(map[string]session.Session),
		localhost:  localhost,
		clusterURL: clusterURL,
		pool:       pool,
		config:     config,
		logger:     logger.WithFields(logrus.Fields{"prefix": "SSHHandler"}),
	}
	return &s, nil
}

// Serve accepts incoming wormhole connections and passes them to the handler
func (s *SSHHandler) Serve(conn net.Conn) {
	s.sshSessionHandler(conn)
}

func makeConfig(key []byte) (*ssh.ServerConfig, error) {
	config := &ssh.ServerConfig{}

	if private, err := ssh.ParsePrivateKey(key); err == nil {
		config.AddHostKey(private)
	} else {
		return nil, err
	}

	return config, nil
}

func (s *SSHHandler) sshSessionHandler(conn net.Conn) {
	// Before use, a handshake must be performed on the incoming net.Conn.
	sess := session.NewSSHSession(s.nodeID, s.pool, conn, s.config)
	s.sessions[sess.ID()] = sess
	err := sess.RequireStream()
	if err != nil {
		s.logger.Errorln("error getting a stream:", err)
		return
	}

	err = sess.RequireAuthentication()
	if err != nil {
		s.logger.Errorln(err)
		return
	}

	s.logger.Println("Client authenticated.")

	defer s.closeSession(sess)

	ln, err := listenTCP()
	if err != nil {
		s.logger.Errorln(err)
		return
	}

	_, port, _ := net.SplitHostPort(ln.Addr().String())
	sess.EndpointAddr = s.localhost + ":" + port
	sess.ClusterURL = s.clusterURL

	if err = sess.RegisterEndpoint(); err != nil {
		s.logger.Errorln("Error registering endpoint:", err)
		return
	}

	s.logger.Infof("Started session %s for %s (%s). Listening on: %s", sess.ID(), sess.NodeID(), sess.Client(), sess.Endpoint())

	sess.HandleRequests(ln)
}

func (s *SSHHandler) closeSession(sess session.Session) {
	sess.Close()
	delete(s.sessions, sess.ID())
}

func (s *SSHHandler) Close() {
	for _, sess := range s.sessions {
		sess.Close()
		delete(s.sessions, sess.ID())
	}
}

func listenTCP() (*net.TCPListener, error) {
	addr, err := net.ResolveTCPAddr("tcp4", ":0")
	if err != nil {
		return nil, errors.New("could not parse TCP addr: " + err.Error())
	}
	ln, err := net.ListenTCP("tcp4", addr)
	if err != nil {
		return nil, errors.New("could not listen on: " + err.Error())
	}
	return ln, nil
}
