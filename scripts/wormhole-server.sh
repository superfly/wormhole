#!/usr/bin/env bash

export PASSPHRASE=qwertyuiopasdfghjklzxcvbnm1234567890
export PRIVATE_KEY=81419e6adfb4c0a6bbcbd6fb15213bd86ec8b0557d6a876a2e94d3c25e9bb472
export PUBLIC_KEY=18ac846a4ce2e530ea814c50d2a59de6051af3c091880f49da28bd6639efed27
export FLY_TOKEN=fla
export REDIS_URL=redis://localhost:6379

WORMHOLE_BIN=$GOPATH/src/github.com/superfly/wormhole/cmd/wormhole
SITE_ID=13
BACKEND_ID=7

# Set data in Redis
redis-cli HSET backend_tokens $FLY_TOKEN $BACKEND_ID
redis-cli HGET backend_tokens $FLY_TOKEN

$WORMHOLE_BIN/wormhole -server

