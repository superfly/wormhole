#!/bin/bash

set -e

# client

client_image_name="861344665010.dkr.ecr.us-east-1.amazonaws.com/wormhole-local:${BUILDKITE_COMMIT}"

docker run --rm \
  -v "$(pwd)/cmd/local:/src" \
  -v /var/run/docker.sock:/var/run/docker.sock \
  centurylink/golang-builder \
  $client_image_name

docker push $client_image_name

# server

server_image_name="861344665010.dkr.ecr.us-east-1.amazonaws.com/wormhole-remote:${BUILDKITE_COMMIT}"

yes | ssh-keygen -f server/id_rsa -N '' -t rsa

docker run --rm \
  -v "$(pwd)/cmd/remote:/src" \
  -v /var/run/docker.sock:/var/run/docker.sock \
  centurylink/golang-builder \
  $server_image_name

docker push $server_image_name
