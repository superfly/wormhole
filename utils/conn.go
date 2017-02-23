package utils

import (
	"io"
	"net"
	"sync/atomic"
)

// InstrumentedTCPConn is a wrapper around TCPConn that allows to trigger
// callback functions to record metrics about the conn.
type InstrumentedTCPConn struct {
	*net.TCPConn
	sentBytes  int64
	rcvdBytes  int64
	MetricFunc func(sentBytes, rcvdBytes int64)
}

// Close closes the connection and also triggers metricFunc if one is set
func (c *InstrumentedTCPConn) Close() error {
	if c.MetricFunc != nil {
		go c.MetricFunc(c.sentBytes, c.rcvdBytes)
	}
	return c.TCPConn.Close()
}

// ReadFrom calls TCPConn's ReadFrom and records number of bytes read from io.Reader r
// and written to TCPConn
// FIXME: since we don't have timeouts ReadFrom is blocking until the connection is severed
// by a call to Close, so sentBytes are not avaiable to MetricFunc. It's probably best to make
// utils.CopyCloseIO return sentBytes and rcvdBytes and hook in a metric collection func there.
func (c *InstrumentedTCPConn) ReadFrom(r io.Reader) (n int64, err error) {
	n, err = c.TCPConn.ReadFrom(r)
	atomic.AddInt64(&c.sentBytes, n)
	return
}

// Read calls TCPConn's Read and records number of bytes read.
func (c *InstrumentedTCPConn) Read(b []byte) (n int, err error) {
	n, err = c.TCPConn.Read(b)
	// fmt.Printf("Read: %d\n bytes", n)
	atomic.AddInt64(&c.rcvdBytes, int64(n))
	return
}

// Write calls TCPConn's Write and records number of bytes written.
func (c *InstrumentedTCPConn) Write(b []byte) (n int, err error) {
	n, err = c.TCPConn.Write(b)
	atomic.AddInt64(&c.sentBytes, int64(n))
	return
}
