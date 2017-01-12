package session

import "github.com/superfly/wormhole/messages"

// Session hold information about connected client
type Session interface {
	ID() string
	BackendID() string
	NodeID() string
	Endpoint() string
	Key() string
	Release() *messages.Release
	RequireStream() error
	RequireAuthentication() error
	Close()
}

type baseSession struct {
	id           string `redis:"id,omitempty"`
	nodeID       string `redis:"node_id,omitempty"`
	backendID    string `redis:"backend_id,omitempty"`
	EndpointAddr string `redis:"endpoint_addr,omitempty"`

	release *messages.Release
	store   *RedisStore

	sessions map[string]Session
}

func (s *baseSession) ID() string {
	return s.id
}

func (s *baseSession) BackendID() string {
	return s.backendID
}

func (s *baseSession) NodeID() string {
	return s.nodeID
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
