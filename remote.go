package wormhole

import (
	"errors"
	"net"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	kcp "github.com/xtaci/kcp-go"
	"github.com/xtaci/smux"
)

var (
	listenPort = os.Getenv("PORT")
	nodeID     = os.Getenv("NODE_ID")
	redisURL   = os.Getenv("REDIS_URL")
	localhost  = os.Getenv("LOCALHOST")
	sessions   map[string]*Session
	redisPool  *redis.Pool
	kcpln      *kcp.Listener
)

// StartRemote ...
func StartRemote() {
	ensureEnvironment()
	go handleDeath()
	block, _ := kcp.NewAESBlockCrypt([]byte(passphrase)[:32])
	ln, err := kcp.ListenWithOptions(":"+listenPort, block, 10, 3)

	if err = ln.SetDSCP(DSCP); err != nil {
		log.Warnln("SetDSCP:", err)
	}
	if err = ln.SetReadBuffer(MaxBuffer); err != nil {
		log.Fatalln("SetReadBuffer:", err)
	}
	if err = ln.SetWriteBuffer(MaxBuffer); err != nil {
		log.Fatalln("SetWriteBuffer:", err)
	}
	kcpln = ln
	if err != nil {
		panic(err)
	}
	defer kcpln.Close()
	log.Println("Listening on", kcpln.Addr().String())

	go DebugSNMP()

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

func ensureEnvironment() {
	if listenPort == "" {
		listenPort = "10000"
	}
	if redisURL == "" {
		log.Fatalln("REDIS_URL is required.")
	}

	redisPool = newRedisPool(redisURL)

	if nodeID == "" {
		nodeID, _ = os.Hostname()
	}

	sessions = make(map[string]*Session)

	if version == "" {
		version = "latest"
	}
	if passphrase == "" {
		passphrase = os.Getenv("PASSPHRASE")
		if passphrase == "" {
			log.Fatalln("PASSPHRASE needs to be set")
		}
	}

}

func setRemoteConnOptions(kcpconn *kcp.UDPSession) {
	kcpconn.SetStreamMode(true)
	kcpconn.SetNoDelay(NoDelay, Interval, Resend, NoCongestion)
	kcpconn.SetMtu(1350)
	kcpconn.SetWindowSize(128, 1024)
	kcpconn.SetACKNoDelay(true)
	kcpconn.SetKeepAlive(KeepAlive)
}

func handleConn(kcpconn *kcp.UDPSession) {
	defer kcpconn.Close()
	setRemoteConnOptions(kcpconn)

	mux, err := smux.Server(kcpconn, smuxConfig)
	if err != nil {
		log.Errorln(err)
		return
	}
	defer mux.Close()

	sess := NewSession(mux)
	err = sess.RequireStream()
	if err != nil {
		log.Errorln("error getting a stream:", err)
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
		ReleaseID: sess.Release.ID,
	}

	if err = endpoint.Register(); err != nil {
		log.Errorln("Error registering endpoint:", err)
		return
	}
	defer endpoint.Remove()

	sess.EndpointAddr = endpoint.Socket
	go sess.UpdateAttribute("endpoint_addr", endpoint.Socket)
	log.Println("Listening on:", endpoint.Socket)

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
	stream, err := mux.OpenStream()
	if err != nil {
		return err
	}
	log.Debug("Opened a stream...")

	go CopyCloseIO(tcpConn, stream)
	return nil
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
				redisConn.Send("SREM", "backend:"+session.BackendID+":sessions", session.ID)
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
