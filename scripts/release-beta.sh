#!/bin/bash
# Temporary script to compile wormhole beta locally and push to docker registry

set -e

dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

$dir/../.buildkite/compile.sh

yes | cp -f $dir/../pkg/wormhole_linux_amd64 $dir/../app

docker build -t flyio/wormhole:beta $dir/../

docker push flyio/wormhole:beta
