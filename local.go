package wormhole

import (
	"flag"
	"net"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jpillora/backoff"

	"github.com/superfly/wormhole/config"
	"github.com/superfly/wormhole/local"
)

const (
	localServerRetry   = 200 * time.Millisecond // how often to retry local server until ready
	minWormholeBackoff = 200 * time.Millisecond // min backoff between retries to wormhole server
	maxWormholeBackoff = 2 * time.Minute        // max backoff between retries to wormhole server
)

// StartLocal ...
func StartLocal(cfg *config.ClientConfig) {
	log := cfg.Logger.WithFields(logrus.Fields{"prefix": "wormhole"})

	release, err := computeRelease(cfg.ReleaseID, cfg.ReleaseDesc, cfg.ReleaseBranch)
	if err != nil {
		log.Warn(err)
	}
	log.Debugln("Computed release:", release)

	var handler local.ConnectionHandler

	switch cfg.Protocol {
	case config.SSH:
		handler = local.NewSSHHandler(cfg, release)
	case config.TCP:
		handler = local.NewTCPHandler(cfg, release)
	case config.TLS:
		handler, err = local.NewTLSHandler(cfg, release)
		if err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatal("Unknown wormhole transport layer protocol selected.")
	}

	args := flag.Args()
	if len(args) > 0 {
		cmd := strings.Join(args, " ")
		process := NewProcess(cfg.Logger, cmd, handler)
		err := process.Run()
		if err != nil {
			log.Fatalf("Error running program: %s", err.Error())
			return
		}
	}

	log.Infoln("Attempting to connect to local server on:", cfg.LocalEndpoint)
	for {
		conn, err := net.Dial("tcp", cfg.LocalEndpoint)
		if conn != nil {
			conn.Close()
		}
		if err == nil {
			log.Infoln("Local server is ready on:", cfg.LocalEndpoint)
			break
		}
		time.Sleep(localServerRetry)
	}

	b := &backoff.Backoff{
		Min:    minWormholeBackoff,
		Max:    maxWormholeBackoff,
		Jitter: true,
	}

	log.Infoln("Attempting to connect to wormhole server on:", cfg.RemoteEndpoint)
	for {
		err := handler.ListenAndServe()
		if err != nil {
			d := b.Duration()
			log.Errorf("Failed to connect to wormhole server: %s. Will try again in %s", err.Error(), d.String())
			time.Sleep(d)
			continue
		}
		log.Debug("Handler exited with no errors. Starting again")
		b.Reset()
	}
}
