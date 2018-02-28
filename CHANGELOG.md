# Graphite Remote Adapter
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/)
and this project adheres to [Semantic Versioning](http://semver.org/).

## [Unreleased]

## [0.0.15] - 2018-02-28
### Added
- Support for larger retention than prom1.x staleness delta
- Support for custom prefix in the query string

## [0.0.14] - 2018-01-29

### Fixed
- Bug with empty config files
- Fixed reads

## [0.0.13] - 2018-01-22
### Added
- Support for Tags

### Changed
- Reuse TCP connection

## [0.0.12] - 2018-01-02
### Breaking changes
- Config file is now top-level instead of Graphite specific
- All the flags have been changed

### Changed
- Migrated to github.com/go-kit/kit/log
- Migrated to gopkg.in/alecthomas/kingpin.v2
- Make golint happy

### Added
- CHANGELOG file

## [0.0.11] - 2017-11-21
### Added
- Support dynamic config reload
- Graphite: paralel fetches

### Changed
- Status page enhanced

## [0.0.10] - 2017-11-16
### Changed
- graphite-url flag now contains scheme and user information
- Graphite read now handles all kinds of label matcher

## [0.0.9] - 2017-09-22
### Fixed
- Fix read-ignore flag usage

### Added
- ignore-read-error flag

## [0.0.8] - 2017-09-01
### Fixed
- Quit if we fail to read config file
- Fix concurrent R/W on template_data

### Added
- read-ignore flag
- Cache for Graphite paths

## [0.0.7] - 2017-07-19
### Fixed
- Don't fail if config-file is not provided

## [0.0.6] - 2017-07-13
### Added
- Enable read using default Graphtie path

## [0.0.5] - 2017-06-16
### Added
- Small status page
- Config file to customize writing Graphite path.

## [0.0.4] - 2017-06-06
### Fixed
- Fix VERSION

## [0.0.3] - 2017-06-05
### Added
- Draft for read support
- Add -snappy-framed flag to support prometheus < 2.x.x

## [0.0.2] - 2017-06-02
### Added
- Initial release with base project.

[Unreleased]: https://github.com/criteo/graphite-remote-adapter/compare/v0.0.15...HEAD
[0.0.15]: https://github.com/criteo/graphite-remote-adapter/compare/v0.0.14...v0.0.15
[0.0.14]: https://github.com/criteo/graphite-remote-adapter/compare/v0.0.13...v0.0.14
[0.0.13]: https://github.com/criteo/graphite-remote-adapter/compare/v0.0.12...v0.0.13
[0.0.12]: https://github.com/criteo/graphite-remote-adapter/compare/v0.0.11...v0.0.12
[0.0.11]: https://github.com/criteo/graphite-remote-adapter/compare/v0.0.10...v0.0.11
[0.0.10]: https://github.com/criteo/graphite-remote-adapter/compare/v0.0.9...v0.0.10
[0.0.9]: https://github.com/criteo/graphite-remote-adapter/compare/v0.0.8...v0.0.9
[0.0.8]: https://github.com/criteo/graphite-remote-adapter/compare/v0.0.7...v0.0.8
[0.0.7]: https://github.com/criteo/graphite-remote-adapter/compare/v0.0.6...v0.0.7
[0.0.6]: https://github.com/criteo/graphite-remote-adapter/compare/v0.0.5...v0.0.6
[0.0.5]: https://github.com/criteo/graphite-remote-adapter/compare/v0.0.4...v0.0.5
[0.0.4]: https://github.com/criteo/graphite-remote-adapter/compare/v0.0.3...v0.0.4
[0.0.3]: https://github.com/criteo/graphite-remote-adapter/compare/v0.0.2...v0.0.3
[0.0.2]: https://github.com/criteo/graphite-remote-adapter/compare/v0.0.2
