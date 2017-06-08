[![Fly.io Community Slack](https://fly.io/slack/badge.svg)](https://fly.io/slack/)
[![Build Status](https://travis-ci.org/superfly/wormhole.svg?branch=master)](https://travis-ci.org/superfly/wormhole)

# wormhole - Fly.io reverse Proxy

## What is wormhole?
Wormhole is a reverse proxy that creates a secure tunnel between two endpoints.

## Compiling
**Wormhole requires Go1.7+**

    go get github.com/Masterminds/glide
    mkdir -p $GOPATH/github.com/superfly
    cd $GOPATH/src/github.com/superfly
    git clone git@github.com:superfly/wormhole.git
    cd wormhole
    glide install
    go build github.com/superfly/wormhole/cmd/wormhole


## Running locally

    brew install redis

    # make sure redis-server is running

    # Start server
    ./scripts/wormhole-server.sh

    # Start clients (defaults to 1)
    ./scripts/wormhole-local.sh <NUM_CLIENTS>

    # The tunnel will be accessible on a randomly chosen port (look at wormhole-server logs):
    # [Feb 20 20:43:50]  INFO SSHHandler: Started session 29ff7b66abcc9871cdf1bc551f6e89728202f3e24e48675ecd9b8556a5dbd60b for Mats-MBP.local ([::1]:63169). Listening on: localhost:63170
