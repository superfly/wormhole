package handler

import (
	"fmt"
	"net"
	"os"

	"github.com/Sirupsen/logrus"
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
	log = logger.WithFields(logrus.Fields{
		"prefix": "SSHHandler",
	})
}

// SSHHandler type represents the handler that accepts incoming wormhole connections
type SSHHandler struct {
	ln         net.Listener
	config     *ssh.ServerConfig
	Port       string
	PrivateKey []byte
}

// InitializeConnection starts a listener for incoming wormhole connections
func (s *SSHHandler) InitializeConnection() error {
	config, err := s.makeConfig()
	if err != nil {
		return fmt.Errorf("Failed to build ssh server config: %s", err)
	}
	s.config = config

	listener, err := net.Listen("tcp", ":"+s.Port)
	if err != nil {
		return fmt.Errorf("Failed to listen on %s (%s)", s.Port, err)
	}
	s.ln = listener

	return nil
}

// ListenAndServe accepts incoming wormhole connections and passes them to the handler
func (s *SSHHandler) ListenAndServe(fn func(net.Conn, *ssh.ServerConfig)) {
	for {
		tcpConn, err := s.ln.Accept()
		if err != nil {
			log.Errorf("Failed to accept wormhole connection (%s)", err)
			break
		}
		log.Debugln("Accepted wormhole TCP conn from:", tcpConn.RemoteAddr())

		go fn(tcpConn, s.config)
	}
}

func (s *SSHHandler) Close() error {
	return nil
}

func (s *SSHHandler) makeConfig() (*ssh.ServerConfig, error) {
	config := &ssh.ServerConfig{}

	if private, err := ssh.ParsePrivateKey(s.PrivateKey); err == nil {
		config.AddHostKey(private)
	} else {
		return nil, err
	}

	return config, nil
}
