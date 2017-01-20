package session

import "github.com/superfly/wormhole/messages"

// Session hold information about connected client
type Session interface {
	ID() string
	Agent() string
	BackendID() string
	NodeID() string
	Client() string
	Cluster() string
	Endpoint() string
	Key() string
	Release() *messages.Release
	RequireStream() error
	RequireAuthentication() error
	Close()
}

type baseSession struct {
	id           string
	agent        string
	nodeID       string
	backendID    string
	clientAddr   string
	EndpointAddr string
	ClusterURL   string

	release *messages.Release
	store   *RedisStore

	sessions map[string]Session
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

func (s *baseSession) Cluster() string {
	return s.ClusterURL
}

func (s *baseSession) Endpoint() string {
	return s.EndpointAddr
}

func (s *baseSession) Key() string {
	return "session:" + s.id
}

func (s *baseSession) Release() *messages.Release {
	return s.release
}
