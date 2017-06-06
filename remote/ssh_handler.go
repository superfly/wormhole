package remote

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
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
	sessions   map[string]session.Session
	pool       *redis.Pool
	logger     *logrus.Entry
	limiter    *limiter.Limiter

	mu sync.Mutex
}

// NewSSHHandler returns a new SSHHandler
func NewSSHHandler(cfg *config.ServerConfig, pool *redis.Pool) (*SSHHandler, error) {
	rate, err := limiter.NewRateFromFormatted("240-H")
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
		sessions:   make(map[string]session.Session),
		localhost:  cfg.Localhost,
		clusterURL: cfg.ClusterURL,
		pool:       pool,
		config:     config,
		logger:     cfg.Logger.WithFields(logrus.Fields{"prefix": "SSHHandler"}),
		limiter:    limiterInstance,
	}
	return &s, nil
}

// Serve accepts incoming wormhole connections and passes them to the handler
func (s *SSHHandler) Serve(conn net.Conn) {
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

func (s *SSHHandler) setSession(sess session.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.ID()] = sess
}

func (s *SSHHandler) sshSessionHandler(conn net.Conn) {
	// Before use, a handshake must be performed on the incoming net.Conn.
	sess := session.NewSSHSession(s.logger.Logger, s.clusterURL, s.nodeID, s.pool, conn, s.config)
	s.setSession(sess)
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

	ln, err := listenTCP("ssh_ingress", sess)
	if err != nil {
		s.logger.Errorln(err)
		return
	}

	_, port, _ := net.SplitHostPort(ln.Addr().String())
	sess.EndpointAddr = s.localhost + ":" + port

	if err = sess.RegisterEndpoint(); err != nil {
		s.logger.Errorln("Error registering endpoint:", err)
		return
	}

	s.logger.Infof("Started session %s for %s %s (%s). Listening on: %s", sess.ID(), sess.NodeID(), sess.Agent(), sess.Client(), sess.Endpoint())

	sess.HandleRequests(ln)
}

func (s *SSHHandler) closeSession(sess session.Session) {
	sess.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sess.ID())
}

// Close closes all sessions handled by SSHandler
func (s *SSHHandler) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, sess := range s.sessions {
		sess.Close()
		delete(s.sessions, sess.ID())
	}
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
