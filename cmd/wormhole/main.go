package main

import (
	"flag"

	"github.com/superfly/wormhole"
)

var (
	version    string
	serverMode = flag.Bool("server", false, "Run the wormhole in server mode.")
)

func main() {
	flag.Parse()
	if *serverMode {
		wormhole.StartRemote(version)
	} else {
		wormhole.StartLocal(version)
	}
}
