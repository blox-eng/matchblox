<picture>
  <source media="(prefers-color-scheme: dark)" srcset="www/assets/matchblox-mark-dark.svg">
  <img alt="matchblox" src="www/assets/matchblox-mark-light.svg" width="84">
</picture>

# matchblox

**A tool for modern software builders.**

[![CI](https://github.com/blox-eng/matchblox/actions/workflows/ci.yml/badge.svg)](https://github.com/blox-eng/matchblox/actions/workflows/ci.yml)
[![Go 1.26](https://img.shields.io/badge/go-1.26-00ADD8.svg)](https://go.dev/dl/)
[![License: MIT](https://img.shields.io/badge/license-MIT-black.svg)](LICENSE)

One terminal console for every AI coding agent you run. It shows which agent
waits for you, what loads the machine, the state of every git checkout, and the
one safe action that resolves each thing.

> **Status: pre-release.** The console runs today on the host. The service,
> the "waiting for you" queue, remote mode and the stoker are in progress. The
> plan is in [design/0002-v0.1-plan.md](design/0002-v0.1-plan.md).

## What it shows

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
- Each step checks its facts again just before it runs: the same process, a
  worktree still clean and unused, a session still idle.

## Install

From source, until the first release:

```bash
go install github.com/blox-eng/matchblox/cmd/matchblox@latest
tmux new-window -n matchblox matchblox
```

`matchblox status --text` prints the same state as a short summary, for scripts
and agents. Thresholds live in `~/.config/matchblox/config.toml`; see
[config.example.toml](config.example.toml).

## Design

[design/0001-v0.1.md](design/0001-v0.1.md) is the contract.
[CONTRIBUTING.md](CONTRIBUTING.md) · [SECURITY.md](SECURITY.md) ·
[CHANGELOG.md](CHANGELOG.md)

*Light the match. Keep it burning.*

[matchblox.com](https://matchblox.com) · MIT · From [Blox Engineering](https://bloxng.com)
