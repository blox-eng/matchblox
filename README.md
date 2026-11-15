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
<img src="www/demo/replay-desk.svg" width="74%" alt="The matchblox console on a desk, on a demo machine: the list of every match beside the chat of the chosen one. A tap opens the chat of flint-auth, a digit marks an answer from the agent's own options, the same digit sends it, and flint-auth moves into WORKING. g opens the room of the chain login">
<img src="www/demo/replay-phone.svg" width="24%" alt="The same console on a phone: the list first, a tap opens a chat, and the back mark goes back to the list. An answer marked and sent from the chat, and the room of one chain">
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

   matchblox looks for a new release once a day and offers it as a setup step.
   `[update] mode` makes it install by itself, or never look: see
   [Update](https://docs.matchblox.com/config/#update).

2. Run `matchblox` outside tmux (the script starts it for you). Answer
   "Where do your agents run?", then open the setup steps (`! 1 setup` in the list's foot):
   tmux, the queue hooks, the way back.

3. In another terminal, start your agents in tmux, for example
   `tmux new -s app claude`. When an
   agent needs you, it goes into `NEEDS YOU`, and its chat shows what it
   asks. `Ctrl+O` jumps to its pane, and `prefix m` comes back.

The full walk is [Get started](https://docs.matchblox.com/start/).

## What it shows

- **Every agent, one chat**: the list at the left puts each match in a
  group, `NEEDS YOU`, `WORKING`, `HELD` or `IDLE`, with what needs you on
  top. The header counts what needs you: `● 3 need you`. A match for each:
  `╿` struck when it needs you, `✦` burning while it works, `╵` burnt out
  at rest. Its row says what it asks, its pull request and its chain.
- **The chat**: the chosen session's conversation and the options of its
  question, beside the list. Type in its line and `Enter` sends it. A
  paste goes in as one piece. A busy agent that takes no typed text gets
  the message when its turn ends. A digit marks an answer, and the same
  digit sends it. `p` merges a pull
  request that waits for your merge, after a typed `y`. `g` shows one
  chain, and `s` one host.
- **Hosts**: `s` shows the next host, then all hosts. A host you hide
  leaves only all hosts: `s` still reaches it, and `h` shows it again.
- **Go straight in**: `Ctrl+O` goes into the agent's own tmux pane, on
  this host or another, and moves only your screen. `ctrl+a m` (your tmux
  prefix, then `m`) comes back, to the chat you left.
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
- **Ships on their rows**: every pull request your sessions drive, from
  CI to live, said in words. One that waits for your merge
  merges with `p` and a typed `y`.
- **A night**: press `n` before you sleep. Each session that fills its
  context compacts itself and leaves one line on how to continue.
- **Orders for the stoker**: matchblox starts the stoker agent you choose
  out of sight, on a tmux server of its own. Say in your own words how to
  run the night ("merge them on green") and turn on the gos
  you give. It starts at once, keeps its own plan, follows [your ways
  file](https://docs.matchblox.com/ways/) and logs every step on a card.
  A merge, release or deploy you did not give waits on the card for your
  `y`, and what the stoker would wait for, matchblox fires itself.
- **Read from the screen's shape**: on a phone the list and the chat take
  turns; on a wider screen the chat is beside the list.
- **Machine**, the second tab: usage, **machine health**, **detached busy
  loops**, the stoker, **git** worktrees that are safe to remove, 24 hours
  of **history** and the tmux panes. The list's foot keeps the
  **tips** with their exact command.

More in [The console](https://docs.matchblox.com/console/).

## Keys

| Key | What it does |
|---|---|
The keys go to the chat's line or to the list. `Esc` moves them to the
list, and `Enter` there moves them back.

| Key | What it does |
|---|---|
| `Enter` | In the line, sends the text to the chosen session. In the list, goes back to the line. |
| `↑`, `↓` | Chooses the session above or below. |
| A digit, 1 to 9 | With the line empty, marks an option of the chosen session's question. The same digit again sends it. |
| `Ctrl+O` | Goes into the chosen agent's pane. |
| `p` | In the list, merges the pull request that waits for your merge. It runs only after a typed `y`. |
| `x` | In the list, kills the match. It runs only after a typed `y`. |
| `y` | In the list, gives the go a stoker step waits for. |
| `c` | In the list, copies what the agent said last. |
| `m` | In the list, opens the session's menu. |
| `g` | In the list, shows only the chain of the chosen session. `Esc` shows all. |
| `s` | In the list, shows the next host, then all hosts. |
| `N` | In the list, starts a new session: where, the agent, its account and a first message (none on a new account). |
| `A` | In the list, switches the chosen session to another agent in the same pane. |
| `Tab` | Goes to Machine, or back. |
| `/` | In the list, searches it. |
| `f`, `n` | In the list, the stoker on or off; a night that ends in the morning. |
| `Ctrl+C` | Quits. The service keeps running. |

Every key: [Keys](https://docs.matchblox.com/keys/).

## From a phone

Set the startup snippet of your SSH app (Termius, for example) to
`matchblox`, or run `ssh -t ws-1 matchblox`. The list is the first
screen. Tap a row to open its chat, and `◂` to come back. Tap an option of
its question to mark it, and tap it again to send it. `m` in the list
opens the session's menu. In Matchblox Pocket a long-press opens it too,
and the keys of the chosen session are on the key belt's Board layout.
Tap `◂ matchblox` in the tmux status line (with `set -g mouse on`) to
come back from a pane, or press `prefix m`. See
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
