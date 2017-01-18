package handler

import (
	"fmt"
	"io"
	"net"
	"os"
	"time"

	msgpack "gopkg.in/vmihailenco/msgpack.v2"

	log "github.com/Sirupsen/logrus"
	"github.com/superfly/wormhole/messages"
	"github.com/superfly/wormhole/utils"
	"golang.org/x/crypto/ssh"
)

type ConnectionHandler interface {
	InitializeConnection() error
	Close() error
}

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
		ClientVersion: "wormhole " + s.Version,
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
	go s.stayAlive()
	go s.registerRelease()

	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if err != io.EOF {
				return fmt.Errorf("Error accepting stream: %s", err)
			}
			return nil
		}

		go forwardConnection(conn, s.LocalEndpoint)
	}
}

func (s *SshHandler) Close() error {
	err := s.ssh.Close()
	if err != nil {
		log.Errorf("SSH conn close: %s", err)
	}
	err = s.ln.Close()
	if err != nil {
		log.Errorf("SSH listener close: %s", err)
	}
	return err
}

func forwardConnection(conn net.Conn, local string) {
	log.Debugln("Accepted SSH tunnel")

	localConn, err := net.DialTimeout("tcp", local, 5*time.Second)
	if err != nil {
		log.Errorln(err)
	}

	log.Debugln("dialed local connection")

	err = utils.CopyCloseIO(localConn, conn)
	if err != nil && err != io.EOF {
		log.Error(err)
	}
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

func (s *SshHandler) registerRelease() {
	log.Debug("Sending register-release...")
	releaseBytes, err := msgpack.Marshal(s.Release)
	_, _, err = s.ssh.SendRequest("register-release", false, releaseBytes)
	if err != nil {
		log.Errorln("register-release error:", err)
		return
	}
	log.Debug("register-release sent.")
}
