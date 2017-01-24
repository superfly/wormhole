package main

import (
	"flag"

	log "github.com/Sirupsen/logrus"
	"github.com/superfly/wormhole"
)

var (
	version string // populated by build server
)

func main() {
	serverMode := flag.Bool("server", false, "Run the wormhole in server mode.")
	proto := flag.String("proto", "ssh", "Choose wormhole transport layer from: ssh, tcp")
	flag.Parse()

	tunnelProto := wormhole.ParseTunnelProto(*proto)
	if tunnelProto == wormhole.UNSUPPORTED {
		log.Fatalf("Unsupported wormhole transport layer: %s", *proto)
	}

	config := &wormhole.Config{
		Protocol: tunnelProto,
		Version:  version,
	}

	if *serverMode {
		wormhole.StartRemote(config)
	} else {
		wormhole.StartLocal(config)
	}
}
