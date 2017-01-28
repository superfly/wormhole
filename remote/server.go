package remote

import (
	"fmt"
	"net"

	"github.com/Sirupsen/logrus"
)

// ListenAndServe accepts incoming wormhole connections and passes them to the handler
func ListenAndServe(addr string, handler Handler) error {
	log := logger.WithFields(logrus.Fields{"prefix": "Server"})
	listener, err := net.Listen("tcp", addr)
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
