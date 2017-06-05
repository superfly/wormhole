#!/bin/bash

set -e

docker run --rm \
  --env GOTEST_TAGS="-tags=integration" \
  -v $(pwd):/go/src/github.com/superfly/wormhole:rw \
  --entrypoint /go/src/github.com/superfly/wormhole/.buildkite/test_entrypoint.sh \
  --workdir /go/src/github.com/superfly/wormhole \
  golang
