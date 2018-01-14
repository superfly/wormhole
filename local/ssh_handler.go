package local

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"os"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/superfly/wormhole/config"
	"github.com/superfly/wormhole/messages"
	wnet "github.com/superfly/wormhole/net"
	"github.com/superfly/wormhole/utils"
	"golang.org/x/crypto/ssh"
)

const (
	sshConnTimeout       = 10 * time.Second
	localConnTimeout     = 5 * time.Second
	sshKeepaliveInterval = 10 * time.Second
	maxKeepaliveLatency  = 20 * time.Second
)

// SSHHandler type represents the handler that SSHs to wormhole server and serves
// incoming requests
type SSHHandler struct {
	RemoteEndpoint         string
	LocalEndpoint          string
	FlyToken               string
	Release                *messages.Release
	Version                string
	ssh                    *ssh.Client
	ln                     net.Listener
	shutdown               *utils.Shutdown
	logger                 *logrus.Entry
	lastKeepaliveReplyAt   int64
	localEndpointTLSConfig *tls.Config
}

// NewSSHHandler initializes SSHHandler
func NewSSHHandler(cfg *config.ClientConfig, release *messages.Release) (*SSHHandler, error) {
	var localTLSConfig *tls.Config
	if cfg.LocalEndpointUseTLS {
		localTLSConfig = &tls.Config{
			InsecureSkipVerify: cfg.LocalEndpointInsecureSkipVerify,
		}
		rootCAs, err := x509.SystemCertPool()
		if err != nil {
			return nil, err
		}
		if len(cfg.LocalEndpointCACert) != 0 {
			ok := rootCAs.AppendCertsFromPEM(cfg.LocalEndpointCACert)
			if !ok {
				return nil, fmt.Errorf("couln't append a root CA")
			}
		}
		localTLSConfig.RootCAs = rootCAs
	}

	return &SSHHandler{
		FlyToken:               cfg.Token,
		RemoteEndpoint:         cfg.RemoteEndpoint,
		LocalEndpoint:          cfg.LocalEndpoint,
		Release:                release,
		Version:                cfg.Version,
		shutdown:               utils.NewShutdown(),
		logger:                 cfg.Logger.WithFields(logrus.Fields{"prefix": "SSHHandler"}),
		localEndpointTLSConfig: localTLSConfig,
	}, nil
}

// ListenAndServe accepts requests coming from wormhole server
// and forwards them to the local server
func (s *SSHHandler) ListenAndServe() error {
	s.shutdown = utils.NewShutdown()
	ssh, ln, err := s.dial()
	if err != nil {
		return err
	}
	defer ln.Close()
	defer ssh.Close()
	s.ssh = ssh
	s.ln = ln
	s.lastKeepaliveReplyAt = time.Now().UnixNano()

	go s.stayAlive()
	go s.registerRelease()
	go s.handleSSH()

	select {
	case <-s.shutdown.WaitBeginCh():
		s.logger.Debug("Shutdown triggered")
		s.shutdown.Complete()
		return s.shutdown.Error()
	}
}

func (s *SSHHandler) handleSSH() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if err != io.EOF {
				s.shutdown.Begin(fmt.Errorf("Failed to accept SSH Session: %s", err.Error()))
				return
			}
			s.logger.Debugln("SSH Tunnel is closed:", err)
			s.shutdown.Begin(nil)
			return
		}

		go s.forwardConnection(conn, s.LocalEndpoint)
	}
}

// Close closes the listener and SSH connection
func (s *SSHHandler) Close() error {
	s.shutdown.Begin(nil)
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
		Timeout:         sshConnTimeout,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
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

	var localConn net.Conn
	var err error
	if s.localEndpointTLSConfig == nil {
		localConn, err = net.DialTimeout("tcp", local, localConnTimeout)
	} else {
		dialer := &net.Dialer{Timeout: localConnTimeout}
		localConn, err = tls.DialWithDialer(dialer, "tcp", local, s.localEndpointTLSConfig)
	}
	if err != nil {
		s.shutdown.Begin(fmt.Errorf("Failed to reach local server: %s", err.Error()))
		return
	}

	s.logger.Debugf("Dialed local server on %s", local)
	_, _, err = wnet.CopyCloseIO(localConn, conn)
	if err != nil && err != io.EOF {
		s.logger.Error(err)
	}
}

func (s *SSHHandler) stayAlive() {
	// set lastPing to something sane
	lastKeepalive := time.Unix(atomic.LoadInt64(&s.lastKeepaliveReplyAt)-1, 0)
	keepaliveCheck := time.NewTicker(time.Second)
	ticker := time.NewTicker(sshKeepaliveInterval)
	s.logger.Debugf("Sending keepalive every %.1f seconds", sshKeepaliveInterval.Seconds())

	defer ticker.Stop()
	defer keepaliveCheck.Stop()

	for {
		select {
		case <-keepaliveCheck.C:
			lastKeepaliveReply := time.Unix(0, atomic.LoadInt64(&s.lastKeepaliveReplyAt))
			needReply := lastKeepaliveReply.Sub(lastKeepalive) < 0
			replyLatency := time.Since(lastKeepaliveReply)

			if needReply && replyLatency > maxKeepaliveLatency {
				s.logger.Infof("Last Keepalive: %v, Last Keepalive reply: %v", lastKeepalive, lastKeepaliveReply)
				err := fmt.Errorf("ssh_handler: connection stale, haven't gotten keepalive reply in %d seconds", int(replyLatency.Seconds()))
				s.shutdown.Begin(err)
				return
			}

		case <-ticker.C:
			lastKeepalive = time.Now()
			go func() {
				_, _, err := s.ssh.SendRequest("keepalive", false, nil)
				if err != nil {
					s.logger.Errorf("Keepalive failed: %s", err.Error())
				} else {
					atomic.StoreInt64(&s.lastKeepaliveReplyAt, time.Now().UnixNano())
				}
			}()
		case <-s.shutdown.WaitBeginCh():
			return
		}
	}
}

func (s *SSHHandler) registerRelease() {
	s.logger.Info("Sending release info...")
	releaseBytes, err := messages.Pack(s.Release)
	_, _, err = s.ssh.SendRequest("register-release", false, releaseBytes)
	if err != nil {
		s.logger.Errorf("Failed to send release info: %s", err.Error())
		return
	}
	s.logger.Debug("Release info sent.")
}
