package session

import "github.com/superfly/wormhole/messages"

type Session interface {
	ID() string
	NodeID() string
	BackendID() string
	Key() string
	Release() *messages.Release
	RequireStream() error
	RequireAuthentication() error
	Close()
}
