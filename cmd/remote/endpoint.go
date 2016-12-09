package main

import (
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
)

// Endpoint ...
type Endpoint struct {
	SessionID string `redis:"session_id"`
	BackendID string `redis:"backend_id"`
	Socket    string `redis:"socket"`
	ReleaseID string `redis:"release_id"`
}

// Register ...
func (endpoint *Endpoint) Register() error {
	redisConn := redisPool.Get()
	defer redisConn.Close()

	redisConn.Send("MULTI")
	redisConn.Send("SADD", endpointsRedisKey(endpoint.BackendID), endpoint.Socket)
	redisConn.Send("HMSET", redis.Args{}.Add(endpoint.redisKey()).AddFlat(endpoint)...)
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

	redisConn.Send("MULTI")
	redisConn.Send("SREM", endpointsRedisKey(endpoint.BackendID), endpoint.Socket)
	redisConn.Send("DEL", endpoint.redisKey())
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

func (endpoint *Endpoint) redisKey() string {
	return fmt.Sprintf("backend:%s:endpoint:%s", endpoint.BackendID, endpoint.Socket)
}
