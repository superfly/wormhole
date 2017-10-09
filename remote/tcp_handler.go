package remote

import (
	"crypto/tls"
	"net"

	"github.com/garyburd/redigo/redis"
	"github.com/sirupsen/logrus"
	"github.com/superfly/wormhole/config"
	"github.com/superfly/wormhole/messages"
	wnet "github.com/superfly/wormhole/net"
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
	tlsConfig  *tls.Config
	logger     *logrus.Entry
}

// NewTCPHandler ...
func NewTCPHandler(cfg *config.ServerConfig, pool *redis.Pool) (*TCPHandler, error) {
	h := TCPHandler{
		nodeID:     cfg.NodeID,
		sessions:   make(map[string]session.Session),
		localhost:  cfg.Localhost,
		clusterURL: cfg.ClusterURL,
		pool:       pool,
		logger:     cfg.Logger.WithFields(logrus.Fields{"prefix": "TCPHandler"}),
	}

	if len(cfg.TLSCert) != 0 && len(cfg.TLSPrivateKey) != 0 {
		keyPair, err := tls.X509KeyPair(cfg.TLSCert, cfg.TLSPrivateKey)
		if err != nil {
			return nil, err
		}

		sConf := &tls.Config{
			Certificates: []tls.Certificate{keyPair},
		}
		h.tlsConfig = sConf
	}

	return &h, nil
}

// Serve accepts incoming wormhole connections and passes them to the handler
func (h *TCPHandler) Serve(conn *net.TCPConn) {
	var useConn net.Conn
	if h.tlsConfig != nil {
		var err error
		useConn, err = wnet.GenericTLSWrap(conn, h.tlsConfig, tls.Server)
		if err != nil {
			h.logger.Errorf("Error establishing TLS wrapping: " + err.Error())
			return
		}
	} else {
		useConn = conn
	}

	buf := make([]byte, 1024)
	nr, err := useConn.Read(buf)
	if err != nil {
		h.logger.Errorf("error reading from stream: " + err.Error())
		return
	}
	msg, err := messages.Unpack(buf[:nr])
	if err != nil {
		h.logger.Errorf("error parsing message from stream: " + err.Error())
		return
	}

	switch m := msg.(type) {
	case *messages.AuthControl:
		go h.tcpSessionHandler(useConn)
	case *messages.AuthTunnel:
		if sess, ok := h.sessions[m.ClientID]; !ok {
			h.logger.Error("New tunnel conn not associated with any session. Closing")
			conn.Close()
		} else {
			// open a proxy conn on current session
			h.logger.Debugf("Adding New tunnel conn to session: %s", sess.ID())
			tcpSess := sess.(*session.TCPSession)
			tcpSess.AddTunnel(useConn)
		}
	default:
		h.logger.Error("unparsable response")
		conn.Close()
	}
}

// Close closes all sessions handled by TCPHandler
func (h *TCPHandler) Close() {
	for _, sess := range h.sessions {
		sess.Close()
		delete(h.sessions, sess.ID())
	}
}

func (h *TCPHandler) tcpSessionHandler(conn net.Conn) {
	// Before use, a handshake must be performed on the incoming net.Conn.
	sess := session.NewTCPSession(h.logger.Logger, h.nodeID, h.pool, conn)
	h.sessions[sess.ID()] = sess

	err := sess.RequireStream()
	if err != nil {
		h.logger.WithField("client_addr", conn.RemoteAddr().String()).Errorln("error getting a stream:", err)
		return
	}

	err = sess.RequireAuthentication()
	if err != nil {
		h.logger.Errorln(err)
		return
	}

	h.logger.Println("Client authenticated.")

	defer h.closeSession(sess)

	ln, err := listenTCP("tcp_ingress", sess)
	if err != nil {
		h.logger.Errorln(err)
		return
	}

	_, port, _ := net.SplitHostPort(ln.Addr().String())
	sess.EndpointAddr = h.localhost + ":" + port
	sess.ClusterURL = h.clusterURL

	if err = sess.RegisterEndpoint(); err != nil {
		h.logger.Errorln("Error registering endpoint:", err)
		return
	}

	h.logger.Infof("Started session %s for %s (%s). Listening on: %s", sess.ID(), sess.NodeID(), sess.Client(), sess.Endpoint())

	sess.HandleRequests(ln)
}

func (h *TCPHandler) closeSession(sess session.Session) {
	sess.Close()
	delete(h.sessions, sess.ID())
}
