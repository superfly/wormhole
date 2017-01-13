package session

import (
	"time"

	"github.com/garyburd/redigo/redis"
)

const (
	sessionTTL              = 60 * 60 * 24 // 24h
	connectedSessionsKey    = "sessions:connected"
	disconnectedSessionsKey = "sessions:disconnected"
)

// Store is an interface to session persistence layer, e.g. Redis
type Store interface {
	RegisterConnection(s Session) error
	RegisterDisconnection(s Session) error
	RegisterRelease(s Session) error
	UpdateAttribute(s Session, name string, value interface{}) error
	BackendIDFromToken(token string) (string, error)
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
	redisConn := r.pool.Get()
	defer redisConn.Close()

	redisConn.Send("HMSET", redis.Args{}.Add(s.Key()).AddFlat(s)...)
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
	redisConn.Send("ZREM", connectedSessionsKey, timeToScore(t), s.ID())
	redisConn.Send("SREM", "node:"+s.NodeID()+":sessions", s.ID())
	redisConn.Send("SREM", "backend:"+s.BackendID()+":sessions", s.ID())
	redisConn.Send("SREM", "backend:"+s.BackendID()+":endpoints", s.Endpoint())
	redisConn.Send("DEL", "backend:"+s.BackendID()+":endpoint:"+s.Endpoint())
	redisConn.Send("EXPIRE", s.Key(), sessionTTL)
	_, err := redisConn.Do("EXEC")
	return err
}

// RegisterEndpoint updates the client endoint addr in stored session and adds
// Endpoint to the list of endpoints stored in Redis
func (r *RedisStore) RegisterEndpoint(s Session) error {
	redisConn := r.pool.Get()
	defer redisConn.Close()

	redisConn.Send("MULTI")
	redisConn.Send("HSET", s.Key(), "endpoint_addr", s.Endpoint())
	redisConn.Send("HSET", s.Key(), "cluster", s.Cluster())
	redisConn.Send("SADD", "backend:"+s.BackendID()+":endpoints", s.Endpoint())
	endpoint := map[string]string{
		"session_id": s.ID(),
		"backend_id": s.BackendID(),
		"socket":     s.Endpoint(),
		"cluster":    s.Cluster(),
	}
	redisConn.Send("HMSET", redis.Args{}.Add("backend:"+s.BackendID()+":endpoint:"+s.Endpoint()).AddFlat(endpoint)...)
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
	_, err := redisConn.Do("EXEC")
	return err
}

// BackendIDFromToken returns a backendID for the token or errors out if none found
func (r *RedisStore) BackendIDFromToken(token string) (string, error) {
	redisConn := r.pool.Get()
	defer redisConn.Close()

	return redis.String(redisConn.Do("HGET", "backend_tokens", token))
}

// UpdateAttribute updates a single Session attribute in Redis
func (r *RedisStore) UpdateAttribute(s Session, name string, value interface{}) error {
	redisConn := r.pool.Get()
	defer redisConn.Close()

	_, err := redisConn.Do("HSET", s.Key(), name, value)
	return err
}

func timeToScore(t time.Time) int64 {
	return t.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
}
