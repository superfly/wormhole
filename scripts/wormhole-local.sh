#!/usr/bin/env bash

# Wormhole client wrapper script, that sets all the defaults to make it really easy
# to connect wormhole client to a local wormhole server.
#
# This script by default launches 1 wormhole client, but it has an option to launch
# multiple clients at once.

# ## Usage:
#
#     $ wormhole-local.sh <NUM_CLIENTS>

NUM_CLIENTS=${1:-1}

# HTTP port of local server
PORT=8080
# HTTPS port of local server
TLS_PORT=8888

dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# wormhole client defaults
export FLY_PROTO=ssh
export FLY_LOG_LEVEL=error
export FLY_REMOTE_ENDPOINT=localhost:10000
export FLY_TLS_CERT_FILE=$dir/cert.pem


LOCAL_SERVER_CMD=\
"go run $GOPATH/src/github.com/valyala/fasthttp/examples/fileserver/fileserver.go"\
" -addrTLS localhost:$TLS_PORT"\
" -certFile=$dir/cert.pem"\
" -keyFile=$dir/key.pem"\
" -addr localhost:$PORT"\
" -dir $dir"

echo "LOCAL CMD: $LOCAL_SERVER_CMD"

CHILD_PIDS=()

register_client() {
  client_id=$1
  token=$2

  redis-cli HSET backend_tokens $token $client_id
}

spawn_wormhole() {
  token=$1

  FLY_TOKEN=$token FLY_PORT=$PORT $GOPATH/src/github.com/superfly/wormhole/cmd/wormhole/wormhole &
  CHILD_PIDS+=("$!")
  echo "DONE (PID: $!)"
}

_term() {
  echo "Caught kill signal!"
  for chpid in ${CHILD_PIDS[@]}; do
    echo "Killing PID: $chpid"
    kill $chpid
  done
}

trap _term SIGINT SIGTERM

for i in `seq 1 $NUM_CLIENTS`; do
  echo -n "Starting wormhole client ID $i... "
  token="token-for-$i"
  register_client $i $token > /dev/null
  spawn_wormhole $token
done

$LOCAL_SERVER_CMD &
CHILD_PIDS+=("$!")

# keep blocking until spawned processes exit
wait
