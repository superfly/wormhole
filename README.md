[![Fly.io Community Slack](https://fly.io/slack/badge.svg)](https://fly.io/slack/)
[![Build Status](https://travis-ci.org/superfly/wormhole.svg?branch=master)](https://travis-ci.org/superfly/wormhole)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fsuperfly%2Fwormhole.svg?type=shield)](https://app.fossa.io/projects/git%2Bgithub.com%2Fsuperfly%2Fwormhole?ref=badge_shield)

# wormhole - Fly.io reverse Proxy

<p align="center">
  <img src="wormhole.png">
</p>

## What is wormhole?
Wormhole is a reverse proxy that creates a secure tunnel between two endpoints.

## Compiling
**Wormhole requires Go1.8+**

    go get github.com/superfly/wormhole
    cd $GOPATH/src/github.com/superfly/wormhole
    make setup
    make binaries

## Running locally

    brew install redis

    # make sure redis-server is running

    # Start server
    ./scripts/wormhole-server.sh

    # Start clients (defaults to 1)
    ./scripts/wormhole-local.sh <NUM_CLIENTS>

    # The tunnel will be accessible on a randomly chosen port (look at wormhole-server logs):
    # [Feb 20 20:43:50]  INFO SSHHandler: Started session 29ff7b66abcc9871cdf1bc551f6e89728202f3e24e48675ecd9b8556a5dbd60b for Mats-MBP.local ([::1]:63169). Listening on: localhost:63170

## Feature Status

| Feature					| Status       |
| :-----:					| :----:       |
| SSH Tunnel					| Supported |
| TCP Tunnel					| Experimental - currently lacking some auth |
| TLS Tunnel              			| Experimental - currently lacking some auth |
| HTTP2 Tunnel            			| Experimental - currently lacking some auth |
| Local Endpoint over TCP			| Supported |
| Local Endpoint over TLS			| Supported |
| Single Tunnel Type per WH Server 		| Supported |
| Multiple Tunnel Types per WH Server 		| Pending [#10](https://github.com/superfly/wormhole/issues/10) |
| Healthcheck for Local Endpoint 		| Pending [#33](https://github.com/superfly/wormhole/issues/33) |
| WH Server Shared Port TLS+SNI forwarding 	| Supported |


## License
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fsuperfly%2Fwormhole.svg?type=large)](https://app.fossa.io/projects/git%2Bgithub.com%2Fsuperfly%2Fwormhole?ref=badge_large)