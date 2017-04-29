package local

// ConnectionHandler specifies interface for handler connecting to wormhole server
type ConnectionHandler interface {
	ListenAndServe() error
	Close() error
}
