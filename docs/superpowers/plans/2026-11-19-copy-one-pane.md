Spec: docs/superpowers/specs/2026-11-18-copy-one-pane-design.md
Spec approved: 2026-11-19
Test command: go test -race ./... && go vet ./... && golangci-lint run
Plan approved: 2026-11-19 by the owner
Next: builder:build

# Plan: the owner drags over an agent's words and pastes exactly those words, on any screen

Design (spec R7, R3 reopened, R4): tmux owns every drag and makes a buffer; each matchblox service watches its own tmux's buffers in control mode and sends each new copy to its consoles; the console the person is using loads it to its own tmux client with `load-buffer -w`. No `set-clipboard` change, no copy-command, no passthrough.

What the code has today:
- `paneMouse` (internal/remote/jump.go:131) is the one bind list for a jump, the beside view (`ViewArgv`, view.go:63) and the peer view (`PeerViewArgv`, used at internal/app/beside.go:346). Its test line is `mouseIn` (jump_test.go:10).
- The console reads each conn in one loop (internal/app/model.go:~700, `fromConn{c, …}`) and ignores kinds it does not know: an older console skips a new kind.
- The service sends a console its pane screen as `proto.KindScreen` (internal/service/watch.go:153); `s.clients` (service.go:276) holds the console conns; loops start at service.go:604.
- `clip` (internal/app/copy.go:35) writes OSC 52 through `tea.SetClipboard`; `tmuxPassesClipboard` and the `tmuxDrops` notice (copy.go:19, :46) and the door `CopyThrough` (internal/doors/doors.go:162, :181) exist only to turn `set-clipboard on`.
- Private tmux servers for tests: `tmuxtest.Server(t)` (internal/tmuxtest/tmuxtest.go:21).
- Measured on tmux 3.4: `load-buffer -w -t <client>` reaches the terminal under `external`; `tmux -C` prints `%paste-buffer-changed <name>`; copy mode trims trailing spaces and joins soft wraps.

## File map

- `internal/remote/jump.go` (change) — `paneMouse`: press held back, whole click on release, every drag to copy mode, double/triple-click select.
- `internal/remote/jump_test.go` (change) — `mouseIn` and the bind rows.
- `internal/remote/mouse_tmux_test.go` (create) — a mouse recorder app in a private tmux proves what the agent receives.
- `internal/proto/proto.go` (change) — `KindCopy` and `Copy{Text}`.
- `internal/service/copywatch.go` (create) — a control-mode client of the service's tmux that turns each new buffer into a copy for the consoles.
- `internal/service/copywatch_test.go` (create) — its tests on a private tmux.
- `internal/service/service.go` (change) — starts the watcher with the other loops; sends each copy to console clients only.
- `internal/app/copy.go` (change) — `deliver`: the console loads a copy to its own client; `clip` goes through it; the set-clipboard words go.
- `internal/app/model.go` (change) — `KindCopy` in the conn reader becomes `copyMsg`.
- `internal/app/copy_test.go` (change) — delivery and `c` tests.
- `internal/doors/doors.go`, `internal/doors/doors_test.go` (change) — the door "Let copy through" goes.
- `docs/console.md`, `CHANGELOG.md`, `www/` (change) — the "Copy" section, the changelog line, the landing page line.

Thirteen files and two new parts (the watcher, one message kind). The count is over the lens's eight: each file holds one job the spec names, and the doors and docs files only lose text.

## Task 1: a click reaches a mouse-reading agent whole, when the button comes up

- **Files:** internal/remote/jump.go, internal/remote/jump_test.go, internal/remote/mouse_tmux_test.go
- **Test first:** `TestAClickReachesTheAgentWholeOnRelease` in mouse_tmux_test.go. A private tmux (`tmuxtest.Server`) runs a jump session built from `Jump.Remote()` lines whose pane runs a recorder: a `sh`/`python3 -I` script that turns on SGR mouse (`\e[?1000h\e[?1006h`) and appends each mouse report to a file. The test sends a press and a release at cell (5,3) to the client with `send-keys -M`-free means: it attaches a client through `script(1)` and writes SGR mouse bytes to that client's terminal. Assert the recorder saw exactly `\e[<0;5;3M` then `\e[<0;5;3m`, and nothing on the press alone. A second case turns on the normal encoding (`\e[?1000h` only) and asserts the normal-encoding pair. A third case: a pane that reads no mouse gets no bytes.
- **Run:** `go test -run TestAClickReachesTheAgentWholeOnRelease ./internal/remote/` — before: fails, the recorder sees the press at once and the release from tmux. After: passes.
- **Do:** in `paneMouse`, `MouseDown1Pane` becomes `select-pane -t =`, and also `send-keys -M` only when `#{pane_in_mode}` (a click clears a copy-mode selection). New `MouseUp1Pane`: when `#{&&:#{mouse_any_flag},#{!:#{pane_in_mode}}}`, run `run-shell` with a fixed `printf` that writes press and release for `#{mouse_x}+1`, `#{mouse_y}+1`, in SGR when `#{mouse_sgr_flag}` is 1, else the normal encoding, and passes it to `send-keys -t = -l`. Update `mouseIn` to the new line.
- **Done when:** `TestAClickReachesTheAgentWholeOnRelease` and `TestJumpRemoteToPane` pass.
- **Commit:** `fix: a click reaches the agent whole, when the button comes up`

## Task 2: a drag, a double-click and a triple-click select in tmux and make one buffer

- **Files:** internal/remote/jump.go, internal/remote/jump_test.go, internal/remote/mouse_tmux_test.go
- **Test first:** `TestADragSelectsInTmuxAndTheAgentGetsNothing`. Same rig; the recorder pane prints `abc     ` then a line longer than the pane. Drag from the start of `abc` to the end of the wrapped line. Assert: the recorder saw no mouse bytes; `show-buffer` is `abc\n<the long line as one line>` (no trailing spaces). `TestADoubleClickCopiesAWord`: double-click on `abc` → buffer `abc`. `TestATripleClickCopiesALine`: triple-click → the whole line, trimmed. `TestTheWheelStillReachesTheAgent`: a wheel-up reaches the recorder as `\e[<64;…M`.
- **Run:** `go test -run 'TestADrag|TestADoubleClick|TestATripleClick|TestTheWheel' ./internal/remote/` — before: the drag goes to the recorder (mouse_any_flag) and double/triple do nothing. After: passes.
- **Do:** `MouseDrag1Pane` becomes `if -F '#{pane_in_mode}' 'send-keys -M' 'copy-mode -M'`. Add `DoubleClick1Pane` and `TripleClick1Pane`: `select-pane -t = ; if -F '#{pane_in_mode}' '' 'copy-mode -H' ; send-keys -X select-word` (`select-line`), `run-shell -d 0.3`, `send-keys -X copy-pipe-and-cancel`, as tmux's own root defaults do. Wheel binds stay.
- **Done when:** the four tests pass.
- **Commit:** `fix: a drag, a double-click and a triple-click copy in tmux in every agent`

## Task 3: the copy message kind, which older consoles skip

- **Files:** internal/proto/proto.go, internal/app/model.go, internal/app/copy_test.go
- **Test first:** `TestACopyMessageBecomesACopyMsg`: a test conn sends `{kind:"copy", body:{"text":"x"}}` then a snapshot; the reader returns `fromConn{c, copyMsg{Text:"x"}}` and then the snapshot. `TestAnUnknownKindIsSkipped` stays green (an older console sees `copy` as unknown).
- **Run:** `go test -run TestACopyMessage ./internal/app/` — before: does not compile (no `KindCopy`). After: passes.
- **Do:** add `KindCopy Kind = "copy"` with a doc line, and `type Copy struct{ Text string }`. In the conn reader, `case proto.KindCopy` unmarshals into `copyMsg`.
- **Done when:** both tests pass.
- **Commit:** `feat: a copy message carries a tmux copy to the console`

## Task 4: the service turns each new tmux buffer into a copy for its consoles

- **Files:** internal/service/copywatch.go, internal/service/copywatch_test.go, internal/service/service.go
- **Test first:** in copywatch_test.go, on `tmuxtest.Server`:
  - `TestANewBufferGoesToEveryConsole`: a service with one console client and one CLI client; `tmux set-buffer -b buffer0 hello`; the console gets `KindCopy{Text:"hello"}` within 2 s; the CLI client gets none.
  - `TestTheConsolesOwnBufferIsNotSentBack`: `set-buffer -b matchblox-copy x` sends nothing (loop guard).
  - `TestABigCopyIsCut`: a 2 MiB buffer sends no copy, and the console gets an event `not copied: <n> MB is over the 1 MB copy limit`.
  - `TestTheWatcherComesBack`: kill the watcher's control client (`kill-session` of its session); a new buffer still arrives within 5 s.
  - `TestTheWatcherLeavesWindowSizesAlone`: with the watcher attached, a 100x30 session keeps 100x30.
  - `TestACopyWithNoConsoleSaysSo`: no console client connected; a new buffer makes `display-message` `not copied: no matchblox console is connected · open the console` on the tmux server.
- **Run:** `go test -run 'TestANewBuffer|TestTheConsoles|TestABigCopy|TestTheWatcher' ./internal/service/` — before: no watcher, the tests time out. After: pass.
- **Do:** `copyWatch(ctx)` starts `tmux -C attach -f no-output,ignore-size,read-only -t =matchblox-copywatch` on the service's socket, after `new-session -d -s matchblox-copywatch` (a hidden session of its own). It reads lines; on `%paste-buffer-changed <name>` with a name that does not start with `matchblox-`, it runs `show-buffer -b <name>`, drops a text over 1 MiB with the event above, and calls `s.sendCopy(text)`, which sends `KindCopy` to each client whose hello said `ClientConsole`. On EOF it waits 1 s and starts again while ctx lives. Start it in the loop list at service.go:604.
- **Done when:** the five tests pass.
- **Commit:** `feat: the service sends each tmux copy to its consoles`

## Task 5: the console puts a copy on the screen the person is using

- **Files:** internal/app/copy.go, internal/app/copy_test.go
- **Test first:** with a fake `Tmux` runner:
  - `TestACopyGoesToTheConsolesOwnClient`: `list-clients` answers two clients of the console's session, one with `client_activity` 2 s ago and one 5 min ago, plus a nested view client whose tty is a pane tty. `copyMsg{"hi"}` runs exactly `load-buffer -w -b matchblox-copy -t <the 2 s client> -` with stdin `hi`, and the header says `copied 2 characters`.
  - `TestNoRecentInputMeansNoCopy`: every client idle 30 s: no `load-buffer`, nothing said (the person is at another screen).
  - `TestAFailedLoadSaysWhy`: `load-buffer` fails with `no current client`: the header says `not copied: <tmux's words>`, never `copied`.
  - Outside tmux (`OutsideTmux`): `copyMsg` writes `tea.SetClipboard` (a terminal with no tmux).
  - `TestAHostCopySaysWhereFrom`: a `copyMsg` from a peer conn of host `ws-1` says `copied 2 characters from ws-1`.
- **Run:** `go test -run 'TestACopyGoes|TestNoRecentInput|TestAFailedLoad' ./internal/app/` — before: no `copyMsg` handling. After: pass.
- **Do:** `deliver(text)`: read `display -p -t <console pane> '#{session_name}'`, then `list-clients -t <session> -F '#{client_name} #{client_activity} #{client_tty}'` and `list-panes -a -F '#{pane_tty}'`; drop clients whose tty is a pane tty; take the one with the newest activity within 10 s; run `load-buffer -w -b matchblox-copy -t <client> -`. `Update` handles `copyMsg` with `deliver`.
- **Done when:** the four tests pass.
- **Commit:** `feat: the console puts a tmux copy on the screen you use`

## Task 6: a copy on a host reaches the desk over matchblox's link

- **Files:** internal/service/copywatch_test.go
- **Test first:** `TestAHostCopyReachesTheDeskConsole`: two private tmux servers (desk, host). A host service runs on the host server; a console model connects to it through `transport` over an in-memory pipe (as a peer conn), and its own `Tmux` runs on the desk server with a client attached through `script(1)`. `set-buffer` on the host → the desk client's terminal log holds one OSC 52 with the text; `show -sv set-clipboard` on both servers is unchanged; no pane on either has `allow-passthrough` on.
- **Run:** `go test -run TestAHostCopyReachesTheDeskConsole ./internal/service/` — before Task 4/5: fails. After: passes.
- **Do:** no new product code if Tasks 3-5 hold; a fix found here goes into the owning task's file.
- **Done when:** the test passes.
- **Commit:** `test: a host copy reaches the desk over matchblox's link`

## Task 7: `c` copies through the same path, and nothing turns set-clipboard on

- **Files:** internal/app/copy.go, internal/app/copy_test.go, internal/doors/doors.go, internal/doors/doors_test.go
- **Test first:**
  - `TestCCopiesThroughTheConsolesClient`: `c` on a row runs `load-buffer -w -b matchblox-copy -t <client> -` with the last message, not OSC 52.
  - `TestNoDoorTurnsSetClipboardOn`: the doors list has no `copy-through`; no door's argv contains `set-clipboard`.
  - `TestAConsoleStartLeavesSetClipboard` (private tmux, `external`): start the console model and a jump; `show -sv set-clipboard` is still `external`; a pane's `\e]52;c;?\a` gets no reply in 1.5 s (a raw-mode reader, as the review's probe).
- **Run:** `go test -run 'TestCCopies|TestNoDoorTurns|TestAConsoleStartLeaves' ./internal/app/ ./internal/doors/` — before: `c` writes OSC 52; the door exists. After: pass.
- **Do:** `clip` calls `deliver` inside tmux and `tea.SetClipboard` outside it; `m.opt.Clip` stays first. Delete `tmuxDrops`, `tmuxPassesClipboard`, `readClip`, `clipOK/clipKnown` and their callers; delete `CopyThrough`, `CopyThroughDoor`, `copyThroughArgv` and its allowlist line. The words `copied: <what> · not checked here` and "Check that copy reaches you" stay.
- **Done when:** the three tests pass, and `rg -n "set-clipboard" internal cmd` finds no code that sets it.
- **Commit:** `fix: copy never turns tmux's set-clipboard on`

## Task 8: the docs, the changelog and the landing page say how copy works

- **Files:** docs/console.md, CHANGELOG.md, www/ (the copy line)
- **Test first:** `go test ./internal/docscheck/...` (the docs check) and `.github/scripts/landing-guard.sh "feat: drag to copy exactly the agent's words, on any screen" <body> origin/main` pass.
- **Run:** the two commands — before: docs/console.md "Copy" names `set-clipboard on` and the door. After: pass.
- **Do:** rewrite "Copy": a drag, a double-click and a triple-click copy in tmux; the copy reaches the screen you use, also over SSH; tmux's `set-clipboard` stays yours; an agent's own copy command (Claude Code /copy) does not reach you; Shift-drag is your terminal's selection, press ctrl+q first. Changelog headline of 8 words or fewer. A landing page line, written in this branch.
- **Done when:** both checks pass.
- **Commit:** `docs: copy reaches the screen you use, with no tmux setting`

## Task 9: live check in the four agents, desk and SSH

- **Files:** none (results go in the PR body)
- **Test first:** none: this proves what no test can see.
- **Run:** a dev build on this host: `go build -o "$TMPDIR/mb" ./cmd/matchblox`, then the console in beside mode.
- **Do:** in Claude Code, codex, opencode and cursor-agent: drag three lines and paste into another app; double-click a path; click a menu item after a drag; scroll with the wheel. Then the same over `matchblox <host>` from the desk. Record each result, and each failure as a finding.
- **Done when:** the PR body holds the 4×2 table of results.
- **Commit:** none.

## Task 10: review the whole change

- Load `builder:review` with the spec docs/superpowers/specs/2026-11-18-copy-one-pane-design.md and the test command `go test -race ./... && go vet ./... && golangci-lint run`.
- **Done when:** no open Critical or Important finding.

## Lens findings

Engineering:
- Scope: tmux copy mode, `load-buffer -w`, the proto link and `paneMouse` already solve the selector, the writer, the carrier and the binds (reused). New parts: the watcher and one message kind; nothing else is new. No new dependency.
- Architecture: the service owns "a copy happened" (it owns the tmux server); the console owns "which screen gets it" (it knows its own client). Data flow: tmux buffer → watcher (cap 1 MiB) → `KindCopy` → console → `load-buffer -w`. Production failures: the watcher's client dies (Task 4 restart test); the desk link is down when a host copies (the copy is lost; Task 4 says it on the host only if no console is connected: added as the event "not copied: no console is connected" to Task 4); two desks attached (Task 5 picks the newest input within 10 s); a person copies on the phone while the desk is idle (Task 5: idle desk gets nothing). Changed Task 4: added the no-console event.
- Tests: map below; no gap left.
- Regression: clicks and the wheel in agents (Tasks 1-2 tests); the console's `c` (Task 7); `TestJumpRemoteToPane` and `TestJumpTablePagesInCopyMode` keep the jump list.
- Performance: one control-mode client per service, idle; one `show-buffer` and one message per copy; one `list-clients` per delivery. Scale: a person's copies, a few a minute (estimated).

Design (what the person sees):
- Success: the console header `copied 42 characters` (existing words): 8. A 10 names where it came from when it is a host's: changed Task 5 to say `copied 42 characters from <host>` for a peer conn.
- Error: `not copied: <tmux's words>`: 7, tmux's words can be cryptic. No change, because the existing "Check that copy reaches you" door names the hop, and the spec's docs table gives the fix.
- Empty: no recent input → nothing said: 9; saying nothing is right when the person is at another screen.
- One actor, one job: the person copying; no screen gains a second job.

Architecture lens:
- 1, 7 (contract written next to code, described in words): `KindCopy` gets a doc line in proto.go like its neighbours: the project's rule.
- Version skew (no default answer in the lens): an older console skips the unknown kind (model.go reader has no default case), and an older service never sends it. No change, because nothing breaks; the newer side simply gets no copies from the older one.
- Same write twice: two buffers in a row are two copies; the clipboard keeps the last. No change, because that is what a person expects.

## Spec check

| Spec line | Task |
|---|---|
| Done: a drag over three lines of Claude Code pastes as those lines, no sidebar, no `│`, no trailing spaces | Task 2; Task 9 |
| Done: the same drag works in codex and cursor-agent | Task 2; Task 9 |
| Done: over SSH from the desk the drag pastes on the desk | Task 6; Task 9 |
| Done: set-clipboard unchanged after a console start and a jump; an OSC 52 read gets no reply | Task 7 |
| Done: a click on a Claude Code menu item and the wheel still work | Task 1; Task 2; Task 9 |
| Done: a drag in opencode pastes the selected text | Task 9 (opencode's drawn borders: spec U1, open) |
| Done: a long link opens whole on a cmd-click | Task 9 (records the result; OSC 8 pass-through is tmux's own, no code) |
| Done: "Check that copy reaches you" passes on the desk, local and over SSH | Task 9 |
| Anti-goal: a paste holds sidebar text, a border or trailing spaces | Task 2 |
| Anti-goal: a drag in one pane selects another pane | Task 2 (copy mode is per pane) |
| Anti-goal: the console says `copied` when a hop dropped it | Task 5 (`TestAFailedLoadSaysWhy`) |
| Anti-goal: a click or the wheel no longer reaches the agent | Tasks 1, 2 |
| Anti-goal: set-clipboard lets a program read the clipboard | Task 7 |
| Surface: an agent's tools — no pane or host reads or writes the clipboard by escape codes | Tasks 6, 7 (hostile input: the OSC 52 read query) |

## Calls

- The plan lives at docs/superpowers/plans/2026-11-19-copy-one-pane.md, next to the earlier plans.
- Spec U1 (self-drawn sidebars in opencode) and U2 (an opt-in for agents' own copy commands) stay open and out of this plan, as the approval said.
- One PR, a `feat` (the landing page changes in Task 8).

## Changes during build

- Task 1: the plan put the new binds in `paneMouse` alone, but `paneMouse` also binds the desk side of a host jump (jump.go, the host jump list), where the pane runs ssh to a nested tmux. Holding the press there starves the host's tmux of it: `TestJumpSwipeScrolls` failed ("a drag selects in the agent's pane" timed out). Change: the old binds move to `relayMouse` for the desk side; `paneMouse` (the agent's own pane: Remote, so jumps and views) gets the new binds. Test file: `mouse_tmux_test.go` reuses the existing jump rig (`newJumpEnv`, a real desk server, host server and fake ssh), Linux only like that rig. Decided in build (a correction inside Task 1's scope, no spec change).
- Task 3: the plan said `TestAnUnknownKindIsSkipped` "stays green", but no such test existed. Added it (internal/app/copymsg_test.go) to guard the version-skew claim. The Task 3 tests live in copymsg_test.go, not copy_test.go.
- Task 5: the console's tmux runner takes no stdin, so `load-buffer … -` became `load-buffer -w -b matchblox-copy -t <client> <file>` with a private temp file (also no argument size limit). The notice keeps the existing words, `copied: 2 characters` (with ` from <host>` for a peer). A console with its own Clip (Pocket) takes no incoming copies (`TestAPhoneTakesNoCopies`): a desk copy must not land on the phone. Security review of Task 4: a client in an agent's pane gets no copies, by the screen rule `aConsole` (`TestAnAgentThatSaysConsoleGetsNoCopies`).
- Security review of Task 5 (owner ruling 2026-11-20, spec Rulings): a peer's copy goes only to a screen that shows that host (beside view, chain view or a jump session of that host) with input in the last 2 s; a local copy to any of the person's screens with input in the last 10 s (a client in a local agent's session is still the person). The service strips control codes and checks `aConsole` at each copy. Tests: TestAHostCopyNeedsItsHostOnScreen, TestAHostCopyDuringAJump, TestACopyCarriesNoControlCodes, TestAnAgentPaneFoundLaterGetsNoCopies. The two agent-pane tests use pane %77: the test fixture already has a session at %1.
- Task 6: the service's test constructor is private, so one Go test cannot run a host service and a console Model. The chain is proven in two halves: Task 4's tests (a real tmux copy reaches a console conn) and `TestAHostCopyReachesTheDeskTerminal` in internal/app/copy_tmux_test.go (a host's copy on the conn reaches a real desk terminal through `load-buffer -w`, set-clipboard external, no passthrough; it fails without `-w`). The whole chain runs live in Task 9. script(1) runs `$SHELL -c`, and zsh expands `=matchblox`: the test quotes it and sets SHELL=/bin/sh.
- Task 7: once the door is gone, no code path in the console can set set-clipboard, so the third test is a repository guard instead of a console start on a private tmux: `TestNothingSetsSetClipboard` (internal/docscheck/clipboard_test.go) fails on any string literal naming set-clipboard in product Go code, and fails when it reads fewer than 50 files. `TestAHostCopyReachesTheDeskTerminal` already checks set-clipboard and passthrough after a real copy. `clip` now returns an error: `c`, the copy check, the login link and the pair link say `not copied: <why>` instead of `copied`, proven by `TestCNeverClaimsACopyThatFailed`. Retired: TestTmuxPassesClipboard, TestCopyNeverClaimsWhatTmuxDrops (copyThroughTmux), TestCopyThroughRunsOnlyTheTmuxSetting.
- Review (2026-11-20), two Important findings fixed: (1) the control-mode client needed a session of its own, and a plain `tmux attach` landed in it (probe); the watcher now reads `list-buffers` every 300 ms instead (no session, no client, no server started: TestTheWatcherLeavesNoSession, TestTheWatcherWaitsForAServer). (2) "not copied: no matchblox console is connected" was false: tmux's own copy reaches a directly attached terminal (probe); no notice now (TestACopyWithNoConsoleSaysNothing), and the size notice says `not carried: … over matchblox's 1 MB copy limit`. R7's mechanism changes from control mode to a buffer read; what R7 approved (no set-clipboard change, no passthrough, copies over matchblox's link) holds.
