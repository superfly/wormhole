#!/bin/bash

set -e

image_name="861344665010.dkr.ecr.us-east-1.amazonaws.com/wormhole-remote:${BUILDKITE_COMMIT}"

curl -O https://storage.googleapis.com/kubernetes-release/release/v1.4.0/bin/linux/amd64/kubectl
chmod +x kubectl

./kubectl set image deployments/wormhole wormhole-remote=$image_name
