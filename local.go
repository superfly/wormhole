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
		handler = local.NewSSHHandler(cfg.Token, cfg.RemoteEndpoint, cfg.LocalEndpoint, cfg.Version, release)
	case config.TCP:
		handler = local.NewTCPHandler(cfg.Token, cfg.RemoteEndpoint, cfg.LocalEndpoint, cfg.Version, release)
	case config.TLS:
		handler, err = local.NewTLSHandler(cfg.Token, cfg.RemoteEndpoint, cfg.LocalEndpoint, cfg.Version, cfg.TLSCert, release)
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

	for {
		conn, err := net.Dial("tcp", cfg.LocalEndpoint)
		if conn != nil {
			conn.Close()
		}
		if err == nil {
			log.Println("Local server is ready on:", cfg.LocalEndpoint)
			break
		}
		time.Sleep(localServerRetry)
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
