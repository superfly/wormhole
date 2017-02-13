package main

import (
	"flag"

	log "github.com/Sirupsen/logrus"
	"github.com/superfly/wormhole"
	"github.com/superfly/wormhole/config"
)

func main() {
	serverMode := flag.Bool("server", false, "Run the wormhole in server mode.")
	flag.Parse()

	if *serverMode {
		config, err := config.NewServerConfig()
		if err != nil {
			log.Fatalf("config error: %s", err.Error())
		}
		wormhole.StartRemote(config)
	} else {
		config, err := config.NewClientConfig()
		if err != nil {
			log.Fatalf("config error: %s", err.Error())
		}
		wormhole.StartLocal(config)
	}
}
