// Package conn is based on [go-conntrack](https://github.com/mwitkow/go-conntrack)
package conn

import (
	"io"
	"net"
	"sync/atomic"

	"time"
)

const (
	defaultName = "default"
)

type listenerOpts struct {
	name         string
	labels       map[string]string
	monitoring   bool
	tcpKeepAlive time.Duration
	deadline     time.Duration
}

type listenerOpt func(*listenerOpts)

// TrackWithName sets the name of the Listener for use in tracking and monitoring.
func TrackWithName(name string) listenerOpt {
	return func(opts *listenerOpts) {
		opts.name = name
	}
}

// FIXME: if passed labels are not matching defaultPromLabels then wormhole will panic,
//        so we need to ensure that there is no listener that doesn't add all the required labels by accidents
// TrackWithLabels sets additional labels of the Listener for use in tracking in monitoring.
func TrackWithLabels(labels map[string]string) listenerOpt {
	return func(opts *listenerOpts) {
		opts.labels = labels
	}
}

// TrackWithoutMonitoring turns *off* Prometheus monitoring for this listener.
func TrackWithoutMonitoring() listenerOpt {
	return func(opts *listenerOpts) {
		opts.monitoring = false
	}
}

// TrackWithTCPKeepAlive makes sure that any `net.TCPConn` that get accepted have a keep-alive.
// This is useful for HTTP servers in order for, for example laptops, to not use up resources on the
// server while they don't utilise their connection.
// A value of 0 disables it.
func TrackWithTCPKeepAlive(keepalive time.Duration) listenerOpt {
	return func(opts *listenerOpts) {
		opts.tcpKeepAlive = keepalive
	}
}

// TrackWithDeadline makes sure that SetDeadline is being called for `net.TCPListener` before each `Accept` call
func TrackWithDeadline(deadline time.Duration) listenerOpt {
	return func(opts *listenerOpts) {
		opts.deadline = deadline
	}
}

type connTrackListener struct {
	net.Listener
	opts *listenerOpts
}

type connTrackTCPListener struct {
	Listener *net.TCPListener
	opts     *listenerOpts
}

// NewListener returns the given listener wrapped in connection tracking listener.
func NewListener(inner net.Listener, optFuncs ...listenerOpt) (net.Listener, error) {
	opts := &listenerOpts{
		name:       defaultName,
		monitoring: true,
	}
	for _, f := range optFuncs {
		f(opts)
	}
	if opts.monitoring {
		err := preRegisterListenerMetrics(opts.name, opts.labels)
		if err != nil {
			return nil, err
		}
	}
	return &connTrackListener{
		Listener: inner,
		opts:     opts,
	}, nil
}

// NewTCPListener returns the given listener wrapped in connection tracking listener.
func NewTCPListener(inner *net.TCPListener, optFuncs ...listenerOpt) net.Listener {
	opts := &listenerOpts{
		name:       defaultName,
		monitoring: true,
	}
	for _, f := range optFuncs {
		f(opts)
	}
	if opts.monitoring {
		preRegisterListenerMetrics(opts.name, opts.labels)
	}
	return &connTrackTCPListener{
		Listener: inner,
		opts:     opts,
	}
}

func (ct *connTrackListener) Accept() (net.Conn, error) {
	// TODO(mwitkow): Add monitoring of failed accept.
	conn, err := ct.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if tcpConn, ok := conn.(*net.TCPConn); ok && ct.opts.tcpKeepAlive > 0 {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(ct.opts.tcpKeepAlive)
	}
	return newServerConnTracker(conn, ct.opts), nil
}

func (ct *connTrackTCPListener) Accept() (net.Conn, error) {
	// TODO(mwitkow): Add monitoring of failed accept.
	if ct.opts.deadline > 0 {
		ct.Listener.SetDeadline(time.Now().Add(ct.opts.deadline))
	}
	conn, err := ct.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if tcpConn, ok := conn.(*net.TCPConn); ok && ct.opts.tcpKeepAlive > 0 {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(ct.opts.tcpKeepAlive)
	}
	return newServerConnTracker(conn, ct.opts), nil
}

func (ct *connTrackTCPListener) Addr() net.Addr {
	return ct.Listener.Addr()
}

func (ct *connTrackTCPListener) Close() error {
	return ct.Listener.Close()
}

type ServerConnTracker struct {
	net.Conn
	opts      *listenerOpts
	startedAt time.Time
	sentBytes int64
	rcvdBytes int64
}

func newServerConnTracker(inner net.Conn, opts *listenerOpts) net.Conn {

	tracker := &ServerConnTracker{
		Conn:      inner,
		opts:      opts,
		startedAt: time.Now(),
	}
	if opts.monitoring {
		reportListenerConnAccepted(opts.name, opts.labels)
	}
	return tracker
}

// ReadFrom calls TCPConn's ReadFrom and records number of bytes read from io.Reader r
// and written to TCPConn
// FIXME: since we don't have timeouts ReadFrom is blocking until the connection is severed
// by a call to Close, so sentBytes are not avaiable to MetricFunc. It's probably best to make
// utils.CopyCloseIO return sentBytes and rcvdBytes and hook in a metric collection func there.
func (ct *ServerConnTracker) ReadFrom(r io.Reader) (n int64, err error) {
	if tcpConn, ok := ct.Conn.(*net.TCPConn); ok {
		n, err = tcpConn.ReadFrom(r)
		atomic.AddInt64(&ct.sentBytes, n)
	}
	return
}

// Read calls TCPConn's Read and records number of bytes read.
func (ct *ServerConnTracker) Read(b []byte) (n int, err error) {
	n, err = ct.Conn.Read(b)
	atomic.AddInt64(&ct.rcvdBytes, int64(n))
	return
}

// Write calls TCPConn's Write and records number of bytes written.
func (ct *ServerConnTracker) Write(b []byte) (n int, err error) {
	n, err = ct.Conn.Write(b)
	atomic.AddInt64(&ct.sentBytes, int64(n))
	return
}

func (ct *ServerConnTracker) Close() error {
	err := ct.Conn.Close()
	if ct.opts.monitoring {
		reportListenerConnClosed(ct.opts.name, ct.opts.labels)
		reportConnDuration(ct.opts.name, ct.opts.labels, time.Since(ct.startedAt).Seconds())
		reportConnRcvdBytes(ct.opts.name, ct.opts.labels, float64(ct.rcvdBytes))
		reportConnSentBytes(ct.opts.name, ct.opts.labels, float64(ct.sentBytes))
	}
	return err
}

func (ct *ServerConnTracker) ReportDataMetrics(sentBytes, rcvdBytes int64) {
	reportConnRcvdBytes(ct.opts.name, ct.opts.labels, float64(rcvdBytes))
	reportConnSentBytes(ct.opts.name, ct.opts.labels, float64(sentBytes))
}
