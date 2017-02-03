package local

import (
	"os"

	"github.com/Sirupsen/logrus"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
)

var logger = logrus.New()
var log *logrus.Entry

func init() {
	logger.Formatter = new(prefixed.TextFormatter)
	if os.Getenv("LOG_LEVEL") == "debug" {
		logger.Level = logrus.DebugLevel
	}
	log = logger.WithFields(logrus.Fields{
		"prefix": "TCPHandler",
	})
}

// ConnectionHandler specifies interface for handler connecting to wormhole server
type ConnectionHandler interface {
	ListenAndServe() error
	Close() error
}
