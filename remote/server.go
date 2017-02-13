package remote

import (
	"crypto/tls"
	"fmt"
	"net"

	"github.com/Sirupsen/logrus"
)

// Server contains configuration options for a TCP Server
type Server struct {
	TLSCert       *[]byte
	TLSPrivateKey *[]byte
	Logger        *logrus.Logger
}

// ListenAndServe accepts incoming wormhole connections and passes them to the handler
func (s *Server) ListenAndServe(addr string, handler Handler) error {
	log := s.Logger.WithFields(logrus.Fields{"prefix": "Server"})
	listener, err := s.newListener(addr)
	if err != nil {
		return fmt.Errorf("Failed to listen on %s (%s)", addr, err.Error())
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Errorf("Failed to accept wormhole connection (%s)", err.Error())
			break
		}
		log.Debugln("Accepted wormhole TCP conn from:", conn.RemoteAddr())

		go handler.Serve(conn)
	}
	return nil
}

func (s *Server) newListener(addr string) (net.Listener, error) {
	if s.encrypted() {
		cert, err := tls.X509KeyPair(*s.TLSCert, *s.TLSPrivateKey)
		if err != nil {
			return nil, err
		}
		return tls.Listen("tcp", addr, &tls.Config{
			Certificates: []tls.Certificate{cert},
		})
	} else {
		return net.Listen("tcp", addr)
	}
}

func (s *Server) encrypted() bool {
	return s.TLSCert != nil &&
		s.TLSPrivateKey != nil &&
		len(*s.TLSCert) > 0 &&
		len(*s.TLSPrivateKey) > 0
}
