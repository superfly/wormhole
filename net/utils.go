package net

import (
	"io"
	"net"
	"strings"
)

// CopyDirection decribes the direction of data copying in full-duplex link
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
