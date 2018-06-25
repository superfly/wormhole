package remote

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/sirupsen/logrus"
	"github.com/superfly/wormhole/config"
	wnet "github.com/superfly/wormhole/net"
	"github.com/superfly/wormhole/session"
	"github.com/ulule/limiter"
	"golang.org/x/crypto/ssh"
)

const (
	sshRemoteForwardRequest      = "tcpip-forward"
	sshForwardedTCPReturnRequest = "forwarded-tcpip"
)

// SSHHandler type represents the handler that accepts incoming wormhole connections
type SSHHandler struct {
	config     *ssh.ServerConfig
	nodeID     string
	localhost  string
	clusterURL string
	registry   *session.Registry
	pool       *redis.Pool
	logger     *logrus.Entry
	limiter    *limiter.Limiter
	lFactory   wnet.ListenerFactory
}

// NewSSHHandler returns a new SSHHandler
func NewSSHHandler(cfg *config.ServerConfig, registry *session.Registry, pool *redis.Pool, factory wnet.ListenerFactory) (*SSHHandler, error) {
	rate, err := limiter.NewRateFromFormatted("30-M")
	if err != nil {
		return nil, fmt.Errorf("Couldn't create a rate limit for SSHHandler: %s", err.Error())
	}
	// use a in-memory store with a goroutine which clears expired keys every 30 seconds
	store := limiter.NewMemoryStore()

	limiterInstance := limiter.NewLimiter(store, rate)

	config, err := makeConfig(cfg.SSHPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("Couldn't create SSH Server Config: %s", err.Error())
	}

	s := SSHHandler{
		nodeID:     cfg.NodeID,
		registry:   registry,
		localhost:  cfg.Localhost,
		clusterURL: cfg.ClusterURL,
		pool:       pool,
		config:     config,
		logger:     cfg.Logger.WithFields(logrus.Fields{"prefix": "SSHHandler"}),
		limiter:    limiterInstance,
		lFactory:   factory,
	}
	return &s, nil
}

// Serve accepts incoming wormhole connections and passes them to the handler
func (s *SSHHandler) Serve(conn *net.TCPConn) {
	conn.RemoteAddr()
	ctx, err := s.limiter.Get(ipForConn(conn))
	if err != nil {
		s.logger.Errorln("error getting limiter info for conn:", err)
	}
	if ctx.Reached {
		s.logger.Errorf("Rate Limit (%d) reached for %s. Closing connection", ctx.Limit, conn.RemoteAddr().String())
		conn.Close()
		return
	}
	s.sshSessionHandler(conn)
}

func makeConfig(key []byte) (*ssh.ServerConfig, error) {
	config := &ssh.ServerConfig{}

	if private, err := ssh.ParsePrivateKey(key); err == nil {
		config.AddHostKey(private)
	} else {
		return nil, err
	}

	return config, nil
}

func (s *SSHHandler) sshSessionHandler(conn net.Conn) {
	// Before use, a handshake must be performed on the incoming net.Conn.
	sess := session.NewSSHSession(s.logger.Logger, s.clusterURL, s.nodeID, s.pool, conn, s.config)
	err := sess.RequireStream()
	if err != nil {
		s.logger.WithField("client_addr", conn.RemoteAddr().String()).Errorln("error getting a stream:", err)
		return
	}

	err = sess.RequireAuthentication()
	if err != nil {
		s.logger.Errorln(err)
		return
	}

	s.logger.Println("Client authenticated.")

	defer s.closeSession(sess)

	lnArgs := &wnet.ListenerFromFactoryArgs{
		ID:       sess.ID(),
		BindHost: s.nodeID,
	}

	ln, err := s.lFactory.Listener(lnArgs)
	if err != nil {
		s.logger.Errorln(err)
		return
	}

	s.logger.Infof("Started session %s for %s (%s)", sess.ID(), sess.NodeID(), sess.Client())

	addr := ln.Addr()
	if multi, ok := addr.(wnet.MultiAddr); ok {
		for _, a := range multi.Addrs() {
			sess.AddEndpoint(a)
		}
	} else {
		sess.AddEndpoint(addr)
	}
	sess.ClusterURL = s.clusterURL
	for _, e := range sess.Endpoints() {
		s.logger.Infof("Session %s for %s (%s) listening on %s addr: %s", sess.ID(), sess.NodeID(), sess.Client(), e.Network(), e.String())
	}

	if err = sess.RegisterEndpoint(); err != nil {
		s.logger.Errorln("Error registering endpoint:", err)
		return
	}

	s.registry.AddSession(sess)

	sess.HandleRequests(ln)
}

func (s *SSHHandler) closeSession(sess session.Session) {
	sess.Close()
	s.registry.RemoveSession(sess)
}

// Close closes all sessions handled by SSHandler
func (s *SSHHandler) Close() {
	s.lFactory.Close()
}

func listenTCP(name string, sess session.Session) (net.Listener, error) {
	addr, err := net.ResolveTCPAddr("tcp4", ":0")
	if err != nil {
		return nil, errors.New("could not parse TCP addr: " + err.Error())
	}
	ln, err := net.ListenTCP("tcp4", addr)
	if err != nil {
		return nil, errors.New("could not listen on: " + err.Error())
	}
	listener := wnet.NewTCPListener(ln, wnet.TrackWithName(name),
		wnet.TrackWithDeadline(1*time.Second),
		wnet.TrackWithLabels(map[string]string{"cluster": sess.Cluster(), "backend": sess.BackendID(), "node": sess.NodeID()}))
	return listener, nil
}

func ipForConn(conn net.Conn) string {
	addr := conn.RemoteAddr()
	host, _, _ := net.SplitHostPort(addr.String())
	return host
}
