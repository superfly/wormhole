package api

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/gomodule/redigo/redis"
	"github.com/sirupsen/logrus"
)

const (
	authHeader        = "authorization"
	authHeaderPrefix  = "Token "
	tlsEndpointPrefix = "tls:"
)

type key uint8

const (
	keyBackendID key = iota
)

var (
	errWrongAuthHeader      = errorResponse{`Authorization header value should use "Token ..." format`}
	errNotFound             = errorResponse{`Not Found`}
	errGenericServerProblem = errorResponse{`Internal Server Error`}
)

// Handler handles the API HTTP server
type Handler struct {
	logger    *logrus.Entry
	router    chi.Router
	redisPool *redis.Pool
}

// NewHandler creates a new API handler
func NewHandler(logger *logrus.Entry, redisPool *redis.Pool) *Handler {
	r := chi.NewRouter()
	h := &Handler{logger: logger, router: r, redisPool: redisPool}
	r.Use(middleware.RequestLogger(&middleware.DefaultLogFormatter{
		Logger: logger,
	}))

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(jsonMiddleware)
		r.Use(h.authMiddleware)
		r.Get("/endpoints", h.endpoints)
	})

	return h
}

type errorResponse struct {
	Error string `json:"error"`
}

func jsonMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("content-type", "application/json")
		next.ServeHTTP(w, req)
	})
}

func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		auth := req.Header.Get(authHeader)
		w.Header().Set("content-type", "application/json")

		if auth == "" || !strings.HasPrefix(auth, authHeaderPrefix) {
			jsonResponse(w, errWrongAuthHeader, http.StatusUnauthorized)
			return
		}

		token := strings.TrimSpace(strings.TrimPrefix(auth, authHeaderPrefix))

		if token == "" {
			jsonResponse(w, errWrongAuthHeader, http.StatusUnauthorized)
			return
		}

		redisConn := h.redisPool.Get()
		defer redisConn.Close()

		backendID, err := redis.String(redisConn.Do("HGET", "backend_tokens", token))
		if err != nil {
			if err == redis.ErrNil {
				jsonResponse(w, errNotFound, http.StatusUnauthorized)
				return
			}
			h.logger.Error(err)
			jsonResponse(w, errGenericServerProblem, http.StatusInternalServerError)
			return
		}

		next.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), keyBackendID, backendID)))
	})
}

func jsonResponse(w http.ResponseWriter, j interface{}, code int) {
	w.WriteHeader(code)
	b, _ := json.Marshal(j)
	w.Write(b)
}

// ServeHTTP serves the wormhole API
func (h *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	h.router.ServeHTTP(w, req)
}

func (h *Handler) endpoints(w http.ResponseWriter, req *http.Request) {
	backendID := req.Context().Value(keyBackendID).(string)

	redisConn := h.redisPool.Get()
	defer redisConn.Close()

	endpoints, err := redis.Strings(redisConn.Do("SMEMBERS", "backend:"+backendID+":endpoints"))
	if err != nil {
		if err == redis.ErrNil {
			jsonResponse(w, errNotFound, http.StatusNotFound)
			return
		}
		h.logger.Error(err)
		jsonResponse(w, errGenericServerProblem, http.StatusInternalServerError)
		return
	}

	goodEndpoints := []map[string]string{}

	if len(endpoints) > 0 {
		for _, ep := range endpoints {
			if strings.HasPrefix(ep, tlsEndpointPrefix) {
				m, err := redis.StringMap(redisConn.Do("HGETALL", "backend:"+backendID+":endpoint:"+ep))
				if err == nil {
					goodEndpoints = append(goodEndpoints, map[string]string{
						"address":      strings.TrimPrefix(ep, tlsEndpointPrefix),
						"cluster":      m["cluster"],
						"connected_at": m["connected_at"],
						"last_seen_at": m["last_seen_at"],
					})
				}
			}
		}

	}
	jsonResponse(w, goodEndpoints, http.StatusOK)
}

// SingleConnListener is a listener that accepts a single, stored, conn
type SingleConnListener struct {
	listener  net.Listener
	conn      net.Conn
	done      bool
	doneMutex sync.Mutex
}

// NewSingleConnListener creates a new single connection listener
func NewSingleConnListener(l net.Listener, c net.Conn) *SingleConnListener {
	return &SingleConnListener{listener: l, conn: c, done: false}
}

// Accept accepts a connection once and then EOFs
func (l *SingleConnListener) Accept() (net.Conn, error) {
	l.doneMutex.Lock()
	if l.done {
		return nil, io.EOF
	}
	l.done = true
	l.doneMutex.Unlock()
	return l.conn, nil
}

// Close does nothing
func (l *SingleConnListener) Close() error {
	return nil
}

// Addr defers to the stored original listener
func (l *SingleConnListener) Addr() net.Addr {
	return l.listener.Addr()
}
