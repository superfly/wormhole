package main // import "github.com/superfly/wormhole/cmd/wormhole"
import (
	"flag"

	"github.com/superfly/wormhole"
)

var (
	serverMode = flag.Bool("server", false, "Run the wormhole in server mode.")
)

func main() {
	flag.Parse()
	if *serverMode {
		wormhole.StartRemote()
	} else {
		wormhole.StartLocal()
	}
}
