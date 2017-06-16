# Root directory of the project (absolute path).
ROOTDIR=$(dir $(abspath $(lastword $(MAKEFILE_LIST))))

# Base path used to install.
DESTDIR=/usr/local

# Used to populate version variable in main package.
VERSION=$(shell git describe --match 'v[0-9]*' --dirty='.m' --always)

PROJECT_ROOT=github.com/superfly/wormhole

# Race detector is only supported on amd64.
RACE := $(shell test $$(go env GOARCH) != "amd64" || (echo "-race"))

# Project packages.
PACKAGES=$(shell go list ./... | grep -v /vendor/)
INTEGRATION_PACKAGE=${PROJECT_ROOT}/integration

# Project binaries.
COMMANDS=wormhole
BINARIES=$(addprefix bin/,$(COMMANDS))

GO_LDFLAGS=-ldflags "-X `go list ./config`.version=$(VERSION)"

.PHONY: clean all fmt vet lint build binaries test integration setup generate checkprotos coverage ci check help install uninstall
.DEFAULT: default

all: check binaries test integration ## run fmt, vet, lint, build the binaries and run the tests

check: fmt vet lint ineffassign misspell ## run fmt, vet, lint, ineffassign, misspell

ci: check binaries checkprotos coverage coverage-integration ## to be used by the CI

setup: ## install dependencies
	@echo "🎈 $@"
	@go get -u github.com/golang/lint/golint
	@go get -u github.com/gordonklaus/ineffassign
	@go get -u github.com/client9/misspell/cmd/misspell
	@go get -u github.com/Masterminds/glide
	@glide install

# Depends on binaries because vet will silently fail if it can't load compiled
# imports
vet: binaries ## run go vet
	@echo "🎈 $@"
	@test -z "$$(go vet ${PACKAGES} 2>&1 | grep -v 'constant [0-9]* not a string in call to Errorf' | egrep -v '(timestamp_test.go|duration_test.go|exit status 1)' | tee /dev/stderr)"

misspell:
	@echo "🎈 $@"
	@test -z "$$(find . -type f | grep -v vendor/ | grep -v bin/ | grep -v .git/ | xargs misspell | tee /dev/stderr)"

fmt: ## run go fmt
	@echo "🎈 $@"
	@test -z "$$(gofmt -s -l . | grep -v vendor/ | grep -v ".pb.go$$" | tee /dev/stderr)" || \
		(echo "👹 please format Go code with 'gofmt -s -w'" && false)
	@test -z "$$(find . -path ./vendor -prune -o -name '*.proto' -type f -exec grep -Hn -e "^ " {} \; | tee /dev/stderr)" || \
		(echo "👹 please indent proto files with tabs only" && false)
	@test -z "$$(find . -path ./vendor -prune -o -name '*.proto' -type f -exec grep -Hn "Meta meta = " {} \; | grep -v '(gogoproto.nullable) = false' | tee /dev/stderr)" || \
		(echo "👹 meta fields in proto files must have option (gogoproto.nullable) = false" && false)

lint: ## run go lint
	@echo "🎈 $@"
	@test -z "$$(golint ./... | grep -v vendor/ | grep -v ".pb.go:" | grep -v ".mock.go" | tee /dev/stderr)"

ineffassign: ## run ineffassign
	@echo "🎈 $@"
	@test -z "$$(ineffassign . | grep -v vendor/ | grep -v ".pb.go:" | grep -v ".mock.go" | tee /dev/stderr)"

build: ## build the go packages
	@echo "🎈 $@"
	@go build -i -v ${GO_LDFLAGS} ${GO_GCFLAGS} ${PACKAGES}

test: ## run tests, except integration tests
	@echo "🎈 $@"
	@go test -parallel 8 ${RACE} $(filter-out ${INTEGRATION_PACKAGE},${PACKAGES})

integration: ## run integration tests
	@echo "🎈 $@"
	@go test -parallel 8 ${RACE} ${INTEGRATION_PACKAGE}

FORCE:

# Build a binary from a cmd.
bin/%: cmd/% FORCE
	@test $$(go list) = "${PROJECT_ROOT}" || \
		(echo "👹 Please correctly set up your Go build environment. This project must be located at <GOPATH>/src/${PROJECT_ROOT}" && false)
	@echo "🎈 $@"
	@go build -i -o $@ ${GO_LDFLAGS}  ${GO_GCFLAGS} ./$<

binaries: $(BINARIES) ## build binaries
	@echo "🎈 $@"

clean: ## clean up binaries
	@echo "🎈 $@"
	@rm -f $(BINARIES)

install: $(BINARIES) ## install binaries
	@echo "🎈 $@"
	@mkdir -p $(DESTDIR)/bin
	@install $(BINARIES) $(DESTDIR)/bin

uninstall:
	@echo "🎈 $@"
	@rm -f $(addprefix $(DESTDIR)/bin/,$(notdir $(BINARIES)))

coverage: ## generate coverprofiles from the unit tests
	@echo "🎈 $@"
	@( for pkg in $(filter-out ${INTEGRATION_PACKAGE},${PACKAGES}); do \
		go test -i ${RACE} -test.short -coverprofile="../../../$$pkg/coverage.txt" -covermode=atomic $$pkg || exit; \
		go test ${RACE} -test.short -coverprofile="../../../$$pkg/coverage.txt" -covermode=atomic $$pkg || exit; \
	done )

coverage-integration: ## generate coverprofiles from the integration tests
	@echo "🎈 $@"
	go test ${RACE} -test.short -coverprofile="../../../${INTEGRATION_PACKAGE}/coverage.txt" -covermode=atomic ${INTEGRATION_PACKAGE}

help: ## this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST) | sort
