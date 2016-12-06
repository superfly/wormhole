#!/bin/bash

if [ -z "$BUILDKITE_TAG" ]; then
  echo "Not building a tag, nothing to do."
  exit 0
fi

docker run --rm \
  -v $(pwd):/go/src/github.com/superfly/wormhole:rw \
  --entrypoint /go/src/github.com/superfly/wormhole/.buildkite/compile.sh \
  --workdir /go/src/github.com/superfly/wormhole \
  golang

GHR='/usr/local/bin/github-release'

if [ ! -f $GHR ]; then
  curl -Lk https://github.com/buildkite/github-release/releases/download/v1.0/github-release-linux-amd64 > $GHR
  chmod +x $GHR
fi

$GHR $BUILDKITE_TAG pkg/* --commit $BUILDKITE_COMMIT \
                          --tag $BUILDKITE_TAG \
                          --github-repository "superfly/wormhole" \
                          --github-access-token $GITHUB_ACCESS_TOKEN
