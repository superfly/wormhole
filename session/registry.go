package session

import (
	"sync"

	"github.com/sirupsen/logrus"
)

// Registry holds references to all active sessions
type Registry struct {
	logger   *logrus.Entry
	registry map[string]Session
	lock     sync.RWMutex
}

// NewRegistry initializes a new Registry struct
func NewRegistry(l *logrus.Logger) *Registry {
	return &Registry{
		registry: make(map[string]Session),
		logger:   l.WithFields(logrus.Fields{"prefix": "SessionRegistry"}),
	}
}

// AddSession adds session to the registry
func (r *Registry) AddSession(s Session) {
	r.lock.Lock()
	r.registry[s.ID()] = s
	r.lock.Unlock()
	r.logger.Debug("Added session: ", s.ID())
}

// GetSession returns session stored in the registry, or nil if not found
func (r *Registry) GetSession(id string) Session {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.registry[id]
}

// RemoveSession removes session if currently stored in the registry
func (r *Registry) RemoveSession(s Session) {
	r.lock.Lock()
	delete(r.registry, s.ID())
	r.lock.Unlock()
	r.logger.Debug("Removed session: ", s.ID())
}

// Close closes and removes all sessions
func (r *Registry) Close() {
	r.lock.Lock()
	for id, sess := range r.registry {
		delete(r.registry, id)
		if sess != nil {
			sess.Close()
		}
	}
	r.lock.Unlock()
}
