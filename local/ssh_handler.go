package handler

import (
	"net"
	"os"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/superfly/wormhole/messages"
	config "github.com/superfly/wormhole/shared"
	"github.com/superfly/wormhole/utils"
	"golang.org/x/crypto/ssh"
)

type SshHandler struct {
	RemoteEndpoint string
	LocalEndpoint  string
	FlyToken       string
	Release        *messages.Release
	Version        string
	ssh            *ssh.Client
	ln             net.Listener
}

func (s *SshHandler) InitializeConnection() error {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	config := &ssh.ClientConfig{
		User:          hostname,
		ClientVersion: s.Version,
		Auth: []ssh.AuthMethod{
			ssh.Password(s.FlyToken),
		},
		Timeout: 10 * time.Second,
	}
	conn, err := ssh.Dial("tcp", s.RemoteEndpoint, config)
	if err != nil {
		return err
	}
	log.Info("Connected to the tunnel.")
	s.ssh = conn

	ln, err := s.ssh.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return err
	}
	s.ln = ln
	return nil
}

func (s *SshHandler) ListenAndServe() error {
	defer s.Close()
	go s.stayAlive()

	for {
		conn, err := s.ln.Accept()
		if err != nil { // Unable to accept new connection - listener likely closed
			log.Errorln("Error accepting stream:", err)
			return err
		}

		go forwardConnection(conn, s.LocalEndpoint)
	}
}

func (s *SshHandler) Close() error {
	return s.ssh.Close()
}

var copyBufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 32*1024)
		return &b
	},
}

func forwardConnection(conn net.Conn, local string) error {
	log.Debugln("Accepted SSH tunnel")

	localConn, err := net.DialTimeout("tcp", local, 5*time.Second)
	if err != nil {
		log.Errorln(err)
		return err
	}

	log.Debugln("dialed local connection")

	if err = localConn.(*net.TCPConn).SetReadBuffer(config.MaxBuffer); err != nil {
		log.Errorln("TCP SetReadBuffer error:", err)
	}
	if err = localConn.(*net.TCPConn).SetWriteBuffer(config.MaxBuffer); err != nil {
		log.Errorln("TCP SetWriteBuffer error:", err)
	}

	log.Debugln("local connection settings has been set...")

	err = utils.CopyCloseIO(localConn, conn)
	return err
}

func (s *SshHandler) stayAlive() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			go func() {
				log.Debug("Sending keepalive...")
				_, _, err := s.ssh.SendRequest("keepalive", false, nil)
				if err != nil {
					log.Errorln("Keepalive error:", err)
					return
				}
				log.Debug("Keepalive sent.")
			}()
		}
	}
}
