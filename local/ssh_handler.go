package local

import (
	"fmt"
	"io"
	"net"
	"os"
	"time"

	msgpack "gopkg.in/vmihailenco/msgpack.v2"

	"github.com/Sirupsen/logrus"
	"github.com/superfly/wormhole/config"
	"github.com/superfly/wormhole/messages"
	"github.com/superfly/wormhole/utils"
	"golang.org/x/crypto/ssh"
)

const (
	sshConnTimeout       = 10 * time.Second
	localConnTimeout     = 5 * time.Second
	sshKeepaliveInterval = 30 * time.Second
)

// SSHHandler type represents the handler that SSHs to wormhole server and serves
// incoming requests
type SSHHandler struct {
	RemoteEndpoint string
	LocalEndpoint  string
	FlyToken       string
	Release        *messages.Release
	Version        string
	ssh            *ssh.Client
	ln             net.Listener
	shutdown       *utils.Shutdown
	logger         *logrus.Entry
}

// NewSSHHandler initializes SSHHandler
func NewSSHHandler(cfg *config.ClientConfig, release *messages.Release) ConnectionHandler {
	return &SSHHandler{
		FlyToken:       cfg.Token,
		RemoteEndpoint: cfg.RemoteEndpoint,
		LocalEndpoint:  cfg.LocalEndpoint,
		Release:        release,
		Version:        cfg.Version,
		shutdown:       utils.NewShutdown(),
		logger:         cfg.Logger.WithFields(logrus.Fields{"prefix": "SSHHandler"}),
	}
}

// ListenAndServe accepts requests coming from wormhole server
// and forwards them to the local server
func (s *SSHHandler) ListenAndServe() error {
	ssh, ln, err := s.dial()
	if err != nil {
		return err
	}
	defer ln.Close()
	defer ssh.Close()
	s.ssh = ssh
	s.ln = ln

	go s.stayAlive()
	go s.registerRelease()

	for {
		select {
		case <-s.shutdown.WaitBeginCh():
			s.shutdown.Complete()
			return nil
		default:
			conn, err := s.ln.Accept()
			if err != nil {
				if err != io.EOF {
					return fmt.Errorf("Failed to accept SSH Session: %s", err.Error())
				}
				return nil
			}

			go s.forwardConnection(conn, s.LocalEndpoint)
		}
	}
}

// Close closes the listener and SSH connection
func (s *SSHHandler) Close() error {
	s.shutdown.Begin()
	s.shutdown.WaitComplete()
	return nil
}

// connects to wormhole server, performs SSH handshake, and
// opens a port on wormhole server that SshHandler can listen on.
// SSH uses FLY_TOKEN for authentication
func (s *SSHHandler) dial() (*ssh.Client, net.Listener, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	config := &ssh.ClientConfig{
		User:          hostname,
		ClientVersion: "wormhole " + s.Version,
		Auth: []ssh.AuthMethod{
			ssh.Password(s.FlyToken),
		},
		Timeout: sshConnTimeout,
	}

	// SSH into wormhole server
	conn, err := ssh.Dial("tcp", s.RemoteEndpoint, config)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to establish SSH connection: %s", err.Error())
	}
	s.logger.Info("Established SSH connection.")

	// open a port on wormhole server that we can listen on
	ln, err := conn.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to open SSH tunnel: %s", err.Error())
	}
	s.logger.Infof("Opened SSH tunnel on %s", ln.Addr().String())
	return conn, ln, nil
}

func (s *SSHHandler) forwardConnection(conn net.Conn, local string) {
	s.logger.Debugf("Accepted SSH session on %s", conn.RemoteAddr())

	localConn, err := net.DialTimeout("tcp", local, localConnTimeout)
	if err != nil {
		s.logger.Errorf("Failed to reach local server: %s", err.Error())
	}

	s.logger.Debugf("Dialed local server on %s", local)

	err = utils.CopyCloseIO(localConn, conn)
	if err != nil && err != io.EOF {
		s.logger.Error(err)
	}
}

func (s *SSHHandler) stayAlive() {
	ticker := time.NewTicker(sshKeepaliveInterval)
	s.logger.Debugf("Sending keepalive every %.1f seconds", sshKeepaliveInterval.Seconds())
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			go func() {
				_, _, err := s.ssh.SendRequest("keepalive", false, nil)
				if err != nil {
					s.logger.Errorf("Keepalive failed: %s", err.Error())
					s.shutdown.Begin()
					return
				}
			}()
		case <-s.shutdown.WaitBeginCh():
			return
		}
	}
}

func (s *SSHHandler) registerRelease() {
	s.logger.Info("Sending release info...")
	releaseBytes, err := msgpack.Marshal(s.Release)
	_, _, err = s.ssh.SendRequest("register-release", false, releaseBytes)
	if err != nil {
		s.logger.Errorf("Failed to send release info: %s", err.Error())
		return
	}
	s.logger.Debug("Release info sent.")
}
