package remote

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"

	"github.com/Sirupsen/logrus"
)

// Server contains configuration options for a TCP Server
type Server struct {
	Encrypted bool
}

// ListenAndServe accepts incoming wormhole connections and passes them to the handler
func (s *Server) ListenAndServe(addr string, handler Handler) error {
	log := logger.WithFields(logrus.Fields{"prefix": "Server"})
	listener, err := newListener(addr, s.Encrypted)
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

func newListener(addr string, encrypted bool) (net.Listener, error) {
	if encrypted {
		cert, err := tls.LoadX509KeyPair(os.Getenv("TLS_CERT_FILE"), os.Getenv("TLS_PRIVATE_KEY_FILE"))
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
