package net

import (
	"github.com/sirupsen/logrus"

	"fmt"
	"net"
)

// FanInListenerFactoryEntry is a type defined for inserting factories into a FanInListenerFactory
type FanInListenerFactoryEntry struct {
	// Factory identifies the factory
	Factory ListenerFactory

	// ShouldCleanup determines if the given factory should close if the fan-in
	// factory closes too
	ShouldCleanup bool
}

type fanInListenerFactory struct {
	factories []FanInListenerFactoryEntry
	logger    *logrus.Entry
}

// FanInListenerFactoryArgs defines how a FanInListenerFactory should be built
type FanInListenerFactoryArgs struct {
	Factories []FanInListenerFactoryEntry
	Logger    *logrus.Logger
}

// NewFanInListenerFactory return a new FanInListenerFactory from the args
func NewFanInListenerFactory(args *FanInListenerFactoryArgs) (ListenerFactory, error) {
	return &fanInListenerFactory{
		factories: args.Factories,
		logger:    args.Logger.WithFields(logrus.Fields{"prefix": "fan_in_listener_factory"}),
	}, nil
}

// Listener creates a new fan-in style listener
//
// This works by creating a new listener from each ListenerFactory that was passed to
// the FanInListenerFactory. Then each of those listeners get their own goroutine to call
// Accept() and pass the given conn down a connection channel. A call to Accept() from the listener
// returned from this function will block on reading from that connection channel - thus maintains
// the blocking nature of the Accept() call
func (f *fanInListenerFactory) Listener(args *ListenerFromFactoryArgs) (net.Listener, error) {
	listeners := []net.Listener{}
	for _, fEntry := range f.factories {
		l, err := fEntry.Factory.Listener(args)
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, l)
	}

	l := &fanInListener{
		listeners: listeners,
		connCh:    make(chan net.Conn),
		done:      make(chan struct{}),
	}

	l.populateCh()

	return l, nil
}

// Close closes all factories from the fan-in
// NOTE: We may shouldn't close factories from handlers. Perhaps best to leave it to top-level
// remote cleanup
func (f *fanInListenerFactory) Close() error {
	var retErr error
	for _, fEntry := range f.factories {
		if fEntry.ShouldCleanup {
			if err := fEntry.Factory.Close(); err != nil {
				retErr = err
			}
		}
	}

	return retErr
}

type fanInListener struct {
	listeners []net.Listener
	connCh    chan net.Conn
	done      chan struct{}
}

func (l *fanInListener) populateCh() {
	for _, listener := range l.listeners {
		go l.populateChListener(listener)
	}
}

func (l *fanInListener) populateChListener(ln net.Listener) error {
	for {
		select {
		case <-l.done:
			return nil
		default:
			conn, err := ln.Accept()
			if err != nil {

				if oErr, ok := err.(*net.OpError); ok {
					if oErr.Temporary() {
						continue
					} else {
						return err
					}
				}

				return err
			}

			go func(c net.Conn) {
				// backup for race if we close between first select and here
				select {
				case <-l.done:
				case l.connCh <- c:
				}
			}(conn)
		}
	}
}

func (l *fanInListener) Accept() (net.Conn, error) {
	c, ok := <-l.connCh
	if !ok {
		return nil, &net.OpError{
			Op:  "accept",
			Net: "unknown",
			Err: fmt.Errorf("SNI TLS Listener closed"),
		}
	}
	return c, nil
}

func (l *fanInListener) Addr() net.Addr {
	mAddr := &multiAddr{
		addrs: []net.Addr{},
	}
	for i := range l.listeners {
		var tmpAddrs []net.Addr
		addr := l.listeners[i].Addr()
		if multi, ok := addr.(MultiAddr); ok {
			tmpAddrs = multi.Addrs()
		} else {
			tmpAddrs = []net.Addr{
				l.listeners[i].Addr(),
			}
		}
		mAddr.addrs = append(mAddr.addrs, tmpAddrs...)
	}

	return mAddr
}

func (l *fanInListener) Close() error {
	close(l.done)
	var retErr error
	for _, listener := range l.listeners {
		if err := listener.Close(); err != nil {
			retErr = err
		}
	}
	return retErr
}
