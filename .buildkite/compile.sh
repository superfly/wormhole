#!/bin/bash

set -e

MD5='md5sum'
unamestr=`uname`
if [[ "$unamestr" == 'Darwin' ]]; then
	MD5='md5'
fi

VERSION=${VERSION:-"latest"}
[ -z "$PASSPHRASE" ] && echo "Need to set PASSPHRASE" && exit 1;

echo "Compiling version: ${VERSION}"

LDFLAGS="-X 'main.version=$VERSION' -X 'main.passphrase=$PASSPHRASE' -s -w"
GCFLAGS=""

# Cleanup
rm -rf pkg && mkdir -p pkg

OSES=(linux darwin windows freebsd)
ARCHS=(amd64 386)
for os in ${OSES[@]}; do
	for arch in ${ARCHS[@]}; do
		suffix=""
		if [ "$os" == "windows" ]
		then
			suffix=".exe"
		fi
		env CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -ldflags "$LDFLAGS" -gcflags "$GCFLAGS" -o pkg/wormhole_${os}_${arch}${suffix} github.com/superfly/wormhole/cmd/wormhole
    $MD5 pkg/wormhole_${os}_${arch}${suffix}
	done
done
