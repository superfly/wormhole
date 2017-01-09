#!/usr/bin/env bash

export LOG_LEVEL=debug
export PASSPHRASE=qwertyuiopasdfghjklzxcvbnm1234567890
export PUBLIC_KEY=18ac846a4ce2e530ea814c50d2a59de6051af3c091880f49da28bd6639efed27
export FLY_TOKEN=fla
export REMOTE_ENDPOINT=localhost:10000
export PORT=8080

WORMHOLE_BIN=$GOPATH/src/github.com/superfly/wormhole/cmd/wormhole
LOCAL_SERVER_CMD="go run /Users/mat/workspace/golang/src/github.com/valyala/fasthttp/examples/fileserver/fileserver.go -addr localhost:$PORT -dir .y"

redis-cli HGET backend_tokens $FLY_TOKEN

$WORMHOLE_BIN/wormhole $LOCAL_SERVER_CMD
