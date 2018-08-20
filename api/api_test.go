package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/rafaeljusto/redigomock"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var (
	mockRedisConn *redigomock.Conn
	mockRedisPool *redis.Pool
	handler       *Handler
)

func TestAPIHandlerAuth(t *testing.T) {
	// no auth
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1", nil)
	handler.ServeHTTP(rr, req)
	res := rr.Result()

	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
	assert.Equal(t, "application/json", res.Header.Get("content-type"))

	// wrong format
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1", nil)
	req.Header.Set("authorization", "bad auth")
	handler.ServeHTTP(rr, req)
	res = rr.Result()

	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
	assert.Equal(t, "application/json", res.Header.Get("content-type"))

	// wrong format
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1", nil)
	req.Header.Set("authorization", "Token ")
	handler.ServeHTTP(rr, req)
	res = rr.Result()

	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
	assert.Equal(t, "application/json", res.Header.Get("content-type"))

	// wrong token
	cmd := mockRedisConn.Command("HGET", "backend_tokens", "blah").ExpectError(redis.ErrNil)

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1", nil)
	req.Header.Set("authorization", "Token blah")
	handler.ServeHTTP(rr, req)
	res = rr.Result()

	if mockRedisConn.Stats(cmd) != 1 {
		t.Fatal("Command was not used")
	}

	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
	assert.Equal(t, "application/json", res.Header.Get("content-type"))

	// right token
	cmd = mockRedisConn.Command("HGET", "backend_tokens", "test").Expect("123")

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1", nil)
	req.Header.Set("authorization", "Token test")
	handler.ServeHTTP(rr, req)
	res = rr.Result()

	if mockRedisConn.Stats(cmd) != 1 {
		t.Fatal("Command was not used")
	}

	assert.Equal(t, http.StatusNotFound, res.StatusCode)

	// redis error
	cmd = mockRedisConn.Command("HGET", "backend_tokens", "error").ExpectError(fmt.Errorf("error"))

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1", nil)
	req.Header.Set("authorization", "Token error")
	handler.ServeHTTP(rr, req)
	res = rr.Result()

	if mockRedisConn.Stats(cmd) != 1 {
		t.Fatal("Command was not used")
	}

	assert.Equal(t, http.StatusInternalServerError, res.StatusCode)
	assert.Equal(t, "application/json", res.Header.Get("content-type"))
}

func TestAPIHandlerEndpoints(t *testing.T) {
	cmd := mockRedisConn.Command("HGET", "backend_tokens", "testendpoints").Expect("123")
	cmdEndpoints := mockRedisConn.Command("SMEMBERS", "backend:123:endpoints").ExpectStringSlice("tls:helloworld.wormhole.test:1234")

	now := time.Now().String()
	cmdEndpoint := mockRedisConn.Command("HGETALL", "backend:123:endpoint:tls:helloworld.wormhole.test:1234").ExpectMap(map[string]string{
		"cluster":      "wormhole.test",
		"region":       "test region",
		"connected_at": now,
		"last_seen_at": now,
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/backend/endpoints", nil)
	req.Header.Set("authorization", "Token testendpoints")
	handler.ServeHTTP(rr, req)
	res := rr.Result()

	if mockRedisConn.Stats(cmd) != 1 {
		t.Fatal("Command was not used")
	}
	if mockRedisConn.Stats(cmdEndpoints) != 1 {
		t.Fatal("SMEMBERS Endpoints command was not used")
	}
	if mockRedisConn.Stats(cmdEndpoint) != 1 {
		t.Fatal("HGETALL Endpoint command was not used")
	}

	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "application/json", res.Header.Get("content-type"))
	body, _ := ioutil.ReadAll(res.Body)
	expectedBody, _ := json.Marshal([]map[string]string{{
		"address":      "helloworld.wormhole.test:1234",
		"cluster":      "wormhole.test",
		"region":       "test region",
		"connected_at": now,
		"last_seen_at": now,
	}})
	assert.JSONEq(t, string(expectedBody), string(body))
}

func init() {
	mockRedisConn = redigomock.NewConn()
	mockRedisPool = redis.NewPool(func() (redis.Conn, error) {
		return mockRedisConn, nil
	}, 10)
	handler = NewHandler(logrus.New(), mockRedisPool)
}
