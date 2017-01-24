package remote

import (
	"net"

	"github.com/garyburd/redigo/redis"
	"github.com/superfly/wormhole/session"
)

// TCPHandler type represents the handler that accepts incoming wormhole connections
// WARNING: TCPHandler is insecure and shouldn't be used in production
type TCPHandler struct {
	nodeID     string
	localhost  string
	clusterURL string
	sessions   map[string]session.Session
	pool       *redis.Pool
}

// NewTCPHandler ...
func NewTCPHandler(localhost, clusterURL, nodeID string, pool *redis.Pool, sessions map[string]session.Session) (*TCPHandler, error) {
	s := TCPHandler{
		nodeID:     nodeID,
		sessions:   sessions,
		localhost:  localhost,
		clusterURL: clusterURL,
		pool:       pool,
	}
	return &s, nil
}

// Serve accepts incoming wormhole connections and passes them to the handler
func (s *TCPHandler) Serve(conn net.Conn) {
	s.tcpSessionHandler(conn)
}

func (s *TCPHandler) tcpSessionHandler(conn net.Conn) {
	// Before use, a handshake must be performed on the incoming net.Conn.
	sess := session.NewTCPSession(s.nodeID, s.pool, s.sessions, conn)
	err := sess.RequireStream()
	if err != nil {
		log.Errorln("error getting a stream:", err)
		return
	}

	err = sess.RequireAuthentication()
	if err != nil {
		log.Errorln(err)
		return
	}

	log.Println("Client authenticated.")

	defer sess.Close()

	ln, err := listenTCP()
	if err != nil {
		log.Errorln(err)
		return
	}

	_, port, _ := net.SplitHostPort(ln.Addr().String())
	sess.EndpointAddr = s.localhost + ":" + port
	sess.ClusterURL = s.clusterURL

	if err = sess.RegisterEndpoint(); err != nil {
		log.Errorln("Error registering endpoint:", err)
		return
	}

	log.Infof("Started session %s for %s (%s). Listening on: %s", sess.ID(), sess.NodeID(), sess.Client(), sess.Endpoint())

	sess.HandleRequests(ln)
}
