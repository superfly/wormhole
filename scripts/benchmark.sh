#!/usr/bin/env bash

NUM_CLIENTS=${1:-1}

PORTS=()
CHILD_PIDS=()

_term() {
  echo "Caught kill signal!"
  for chpid in ${CHILD_PIDS[@]}; do
    echo "Killing PID: $chpid"
    kill $chpid
  done
}

trap _term SIGINT SIGTERM

launch_wrk() {
  port=$1
  cmd="wrk -t 4 -c 10 -d 5s --latency http://localhost:$port"
  echo "Launching: \"$cmd\"..."
  output=`$cmd`
  CHILD_PIDS=("$!")
  echo "$output"
}

for i in `seq 1 $NUM_CLIENTS`; do
  port=`redis-cli SMEMBERS backend:$i:endpoints | head -n 1 | tr ':' '\n' | tail -n 1`
  if [ -n "$port" ]; then
    echo "Client ID=$i is on port: $port"
    PORTS+=("$port")
  else
    # assume the client is not registered if endpoints SET doesn't have HOST:PORT member
    echo "No Client ID=$i registered"
  fi
done

for port in ${PORTS[@]}; do
  launch_wrk $port &
done

wait
