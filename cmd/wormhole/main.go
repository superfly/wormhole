package main

import (
	"flag"
	"fmt"
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/superfly/wormhole"
	"github.com/superfly/wormhole/config"
)

func main() {
	serverMode := flag.Bool("server", false, "Run the wormhole in server mode.")
	versionFlag := flag.Bool("version", false, "Display wormhole version.")
	helpFlag := flag.Bool("help", false, "Show help")
	flag.BoolVar(helpFlag, "h", false, "Show help")
	flag.Parse()

	if *helpFlag {
		fmt.Printf(
			`Wormhole v%s

Usage
=====

  - First, set the required environment variables. (details below)
  - Second, choose a mode to run. (more details below)

    (supervisor)
    wormhole path/to/executable

    (sidecar)
    wormhole

Environment Variables
=====================

  (required)
    FLY_TOKEN: Token for your backend, found on your fly.io backend's page

  (optional)
    FLY_LOCAL_ENDPOINT: Local server to tunnel for. (defaults to "127.0.0.1:5000")
      or
    FLY_PORT: Local port to tunnel. (defaults to 5000)

    FLY_REMOTE_ENDPOINT: Wormhole server instance. Defaults to Fly.io's servers.
    FLY_RELEASE_ID_VAR: ENV var with current released version of your web server (inferred from git if available)
    FLY_RELEASE_DESC_VAR: ENV name with commit message of the current released version of your web server (inferred from git if available)

Modes
=====

  Supervisor (preferred)
  ----------
    
    Wormhole will start your application (the specified executable) and supervise the process.

    Examples:

      export FLY_TOKEN=x # Always set this.

      # Rails, setting a port
      export FLY_PORT=3000 # Defaults to: 5000
      wormhole bundle exec rails server -p 3000

      # Go, setting a full endpoint
      export FLY_ENDPOINT=127.0.0.1:3000 # Defaults to: 127.0.0.1:5000
      wormhole path/to/app

  Sidecar
  -------

    Wormhole assumes your server is already running.
    You need to set the proper environment variables.

    Examples:

      export FLY_TOKEN=x # Always set this.
  
      # Setting a port
      export FLY_PORT=3000 # Defaults to: 5000
      wormhole
  
      # Setting a full endpoint
      export FLY_ENDPOINT=127.0.0.1:3000 # Defaults to: 127.0.0.1:5000
      wormhole

Other commands
==============

  -v, --version: Prints the version for wormhole.
  -h, --help:    Prints this help information.
      --server:  Starts wormhole in server mode. You probably don't want this.

`, config.Version())
		return
	}

	if *versionFlag {
		fmt.Printf("wormhole %s\n", config.Version())
		return
	}

	if *serverMode {
		config, err := config.NewServerConfig()
		if err != nil {
			log.Fatalf("config error: %s", err.Error())
		}

		// Expose the registered metrics via HTTP.
		go func() {
			http.Handle("/metrics", promhttp.Handler())
			config.Logger.Fatal(http.ListenAndServe(":"+config.MetricsAPIPort, nil))
		}()

		wormhole.StartRemote(config)
	} else {
		config, err := config.NewClientConfig()
		if err != nil {
			log.Fatalf("config error: %s", err.Error())
		}
		wormhole.StartLocal(config)
	}
}
