<picture>
  <source media="(prefers-color-scheme: dark)" srcset="www/assets/matchblox-mark-dark.svg">
  <img alt="matchblox" src="www/assets/matchblox-mark-light.svg" width="84">
</picture>

# matchblox

**Light an idea. Keep it burning.**

[![CI](https://github.com/blox-eng/matchblox/actions/workflows/ci.yml/badge.svg)](https://github.com/blox-eng/matchblox/actions/workflows/ci.yml)
[![Go 1.27](https://img.shields.io/badge/go-1.27-00ADD8.svg)](https://go.dev/dl/)
[![License: MIT](https://img.shields.io/badge/license-MIT-black.svg)](LICENSE)

<p>
<img src="www/demo/replay-desk.svg" width="74%" alt="The matchblox board on a desk, on a demo machine: four columns, NEEDS YOU, WORKING, SHIPPING and QUIET. A digit marks an answer from the agent's own options, the same digit sends it, and the card strikes into WORKING. p and a typed y merge a pull request. A tap hides the WORKING column and brings it back, and g shows one chain">
<img src="www/demo/replay-phone.svg" width="24%" alt="The same board on a phone: one column a page, with the page marks at the top. An answer marked and sent from the card's peek, a merge, a page hidden and shown again, and one chain">
</p>

<p>
<img src="www/demo/stoker.svg" width="32%" alt="The stoker takes orders, on a phone on a demo machine: every quiet session says why, n asks how to run tonight with the gos you give, at 03:12 it works its own plan with a release waiting for your y and a trigger fired, the card in the morning, a new order by day, and the ways file">
</p>

One terminal console for every AI coding agent you run in tmux. It shows
which agent needs you, what loads the machine, the state of every git
checkout, and the one safe action that resolves each thing. It works on the
host, over SSH, and from a phone.

Works with Claude Code, Codex, OpenCode, Cursor, and any agent in tmux.

**Docs: [docs.matchblox.com](https://docs.matchblox.com)**

**On your phone:** [Matchblox Pocket](https://testflight.apple.com/join/t2rtbjHz)
runs the console on iPhone and iPad — pair with one scan, get a buzz when a
session needs you, and answer from anywhere. In beta on TestFlight.


## Quick start

1. Install — with the script, Go, or Homebrew:

   ```sh
   curl -fsSL https://matchblox.sh | sh
   ```

   Read the script first at [matchblox.sh](https://matchblox.sh/install.sh).
   You get the release binary for your machine, its checksum checked and,
   with `gh` logged in, proof that CI built it from this repository. With
   Go, the same version builds from source:

   ```sh
   go install github.com/blox-eng/matchblox/cmd/matchblox@latest
   ```

   With Homebrew, on macOS or Linux:

   ```sh
   brew install blox-eng/tap/matchblox
   ```

2. Run `matchblox` outside tmux (the script starts it for you). Answer
   "Where do your agents run?", then open the setup steps above the board:
   tmux, the queue hooks, the way back.

3. In another terminal, start your agents in tmux, for example
   `tmux new -s app claude`. When an
   agent needs you, its card goes into `NEEDS YOU`. `Enter` jumps to
   it, and `prefix m` comes back.

The full walk is [Get started](https://docs.matchblox.com/start/).

## What it shows

- **The board**: four columns, `NEEDS YOU`, `WORKING`, `SHIPPING` and
  `QUIET`. Each match has one card in one column. The header counts what
  needs you: `● 3 need you`. A match for each: `✦` burning while it works,
  `╿` at rest, `│` spent. Its card says what it does, its pull request on
  its track, its context use and its chain.
- **The peek**: the chosen card shows what the agent said last and the
  options of its question. A digit marks an answer, and the same digit
  sends it. `p` merges a pull request that waits for your merge, after a
  typed `y`. A tap on a column's name hides the column. `g` shows one
  chain, and `s` one host.
- **Go straight in**: `Enter` goes into the agent's own tmux pane, on this
  host or another, and moves only your screen. A wide screen shows the
  chosen agent's live screen, read-only, beside the board. `ctrl+a m` (your
  tmux prefix, then `m`) comes back, with the next card that needs you
  chosen.
- **Switch the agent, keep the seat**: `A` runs another agent in the same
  pane (Claude Code, Codex, OpenCode or Cursor) with the session's name,
  directory and pull request, on the account you last started that agent
  on, else on `default`. The new agent gets saved handover evidence and a
  guide to its workspace and neighboring matches. Codex launches include
  workspace trust and workspace-write network access.
  `matchblox switch` does it from the shell.
- **Get your agents back after tmux dies**: while your agents run, the
  service records each one's conversation, tmux session, folder, branch
  and server. When the server dies, `matchblox recover` lists them, and
  `matchblox recover --all` reopens each conversation in its folder,
  waiting for you, with no duplicates.
- **More than one sign-in**: `N` starts Claude Code, Codex or Cursor on an
  account, a folder beside your default sign-in, or makes a new one: like
  your default, an empty one, or a copy of it that is cloned and its own,
  and a start says what stays shared with the default. You
  finish the sign-in in the session. `N` opens on the agent and the
  account you last started. Cursor accounts run on macOS only for now.
  [Accounts](https://docs.matchblox.com/agents/#accounts).
- **Start where you work**: `N` starts an agent in a tmux session of its
  own. Its `where` offers your repositories: after one yes, matchblox finds
  the Git checkouts in your home folder, on this host only, and keeps the
  list fresh, and says which folders it skipped. `matchblox scan` does it
  from the shell. The list shows each repo with its worktrees and `N live`,
  the ones in use first. Paste a git URL in `where` and N clones it, then
  starts the agent there.
- **Ships on their cards**: every pull request your sessions drive, on one
  track from CI to live, said in words. One that waits for your merge
  merges with `p` and a typed `y`.
- **The strip**: one line under the board. `u` turns it: what `/usage` says
  for each account, in percentages, each named by its provider
  (`✻ Claude Team`, `❋ OpenAI Team`), and the machine. Press `n`
  before you sleep: each session that fills its context compacts itself and
  leaves one line on how to continue.
- **Orders for the stoker**: matchblox starts the stoker agent you choose
  out of sight, on a tmux server of its own. Say in your own words how to
  run the night ("merge them on green") and turn on the gos
  you give. It starts at once, keeps its own plan, follows [your ways
  file](https://docs.matchblox.com/ways/) and logs every step on a card.
  A merge, release or deploy you did not give waits on the card for your
  `y`, and what the stoker would wait for, matchblox fires itself.
- **Three layouts, read from the screen's shape**: one column a page on a
  phone, three columns and a drawer for `QUIET` on a medium screen, and four
  columns on a desk, with the chosen agent's live screen beside them from
  152 columns.
- **Machine**, the second tab: usage, **machine health**, **detached busy
  loops**, the stoker, **git** worktrees that are safe to remove, 24 hours
  of **history** and the tmux panes. The board keeps the
  **recommendations** with their exact command.

More in [The console](https://docs.matchblox.com/console/).

## Keys

| Key | What it does |
|---|---|
| `↑` or `k`, `↓` or `j` | Chooses the card above or below. The chosen card shows its peek. |
| `←` `→` | Goes to the column at the left or the right. |
| `Enter` | The safe action: go into the chosen agent's pane. |
| `a` | Goes into the pane, as `Enter` does. |
| A digit, 1 to 9 | Marks an option of the chosen card's question. The same digit again sends it. |
| `p` | Merges the card's pull request that waits for your merge. It runs only after a typed `y`. |
| `x` | Kills the match. It runs only after a typed `y`. |
| `y` | Gives the go a stoker step waits for. |
| `c` | Copies what the agent said last. |
| `m` | Opens the card's menu. |
| `v` | Shows each card whole, or as one line. |
| `g` | Shows only the chain of the chosen card. `Esc` shows all. |
| `s` | Shows the next host, then all hosts. |
| `u` | Turns the strip: usage, the machine, hidden. |
| `N` | Starts a new session: where, the agent, its account and a first message (none on a new account). |
| `A` | Switches the chosen session to another agent in the same pane. |
| `Tab` | Goes to the board, or to Machine. |
| `/` | Searches the board. |
| `f`, `n` | The stoker on or off; a night that ends in the morning. |
| `q` | Quits. The service keeps running. |

Every key: [Keys](https://docs.matchblox.com/keys/).

## From a phone

Set the startup snippet of your SSH app (Termius, for example) to
`matchblox`, or run `ssh -t ws-1 matchblox`. The board is the first
screen, one column a page: swipe sideways, or tap a page's mark at the top.
Tap a card to see its peek. Tap an option of its question to mark it, and
tap it again to send it. Tap the card again to go into its pane. `m` opens
the card's menu. In Matchblox Pocket a long-press opens it too, a sideways
swipe turns the page, and the keys of the chosen card are on the key belt's
Board layout. Tap `◂ matchblox` in the tmux status line
(with `set -g mouse on`) to come back from a pane, or press `prefix m`. See
[The phone](https://docs.matchblox.com/phone/).

## Safety

Read-only by default. Every action shows its exact command before it runs.
A destructive action needs its key (`x` kills, `p` merges) and then a typed
`y`, and checks its facts
again just before it runs. See [Safety](https://docs.matchblox.com/safety/)
and [SECURITY.md](SECURITY.md).

## Config

matchblox works without a config. The first run writes
`~/.config/matchblox/config.toml` with goals for this machine and the agents
it finds. Every known agent (Claude Code, Codex, OpenCode, Cursor, Gemini,
aider, crush, qwen) is a session wherever it is installed; `agents` adds
others. Every key is in [config.example.toml](config.example.toml) and on
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
