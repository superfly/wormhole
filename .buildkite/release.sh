#!/bin/bash

set -e

if [ -z "$BUILDKITE_TAG" ]; then
  echo "Not building a tag, nothing to do."
  exit 0
fi

echo "+++ Fetching :go: binaries"

buildkite-agent artifact download "pkg/wormhole*" pkg/
num_binaries=`ls pkg/wormhole* | wc -l`

# right now we support 32- and 64-bit builds for Windows, macOS, Linux and FreeBSD
if [ "$num_builds" -lt "8" ]; then
  echo "Missing some wormhole binaries. Cannot make a release."
  exit 1
fi

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

echo "+++ Pushing binaries to S3"

buildkite-agent artifact upload "pkg/wormhole*" s3://flyio-wormhole-builds/$BUILDKITE_TAG

# also set the version as the latest
# TODO: there must be a better way to copy/symlink objects in S3 instead of uploading again
buildkite-agent artifact upload "pkg/wormhole*" s3://flyio-wormhole-builds/latest

echo "+++ Building and pushing to Docker Hub"

base_image_name="flyio/wormhole"

# ensure we have the binary before building an image
wormhole_linux_bin=pkg/wormhole_linux_amd64
if [ ! -f "$wormhole_linux_bin" ]; then
  echo "missing wormhole binary in $wormhole_linux_bin"
  exit 1
fi

yes | cp -f $wormhole_linux_bin app
docker build -t $base_image_name .

semver=${BUILDKITE_TAG:1}
IFS='.'; version_parts=($semver); unset IFS
major=${version_parts[0]}
minor=${version_parts[1]}
patch=${version_parts[2]}

# clean up
rm -f ./app

declare -a tag_versions=("${major}" "${major}.${minor}" "${major}.${minor}.${patch}")
for i in "${tag_versions[@]}"; do
  echo "Tagging and pushing ${base_image_name}:${i}"
  docker tag $base_image_name "${base_image_name}:${i}"
  docker push "${base_image_name}:${i}"
done

# TODO: figure a good way to determining if a build is stable or not
# then tag it and push it
stable=true
if [ $stable ]; then
  docker_tag="${base_image_name}:stable"
  echo "Tagging and pushing $docker_tag"
  docker tag $base_image_name $docker_tag
  docker push $docker_tag
fi
