package utils

import (
	"io"
	"net"
	"os"
	"strings"

	"github.com/Sirupsen/logrus"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
)

var logger = logrus.New()
var log *logrus.Entry

func init() {
	logger.Formatter = new(prefixed.TextFormatter)
	if os.Getenv("LOG_LEVEL") == "debug" {
		logger.Level = logrus.DebugLevel
	}
	log = logger.WithFields(logrus.Fields{
		"prefix": "CopyCloseIO",
	})
}

// CopyCloseIO establishes a full-duplex link between 2 ReadWriteClosers
func CopyCloseIO(lconn, rconn io.ReadWriteCloser) (err error) {
	defer func() {
		err1 := lconn.Close()
		if isNormalTerminationError(err1) {
			log.Debugf("lconn Close() err: %s", err1.Error())
		} else if err1 != nil {
			log.Errorf("lconn Close() err: %s", err1.Error())
		}
	}()

	defer func() {
		err2 := rconn.Close()
		if isNormalTerminationError(err2) {
			log.Debugf("rconn Close() err: %s", err2.Error())
		} else if err2 != nil {
			log.Errorf("rconn Close() err: %s", err2.Error())
		}
	}()

	errCh := make(chan error, 1)

	// start tunnel
	c1die := make(chan struct{})
	go func() {
		// receive data
		n1, c1err := io.Copy(lconn, rconn)
		log.Debugf("Received %d bytes", n1)
		if isNormalTerminationError(c1err) {
			log.Debugf("recv Copy() err: %s", c1err.Error())
		} else if c1err != nil {
			log.Errorf("recv Copy() err: %s", c1err.Error())
		}
		errCh <- c1err
		close(c1die)
	}()

	c2die := make(chan struct{})
	go func() {
		// send data
		n2, c2err := io.Copy(rconn, lconn)
		log.Debugf("Sent %d bytes", n2)
		if isNormalTerminationError(c2err) {
			log.Debugf("send Copy() err: %s", c2err.Error())
		} else if c2err != nil {
			log.Errorf("send Copy() err: %s", c2err.Error())
		}
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
