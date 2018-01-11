package net

import (
	"net"

	"github.com/sirupsen/logrus"
)

type multiPortTCPListenerFactory struct {
	addr   string
	logger *logrus.Entry
}

// MultiPortTCPListenerFactoryArgs defines how to create a MultiPortTCPListenerFactory
type MultiPortTCPListenerFactoryArgs struct {
	// BindAddr should be only an IP or HostName. :0 will be appended to any given value
	BindAddr string
	Logger   *logrus.Logger
}

// NewMultiPortTCPListenerFactory defines a new listener factory for standard type listeners
func NewMultiPortTCPListenerFactory(args *MultiPortTCPListenerFactoryArgs) (ListenerFactory, error) {
	return &multiPortTCPListenerFactory{
		addr:   args.BindAddr + ":0",
		logger: args.Logger.WithFields(logrus.Fields{"prefix": "multi_port_tcp_listener_factory"}),
	}, nil
}

func (ml *multiPortTCPListenerFactory) Listener(args *ListenerFromFactoryArgs) (net.Listener, error) {
	l, err := net.Listen("tcp", ml.addr)
	if err != nil {
		return nil, err
	}

	return &multiPortTCPListener{
		listener: l,
		addr: &addr{
			rawAddr:  l.Addr(),
			bindHost: args.BindHost,
		},
	}, nil
}

func (ml *multiPortTCPListenerFactory) Close() error {
	// TODO: Record all opened conns and close?
	return nil
}

type multiPortTCPListener struct {
	listener net.Listener
	addr     *addr
}

func (l *multiPortTCPListener) Accept() (net.Conn, error) {
	return l.listener.Accept()
}

func (l *multiPortTCPListener) Close() error {
	return l.listener.Close()
}

func (l *multiPortTCPListener) Addr() net.Addr {
	return l.addr
}
