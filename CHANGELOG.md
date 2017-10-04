# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/)
and this project adheres to [Semantic Versioning](http://semver.org/).

## [Unreleased]
### Added
* Experimental HTTP2 support
* Experimental connection pool

### Changed
* The top-level listener at the wh-server level now only listens on TCP - allowing each handler control over TLS/SSH

### Removed

### Fixed
* Complies with bug introduces from https://github.com/golang/go/issues/19767

## [0.5.35] - 2017-06-15
### Added
- Makefile
- Travis CI integration

### Changed
- First open-source release of Wormhole

### Fixed
- go {fmt,vet,lint} the code base
