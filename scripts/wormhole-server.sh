#!/usr/bin/env bash

echo `pwd`
echo $0
dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
export SSH_PRIVATE_KEY="$dir/id_rsa"
if [ ! -f $SSH_PRIVATE_KEY ]; then
  # generate fresh rsa key
  ssh-keygen -f $SSH_PRIVATE_KEY -N '' -t rsa
fi

export LOG_LEVEL=debug
export FLY_TOKEN=fla
export REDIS_URL=redis://localhost:6379
export CLUSTER_URL=localhost
export LOCALHOST=localhost

PROTO=ssh
WORMHOLE_BIN=$GOPATH/src/github.com/superfly/wormhole/cmd/wormhole/wormhole
SITE_ID=13
BACKEND_ID=7

# Set data in Redis
redis-cli HSET backend_tokens $FLY_TOKEN $BACKEND_ID
redis-cli HGET backend_tokens $FLY_TOKEN

$WORMHOLE_BIN -server -proto $PROTO
