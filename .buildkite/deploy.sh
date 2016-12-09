#!/bin/bash

set -e

image_name="flyio/wormhole:${BUILDKITE_TAG}"

curl -O https://storage.googleapis.com/kubernetes-release/release/v1.4.0/bin/linux/amd64/kubectl
chmod +x kubectl

./kubectl set image deployments/wormhole wormhole=$image_name
