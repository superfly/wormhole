#!/bin/bash

set -e

semver=${BUILDKITE_TAG:1}
IFS='.'; version_parts=($semver); unset IFS
major=${version_parts[0]}
minor=${version_parts[1]}
patch=${version_parts[2]}

image_name="flyio/wormhole:${major}.${minor}.${patch}"

curl -O https://storage.googleapis.com/kubernetes-release/release/v1.4.0/bin/linux/amd64/kubectl
chmod +x kubectl

./kubectl set image deployments/wormhole wormhole=$image_name
