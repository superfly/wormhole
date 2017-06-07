// Package net is based on [go-conntrack](https://github.com/mwitkow/go-conntrack)
package net

import (
	"io"
	"net"

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

// ListenerOpt wraps listenerOpts in a func as convenience method to provide friendly API
// to configure connTrackListener
type ListenerOpt func(*listenerOpts)

// TrackWithName sets the name of the Listener for use in tracking and monitoring.
func TrackWithName(name string) ListenerOpt {
	return func(opts *listenerOpts) {
		opts.name = name
	}
}

// TrackWithLabels sets additional labels of the Listener for use in tracking in monitoring.
func TrackWithLabels(labels map[string]string) ListenerOpt {
	return func(opts *listenerOpts) {
		opts.labels = labels
	}
}

// TrackWithoutMonitoring turns *off* Prometheus monitoring for this listener.
func TrackWithoutMonitoring() ListenerOpt {
	return func(opts *listenerOpts) {
		opts.monitoring = false
	}
}

// TrackWithTCPKeepAlive makes sure that any `net.TCPConn` that get accepted have a keep-alive.
// This is useful for HTTP servers in order for, for example laptops, to not use up resources on the
// server while they don't utilise their connection.
// A value of 0 disables it.
func TrackWithTCPKeepAlive(keepalive time.Duration) ListenerOpt {
	return func(opts *listenerOpts) {
		opts.tcpKeepAlive = keepalive
	}
}

// TrackWithDeadline makes sure that SetDeadline is being called for `net.TCPListener` before each `Accept` call
func TrackWithDeadline(deadline time.Duration) ListenerOpt {
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
func NewListener(inner net.Listener, optFuncs ...ListenerOpt) (net.Listener, error) {
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
func NewTCPListener(inner *net.TCPListener, optFuncs ...ListenerOpt) net.Listener {
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

// Accept implements the Accept method in the Listener interface;
// it waits for the next call and returns a ServerConnTracker.
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

// Accept implements the Accept method in the Listener interface;
// it waits for the next call and returns a ServerConnTracker.
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

// ServerConnTracker is a wrapper around Net.Conn that tracks when connection is opened,
// closed and the duration of the connection
type ServerConnTracker struct {
	net.Conn
	opts      *listenerOpts
	startedAt time.Time
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

// ReadFrom delegates to TCPConn's ReadFrom
func (ct *ServerConnTracker) ReadFrom(r io.Reader) (n int64, err error) {
	if tcpConn, ok := ct.Conn.(*net.TCPConn); ok {
		n, err = tcpConn.ReadFrom(r)
	}
	return
}

// Read delegates to TCPConn's Read
func (ct *ServerConnTracker) Read(b []byte) (n int, err error) {
	return ct.Conn.Read(b)
}

// Write delegates to TCPConn's Write
func (ct *ServerConnTracker) Write(b []byte) (n int, err error) {
	return ct.Conn.Write(b)
}

// Close closes the connection and records metrics
func (ct *ServerConnTracker) Close() error {
	err := ct.Conn.Close()
	if ct.opts.monitoring {
		reportListenerConnClosed(ct.opts.name, ct.opts.labels)
		reportConnDuration(ct.opts.name, ct.opts.labels, time.Since(ct.startedAt).Seconds())
	}
	return err
}

// ReportDataMetrics reports bytes sent and received over the course of the duration of net.Conn
func (ct *ServerConnTracker) ReportDataMetrics(sentBytes, rcvdBytes int64) {
	reportConnRcvdBytes(ct.opts.name, ct.opts.labels, float64(rcvdBytes))
	reportConnSentBytes(ct.opts.name, ct.opts.labels, float64(sentBytes))
}
