#!/bin/bash

set -e

if [ -z "$BUILDKITE_TAG" ]; then
  echo "Not building a tag, nothing to do."
  exit 0
fi

echo "+++ Compiling :go: binaries"

docker run --rm \
  -v $(pwd):/go/src/github.com/superfly/wormhole:rw \
  --entrypoint /go/src/github.com/superfly/wormhole/.buildkite/compile.sh \
  --workdir /go/src/github.com/superfly/wormhole \
  -e VERSION=${BUILDKITE_TAG} \
  golang

GHR='./github-release'

if [ ! -f $GHR ]; then
  curl -Lk https://github.com/buildkite/github-release/releases/download/v1.0/github-release-linux-amd64 > $GHR
  chmod +x $GHR
fi

echo "+++ Creating :github: release"

$GHR $BUILDKITE_TAG pkg/* --commit $BUILDKITE_COMMIT \
                          --tag $BUILDKITE_TAG \
                          --github-repository "superfly/wormhole" \
                          --github-access-token $GITHUB_ACCESS_TOKEN

echo "+++ Building and pushing to Docker Hub"

base_image_name="flyio/wormhole-local"

semver=${BUILDKITE_TAG:1}
IFS='.'; version_parts=($semver); unset IFS
major=${version_parts[0]}
minor=${version_parts[1]}
patch=${version_parts[2]}

docker run --rm \
  -v $(pwd):/go/src/github.com/superfly/wormhole \
  -e CGO_ENABLED=0 -e GOOS=linux \
  golang \
  go build -a -o /go/src/github.com/superfly/wormhole/app github.com/superfly/wormhole/cmd/local

docker build -t $base_image_name .

declare -a tag_versions=("${major}" "${major}.${minor}" "${major}.${minor}.${patch}")
for i in "${tag_versions[@]}"; do
  echo "Tagging and pushing ${base_image_name}:${i}"
  docker tag $base_image_name "${base_image_name}:${i}"
  docker push "${base_image_name}:${i}"
done
