package main

import (
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"time"

	msgpack "gopkg.in/vmihailenco/msgpack.v2"

	"github.com/garyburd/redigo/redis"
	"github.com/rs/xid"
	kcp "github.com/xtaci/kcp-go"
	"github.com/xtaci/smux"

	log "github.com/Sirupsen/logrus"

	"github.com/superfly/wormhole"
)

const (
	noDelay      = 0
	interval     = 30
	resend       = 2
	noCongestion = 1
	maxBuffer    = 4194304
)

var (
	port           = os.Getenv("PORT")
	nodeID         = os.Getenv("NODE_ID")
	redisURL       = os.Getenv("REDIS_URL")
	logLevel       = os.Getenv("LOG_LEVEL")
	controlStreams map[string]*ControlStream
	redisPool      *redis.Pool
	smuxConfig     *smux.Config
)

func init() {
	if port == "" {
		port = "10000"
	}
	if redisURL == "" {
		panic("REDIS_URL is required.")
	}

	redisPool = newRedisPool(redisURL)

	if logLevel == "" {
		log.SetLevel(log.InfoLevel)
	} else if logLevel == "debug" {
		log.SetLevel(log.DebugLevel)
	}

	if nodeID == "" {
		nodeID, _ = os.Hostname()
	}
	// smux conf
	smuxConfig = smux.DefaultConfig()
	smuxConfig.MaxReceiveBuffer = maxBuffer

	// logging
	textFormatter := &log.TextFormatter{FullTimestamp: true}
	log.SetFormatter(textFormatter)
}

func main() {
	ln, err := kcp.ListenWithOptions(":"+port, nil, 10, 3)
	if err != nil {
		panic(err)
	}
	defer ln.Close()
	log.Println("Listening on", ln.Addr().String())
	for {
		kcpconn, err := ln.AcceptKCP()
		if err != nil {
			log.Errorln(err)
			break
		}
		go handleConn(kcpconn)
		log.Println("Accepted connection from:", kcpconn.RemoteAddr())
	}
	log.Println("Stopping server KCP...")
}

func setConnOptions(kcpconn *kcp.UDPSession) {
	kcpconn.SetStreamMode(true)
	kcpconn.SetNoDelay(noDelay, interval, resend, noCongestion)
	kcpconn.SetMtu(1350)
	kcpconn.SetWindowSize(1024, 1024)
	kcpconn.SetACKNoDelay(true)
	kcpconn.SetKeepAlive(10)
}

func handleConn(kcpconn *kcp.UDPSession) {
	defer kcpconn.Close()
	setConnOptions(kcpconn)

	mux, err := smux.Server(kcpconn, smuxConfig)
	if err != nil {
		log.Errorln(err)
		return
	}
	defer mux.Close()

	sess, err := getControlStream(mux)
	if err != nil {
		log.Errorln(err)
		return
	}

	log.Println("Client authenticated.")

	ln, err := listenTCP()
	if err != nil {
		log.Errorln(err)
		return
	}
	defer ln.Close()

	go sess.UpdateAttribute("endpoint_addr", ln.Addr().String())
	log.Println("Listening on:", ln.Addr().String())

	for {
		tcpConn, err := ln.AcceptTCP()
		if err != nil {
			log.Errorln("Could not accept tcp conn:", err)
			break
		}
		// log.Debugln("Accepted tcp connection from:", tcpConn.RemoteAddr())

		err = handleTCPConn(mux, tcpConn)
		if err != nil {
			log.Error(err)
			break
		}
	}

	go sess.RegisterDisconnection(time.Now())
	log.Println("Session ended.")
}

func listenTCP() (*net.TCPListener, error) {
	addr, err := net.ResolveTCPAddr("tcp4", ":0")
	if err != nil {
		return nil, errors.New("could not parse TCP addr: " + err.Error())
	}
	ln, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, errors.New("could not listen on: " + err.Error())
	}
	return ln, nil
}

func getControlStream(mux *smux.Session) (*Session, error) {
	stream, err := mux.AcceptStream()
	if err != nil {
		log.Errorln(err)
	}
	defer stream.Close()

	var am wormhole.AuthMessage
	log.Println("Waiting for auth message...")
	buf := make([]byte, 1024)
	nr, err := stream.Read(buf)
	if err != nil {
		return nil, errors.New("error reading from stream: " + err.Error())
	}
	err = msgpack.Unmarshal(buf[:nr], &am)
	if err != nil {
		return nil, errors.New("unparsable auth message")
	}

	var resp wormhole.Response

	sess := Session{
		ID:         xid.New().String(),
		Client:     am.Client,
		NodeID:     nodeID,
		ClientAddr: stream.RemoteAddr().String(),
	}

	backendID, err := BackendIDFromToken(am.Token)
	if err != nil {
		resp.Ok = false
		resp.Errors = []string{"Error retrieving token."}
		sendResponse(&resp, stream)
		return nil, errors.New("error retrieving token: " + err.Error())
	}
	if backendID == "" {
		resp.Ok = false
		resp.Errors = []string{"Token not found."}
		sendResponse(&resp, stream)
		return nil, errors.New("could not find token")
	}

	sess.BackendID = backendID
	resp.Ok = true

	go sess.RegisterConnection(time.Now())

	sendResponse(&resp, stream)

	return &sess, nil
}

func sendResponse(resp *wormhole.Response, stream *smux.Stream) error {
	buf, err := msgpack.Marshal(resp)
	if err != nil {
		return errors.New("error marshalling response: " + err.Error())
	}
	_, err = stream.Write(buf)
	if err != nil {
		return errors.New("error writing response: " + err.Error())
	}
	return nil
}

func handleTCPConn(mux *smux.Session, tcpConn *net.TCPConn) error {
	if err := tcpConn.SetReadBuffer(maxBuffer); err != nil {
		return err
	}
	if err := tcpConn.SetWriteBuffer(maxBuffer); err != nil {
		return err
	}

	stream, err := mux.OpenStream()
	if err != nil {
		return err
	}
	log.Info("Opened a stream...")

	go handleClient(tcpConn, stream)
	return nil
}

func handleClient(c1, c2 io.ReadWriteCloser) {
	defer c1.Close()
	defer c2.Close()

	// start tunnel
	c1die := make(chan struct{})
	go func() { io.Copy(c1, c2); close(c1die) }()

	c2die := make(chan struct{})
	go func() { io.Copy(c2, c1); close(c2die) }()

	// wait for tunnel termination
	select {
	case <-c1die:
	case <-c2die:
	}
}

func newRedisPool(redisURL string) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			conn, err := redis.DialURL(redisURL)
			if err != nil {
				return nil, err
			}

			parsedURL, err := url.Parse(redisURL)
			if err != nil {
				return nil, err
			}
			if parsedURL.User != nil {
				if password, hasPassword := parsedURL.User.Password(); hasPassword == true {
					if _, authErr := conn.Do("AUTH", password); authErr != nil {
						conn.Close()
						return nil, authErr
					}
				}
			}
			return conn, nil
		},
		TestOnBorrow: func(conn redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := conn.Do("PING")
			return err
		},
	}
}
