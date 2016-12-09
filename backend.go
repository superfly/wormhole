package wormhole

import "github.com/garyburd/redigo/redis"

// BackendIDFromToken ...
func BackendIDFromToken(token string) (string, error) {
	redisConn := redisPool.Get()
	defer redisConn.Close()

	return redis.String(redisConn.Do("HGET", "backend_tokens", token))
}
