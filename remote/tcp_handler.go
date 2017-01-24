package remote

import (
	"net"

	msgpack "gopkg.in/vmihailenco/msgpack.v2"

	"github.com/garyburd/redigo/redis"
	"github.com/superfly/wormhole/messages"
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
func NewTCPHandler(localhost, clusterURL, nodeID string, pool *redis.Pool) (*TCPHandler, error) {
	s := TCPHandler{
		nodeID:     nodeID,
		sessions:   make(map[string]session.Session),
		localhost:  localhost,
		clusterURL: clusterURL,
		pool:       pool,
	}
	return &s, nil
}

// Serve accepts incoming wormhole connections and passes them to the handler
func (s *TCPHandler) Serve(conn net.Conn) {
	var controlMsg messages.AuthControl
	var tunnelMsg messages.AuthTunnel

	buf := make([]byte, 1024)
	nr, err := conn.Read(buf)
	if err != nil {
		log.Errorf("error reading from stream: " + err.Error())
		return
	}
	err = msgpack.Unmarshal(buf[:nr], &controlMsg)
	if err == nil {
		go s.tcpSessionHandler(conn)
		log.Error("unparsable response")
		return
	}
	err = msgpack.Unmarshal(buf[:nr], &tunnelMsg)
	if err == nil {
		// open a proxy conn on current session
		return
	}
	log.Error("unparsable response")
	conn.Close()
	return
}

func (s *TCPHandler) tcpSessionHandler(conn net.Conn) {
	// Before use, a handshake must be performed on the incoming net.Conn.
	sess := session.NewTCPSession(s.nodeID, s.pool, conn)
	s.sessions[sess.ID()] = sess

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

	defer s.closeSession(sess)

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

func (s *TCPHandler) Close() {
	for _, sess := range s.sessions {
		sess.Close()
		delete(s.sessions, sess.ID())
	}
}

func (s *TCPHandler) closeSession(sess session.Session) {
	sess.Close()
	delete(s.sessions, sess.ID())
}
