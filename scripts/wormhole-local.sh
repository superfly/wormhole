#!/usr/bin/env bash

export FLY_PROTO=ssh
export FLY_LOG_LEVEL=debug
export FLY_TOKEN=fla
export FLY_REMOTE_ENDPOINT=localhost:10000
export FLY_PORT=8080
export FLY_TLS_CERT_FILE=$GOPATH/src/github.com/superfly/wormhole/scripts/cert.pem


WORMHOLE_BIN=$GOPATH/src/github.com/superfly/wormhole/cmd/wormhole/wormhole
LOCAL_SERVER_CMD="go run $GOPATH/src/github.com/valyala/fasthttp/examples/fileserver/fileserver.go -addr localhost:$FLY_PORT -dir ."

redis-cli HGET backend_tokens $FLY_TOKEN

$WORMHOLE_BIN $LOCAL_SERVER_CMD
