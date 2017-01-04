package session

import (
	"errors"
	"time"

	log "github.com/Sirupsen/logrus"

	msgpack "gopkg.in/vmihailenco/msgpack.v2"

	"github.com/garyburd/redigo/redis"
	"github.com/rs/xid"
	"github.com/superfly/smux"
	"github.com/superfly/wormhole/messages"
)

const (
	connectedSessionsKey    = "sessions:connected"
	disconnectedSessionsKey = "sessions:disconnected"
)

// Session ...
type SmuxSession struct {
	ID     string `redis:"id,omitempty"`
	Client string `redis:"client,omitempty"`
	NodeID string `redis:"node_id,omitempty"`

	BackendID string `redis:"backend_id,omitempty"`

	ClientAddr   string `redis:"client_addr,omitempty"`
	EndpointAddr string `redis:"endpoint_addr,omitempty"`

	Release *messages.Release

	stream *smux.Stream
	mux    *smux.Session

	sessions  map[string]Session
	redisPool *redis.Pool
	nodeID    string

	close chan bool
}

// RegisterConnection ...
func (s *SmuxSession) RegisterConnection(t time.Time) error {
	redisConn := s.redisPool.Get()
	defer redisConn.Close()

	s.sessions[s.ID] = s

	redisConn.Send("MULTI")
	redisConn.Send("HMSET", redis.Args{}.Add(s.redisKey()).AddFlat(s)...)
	redisConn.Send("ZADD", connectedSessionsKey, timeToScore(t), s.ID)
	redisConn.Send("SADD", "node:"+s.nodeID+":sessions", s.ID)
	redisConn.Send("SADD", "backend:"+s.BackendID+":sessions", s.ID)
	redisConn.Send("ZADD", "backend:"+s.BackendID+":releases", "NX", timeToScore(t), s.Release.ID)
	redisConn.Send("HMSET", redis.Args{}.Add("backend:"+s.BackendID+":release:"+s.Release.ID).AddFlat(s.Release)...)
	_, err := redisConn.Do("EXEC")

	return err
}

// RegisterDisconnection ...
func (s *SmuxSession) RegisterDisconnection() error {
	t := time.Now()
	redisConn := s.redisPool.Get()
	defer redisConn.Close()

	redisConn.Send("MULTI")
	redisConn.Send("ZADD", disconnectedSessionsKey, timeToScore(t), s.ID)
	redisConn.Send("SREM", "node:"+s.nodeID+":sessions", s.ID)
	redisConn.Send("SREM", "backend:"+s.BackendID+":sessions", s.ID)
	_, err := redisConn.Do("EXEC")
	return err
}

// Close ...
func (s *SmuxSession) Close() {
	s.mux.Close()
	s.RegisterDisconnection()
}

// IsClosed ...
func (s *SmuxSession) IsClosed() bool {
	return s.mux.IsClosed()
}

// UpdateAttribute ...
func (s *SmuxSession) UpdateAttribute(name string, value interface{}) error {
	redisConn := s.redisPool.Get()
	defer redisConn.Close()

	_, err := redisConn.Do("HSET", s.redisKey(), name, value)
	return err
}

func (s *SmuxSession) redisKey() string {
	return "session:" + s.ID
}

func timeToScore(t time.Time) int64 {
	return t.UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
}

// NewSmuxSession ...
func NewSmuxSession(nodeID string, redisPool *redis.Pool, sessions map[string]Session, mux *smux.Session) *SmuxSession {
	return &SmuxSession{
		ID:        xid.New().String(),
		mux:       mux,
		close:     make(chan bool),
		sessions:  sessions,
		redisPool: redisPool,
		nodeID:    nodeID,
	}
}

// RequireStream ...
func (s *SmuxSession) RequireStream() error {
	stream, err := s.mux.AcceptStream()
	if err != nil {
		return err
	}
	err = s.setStream(stream)
	return err
}

func (s *SmuxSession) setStream(stream *smux.Stream) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("stream ded?")
		}
	}()
	s.stream = stream
	s.ClientAddr = stream.RemoteAddr().String() // sometime panics
	return
}

// RequireAuthentication ...
func (s *SmuxSession) RequireAuthentication() error {
	var am messages.AuthMessage
	log.Println("Waiting for auth message...")
	buf := make([]byte, 1024)
	nr, err := s.stream.Read(buf)
	if err != nil {
		return errors.New("error reading from stream: " + err.Error())
	}
	err = msgpack.Unmarshal(buf[:nr], &am)
	if err != nil {
		return errors.New("unparsable auth message")
	}

	var resp messages.Response

	s.Client = am.Client
	s.NodeID = s.nodeID
	s.Release = am.Release

	backendID, err := s.backendIDFromToken(am.Token)
	if err != nil {
		resp.Ok = false
		resp.Errors = []string{"Error retrieving token."}
		s.sendResponse(&resp)
		return errors.New("error retrieving token: " + err.Error())
	}
	if backendID == "" {
		resp.Ok = false
		resp.Errors = []string{"Token not found."}
		s.sendResponse(&resp)
		return errors.New("could not find token")
	}

	s.BackendID = backendID
	resp.Ok = true

	go s.RegisterConnection(time.Now())

	s.sendResponse(&resp)

	return nil
}

func (s *SmuxSession) sendResponse(resp *messages.Response) error {
	buf, err := msgpack.Marshal(resp)
	if err != nil {
		return errors.New("error marshalling response: " + err.Error())
	}
	_, err = s.stream.Write(buf)
	if err != nil {
		return errors.New("error writing response: " + err.Error())
	}
	return nil
}

// LivenessLoop ...
func (s *SmuxSession) LivenessLoop() {
	err := initPing(s.stream)
	if err != nil {
		log.Errorln("PING error", s.stream.RemoteAddr().String(), "because:", err)
		s.mux.Close()
	}
}

func initPing(stream *smux.Stream) (err error) {
	time.Sleep(5 * time.Second)
	for {
		stream.Write([]byte("ping"))
		stream.SetDeadline(time.Now().Add(5 * time.Second))
		readbuf := make([]byte, 4)
		_, err = stream.Read(readbuf)
		if err != nil {
			break
		}
		if string(readbuf) != "pong" {
			err = errors.New("Unexpected response to ping: " + string(readbuf))
			break
		}
		time.Sleep(1 * time.Second)
	}
	return err
}

func (s *SmuxSession) backendIDFromToken(token string) (string, error) {
	redisConn := s.redisPool.Get()
	defer redisConn.Close()

	return redis.String(redisConn.Do("HGET", "backend_tokens", token))
}
