package net

import (
	"crypto/tls"
	"fmt"
	"golang.org/x/net/http2"
	"io"
	"net"
	"reflect"
	"strings"
	"time"
)

// CopyDirection describes the direction of data copying in full-duplex link
type CopyDirection int

const (
	// LConnWrite means io.Copy writing to lconn
	LConnWrite = iota
	// RConnWrite means io.Copy writing to rconn
	RConnWrite
)

type copyStatus struct {
	n         int64
	err       error
	direction CopyDirection
}

type multiError struct {
	errs []error
}

func (me *multiError) Append(err error) {
	me.errs = append(me.errs, err)
}

func (me multiError) Error() string {
	var errStr string
	for i, err := range me.errs {
		errStr += err.Error()
		if i < len(me.errs)-1 {
			errStr += ", "
		}
	}
	return errStr
}

// TLSWrapperFunc represents a TLS Wrapper. This is intended to be either
// tls.Client or tls.Server see https://golang.org/pkg/crypto/tls/ for info
type TLSWrapperFunc func(conn net.Conn, cfg *tls.Config) *tls.Conn

// GenericTLSWrap takes a TCP connection, a tls config, and an upgrade function
// and returns the new connection
func GenericTLSWrap(conn *net.TCPConn, cfg *tls.Config, tFunc TLSWrapperFunc) (*tls.Conn, error) {
	var tConn *tls.Conn

	tCfg := cfg.Clone()
	tCfg.MinVersion = tls.VersionTLS12

	for {
		if err := conn.SetDeadline(time.Now().Add(time.Second * 10)); err != nil {
			return nil, err
		}

		tConn = tFunc(conn, tCfg)

		// check if the connection is upgraded before returning
		// we want to catch the error early
		if err := tConn.Handshake(); err != nil {
			if netErr, ok := err.(net.Error); ok {
				if netErr.Timeout() || netErr.Temporary() {
					continue
				}
			}
			return nil, err
		}
		if err := conn.SetDeadline(time.Time{}); err != nil {
			return nil, err
		}
		break
	}

	return tConn, nil
}

// HTTP2ALPNTLSWrap returns a TLS connection that has been negotiated with `h2` ALPN
// tFunc must be either tls.Client or tls.Server. See https://golang.org/pkg/crypto/tls/
// for proper usage of the tls.Config with either of these options
//
// NOTE: The ALPN is a requirement of the spec for HTTP/2 capability discovery
// While technically the golang implementation will allow us not to perform ALPN,
// this breaks the http/2 spec. The goal here is to follow the RFC to the letter
// as documented in http://httpwg.org/specs/rfc7540.html#starting
func HTTP2ALPNTLSWrap(conn *net.TCPConn, cfg *tls.Config, tFunc TLSWrapperFunc) (*tls.Conn, error) {
	protoCfg := cfg.Clone()
	// TODO: append here
	protoCfg.NextProtos = []string{http2.NextProtoTLS}
	protoCfg.MinVersion = tls.VersionTLS12

	var tlsConn *tls.Conn
	for {
		if err := conn.SetDeadline(time.Now().Add(time.Second * 10)); err != nil {
			return nil, err
		}
		tlsConn = tFunc(conn, protoCfg)

		if err := tlsConn.Handshake(); err != nil {
			if netErr, ok := err.(net.Error); ok {
				if netErr.Timeout() || netErr.Temporary() {
					continue
				}
			}
			return nil, err
		}
		if err := conn.SetDeadline(time.Time{}); err != nil {
			return nil, err
		}
		break
	}

	// Check if we're creating a client conn before checking verification
	if isTLSClient(tFunc) {
		if !protoCfg.InsecureSkipVerify {
			if err := tlsConn.VerifyHostname(protoCfg.ServerName); err != nil {
				return nil, err
			}
		}
	}

	state := tlsConn.ConnectionState()
	if p := state.NegotiatedProtocol; p != http2.NextProtoTLS {
		return nil, fmt.Errorf("http2: unexpected ALPN protocol %q; want %q", p, http2.NextProtoTLS)
	}

	if !state.NegotiatedProtocolIsMutual {
		return nil, fmt.Errorf("http2: could not negotiate protocol mutually")
	}

	return tlsConn, nil
}

func isTLSClient(tFunc TLSWrapperFunc) bool {
	return reflect.ValueOf(tFunc).Pointer() == reflect.ValueOf(tls.Client).Pointer()
}

// CopyCloseIO establishes a full-duplex link between 2 ReadWriteClosers
func CopyCloseIO(lconn, rconn io.ReadWriteCloser) (lconnWritten, rconnWritten int64, err error) {
	copyCh := make(chan copyStatus, 2)
	var errs multiError

	// start tunnel
	c1die := make(chan struct{})
	go func() {
		// receive data
		n1, c1err := io.Copy(lconn, rconn)
		if isNormalTerminationError(c1err) {
			// ignore the error
			c1err = nil
		}
		copyCh <- copyStatus{n: n1, err: c1err, direction: LConnWrite}
		close(c1die)
	}()

	c2die := make(chan struct{})
	go func() {
		// send data
		n2, c2err := io.Copy(rconn, lconn)
		if isNormalTerminationError(c2err) {
			// ignore the error
			c2err = nil
		}
		copyCh <- copyStatus{n: n2, err: c2err, direction: RConnWrite}
		close(c2die)
	}()

	// wait for tunnel termination
	select {
	case <-c1die:
	case <-c2die:
	}
	if lerr := lconn.Close(); lerr != nil && !isNormalTerminationError(lerr) {
		errs.Append(lerr)
	}
	if rerr := rconn.Close(); rerr != nil && !isNormalTerminationError(rerr) {
		errs.Append(rerr)
	}

	// wait for io.Copy status
	status1 := <-copyCh
	status2 := <-copyCh

	if status1.direction == LConnWrite {
		lconnWritten = status1.n
		rconnWritten = status2.n
	} else {
		lconnWritten = status2.n
		rconnWritten = status1.n
	}

	if status1.err != nil && !isNormalTerminationError(status1.err) {
		errs.Append(status1.err)
	}
	if status2.err != nil && !isNormalTerminationError(status2.err) {
		errs.Append(status2.err)
	}

	if len(errs.errs) > 0 {
		err = errs
	}
	return
}

func isNormalTerminationError(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF {
		return true
	}
	e, ok := err.(*net.OpError)
	if ok && e.Timeout() {
		return true
	}

	for _, cause := range []string{
		"use of closed network connection",
		"broken pipe",
		"connection reset by peer",
	} {
		if strings.Contains(err.Error(), cause) {
			return true
		}
	}

	return false
}
