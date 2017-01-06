package session

import (
	"time"

	"github.com/garyburd/redigo/redis"
)

type SessionStore interface {
	RegisterConnection(s Session) error
	RegisterDisconnection(s Session) error
	UpdateAttribute(s Session, name string, value interface{}) error
	BackendIDFromToken(token string) (string, error)
}

type RedisSessionStore struct {
	pool *redis.Pool
}

func NewRedisSessionStore(pool *redis.Pool) *RedisSessionStore {
	return &RedisSessionStore{pool: pool}
}

func (r *RedisSessionStore) RegisterConnection(s Session) error {
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

func (r *RedisSessionStore) RegisterDisconnection(s Session) error {
	t := time.Now()
	redisConn := r.pool.Get()
	defer redisConn.Close()

	redisConn.Send("MULTI")
	redisConn.Send("ZADD", disconnectedSessionsKey, timeToScore(t), s.ID())
	redisConn.Send("SREM", "node:"+s.NodeID()+":sessions", s.ID())
	redisConn.Send("SREM", "backend:"+s.BackendID()+":sessions", s.ID())
	_, err := redisConn.Do("EXEC")
	return err
}

// UpdateAttribute ...
func (r *RedisSessionStore) UpdateAttribute(s Session, name string, value interface{}) error {
	redisConn := r.pool.Get()
	defer redisConn.Close()

	_, err := redisConn.Do("HSET", s.Key(), name, value)
	return err
}

func timeToScore(t time.Time) int64 {
	return t.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
}

func (r *RedisSessionStore) RegisterRelease(s Session) error {
	t := time.Now()
	redisConn := r.pool.Get()
	defer redisConn.Close()

	redisConn.Send("MULTI")
	redisConn.Send("ZADD", "backend:"+s.BackendID()+":releases", "NX", timeToScore(t), s.Release().ID)
	redisConn.Send("HMSET", redis.Args{}.Add("backend:"+s.BackendID()+":release:"+s.Release().ID).AddFlat(s.Release())...)
	_, err := redisConn.Do("EXEC")
	return err
}

func (r *RedisSessionStore) BackendIDFromToken(token string) (string, error) {
	redisConn := r.pool.Get()
	defer redisConn.Close()

	return redis.String(redisConn.Do("HGET", "backend_tokens", token))
}
