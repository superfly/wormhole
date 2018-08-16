package remote

import "net"

// Handler serves a connection.
// It's the entry point for starting a session, managing a handshake, auth
// and encryption (e.g. SSH)
type Handler interface {
	Serve(net.Conn)
	Close()
}
