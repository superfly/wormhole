package main

import (
	"flag"

	log "github.com/Sirupsen/logrus"
	"github.com/superfly/wormhole"
)

func main() {
	serverMode := flag.Bool("server", false, "Run the wormhole in server mode.")
	flag.Parse()

	if *serverMode {
		config, err := wormhole.NewServerConfig()
		if err != nil {
			log.Fatalf("config error: %s", err.Error())
		}
		wormhole.StartRemote(config)
	} else {
		config, err := wormhole.NewClientConfig()
		if err != nil {
			log.Fatalf("config error: %s", err.Error())
		}
		wormhole.StartLocal(config)
	}
}
