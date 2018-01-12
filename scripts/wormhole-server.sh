#!/usr/bin/env bash

# Wormhole server wrapper script, that sets all the defaults to make it really easy
# to start a wormhole server locally.

# ## Usage:
#
#     $ wormhole-server.sh

echo `pwd`
echo $0
dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
export FLY_SSH_PRIVATE_KEY_FILE="$dir/id_rsa"
if [ ! -f $FLY_SSH_PRIVATE_KEY_FILE ]; then
  # generate fresh rsa key
  ssh-keygen -f $FLY_SSH_PRIVATE_KEY_FILE -N '' -t rsa
fi

export FLY_PROTO=ssh
export FLY_USE_SHARED_PORT_FORWARDING=1
export FLY_SHARED_TLS_FORWARDING_PORT=5442
export FLY_LOG_LEVEL=debug
export FLY_REDIS_URL=redis://127.0.0.1:6379
export FLY_CLUSTER_URL=127.0.0.1
export FLY_LOCALHOST=localhost
export FLY_NODE_ID=localhost
export FLY_TLS_CERT_FILE=$GOPATH/src/github.com/superfly/wormhole/scripts/cert.pem
export FLY_TLS_PRIVATE_KEY_FILE=$GOPATH/src/github.com/superfly/wormhole/scripts/key.pem


WORMHOLE_BIN=$GOPATH/src/github.com/superfly/wormhole/bin/wormhole

$WORMHOLE_BIN -server
