#!/usr/bin/env bash

export LOG_LEVEL=debug
export FLY_TOKEN=fla
export REMOTE_ENDPOINT=localhost:10000
export PORT=8080

WORMHOLE_BIN=$GOPATH/src/github.com/superfly/wormhole/cmd/wormhole/wormhole
LOCAL_SERVER_CMD="go run /Users/mat/workspace/golang/src/github.com/valyala/fasthttp/examples/fileserver/fileserver.go -addr localhost:$PORT -dir .y"

redis-cli HGET backend_tokens $FLY_TOKEN

$WORMHOLE_BIN $LOCAL_SERVER_CMD
