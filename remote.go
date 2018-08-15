package wormhole

import (
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/sirupsen/logrus"
	"github.com/superfly/wormhole/config"
	wnet "github.com/superfly/wormhole/net"
	handler "github.com/superfly/wormhole/remote"
	"github.com/superfly/wormhole/session"
	tlsc "github.com/superfly/wormhole/tls"
)

var (
	redisPool *redis.Pool
	log       *logrus.Entry
)

// StartRemote ...
func StartRemote(cfg *config.ServerConfig) {
	log = cfg.Logger.WithFields(logrus.Fields{"prefix": "wormhole"})
	ensureRemoteEnvironment(cfg)

	registry := session.NewRegistry(cfg.Logger)

	var h handler.Handler
	var err error
	server := &handler.Server{Logger: cfg.Logger}

	listenerFactory, err := listenerFactoryFromConfig(registry, cfg)
	if err != nil {
		log.Fatalf("Could not create listener factory: %+v", err)
	}

	switch cfg.Protocol {
	case config.SSH:
		h, err = handler.NewSSHHandler(cfg, registry, redisPool, listenerFactory)
		if err != nil {
			log.Fatal(err)
		}
	case config.TCP:
		h, err = handler.NewTCPHandler(cfg, registry, redisPool, listenerFactory)
		if err != nil {
			log.Fatal(err)
		}
	case config.HTTP2:
		h, err = handler.NewHTTP2Handler(cfg, registry, redisPool, listenerFactory)
		if err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatal("Unknown wormhole transport layer protocol selected.")
	}

	go handleDeath(h, registry)
	server.ListenAndServe(":"+cfg.Port, h)
}

func listenerFactoryFromConfig(registry *session.Registry, cfg *config.ServerConfig) (wnet.ListenerFactory, error) {
	multiPortFactoryArgs := &wnet.MultiPortTCPListenerFactoryArgs{
		BindAddr: "0.0.0.0",
		Logger:   cfg.Logger,
	}

	listenerFactory, err := wnet.NewMultiPortTCPListenerFactory(multiPortFactoryArgs)
	if err != nil {
		return nil, err
	}

	if cfg.UseSharedPortForwarding {
		tlsconf, err := tlsc.NewConfig(cfg.SharedPortTLSCert, cfg.SharedPortTLSPrivateKey, registry)
		if err != nil {
			return nil, err
		}

		sharedArgs := &wnet.SharedPortTLSListenerFactoryArgs{
			Address:   ":" + cfg.SharedTLSForwardingPort,
			Logger:    cfg.Logger,
			TLSConfig: tlsconf.GetDefaultConfig(),
			RedisPool: redisPool,
		}
		sharedL, err := wnet.NewSharedPortTLSListenerFactory(sharedArgs)
		if err != nil {
			return nil, err
		}

		fanInArgs := &wnet.FanInListenerFactoryArgs{
			Factories: []wnet.FanInListenerFactoryEntry{
				{
					Factory:       sharedL,
					ShouldCleanup: true,
				},
				{
					Factory:       listenerFactory,
					ShouldCleanup: true,
				},
			},
			Logger: cfg.Logger,
		}
		fanInListener, err := wnet.NewFanInListenerFactory(fanInArgs)
		if err != nil {
			return nil, err
		}

		listenerFactory = fanInListener
	}
	return listenerFactory, nil
}

func ensureRemoteEnvironment(cfg *config.ServerConfig) {
	var err error

	redisPool = newRedisPool(cfg.RedisURL)

	redisConn := redisPool.Get()
	defer redisConn.Close()
	_, err = redisConn.Do("PING")
	if err != nil {
		log.Fatalf("Couldn't connect to Redis: %s", err.Error())
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
func handleDeath(h handler.Handler, r *session.Registry) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func(c <-chan os.Signal) {
		for range c {
			log.Print("Cleaning up before exit...")
			h.Close()
			r.Close()
			log.Print("Cleaned up connections.")
			os.Exit(1)
		}
	}(c)
}
