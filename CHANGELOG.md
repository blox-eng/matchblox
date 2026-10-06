# Changelog

All notable changes to matchblox are in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- `curl -fsSL https://matchblox.sh | sh` installs matchblox and starts it.
  It verifies the checksum (and the build provenance, with `gh`) of a
  release binary; until v0.1.0 it builds from source with Go.

- The console from builder-tui: sessions by tmux pane with context use and
  burn, machine load and health, detached busy loops, git checkouts and merged
  worktrees, ranked recommendations with guarded actions, and 24 h history.
- `matchblox serve`: one service on the host owns the sampling, the state
  and the actions. `matchblox` connects to it on a local socket, or starts it
  in the background; any number of consoles share one sample loop.
- `matchblox status` reads from the running service.
- `matchblox serve --stdio`, for consoles that reach the host over SSH.
- The start screen: the matchbox builds itself and the match strikes, in
  less than 600 ms. Any key skips it; `--no-motion` or `NO_MOTION=1` shows
  the last frame.
- The header names the host: `▰ matchblox · <host>`.

### Fixed

- `go install github.com/blox-eng/matchblox/cmd/matchblox@latest` failed:
  test fixture names with ":" broke the module zip.
