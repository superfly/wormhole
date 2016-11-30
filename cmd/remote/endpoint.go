package main

import (
	"fmt"

	log "github.com/Sirupsen/logrus"
)

// Endpoint ...
type Endpoint struct {
	SessionID string `redis:"session_id"`
	BackendID string `redis:"backend_id"`
	Socket    string `redis:"socket"`
}

// Register ...
func (endpoint *Endpoint) Register() error {
	redisConn := redisPool.Get()
	defer redisConn.Close()

	_, err := redisConn.Do("SADD", endpointsRedisKey(endpoint.BackendID), endpoint.Socket)
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
	_, err := redisConn.Do("SREM", endpointsRedisKey(endpoint.BackendID), endpoint.Socket)
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
