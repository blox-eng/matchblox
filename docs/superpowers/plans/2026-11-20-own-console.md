Spec: https://github.com/blox-eng/matchblox/issues/448
Spec approved: 2026-11-20
Test command: SSH_AUTH_SOCK=~/.ssh/agent.sock go test ./... -count=1
Plan approved: 2026-11-20 by the owner
Next: builder:build

# Plan: each terminal holds its own console, and a click on the list takes the keys back

What the code does today:
- `goHome` (`cmd/matchblox/main.go`) execs `panes.HomeArgv`: `tmux new-session -A -s matchblox`. Every terminal attaches to that one session.
- Every tmux guard names that one session: `BesideKeysArgv` (`ctrl+q`), `endBeside`, `atConsole` (marks, belt, taps, viewer hook), `goTo` and `homeExists` (the way back), `handOff` (`⌥s`, `⌥n`), and `SizeArgv`.
- `remote.FocusArgv` (Ctrl+O and the phone's open) moves a client into a new `matchblox-in-<ns>` session. From there the way back always goes to `=matchblox`.
- `sideTap` (`internal/app/sidescreen.go`) runs `chooseSide(k).showChosen()` on a row in beside mode. That gives the agent the keys, as `Enter` does.
- Alerts and pushes come from the service (`s.AlertHook`, `pushSender`), not from the console. More consoles do not send more buzzes.
- Views are nested clients in `matchblox-jump-<random id>` sessions (`m.jumpID()`). Two consoles that show one agent get two separate view sessions.

## The file map

- `internal/panes/home.go` (change): one name rule for console sessions, the `@matchblox_home` mark, the guards and the way back that follow the client's own console.
- `internal/panes/beside.go` (change): `ctrl+q` and `endBeside` act in every console session, each on its own window.
- `internal/panes/flip.go` (change): `⌥s` and `⌥n` hand off to the client's own console.
- `internal/panes/size.go` or `home.go` (change): `SizeArgv` sizes the window of the console that runs it.
- `internal/panes/own_tmux_test.go` (create): the real-tmux tests of two consoles side by side.
- `internal/panes/home_test.go` (change): the tests of the name rule.
- `internal/remote/remote.go` (change): `FocusArgv` marks the new session with the console it came from; `FocusAllowed` accepts the mark.
- `internal/remote/remote_test.go` (change): the tests of the mark.
- `cmd/matchblox/main.go` (change): `goHome` picks the session name; the console marks its own session at start.
- `cmd/matchblox/gate.go` (change): a phone joins the first console.
- `cmd/matchblox/gate_test.go` (change): the test of the phone's join.
- `internal/app/sidescreen.go` (change): a click on a row in beside mode chooses the row and shows it, and the keys stay in the list.
- `internal/app/sidedesk_test.go` (change): the tests of the click.
- `internal/app/into.go`, `internal/app/model.go` (change): pass the console's session name to `FocusArgv`.
- `docs/keys.md`, `docs/console.md`, `CHANGELOG.md`, `www/` (change): what each terminal and a click now do.

## The tasks

### Task 1: keys typed in one terminal reach only that terminal's agent

Surface: an agent's tools

- **Files:** `internal/panes/own_tmux_test.go`, `internal/panes/home.go`, `internal/panes/beside.go`.
- **Test first:** `TestTwoConsolesKeepTheirKeysApart`. It starts a test server with two console sessions, `matchblox` and `matchblox-2`. Each has a console pane (raw `cat` into a file, as `beside_tmux_test.go` does) and an agent pane beside it (`cat` into its own file), marked with `ConsoleOption` and `BesideOption`. The test binds `BesideKeysArgv`, then attaches client 1 to `matchblox` and client 2 to `matchblox-2`. Client 2 presses `ctrl+q`, then types `rm -rf x`. Assert: the text is in `matchblox-2`'s agent file and not in `matchblox`'s agent file, and client 1's active pane did not change.
- **Run:** `go test ./internal/panes -run TestTwoConsolesKeepTheirKeysApart -count=1`. Before: it fails. In `matchblox-2` the `ctrl+q` guard is false, so `ctrl+q` goes into the console as a key, and no agent pane gets the text. After: it passes.
- **Do:** add `HomeOption = "@matchblox_home"`, a session option. Add `atHome = "#{" + HomeOption + "}"`. In `BesideKeysArgv` and `atConsole`, replace `#{==:#{session_name},matchblox}` with `atHome`. The test sets the option on both sessions, as Task 4's start will.
- **Done when:** the test passes, and `TestCtrlQGoesToTheAgentAndBack` still passes after it sets the option on its `matchblox` session.
- **Commit:** `feat: ctrl+q flips the keys in every console's own window`

### Task 2: a console that ends takes only its own agent pane

- **Files:** `internal/panes/own_tmux_test.go`, `internal/panes/beside.go`.
- **Test first:** `TestAConsoleEndsOnlyItsOwnAgentPane`. Use the two consoles of Task 1 and `BesideHookArgv`. Kill `matchblox-2`'s console pane. Assert: `matchblox-2`'s agent pane is gone. `matchblox`'s console and agent pane still exist.
- **Run:** before: it fails, because `endBeside` walks only the session named `matchblox`, so `matchblox-2`'s agent pane stays. After: it passes.
- **Do:** in `endBeside`, walk each session with `HomeOption`, not the one named `Home`. Keep the rule that each window ends once.
- **Done when:** the test passes, and `TestTheAgentPanesEndWithTheConsole` and `TestARespawnedConsoleKeepsItsAgentPane` pass.
- **Commit:** `feat: a console that ends takes only its own agent pane`

### Task 3: the way back goes to the console of the terminal that pressed it

- **Files:** `internal/panes/own_tmux_test.go`, `internal/panes/home.go`, `internal/panes/flip.go`, `internal/remote/remote.go`, `internal/remote/remote_test.go`, `internal/app/into.go`, `internal/app/model.go`.
- **Test first:**
  - `TestTheWayBackStaysInItsOwnConsole`: client 2 is on `matchblox-2`'s agent pane and presses `HomeKey`. Assert: client 2 is still in `matchblox-2`, and the console pane there is active.
  - `TestTheWayBackFromAFocusGoesToItsConsole`: a session `matchblox-in-1` with `@matchblox_back` set to `matchblox-2`. Client 2 is there and presses `HomeKey`. Assert: client 2 is in `matchblox-2`.
  - `TestTheWayBackWithNoMarkGoesToTheFirstConsole`: a client in a plain `work` session presses `HomeKey`. Assert: it goes to `matchblox`, as today.
  - `TestFlipHandsOffToItsOwnConsole`: `⌥s` in a marked agent pane of `matchblox-in-1` (back = `matchblox-2`) reaches `matchblox-2`'s console file and not `matchblox`'s.
  - In `remote_test.go`: `TestFocusArgvMarksItsConsole` asserts that `FocusArgv(tty, pane, at, "matchblox-2")` sets `@matchblox_back matchblox-2` on the new session, and that `FocusAllowed` accepts it. `TestFocusAllowedRefusesABadBack` asserts that a back of `x;kill-server` or `../a` is refused.
- **Run:** `go test ./internal/panes ./internal/remote -count=1`. Before: the first, second and fourth tests fail, because `goTo` and `handOff` switch to `=matchblox`. The remote tests do not compile, because `FocusArgv` takes no back. After: all pass.
- **Do:**
  - Add the format `consoleOf` to `home.go`: the client's session name when it has `HomeOption`; else its `@matchblox_back` when that session exists; else `matchblox`.
  - `goTo` switches to `=#{consoleOf}` and selects that session's console pane. Its fallbacks (a console run in place, `NoHome`) stay.
  - `handOff` runs its `run-shell -C -t` on `=#{consoleOf}:`.
  - `FocusArgv` takes `back string` and adds `set-option -t <name> @matchblox_back <back>`. `FocusAllowed` checks `back` against `^matchblox(-[0-9]{1,4})?$`.
  - The console passes its own session name. `main.go` reads it once at start (`display -p -t $TMUX_PANE #{session_name}`) into `Options.HomeSession`.
- **Done when:** the six tests pass, and every test in `home_linux_test.go` and `flip_linux_test.go` passes.
- **Commit:** `feat: the way back goes to the console of the terminal that pressed it`

### Task 4: a second terminal opens a console of its own

- **Files:** `internal/panes/home.go`, `internal/panes/home_test.go`, `internal/panes/own_tmux_test.go`, `cmd/matchblox/main.go`, `cmd/matchblox/gate.go`, `cmd/matchblox/gate_test.go`.
- **Test first:**
  - `TestHomeSessionName` (a table): no `matchblox` session gives `matchblox`. `matchblox` with no client gives `matchblox`. `matchblox` with a client gives `matchblox-2`. `matchblox` and `matchblox-2` both taken gives `matchblox-3`. `join` set gives `matchblox` whatever the clients.
  - `TestHomeArgvOutsideTmux` (changed): for `matchblox-2` the argv is `new-session -A -s matchblox-2 -e MATCHBLOX_HOME=1 …`.
  - `TestAClosedTerminalEndsItsConsole`: run `HomeMarkArgv` on `matchblox-2` with a client attached, then detach the client. Assert: `matchblox-2` is gone and `matchblox` still exists.
  - `TestTheFirstConsoleOutlivesItsTerminal`: the same on `matchblox`. Assert: it stays.
  - `TestAPhoneJoinsTheFirstConsole` in `gate_test.go`: the phone mode sets `panes.JoinEnv=1` before `gateRun`.
- **Run:** `go test ./internal/panes ./cmd/matchblox -count=1`. Before: the tests do not compile (`HomeSession`, `HomeMarkArgv` and `JoinEnv` do not exist). After: they pass.
- **Do:**
  - Add `HomeSession(sessions map[string]int, join bool) string`, a pure function of the client count per session.
  - Add `HomeArgv(inTmux, haveTmux bool, name string, self []string)`.
  - Add `HomeMarkArgv(pane, name string)`. It sets `HomeOption` on the pane's session. On a name other than `matchblox`, it also sets `destroy-unattached on`.
  - `goHome` lists the sessions with their client counts (`list-sessions -F '#{session_name} #{session_attached}'`; no server is an empty map), picks the name and execs.
  - The console runs `HomeMarkArgv` with its other binds, when `HomeEnv` is 1.
  - `SizeArgv` targets the console's own window (`-t $TMUX_PANE`), not `=matchblox:`.
  - `gate.go` sets `JoinEnv` in phone mode.
- **Done when:** the five tests pass.
- **Commit:** `feat: a second terminal opens a console of its own`

### Task 5: a click on a row keeps the keys in the list

- **Files:** `internal/app/sidescreen.go`, `internal/app/sidedesk_test.go`.
- **Test first:**
  - `TestAClickOnARowKeepsTheKeysInTheList`: a beside-mode model shows `auth`. A click on the `docs` row chooses `docs` and runs a view of `docs` with `preview` (the keys stay in the list). Assert: no `select-pane` of the agent pane runs, and the key line says `in the list`.
  - `TestAClickOnTheShownRowKeepsTheKeysInTheList`: a click on `auth`, which is already shown, starts no `focus` action.
  - `TestATapOnARowStillOpensItOnOnePane`: on a window narrow enough for `onePaneAt`, a tap on a row still shows the agent full screen, as today.
- **Run:** `go test ./internal/app -run 'Click|TapOnARow' -count=1`. Before: the first two fail. `sideTap` calls `showChosen`, which gives the agent the keys. After: they pass.
- **Do:** in `sideTap`, a hit on a row in beside mode, when the window is not `onePaneAt`, runs `chooseSide(k)`, then the look at once (`look` with a fresh `previewSeq`, no 150 ms rest). `onePaneAt` keeps `showChosen`. Change the doc comment of `sideTap` to say this.
- **Done when:** the three tests pass, and every test in `sidedesk_test.go` and `beside_test.go` passes. A test that asserted the old click is changed, and the commit names it.
- **Commit:** `feat: a click on the list takes the keys back and keeps them there`

### Task 6: the docs and the landing page say it

- **Files:** `docs/keys.md`, `docs/console.md`, `CHANGELOG.md`, `www/`.
- **Test first:** no test can see prose. Check: `rg -n "Devices share one view|a click on a row gives it the keys|click or a tap on a row does the same" docs` finds lines before the change, and none after it. Then the docs check runs in the suite (`internal/docscheck`).
- **Do:** say that each terminal has its own console, that the phone joins the first, that the way back goes to its own terminal's console, and what a click does. Add a changelog line under `## [Unreleased]` of 8 words or fewer. Update the landing page in this branch (memory: a feat PR updates `www/` itself).
- **Done when:** the `rg` check finds nothing, and `.github/scripts/landing-guard.sh` passes on the PR body.
- **Commit:** `docs: each terminal has its own console, and a click returns to the list`

### Task 7: review the whole change

Load `builder:review` with the spec (#448), this plan and the test command. Run the race detector three times on `./internal/panes ./internal/app ./internal/remote`. Use it in a real terminal: two `matchblox` windows, the way back in each, close one, `tmux ls`.

## Lens findings

- Engineering: two terminals that start at the same moment can both see `matchblox` with no client, and both attach to it. No change, because it is rare and gives today's behavior. The next terminal gets its own console.
- Engineering: a session the person names `matchblox-2` themselves would be taken for a console name. Changed: `HomeSession` skips each name that exists, and only `HomeOption` (never the name) makes a session a console. Task 4.
- Engineering: `@matchblox_back` from a jump (`matchblox-jump-*`) is not set. No change, because a view's client is nested and never presses the way back in its own session: the outer client does.
- Design: the key line already says `in the list` or `writing to <name>`. A second console needs no label (spec call). No change.
- Design: with the tmux mouse off, a click cannot reach the list. No change, because `ctrl+q` stays, and the "Add the way back" step turns the mouse on.
- Architecture: the boundary is tmux's server state. One option (`@matchblox_home`) owns "this session is a console". Every guard reads that option, never the name. Tasks 1 to 4.
- Architecture: alerts and pushes live in the service, so a second console adds none. No change. Task 7 checks it.

## Check against the spec

| Spec line | Task |
|---|---|
| Two terminals each show and type into their own agent | Task 1, Task 4 |
| ↑ ↓ in terminal 2 do not move terminal 1 | Task 4 (separate console processes) |
| `ctrl+a m` in terminal 2 shows its own list; terminal 1 keeps `auth` | Task 3 |
| A click on a row: keys in the list, row chosen, its agent shown | Task 5 |
| Close terminal 2: no console session left, every agent runs | Task 4, Task 2 |
| A wait buzzes once with two consoles | Lens (service sends them), Task 7 |
| Docs and landing page say it | Task 6 |
| Anti-goal: text typed in terminal 1 reaches terminal 2's agent | Task 1 |
| Anti-goal: a closing terminal ends an agent or another console | Task 2, Task 4 |
| Anti-goal: `ctrl+a m` in terminal 2 goes to terminal 1's console | Task 3 |
| Anti-goal: one wait buzzes once per console | Lens, Task 7 |
| Anti-goal: closed terminals leave consoles running | Task 4 |
| Surface: an agent's tools | Task 1 |
| Call: the phone joins the first console | Task 4 |

## Changes during build

- Task 1 also adds `HomeMarkArgv(pane)`, and the console runs it at start (from Task 4). Because: once the guards read `HomeOption`, a console that does not set it loses `ctrl+q` and the marks. Each commit must work on its own. Task 4 adds `destroy-unattached` to it. Test: `TestAConsoleMarksItsOwnSession`.
