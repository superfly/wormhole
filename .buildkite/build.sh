#!/bin/bash

set -e

echo "+++ Compiling :go: binaries"

if [ -z "$BUILDKITE_TAG" ]; then
  version=${BUILDKITE_COMMIT:0:7}
  echo "Not building a tag, using commit SHA $version as version"
else
  version=${BUILDKITE_TAG}
fi

docker run --rm \
  -v $(pwd):/go/src/github.com/superfly/wormhole:rw \
  --entrypoint /go/src/github.com/superfly/wormhole/.buildkite/compile.sh \
  --workdir /go/src/github.com/superfly/wormhole \
  -e VERSION=${version} \
  golang

buildkite-agent artifact upload "pkg/wormhole*"
