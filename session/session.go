package session

import (
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"fmt"
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
	RequiresClientAuth() bool
	ClientCAs() (*x509.CertPool, error)
	ValidCertificate(c *x509.Certificate) (bool, error)
	Close()
}

// baseSession struct implements the Session interface and provides
// common methods for concrete Session types (e.g. HTTP2 or SSH)
type baseSession struct {
	id                 string
	agent              string
	nodeID             string
	backendID          string
	clientAddr         string
	endpoints          []net.Addr
	ClusterURL         string
	requiresClientAuth bool

	release *messages.Release
	store   Store
	logger  *logrus.Entry
}

// ID returns ID of this session
func (s *baseSession) ID() string {
	return s.id
}

// Agent returns the wormhole client information (e.g. version of the binary)
func (s *baseSession) Agent() string {
	return s.agent
}

// BackendID returns and ID of the backend that this session belongs to
func (s *baseSession) BackendID() string {
	return s.backendID
}

// NodeID returns an id of the wormhole server on which is session is registered
func (s *baseSession) NodeID() string {
	return s.nodeID
}

// Client returns the client address (likely IP:PORT) of this session's client
func (s *baseSession) Client() string {
	return s.clientAddr
}

// ClientIP returns an IP address of this session's client
func (s *baseSession) ClientIP() string {
	host, _, _ := net.SplitHostPort(s.clientAddr)
	return host
}

// Cluster returns a cluster identifier
func (s *baseSession) Cluster() string {
	return s.ClusterURL
}

// Endpoints returns a list of endpoint addresses that have been registered for
// this session.
func (s *baseSession) Endpoints() []net.Addr {
	return s.endpoints
}

// AddEndpoint add an endpoint addr to this session.
func (s *baseSession) AddEndpoint(e net.Addr) {
	s.endpoints = append(s.endpoints, e)
}

// Key returns a session key
func (s *baseSession) Key() string {
	return "session:" + s.id
}

// Release returns release information, if one has been received for this session
func (s *baseSession) Release() *messages.Release {
	return s.release
}

// RequiresClientAuth returns true if the session requires a client certificate
// authentication.
func (s *baseSession) RequiresClientAuth() bool {
	return s.requiresClientAuth
}

// RequireAuthentication is an API for concrete session types to implement session
// authentication
func (s *baseSession) RequireAuthentication() error {
	return errors.New("not implemented")
}

// RequireStream is an API for concrete session types to implement session
// etasblishment
func (s *baseSession) RequireStream() error {
	return errors.New("not implemented")
}

// ClientCAs returns a CertPool for the session that is used for client
// certificate authentication.
func (s *baseSession) ClientCAs() (*x509.CertPool, error) {
	bytes, err := s.store.GetClientCAs(s.backendID)
	if err != nil {
		return nil, err
	}

	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM(bytes); !ok {
		return nil, fmt.Errorf("Bad cert for backend ID(='%s')", s.backendID)
	}
	return pool, nil
}

// ValidCertificate returns true if a certificate is in the list of
// valid certificates.
func (s *baseSession) ValidCertificate(c *x509.Certificate) (bool, error) {
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(c.Raw))
	return s.store.ValidCertificate(s.BackendID(), fingerprint)
}

// Close closes the session
// Close should be implemented by structs that embed baseSession
func (s *baseSession) Close() {
}
