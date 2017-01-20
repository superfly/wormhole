package utils

import (
	"io"
	"net"
	"strings"

	log "github.com/Sirupsen/logrus"
)

// CopyCloseIO ...
func CopyCloseIO(c1, c2 io.ReadWriteCloser) (err error) {
	defer func() {
		log.Debug("closing c1")
		err1 := c1.Close()
		if isNormalTerminationError(err1) {
			log.Debugf("CopyCloseIO err1: %s", err1.Error())
		} else if err1 != nil {
			log.Errorf("CopyCloseIO err1 : %s", err1.Error())
		}
	}()

	defer func() {
		log.Debug("closing c2")
		err2 := c2.Close()
		if isNormalTerminationError(err2) {
			log.Debugf("CopyCloseIO err2: %s", err2.Error())
		} else if err2 != nil {
			log.Errorf("CopyCloseIO err2 : %s", err2.Error())
		}
	}()

	errCh := make(chan error, 1)

	// start tunnel
	c1die := make(chan struct{})
	go func() {
		_, c1err := io.Copy(c1, c2)
		errCh <- c1err
		close(c1die)
	}()

	c2die := make(chan struct{})
	go func() {
		_, c2err := io.Copy(c2, c1)
		errCh <- c2err
		close(c2die)
	}()

	// wait for tunnel termination
	select {
	case <-c1die:
	case <-c2die:
	}
	err = <-errCh
	return err
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
