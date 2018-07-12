package session

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"

	"github.com/gomodule/redigo/redis"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/xid"
	"github.com/sirupsen/logrus"
	"github.com/oknoah/wormhole/messages"
	"golang.org/x/crypto/ssh"

	wnet "github.com/oknoah/wormhole/net"
)

const (
	sshRemoteForwardRequest      = "tcpip-forward"
	sshForwardedTCPReturnRequest = "forwarded-tcpip"
)

var (
	openSessionsMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "wormhole",
			Subsystem: "ssh_session",
			Name:      "open_sessions_total",
			Help:      "Number of active sessions, partitioned by backend, node and cluster.",
		},
		[]string{
			// Which backend this session belongs to?
			"backend",
			// What wormhole instance this session is running on?
			"node",
			// What region this session belongs to?
			"cluster",
		},
	)

	openChannelsMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "wormhole",
			Subsystem: "ssh_session",
			Name:      "open_channels_total",
			Help:      "Number of active channels, partitioned by backend, node and cluster.",
		},
		[]string{
			// Which backend this channel belongs to?
			"backend",
			// What wormhole instance this session is running on?
			"node",
			// What region this session belongs to?
			"cluster",
		},
	)

	ingressConnDurationMetric = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  "wormhole",
			Subsystem:  "ssh_session",
			Name:       "ingress_conn_duration_seconds",
			Help:       "Duration in seconds of ingress connections, paritioned by backend, node and cluster.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{
			// Which backend this channel belongs to?
			"backend",
			// What wormhole instance this session is running on?
			"node",
			// What region this session belongs to?
			"cluster",
		},
	)

	ingressConnRcvdBytesMetric = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "wormhole",
			Subsystem: "ssh_session",
			Name:      "ingress_conn_rcvd_bytes",
			Help:      "Number of bytes received from ingress connections, paritioned by backend, node and cluster.",
		},
		[]string{
			// Which backend this channel belongs to?
			"backend",
			// What wormhole instance this session is running on?
			"node",
			// What region this session belongs to?
			"cluster",
		},
	)

	ingressConnSentBytesMetric = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "wormhole",
			Subsystem: "ssh_session",
			Name:      "ingress_conn_sent_bytes",
			Help:      "Number of bytes sent to ingress connections, paritioned by backend, node and cluster.",
		},
		[]string{
			// Which backend this channel belongs to?
			"backend",
			// What wormhole instance this session is running on?
			"node",
			// What region this session belongs to?
			"cluster",
		},
	)
)

func init() {
	prometheus.MustRegister(openSessionsMetric)
	prometheus.MustRegister(openChannelsMetric)
	prometheus.MustRegister(ingressConnDurationMetric)
	prometheus.MustRegister(ingressConnRcvdBytesMetric)
	prometheus.MustRegister(ingressConnSentBytesMetric)
}

// SSHSession extends information about connected client stored in Session.
// It also includes SSH-specific information like the SSH conn, SSH server config, etc.
type SSHSession struct {
	baseSession

	sshSessionID string
	config       *ssh.ServerConfig
	tcpConn      net.Conn
	conn         *ssh.ServerConn
	reqs         <-chan *ssh.Request
	chans        <-chan ssh.NewChannel
}

type tcpipForward struct {
	Host string
	Port uint32
}

type directForward struct {
	Host1 string
	Port1 uint32
	Host2 string
	Port2 uint32
}

type instrumentedIO struct {
	rwc        io.ReadWriteCloser
	metricFunc func()
}

func (io instrumentedIO) Close() error {
	go io.metricFunc()
	return io.rwc.Close()
}

func (io instrumentedIO) Read(p []byte) (int, error) {
	return io.rwc.Read(p)
}

func (io instrumentedIO) Write(p []byte) (int, error) {
	return io.rwc.Write(p)
}

// NewSSHSession creates new SshSession struct
func NewSSHSession(logger *logrus.Logger, clusterURL, nodeID string, redisPool *redis.Pool, tcpConn net.Conn, config *ssh.ServerConfig) *SSHSession {
	base := baseSession{
		id:         xid.New().String(),
		nodeID:     nodeID,
		store:      NewRedisStore(redisPool),
		ClusterURL: clusterURL,
		logger:     logger.WithFields(logrus.Fields{"prefix": "SSHSession"}),
	}
	s := &SSHSession{
		tcpConn:     tcpConn,
		baseSession: base,
	}
	config.PasswordCallback = s.authFromToken
	s.config = config
	return s
}

// RequireStream performs SSH handshake and ensures SSHSession is ready to receive
// and send data
func (s *SSHSession) RequireStream() error {
	// Before use, a handshake must be performed on the incoming net.Conn.
	sshConn, chans, reqs, err := ssh.NewServerConn(s.tcpConn, s.config)
	if err != nil {
		return err
	}
	s.conn = sshConn
	s.chans = chans
	s.reqs = reqs
	go handleChannels(chans)
	go openSessionsMetric.With(labels(s)).Add(1)
	return nil
}

// HandleRequests handles all requests coming over the SSH connection from the client.
// The main function is to accept ingress traffic (from the listener) once the remote port
// forwarding is set up.
// It also handles out-of-band SSH request types, like the keepalive or register-release.
func (s *SSHSession) HandleRequests(ln net.Listener) {
	for req := range s.reqs {
		switch req.Type {
		case sshRemoteForwardRequest:
			go func() {
				s.handleRemoteForward(req, ln)
			}()
		case "register-release":
			go s.registerRelease(req)
		case "keepalive":
			go s.handleKeepalive(req)
		}
	}
}

// RequireAuthentication registers the connection, since authentication is part of the SSH handshake
// TODO: figure out a better interface for Session
func (s *SSHSession) RequireAuthentication() error {
	// done as a hook to ssh handshake
	go s.store.RegisterConnection(s)
	return nil
}

// RegisterEndpoint registers the endpoint and adds it to the current session record
// The endpoint is a particular instance of a running wormhole client
func (s *SSHSession) RegisterEndpoint() error {
	return s.store.RegisterEndpoint(s)
}

// Close closes SSHSession and registers disconnection
func (s *SSHSession) Close() {
	s.store.RegisterDisconnection(s)
	s.logger.Infof("Closed session %s for %s %s (%s).", s.ID(), s.NodeID(), s.Agent(), s.Client())
	go func() {
		openSessionsMetric.With(labels(s)).Sub(1)
	}()
	s.conn.Close()
}

func (s *SSHSession) authFromToken(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
	backendID, err := s.store.BackendIDFromToken(strings.TrimSpace(string(pass)))
	if err != nil && err != redis.ErrNil {
		return nil, err
	}
	if backendID == "" {
		return nil, errors.New("token '" + string(pass) + "' rejected")
	}

	// assume false if not set
	requiresClientAuth, err := s.store.BackendRequiresClientAuth(backendID)
	if err != nil {
		return nil, err
	}

	s.backendID = backendID
	s.agent = string(c.ClientVersion())
	s.sshSessionID = hex.EncodeToString(c.SessionID())
	s.requiresClientAuth = requiresClientAuth
	s.clientAddr = c.RemoteAddr().String()

	return nil, nil
}

func (s *SSHSession) setSSHPort(req *ssh.Request, ln net.Listener) tcpipForward {
	t := tcpipForward{}
	ssh.Unmarshal(req.Payload, &t)

	reply := (t.Port == 0) && req.WantReply

	if reply { // Client sent port 0. let them know which port is actually being used
		_, port, _ := net.SplitHostPort(ln.Addr().String())
		portNum, _ := strconv.Atoi(port)
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, uint32(portNum))
		t.Port = uint32(portNum)
		req.Reply(true, b)
	} else {
		req.Reply(true, nil)
	}

	return t
}

func (s *SSHSession) handleRemoteForward(req *ssh.Request, ln net.Listener) {
	defer func() {
		err := ln.Close()
		if err != nil {
			s.logger.Debugf("Couldn't close ingress conn: %s", err)
			return
		}
		s.logger.Debugf("Closed ingress conn: %s", ln.Addr().String())
	}()

	t := s.setSSHPort(req, ln)
	p := directForward{}
	quit := make(chan bool)
	go func(ln net.Listener) { // Handle incoming connections on this new listener
		for {
			select {
			case <-quit:
				return
			default:
				ingressConn, err := ln.Accept()

				if err != nil {
					netErr, ok := err.(net.Error)

					//If this is a timeout, then continue to wait for
					//new connections
					if ok && netErr.Timeout() && netErr.Temporary() {
						continue
					}
					s.logger.Errorln("Could not accept Ingress TCP conn:", err)
					return
				}
				s.logger.Debugln("Accepted Ingress TCP conn from:", ingressConn.RemoteAddr())

				host, port, err := net.SplitHostPort(ingressConn.RemoteAddr().String())
				if err != nil {
					return
				}
				portnum, err := strconv.Atoi(port)
				if err != nil {
					return
				}

				p.Host1 = t.Host
				p.Port1 = t.Port
				p.Host2 = host
				p.Port2 = uint32(portnum)

				ch, reqs, err := s.conn.OpenChannel(sshForwardedTCPReturnRequest, ssh.Marshal(p))
				if err != nil {
					s.logger.Errorf("Open forwarded Channel error: %s", err.Error())
					return
				}
				go ssh.DiscardRequests(reqs)
				go openChannelsMetric.With(labels(s)).Add(1)
				go func() {
					chWritten, ingressConnWritten, err := wnet.CopyCloseIO(ch, ingressConn)
					openChannelsMetric.With(labels(s)).Sub(1)
					if connWithMetrics, ok := ingressConn.(*wnet.ServerConnTracker); ok {
						connWithMetrics.ReportDataMetrics(ingressConnWritten, chWritten)
					}
					if err != nil && err != io.EOF {
						s.logger.Error(err)
					}
				}()
			}
		}
	}(ln)

	s.conn.Wait()
	quit <- true
}

func handleChannels(chans <-chan ssh.NewChannel) {
	for range chans {
		// nothing for now.
	}
}

func (s *SSHSession) handleKeepalive(req *ssh.Request) {
	if req.WantReply {
		req.Reply(true, nil)
	}
	go func() {
		if err := s.store.RegisterHeartbeat(s); err != nil {
			s.logger.Warnf("Failed to register session heartbeat: %s", err.Error())
		}
	}()
}

func (s *SSHSession) registerRelease(req *ssh.Request) {
	if req.WantReply {
		req.Reply(true, nil)
	}

	msg, err := messages.Unpack(req.Payload)

	if err != nil {
		s.logger.Warnf("Couldn't process release info: %s", err.Error())
		return
	}

	if release, ok := msg.(*messages.Release); ok {
		s.release = release
		s.store.RegisterRelease(s)
	} else {
		s.logger.Warnf("Couldn't process release info: Unexpected message type")
	}
}

func labels(s Session) prometheus.Labels {
	return prometheus.Labels{"backend": s.BackendID(),
		"node":    s.NodeID(),
		"cluster": s.Cluster(),
	}
}
