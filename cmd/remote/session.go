package main

import (
	"time"

	"github.com/garyburd/redigo/redis"
)

const (
	connectedSessionsKey    = "sessions:connected"
	disconnectedSessionsKey = "sessions:disconnected"
)

// Session ...
type Session struct {
	ID     string `redis:"id,omitempty"`
	Client string `redis:"client,omitempty"`
	NodeID string `redis:"node_id,omitempty"`

	BackendID string `redis:"backend_id,omitempty"`

	ClientAddr   string `redis:"client_addr,omitempty"`
	EndpointAddr string `redis:"endpoint_addr,omitempty"`
}

// RegisterConnection ...
func (session *Session) RegisterConnection(t time.Time) error {
	redisConn := redisPool.Get()
	defer redisConn.Close()

	redisConn.Send("MULTI")
	redisConn.Send("HMSET", redis.Args{}.Add(session.redisKey()).AddFlat(session)...)
	redisConn.Send("ZADD", connectedSessionsKey, timeToScore(t), session.ID)
	redisConn.Send("SADD", "node:"+nodeID+":sessions", session.ID)
	_, err := redisConn.Do("EXEC")

	return err
}

// RegisterDisconnection ...
func (session *Session) RegisterDisconnection(t time.Time) error {
	redisConn := redisPool.Get()
	defer redisConn.Close()

	redisConn.Send("MULTI")
	redisConn.Send("ZADD", disconnectedSessionsKey, timeToScore(t), session.ID)
	redisConn.Send("SREM", "node:"+nodeID+":sessions", session.ID)
	_, err := redisConn.Do("EXEC")
	return err
}

// UpdateAttribute ...
func (session *Session) UpdateAttribute(name string, value interface{}) error {
	redisConn := redisPool.Get()
	defer redisConn.Close()

	_, err := redisConn.Do("HSET", session.redisKey(), name, value)
	return err
}

func (session *Session) redisKey() string {
	return "session:" + session.ID
}

func timeToScore(t time.Time) int64 {
	return t.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
}
