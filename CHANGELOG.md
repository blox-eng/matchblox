# Changelog

All notable changes to matchblox are in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- The console from builder-tui: sessions by tmux pane with context use and
  burn, machine load and health, detached busy loops, git checkouts and merged
  worktrees, ranked recommendations with guarded actions, and 24 h history.
- `matchblox serve`: one service on the host owns the sampling, the state
  and the actions. `matchblox` connects to it on a local socket, or starts it
  in the background; any number of consoles share one sample loop.
- `matchblox status` reads from the running service.
- `matchblox serve --stdio`, for consoles that reach the host over SSH.
