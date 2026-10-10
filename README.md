<picture>
  <source media="(prefers-color-scheme: dark)" srcset="www/assets/matchblox-mark-dark.svg">
  <img alt="matchblox" src="www/assets/matchblox-mark-light.svg" width="84">
</picture>

# matchblox

**A tool for modern software builders.**

[![CI](https://github.com/blox-eng/matchblox/actions/workflows/ci.yml/badge.svg)](https://github.com/blox-eng/matchblox/actions/workflows/ci.yml)
[![Go 1.26](https://img.shields.io/badge/go-1.26-00ADD8.svg)](https://go.dev/dl/)
[![License: MIT](https://img.shields.io/badge/license-MIT-black.svg)](LICENSE)

<img src="www/demo/replay.svg" width="100%" alt="The matchblox console on a demo machine: the queue of agents that wait, each session with its match, machine health, and a guarded kill of a detached busy loop">

One terminal console for every AI coding agent you run in tmux. It shows
which agent waits for you, what loads the machine, the state of every git
checkout, and the one safe action that resolves each thing. It works on the
host, over SSH, and from a phone.

Works with Claude Code, Codex, OpenCode, and any agent in tmux.

**Docs: [docs.matchblox.com](https://docs.matchblox.com)**

<!-- until-release -->
> **Status: pre-release.** Everything below works today. There is no release
> binary yet: the install script builds from source
> ([limits](https://docs.matchblox.com/limits/)).
<!-- /until-release -->

## Quick start

1. Install:

   ```sh
   curl -fsSL https://matchblox.sh | sh
   ```

   Read the script first at [matchblox.sh](https://matchblox.sh/install.sh).
   <!-- until-release -->
   Until the first release it builds from source and needs Go, the same as
   `go install github.com/blox-eng/matchblox/cmd/matchblox@latest`.
   <!-- /until-release -->
   <!-- after-release
   You get the release binary for your machine, its checksum checked and,
   with `gh` logged in, proof that CI built it from this repository.
   after-release -->

2. Run `matchblox` outside tmux (the script starts it for you). Answer
   "Where do your agents run?", then open the setup steps in the queue:
   tmux, the queue hooks, the way back.

3. In another terminal, start your agents in tmux, for example
   `tmux new -s app claude`. When an
   agent waits for you, it comes to the top of the queue. `Enter` jumps to
   it, and `prefix m` comes back.

The full walk is [Get started](https://docs.matchblox.com/start/).

## What it shows

- **The queue**: each agent that asks, waits, paused or finished its turn,
  oldest first, with its last line and its open pull request.
- **A match for each session**: `✦` burning while it works, `╿` at rest while
  it waits, `│` spent when its context is full.
- **Sessions**: context use (Claude Code, Codex, OpenCode), the account
  (`✻` Anthropic, `❋` OpenAI, `▣` OpenCode), token burn, CPU, and when to
  compact or clear.
- **The stoker**: press `n` before you sleep. Each session that fills its
  context compacts itself and leaves one line on how to continue.
- **Machine health**, **detached busy loops**, **git** worktrees that are
  safe to remove, **recommendations** with their exact command, and 24 hours
  of **history**.

More in [The console](https://docs.matchblox.com/console/).

## Keys

| Key | What it does |
|---|---|
| `↑` or `k`, `↓` or `j` | Selects a row. |
| `Enter` | The safe action: go to the agent's pane. |
| `a` | Answers the selected agent in one line. |
| `x` | The destructive action. It runs only after a typed `y`. |
| `1-7` | Opens a tab. |
| `/` | Searches the open tab. |
| `f`, `n` | The stoker on or off; a night that ends in the morning. |
| `q` | Quits. The service keeps running. |

Every key: [Keys](https://docs.matchblox.com/keys/).

## From a phone

Set the startup snippet of your SSH app (Termius, for example) to
`matchblox`, or run `ssh -t ws-1 matchblox`. The console is the first
screen. Tap a row twice to go to its pane; tap `◂ matchblox` in the tmux
status line (with `set -g mouse on`) to come back, or press `prefix m`. See
[The phone](https://docs.matchblox.com/phone/).

## Safety

Read-only by default. Every action shows its exact command before it runs.
A destructive action needs `x` and then a typed `y`, and checks its facts
again just before it runs. See [Safety](https://docs.matchblox.com/safety/)
and [SECURITY.md](SECURITY.md).

## Config

matchblox works without a config. The first run writes
`~/.config/matchblox/config.toml` with goals for this machine and the agents
it finds. Every key is in [config.example.toml](config.example.toml) and on
[Config](https://docs.matchblox.com/config/).

## Built on tmux

matchblox stands on [tmux](https://github.com/tmux/tmux), and takes its
model from it: a server that keeps running, and clients that attach from
any terminal. See [tmux, the engine](https://docs.matchblox.com/tmux/).

## Help

Questions: [Discord](https://discord.gg/tYxBUpGfX3). Bugs and ideas:
[GitHub issues](https://github.com/blox-eng/matchblox/issues).

## Design

[DESIGN.md](DESIGN.md) is how matchblox looks, moves and speaks: the match,
colour, motion, layout and copy. [design/](design/) holds the specs and plans.
[CONTRIBUTING.md](CONTRIBUTING.md) · [SECURITY.md](SECURITY.md) ·
[CHANGELOG.md](CHANGELOG.md)

*Light the match. Keep it burning.*

[matchblox.com](https://matchblox.com) · MIT · From [Blox Engineering](https://bloxng.com)
