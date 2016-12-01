package main // import "github.com/superfly/wormhole/cmd/remote"

import (
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/garyburd/redigo/redis"
	kcp "github.com/xtaci/kcp-go"
	"github.com/xtaci/smux"

	log "github.com/Sirupsen/logrus"
)

const (
	noDelay      = 0
	interval     = 30
	resend       = 2
	noCongestion = 1
	maxBuffer    = 4194304
)

var (
	port       = os.Getenv("PORT")
	nodeID     = os.Getenv("NODE_ID")
	redisURL   = os.Getenv("REDIS_URL")
	logLevel   = os.Getenv("LOG_LEVEL")
	localhost  = os.Getenv("LOCALHOST")
	sessions   map[string]*Session
	redisPool  *redis.Pool
	smuxConfig *smux.Config
	kcpln      *kcp.Listener
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
	smuxConfig.KeepAliveInterval = 5 * time.Second
	smuxConfig.KeepAliveTimeout = 5 * time.Second

	// logging
	textFormatter := &log.TextFormatter{FullTimestamp: true}
	log.SetFormatter(textFormatter)

	sessions = make(map[string]*Session)
}

func main() {
	go handleDeath()
	ln, err := kcp.ListenWithOptions(":"+port, nil, 10, 3)
	kcpln = ln
	if err != nil {
		panic(err)
	}
	defer kcpln.Close()
	log.Println("Listening on", kcpln.Addr().String())

	for {
		kcpconn, err := kcpln.AcceptKCP()
		if err != nil {
			log.Errorln("error accepting KCP:", err)
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
	kcpconn.SetKeepAlive(5)
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

	sess := NewSession(mux)
	err = sess.RequireStream()
	if err != nil {
		log.Errorln(err)
		return
	}

	err = sess.RequireAuthentication()
	if err != nil {
		log.Errorln(err)
		return
	}

	log.Println("Client authenticated.")

	defer sess.RegisterDisconnection()
	go sess.LivenessLoop()

	ln, err := listenTCP()
	if err != nil {
		log.Errorln(err)
		return
	}
	defer ln.Close()

	_, port, _ := net.SplitHostPort(ln.Addr().String())

	endpoint := &Endpoint{
		BackendID: sess.BackendID,
		SessionID: sess.ID,
		Socket:    localhost + ":" + port,
	}

	if err = endpoint.Register(); err != nil {
		log.Errorln("Error registering endpoint:", err)
		return
	}
	defer endpoint.Remove()

	go sess.UpdateAttribute("endpoint_addr", ln.Addr().String())
	log.Println("Listening on:", ln.Addr().String())

	for {
		ln.SetDeadline(time.Now().Add(time.Second))
		tcpConn, err := ln.AcceptTCP()

		if sess.IsClosed() {
			log.Println("session is closed, breaking listen loop")
			return
		}

		if err != nil {
			netErr, ok := err.(net.Error)

			//If this is a timeout, then continue to wait for
			//new connections
			if ok && netErr.Timeout() && netErr.Temporary() {
				continue
			}
			log.Errorln("Could not accept tcp conn:", err)
			return
		}
		log.Debugln("Accepted tcp connection from:", tcpConn.RemoteAddr())

		err = handleTCPConn(mux, tcpConn)
		if err != nil {
			log.Error(err)
			return
		}
	}
}

func listenTCP() (*net.TCPListener, error) {
	addr, err := net.ResolveTCPAddr("tcp4", ":0")
	if err != nil {
		return nil, errors.New("could not parse TCP addr: " + err.Error())
	}
	ln, err := net.ListenTCP("tcp4", addr)
	if err != nil {
		return nil, errors.New("could not listen on: " + err.Error())
	}
	return ln, nil
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
	log.Debug("Opened a stream...")

	go handleClient(tcpConn, stream)
	return nil
}

func handleClient(c1, c2 io.ReadWriteCloser) {
	defer c1.Close()
	defer c2.Close()

	// start tunnel
	c1die := make(chan struct{})
	go func() {
		_, err := io.Copy(c1, c2)
		if err != nil {
			log.Error(err)
		}
		close(c1die)
	}()

	c2die := make(chan struct{})
	go func() {
		_, err := io.Copy(c2, c1)
		if err != nil {
			log.Error(err)
		}
		close(c2die)
	}()

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

// IT CAN BE HANDLED!
func handleDeath() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func(c <-chan os.Signal) {
		for _ = range c {
			log.Print("Cleaning up before exit...")
			// _ = kcpln.Close()
			// log.Info("Closed KCP listener.")
			redisConn := redisPool.Get()
			defer redisConn.Close()

			t := time.Now()

			// Register their disconnection, massively.
			redisConn.Send("MULTI")
			for id, session := range sessions {
				redisConn.Send("ZADD", disconnectedSessionsKey, timeToScore(t), id)
				redisConn.Send("SREM", "node:"+nodeID+":sessions", id)
				redisConn.Send("SREM", "backend:"+session.BackendID+":endpoints", session.EndpointAddr)
			}
			_, err := redisConn.Do("EXEC")
			if err != nil {
				log.Errorln("Cleaning up redis failed:", err)
			} else {
				log.Print("Cleaned up Redis.")
			}

			// Actually closes the muxes
			for id, session := range sessions {
				if !session.IsClosed() {
					session.Close()
				}
				delete(sessions, id)
			}
			log.Print("Cleaned up connections.")
			os.Exit(1)
		}
	}(c)
}
