#!/usr/bin/env bash

set -e

if [ -n "$TRAVIS_TAG" ]; then
  semver=${TRAVIS_TAG:1}
  IFS='.'; version_parts=($semver); unset IFS

  # make sure we don't set random tags as stable releases
  if [ "${#version_parts[@]}" -eq "3" ]; then
    MAJOR=${version_parts[0]}
    MINOR=${version_parts[1]}
    PATCH=${version_parts[2]}
    CHANNEL=stable
    VERSION=$MAJOR.$MINOR.$PATCH
  else
    # tags should be of the form 'vX.Y.Z'
    # assuming this is some sort of beta tag
    MAJOR=0
    MINOR=0
    PATCH=0-beta.$TRAVIS_TAG
    CHANNEL=beta
    VERSION=$MAJOR.$MINOR.$PATCH
  fi
elif [ -z "$TRAVIS_TAG" ] && [ "$TRAVIS_BRANCH" == "master" ]; then
  MAJOR=0
  MINOR=0
  PATCH=0-beta.${TRAVIS_COMMIT:0:7}
  CHANNEL=beta
  VERSION=$MAJOR.$MINOR.$PATCH
else
  MAJOR=0
  MINOR=0
  PATCH=0-alpha.${TRAVIS_COMMIT:0:7}
  CHANNEL=alpha
  VERSION=$MAJOR.$MINOR.$PATCH
  BUILD_UPLOAD=${BUILD_UPLOAD:-false}
fi

# download dep if it's not available
if [ ! -x "$(command -v dep)" ]; then
  go get github.com/golang/dep/cmd/dep
fi

dep ensure

MD5='md5sum'
unamestr=`uname`
if [[ "$unamestr" == 'Darwin' ]]; then
	MD5='md5'
fi

VERSION=${VERSION:-"latest"}

echo "Compiling version: ${VERSION}"

LDFLAGS="-X 'github.com/superfly/wormhole/config.version=$VERSION' -s -w"
GCFLAGS=""

# Cleanup
rm -rf pkg && mkdir -p pkg

OSES=(linux darwin windows freebsd)
ARCHS=(amd64 386 arm)
for os in ${OSES[@]}; do
	for arch in ${ARCHS[@]}; do
		suffix=""
		if [ "$os" == "windows" ]; then
			suffix=".exe"
		fi

		# only compile ARM target for linux
		if [ "$arch" == "arm" ] && [ "$os" != "linux" ]; then
			continue
		fi
		env CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o pkg/wormhole_${os}_${arch}${suffix} github.com/superfly/wormhole/cmd/wormhole
		$MD5 pkg/wormhole_${os}_${arch}${suffix}
	done
done

num_binaries=`ls pkg/wormhole* | wc -l`

# right now we support 32- and 64-bit builds for Windows, macOS, Linux and FreeBSD
# plus ARM on Linux ;)
if [ $num_binaries -lt 9 ]; then
  echo "Missing some wormhole binaries. Cannot make a release."
  exit 1
fi

if [ -z "$BUILD_UPLOAD" ] || [ "$BUILD_UPLOAD" == "false" ]; then
  echo "BUILD_UPLOAD is not set. Not going to upload binaries."
  exit 0
fi

if [ "$TRAVIS_SECURE_ENV_VARS" == "false" ]; then
  echo "Untrusted source. Not going to upload binaries."
  exit 0
fi

if [ "$CHANNEL" == "stable" ]; then
  GHR='./github-release'

  if [ ! -f $GHR ]; then
    ghr_bin=github-release-linux-amd64
    if [[ "$unamestr" == 'Darwin' ]]; then
      ghr_bin=github-release-darwin-amd64
    fi
    curl -Lk https://github.com/buildkite/github-release/releases/download/v1.0/$ghr_bin > $GHR
    chmod +x $GHR
  fi

  echo "Creating GitHub release"

  $GHR $TRAVIS_TAG pkg/* --commit $TRAVIS_COMMIT \
                            --tag $TRAVIS_TAG \
                            --github-repository "superfly/wormhole" \
                            --github-access-token $GITHUB_ACCESS_TOKEN
fi

echo "Pushing binaries to S3"

curl -Lk https://dl.minio.io/client/mc/release/linux-amd64/mc > /usr/local/bin/mc

mc config host add s3 https://s3.amazonaws.com $AWS_S3_ACCESS_KEY_ID $AWS_S3_SECRET_ACCESS_KEY

echo "Pushing to s3/flyio-wormhole-builds/$VERSION/"

mc -q mirror --overwrite pkg/ s3/flyio-wormhole-builds/$VERSION
mc policy public s3/flyio-wormhole-builds/$VERSION

echo "Pushing to s3/flyio-wormhole-builds/$CHANNEL/"
# also set the version as the latest
# TODO: there must be a better way to copy/symlink objects in S3 instead of uploading again
mc -q mirror --overwrite pkg/ s3/flyio-wormhole-builds/$CHANNEL
mc policy public s3/flyio-wormhole-builds/$CHANNEL

echo "Building and pushing to Docker Hub"

base_image_name="flyio/wormhole"

# ensure we have the binary before building an image
wormhole_linux_bin=pkg/wormhole_linux_amd64
if [ ! -f "$wormhole_linux_bin" ]; then
  echo "Missing wormhole binary in $wormhole_linux_bin"
  exit 1
fi

yes | cp -f $wormhole_linux_bin app
docker build -t $base_image_name .

# clean up
rm -f ./app

if [ "$CHANNEL" == "stable" ]; then
  tag_versions=("${MAJOR}" "${MAJOR}.${MINOR}" "${MAJOR}.${MINOR}.${PATCH}" "$CHANNEL")
elif [ "$CHANNEL" == "beta" ] && [ "$TRAVIS_BRANCH" = "master" ]; then
  tag_versions=("${VERSION}" "${CHANNEL}")
else
  tag_versions=("$VERSION")
fi

# this shouldn't happen, at this point we're either in "stable" or "beta" release
if [ "${#tag_versions[@]}" -lt "1" ]; then
  echo "Expected to have some tags for the Docker image, got: '${tag_versions[@]}' instead"
  exit 1
fi

docker login --username $DOCKER_LOGIN --password $DOCKER_PASSWORD

for i in "${tag_versions[@]}"; do
  echo "Tagging and pushing ${base_image_name}:${i}"
  docker tag $base_image_name "${base_image_name}:${i}"
  docker push "${base_image_name}:${i}"
done
