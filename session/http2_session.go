package session

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/rs/xid"
	"github.com/sirupsen/logrus"
	"github.com/superfly/wormhole/messages"
	wnet "github.com/superfly/wormhole/net"

	"golang.org/x/net/http2"
)

// HTTP2Session extends information about connected client stored in Session.
// It also includes:
// - control connection for exchanging communication with the client
// - channel with available tunnel connections
// - timestamp with the last known ping from the client
type HTTP2Session struct {
	baseSession

	control   net.Conn
	conns     wnet.ConnPool
	server    *http.Server
	transport *http2.Transport

	lastPingAt int64
}

// HTTP2SessionArgs defines the arguments to be passed to NewHTTP2Session
type HTTP2SessionArgs struct {
	Logger    *logrus.Logger
	NodeID    string
	TLSConfig *tls.Config
	RedisPool *redis.Pool
	Conn      net.Conn
}

// NewHTTP2Session creates new TCPSession struct
func NewHTTP2Session(args *HTTP2SessionArgs) (*HTTP2Session, error) {
	base := baseSession{
		id:     xid.New().String(),
		nodeID: args.NodeID,
		store:  NewRedisStore(args.RedisPool),
		logger: args.Logger.WithFields(logrus.Fields{"prefix": "HTTP2Session"}),
	}
	s := &HTTP2Session{
		control:     args.Conn,
		baseSession: base,
		transport:   &http2.Transport{},
		lastPingAt:  time.Now().UnixNano(),
	}

	server := &http.Server{
		Handler:   s,
		TLSConfig: args.TLSConfig.Clone(), // Currently doesn't do anything since we listen with tcp
	}

	if err := http2.ConfigureServer(server, &http2.Server{}); err != nil {
		return nil, err
	}

	s.server = server

	pool, err := wnet.NewConnPool(
		args.Logger.WithFields(logrus.Fields{"prefix": "HTTP2ConnPool"}),
		10,
		[]wnet.ConnPoolObject{})
	if err != nil {
		return nil, err
	}
	s.conns = pool

	return s, nil
}

type http2Tunnel struct {
	conn        *tls.Conn
	cc          *http2.ClientConn
	cStreams    uint32
	maxCStreams uint32

	valC chan int
}

func (c *http2Tunnel) Close() error {
	return c.conn.Close()
}

func (c *http2Tunnel) ShouldDelete() bool {
	return !c.cc.CanTakeNewRequest()
}

func (c *http2Tunnel) Value() <-chan int {
	return c.valC
}

func (c *http2Tunnel) incrementStreamCount() {
	atomic.AddUint32(&c.cStreams, 1)
}

func (c *http2Tunnel) decrementStreamCount() {
	atomic.AddUint32(&c.cStreams, ^uint32(0))
}

func (c *http2Tunnel) rewriteRequest(r *http.Request) {
	r.URL.Host = c.conn.RemoteAddr().String()
	r.URL.Scheme = "https"
	r.Host = c.conn.RemoteAddr().String()
	r.RequestURI = ""
}

func (c *http2Tunnel) updateValue() {
	c.valC <- int(atomic.LoadUint32(&c.cStreams))
}

func (c *http2Tunnel) RoundTrip(r *http.Request) (*http.Response, error) {
	c.rewriteRequest(r)

	c.incrementStreamCount()
	defer c.decrementStreamCount()

	go c.updateValue()

	return c.cc.RoundTrip(r)
}

// AddTunnel adds a connection to the pool of tunnel connections
func (s *HTTP2Session) AddTunnel(conn *tls.Conn) error {
	cc, err := s.transport.NewClientConn(conn)
	if err != nil {
		return err
	}

	poolObj := &http2Tunnel{
		conn:        conn,
		cc:          cc,
		cStreams:    0,
		maxCStreams: 10,
		valC:        make(chan int, 1),
	}

	// set initial value to 0
	poolObj.valC <- 0
	ok, err := s.conns.Insert(poolObj)
	if err != nil {
		return err
	}
	if !ok {
		s.logger.Warn("Connection pool is full while trying to add ClientConn")
	}

	return nil
}

// RequireStream sends a request to the client to open a new tunnel Connection
// for this Session.
func (s *HTTP2Session) RequireStream() error {
	return s.openTunnel()
}

// HandleRequests handles all requests coming over the control connection from the client.
// The main function is to accept ingress traffic (from the listener) once the remote port
// forwarding is set up.
// It also handles out-of-band communication, like the maintaining the Session heartbeat or
// request the client to open new tunnel connections.
func (s *HTTP2Session) HandleRequests(ln net.Listener) {
	go s.controlLoop()
	go s.heartbeat()
	s.handleRemoteForward(ln)
}

// RequireAuthentication registers the connection
// TODO: add authentication here
func (s *HTTP2Session) RequireAuthentication() error {
	s.store.RegisterConnection(s)
	return nil
}

// RegisterEndpoint registers the endpoint and adds it to the current session record
// The endpoint is a particular instance of a running wormhole client
func (s *HTTP2Session) RegisterEndpoint() error {
	return s.store.RegisterEndpoint(s)
}

// Close closes SSHSession and registers disconnection
func (s *HTTP2Session) Close() {
	s.store.RegisterDisconnection(s)
	s.logger.Infof("Closed session %s for %s %s (%s).", s.ID(), s.NodeID(), s.Agent(), s.Client())
	s.server.Close()
	s.control.Close()
}

// handleRemoteForward listens for TLS connection and connects it to a session
// NOTE: http2 in golang REQUIRES tls. No h2c spec supported
// TODO: instead of manually listening for TCP conns - just listen via a http2 server
// TODO: Currently only handles TCP not TLS
func (s *HTTP2Session) handleRemoteForward(ln net.Listener) {
	defer func() {
		s.logger.Debugf("Closed ingress conn: %s", ln.Addr().String())
	}()

	if err := s.server.Serve(ln); err != nil {
		if err != http.ErrServerClosed {
			err := ln.Close()
			if err != nil {
				s.logger.Debugf("Couldn't close listener: %s", err)
				return
			}
		}
		s.logger.Errorf("Stopped being able to serve ingress traffic: %v+", err)
		return
	}
}

// ServeHTTP...
func (s *HTTP2Session) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var resp *http.Response
	var err error
	for {
		obj := s.conns.Get()
		defer obj.Done()

		conn, ok := obj.ConnPoolObject().(*http2Tunnel)
		if !ok {
			s.logger.Error("Got wrong object type from connection pool")
			return
		}

		// HTTP2 doesn't support these header types, so delete them
		// This is if the end client doesn't support HTTP2
		// Taken from go core at net/http2/transport.go
		if v := r.Header.Get("Upgrade"); v != "" {
			r.Header.Del("Upgrade")
		}
		if v := r.Header.Get("Transfer-Encoding"); (v != "" && v != "chunked") || len(r.Header["Transfer-Encoding"]) > 1 {
			r.Header.Del("Transfer-Encoding")
		}
		if v := r.Header.Get("Connection"); (v != "" && v != "close" && v != "keep-alive") || len(r.Header["Connection"]) > 1 {
			r.Header.Del("Connection")
		}

		resp, err = conn.RoundTrip(r)
		// TODO: Handle this error
		if err != nil {
			if conn.ShouldDelete() {
				continue
			}
			s.logger.Warnf("Error with requiest: %+v", err)
			if netErr, ok := err.(net.Error); !ok {
				// do something not network error related
				// maybe retry?
				_ = netErr
			}
			break
		}
		break
	}

	// Delete this so we don't copy it over
	// Will be handled by http.ResponseWriter
	resp.Header.Del("Content-Length")

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)

	defer resp.Body.Close()

	nr, err := io.Copy(w, resp.Body)
	if err != nil {
		s.logger.Errorf("Could not copy response body")
		return
	}
	s.logger.Infof("Copied %d bytes between connection bodies", nr)
}

func (s *HTTP2Session) openTunnel() error {
	msg := &messages.OpenTunnel{ClientID: s.id}
	b, err := messages.Pack(msg)
	if err != nil {
		return fmt.Errorf("Couldn't create a request to open new tunnel: %s", err.Error())
	}
	_, err = s.control.Write(b)
	if err != nil {
		return fmt.Errorf("Failed to send request to open new tunnel: %s", err.Error())
	}
	return nil
}

func (s *HTTP2Session) heartbeat() {
	// timer for detecting heartbeat failure
	connCheck := time.NewTicker(connCheckInterval)
	defer connCheck.Stop()

	for {
		select {
		case <-connCheck.C:
			lastPing := time.Unix(0, atomic.LoadInt64(&s.lastPingAt))
			if time.Since(lastPing) > pingTimeoutInterval {
				s.Close()
				return
			}
		}
	}
}

func (s *HTTP2Session) controlLoop() {
	b := make([]byte, 1024)

	for {
		nr, err := s.control.Read(b)
		if err == io.EOF {
			continue
		}
		if err != nil {
			s.logger.Errorf("error reading from control: " + err.Error())
			s.Close()
			return
		}
		msg, err := messages.Unpack(b[:nr])
		if err != nil {
			s.logger.Errorf("error parsing message from stream: " + err.Error())
			s.Close()
			return
		}
		switch m := msg.(type) {
		case *messages.Shutdown:
			s.logger.Debugf("Received Shutdown message: %s", m.Error)
			s.Close()
			return
		case *messages.Ping:
			s.logger.Debug("Received Ping message.")
			atomic.StoreInt64(&s.lastPingAt, time.Now().UnixNano())
			bw, err := messages.Pack(&messages.Pong{})
			if err != nil {
				s.logger.Errorf("Couldn't create a Pong message: %s", err.Error())
			}
			_, err = s.control.Write(bw)
			if err != nil {
				s.logger.Errorf("Failed to send Pong message: %s", err.Error())
			}
		default:
			s.logger.Warn("Unrecognized command. Ignoring.")
		}
	}
}
