package main

import (
	"flag"
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/superfly/wormhole"
	"github.com/superfly/wormhole/config"
)

func main() {
	serverMode := flag.Bool("server", false, "Run the wormhole in server mode.")
	versionFlag := flag.Bool("version", false, "Display wormhole version.")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("wormhole %s\n", config.Version())
		return
	}

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
