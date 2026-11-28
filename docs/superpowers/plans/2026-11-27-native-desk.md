# Native desk Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Spec: docs/superpowers/specs/2026-11-27-native-list-bar-chrome-design.md
Spec approved: 2026-11-27
Test command: go test ./internal/app ./internal/remote ./internal/panes ./internal/service ./internal/state ./cmd/matchblox -count=1
Plan approved: 2026-11-27 by the owner
Next: builder:build

**Goal:** A builder at a desk sees which repo needs them and what finished, with the agent's own pane beside the list.

**Architecture:** Branch from `origin/main` after #514. The desk window is four mux panes: header, list, agent (`ViewArgv` of the agent's window), bar. The list is today's bubbletea console. Header and bar are `matchblox frame` clients of the same service, display-only. No vt. No herdr. Last-given-keys live on the service (one record per owner). `t` splits in the agent's tmux window. `D` respawns the agent seat with a matchblox diff.

**Tech Stack:** Go, bubbletea v2, tmux via `internal/mux` (existing). No `charmbracelet/x/vt`. No new module.

**Spec:** `docs/superpowers/specs/2026-11-27-native-list-bar-chrome-design.md`

## Global Constraints

- Start from `origin/main` after #514, not from an older checkout.
- No `charmbracelet/x/vt`, no `creack/pty` added by this work. `go list -m all` stays free of both.
- New session is `N`. `n` stays night.
- Park (`holdOf`) still beats an ask: a parked row stays HELD, as today.
- Four-pane frame only when beside is on, home session, width ≥ 100, not Pocket. Under 100 columns and Pocket: today's two-pane or one-pane sidebar.
- `ctrl+q`: list pane active → send to matchblox (close sheet, else leave REVIEW, else flip). Agent or shell active → today's flip to the list.
- LIST `t` opens a shell or, if one exists, focuses SHELL. SHELL `t` focuses AGENT. AGENT does not steal `t`.
- Commits: signed Conventional Commits. Never `--no-verify`.
- This is a feat: edit `www/` in the same PR. Landing headline: `Every repo. Every agent. One desk.`
- No herdr.

## Review Focus

1. **A key meant for the agent reaches the list, or the reverse.** Hostile: with AGENT keys, `,` types a comma into the agent pane capture; `?` does not open. → Task 5, `TestCommaTypesIntoTheAgent`.
2. **A review paste hits the wrong pane.** Hostile: two sessions; `D` on ux-pass; send must Type/Paste only ux-pass. → Task 8, `TestReviewPasteHitsOnlyThatPane`.
3. **A diff line with CSI in the file moves the terminal title.** Hostile: file contains `\033]0;pwned\007`. Capture of the review pane has the CSI as visible text; the test process title is unchanged. → Task 8, `TestDiffEscapeStaysVisibleText`.
4. **DONE hides a wait.** Hostile: busy→idle with an ask on the next sample; row is NEEDS YOU, not DONE. → Task 3, `TestAskBeatsDone`.
5. **Frame kill of the agent session.** First start must kill only the old viewer pane, never the agent's session. → Task 1, `TestUpgradeLeavesTheAgentSession`.

## File map

- `internal/mux/mux.go` / `internal/mux/tmux.go` (change) — `SplitEdge(target, argv, edge, cells)`: `above`/`below`/`right`, size in cells. `SplitIn(target, argv, dir)`: split with `split-window -c dir`.
- `internal/app/desk.go` (create) — opens header and bar panes, 100-column gate, first-start migrate.
- `internal/app/desk_tmux_test.go` (create) — real-tmux tests of the four-pane frame.
- `internal/app/frame.go` (create) — `matchblox frame --kind=header|bar|review` (display or review). Do not reuse `internal/app/sheet.go` (stoker launch).
- `internal/app/spaces.go` (create) — space chips, filter, `+N ▾` menu over the list; `●N` is needs-you only.
- `internal/app/sidestate.go` / `internal/app/sidelist.go` (change) — insert `sideDone` between need and work; ages; `shell` word on the row.
- `internal/state/state.go` / `internal/proto/proto.go` (change) — per-owner `KeysAt` (session id → time) on `state.Doc`.
- `internal/service/keysat.go` (create) — records KeysAt when the agent gets the keys. Do not touch `internal/service/keys.go` (send-keys).
- `internal/app/bar.go` (create) — mode word, host/usage/stoker chips, CONFIRM text; no key line on the desk.
- `internal/app/keysheet.go` (create) — `?` keys sheet over the list.
- `internal/app/cogsheet.go` (create) — `,` ⚙ sheet; reuses Machine body from `internal/app/dock.go`.
- `internal/app/shell.go` (create) — `t` split or focus in the agent's window.
- `internal/app/review.go` (create) — `D` via `frame --kind=review`; comments; one send.
- `internal/panes/beside.go` (change) — `ctrl+q` bind per the spec; `t` bind only on the shell pane.
- `internal/remote/view.go` (change) — only if Task 7's visibility test fails; today's grouped session already shares the agent's window.
- `cmd/matchblox/main.go` (change) — `frame` subcommand; desk wiring.
- `DESIGN.md` (change) — §4 glyphs; §6 follows today's sidebar plus the four-pane desk at ≥100 columns (do not resurrect the board as home).
- `README.md`, `docs/console.md`, `docs/keys.md`, `docs/phone.md`, `www/`, `ios/Matchblox` belt + `internal/app/boardkeys.go` (change) — merge pass.

---

### Task 1: Four mux panes, native agent

**Files:** `internal/mux/mux.go`, `internal/mux/tmux.go`, `internal/app/desk.go`, `internal/app/desk_tmux_test.go`, `internal/app/frame.go`, `internal/panes/beside.go`, `cmd/matchblox/main.go`

**Surface:** an agent's tools

**Test first:**
- `TestSplitEdgeOpensAOneRowPaneAbove` — mux `SplitEdge(target, argv, "above", 1)` returns a pane; `list-panes` height is 1.
- `TestDeskFrameOpensFourPanes` — home session, 160×40; four panes: header and bar run `matchblox frame`; list runs the console; agent runs `ViewArgv` (not `matchblox`).
- `TestDeskFrameStaysTwoPanesUnder100` — width 90; no header/bar panes.
- `TestUpgradeLeavesTheAgentSession` — existing two-pane beside plus agent session `ux-pass`; first desk start kills the old viewer pane only; `ux-pass` still exists; list foot once contains `desk frame on · the agent pane is still yours`. Flag `Doc.DeskFrame` so a second start is silent.
- `TestDeskFrameHeaderGoneSaysSo` — kill the header pane; list foot `the desk frame did not open:` then the error.
- `TestCtrlQFromListSendsToMatchblox` — list focused, `C-q` is sent to the console pane (not `send-keys` into the agent). Flip in this task may be a no-op if no agent focus yet.
- `TestNoVtDependency` — `go list -m all` has no `charmbracelet/x/vt` and no new `creack/pty`.
- Existing: `TestTheFlipKeyTypesOutsideTheConsoleWindow`, `TestAViewTypesIntoTheAgentsOwnPane` still pass.

**Run:** `go test ./internal/app ./internal/panes ./cmd/matchblox -count=1 -run 'TestDeskFrame|TestUpgrade|TestCtrlQFromList|TestNoVt'`
Before: FAIL undefined desk. After: PASS.

**Do:**
1. Add `Mux.SplitEdge(target, argv, edge, cells)` on the tmux adapter (`split-window -v/-h -l cells -b` as edge needs). Task 1's four-pane layout uses it; do not call tmux outside mux.
2. `desk.go`: if `opt.Beside` and window width ≥ 100, `SplitEdge` a 1-row header above and a 1-row bar below, then keep today's list|agent split in the middle. Window options: keep `@matchblox_console`, `@matchblox_beside`, `@matchblox_chain`; add `@matchblox_header`, `@matchblox_bar`, `@matchblox_mode`. The **list process** writes `@matchblox_mode` whenever keys move (LIST, AGENT, …).
3. Header and bar argv: `matchblox frame --kind=header|bar` (same binary, same socket). They only View. Clicks send acts, or no-op until Task 2/4.
4. First start: if a beside pane exists without header/bar options, kill that viewer pane (not the agent session), open the four-pane frame, set `Doc.DeskFrame`, notice once.
5. `BesideKeysArgv`: if `pane_id == @matchblox_console`, `send-keys C-q` to that pane; else today's `select-pane -Z -t @matchblox_console`.

**Done when:** `TestDeskFrameOpensFourPanes` and `TestUpgradeLeavesTheAgentSession` pass.

**Commit:** `feat(app): four mux panes around the native agent`

---

### Task 2: Spaces in the header

**Files:** `internal/app/spaces.go`, `internal/app/desk.go`, `internal/app/frame.go`

**Test first:**
- `TestSpaceFilterKeepsNeedYouCount` — three repos, waits on docs, matchblox, api; tap `docs ●1`; list names only docs; header still `● 3 need you`.
- `TestSpaceCountIgnoresDone` — a DONE session in matchblox does not add to `matchblox ●N`.
- `TestSpacesOverflowOpensMenu` — more spaces than fit; `+3 ▾` then ⏎/tap opens the rest **over the list** (header stays one row); choosing one filters.
- Empty: `Nothing in api on gpu-1. esc shows all.`
- Selected space lives on the list Model (`m.space`) and in window option `@matchblox_space` so the header frame reads it. It is not a snapshot field.

**Run:** `go test ./internal/app -count=1 -run 'TestSpace'`
Before: FAIL. After: PASS.

**Do:** Reuse `repoOf`. `[` `]` from LIST cycle spaces. `all` has no count. One repo: no space chips. Combine with host filter `s`. Header frame reads `@matchblox_space` and the snapshot's sessions; it does not own the filter.

**Done when:** `TestSpaceFilterKeepsNeedYouCount` passes.

**Commit:** `feat(app): repo spaces filter the desk list`

---

### Task 3: DONE, ages, glyphs

**Files:** `internal/app/sidestate.go`, `internal/app/sidelist.go`, `internal/service/keysat.go`, `internal/state/state.go`, `internal/proto/proto.go`, `DESIGN.md`

**Surface:** an agent's tools (KeysAt is not a secret; it is a timestamp)

**Test first:**
- `TestBusyToIdleWithNoAskIsDone` — sample busy then idle, no question, no permission, no stuck PR; row group `DONE`; time `now`; second line starts `said `.
- `TestAskBeatsDone` — same, but the idle sample has an ask; group `NEEDS YOU`.
- `TestEnterLeavesDone` — ⏎ on a DONE row records KeysAt; next snapshot group `IDLE`; `✓ N done` drops by one.
- `TestArrowsDoNotLeaveDone` — ↑↓ keep DONE.
- `TestStuckPRStaysNeedsYou` — CI failed / conflicts / changes requested stays `NEEDS YOU`.
- `TestChainDoneMarkDoesNotCount` — ask + finished member: chain in NEEDS YOU; `✓ N done` does not include the nested member.
- `TestParkedAskStaysHeld` — holdByYou and an ask → HELD (today).

**Run:** `go test ./internal/app ./internal/service -count=1 -run 'TestBusyToIdle|TestAskBeats|TestEnterLeaves|TestArrows|TestStuckPR|TestChainDone|TestParkedAsk'`
Before: FAIL no groupDone. After: PASS.

**Do:**
1. In `sidestate.go`, insert `sideDone` between `sideNeed` and `sideWork`. Heads: NEEDS YOU, DONE, WORKING, IDLE, HELD. Empty group: no head. Keep `holdOf` + `holdByYou` first (park beats ask).
2. `KeysAt` on `state.Doc`, per owner, keyed by session id. Set on ⏎, a row click that gives keys, `ctrl+q` onto the agent, `⌃o`.
3. DONE = last turn ended (Works/Busy → idle, no ask, no permission, no stuck PR) after KeysAt, and not held by you. **KeysAt zero:** the first such idle turn is DONE (never given keys counts as before the turn).
4. Time in state at the right before the agent mark: `now` under a minute, else `10m` (today's age cell).
5. `DESIGN.md` §4: works `✦`, asks `╿`, done `✓` (`match-ember`), idle `╵`, held `╽`, spent `│`. One meaning each.

**Done when:** `TestAskBeatsDone` and `TestEnterLeavesDone` pass.

**Commit:** `feat(app): DONE group and time in state`

---

### Task 4: Bar, no key line on the desk

**Files:** `internal/app/bar.go`, `internal/app/frame.go`, `internal/app/model.go`

**Test first:**
- `TestBarShowsBusyLoopOnThatHostOnly` — demo busy loop on gpu-1; bar contains `⚠ busy loop` on gpu-1's chip; ⚙ shows `!`; other host chips have no neg.
- `TestConfirmUsesTheBar` — kill confirm: bar `CONFIRM  kill flint-auth?  y kill · esc keep`; list still has the keys (`y` kills).
- `TestDeskHasNoKeyLine` — 160 columns; the window's last row is the bar, not `c v s`.
- `TestTabSilentOnDesk` — ⇥ on the four-pane desk does not open Machine and prints nothing.

**Run:** `go test ./internal/app -count=1 -run 'TestBar|TestConfirmUsesTheBar|TestDeskHasNoKeyLine|TestTabSilent'`
Before: FAIL. After: PASS.

**Do:** Mode word from `@matchblox_mode` (list process writes it). Hosts, usage (5h and week with ↻), stoker with `≋ N`. Trouble colour on its chip only. AGENT/SHELL: bar shows `ctrl+q list` before `? keys`. Notices stay 2.5 s at the list foot. Machine tab gone on the four-pane desk only. Chip tap is Task 6.

**Done when:** `TestBarShowsBusyLoopOnThatHostOnly` passes.

**Commit:** `feat(app): desk bar for mode and the machine`

---

### Task 5: `?` over the list

**Files:** `internal/app/keysheet.go`

**Test first:**
- `TestQuestionOpensOverTheList` — `?` from LIST; agent pane capture is unchanged; list contains the keys sheet.
- `TestQuestionShellRunsT` — type `shell`, ⏎; same act as `t` (Task 7 may stub a call).
- `TestCommaTypesIntoTheAgent` — focus agent; `,` appears in the agent pane; no ⚙.
- `TestNoKeyDoesX` — filter `xyz` → `No key does "xyz". esc clears it.`

**Run:** `go test ./internal/app -count=1 -run 'TestQuestion|TestCommaTypes|TestNoKeyDoes'`
Before: FAIL. After: PASS.

**Do:** Groups: anywhere (`ctrl+q`, `ctrl+o`, `N`, `s`, `esc`), the list (`⏎` `↑↓` `[` `]` `t` `D` `g` `G` `z` `x` `/` `m` `c`), a review, the shell, ⚙. `ctrl+q` closes the sheet. `?` from AGENT types.

**Done when:** `TestQuestionOpensOverTheList` passes.

**Commit:** `feat(app): keys sheet over the list`

---

### Task 6: `,` opens ⚙ over the list

**Files:** `internal/app/cogsheet.go`, `internal/app/dock.go` (reuse Machine body; do not rewrite it)

**Test first:**
- `TestCommaOpensCogOverTheList` — `,` from LIST; list shows today's Machine content (from `dock.go`); agent pane unchanged.
- `TestEscReturnsToTheSameRow` — choose a row, `,`, `esc`; same row chosen.
- `TestCtrlQClosesCog` — `ctrl+q` from ⚙ → LIST, sheet gone.
- `TestHeaderCogOpensMachine` — tap `⚙` in the header (same as `,`).
- `TestBarChipOpensCogOnThatLoop` — tap gpu-1's `⚠ busy loop` chip; ⚙ opens on that loop; `esc` returns to the same row.

**Run:** `go test ./internal/app -count=1 -run 'TestCommaOpens|TestEscReturns|TestCtrlQClosesCog|TestHeaderCog|TestBarChipOpens'`
Before: FAIL. After: PASS.

**Do:** Draw Machine from `dock.go` in a sheet over the list (`cogsheet.go`, new type, leave `sheet.go` as the stoker launch). `⇥` stays silent on the desk (Task 4). Phone/narrow: `⇥` still opens Machine.

**Done when:** `TestCommaOpensCogOverTheList` and `TestEscReturnsToTheSameRow` pass.

**Commit:** `feat(app): comma opens Machine over the list`

---

### Task 7: `t` shell in the agent's window

**Files:** `internal/mux/tmux.go` (`SplitIn`), `internal/app/shell.go`, `internal/panes/beside.go`, `internal/app/sidelist.go`; `internal/remote/view.go` only if `TestTOpensShellInAgentWindow` fails on today's grouped `ViewArgv`

**Surface:** an agent's tools

**Test first:**
- `TestTOpensShellInAgentWindow` — LIST `t` on ux-pass; a new pane in ux-pass's window has cwd = `s.Cwd`; desk agent capture shows the split; divider `shell · <cwd> · t switches`.
- `TestTSecondPressFocusesShell` — LIST `t` again does not add a third pane; it focuses the existing shell.
- `TestTSurvivesQuit` — quit matchblox, start again; that shell pane still exists.
- `TestTFromShellFlipsToAgent` — with keys on the shell pane, `t` focuses the agent pane (tmux bind on that pane only).
- `TestTFromAgentTypesT` — AGENT keys; `t` appears in the agent capture (no window-wide `t` bind).
- `TestTNotReached` — unreached host → `not opened: gpu-1 is not reached`.
- `TestTNoFolder` — no cwd → `not opened: ux-pass has no folder`.
- `TestTRowSaysShell` — row shows the word `shell` in muted before the agent mark, never `▣`.
- `TestTEndsWhenTheShellExits` — `exit` in the shell; tag and divider go.

**Run:** `go test ./internal/app ./internal/remote ./internal/mux -count=1 -run 'TestTOpens|TestTSecond|TestTSurvives|TestTFromShell|TestTFromAgent|TestTNotReached|TestTNoFolder|TestTRow|TestTEnds'`
Before: FAIL. After: PASS.

**Do:**
1. `SplitIn(target, argv, dir)` → `split-window -c dir`. argv is `$SHELL` if set, else `/bin/sh`, login not required.
2. LIST `t`: if this session already has a matchblox shell pane, `Focus` it (mode SHELL). Else `SplitIn` once under the agent pane in the agent's window.
3. Bind `t` **only** on the shell pane: send to the console or `select-pane` the agent. Do not bind `t` on the agent pane.
4. Keep today's `ViewArgv` grouped session. If the visibility test fails, then change `ViewArgv` so the split is visible; do not attach the agent's own named session (that would steal other clients).
5. `?` `shell` ⏎ calls the same function as LIST `t`.

**Done when:** `TestTOpensShellInAgentWindow`, `TestTSecondPressFocusesShell` and `TestTFromAgentTypesT` pass.

**Commit:** `feat(app): shell under the agent in its window`

---

### Task 8: `D` review in the agent seat

**Files:** `internal/app/review.go`, `internal/app/frame.go` (`--kind=review`), `internal/service/keysat.go` or Start path, `internal/proto/proto.go` (`StartBranch` on the session)

**Surface:** an agent's tools; secrets (file text, review paste)

**Test first:**
- `TestDHidesTheAgentView` — `D` on ux-pass; beside pane runs `matchblox frame --kind=review`, not `ViewArgv`.
- `TestTwoCommentsSendOnePaste` — `c`, type, ⏎, `c` on another line, type, ⏎, ⏎ with no field; one `paste:` to ux-pass contains both `file:line`; then `ViewArgv` is back; keys LIST; row WORKING.
- `TestReviewPasteHitsOnlyThatPane` — two sessions; paste target is only ux-pass.
- `TestDiffEscapeStaysVisibleText` — file with `\033]0;pwned\007`; review pane **shows those bytes as visible text** (render ESC as `^[` or equivalent; do **not** strip them); the test process title is unchanged.
- `TestEscLeavesReview` — `esc` in REVIEW restores `ViewArgv`, keys LIST.
- `TestReviewNoGitFolder` / `TestReviewNoChanges` / `TestReviewBaseGone` — spec strings.
- Base: `StartBranch` recorded at Start as `git rev-parse --abbrev-ref HEAD` in the session dir. If missing: `git symbolic-ref refs/remotes/origin/HEAD`, else `main`, else `master`.
- Too large: a file over 4000 lines or 400 KiB → file list only; ⏎ opens that file.

**Run:** `go test ./internal/app ./internal/service -count=1 -run 'TestDHides|TestTwoComments|TestReviewPaste|TestDiffEscape|TestEscLeavesReview|TestReviewNo'`
Before: FAIL. After: PASS.

**Do:** Respawn the beside pane with `matchblox frame --kind=review --session ux-pass` (same `frame` as header/bar, not a third subcommand). Draw file bytes with CSI/OSC visible as text. Send through today's `paste:` act and cap. After send, respawn `ViewArgv`. `ctrl+q` in REVIEW is the same as `esc`.

**Done when:** `TestDiffEscapeStaysVisibleText`, `TestReviewPasteHitsOnlyThatPane` and `TestTwoCommentsSendOnePaste` pass.

**Commit:** `feat(app): review in the agent seat`

---

### Task 9: Landing, docs, Pocket belt, DESIGN §6

**Files:** `DESIGN.md`, `README.md`, `docs/console.md`, `docs/keys.md`, `docs/phone.md`, `www/`, `www/demo/`, `cmd/matchblox/replay_test.go`, `ios/Matchblox` belt, `internal/app/boardkeys.go`

**Test first:** `.github/scripts/landing-guard.sh "feat(app): the native desk" /tmp/body origin/main` fails if `www/` is unchanged. After: pass with www edited. `go test ./cmd/matchblox -run TestReplay -count=1` updates or asserts desk frames.

**Do:**
1. Landing H1: `Every repo. Every agent. One desk.` Stills and replay: `TestReplay` (`cmd/matchblox/replay_test.go`) with `-update` if the suite uses it. Demo names only (`ws-1`, `flint-auth`, `gpu-1` as the demo host). No real people or private hosts.
2. In `docs/console.md` the **desk** subsection names the bar, `?`, `,`, DONE, `t`, `D` and does not contain the phrase `key line`. `docs/phone.md` and the sidebar subsection may still say key line / Machine / `⇥`.
3. `DESIGN.md` §6: four-pane desk at ≥100 columns; under 100 and Pocket keep **today's sidebar console** (`docs/console.md`), not the board. Do not make the board the home again.
4. Pocket belt: add `?` `,` `t` `D` in `ios/Matchblox` and the Go belt ids in `internal/app/boardkeys.go`. Drop or fold keys so the tail still fits; layout stays sidebar. Run the existing Swift belt test if the machine has Xcode; otherwise the Go belt-id test.
5. `g` still tiles in the agent seat; `G` opens the chain room in the list (header and bar stay).

**Done when:** landing-guard passes; `docs/console.md` desk subsection has no `key line`.

**Commit:** `feat: the desk on the landing and in the docs`

---

### Task 10: Review the native desk

**Files:** none new unless review fixes.

**Do:** Load `builder:review`. Spec: `docs/superpowers/specs/2026-11-27-native-list-bar-chrome-design.md`. Test command: the header's. Grep the diff for `charmbracelet/x/vt`, `creack/pty`, real host names in `www/` and `design/`. No open Critical or Important. Hostile tests of Tasks 1, 3, 5, 8 still pass.

**Done when:** review has no open Critical or Important finding.

**Commit:** none unless `fix(app): …` for a finding.

---

## Lens findings

**Engineering — scope:** Reuse beside, `repoOf`, Machine view, `ViewArgv`, mux Split/Focus. Minimum is frame + groups + bar + two sheets + `t` + `D` + merge pass. More than eight files. No cut, because each file is one spec surface and cutting `t` or `D` drops a done-when. No new module.

**Engineering — architecture:** Service owns KeysAt and StartBranch. App owns drawing and sheets. Mux owns splits (`SplitEdge`, `SplitIn`). Frame clients are display; keys live in the list, agent, shell, or review pane. Production failure: header pane dies — `TestDeskFrameHeaderGoneSaysSo`. Review paste to the wrong pane: Task 8.

**Engineering — tests:**

```
four panes at 160; two under 100              — Task 1
upgrade does not kill the agent session       — Task 1
header pane gone says so                      — Task 1
space filter; ● N counts all waits            — Task 2
busy→idle → DONE; ask beats DONE              — Task 3
⏎ leaves DONE; arrows do not                  — Task 3
bar busy-loop chip; confirm in bar            — Task 4
AGENT bar shows ctrl+q list                   — Task 4
? over list; , types into agent               — Task 5
comma opens ⚙; esc same row; chip tap         — Task 6
t in agent window; t types in AGENT           — Task 7
D paste one message; CSI stays text           — Task 8
landing + docs                                — Task 9
```

**Engineering — regression:** today's beside, `ctrl+q` outside the console window (still send-keys), Pocket sidebar, `N`/`n`. Tests: existing `TestTheFlipKeyTypesOutsideTheConsoleWindow`, Task 5 `TestCommaTypesIntoTheAgent`, Task 1 under-100.

**Design — every state:** empty (`No sessions yet. + new session starts one here (N).`); frame error (spec table); `D` fail strings; `t` fail strings; `No key does "xyz"`. Rating: spec copy ≥ 8. No change, because the words are the spec's.

**Architecture lens:** KeysAt is a new fact on the service (one owner). Frame clients must not keep a second KeysAt. Guard: only `internal/service` writes it. No new HTTP API. Agent-tool risk: review paste and `t` split — Tasks 7–8 hostile tests.

## Spec coverage

| Spec line | Task |
|---|---|
| Space filter; `● N need you` counts every wait | Task 2 |
| busy→idle → DONE `said ` + `now`; ⏎ → IDLE | Task 3 |
| Chain in NEEDS YOU; nested done not in `✓ N` | Task 3 |
| Busy loop chip; ⚙ `!`; esc same row | Task 4, 6 |
| `t` in worktree; survives quit | Task 7 |
| `D` two comments, one paste, CSI safe | Task 8 |
| `?` over list; `shell` ⏎ = `t` | Task 5, 7 |
| `,` over list; `,` types in AGENT | Task 5, 6 |
| Drag/wheel/copy-mode tmux | Task 1 (existing beside + no vt) |
| Under 100 and Pocket: sidebar | Task 1, 9 |
| First start notice; agent session lives; `ctrl+q` flips | Task 1 |
| DESIGN.md §4 six glyphs | Task 3 |
| Landing, docs, Pocket belt, landing-guard | Task 9 |
| Anti-goal: lose native select/wheel/copy-mode | Task 1 |
| Anti-goal: wait hidden by filter | Task 2 |
| Anti-goal: chain split | Task 3 |
| Anti-goal: leave DONE without keys (except needs-you) | Task 3 |
| Anti-goal: review to the wrong session | Task 8 |
| Anti-goal: diff CSI | Task 8 |
| Anti-goal: bar flash | Task 4 (trouble colour on that chip only) |
| Anti-goal: shell wrong folder | Task 7 |
| Anti-goal: `?`/`,` replace agent; `D` in the list | Task 5, 6, 8 |
| Anti-goal: kill agent as a side effect | Task 1 |
| Surfaces: agent tools, secrets, personal data | Tasks 1, 7, 8, 9 (demo names in www) |

## Calls

- Header/bar/review are `matchblox frame` clients, not extra ttys of one process — because bubbletea is one tty. Cost if wrong: swap the painter, keep the four-pane tests.
- Click on a DONE row gives keys (clears DONE) — because the spec listed a click that gives keys. Cost if wrong: only ⏎ clears DONE.
- Diff too large = 4000 lines or 400 KiB — so the done-when has a number. Cost if wrong: change the constant.
- First-start copy: `desk frame on · the agent pane is still yours`.
- `G` uses the list (header and bar stay). `g` tiles in the agent seat, as today.
- Park by the owner still beats an ask (today's `sidestate.go`). Cost if wrong: a parked ask jumps to NEEDS YOU.
- KeysAt zero: first quiet idle turn is DONE.
- Frame never opens when `onPocket` is true, even at 160 columns. Until the belt handshake, treat as not Pocket.
- `ctrl+q`: send to matchblox when the active pane is the list **or** `frame --kind=review`; else flip to the list. COMMENT `ctrl+q` is the review process. CONFIRM is the list.
- List (or a pane-focus hook) writes `@matchblox_mode` when focus changes, so a tmux flip still updates the bar.
- `SplitEdge` and `SplitIn` go on the `Mux` interface and on `muxtest.Fake`, not only on `Tmux`.
- KeysAt: console sends `keys:<session-id>` when it gives keys; the service is the one writer (all screens).
- SHELL `t` bind: `select-pane` the agent. Do not send `t` to the list.
- `StartBranch` is a field on `sample.Session`, set at Start on that session's host (`git rev-parse --abbrev-ref HEAD` there). Review git runs on that host through the service.
- Pocket belt files: `ios/MatchbloxKit` (`BoardBelt.swift`, tests) and `internal/app/boardkeys.go`.

## Cold read

Cold read: 2026-11-27, 2 fresh reader(s). Blocker 9, major 23, nit 8. Fixed 32, contested 0, for the person 0, declined 8. Second read: 15 left.

Second read (not fixed): wide Pocket vs `onPocket`; `ctrl+q` on REVIEW/COMMENT (agent-seat bind); who writes `@matchblox_mode` after a tmux flip; `SplitEdge` on the Mux interface vs Tmux only; header-gone copy vs open-fail copy; KeysAt wire (`keys:<id>` vs local map); `?` map `N`/`c` vs spec `n`; SHELL `t` bind must select the agent, not send `t` to the list; `StartBranch` type/file; review git on the session host; Pocket belt lives in `ios/MatchbloxKit`; nits on `model.go` file map, paste cap, `design/0022-the-desk.html`; Task 10 done-when is a review verdict.

Calls below take the second-read majors. The person checks them with the plan.
