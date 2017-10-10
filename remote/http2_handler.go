package remote

import (
	"crypto/tls"
	"io"
	"net"

	"github.com/garyburd/redigo/redis"
	"github.com/sirupsen/logrus"
	"github.com/superfly/wormhole/config"
	"github.com/superfly/wormhole/messages"
	wnet "github.com/superfly/wormhole/net"
	"github.com/superfly/wormhole/session"
)

// HTTP2Handler type represents the handler that accepts incoming wormhole connections
type HTTP2Handler struct {
	nodeID     string
	localhost  string
	clusterURL string
	sessions   map[string]session.Session
	pool       *redis.Pool
	logger     *logrus.Entry
	tlsConfig  *tls.Config
}

// NewHTTP2Handler ...
func NewHTTP2Handler(cfg *config.ServerConfig, pool *redis.Pool) (*HTTP2Handler, error) {
	h := HTTP2Handler{
		nodeID:     cfg.NodeID,
		sessions:   make(map[string]session.Session),
		localhost:  cfg.Localhost,
		clusterURL: cfg.ClusterURL,
		pool:       pool,
		logger:     cfg.Logger.WithFields(logrus.Fields{"prefix": "HTTP2Handler"}),
	}

	crt, err := tls.X509KeyPair(cfg.TLSCert, cfg.TLSPrivateKey)
	if err != nil {
		return nil, err
	}
	h.tlsConfig = &tls.Config{
		Certificates: []tls.Certificate{crt},
	}
	return &h, nil
}

// Serve accepts incoming wormhole connections and passes them to the handler
// We are explicit with the *net.TCPConn since we need to be this way - and let the handler and
// sessions handle wrapping in TLS. Having a TCPConn all the way down will highlight the dangers
// of sending data over the socket without first wrapping in TLS
func (h *HTTP2Handler) Serve(conn *net.TCPConn) {
	tlsConn, err := h.genericTLSWrap(conn)
	if err != nil {
		h.logger.Errorf("error establishing tls session: " + err.Error())
		return
	}

	buf := make([]byte, 1024)

	waitClose := true
	nr, err := tlsConn.Read(buf)
	if err != nil {
		// optimize for closing of initial TLS conn
		if err == io.EOF {
			waitClose = false
		} else {
			h.logger.Errorf("error reading from stream: " + err.Error())
			return
		}
	}
	msg, err := messages.Unpack(buf[:nr])
	if err != nil {
		h.logger.Errorf("error parsing message from stream: " + err.Error())
		return
	}

	switch m := msg.(type) {
	case *messages.AuthControl:
		go h.http2SessionHandler(tlsConn)
	case *messages.AuthTunnel:
		if sess, ok := h.sessions[m.ClientID]; !ok {
			h.logger.Error("New tunnel conn not associated with any session. Closing")
			tlsConn.Close()
		} else {
			// open a proxy conn on current session
			h.logger.Debugf("Adding New tunnel conn to session: %s", sess.ID())
			http2Sess := sess.(*session.HTTP2Session)

			for waitClose {
				if _, err := tlsConn.Read(buf); err != nil {
					if err != io.EOF {
						h.logger.Errorf("Failed to get TLS Closed: %s", err.Error())
						return
					}
					waitClose = false
				}
			}

			if err := tlsConn.CloseWrite(); err != nil {
				h.logger.Errorf("failed to close tls conn: %s", err.Error())
			}
			alpnConn, err := h.http2ALPNTLSWrap(conn)
			if err != nil {
				h.logger.Errorf("Couldn't establish ALPN connection")
				return
			}

			if err := http2Sess.AddTunnel(alpnConn); err != nil {
				h.logger.Errorf("Error establishing Tunnel: %v+", err)
			}
			h.logger.Debugf("Successfully Added New tunnel conn to session: %s", sess.ID())
		}
	default:
		h.logger.Error("unparsable response")
		tlsConn.Close()
	}
}

func (h *HTTP2Handler) genericTLSWrap(conn *net.TCPConn) (*tls.Conn, error) {
	return wnet.GenericTLSWrap(conn, h.tlsConfig, tls.Server)
}

// NOTE: The ALPN is a requirement of the spec for HTTP/2 capability discovery
// While technically the golang implementation will allow us not to perform ALPN,
// this breaks the http/2 spec. The goal here is to follow the RFC to the letter
// as documented in http://httpwg.org/specs/rfc7540.html#starting
func (h *HTTP2Handler) http2ALPNTLSWrap(conn *net.TCPConn) (*tls.Conn, error) {
	return wnet.HTTP2ALPNTLSWrap(conn, h.tlsConfig, tls.Server)
}

// Close closes all sessions handled by HTTP2Handler
func (h *HTTP2Handler) Close() {
	for _, sess := range h.sessions {
		sess.Close()
		delete(h.sessions, sess.ID())
	}
}

func (h *HTTP2Handler) http2SessionHandler(conn net.Conn) {
	args := &session.HTTP2SessionArgs{
		Logger:    h.logger.Logger,
		NodeID:    h.nodeID,
		RedisPool: h.pool,
		Conn:      conn,
		TLSConfig: h.tlsConfig,
	}

	sess, err := session.NewHTTP2Session(args)
	if err != nil {
		h.logger.WithField("client_addr", conn.RemoteAddr().String()).Errorln("error creating a session:", err)
		return
	}
	h.sessions[sess.ID()] = sess

	if err := sess.RequireStream(); err != nil {
		h.logger.WithField("client_addr", conn.RemoteAddr().String()).Errorln("error getting a stream:", err)
		return
	}

	if err := sess.RequireAuthentication(); err != nil {
		h.logger.Errorln(err)
		return
	}

	defer h.closeSession(sess)

	ln, err := listenTCP("tcp_ingress", sess)
	if err != nil {
		h.logger.Errorln(err)
		return
	}

	_, port, _ := net.SplitHostPort(ln.Addr().String())
	sess.EndpointAddr = h.localhost + ":" + port
	sess.ClusterURL = h.clusterURL

	if err := sess.RegisterEndpoint(); err != nil {
		h.logger.Errorln("Error registering endpoint:", err)
		return
	}

	h.logger.Infof("Started session %s for %s (%s). Listening on: %s", sess.ID(), sess.NodeID(), sess.Client(), sess.Endpoint())

	sess.HandleRequests(ln)
}

func (h *HTTP2Handler) closeSession(sess session.Session) {
	sess.Close()
	delete(h.sessions, sess.ID())
}
