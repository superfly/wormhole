#!/usr/bin/env bash

NUM_CLIENTS=${1:-1}
PORT=8080

dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# wormhole client defaults
export FLY_PROTO=ssh
export FLY_LOG_LEVEL=debug
export FLY_REMOTE_ENDPOINT=localhost:10000
export FLY_TLS_CERT_FILE=$dir/cert.pem


$dir/register-clients.sh $NUM_CLIENTS

LOCAL_SERVER_CMD="go run $GOPATH/src/github.com/valyala/fasthttp/examples/fileserver/fileserver.go -addr localhost:$PORT -dir ."

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
  # this is a hack bc I cannot figure a better way to kill all the decendants
  # (actual wormhole process is a child of $chpid)
  # let's kill all the wormole clients
  #`ps aux | grep wormhole | grep -v '-server' | awk '{ print $2}' | xargs kill`
}

trap _term SIGINT SIGTERM

for i in `seq 1 $NUM_CLIENTS`; do
  echo -n "Starting wormhole client ID $i... "
  token="token-for-$i"
  register_client $i $token > /dev/null
  #($dir/wormhole-local.sh $token) &
  spawn_wormhole $token
done

$LOCAL_SERVER_CMD &
CHILD_PIDS+=("$!")

# keep blocking until spawned processes exit
wait
