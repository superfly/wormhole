package wormhole

import (
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/superfly/smux"
)

var (
	version string

	logLevel   = os.Getenv("LOG_LEVEL")
	smuxConfig *smux.Config
)

func init() {
	if logLevel == "" {
		log.SetLevel(log.InfoLevel)
	} else if logLevel == "debug" {
		log.SetLevel(log.DebugLevel)
	}
	// logging
	textFormatter := &log.TextFormatter{FullTimestamp: true}
	log.SetFormatter(textFormatter)
}

func ensureEnvironment() {
	if version == "" {
		version = "latest"
	}
}
