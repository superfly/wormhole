#!/bin/bash

set -e

# download glide if it's not available
if [ ! -x "$(command -v glide)" ]; then
  go get -v github.com/Masterminds/glide
fi

glide install

tags=${GOTEST_TAGS:-}
# we don't want to run tests on all vendorized dependencies,
# so need to ignore /vendor dir
echo $tags

go test -v -race $tags $(glide novendor)
