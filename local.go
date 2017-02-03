package wormhole

import (
	"flag"
	"net"
	"os"
	"strings"
	"time"

	"github.com/jpillora/backoff"

	"github.com/superfly/wormhole/local"
)

const (
	defaultLocalPort      = "5000"
	defaultLocalHost      = "127.0.0.1"
	defaultRemoteEndpoint = "wormhole.fly.io:30000"
	localServerRetry      = 200 * time.Millisecond // how often to retry local server until ready
	maxWormholeBackoff    = 2 * time.Minute        // max backoff between retries to wormhole server
)

var (
	localEndpoint  = os.Getenv("LOCAL_ENDPOINT")
	port           = os.Getenv("PORT")
	remoteEndpoint = os.Getenv("REMOTE_ENDPOINT")
	flyToken       = os.Getenv("FLY_TOKEN")
	releaseIDVar   = os.Getenv("FLY_RELEASE_ID_VAR")
	releaseDescVar = os.Getenv("FLY_RELEASE_DESC_VAR")
)

func ensureLocalEnvironment() {
	ensureEnvironment()
	if flyToken == "" {
		log.Fatalln("FLY_TOKEN is required, please set this environment variable.")
	}

	if releaseIDVar == "" {
		releaseIDVar = "FLY_RELEASE_ID"
	}

	if releaseDescVar == "" {
		releaseDescVar = "FLY_RELEASE_DESC"
	}

	if remoteEndpoint == "" {
		remoteEndpoint = defaultRemoteEndpoint
	}

	if localEndpoint == "" {
		if port == "" {
			localEndpoint = defaultLocalHost + ":" + defaultLocalPort
		} else {
			localEndpoint = defaultLocalHost + ":" + port
		}
	}
}

// StartLocal ...
func StartLocal(cfg *Config) {
	ensureLocalEnvironment()
	release, err := computeRelease(releaseIDVar, releaseDescVar)
	if err != nil {
		log.Warn(err)
	}
	log.Debugln("Computed release:", release)

	var handler local.ConnectionHandler

	switch cfg.Protocol {
	case SSH:
		handler = local.NewSSHHandler(flyToken, remoteEndpoint, localEndpoint, cfg.Version, release)
	case TCP:
		handler = &local.TCPHandler{
			FlyToken:       flyToken,
			RemoteEndpoint: remoteEndpoint,
			LocalEndpoint:  localEndpoint,
			Release:        release,
			Version:        version,
		}

	default:
		log.Fatal("Unknown wormhole transport layer protocol selected.")
	}
	args := flag.Args()
	if len(args) > 0 {
		cmd := strings.Join(args, " ")
		process := NewProcess(cmd, handler)
		err := process.Run()
		if err != nil {
			log.Fatalf("Error running program: %s", err.Error())
			return
		}

		for {
			conn, err := net.Dial("tcp", localEndpoint)
			if conn != nil {
				conn.Close()
			}
			if err == nil {
				log.Println("Local server is ready on:", localEndpoint)
				break
			}
			time.Sleep(localServerRetry)
		}
	}

	b := &backoff.Backoff{
		Max: 2 * time.Minute,
	}

	for {
		err := handler.ListenAndServe()
		if err != nil {
			log.Error(err)
			d := b.Duration()
			time.Sleep(d)
			continue
		}
		b.Reset()
	}
}
