package remote

import (
	"fmt"
	"net"

	"github.com/sirupsen/logrus"
)

// Server contains configuration options for a TCP Server
type Server struct {
	Logger *logrus.Logger
}

// ListenAndServe accepts incoming wormhole connections and passes them to the handler
func (s *Server) ListenAndServe(addr string, handler Handler) error {
	log := s.Logger.WithFields(logrus.Fields{"prefix": "Server"})
	listener, err := s.newTCPListener(addr)
	if err != nil {
		return fmt.Errorf("Failed to listen on %s (%s)", addr, err.Error())
	}

	for {
		tcpConn, err := listener.AcceptTCP()
		if err != nil {
			log.Errorf("Failed to accept wormhole connection (%s)", err.Error())
			break
		}
		log.Debugln("Accepted wormhole TCP conn from:", tcpConn.RemoteAddr())

		go handler.Serve(tcpConn)
	}
	return nil
}

func (s *Server) newTCPListener(addr string) (*net.TCPListener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	tcpLN, ok := ln.(*net.TCPListener)
	if !ok {
		return nil, fmt.Errorf("Could not create tcp listener")
	}
	return tcpLN, nil
}
