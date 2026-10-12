# 0006 Every screen

The console fits the screen it is on: a closed phone, a phone opened up, a
desk. The approved mock is the reference for every layout in this document:
https://claude.ai/artifact/FzgpXWDRgAE8zPiVcHG7QD (version 4, owner
2026-10-12: "love the design").

## 1. Why

Martin runs matchblox from Termius on his phone (`ssh masterblox`, then
`matchblox`) as much as at the desk. On the phone it is a full control
seat, not a status widget: "a software factory in your hands". He sees and
switches between sessions, unsticks them, answers them, turns the stoker and
the night on, opens a session to guide it, and sometimes starts one.

What feels wrong today, from a capture at 46×34 (the fixture machine):

- `a` answers blind. The action line turns into `ANSWER %3 ▏`; the question
  the session asked is not on the screen.
- Six of the seven tabs show only their digit (`1 QUEUE  2  3  4  5  6  7`).
- The width goes to the wrong things: a truncated door hint in the header,
  `20d2…` ids and `estimated` before the name, a key line cut at `q …`.
- The bottom half of a phone screen is empty.
- A pull request reaches a session only through the session's own cwd. Every
  session of Martin's sits in `~/dev/bloxdev` on `main` and does its branch
  work in worktrees by path, so no session shows its PR. Nothing says
  whether a PR's CI runs, fails, conflicts, merged or deployed.

## 2. Goals

1. **A glance on a phone.** One line a session, grouped by what it needs
   from you. Moving the cursor is reading: the row under it opens in place.
2. **Answer from a peek.** Claude's last message and your answer in one
   panel over the list, then the next session that waits.
3. **Ships.** Every PR a session drives, on one track from open to live, and
   why one is stuck.
4. **Usage and the machine** in the rows the list leaves free.
5. **One console at three sizes.** The same keys and the same data; the
   width decides the layout.

## 3. Non-goals and anti-goals

- No new agent features: matchblox still does not start agents, edit
  prompts or review diffs (design/0001 §15).
- Nothing here needs a setting to look right. The layouts follow the
  terminal's size and nothing else.
- Anti-goal: a phone layout that hides a capability. Every key and action of
  the desk works on the phone.
- Anti-goal: a ship that claims a state it did not read. An unknown state
  says so (`CI unknown · gh not signed in`), never a guess.
- Anti-goal: more GitHub calls than the rate limit allows. The API limit is
  shared with every agent on the machine and ran out on 2026-10-12.

## 4. The three layouts

The screen's shape picks first, then its width (DESIGN.md §6). A cell is
about twice as tall as it is wide, so a screen is portrait when
rows × 2 ≥ columns. A portrait screen under 90 columns is a phone (one
column; from 70 columns its rows use the room for the plan and the ship).
From 90 columns (owner 2026-10-12, on a 107-wide iPhone held upright:
107×60 in Termius with the keyboard up, 107×101 with it down) it splits
the list and the detail at the middle, the detail a drawer on the right,
and no row opens in place. Every portrait screen keeps one dock panel at a
time (`u` switches) and a bottom peek of 72 columns at most, centred, and
keeps its layout as the keyboard comes and goes. A landscape screen goes by
its width, on the breakpoints below, so a phone on its side (205×21) is a
desk whose dock folds when it does not fit. In every layout a row spans its
pane and its columns are fixed tracks, the name taking what is left
(DESIGN.md §6 Rows).

| Layout | Columns | Shape |
|---|---|---|
| Phone | under 60 | One column. Sessions one line each; the selected one opens to 3–4 lines. One dock panel at a time. |
| Opened phone | 60–99 | Two panes split at the middle: the list on the left, the selected item's detail on the right. Usage and machine side by side in the dock. |
| Desk | 100 and over | The list with every column on the left (≈78 columns), the detail on the right, the dock under both. |

### 4.1 The screen, top to bottom

1. **Header**, one line: the mark, `matchblox`, the host (dropped on a
   phone); on the right `● N wait for you` (accent, a tap selects the first
   one) and the stoker word (`stoker off` faint, `stoker` text,
   `☾ night → 07:00` in the night token). A toast replaces the right side
   for 2.5 s after an action (`sent to Billed`, `merged #4556`).
2. **Tabs**, one line, every tab named. The open tab is uppercase in the
   accent with its rule; the others lowercase in `faint`. A count follows a
   name when it means something (`SESSIONS 14`, `ships 4`). When the names
   do not fit, the open tab and its neighbours keep their names and the
   rest shorten to their digit. Order and digits: §11.6.
3. A hairline.
4. **The body** of the open tab.
5. **The dock** (§7), when it fits.
6. A hairline and **the key line**: the keys of the open tab, each a tap
   target, in the accent with a `muted` word (`⏎ open  a answer  p ships
   f stoker`). The desk adds `↑↓ move` and `/ find`, `q quit`.

The key line at the bottom reverses the owner ruling of 2026-10-08 (the
action line under the tabs, because a phone keyboard covers the bottom).
See §11.1. `RUN <command>`, `tap again: <command>`, a search and
the take-over question keep their place in the key line's row.

## 5. Sessions

### 5.1 Grouped by need

Sessions is one list in three groups, each with a faint label and count:

- `WAITS FOR YOU · N`: `asks`, `waits`, `done`, `paused` (the queue's
  waiting rows), oldest wait first.
- `WORKING · N`: `busy`, `agents` (#64), a session running a shell
  (`tests` when the shell is a test runner, else `shell`).
- `QUIET · N`: idle, cold and stale, newest first.

A row: the selection mark `▌`, the match glyph, the name, then on the right
the state word, the wait (`4m`), the context (`24%`, `warn` from 70%, `neg`
past the line). The desk adds the plan bar (`▰▰▰▰▱ 93%`), the PR with its
track (`#4551 ●━◉┄○┄○`) and the place. `s`/`S` sorting stays; while a sort
other than the default is on, the groups go and the list is flat.

### 5.2 The row opens in place (phone)

On a phone the selected row grows, on the wash, by:

1. Claude's last words, in quotes, two lines at most, `muted`, cut with `…`.
2. The PR, when the session has one: `#4545 ●━✕┄○┄○ conflicts with main`.
3. The plan bar and its percent on the left; on the right the row's keys
   (`a answer  ⏎ open` for one that waits, `⏎ open` for the others).

The opened phone and the desk show the same in the detail pane instead:
name, state in words with the wait and the place, `CLAUDE SAID` and the
last message (six lines at most), context and plan bars, the ship, the
subagent count, the background shells, and the keys.

### 5.3 Moving

`↑↓`/`jk` move the selection; Termius turns a vertical swipe into them. A
tap on a row selects it; a tap on the selected session opens the peek
(§6), to answer it or to message it. The list scrolls so the whole
opened row is on the screen.

## 6. The answer peek

`a` on any session with a pane opens a panel over the list, everything
around it dimmed (owner 2026-10-12: message any session, not only one that
waits; Claude Code queues a message typed while it works):

- **Phone:** full width up to 72 columns (centred past that), anchored to
  the bottom, above the key line (see §11.1).
- **Opened phone and desk:** a centered box, 62 columns at most.

Inside, under a title `Billed asks` (`Billed said` when it waits with no
question, `To Billed` when it does not wait):

1. Claude's last message, wrapped (from the transcript's last assistant
   text, as `said` reads it; 8 lines on a phone, 9 elsewhere, the rest cut
   with `…`).
2. When the session asks with its question tool (AskUserQuestion), its
   options, numbered: `› 1 Start task 11`. `↑↓` moves, a digit or a tap
   picks and sends. The last option is always `Type an answer…`.
3. A typing line `› ▏` when there are no options or the typing option is
   picked.
4. The footer: `⏎ send ─ ← prev  3/7  next → ─ esc` (the place in the
   list when it fits). Each part is a tap target.

`⏎` sends: a typed line goes into the pane as today's answer does; an
option is sent the way the question tool takes it in the pane (the builder
reads how it renders; it must never type text into a menu). The peek is the
review: what you see is what is sent, to the pane named in the title, so no
second confirm (today's `⏎ review` step goes, §11.2).
After a send the toast says `sent to <name>` and the peek moves to the next
session that waits, or closes when none does. `→` and `←` (Termius sends
them for a horizontal swipe), and `⇥`/Shift+Tab, move the peek to the
next and previous session in the Sessions order (who waits first),
wrapping. Every letter is text, `n` and `p` too (owner 2026-10-12); an
option is picked with `↑↓` and `⏎`, a digit or a tap. Each move opens with nothing
picked and takes no key for 400 ms. `esc` closes. A message to a session
that does not wait has the answer's guards: the same agent is still in the
pane, it does not ask with the question tool (its options path stays) and
is not at a permission prompt; a failed guard sends nothing and says why. A permission prompt (said.Permits) is not answered here: its
peek shows the prompt and `⏎ open` to answer it in its pane.

## 7. The dock: usage and the machine

The dock sits between the body and the key line when the body leaves room
for it and one blank row (the body's lines, or the taller of list and
detail, plus the dock's height plus one, fit the rows). It is never
scrolled: when it does not fit it folds away and its content is on the
Machine tab.

- **Usage**, one block per account a session runs under, what `/usage`
  says: Claude the 5-hour window, the week and the week's Opus (or Sonnet)
  window when the reading has it; Codex its windows; an account with no
  reading `not measured`. Each window: the name (`5h`, `wk`, `opus`), a bar
  (`warn` from 60%, `neg` from 90%), the percent, the reset (`· 16:00`,
  `· Thu 09:00`). On a phone each account takes at most two lines (Codex
  folds to one: `5h ▰▱▱ 12% · wk 30%`). The data is today's limits package;
  the header's limit lines (DESIGN.md §6 Limits) move into the dock and the
  Machine tab, and the header no longer has them.
- **Machine**: cpu with a sparkline of the last minute and the load, memory
  `41 of 64 GB`, disk, and `runs 13 claude · 1 codex · 1 opencode`.
- **Phone:** one panel at a time, usage first. Its label line ends with
  `machine ›` (or `usage ›`) in the accent; `u` or a tap switches.
- **Opened phone and desk:** usage on the left, machine on the right, split
  as the panes above.
- **Folded** (the keyboard is up, the list is long): the Machine tab is
  named `machine`, and shows usage first, then the machine, then its
  existing lists.

## 8. Ships

A new tab, `2 SHIPS N` (N = PRs in flight), key `p` or `2`.

### 8.1 What a ship is

A pull request a session drives. A session drives a PR when its own
transcript shows it, strongest first:

1. it created it: a `gh pr create` result with the PR's URL;
2. it pushed its branch (`git push … <branch>`) and an open PR has that head;
3. it acted on it: `gh pr merge|checks|view|edit|ready|comment` with the
   PR as its selector, by number or URL.

Mentions in prose do not count, nor does a PR URL anywhere else in a
command (`curl`, `echo`, `gh api`, a commit message or a PR body). One PR belongs to the session with the
strongest and then the newest evidence. The repository comes from the URL or
the command's directory. Today's link through the worktree's branch (§6
Pull requests) stays as a fourth source. The transcript is read the way the
sampler already reads it, incrementally, never whole each sample.

### 8.2 The track

`open → ci → green → merged → live`, drawn `●━●━◉┄○┄○`: done stages `●`
muted, the current one `◉` (`warn` while running, accent when it waits for
you) or `✕` in `neg` when stuck, the rest `○` faint. The note says the
state in words:

| State | Note |
|---|---|
| Checks running | `CI · 6 of 9 jobs` |
| A check failed | `CI failed · <job>` |
| Conflicts / behind | `conflicts with main` / `behind main` |
| Draft | `draft` |
| Checks green, not merged | `green · waits for your merge`, or `auto-merge on` |
| Review requested or changes requested | `waits for review` / `changes requested` |
| Merged, deploy running | `merged · deploying` |
| Merged, deploy green | `live` |
| Unknown | `state unknown · <why>` |

**Live** is: every workflow run on the merge commit finished, and green. A
repository can name its deploy workflows in config; the default is every
run on that commit.

A ship stays on the list until the end of the day it went live, then drops.

### 8.3 Reading GitHub

- One GraphQL query per repository per poll, only for the PRs in flight
  (open, or merged and not yet live): state, mergeable, mergeStateStatus,
  isDraft, reviewDecision, autoMergeRequest, the check rollup counts, and
  the merge commit with its workflow runs.
- Every 60 s while any PR is in flight, none when none is. A poll that
  finds the remaining GraphQL budget under 10% stops until the reset and
  the notes say `paused · GitHub rate limit until 14:33`.
- No `gh`, or `gh` not signed in: ships still list (from the transcript)
  with `state unknown · gh not signed in`.
- The serve does the reading and puts ships in the state, like the git scan;
  every console reads them from there.

### 8.4 Rows and keys

Phone: `▌#4551 The Playwright nightly tests…  6m`, then the track and the
note; the selected one adds `by Playwright` and its keys. Opened phone and
desk: the detail shows the title, `by <session>`, and the five stages
vertically, each with its time or its note.

`⏎` goes to the driving session (selects it in Sessions). `x` on a ship that
waits for your merge shows `RUN gh pr merge <n> --squash --match-head-commit
<sha>` (the repository's merge method, pinned to the head read green) and
runs after a typed `y`, as every destructive step does. A ship that is stuck
has `a`: the answer peek on its session.

## 9. What stays

- Every key of docs/keys.md keeps its meaning, except the tab digits
  (§11.6); new keys are `p` (Ships) and `u` (dock panel). `f` and `n` stay
  the stoker and the night (the mock's stoker picker is not built).
- Git, History and Panes keep their bodies; they gain the bottom key line
  and the dock.
- Doors, the STOKED card, recommendations and the take-over question keep
  their behaviour, at the top of Sessions.
- The night look, the theme tokens and the motion rules (DESIGN.md §2, §5).

## 10. Done when

- At 44×31, 86×30 and 132×32 on the fixture machine, the console matches
  the mock's layout for Sessions, the answer peek, Ships and the dock.
  Replay frames at the three sizes are in `www/demo` and tested.
- `a` on a session that asked with the question tool shows its options, and
  picking one answers it in a real Claude Code pane (a live check on a
  private tmux socket).
- A PR created by a session shows on its row and in Ships within one poll,
  through CI to live, on a real repository.
- With the keyboard up (fewer rows) the dock folds into the Machine tab.
- docs/console.md, docs/keys.md, docs/phone.md and DESIGN.md §6 describe
  what shipped; the landing page's device switch is a follow-on (§12).

## 11. Decided (owner, 2026-10-12: "agreed with everything")

1. **The key line sits at the bottom**, reversing the 2026-10-08 ruling: a
   phone app that shrinks the terminal for its keyboard keeps the bottom row
   above it. The live check on Martin's phone, keyboard up, confirms it
   before merge.
2. **The answer peek sends on `⏎`.** The peek shows what goes where; it is
   the review. Today's typed-`y` confirm for an answer goes.
3. **Merge from Ships is `x` then a typed `y`.**
4. **A ship drops off the list at the end of the day it went live.**
5. **Usage speaks `/usage` percentages** and keeps the forecast
   (`lasts the week`, `out Fri ~20:00`) on its block. The header's limit
   lines move into the dock and the Machine tab; matches and sparks (#38) go.
6. **Tabs:** `1 SESSIONS` (today's Queue folds in: the STOKED card, the
   doors and the recommendations sit above `WAITS FOR YOU`, and the console
   opens here), `2 SHIPS`, `3 MACHINE` (usage, the machine, then the
   processes list of today's Procs tab), `4 GIT`, `5 HISTORY`, `6 PANES`.
   The Queue and Procs tabs go; the digits follow the new order; `p` opens
   Ships. Everything the Queue and Procs tabs did (their rows, `⏎`, `x`,
   the procs alert marker) moves with their rows.
7. **`⏎ open` is today's jump** into the session's own pane (tmux
   switch-client or attach); the mock's in-console screen stood for it.
8. **The question tool, fully:** several questions are shown one at a time
   (`1 of 2` in the title), multi-select questions tick with `space` and
   send with `⏎`; options are read from the pending tool call, from the
   `PreToolUse` hook of the question tool (auto mode writes the call to the
   transcript only once it is answered) or else the transcript, and sent
   as the tool's own menu takes them in the pane.
9. **Live** is: every workflow run on the merge commit finished and none
   failed. A cancelled or skipped run counts when a later run of the same
   workflow on the default branch succeeded (a deploy superseded by a newer
   one). Repositories may name their deploy workflows in config.
10. **Ships survive a restart.** Found ships and each transcript's read
    offset persist in the state directory. A transcript is read whole once
    (in the background, for PR evidence only), then only its new bytes.

## 12. Later

- The landing page shows this mock with its device switch, playing a short
  scripted run until a visitor touches it.
- Starting a session from the console (#10).
