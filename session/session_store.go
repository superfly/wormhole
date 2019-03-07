package session

import (
	"net"
	"time"

	"github.com/gomodule/redigo/redis"
	wnet "github.com/superfly/wormhole/net"
)

const (
	sessionTTL              = 60 * 60 * 1 // 1h
	connectedSessionsKey    = "sessions:connected"
	disconnectedSessionsKey = "sessions:disconnected"
)

// Store is an interface to session persistence layer, e.g. Redis
type Store interface {
	RegisterConnection(s Session) error
	RegisterDisconnection(s Session) error
	RegisterRelease(s Session) error
	RegisterEndpoint(s Session) error
	RegisterHeartbeat(s Session) error
	UpdateAttribute(s Session, name string, value interface{}) error
	BackendIDFromToken(token string) (string, error)
	BackendRequiresClientAuth(backendID string) (bool, error)
	ValidCertificate(backendID, fingerprint string) (bool, error)
	GetClientCAs(backendID string) ([]byte, error)
	Announce(rep []byte)
}

// RedisStore is session persistence using Redis
type RedisStore struct {
	pool *redis.Pool
}

// NewRedisStore returns RedisStore struct
func NewRedisStore(pool *redis.Pool) *RedisStore {
	return &RedisStore{pool: pool}
}

// RegisterConnection writes Session connection info in Redis
// Should be called when a client connects.
func (r *RedisStore) RegisterConnection(s Session) error {
	t := time.Now()
	session := map[string]string{
		"id":           s.ID(),
		"node_id":      s.NodeID(),
		"backend_id":   s.BackendID(),
		"cluster":      s.Cluster(),
		"region":       s.Region(),
		"client_addr":  s.Client(),
		"agent":        s.Agent(),
		"connected_at": t.Format(time.RFC3339),
		"last_seen_at": t.Format(time.RFC3339),
	}
	redisConn := r.pool.Get()
	defer redisConn.Close()

	redisConn.Send("HMSET", redis.Args{}.Add(s.Key()).AddFlat(session)...)
	redisConn.Send("ZADD", connectedSessionsKey, timeToScore(t), s.ID())
	redisConn.Send("SADD", "node:"+s.NodeID()+":sessions", s.ID())
	redisConn.Send("SADD", "backend:"+s.BackendID()+":sessions", s.ID())
	_, err := redisConn.Do("EXEC")

	return err
}

// RegisterDisconnection removes Session connection info from Redis
// Should be called when a client disconnects.
func (r *RedisStore) RegisterDisconnection(s Session) error {
	t := time.Now()
	redisConn := r.pool.Get()
	defer redisConn.Close()

	redisConn.Send("MULTI")
	redisConn.Send("ZADD", disconnectedSessionsKey, timeToScore(t), s.ID())
	redisConn.Send("ZADD", dailyClientIpsKey(s, timeToDate(t)), timeToScore(t), s.ClientIP())
	redisConn.Send("ZREM", connectedSessionsKey, s.ID())
	redisConn.Send("SREM", "node:"+s.NodeID()+":sessions", s.ID())
	redisConn.Send("SREM", "backend:"+s.BackendID()+":sessions", s.ID())
	for _, endpointAddr := range s.Endpoints() {
		redisConn.Send("SREM", "backend:"+s.BackendID()+":endpoints", redisEndpointString(endpointAddr))
		redisConn.Send("DEL", "backend:"+s.BackendID()+":endpoint:"+redisEndpointString(endpointAddr))
	}
	redisConn.Send("EXPIRE", s.Key(), sessionTTL)
	_, err := redisConn.Do("EXEC")
	return err
}

// RegisterEndpoint updates the client endoint addr in stored session and adds
// Endpoint to the list of endpoints stored in Redis
func (r *RedisStore) RegisterEndpoint(s Session) error {
	t := time.Now()
	redisConn := r.pool.Get()
	defer redisConn.Close()

	redisConn.Send("MULTI")
	redisConn.Send("HSET", s.Key(), "cluster", s.Cluster())
	for _, endpointAddr := range s.Endpoints() {
		redisConn.Send("SADD", "backend:"+s.BackendID()+":endpoints", redisEndpointString(endpointAddr))
		endpoint := map[string]string{
			"session_id":   s.ID(),
			"backend_id":   s.BackendID(),
			"cluster":      s.Cluster(),
			"region":       s.Region(),
			"connected_at": t.Format(time.RFC3339),
			"last_seen_at": t.Format(time.RFC3339),
		}

		if extended, ok := endpointAddr.(wnet.ExtendedAddr); ok {
			switch t := extended.Data().(type) {
			case wnet.SharedTLSAddrExtendedData:
				endpoint["ca_cert"] = string(t.CACert)
			default:
			}
		}

		redisConn.Send("HMSET", redis.Args{}.Add(endpointKey(s, endpointAddr)).AddFlat(endpoint)...)
	}
	_, err := redisConn.Do("EXEC")
	return err
}

// RegisterRelease updates VCS (e.g git) info collected by the client
func (r *RedisStore) RegisterRelease(s Session) error {
	t := time.Now()
	redisConn := r.pool.Get()
	defer redisConn.Close()

	redisConn.Send("MULTI")
	redisConn.Send("ZADD", "backend:"+s.BackendID()+":releases", "NX", timeToScore(t), s.Release().ID)
	redisConn.Send("HMSET", redis.Args{}.Add("backend:"+s.BackendID()+":release:"+s.Release().ID).AddFlat(s.Release())...)
	for _, endpointAddr := range s.Endpoints() {
		redisConn.Send("HSET", endpointKey(s, endpointAddr), "branch", s.Release().Branch)
	}
	_, err := redisConn.Do("EXEC")
	return err
}

// RegisterHeartbeat updates timestamps for session and endpoint keys
func (r *RedisStore) RegisterHeartbeat(s Session) error {
	t := time.Now()
	redisConn := r.pool.Get()
	defer redisConn.Close()

	redisConn.Send("MULTI")
	redisConn.Send("HSET", s.Key(), "last_seen_at", t.Format(time.RFC3339))
	for _, endpointAddr := range s.Endpoints() {
		redisConn.Send("HSET", endpointKey(s, endpointAddr), "last_seen_at", t.Format(time.RFC3339))
	}
	_, err := redisConn.Do("EXEC")
	return err
}

// BackendIDFromToken returns a backendID for the token or errors out if none found
func (r *RedisStore) BackendIDFromToken(token string) (string, error) {
	redisConn := r.pool.Get()
	defer redisConn.Close()

	return redis.String(redisConn.Do("HGET", "backend_tokens", token))
}

// BackendRequiresClientAuth returns a backendID for the token or errors out if none found
func (r *RedisStore) BackendRequiresClientAuth(backendID string) (bool, error) {
	redisConn := r.pool.Get()
	defer redisConn.Close()

	// assume client auth is required unless explicitly disabled
	authDisabled, err := redis.Bool(redisConn.Do("HGET", "backend:"+backendID, "client_auth_disabled"))
	if err != nil {
		if err == redis.ErrNil {
			return true, nil
		}

		// in case there's conn error and err handling is not properly handled up the chain,
		// require client auth just to be on the safe side
		return true, err
	}

	// might seem a bit unintuitive, but backend requires client auth if authDisabled is false
	// and doesn't when authDisabled is true
	return !authDisabled, nil
}

// UpdateAttribute updates a single Session attribute in Redis
func (r *RedisStore) UpdateAttribute(s Session, name string, value interface{}) error {
	redisConn := r.pool.Get()
	defer redisConn.Close()

	_, err := redisConn.Do("HSET", s.Key(), name, value)
	return err
}

// GetClientCAs returns full unparsed certificate chain for the client auth for the backend
func (r *RedisStore) GetClientCAs(backendID string) ([]byte, error) {
	redisConn := r.pool.Get()
	defer redisConn.Close()

	return redis.Bytes(redisConn.Do("HGET", "backend:"+backendID, "client_auth_chain"))
}

// ValidCertificate returns true if a fingerprint is a in the list of
// valid certificates for the backend.
func (r *RedisStore) ValidCertificate(backendID, fingerprint string) (bool, error) {
	redisConn := r.pool.Get()
	defer redisConn.Close()

	return redis.Bool(redisConn.Do("SISMEMBER", "backend:"+backendID+":valid_certificates", fingerprint))
}

// Announce announces the server on redis
// rep is a serialized representation of the current server
func (r *RedisStore) Announce(rep []byte) {
	announce(r.pool, rep)
	ticker := time.NewTicker(10 * time.Second)
	for range ticker.C {
		announce(r.pool, rep)
	}
}

const announceKey = "servers"

func announce(pool *redis.Pool, rep []byte) {
	redisConn := pool.Get()
	defer redisConn.Close()

	redisConn.Do("ZADD", announceKey, time.Now().Unix(), rep)
}

func timeToScore(t time.Time) int64 {
	return t.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
}

func timeToDate(t time.Time) string {
	return t.Format("2006-01-02")
}

func dailyClientIpsKey(s Session, date string) string {
	return "node:" + s.NodeID() + ":clients:" + date
}

func endpointKey(s Session, endpoint net.Addr) string {
	return "backend:" + s.BackendID() + ":endpoint:" + redisEndpointString(endpoint)
}

func redisEndpointString(e net.Addr) string {
	prefix := ""
	if e.Network() == "tcp+tls" {
		prefix = "tls:"
	}
	return prefix + e.String()
}
