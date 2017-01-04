package utils

import "io"

// CopyCloseIO ...
func CopyCloseIO(c1, c2 io.ReadWriteCloser) (err error) {
	defer c1.Close()
	defer c2.Close()

	// start tunnel
	c1die := make(chan struct{})
	go func() {
		_, err = io.Copy(c1, c2)
		close(c1die)
	}()

	c2die := make(chan struct{})
	go func() {
		_, err = io.Copy(c2, c1)
		close(c2die)
	}()

	// wait for tunnel termination
	select {
	case <-c1die:
	case <-c2die:
	}
	return err
}
