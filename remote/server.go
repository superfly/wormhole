package remote

import (
	"net"

	"github.com/sirupsen/logrus"
)

// Server contains configuration options for a TCP Server
type Server struct {
	Logger *logrus.Logger
}

// Serve accepts incoming wormhole connections and passes them to the handler
func (s *Server) Serve(listener net.Listener, handler Handler) error {
	log := s.Logger.WithFields(logrus.Fields{"prefix": "Server"})

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Errorf("Failed to accept wormhole connection (%s)", err.Error())
			break
		}

		log.Println("Accepted wormhole TCP conn from:", conn.RemoteAddr())

		go handler.Serve(conn)
	}
	return nil
}

// func (s *Server) newTCPListener(addr string) (*net.TCPListener, error) {
// 	ln, err := net.Listen("tcp", addr)
// 	if err != nil {
// 		return nil, err
// 	}
// 	tcpLN, ok := ln.(*net.TCPListener)
// 	if !ok {
// 		return nil, fmt.Errorf("Could not create tcp listener")
// 	}
// 	return tcpLN, nil
// }
