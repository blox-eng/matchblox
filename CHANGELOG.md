# Changelog

All notable changes to matchblox are in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.0] - 2026-10-10

The first release: one console for every coding agent you run in tmux, on
the host, over SSH and from a phone.

### Added

- **The queue**: every agent that asks for permission, waits for input or
  finished its turn, oldest first, with its last line. `Enter` goes to its
  pane, and `a` answers in one line after a typed `y`. Claude Code reaches
  the queue through its hooks; Codex, OpenCode, Gemini CLI, aider,
  cursor-agent and any command named in `agents` reach it from their pane.
- **Each session is a match**: `✦` burning while it works, `╿` at rest
  while it waits, `│` spent at the compact limit.
- **Sessions**: how full each context is (Claude Code, OpenCode, and Codex
  on Linux), the queue's word (`asks`, `waits`, `done`, `paused`), the
  account each one runs under, and its open pull request as `#N`.
- **The machine**: load, memory and health, detached busy loops, git
  checkouts and merged worktrees, recommendations with one guarded action
  each, and 24 h of history.
- **The stoker**: press `n` before you sleep, or `f` for a lunch. Each
  Claude Code session that fills its context compacts itself and leaves one
  RESUME line. In the morning, STOKED shows each one, its context before and
  after. A session that waits for you, or holds a line you started to type,
  is left as it is.
- **Your limits**: one line for each account under the metrics, with the
  matches left in this 5-hour window, the sparks left this week, and if the
  week lasts (`out Fri ~20:00`). When it will not, the queue names the
  session that burns the most, one key from a compact. Claude Code's come
  from your status line, Codex reports its own.
- **Any terminal, even a phone**: a tap selects, a second tap acts. The way
  back, `prefix m` or a tap on `◂ matchblox`, returns to the console from
  any pane; `m` in its preview turns the tmux mouse on for the tap.
- **Remote hosts**: `matchblox connect <host>` connects a host once, with a
  key that can start only the console's stream, and `matchblox <host>`
  opens its console.
- **Setup in the queue**: tmux, the queue hooks, your limits, the way back
  and a guide session, each shown as the exact change before a typed `y`.
- **Install in one line**: `curl -fsSL https://matchblox.sh | sh` installs
  the release binary for Linux or macOS, amd64 or arm64, its checksum and
  build provenance checked, and starts it. `go install` builds the same
  version, and `matchblox version` names it.
- **Docs** at [docs.matchblox.com](https://docs.matchblox.com).

[Unreleased]: https://github.com/blox-eng/matchblox/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/blox-eng/matchblox/releases/tag/v0.1.0
