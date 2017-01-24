package local

import (
	"fmt"
	"io"
	"net"

	msgpack "gopkg.in/vmihailenco/msgpack.v2"

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
	control        net.Conn
	conns          []net.Conn
}

// InitializeConnection connects to wormhole server
func (s *TCPHandler) InitializeConnection() error {
	// TCP into wormhole server
	conn, err := net.Dial("tcp", s.RemoteEndpoint)
	if err != nil {
		return fmt.Errorf("Failed to establish TCP connection: %s", err.Error())
	}
	log.Info("Established TCP connection.")
	s.control = conn

	return nil
}

// ListenAndServe accepts requests coming from wormhole server
// and forwards them to the local server
func (s *TCPHandler) ListenAndServe() error {
	ctlAuthMsg := messages.AuthControl{
		Token: s.FlyToken,
	}
	buf, err := msgpack.Marshal(ctlAuthMsg)

	_, err = s.control.Write(buf)
	if err != nil {
		return fmt.Errorf("error writing to control: " + err.Error())
	}

	b := make([]byte, 1024)
	nr, err := s.control.Read(b)
	if err != nil {
		return fmt.Errorf("error reading from control: " + err.Error())
	}

	var shutdownMsg messages.Shutdown
	var openTunnel messages.OpenTunnel

	if err = msgpack.Unmarshal(b[:nr], &shutdownMsg); err == nil {
		return s.Close()
	}

	if err = msgpack.Unmarshal(b[:nr], &openTunnel); err == nil {
		conn, err := net.Dial("tcp", s.RemoteEndpoint)
		if err != nil {
			return fmt.Errorf("Failed to establish TCP connection: %s", err.Error())
		}
		log.Info("Established TCP connection.")
		s.conns = append(s.conns, conn)
		s.forwardConnection(conn, s.LocalEndpoint)
	}

	return nil
}

// Close closes the listener and TCP connection
func (s *TCPHandler) Close() error {
	err := s.control.Close()
	if err != nil {
		log.Errorf("Control TCP conn close: %s", err)
	}
	for _, c := range s.conns {
		err = c.Close()
		if err != nil {
			log.Errorf("Proxy TCP conn close: %s", err)
		}
	}
	return err
}

func (s *TCPHandler) forwardConnection(tunnel net.Conn, local string) {
	log.Debugf("Accepted TCP session on %s", tunnel.RemoteAddr())

	localConn, err := net.DialTimeout("tcp", local, localConnTimeout)
	if err != nil {
		log.Errorf("Failed to reach local server: %s", err.Error())
	}

	log.Debugf("Dialed local server on %s", local)

	err = utils.CopyCloseIO(localConn, tunnel)
	if err != nil && err != io.EOF {
		log.Error(err)
	}
}
