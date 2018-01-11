package net

import (
	"net"
)

// ListenerFromFactoryArgs specify how to create a listener from a factory
type ListenerFromFactoryArgs struct {
	// ID specifies an ID provided in an SNI
	// for now is ignored unless used from a sharedPortTLSListenerFactory instance
	ID string

	// BindHost allows us to wrap the returning Addr to report a hostname:port rather than an ip:port
	// If unspecified will use the IP
	BindHost string
}

// ListenerFactory defines an interface for a factory which can create listeners
type ListenerFactory interface {
	// Listener returns a net.Listener
	Listener(args *ListenerFromFactoryArgs) (net.Listener, error)

	// Close tears down the factory and all required additions
	Close() error
}

// ExtendedAddr allows for a net.Addr which provides extended data beyond String and Network
type ExtendedAddr interface {
	Data() interface{}
	net.Addr
}

// MultiAddr allows for a net.Addr to contain multiple net.Addrs - where the base net.Addr is simply
// the first in the slice of []net.Addrs
type MultiAddr interface {
	Addrs() []net.Addr
	net.Addr
}

type multiAddr struct {
	addrs []net.Addr
}

func (m *multiAddr) Network() string {
	if len(m.addrs) == 0 {
		return ""
	}
	return m.addrs[0].Network()
}

func (m *multiAddr) String() string {
	if len(m.addrs) == 0 {
		return ""
	}
	return m.addrs[0].String()
}

func (m *multiAddr) Addrs() []net.Addr {
	return m.addrs
}

type extendedAddr struct {
	addr net.Addr
	data interface{}
}

func (e *extendedAddr) Network() string {
	return e.addr.Network()
}

func (e *extendedAddr) String() string {
	return e.addr.String()
}

func (e *extendedAddr) Data() interface{} {
	return e.data
}

type addr struct {
	rawAddr     net.Addr
	bindHost    string
	virtualHost string
	network     string
}

func (a *addr) Network() string {
	if a.network == "" {
		return a.rawAddr.Network()
	}

	return a.network
}

func (a *addr) String() string {
	if a.bindHost == "" {
		return a.rawAddr.String()
	}

	prefix := ""
	if a.virtualHost != "" {
		prefix = a.virtualHost + "."
	}

	_, port, _ := net.SplitHostPort(a.rawAddr.String())
	return prefix + a.bindHost + ":" + port
}
