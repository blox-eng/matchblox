# Changelog

All notable changes to matchblox are in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Fixed

- A turn that ends on a question waits for you; it no longer shows as done
  because the progress line came after the question. Control keys in an
  agent's reply never reach the console.

### Added

- The docs at docs.matchblox.com: get started, the console, every key, the
  phone, agents, remote hosts, config, safety, limits, and tmux, the engine
  under matchblox. A test holds every key and command in the README and the
  docs to the binary.
- Context use for Codex and OpenCode, read from each agent's own files: the
  rollout Codex holds open, and OpenCode's session storage counted the way
  OpenCode counts it. The window comes from the model the agent reports;
  `[sessions.windows]` overrides it. A session is "fresh" before its first
  turn and "not measured" when its files cannot be read, never a guess.
  Codex context use needs Linux (its session is found by the files the
  process holds open).
- An ACCOUNT column in Sessions on wide screens: the provider's mark (`✻`
  Anthropic, `❋` OpenAI, `▣` OpenCode), then the email and plan or "API
  key". Each Claude Code process reads its own config directory
  (`CLAUDE_CONFIG_DIR`), so two logins show two labels. Only those fields are
  read from the auth files; no token reaches the snapshot or the screen.
- Every agent in tmux: Codex, OpenCode, Gemini CLI, aider, cursor-agent or
  any command named in `agents` is a session, found by its process name,
  argv[0] or the script after node or python. Without hooks it is busy while
  its pane moves and waits when it is idle and its last lines ask
  ("from the pane:" on its queue row); context use shows "not measured". The
  first run writes the agents it finds to `agents`.
- An agent's open pull request shows as `#N` on its queue and Sessions rows,
  and with its title and link in the detail.
- Sessions shows the queue's word (`asks`, `waits`, `done`) instead of
  `idle`, and `paused` for a turn that stopped with its progress bar under
  100%.

- Remote mode: the first run asks where your agents run. `matchblox connect
  <host>` connects a host once (it installs the verified matchblox there and
  adds a key that can start only the console's stream), and `matchblox <host>` opens its
  console. Hosts sit above the console as a layer: `esc` goes back, `+ add a
  host` adds one. A lost host stays on the screen as stale and connects again.
- The queue, tab 1: each agent that asks for permission, waits for input or
  finished its turn, oldest first, with its last line. Claude Code hooks
  call `matchblox hook <event>`; without hooks the queue is estimated.
- Each session is a match: `✦` burning while it works, `╿` at rest while it
  waits (it breathes when it asks), `│` spent at the compact limit. It
  strikes when work starts. `--no-motion` keeps it still.
- Enter goes to the pane (it attaches outside tmux), and `a` answers in one
  line after a typed `y`. A permission prompt is answered in its pane.
- The Panes tab lists every tmux pane.
- `DESIGN.md`: colour, the mark, the match, motion, layout, copy, the site.

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
