#!/bin/bash

set -e

if [ -n "$BUILDKITE_TAG" ]; then
  semver=${BUILDKITE_TAG:1}
  IFS='.'; version_parts=($semver); unset IFS

  # make sure we don't set random tags as stable releases
  if [ "${#version_parts[@]}" -eq "3" ]; then
    MAJOR=${version_parts[0]}
    MINOR=${version_parts[1]}
    PATCH=${version_parts[2]}
    CHANNEL=stable
  else
    # tags should be of the form 'vX.Y.Z'
    # assuming this is some sort of beta tag
    MAJOR=0
    MINOR=0
    PATCH=0-beta.$BUILDKITE_TAG
    CHANNEL=beta
  fi
elif [ -z "$BUILDKITE_TAG" ] && [ "$BUILDKITE_BRANCH" = "master" ]; then
  MAJOR=0
  MINOR=0
  PATCH=0-beta.${BUILDKITE_COMMIT:0:7}
  CHANNEL=beta
else
  echo "Not building a tag, nothing to do."
  exit 0
fi

VERSION="${MAJOR}.${MINOR}.${PATCH}"

echo "+++ Fetching :go: binaries"

buildkite-agent artifact download "pkg/wormhole*" pkg/
num_binaries=`ls pkg/wormhole* | wc -l`

# right now we support 32- and 64-bit builds for Windows, macOS, Linux and FreeBSD
# plus ARM on Linux ;)
if [ "$num_builds" -lt "9" ]; then
  echo "Missing some wormhole binaries. Cannot make a release."
  exit 1
fi

if [ "$CHANNEL" = "stable" ]; then
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
fi

echo "+++ Pushing binaries to S3"


buildkite-agent artifact upload "pkg/wormhole*" s3://flyio-wormhole-builds/$VERSION

# also set the version as the latest
# TODO: there must be a better way to copy/symlink objects in S3 instead of uploading again
buildkite-agent artifact upload "pkg/wormhole*" s3://flyio-wormhole-builds/$CHANNEL

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

# clean up
rm -f ./app

if [ "$CHANNEL" = "stable" ]; then
  tag_versions=("${major}" "${major}.${minor}" "${major}.${minor}.${patch}" "$CHANNEL")
elif [ "$CHANNEL" = "beta" ]; then
  tag_versions=("$CHANNEL")
fi

# this shouldn't happen, at this point we're either in "stable" or "beta" release
if [ "${#tag_versions[@]}" -lt "1" ]; then
  echo "expected to have some tags for the Docker image, got: '${tag_versions[@]}' instead"
  exit 1
fi

for i in "${tag_versions[@]}"; do
  echo "Tagging and pushing ${base_image_name}:${i}"
  docker tag $base_image_name "${base_image_name}:${i}"
  docker push "${base_image_name}:${i}"
done
