# Changelog

All notable changes to matchblox are in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Changed

- **matchblox.com shows the console on an iPhone Duo.** Folded it plays the
  phone's one column; it unfolds sideways into the desk layout partway
  through, and a tap folds it again. It replaces the Phone, Wide and Desk
  switch, and the 107×60 recording that only the switch used.

## [0.6.0] - 2026-10-12

### Added

- **The console fits the screen it is on.** A closed phone shows one line
  a session, grouped by what each needs from you (waits for you, working,
  quiet), and the selected one opens in place with what it said and its
  keys. A screen held upright is read by its shape, so its keyboard does
  not change the layout: under 90 columns it is this one column, from 90
  (a phone with a small font) the detail is a drawer beside the list. On a
  screen held sideways, from 60 columns, the selected session's detail
  sits beside the list. Every column of a list lines up, the name taking
  what is left, and the selected row is washed across the whole pane with
  its name in the accent, the others muted.
- **Answer or message from a peek.** `a`, or a tap on a selected session,
  opens the conversation: the detail pane itself where there is one, the
  list staying beside it, else a panel at the bottom of a phone, with
  what the agent said last and your answer; a session
  that works gets your line as a message, which Claude Code queues. `→`
  and `←` (a swipe on a phone), or a tap on `next →` and `← prev`, move
  the peek to the next or previous session. When Claude
  Code asks with its question tool, its options are there, numbered: a digit
  or a tap picks one, several questions come one at a time, and a question
  of several choices ticks with `Space`. `Enter` sends, and the peek moves
  to the next session that waits.
- **Ships.** Every pull request a session drives, on one track from open to
  live: CI, a conflict, a review, the merge and the deploy, said in words.
  `x` merges a ship that waits for you, after a typed `y`, pinned to the
  commit that was read green. Name the
  workflows that deploy a repository under the new `[ships.deploy]` config
  key; by default every run on the merge commit counts.
  A running stage says how long it has run (`merged · deploying · 2h`,
  `CI · 6 of 9 jobs · 12m`) or waited (`merged · deploy queued · 1h`). A
  failed run on the merge commit counts once a newer run of its workflow
  on the default branch passed.
- **Usage, the machine and the stoker in the rows the list leaves free.**
  Each account's windows as `/usage` says them, the machine (cpu, memory,
  the disk your home is on, the agents that run) and the stoker, whose
  panel takes the dock while a run is in charge; `c` opens its whole card.
  With rows to spare the dock grows: forecasts, the top processes, the
  recent steps.
- **Recommendations you can put away.** They sit under the sessions now.
  `d` dismisses one until its evidence changes, and `h` folds them all to
  one line; the console remembers both.

### Changed

- The tabs are Sessions, Ships, Machine, Git, History and Panes, each
  named, and the digits `1-6` follow that order. The Queue is the top of
  Sessions, the processes are on Machine, and `p` opens Ships.
- Usage speaks in `/usage` percentages and keeps its forecast; the matches
  and sparks of the header are gone.
- An answer goes on `Enter`: the peek shows what goes where, so there is no
  typed `y` for it any more.
- The keys are at the bottom of the screen, each a tap target, where a
  phone app keeps them above its keyboard.
- **The queue hooks include the question tool.** A `PreToolUse` hook for
  `AskUserQuestion` tells the console what Claude Code asks before its menu
  draws, so the answer peek shows the options in auto mode too, where the
  question reaches the transcript only once it is answered. The step "Add
  the queue hooks" asks to be opened again, with the one entry it adds.

## [0.5.1] - 2026-10-12

### Fixed

- **A session working through its subagents reads `agents`, not idle.**
  Its turn ended, but their results start its next turn: the queue no
  longer lists it as waiting for you, and the stoker still compacts it at
  that turn's end when its pace calls for it. A turn that ended with only
  background shells running stays idle.

## [0.5.0] - 2026-10-12

### Added

- **The stoker compacts each session at the moment that suits it.** It
  follows each session's pace (tokens per point of its progress bar): one
  that finishes under the line is left alone, one whose next step would
  cross it is compacted at the end of this turn. The card says what it
  decided for every session, and why.
- **A session that ended its turn is idle, even while its background
  works.** Its detail shows the subagents and shells it left running, and a
  shell that has run for an hour or more can be ended with `x`.
- **A net under every session.** A door turns Claude Code's own
  auto-compact on at the stoker's line, the only thing that compacts inside
  a turn.

### Fixed

- The site's news line names the latest release: each release writes it
  from the first headline of its changelog section, and links to the
  release.

## [0.4.0] - 2026-10-11

### Added

- **See which sessions your plan's limit stopped, and until when.** A
  session whose turn the limit refused, or whose account spent its window,
  says `out` with the time it can work again, in Sessions and in
  `matchblox status`.
- **The stoker's card tells the truth about the limit.** A run the limit
  stopped says so on its first line (`limit out 11:51 → 14:50`), and a
  session that is out is not counted as ready.
- **A 1M window can fill.** `compact_tokens` (350k by default) fills a
  session before `compact_at`'s percentage does, so the stoker and the
  `! compact` mark act on large windows too. To keep the percentage alone,
  set `compact_tokens = 0`.
- **Every stoker card is kept** in `stoker-runs.jsonl`: a new run never
  erases the one before it.

## [0.3.0] - 2026-10-11

### Added

- **See all of matchblox before you install it.** The replay on
  matchblox.com and in the README walks one evening into the morning: an
  answer typed in one line, a compact and a worktree removal you confirm,
  a busy loop killed, then a night the stoker keeps burning and its score
  at 07:00.

### Fixed

- **A session without hooks shows what it asked** on its queue row, so `a`
  answers a question you can read.
- **A removed worktree leaves the Git tab at once**, not at the next scan.
- One merged worktree and one process are named in the singular.

## [0.2.0] - 2026-10-11

### Added

- **You can see the night.** While the stoker or a night runs, the whole
  console goes dark blue and only the matches keep their fire.
- **Take over with one key.** The first time you act on a session during a
  run, the console asks `Take over?`. `y` rests the stoker while you work,
  and it goes on by itself once you leave. `n` keeps it on.
- **Wake up to the night's score.** Every night and every stoker run ends
  with a card in the queue, even when there was nothing to do: what it
  watched and compacted, the tokens it gave back, and `ready 6/7`, how
  many sessions can go on, with what each of the others needs.
  `matchblox status` shows it too.

## [0.1.1] - 2026-10-11

### Fixed

- **docs.matchblox.com shows the docs of the latest release**, with the
  release install. It showed the text from before the first release.

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

[Unreleased]: https://github.com/blox-eng/matchblox/compare/v0.6.0...HEAD
[0.6.0]: https://github.com/blox-eng/matchblox/compare/v0.5.1...v0.6.0
[0.5.1]: https://github.com/blox-eng/matchblox/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/blox-eng/matchblox/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/blox-eng/matchblox/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/blox-eng/matchblox/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/blox-eng/matchblox/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/blox-eng/matchblox/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/blox-eng/matchblox/releases/tag/v0.1.0
