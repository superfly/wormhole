package net

import (
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

type sharedPortTLSListenerFactory struct {
	listener net.Listener

	forward map[string]*sharedPortTLSListener
	fLock   sync.Mutex
	logger  *logrus.Entry

	stopC chan struct{}
}

// SharedPortTLSListenerFactoryArgs provides the data needed to create a SharedPortTLSListenerFactory
type SharedPortTLSListenerFactoryArgs struct {
	TLSConfig *tls.Config
	Address   string
	Logger    *logrus.Logger
}

// NewSharedPortTLSListenerFactory creates a new listener factory for shared port TLS
func NewSharedPortTLSListenerFactory(args *SharedPortTLSListenerFactoryArgs) (ListenerFactory, error) {
	if args.TLSConfig == nil {
		return nil, fmt.Errorf("Must set TLS Config")
	}
	listener, err := tls.Listen("tcp", args.Address, args.TLSConfig)
	if err != nil {
		return nil, err
	}

	f := &sharedPortTLSListenerFactory{
		listener: listener,
		forward:  make(map[string]*sharedPortTLSListener),
		stopC:    make(chan struct{}),
		logger:   args.Logger.WithFields(logrus.Fields{"prefix": "shared_port_tls_listener_factory"}),
	}

	go func() {
		if err := f.populateCh(); err != nil {
			if err := f.Close(); err != nil {
				f.logger.Errorf("Error closing shared port listener: %+v", err)
				return
			}
		}
	}()

	return f, nil
}

// Listener creates a new listener from the factory
func (sl *sharedPortTLSListenerFactory) Listener(args *ListenerFromFactoryArgs) (net.Listener, error) {
	if args.ID == "" {
		return nil, fmt.Errorf("Must supply an ID")
	}
	listener := &sharedPortTLSListener{
		connCh: make(chan net.Conn),
		done:   make(chan struct{}),
		addr: &extendedAddr{
			addr: &addr{
				rawAddr:     sl.listener.Addr(),
				bindHost:    args.BindHost,
				virtualHost: args.ID,
				network:     "tcp+tls",
			},
			data: SharedTLSAddrExtendedData{ID: args.ID},
		},
		id: args.ID,
		f:  sl,
	}
	sl.fLock.Lock()
	sl.forward[args.ID] = listener
	sl.fLock.Unlock()
	return listener, nil
}

// Close...
func (sl *sharedPortTLSListenerFactory) Close() error {
	close(sl.stopC)
	return nil
}

func (sl *sharedPortTLSListenerFactory) populateCh() error {
	for {
		select {
		case <-sl.stopC:
			return nil
		default:
			conn, err := sl.listener.Accept()
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
			sl.logger.Debugf("Accepted conn from: %s", conn.RemoteAddr().String())

			tlsConn, ok := conn.(*tls.Conn)
			if !ok {
				sl.logger.Errorf("Conn from listener is not TLS Conn")
				continue
			}
			go func(c *tls.Conn) {
				id, err := sl.getID(c)
				if err != nil {
					sl.logger.Errorf("Error finding ID from SNI: %+v", err)
					return
				}

				sl.fLock.Lock()
				ch, ok := sl.forward[id]
				sl.fLock.Unlock()
				if !ok {
					sl.logger.Errorf("SNI ID %s not found", id)
					return
				}
				select {
				case <-ch.done:
				case ch.connCh <- c:
				}
			}(tlsConn)
		}
	}
}

func (sl *sharedPortTLSListenerFactory) getID(c *tls.Conn) (string, error) {
	// pre-process handshake so we don't have to test it later
	if err := c.Handshake(); err != nil {
		return "", err
	}
	sn := c.ConnectionState().ServerName

	snParts := strings.Split(sn, ".")
	if len(snParts) == 0 {
		return "", fmt.Errorf("No ID found from SNI")
	}

	return snParts[0], nil
}

type sharedPortTLSListener struct {
	connCh chan net.Conn
	done   chan struct{}
	addr   *extendedAddr
	id     string

	// f is stored such that we can delete ourselves when Close is called
	f *sharedPortTLSListenerFactory
}

// Close...
func (l *sharedPortTLSListener) Close() error {
	close(l.done)
	defer close(l.connCh)

	// delete all refs so garbage collector will clean us up
	l.f.fLock.Lock()
	delete(l.f.forward, l.id)
	l.f.fLock.Unlock()
	l.f = nil

	return nil
}

// Accept accepts a conn from the listener
func (l *sharedPortTLSListener) Accept() (net.Conn, error) {
	c, ok := <-l.connCh
	if !ok {
		return nil, &net.OpError{
			Op:     "accept",
			Net:    "tcp+tls",
			Source: l.addr,
			Addr:   l.addr,
			Err:    fmt.Errorf("SNI TLS Listener closed"),
		}
	}
	return c, nil
}

// Addr returns the listener's address
func (l *sharedPortTLSListener) Addr() net.Addr {
	return l.addr
}

// SharedTLSAddrExtendedData can be used when wanting to know more about an Addr
type SharedTLSAddrExtendedData struct {
	ID     string
	CACert []byte
}
