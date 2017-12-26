# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/)
and this project adheres to [Semantic Versioning](http://semver.org/).

## [Unreleased]
### Added

* Can now connect to local endpoints via TLS

### Fixed
* Race condition with session access in remote/http2 (#26)
* Errant `FLY_ENDPOINT` references in usage output


## [0.5.36] - 2017-10-09
### Added
* Experimental HTTP2 support
* Experimental connection pool
* Travis CI integration with fly.io/slack

### Changed
* The top-level listener at the wh-server level now only listens on TCP - allowing each handler control over TLS/SSH
* TCPHandler now accepts both TCP and TLS depending on configuration

### Removed

### Fixed
* Complies with breaking ssh behavior changes introduced in https://github.com/golang/go/issues/19767
* Fully migrated testing/building/releasing to Travis CI
* Bug with unhandled error in TLS wrappers
* Bug in build script that caused binaries to be uploaded for each Go version (#22)
* Wormhole start up in supervisor mode on Windows (#24)


## [0.5.35] - 2017-06-15
### Added
- Makefile
- Travis CI integration

### Changed
- First open-source release of Wormhole

### Fixed
- go {fmt,vet,lint} the code base
