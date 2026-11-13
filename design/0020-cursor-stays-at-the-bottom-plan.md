# Plan: a builder who comes back to a long Cursor session sees its last screen

```
Spec: design/0019-cursor-stays-at-the-bottom.md
Spec approved: 2026-11-13
Test command: go test ./...
Plan approved: 2026-11-14 by the owner
Next: builder:build
```

## What the code says today

- Cursor starts in three places, all through `launchArgv` and `Service.agentBin`: a new session and a switch (`internal/service/matchcontext.go:149`), and a recover (`internal/service/recover.go:229`).
- The sampler ties a pane to its agent by the process tree under `pane_pid` (`internal/sample/tmux.go:104`, `lineage.go`). An agent under a shell is normal; an agent under `matchblox` is new.
- `cmd/matchblox/main.go:160` already routes hidden subcommands (`hook`) before the flags. The guard goes beside them.
- `golang.org/x/sys` is a dependency. It opens a pty on Linux and macOS; no new dependency is needed.
- `internal/tmuxtest` starts a scratch tmux server for tests.
- Cursor's clear codes are `ESC[H ESC[2J ESC[3J`, written by `src/components/alt-screen.tsx` on mount, on a width change and on its own clear trigger.

## File map

- `internal/guard/filter.go` (create) — turns Cursor's output into what the pane gets: unchanged, or held and cut to its tail.
- `internal/guard/filter_test.go` (create) — the tests of the filter.
- `internal/guard/run_unix.go` (create) — runs a command in a pty and copies keys, output, size and exit status.
- `internal/guard/pty_linux.go`, `internal/guard/pty_darwin.go` (create) — open one pty pair with `x/sys/unix`.
- `internal/guard/run_other.go` (create) — on Windows: says the guard does not run there.
- `internal/guard/run_test.go` (create) — the guard in a pty and in tmux, with a stand-in for Cursor.
- `cmd/matchblox/main.go` (change) — routes `matchblox guard -- <command>` to the guard.
- `internal/service/lifecycle.go` (change) — `guardArgv`: puts the guard in front of Cursor's command.
- `internal/service/matchcontext.go`, `internal/service/recover.go` (change) — call `guardArgv`.
- `internal/sample/lineage_test.go` (change) — a Cursor under the guard is still a Cursor session.
- `design/0019-cursor-stays-at-the-bottom.md` (change) — one line, see the first lens finding.

## Tasks

### Task 1: the issue exists and is Started

- **Files:** none.
- **Do:** open the issue "A long Cursor session replays in full on each width change" in this repository, with the spec's Who and Why. Add it to the project, Status Started, Work on 2026-11-13. This is the standing rule for a caught bug.
- **Done when:** `gh issue view <n>` shows it, and the project item is Started.
- **Commit:** none. Later commits name `(#<n>)`.

### Task 2: outside a replay, every byte passes unchanged

- **Surface:** an agent's tools.
- **Files:** `internal/guard/filter.go`, `internal/guard/filter_test.go`.
- **Test first:** `TestFilterPassesUnchanged`. Input: 1 MiB of random bytes with no clear codes, written in chunks of 1, 7 and 4,096 bytes. Assert: the output equals the input. Hostile input: `TestFilterHalfCodes` writes `ESC[H ESC[2J` with no `ESC[3J`, then text; and the full codes split over three writes. Assert: the first comes out unchanged; the second starts a hold.
- **Run:** `go test ./internal/guard -run TestFilter`. Before: does not compile. After: passes.
- **Do:** `type Filter`; `NewFilter(out io.Writer, clock)`; `Write(p []byte)`. A matcher keeps at most 9 bytes back while they can still be the start of the codes.
- **Done when:** both tests pass.
- **Commit:** `feat: a guard passes an agent's output unchanged`

### Task 3: a replay becomes one line, then the end of the chat

- **Files:** `internal/guard/filter.go`, `internal/guard/filter_test.go`.
- **Test first:** `TestFilterKeepsTail`. Input: the clear codes, then 50,000 lines `line N`, then a 3-line input box, with a fake clock. Assert, in order: the output holds the codes and `Cursor redraws for the new width…`; then, after the burst ends, the codes again, an SGR reset, the last 1,000 lines and the box; `line 1` is not in the output. `TestFilterTrickle`: the codes, then 200 bytes each 100 ms. Assert: the hold ends within 250 ms and later bytes pass as they come. `TestFilterCap`: a burst that never slows. Assert: at 60 s the tail is written and the rest passes.
- **Run:** `go test ./internal/guard -run TestFilter`. Before: the three fail (no hold). After: pass.
- **Do:** on the codes: write them and the line, then hold. While held, keep the last 1,000 whole lines, at most 4 MiB. The hold ends when a 250 ms window brings under 16 KiB, or at 60 s. Then write the codes, `ESC[0m` and the kept lines.
- **Done when:** the three tests pass.
- **Commit:** `feat: the guard shows the end of a replay, not the replay`

### Task 4: the guard runs a command in a terminal of its own

- **Surface:** an agent's tools.
- **Files:** `internal/guard/run_unix.go`, `pty_linux.go`, `pty_darwin.go`, `run_other.go`, `internal/guard/run_test.go`.
- **Test first:** `TestRunCopies`: the guard runs `cat` inside a test pty. Typed `héllo\x1b[200~x\x1b[201~\r` comes back the same. `TestRunSize`: the child prints `stty size`; after the outer pty goes from 100x30 to 80x30, the child reads `30 80`. `TestRunStatus`: the child exits 3; the guard returns 3. Hostile input: `TestRunKeysDuringHold`: while the filter holds, typed bytes still reach the child at once.
- **Run:** `go test ./internal/guard -run TestRun`. Before: does not compile. After: passes on Linux and macOS.
- **Do:** `Run(argv []string, in, out *os.File) (status int, err error)`: open a pty at the outer size; start argv on it as its controlling terminal; set the outer terminal raw and put it back at the end; copy keys in; copy output through the Filter; on SIGWINCH copy the size; pass HUP and TERM to the child.
- **Done when:** the four tests pass.
- **Commit:** `feat: the guard runs an agent in a terminal of its own`

### Task 5: `matchblox guard -- <command>`, and the words when it fails

- **Files:** `cmd/matchblox/main.go`, `internal/guard/run_unix.go`, `internal/guard/run_test.go`.
- **Test first:** `TestGuardCannotStart`: the pty opener fails with "no ptys left". Assert: the pane gets `Cursor runs without the redraw guard: no ptys left`, and the command still runs, directly. `TestGuardStops`: the copy loop panics. Assert: the terminal is put back, and the pane gets `matchblox's redraw guard stopped: <reason>. Start again: matchblox switch <name> --agent cursor-agent`. `TestGuardNoCommand`: `matchblox guard` alone says `guard needs a command: matchblox guard -- <command>`.
- **Run:** `go test ./internal/guard ./cmd/matchblox -run 'TestGuard'`. Before: fail. After: pass.
- **Do:** route `guard` beside `hook` in `run`. `<name>` comes from `MATCHBLOX_SESSION`, which `guardArgv` sets (Task 8).
- **Done when:** the three tests pass, and `go test ./cmd/matchblox` passes (the docs check knows the command or leaves a hidden one out).
- **Commit:** `feat: matchblox guard runs a command behind the redraw guard`

### Task 6: in tmux, a width change shows the end and never the chat

- **Files:** `internal/guard/run_test.go`.
- **Test first:** `TestGuardInTmux`, on `tmuxtest`. A stand-in for Cursor (the test binary, started again in a helper mode): it prints the codes, 20,000 lines and a 3-line box at its start and at each change of columns, and nothing at a change of rows. Two panes run it: one direct, one behind the guard. Steps and asserts:
  1. 216 to 180 columns and back. The guarded pane's piped output is under 200 KiB for each change; the direct pane's is over 1 MiB.
  2. After each change, `capture-pane` of the guarded pane equals that of the direct pane: the same last screen.
  3. `line 1` never appears in the guarded pane's piped output.
  4. 71 to 70 rows: the guarded pane's piped output grows by 0 bytes.
  5. `abc` typed during the replay is in the stand-in's input line when it ends.
- **Run:** `go test ./internal/guard -run TestGuardInTmux`. Before: fails at step 1 only if Tasks 2 to 5 are wrong; write it before any fix it calls for. After: passes.
- **Done when:** the test passes three times in a row.
- **Commit:** `test: behind the guard, a width change shows the last screen only`

### Task 7: nothing of the chat lands on disk

- **Surface:** secrets.
- **Files:** `internal/guard/run_test.go`.
- **Test first:** `TestGuardWritesNoFile`. Hostile input: the stand-in prints `tok-9f3c-SECRET` inside a replay and outside one, and the test types `pw-77aa-SECRET`. `HOME` and the state folders point at a temp folder (`internal/testhome`). Assert: after the guard ends, no file under the temp folder holds either string, and the guard's stderr holds neither.
- **Run:** `go test ./internal/guard -run TestGuardWritesNoFile`. After: passes.
- **Do:** nothing new if the test passes. The guard opens no file and logs no content.
- **Done when:** the test passes.
- **Commit:** `test: the guard keeps what passes through it in memory only`

### Task 8: the console still knows a guarded Cursor

- **Files:** `internal/sample/lineage_test.go`.
- **Test first:** `TestCursorUnderGuard`. A fake process table: the pane's process is `matchblox guard -- cursor-agent --resume x`, its child is `cursor-agent --resume x`. Assert: one session, agent `cursor-agent`, on that pane, and `matchblox` is not a session.
- **Run:** `go test ./internal/sample -run TestCursorUnderGuard`. Before: unknown; if it fails, the change goes in `lineage.go` and joins the file map. After: passes.
- **Done when:** the test passes, and `go test ./internal/sample ./internal/queue` passes.
- **Commit:** `test: a Cursor behind the guard is still a Cursor session`

### Task 9: matchblox starts Cursor behind the guard

- **Files:** `internal/service/lifecycle.go`, `matchcontext.go`, `recover.go`, their tests.
- **Test first:** `TestGuardArgv`. For `cursor-agent` on linux and darwin: argv becomes `env MATCHBLOX_SESSION=<name> <self> guard -- cursor-agent …`. For claude, codex, opencode: unchanged. For windows, or when this binary's path is unknown: unchanged. The existing new, switch and recover tests assert the new argv for Cursor.
- **Run:** `go test ./internal/service`. Before: `TestGuardArgv` does not compile. After: all pass.
- **Do:** `guardArgv(self, goos, agent, name string, argv []string) []string`; call it after `launchArgv` in `matchcontext.go` and in `recover.go`.
- **Done when:** `go test ./...` passes.
- **Commit:** `feat: a long Cursor session stays at the bottom when its width changes (#<n>)`

### Task 10: measured on a real Cursor, and the landing page has its words

- **Files:** none in the repository.
- **Do:**
  1. On a host with a signed-in Cursor, resume a long chat in a scratch tmux server, direct and behind the guard. Change the width by 36 columns, in and out. Record seconds and bytes for both. Type a line during the replay.
  2. Check that the console shows the guarded session as Cursor, with state and context.
  3. Send matchbloxrelease the words for `www/`: "A long Cursor session stays at the bottom when your screen changes width." with the two measured times.
  4. Run `.github/scripts/landing-guard.sh "<title>" <body-file> origin/main`.
- **Done when:** the report holds both times and both byte counts; the guarded pane shows the end of the chat within 5 seconds; the landing guard passes.
- **Commit:** none.

### Task 11: the review

- **Do:** load `builder:review` on the whole change, with `design/0019-cursor-stays-at-the-bottom.md` and `go test ./...`.
- **Done when:** no open Critical or Important finding, and no open check on the two surfaces.

## Lens findings

Engineering:

- Scope: the spec says a guard that cannot start shows its words in "the session's row". The row needs a new message from the guard to the service, a second new part. Changed: the words go in the pane, above Cursor's first screen (Task 5). The spec's line changes to match; approving this plan approves that line.
- Scope: 10 files and one new part (the guard). Over the eight-file smell by the two pty files and the Windows stub. No change, because one pty library would be a new dependency for 60 lines.
- Reuse: no pty code exists here; `x/sys/unix` is the nearest. `tmuxtest` and `testhome` are reused.
- Architecture: the guard owns no data. Flow: keys → pty → Cursor; Cursor → pty → Filter → pane. Production failures: the host has no ptys left (Task 5); the guard dies (Task 5); Cursor changes its clear codes (then every byte passes: today's behaviour, Task 2); a replay pauses for over 250 ms (the tail prints, the rest scrolls: Task 3 fixes the rule, Task 10 measures if it happens).
- Tests: a hold started by Cursor's own clear, not a resize, was a gap. Closed: `TestFilterTrickle` in Task 3.
- Regression: the sampler (Task 8, before the launch changes), the launch argv of the other agents (Task 9), the docs check of commands (Task 5).
- Performance: one pass over each byte; at most 1,000 lines or 4 MiB kept. A replay's size is unknown; Task 10 measures it.

Design:

- The line `Cursor redraws for the new width…`: 7. It does not say how long. A 10 counts. No change, because the guard cannot know the chat's length; Task 10's time decides if a counter is worth a second pass.
- `Cursor runs without the redraw guard: <reason>`: 8. It says what happens; nothing is broken, so no fix is asked.
- `matchblox's redraw guard stopped: …. Start again: matchblox switch <name> --agent cursor-agent`: 9. It names the fix.
- A session that resumes a long chat at its start also holds: the builder lands at the bottom there too. Kept; it is what "always" asks for.

Architecture lens: the guard is the terminal of an agent. It adds no command to the agent, reads no key, and changes no byte outside a replay (Tasks 2 and 4).

## Spec lines and their tasks

| Spec line | Task |
|---|---|
| The pane shows the end of the chat within 5 seconds, and the chat never scrolls by | Task 6: in tmux, a width change shows the end; Task 10 on a real Cursor |
| The same visit without the guard, measured beside it | Task 10: measured on a real Cursor |
| A change of rows alone prints nothing new through the guard | Task 6, step 4 |
| A line typed during a replay is in Cursor's input box | Task 4: `TestRunKeysDuringHold`; Task 6, step 5 |
| The console shows the session as Cursor | Task 8: the console still knows a guarded Cursor; Task 10, step 2 |
| No file under matchblox's state folder holds a line of the chat | Task 7: nothing of the chat lands on disk |
| Anti-goal: the pane shows a screen that is not Cursor's | Task 6, step 2 |
| Anti-goal: a key does not reach Cursor, or late | Task 4 |
| Anti-goal: matchblox no longer knows the session is Cursor | Task 8 |
| Anti-goal: what passes through lands in a file or a log | Task 7 |
| Anti-goal: the pane looks frozen | Task 3: the line is written before the hold |
| Goes wrong: the wrapper cannot start; the wrapper ends | Task 5 |
| Goes wrong: a replay never goes quiet for 60 seconds | Task 3: `TestFilterCap` |
| Call: how long a replay takes when nothing draws it | Task 10 |
| Call: the scrollback keeps the last 1,000 lines | Task 3 |
| Call: the report to Cursor | Not in this plan. It waits for the owner's yes. |

## Changes during build

- 2026-11-14, Task 1: the issue is #399. Work on is 2026-11-14, the day the build began.
- 2026-11-14, Task 2: `Filter` gets `Tick()` and `Holding()`, and `NewFilter` takes `now func() time.Time`. Reason: output that ends on a start of the clear codes (a lone `ESC[H`) must pass when no more output comes, and a `Write` alone cannot see time pass. `Run` calls `Tick` on a timer (Task 4). The tests assert it: a held-back `ESC[H` comes out after the wait.
- 2026-11-14, Task 3: one more test, `TestFilterShortReplay`. Reason: a replay shorter than the tail lost its first blank line; the test fails with the first trim rule.
- 2026-11-14, Task 4: `Filter` gets `Flush()`, which `Run` calls when the agent ends. Reason: an agent that ends during a replay must still leave its last screen. The runner's files build on Linux and macOS only (`linux || darwin`), not on every Unix: the pty code exists for those two.
- 2026-11-14, Task 9 and the review: the guard runs under its own name, a link `matchblox-guard` to the binary beside the state file (`Service.guardLink`). Reason: tmux names a pane after its program, and the way back takes a pane named `matchblox` for a console; the home key was typed into Cursor. `guardArgv` takes the link, not the binary.
- 2026-11-14, the review: a process on an agent's own terminal is placed in that agent's pane (`paneOfTerminal`). Reason: behind the guard, Cursor's terminal is no tmux pane, and a process it left behind passed for the person.
- 2026-11-14, the review, Task 3's rule replaced: a clear starts a hold only within 3 s of the agent's start or of a width change the guard passed on (`Filter.Arm`), and a hold ends after 300 ms with no output, or at 60 s. Reason: the first rule (under 16 KiB in 250 ms) let a replay slower than 64 KiB/s scroll by whole. Cursor's own clear, with no width change, now passes unchanged and shows no notice.
- 2026-11-14, the review: during a hold, what Cursor asks of the terminal or sets in it (private modes, the keyboard protocol, status, attribute and colour queries) passes at once and stays out of the kept lines. Reason: a query held back was answered late, and a mode set before the last 1,000 lines was never set.
- 2026-11-14, the review: 3 s after a hangup or a stop that the guard passed on, it takes the agent's terminal away and ends it. Reason: an agent that ignores the hangup lived on behind its guard after its pane went.
- 2026-11-14, the review, second round: a hold also ends when 300 ms bring under 10 new lines; a switch to or from the alternate screen ends the hold first; an OSC passes at once only when its last parameter is the question mark; two launches at once both get the guard's link; a two-byte escape in a replay no longer breaks the guard. Reasons: a resize under a streamed answer showed only the notice for 60 s; a full-screen view opened during a hold got the chat drawn into it; a link whose address holds `;?` left its place; one of two launches lost the link; the guard broke and ended Cursor.
- 2026-11-14, left open on purpose, both Minor: a process a guarded Cursor leaves behind passes for the person until the first sample lists that session; an agent that needs over 3 s after a hangup is ended before it finishes.
