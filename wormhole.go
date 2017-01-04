package wormhole

import (
	"encoding/hex"
	"os"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/superfly/smux"
	config "github.com/superfly/wormhole/shared"
)

var (
	passphrase string
	publicKey  string
	version    string

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

	smuxConfig = smux.DefaultConfig()
	smuxConfig.MaxReceiveBuffer = config.MaxBuffer
	smuxConfig.KeepAliveInterval = config.KeepAlive * time.Second
}

func ensureEnvironment() {
	if passphrase == "" {
		passphrase = os.Getenv("PASSPHRASE")
		if passphrase == "" {
			log.Fatalln("PASSPHRASE needs to be set")
		} else if len([]byte(passphrase)) < config.SecretLength {
			log.Fatalf("PASSPHRASE needs to be at least %d bytes long\n", config.SecretLength)
		}
	}
	if publicKey == "" {
		publicKey = os.Getenv("PUBLIC_KEY")
		if publicKey == "" {
			log.Fatalln("PUBLIC_KEY needs to be set")
		}
	}
	publicKeyBytes, err := hex.DecodeString(publicKey)
	if err != nil {
		log.Fatalf("PUBLIC_KEY needs to be in hex format. Details: %s", err.Error())
	}
	if len(publicKeyBytes) != config.SecretLength {
		log.Fatalf("PUBLIC_KEY needs to be %d bytes long\n", config.SecretLength)
	}
	copy(smuxConfig.ServerPublicKey[:], publicKeyBytes)
	if version == "" {
		version = "latest"
	}
}
