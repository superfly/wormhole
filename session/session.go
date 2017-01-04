package session

type Session interface {
	RequireStream() error
	RequireAuthentication() error
	Close()
}
