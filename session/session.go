package session

import (
	"net"

	"github.com/sirupsen/logrus"
	"github.com/superfly/wormhole/messages"
)

// Session hold information about connected client
type Session interface {
	ID() string
	Agent() string
	BackendID() string
	NodeID() string
	Client() string
	ClientIP() string
	Cluster() string
	Endpoints() []net.Addr
	AddEndpoint(endpoint net.Addr)
	Key() string
	Release() *messages.Release
	RequireStream() error
	RequireAuthentication() error
	Close()
}

type baseSession struct {
	id         string
	agent      string
	nodeID     string
	backendID  string
	clientAddr string
	endpoints  []net.Addr
	ClusterURL string

	release *messages.Release
	store   *RedisStore
	logger  *logrus.Entry
}

func (s *baseSession) ID() string {
	return s.id
}

func (s *baseSession) Agent() string {
	return s.agent
}

func (s *baseSession) BackendID() string {
	return s.backendID
}

func (s *baseSession) NodeID() string {
	return s.nodeID
}

func (s *baseSession) Client() string {
	return s.clientAddr
}

func (s *baseSession) ClientIP() string {
	host, _, _ := net.SplitHostPort(s.clientAddr)
	return host
}

func (s *baseSession) Cluster() string {
	return s.ClusterURL
}

func (s *baseSession) Endpoints() []net.Addr {
	return s.endpoints
}

func (s *baseSession) AddEndpoint(e net.Addr) {
	s.endpoints = append(s.endpoints, e)
}

func (s *baseSession) Key() string {
	return "session:" + s.id
}

func (s *baseSession) Release() *messages.Release {
	return s.release
}
