#!/bin/bash

set -e

# download glide if it's not available
if [ ! -x "$(command -v glide)" ]; then
  go get -v github.com/Masterminds/glide
fi

# we don't want to run tests on all vendorized dependencies,
# so need to ignore /vendor dir
go test -v -race $(glide novendor)
