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
	id           string `redis:"id,omitempty"`
	client       string `redis:"client,omitempty"`
	clientAddr   string `redis:"client_addr,omitempty"`
	nodeID       string `redis:"node_id,omitempty"`
	backendID    string `redis:"backend_id,omitempty"`
	EndpointAddr string `redis:"endpoint_addr,omitempty"`

	release *messages.Release

	stream *smux.Stream
	mux    *smux.Session

	sessions map[string]Session
	store    *RedisStore

	close chan bool
}

// NewSmuxSession ...
func NewSmuxSession(nodeID string, redisPool *redis.Pool, sessions map[string]Session, mux *smux.Session) *SmuxSession {
	return &SmuxSession{
		id:       xid.New().String(),
		mux:      mux,
		close:    make(chan bool),
		sessions: sessions,
		store:    NewRedisStore(redisPool),
		nodeID:   nodeID,
	}
}

// RegisterConnection ...
func (s *SmuxSession) RegisterConnection(t time.Time) error {
	s.sessions[s.id] = s
	return s.store.RegisterConnection(s)
}

// RegisterDisconnection ...
func (s *SmuxSession) RegisterDisconnection() error {
	return s.store.RegisterDisconnection(s)
}

// RegisterEndpoint ...
func (s *SmuxSession) RegisterEndpoint() error {
	return s.store.RegisterEndpoint(s)
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
	return s.store.UpdateAttribute(s, name, value)
}

func (s *SmuxSession) ID() string {
	return s.id
}

func (s *SmuxSession) Key() string {
	return "session:" + s.id
}

func (s *SmuxSession) BackendID() string {
	return s.backendID
}

func (s *SmuxSession) Endpoint() string {
	return s.EndpointAddr
}

func (s *SmuxSession) Client() string {
	return s.clientAddr
}

func (s *SmuxSession) NodeID() string {
	return s.nodeID
}

func (s *SmuxSession) Release() *messages.Release {
	return s.release
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
	s.clientAddr = stream.RemoteAddr().String() // sometime panics
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

	s.client = am.Client
	s.release = am.Release

	backendID, err := s.store.BackendIDFromToken(am.Token)
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

	s.backendID = backendID
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
