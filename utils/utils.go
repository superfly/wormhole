package utils

import "io"

// CopyCloseIO ...
func CopyCloseIO(c1, c2 io.ReadWriteCloser) (err error) {
	defer c1.Close()
	defer c2.Close()
	errCh := make(chan error)

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
