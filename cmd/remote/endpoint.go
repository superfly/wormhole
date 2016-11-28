package main

import (
	"fmt"

	"github.com/garyburd/redigo/redis"

	log "github.com/Sirupsen/logrus"
)

// Endpoint ...
type Endpoint struct {
	BackendID string `redis:"backend_id"`
	IP        string `redis:"ip"`
	Port      string `redis:"port"`
	SessionID string `redis:"session_id"`
	Name      string `redis:"name"`
}

// ID ...
func (endpoint *Endpoint) ID() string {
	return endpoint.SessionID
}

// RedisKey ...
func (endpoint *Endpoint) RedisKey() string {
	return fmt.Sprintf("backend:%s:endpoint:%s", endpoint.BackendID, endpoint.ID())
}

// Register ...
func (endpoint *Endpoint) Register() error {
	redisConn := redisPool.Get()
	defer redisConn.Close()

	redisConn.Send("MULTI")
	redisConn.Send("SADD", endpointsRedisKey(endpoint.BackendID), endpoint.ID())
	redisConn.Send("HMSET", redis.Args{}.Add(endpoint.RedisKey()).AddFlat(endpoint)...)

	_, err := redisConn.Do("EXEC")
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"endpoint": endpoint,
	}).Info("Endpoint added.")

	return nil
}

// Remove ...
func (endpoint *Endpoint) Remove() error {
	redisConn := redisPool.Get()
	defer redisConn.Close()

	// Pipelining it up!
	redisConn.Send("MULTI")
	redisConn.Send("SREM", endpointsRedisKey(endpoint.BackendID), endpoint.ID())
	redisConn.Send("DEL", endpoint.RedisKey())

	_, err := redisConn.Do("EXEC")
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"endpoint": endpoint,
	}).Info("Endpoint removed.")

	return nil
}

func endpointsRedisKey(backendID string) string {
	return fmt.Sprintf("backend:%s:endpoints", backendID)
}
