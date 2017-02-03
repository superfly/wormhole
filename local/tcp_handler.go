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
	control        net.Conn
	conns          []net.Conn
}

// ListenAndServe accepts requests coming from wormhole server
// and forwards them to the local server
func (s *TCPHandler) ListenAndServe() error {
	control, err := s.dial()
	if err != nil {
		return err
	}
	defer control.Close()

	s.control = control
	ctlAuthMsg := &messages.AuthControl{
		Token: s.FlyToken,
	}
	buf, err := messages.Pack(ctlAuthMsg)
	if err != nil {
		return fmt.Errorf("error packing message to control: " + err.Error())
	}

	_, err = s.control.Write(buf)
	if err != nil {
		return fmt.Errorf("error writing to control: " + err.Error())
	}

	b := make([]byte, 1024)
	for {
		nr, err := s.control.Read(b)
		if err == io.EOF {
			continue
		}
		if err != nil {
			return fmt.Errorf("error reading from control: " + err.Error())
		}
		msg, err := messages.Unpack(b[:nr])
		if err != nil {
			return fmt.Errorf("error parsing message from stream: " + err.Error())
		}
		switch m := msg.(type) {
		case *messages.OpenTunnel:
			log.Debug("Received Open Tunnel message.")
			conn, err := net.Dial("tcp", s.RemoteEndpoint)
			if err != nil {
				return fmt.Errorf("Failed to establish TCP connection: %s", err.Error())
			}
			authMsg := &messages.AuthTunnel{ClientID: m.ClientID, Token: s.FlyToken}
			b, _ := messages.Pack(authMsg)
			_, err = conn.Write(b)
			if err != nil {
				return fmt.Errorf("Failed to auth tunnel: %s", err.Error())
			}

			log.Infof("Established TCP Tunnel connection for Session: %s", m.ClientID)
			s.conns = append(s.conns, conn)
			go s.forwardConnection(conn, s.LocalEndpoint)
		case *messages.Shutdown:
			log.Debugf("Received Shutdown message: %s", m.Error)
			return s.Close()
		default:
			log.Warn("Unrecognized command. Ignoring.")
		}
	}
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

// connects to wormhole server
func (s *TCPHandler) dial() (net.Conn, error) {
	// TCP into wormhole server
	conn, err := net.Dial("tcp", s.RemoteEndpoint)
	if err != nil {
		return nil, fmt.Errorf("Failed to establish TCP connection: %s", err.Error())
	}
	log.Info("Established TCP connection.")

	return conn, nil
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
