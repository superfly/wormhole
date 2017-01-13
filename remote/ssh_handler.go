package handler

import (
	"fmt"
	"net"

	log "github.com/Sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

const (
	sshRemoteForwardRequest      = "tcpip-forward"
	sshForwardedTCPReturnRequest = "forwarded-tcpip"
)

type SshHandler struct {
	ln         net.Listener
	config     *ssh.ServerConfig
	Port       string
	PrivateKey []byte
}

func (s *SshHandler) InitializeConnection() error {
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

func (s *SshHandler) ListenAndServe(fn func(net.Conn, *ssh.ServerConfig)) {
	for {
		tcpConn, err := s.ln.Accept()
		if err != nil {
			log.Errorf("Failed to accept wormhole connection (%s)", err)
			break
		}
		log.Debugln("Accepted wormhole TCP conn from:", tcpConn.RemoteAddr())

		fn(tcpConn, s.config)
	}
}

func (s *SshHandler) Close() error {
	return nil
}

func (s *SshHandler) makeConfig() (*ssh.ServerConfig, error) {
	config := &ssh.ServerConfig{}

	if private, err := ssh.ParsePrivateKey(s.PrivateKey); err == nil {
		config.AddHostKey(private)
	} else {
		return nil, err
	}

	return config, nil
}
