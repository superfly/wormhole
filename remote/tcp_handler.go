package remote

import (
	"crypto/tls"
	"net"

	"github.com/gomodule/redigo/redis"
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
	registry   *session.Registry
	pool       *redis.Pool
	tlsConfig  *tls.Config
	logger     *logrus.Entry
	lFactory   wnet.ListenerFactory
}

// NewTCPHandler ...
func NewTCPHandler(cfg *config.ServerConfig, registry *session.Registry, pool *redis.Pool, factory wnet.ListenerFactory) (*TCPHandler, error) {
	h := TCPHandler{
		nodeID:     cfg.NodeID,
		registry:   registry,
		localhost:  cfg.Localhost,
		clusterURL: cfg.ClusterURL,
		pool:       pool,
		lFactory:   factory,
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
func (h *TCPHandler) Serve(conn net.Conn) {
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
		if sess := h.registry.GetSession(m.ClientID); sess == nil {
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
	h.lFactory.Close()
}

func (h *TCPHandler) tcpSessionHandler(conn net.Conn) {
	// Before use, a handshake must be performed on the incoming net.Conn.
	sess := session.NewTCPSession(h.logger.Logger, h.nodeID, h.pool, conn)
	h.registry.AddSession(sess)

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

	/*
		ln, err := listenTCP("tcp_ingress", sess)
		if err != nil {
			h.logger.Errorln(err)
			return
		}
	*/

	lnArgs := &wnet.ListenerFromFactoryArgs{
		ID:       sess.ID(),
		BindHost: h.nodeID,
	}

	ln, err := h.lFactory.Listener(lnArgs)
	if err != nil {
		h.logger.Errorln(err)
		return
	}

	h.logger.Infof("Started session %s for %s (%s)", sess.ID(), sess.NodeID(), sess.Client())

	addr := ln.Addr()
	if multi, ok := addr.(wnet.MultiAddr); ok {
		for _, a := range multi.Addrs() {
			sess.AddEndpoint(a)
		}
	} else {
		sess.AddEndpoint(addr)
	}
	sess.ClusterURL = h.clusterURL
	for _, e := range sess.Endpoints() {
		h.logger.Infof("Session %s for %s (%s) listening on %s addr: %s", sess.ID(), sess.NodeID(), sess.Client(), e.Network(), e.String())
	}

	if err = sess.RegisterEndpoint(); err != nil {
		h.logger.Errorln("Error registering endpoint:", err)
		return
	}

	sess.HandleRequests(ln)
}

func (h *TCPHandler) closeSession(sess session.Session) {
	sess.Close()
	h.registry.RemoveSession(sess)
}
