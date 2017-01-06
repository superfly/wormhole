package wormhole

import (
	"encoding/hex"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"

	log "github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/superfly/smux"
	handler "github.com/superfly/wormhole/remote"
	"github.com/superfly/wormhole/session"
	config "github.com/superfly/wormhole/shared"
	"github.com/superfly/wormhole/utils"
	kcp "github.com/xtaci/kcp-go"
)

var (
	listenPort    = os.Getenv("PORT")
	nodeID        = os.Getenv("NODE_ID")
	redisURL      = os.Getenv("REDIS_URL")
	localhost     = os.Getenv("LOCALHOST")
	privateKey    = os.Getenv("PRIVATE_KEY")
	sessions      map[string]session.Session
	redisPool     *redis.Pool
	kcpln         *kcp.Listener
	sshPrivateKey []byte
	sshPort       = "22222"
)

// StartRemote ...
func StartRemote(pass, ver string) {
	passphrase = pass
	version = ver
	ensureRemoteEnvironment()
	go handleDeath()

	handler := &handler.SshHandler{
		Port:       sshPort,
		PrivateKey: sshPrivateKey,
	}
	/*
		handler := &handler.SmuxHandler{
			Passphrase: passphrase,
			ListenPort: listenPort,
		}
	*/
	err := handler.InitializeConnection()
	if err != nil {
		log.Fatal(err)
	}
	defer handler.Close()

	handler.ListenAndServe(sshSessionHandler)
	//handler.ListenAndServe(sessionHandler)
}

func ensureRemoteEnvironment() {
	ensureEnvironment()
	if privateKey == "" {
		log.Fatalln("PRIVATE_KEY is required.")
	}
	privateKeyBytes, err := hex.DecodeString(privateKey)
	if err != nil {
		log.Fatalf("PRIVATE_KEY needs to be in hex format. Details: %s", err.Error())
	}
	if len(privateKeyBytes) != config.SecretLength {
		log.Fatalf("PRIVATE_KEY needs to be %d bytes long\n", config.SecretLength)
	}
	copy(smuxConfig.ServerPrivateKey[:], privateKeyBytes)

	sshPrivateKeyFile := os.Getenv("SSH_PRIVATE_KEY")
	sshPrivateKey, err = ioutil.ReadFile(sshPrivateKeyFile)
	if err != nil {
		log.Fatalf("Failed to load private key (%s)", sshPrivateKeyFile)
	}

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

	sessions = make(map[string]session.Session)
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
			for id, session := range sessions {
				session.Close()
				delete(sessions, id)
			}
			log.Print("Cleaned up connections.")
			os.Exit(1)
		}
	}(c)
}

func sessionHandler(conn io.ReadWriteCloser) {
	mux, err := smux.EncryptedServer(conn, smuxConfig)
	if err != nil {
		log.Errorln(err)
		return
	}
	defer mux.Close()

	sess := session.NewSmuxSession(nodeID, redisPool, sessions, mux)
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

func sshSessionHandler(conn net.Conn, config *ssh.ServerConfig) {
	// Before use, a handshake must be performed on the incoming net.Conn.
	sess := session.NewSshSession(nodeID, redisPool, sessions, conn, config)
	err := sess.RequireStream()
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
		ReleaseID: "none", // we don't have Release over SSH yet - sess.Release.ID,
	}

	if err = endpoint.Register(); err != nil {
		log.Errorln("Error registering endpoint:", err)
		return
	}
	defer endpoint.Remove()

	sess.EndpointAddr = endpoint.Socket
	go sess.UpdateAttribute("endpoint_addr", endpoint.Socket)
	log.Println("Listening on:", endpoint.Socket)

	sess.HandleRequests(ln)
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

	go utils.CopyCloseIO(tcpConn, stream)
	return nil
}
