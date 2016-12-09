package main

import (
	"flag"

	"github.com/superfly/wormhole"
)

var (
	passphrase string
	version    string
	serverMode = flag.Bool("server", false, "Run the wormhole in server mode.")
)

func main() {
	flag.Parse()
	if *serverMode {
		wormhole.StartRemote(passphrase, version)
	} else {
		wormhole.StartLocal(passphrase, version)
	}
}
