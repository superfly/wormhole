package utils

import (
	"io"

	log "github.com/Sirupsen/logrus"
)

// CopyCloseIO ...
func CopyCloseIO(c1, c2 io.ReadWriteCloser) (err error) {
	defer func() {
		log.Debug("closing c1")
		err1 := c1.Close()
		if err1 != nil && err1 != io.EOF {
			log.Errorf("CopyCloseIO err1 : %s", err1.Error())
		}
	}()

	defer func() {
		log.Debug("closing c2")
		err2 := c2.Close()
		if err2 != nil && err2 != io.EOF {
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
