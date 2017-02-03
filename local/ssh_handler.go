package local

import (
	"fmt"
	"io"
	"net"
	"os"
	"time"

	msgpack "gopkg.in/vmihailenco/msgpack.v2"

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
}

// NewSSHHandler initializes SSHHandler
func NewSSHHandler(token, remoteEndpoint, localEndpoint, version string, release *messages.Release) ConnectionHandler {
	return &SSHHandler{
		FlyToken:       token,
		RemoteEndpoint: remoteEndpoint,
		LocalEndpoint:  localEndpoint,
		Release:        release,
		Version:        version,
		shutdown:       utils.NewShutdown(),
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

			go forwardConnection(conn, s.LocalEndpoint)
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
	log.Info("Established SSH connection.")

	// open a port on wormhole server that we can listen on
	ln, err := conn.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to open SSH tunnel: %s", err.Error())
	}
	log.Infof("Opened SSH tunnel on %s", ln.Addr().String())
	return conn, ln, nil
}

func forwardConnection(conn net.Conn, local string) {
	log.Debugf("Accepted SSH session on %s", conn.RemoteAddr())

	localConn, err := net.DialTimeout("tcp", local, localConnTimeout)
	if err != nil {
		log.Errorf("Failed to reach local server: %s", err.Error())
	}

	log.Debugf("Dialed local server on %s", local)

	err = utils.CopyCloseIO(localConn, conn)
	if err != nil && err != io.EOF {
		log.Error(err)
	}
}

func (s *SSHHandler) stayAlive() {
	ticker := time.NewTicker(sshKeepaliveInterval)
	log.Debugf("Sending keepalive every %.1f seconds", sshKeepaliveInterval.Seconds())
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			go func() {
				_, _, err := s.ssh.SendRequest("keepalive", false, nil)
				if err != nil {
					log.Errorf("Keepalive failed: %s", err.Error())
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
	log.Info("Sending release info...")
	releaseBytes, err := msgpack.Marshal(s.Release)
	_, _, err = s.ssh.SendRequest("register-release", false, releaseBytes)
	if err != nil {
		log.Errorf("Failed to send release info: %s", err.Error())
		return
	}
	log.Debug("Release info sent.")
}
