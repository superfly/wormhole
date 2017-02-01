package wormhole

import (
	"os"

	"github.com/Sirupsen/logrus"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
)

var (
	version string

	logLevel = os.Getenv("LOG_LEVEL")
	logger   = logrus.New()
	log      *logrus.Entry
)

func init() {
	logger.Formatter = new(prefixed.TextFormatter)
	if logLevel == "" {
		logger.Level = logrus.InfoLevel
	} else if logLevel == "debug" {
		logger.Level = logrus.DebugLevel
	}
	log = logger.WithFields(logrus.Fields{
		"prefix": "wormhole",
	})
}

func ensureEnvironment() {
	if version == "" {
		version = "latest"
	}
}
