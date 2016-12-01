package main

import (
	"errors"
	"net"
	"time"

	log "github.com/Sirupsen/logrus"

	msgpack "gopkg.in/vmihailenco/msgpack.v2"

	"github.com/garyburd/redigo/redis"
	"github.com/rs/xid"
	"github.com/superfly/wormhole"
	"github.com/xtaci/smux"
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

	stream *smux.Stream
	mux    *smux.Session

	close chan bool
}

// RegisterConnection ...
func (session *Session) RegisterConnection(t time.Time) error {
	redisConn := redisPool.Get()
	defer redisConn.Close()

	sessions[session.ID] = session

	redisConn.Send("MULTI")
	redisConn.Send("HMSET", redis.Args{}.Add(session.redisKey()).AddFlat(session)...)
	redisConn.Send("ZADD", connectedSessionsKey, timeToScore(t), session.ID)
	redisConn.Send("SADD", "node:"+nodeID+":sessions", session.ID)
	_, err := redisConn.Do("EXEC")

	return err
}

// RegisterDisconnection ...
func (session *Session) RegisterDisconnection() error {
	t := time.Now()
	redisConn := redisPool.Get()
	defer redisConn.Close()

	redisConn.Send("MULTI")
	redisConn.Send("ZADD", disconnectedSessionsKey, timeToScore(t), session.ID)
	redisConn.Send("SREM", "node:"+nodeID+":sessions", session.ID)
	_, err := redisConn.Do("EXEC")
	return err
}

// Close ...
func (session *Session) Close() {
	session.mux.Close()
}

// IsClosed ...
func (session *Session) IsClosed() bool {
	return session.mux.IsClosed()
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

// NewSession ...
func NewSession(mux *smux.Session) *Session {
	return &Session{
		ID:    xid.New().String(),
		mux:   mux,
		close: make(chan bool),
	}
}

// RequireStream ...
func (session *Session) RequireStream() error {
	stream, err := session.mux.AcceptStream()
	err = session.setStream(stream)
	return err
}

func (session *Session) setStream(stream *smux.Stream) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("stream ded?")
		}
	}()
	session.stream = stream
	_, port, err := net.SplitHostPort(stream.RemoteAddr().String())
	if err != nil {
		return
	}
	host, _, err := net.SplitHostPort(stream.LocalAddr().String())
	if err != nil {
		return
	}
	session.ClientAddr = host + ":" + port
	return
}

// RequireAuthentication ...
func (session *Session) RequireAuthentication() error {
	var am wormhole.AuthMessage
	log.Println("Waiting for auth message...")
	buf := make([]byte, 1024)
	nr, err := session.stream.Read(buf)
	if err != nil {
		return errors.New("error reading from stream: " + err.Error())
	}
	err = msgpack.Unmarshal(buf[:nr], &am)
	if err != nil {
		return errors.New("unparsable auth message")
	}

	var resp wormhole.Response

	session.Client = am.Client
	session.NodeID = nodeID

	backendID, err := BackendIDFromToken(am.Token)
	if err != nil {
		resp.Ok = false
		resp.Errors = []string{"Error retrieving token."}
		session.sendResponse(&resp)
		return errors.New("error retrieving token: " + err.Error())
	}
	if backendID == "" {
		resp.Ok = false
		resp.Errors = []string{"Token not found."}
		session.sendResponse(&resp)
		return errors.New("could not find token")
	}

	session.BackendID = backendID
	resp.Ok = true

	go session.RegisterConnection(time.Now())

	session.sendResponse(&resp)

	return nil
}

func (session *Session) sendResponse(resp *wormhole.Response) error {
	buf, err := msgpack.Marshal(resp)
	if err != nil {
		return errors.New("error marshalling response: " + err.Error())
	}
	_, err = session.stream.Write(buf)
	if err != nil {
		return errors.New("error writing response: " + err.Error())
	}
	return nil
}

// LivenessLoop ...
func (session *Session) LivenessLoop() {
	err := wormhole.InitPing(session.stream)
	if err != nil {
		log.Errorln("liveness broken:", err)
	}
}
