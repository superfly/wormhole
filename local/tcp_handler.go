package local

import (
	"fmt"
	"io"
	"net"

	"github.com/superfly/wormhole/messages"
	"github.com/superfly/wormhole/utils"
)

// TCPHandler type represents the handler that opens a TCP conn to wormhole server and serves
// incoming requests
// WARNING: TCPHandler is insecure and shouldn't be used in production
type TCPHandler struct {
	RemoteEndpoint string
	LocalEndpoint  string
	FlyToken       string
	Release        *messages.Release
	Version        string
	ln             net.Listener
	conn           net.Conn
}

// InitializeConnection connects to wormhole server
func (s *TCPHandler) InitializeConnection() error {
	// TCP into wormhole server
	conn, err := net.Dial("tcp", s.RemoteEndpoint)
	if err != nil {
		return fmt.Errorf("Failed to establish TCP connection: %s", err.Error())
	}
	log.Info("Established TCP connection.")
	s.conn = conn

	return nil
}

// ListenAndServe accepts requests coming from wormhole server
// and forwards them to the local server
func (s *TCPHandler) ListenAndServe() error {
	s.forwardConnection(s.LocalEndpoint)
	return nil
}

// Close closes the listener and TCP connection
func (s *TCPHandler) Close() error {
	err := s.conn.Close()
	if err != nil {
		log.Errorf("TCP conn close: %s", err)
	}
	return err
}

func (s *TCPHandler) forwardConnection(local string) {
	log.Debugf("Accepted TCP session on %s", s.conn.RemoteAddr())

	localConn, err := net.DialTimeout("tcp", local, localConnTimeout)
	if err != nil {
		log.Errorf("Failed to reach local server: %s", err.Error())
	}

	log.Debugf("Dialed local server on %s", local)

	err = utils.CopyCloseIO(localConn, s.conn)
	if err != nil && err != io.EOF {
		log.Error(err)
	}
}
