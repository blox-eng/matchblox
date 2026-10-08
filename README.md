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

One terminal console for every AI coding agent you run. It shows which agent
waits for you, what loads the machine, the state of every git checkout, and the
one safe action that resolves each thing.

> **Status: pre-release.** The service, the console, the queue of agents that
> wait for you, the control pane for a phone and setup run today on the host.
> Remote mode and night mode are in progress. The plan is in
> [design/0004-v0.1-plan.md](design/0004-v0.1-plan.md).

## What it shows

- **The queue**: each agent that asks for permission, waits for input or
  finished its turn, oldest first, with its last line. `Enter` goes to its
  pane; `a` answers in one line.
- **A match for each session**: `✦` burning while it works, `╿` at rest while
  it waits, `│` spent when its context is full.
- **Sessions** by tmux pane: busy or idle, context use against the model's
  window, 30-minute token burn, CPU of the whole process tree, and when to
  compact or clear.
- **Machine health**: CPU per core, load, pressure, memory and swap,
  temperature, CPU power limits, GPUs, and the CPU share of the container groups
  you name.
- **Detached busy loops**: shells that keep burning CPU after their pane is
  gone, named to the pane that started them.
- **Git**: dirty paths, the default branch behind its remote, worktrees that
  are merged and safe to remove, and what agents spend on git.
- **Recommendations**, ranked, each with its evidence and its exact command.
- **History**: 24 hours of CPU, load, pressure, temperature, memory and swap.

## Safety

- Read-only by default.
- Every action shows its exact command before it runs.
- A destructive action needs `x` and then a typed `y`.
- Each step runs on the host, and checks its facts again just before it runs:
  the same process, a worktree still clean and unused, a session still idle.
- Two consoles that confirm the same action run it once.
- A remote host's stream uses its own key, which can start only the stream
  there, with a strict host-key check and nothing forwarded.

## Install

```bash
curl -fsSL https://matchblox.sh | sh
```

The script verifies the release binary and starts matchblox; read it first at
[matchblox.sh](https://matchblox.sh/install.sh). Until v0.1.0 it builds from
source, the same as:

```bash
go install github.com/blox-eng/matchblox/cmd/matchblox@latest
```

`matchblox` connects to the service on the host, or starts it in the
background. The service samples the machine once for every console that
watches, and keeps history and the state file when no console is open.
`matchblox serve` runs it in the foreground.

### Agents on another machine

The first run asks where your agents run. Pick "another machine", then a
host from `~/.ssh/config`, or type one. matchblox connects it once:

```bash
matchblox connect ws-1
```

This uses your own ssh login one time. It installs the verified matchblox on
`ws-1`, and adds a key that can start only the console's stream there (no
shell, no forwarding). After that, `matchblox ws-1` opens the console of
`ws-1`, and `esc` goes back to the list of hosts. A jump to a pane uses your
own ssh login. The stream can answer your agents, as the console does: keep
`~/.ssh/matchblox_ed25519` as private as any ssh key.

`matchblox status --text` prints the service's state as a short summary, for
scripts and agents. Thresholds live in `~/.config/matchblox/config.toml`; see
[config.example.toml](config.example.toml).

## Design

[DESIGN.md](DESIGN.md) is how matchblox looks, moves and speaks: the match,
colour, motion, layout and copy. [design/](design/) holds the specs and plans.
[CONTRIBUTING.md](CONTRIBUTING.md) · [SECURITY.md](SECURITY.md) ·
[CHANGELOG.md](CHANGELOG.md)

*Light the match. Keep it burning.*

[matchblox.com](https://matchblox.com) · MIT · From [Blox Engineering](https://bloxng.com)
