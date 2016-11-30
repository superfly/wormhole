#!/bin/bash

set -e

# client

client_image_name="861344665010.dkr.ecr.us-east-1.amazonaws.com/wormhole-local:${BUILDKITE_COMMIT}"

docker run --rm \
  -v $(pwd):/go/src/github.com/superfly/wormhole \
  -e CGO_ENABLED=0 -e GOOS=linux \
  golang \
  go build -a -o /go/src/github.com/superfly/wormhole/app github.com/superfly/wormhole/cmd/local

docker build -t $client_image_name .
docker push $client_image_name

# server

server_image_name="861344665010.dkr.ecr.us-east-1.amazonaws.com/wormhole-remote:${BUILDKITE_COMMIT}"

docker run --rm \
  -v $(pwd):/go/src/github.com/superfly/wormhole \
  -e CGO_ENABLED=0 -e GOOS=linux \
  golang \
  go build -a -o /go/src/github.com/superfly/wormhole/app github.com/superfly/wormhole/cmd/remote

docker build -t $server_image_name .
docker push $server_image_name
