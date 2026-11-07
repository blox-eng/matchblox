# Enter Goes Into the Pane Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `Enter`, `→`, `a`, `ctrl+o` and a row tap go into the agent's real tmux pane on every host and device, moving only the terminal that asked; Sessions keeps a read-only live view; the demo gets a full-screen stand-in; the key-giving chat is deleted.

**Architecture:** Every entry point calls one `goInto`, which builds today's 4-argument nav step plus the asking client's tty on the action. The tty comes from a marker the iOS app sends before its own CR and mouse presses (a tmux user-key whose binding hands the console `#{client_tty}`), else from the last-active non-device client that shows the console. On the home host the step runs as `remote.FocusArgv(tty, pane)`; on another host `Jump.Local` switches `-c tty`. `chat.go` becomes `liveview.go`: drawing stays, key code goes, and its full-screen mode survives only as the demo's stand-in, whose keys reach `World` through the service's `keys:` act, which the real service now refuses.

**Tech Stack:** Go 1.26 (Bubble Tea v2, bubbles/key, ultraviolet), tmux 3.2+ (3.3+ for the agent signal passthrough), Swift 5 / UIKit / SwiftTerm, XCTest.

**Spec:** `docs/superpowers/specs/2026-11-07-enter-goes-into-the-pane-design.md` (with the eng review's D1-D4 at c27fa942). Review record: `~/.gstack/projects/blox-eng-matchblox/feat-334-enter-into-pane-eng-review-20261107-220838.md`; test plan T1-T25: `…/blox-master-feat-334-enter-into-pane-eng-review-test-plan-20261107-223032.md`.

## Global Constraints

- User-facing strings, verbatim: `<name> runs outside tmux: no pane to go into` (replaces today's `… there is no pane to type into`, `chat.go:484`); `enter goes into <name>` (the live view's key line); `this device is not attached: reconnect`; `type 2 to answer` (the stand-in's hint); `‹ Sessions` (the stand-in's top line, as `chatScreen` draws it today); pairing beat 3 contains `go into an agent from it`; `take over?` (unchanged `asksTakeOver` prompt).
- A jump moves only the asking client. No code path may run a bare `tmux switch-client -t <pane>` when an asking tty is known.
- A mark is spent by the next input event (`tea.KeyPressMsg` or `tea.MouseClickMsg`) and lasts at most 300 ms.
- The marker binding runs only `atConsole` (`#{&&:#{==:#{session_name},matchblox},#{==:#{pane_current_command},matchblox}}`, `panes/home.go:83`). An old app or an old console sends or reads no marker, and nothing is typed out.
- The nav step stays 4 arguments for `remote.NavAllowed`; the tty rides on `action`, `FocusArgv` runs only for a home-host jump, `Jump.ClientTTY` is checked by `JumpAllowed`.
- Command keys type only fixed `text` from `internal/keycmd/commands.json`, never text from the app, and only into a pane whose `@matchblox_agent` equals the command's `agent`.
- No flag, option or build tag keeps the key-giving chat. The demo's stand-in is the only full-screen mode, reachable only when `Options.Demo != ""`.
- Go checks: `go test ./internal/... ./cmd/...`, `scripts/validate.sh` (gofmt, vet, golangci-lint, tests, smoke, cross builds). Tests that start tmux use `tmuxtest.Server(t)` (the `-L` rule of `internal/tmuxtest`).
- Swift: `make -C ios test` (MatchbloxKit, `swift test`) and `make -C ios uitest DESTINATION='platform=iOS Simulator,name=iPhone 16'` (scheme `Matchblox`, target `MatchbloxUITests`). Neither runs on this Linux host (no `swift`, no `xcodebuild`): run them on a Mac.
- Commits: signed (`git commit -S`), Conventional Commits, `(#334)` in the subject, body ending with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Docs: short active sentences, as the repo's docs are written.

## Review Focus

1. A phone that attaches while the console already runs hears the marker announce only at the next graphics poll (`graphicsEvery`, 2 s, `logos.go:29`); its Enter before that is unmarked and must not move the desk. Expected: an unmarked key from a lone device still jumps that device (the fallback in `deskOf`). Test: `TestADeviceAloneIsItsOwnDesk` (Task 4).
2. A console run in place in the builder's own tmux session (not `=matchblox`, `NoPrefixM`): clients are matched by the console's own pane id, not the session name. Expected: the desk in session `work` is still the desk. Test: `TestADeskInAnotherSessionIsStillTheDesk` (Task 4).
3. tmux slow or failing on `list-clients`: expected the jump still happens, as today's plain `switch-client -t <pane>`. Test: `TestATmuxErrorJumpsAsBefore` (Task 4).
4. A desk key within 300 ms of a phone's unspent mark (the phone sent the mark, its CR was lost): expected the mark goes only to the very next event, and a stale mark never outlives 300 ms. Tests: `TestAMarkIsSpentByTheNextEvent`, `TestAMarkLapsesAfter300ms` (Task 4).
5. The pane ends between the selection and Enter: expected the existing `failed: … ` flash names it and the selection does not move. Test: `TestAJumpToAGonePaneSaysSo` (Task 4).

---

## File Structure

| File | Change | Responsibility |
|---|---|---|
| `internal/remote/jump.go` | modify | `Jump.ClientTTY`; `JumpAllowed` checks it; `Local` switches `-c` |
| `internal/remote/remote.go` | modify | `AgentMarkArgv`, `AgentMarkAllowed` (set `@matchblox_agent` on a pane) |
| `internal/keycmd/commands.json`, `keycmd.go` | modify | compact's `text`; marker key, marked prefix, pane signal, marks signal |
| `internal/panes/home.go` | modify | `MarkKeyArgv`; `CommandKeysArgv` types into marked agent panes |
| `cmd/matchblox/main.go` | modify | bind the marker; pass `Pane` (TMUX_PANE) and `ClientList` |
| `internal/app/into.go` | create | `goInto`, `runInto`, `intoCmd`, `noPane` |
| `internal/app/asker.go` | create | marks, `consoleClient`, `clientsOf`, `deskOf`, `othersHere`, `askingTTY` |
| `internal/app/chat.go` → `internal/app/liveview.go` | rename + cut | read-only view drawing; the demo stand-in; no key-giving |
| `internal/app/menutap.go` | delete | menu taps of the chat |
| `internal/app/model.go`, `keys.go`, `mouse.go`, `stoker.go`, `ships.go`, `stokertab.go`, `tabs.go`, `sessions.go`, `view.go`, `focus.go`, `scanask.go`, `layers.go`, `logos.go`, `demo.go`, `pairpage.go`, `remote.go` | modify | call sites of the above |
| `internal/service/keys.go`, `watch.go`, `hands.go`, `actions.go` | modify | `keys:` act only for the demo's World; wheel always scrolls back; `cmd:` act deleted |
| `internal/demo/world.go` | modify | `KeysToWorld`; arrow moves on a menu |
| `ios/MatchbloxKit/Sources/MatchbloxKit/InputMark.swift` | create | `InputMark`, `PaneAgent` |
| `ios/MatchbloxKit/Sources/MatchbloxKit/AgentSignal.swift` | modify | `ConsoleSignal` parse of `agent=<id>[;pane=<p>][;marks]` |
| `ios/Matchblox/ConsoleView.swift` | modify | mark in `send(source:data:)`; light commands in a known agent's pane |
| `ios/MatchbloxUITests/DemoConsoleTests.swift` | modify | the belt path to the invite |
| `README.md`, `docs/console.md`, `docs/keys.md`, `docs/phone.md`, `CHANGELOG.md` | modify | the jump and the read-only view |

Re-counted at plan time (finding 3): `chat.go` is 1186 lines, its key-giving half runs from `homeKey` (`chat.go:446`) to `chatKeyLine` (`chat.go:1112`) plus `wheelChat`; `menutap.go` 240; `service/keys.go` 134; `chat_test.go` 1920 lines, 94 tests.

---

### Task 1: A host jump moves only the asking client

**Files:**
- Modify: `internal/remote/jump.go:16-27` (struct), `:76-90` (`JumpAllowed`), `:210-235` (`Local`)
- Modify: `internal/remote/remote.go` (after `FocusAllowed`, `:465`)
- Test: `internal/remote/jump_test.go`, `internal/remote/jump_tmux_test.go`, `internal/remote/remote_test.go`

**Interfaces:**
- Consumes: `ClientTTY(tty string) bool` (`remote.go:450`), `PaneID`, `keycmd.ForAgent`.
- Produces: `Jump.ClientTTY string` ("" = tmux's current client, as today); `func AgentMarkArgv(pane, agent string) []string` = `tmux set-option -p -t <pane> @matchblox_agent <agent>`; `func AgentMarkAllowed(argv []string) bool` (exact shape, `PaneID(pane)`, `len(keycmd.ForAgent(agent)) > 0`).

- [ ] **Step 1: Write the failing tests (T5)**

```go
// jump_test.go
func TestJumpLocalSwitchesOnlyItsClient(t *testing.T) {
	j := Jump{Host: "ws-1", Pane: "%12", Prefix: "C-b", ID: "0a1b2c3d", ClientTTY: "/dev/pts/7"}
	got := Line(j.Local([]string{"ssh", "x"}))
	if !strings.Contains(got, " switch-client -c /dev/pts/7 -t '=matchblox-ws-1-0a1b2c3d' ';'") {
		t.Errorf("the switch names no client: %s", got)
	}
	j.ClientTTY = ""
	if got := Line(j.Local([]string{"ssh", "x"})); strings.Contains(got, "switch-client -c") {
		t.Errorf("no tty, yet a -c: %s", got)
	}
}
// TestJumpAllowed gains:
//   "client tty":     {with(func(j *Jump) { j.ClientTTY = "/dev/pts/7" }), true},
//   "bad client tty": {with(func(j *Jump) { j.ClientTTY = "/dev/pts/7;x" }), false},
//   "relative tty":   {with(func(j *Jump) { j.ClientTTY = "pts/7" }), false},

// remote_test.go
func TestAgentMarkAllowedIsExact(t *testing.T) {
	for _, c := range []struct{ argv []string; want bool }{
		{AgentMarkArgv("%3", "claude"), true},
		{AgentMarkArgv("%3", "codex"), true},
		{AgentMarkArgv("%3", "zsh"), false},
		{AgentMarkArgv("%3;x", "claude"), false},
		{AgentMarkArgv("3", "claude"), false},
	} {
		if got := AgentMarkAllowed(c.argv); got != c.want {
			t.Errorf("%q: %v, want %v", c.argv, got, c.want)
		}
	}
	if !slices.Equal(AgentMarkArgv("%3", "claude"), []string{"tmux", "set-option", "-p", "-t", "%3", "@matchblox_agent", "claude"}) {
		t.Error("the mark's argv changed")
	}
}

// jump_tmux_test.go, beside TestJumpKeepsOtherClientsWindow (part of T19)
func TestJumpWithAClientTTYLeavesTheOtherClientHome(t *testing.T) {
	e := newJumpEnv(t)
	desk := attach(t, e.outer, "matchblox")
	phone := attach(t, e.outer, "matchblox")
	j := e.newJump("0a1b2c3d")
	j.ClientTTY = phone.tty // attach sets client.tty
	e.jump(t, phone, j, e.ssh)
	waitFor(t, "the phone is in the jump", func() bool { return phone.session() == HostSessionName(j.Host, j.ID) })
	if desk.session() != "matchblox" {
		t.Errorf("the desk moved to %q", desk.session())
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/remote -run 'TestJumpLocalSwitchesOnlyItsClient|TestJumpAllowed|TestAgentMarkAllowedIsExact|TestJumpWithAClientTTY' -v`
Expected: FAIL, `j.ClientTTY undefined`, `undefined: AgentMarkArgv`.

- [ ] **Step 3: Implement**

`Jump.ClientTTY` in the struct; `JumpAllowed` adds `j.ClientTTY == "" || ClientTTY(j.ClientTTY)`; `Local`'s `{"switch-client", "-t", "=" + s}` becomes `{"switch-client", "-c", j.ClientTTY, "-t", "=" + s}` when `ClientTTY != ""`. `AgentMarkArgv` / `AgentMarkAllowed` in `remote.go` beside `FocusArgv`.

- [ ] **Step 4: Run the package**

Run: `go test ./internal/remote/...`
Expected: PASS (the tmux tests skip where tmux is missing).

- [ ] **Step 5: Commit**

```bash
git add internal/remote
git commit -S -m "feat(remote): a host jump switches only the client that asked (#334)" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: The marker key and the command texts

**Files:**
- Modify: `internal/keycmd/commands.json:2-9` (compact), `internal/keycmd/keycmd.go:56-63`
- Modify: `internal/panes/home.go` (after `CommandKeysArgv`, `:96`)
- Modify: `cmd/matchblox/main.go:295-303` (binds)
- Test: `internal/keycmd/keycmd_test.go`, `internal/panes/home_test.go`, `internal/panes/home_linux_test.go`

**Interfaces:**
- Consumes: `hexKeys`, `Home`, the `atConsole` format of `CommandKeysArgv`.
- Produces (new, `keycmd`):
  - `const MarkKey = "\x1b_matchblox;mark\x1b\\"` — what the app sends before a CR or a press.
  - `const MarkSlot = 950` — its user-keys slot (900 is home, 901-914 commands).
  - `const MarkedPrefix = "\x1b_matchblox;from="` — what the binding hands the console, then the tty, then `ESC \`.
  - `func PaneSignal(agent, pane string) string` = `AgentSignal(agent + ";pane=" + pane)`.
  - `func MarksSignal() string` = `AgentSignal(";marks")`: the console's announce that it reads the marker.
- Produces (new, `panes`): `func MarkKeyArgv() []string`.

- [ ] **Step 1: Write the failing tests (T3, T19 marker half, T20 Go half, T23 table half)**

```go
// keycmd_test.go
func TestCompactTypesItsText(t *testing.T) {
	c, ok := ByID("compact")
	if !ok || c.Text != "/compact" || c.Agent != "claude" {
		t.Errorf("compact = %+v", c)
	}
}
func TestTheSignalsNameThePaneAndTheMarks(t *testing.T) {
	if got := PaneSignal("claude", "%12"); got != "\x1b_matchblox;agent=claude;pane=%12\x1b\\" {
		t.Errorf("PaneSignal = %q", got)
	}
	if got := MarksSignal(); got != "\x1b_matchblox;agent=;marks\x1b\\" {
		t.Errorf("MarksSignal = %q", got)
	}
}

// home_test.go: TestHomeAndCommandKeysDoNotCollide also holds MarkSlot and MarkKey apart.
func TestMarkKeyArgvRunsOnlyAtTheConsole(t *testing.T) {
	line := strings.Join(MarkKeyArgv(), " ")
	for _, want := range []string{"user-keys[950]", "bind-key -n User950 if-shell -F",
		"#{&&:#{==:#{session_name},matchblox},#{==:#{pane_current_command},matchblox}}", "#{client_tty}"} {
		if !strings.Contains(line, want) {
			t.Errorf("MarkKeyArgv lacks %q: %s", want, line)
		}
	}
}

// home_linux_test.go, with consolePane / attach / eventLine / waitFor
func TestAMarkedReturnNamesTheClientThatSentIt(t *testing.T) { // T3, T19
	sock, tmux, got := consolePane(t)
	tmux(MarkKeyArgv()[1:]...)
	desk, phone := attach(t, sock, Home), attach(t, sock, Home)
	_ = desk
	ttys := strings.Fields(tmux("list-clients", "-F", "#{client_tty}"))
	phoneTTY := ttys[len(ttys)-1] // the newest client; pick by client_created if the order differs
	if _, err := phone.WriteString(keycmd.MarkKey + "\r"); err != nil {
		t.Fatal(err)
	}
	want := eventLine(uv.UnknownApcEvent(keycmd.MarkedPrefix + phoneTTY + "\x1b\\"))
	waitFor(t, func() bool { return slices.Contains(paneRead(t, got), want) })
	if lines := paneRead(t, got); slices.Index(lines, want) > slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "enter") }) {
		t.Errorf("the mark came after the Return: %q", lines)
	}
}
func TestAMarkElsewhereTypesNothing(t *testing.T) { // T3, T20
	sock := tmuxtest.Server(t)
	// a shell pane in session "work" running cat -v into a file; bind MarkKeyArgv; type MarkKey into a client there
	// want: the file stays empty (no letters of the APC typed out)
}
```

Note for `TestAMarkedReturnNamesTheClientThatSentIt`: `consolePane` runs `TestDecodePane` under the name `matchblox`, so `pane_current_command` matches; the Return decodes as a key press event line, so match the line `eventLine(uv.KeyPressEvent{Code: uv.KeyEnter})` exactly once you see its format.

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/keycmd ./internal/panes -run 'Compact|Signals|MarkKey|AMarked|AMarkElsewhere|DoNotCollide' -v`
Expected: FAIL, `undefined: PaneSignal`, `undefined: MarkKeyArgv`.

- [ ] **Step 3: Implement**

`commands.json`: compact gains `"text": "/compact"` (its `run` stays `compact`). `keycmd.go`: the constants and two functions above. `MarkKeyArgv` is this exact list (the quoting is the algorithm: `run-shell -C` expands `#{client_tty}` and `#{pane_id}`, then runs the three `send-keys` as one list):

```go
func MarkKeyArgv() []string {
	const atConsole = "#{&&:#{==:#{session_name}," + Home + "},#{==:#{pane_current_command}," + Home + "}}"
	slot := strconv.Itoa(keycmd.MarkSlot)
	hand := "run-shell -C 'send-keys -t #{pane_id} -H " + hexKeys(keycmd.MarkedPrefix) +
		" ; send-keys -t #{pane_id} -l #{client_tty} ; send-keys -t #{pane_id} -H 1b 5c'"
	return []string{"tmux", "set-option", "-s", "user-keys[" + slot + "]", keycmd.MarkKey,
		";", "bind-key", "-n", "User" + slot, "if-shell", "-F", atConsole, hand}
}
```

Hoist `atConsole` to a package const shared with `CommandKeysArgv`. `main.go`: append `panes.MarkKeyArgv()` beside `panes.CommandKeysArgv()` under `HomeEnv == "1"`.

- [ ] **Step 4: Run the packages**

Run: `go test ./internal/keycmd ./internal/panes ./cmd/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/keycmd internal/panes cmd/matchblox/main.go
git commit -S -m "feat(panes): a marker key names the client that pressed (#334)" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Every way in goes into the pane

The chat still exists after this task, but nothing outside the demo opens it. In the demo `goInto` still opens the chat (`chatOn`), so `internal/demo` stays green; Task 5 swaps it for the stand-in.

**Files:**
- Create: `internal/app/into.go`
- Modify: `internal/app/model.go:1347-1395` (`key`: Open, Chat, Jump), `:1053-1108` (`takeFocus`)
- Modify: `internal/app/keys.go:96-97` (`Chat` → `Into`), `internal/app/mouse.go:50-160`, `internal/app/stoker.go:172-226` (`asksTakeOver`, `hold`), `internal/app/ships.go:473-490`, `internal/app/stokertab.go:237-251`, `internal/app/sessions.go:326,690-705` (hints), `internal/app/chat.go:475-489,620-630`
- Test: `internal/app/into_test.go` (new); delete or rewrite the chat-opening tests in `chat_test.go` (`TestEnterGivesTheChatTheKeys`, `TestEnterOnASessionOutsideTmuxSaysWhy`, `TestCtrlOGoesIntoThePane`, `TestATapOnAPhoneRowOpensItsChat`, `TestAOpensTheChatOnShipsAndStoker`), `tap_test.go` (`TestFocusOpensTheChatOfAWait`, `TestFocusKeepsAChatBeingTyped`, `TestFocusStartsTheMenusQuiet`), `app_test.go` (`TestJumpPrintsCommandThenRunsOnConfirm`)

**Interfaces:**
- Consumes: `m.jumpOn(host, pane) *action` (`model.go:1591`), `m.forHost` (`remote.go:18`), `m.confirm` (`model.go:1440`), `m.takeOverOn`, `m.chatOn` (demo only, until Task 5).
- Produces (new):
  - `func noPane(name string) string` → `name + " runs outside tmux: no pane to go into"`.
  - `func (m Model) goInto(host, pane string) (tea.Model, tea.Cmd)` — builds `m.forHost(host, m.jumpOn(host, pane))` and hands it to `runInto`; in the demo, the chat (Task 5: the stand-in).
  - `func (m Model) runInto(a *action) (tea.Model, tea.Cmd)` — a home-host nav step runs at once through `m.confirm(*a)`; any other (another host's ssh jump, a refusal) waits as `m.pending`, as Enter does today.
  - `mainKeys.Into` (renamed from `Chat`): `bind(ks("a", "right"), "a", "go in")`; `Open` keeps `⏎`, its hint word becomes `go in`; `Jump` keeps `ctrl+o`, desc `go in`.

- [ ] **Step 1: Write the failing tests (T1, T11, T12, T13, goal 7)**

```go
// into_test.go
func TestEveryWayInJumps(t *testing.T) { // T1
	for _, k := range []string{"enter", "right", "a", "ctrl+o"} {
		var ran [][]string
		m := loaded(t, 100)
		m.opt.Run = func(argv []string) error { ran = append(ran, argv); return nil }
		next, cmd := key(m, k)
		drain(cmd)
		if nm := next.(Model); nm.chatKeys || nm.pending != nil {
			t.Errorf("%s: chat %v, pending %v: want the jump at once", k, nm.chatKeys, nm.pending)
		}
		if len(ran) != 1 || strings.Join(ran[0], " ") != "tmux switch-client -t %1" {
			t.Errorf("%s ran %q", k, ran)
		}
	}
}
func TestATapOnARowJumps(t *testing.T) { // T1: one column, and the second tap of a split screen
	// loaded at 44 wide: one tap on app-feature's row runs the jump; at 100 wide the row's second tap does
}
func TestAPanelessRowSaysSo(t *testing.T) { // T13
	m, _ := loadedWith(t, 100, outsideTmux(fixtureState())) // account_test.go: every pane ""
	m.tab = tabSessions
	s, _ := m.selectedSession()
	for _, k := range []string{"enter", "a", "right"} {
		next, _ := key(m, k)
		if got, want := next.(Model).flash, s.Name+" runs outside tmux: no pane to go into"; got != want {
			t.Errorf("%s: flash %q, want %q", k, got, want)
		}
	}
}
func TestEnterOnAStokerHostAsksToTakeOver(t *testing.T) { // T11
	m := sized(t, 100, 30, stokerState(runningNight(time.Now())))
	var ran [][]string
	m.opt.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	next, _ := key(m, "enter")
	if nm := next.(Model); nm.held == nil || len(ran) != 0 {
		t.Fatalf("held %v ran %q: want take over? first", nm.held != nil, ran)
	}
	next, cmd := key(next, "n") // keep the stoker: the jump still runs
	drain(cmd)
	if len(ran) != 1 || ran[0][1] != "switch-client" {
		t.Errorf("after the answer ran %q", ran)
	}
}
func TestAOnAStuckShipJumps(t *testing.T) { // T12, with shipsAt / withShips
	// a on the stuck ship of app-feature runs "tmux switch-client -t %1"; a on a ship that is not stuck flashes "a answers the session of a stuck ship" → reword to "a goes into the session of a stuck ship"
}
func TestAOnAFailedStepJumps(t *testing.T) { // T12, with tabNight / selectStep
	// a on a failed step runs "tmux switch-client -t <its pane>"; on a step that did not fail flashes "a goes into the session of a failed step"
}
func TestAFocusOnAWaitGoesIntoThePane(t *testing.T) { // goal 7, rewrites TestFocusOpensTheChatOfAWait
	m, _ := loadedWith(t, 100, fixtureState())
	var ran [][]string
	m.opt.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	m.opt.Clients = func() ([]string, error) { return []string{"/dev/ttys004"}, nil }
	st := focused("%2", time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC))
	st.Focus.Client = "/dev/ttys004"
	next, cmd := m.Update(stateMsg(st))
	drain(cmd)
	if next.(Model).chatKeys || len(ran) != 1 || !slices.Equal(ran[0], remote.FocusArgv("/dev/ttys004", "%2")) {
		t.Errorf("ran %q, chat %v: want FocusArgv for the phone", ran, next.(Model).chatKeys)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/app -run 'TestEveryWayInJumps|TestATapOnARowJumps|TestAPanelessRowSaysSo|TestEnterOnAStokerHost|TestAOnA|TestAFocusOnAWait' -v`
Expected: FAIL (Enter opens the chat; `ctrl+o` leaves a pending confirm).

- [ ] **Step 3: Implement `goInto`, `runInto`, `noPane` in `internal/app/into.go` and wire every entry**

- `key`: `Open` on a home session, `Into` on Sessions and `Jump` all call `goInto(s.Host, s.Pane)`, or flash `noPane(s.Name)` when `s.Pane == ""`. Another host's row keeps today's `m.pending = m.primary()` through `runInto`.
- `Into` on Ships calls `shipInto` (renamed `shipChat`; `goInto(s.Host, s.Pane)` for any host); on the Stoker tab, `stokerInto` (renamed from `stokerChatTarget` + `chatOn`) calls `goInto(m.opt.Host, r.e.Pane)`.
- `mouse.go`: the one-column row tap, the tap right of the list and the second tap on a selected session call `goInto`. `tapAct` is unchanged (its nav runs at once already).
- `asksTakeOver`: delete the `onSession && s.Host == m.opt.Host && Open` and `Chat` "looking around" case; `Into` joins the `Jump` case. `hold`: for `Open`, `Into` and `Jump` on a session, capture `a := m.primary()` now and return `func(m Model) (tea.Model, tea.Cmd) { return m.runInto(a) }`.
- `takeFocus`: drop the `waits` → `chatOn` branch and the chat `typing` hold; every found session with a pane goes through `focusJump`.
- `intoPane` and `openChat`'s non-demo path go; `chatTarget` remains only for the demo until Task 5.

- [ ] **Step 4: Run the package and fix the tests this changes**

Run: `go test ./internal/app/...`
Expected: PASS after the tests named under Files are deleted or rewritten to the jump. `TestJumpPrintsCommandThenRunsOnConfirm` becomes `TestCtrlORunsTheJumpAtOnce`.

- [ ] **Step 5: Commit**

```bash
git add internal/app
git commit -S -m "feat(console): Enter, a, → and a tap go into the agent's pane (#334)" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: The jump moves the client that asked

**Files:**
- Create: `internal/app/asker.go`
- Modify: `internal/app/into.go`, `internal/app/model.go:174-178` (`ranMsg`), `:182-212` (`action`), `:281+` (Model fields), `:597-616` (`Update`), `:648-653` (APC), `:918-932` (`ranMsg` case), `:1131-1150` (`focusJump`), `:1563-1589` (`runNav`, `jump`)
- Modify: `internal/app/remote.go:48-94` (`jumpOnHost` sets `j.ClientTTY = a.tty`), `internal/app/logos.go:70-72` (re-announce), `internal/app/layers.go:254-260` (announce at start), `internal/app/demo.go:15-36` (`ClientList` refused)
- Modify: `cmd/matchblox/main.go:624-640` (`Pane: os.Getenv("TMUX_PANE")`)
- Test: `internal/app/asker_test.go` (new), `internal/app/into_test.go`, `internal/app/remote_test.go`

**Interfaces:**
- Consumes: `keycmd.MarkedPrefix`, `keycmd.PaneSignal`, `keycmd.MarksSignal`, `keycmd.AgentOf`, `keycmd.ForAgent`, `remote.FocusArgv`, `remote.FocusAllowed`, `remote.AgentMarkArgv/Allowed`, `Jump.ClientTTY` (Task 1), `goInto`/`runInto` (Task 3), `sayAgent(say string, o Options) tea.Cmd` (`chat.go:790`), `PairHost.DeviceTTYs`.
- Produces (new):
  - `Options.Pane string` — the console's own tmux pane (TMUX_PANE); "" outside tmux.
  - `Options.ClientList func() (string, error)` — `tmux list-clients -F '#{client_activity} #{client_tty} #{pane_id}'`; nil asks tmux (500 ms, as `tmuxClients`).
  - `type consoleClient struct { tty string; active int64; here bool }`; `func clientsOf(out, consolePane string) []consoleClient` (`here` = `pane_id == consolePane`; bad lines skipped).
  - `func deskOf(cs []consoleClient, devices map[string]string) string` — the latest-active `here` client that is not in `devices`; else the latest-active `here` client; else "".
  - `func othersHere(cs []consoleClient, tty string) bool` — a `here` client other than `tty`.
  - `type clientMark struct { tty string; at time.Time }`, `const markLasts = 300 * time.Millisecond`, `func (c clientMark) spend(now time.Time) string` (the tty when `now - at <= markLasts`, else "").
  - Model fields `marked clientMark`, `askedBy string` (the tty of the event being handled; "" unmarked).
  - `func (m Model) askingTTY() string` — `askedBy`, else `deskOf(...)` of `ClientList`, "" outside tmux or on a tmux error.
  - `action.tty string`; `ranMsg.tty string`, `ranMsg.watched bool`.
  - `var errNotAttached = errors.New("this device is not attached: reconnect")`.
  - `func (m Model) intoCmd(a action) tea.Cmd` — the home-host jump, off the update loop.

- [ ] **Step 1: Write the failing tests (T2, T4, T9, T21, T24, T15 Go half, Review Focus 1-5)**

```go
// asker_test.go
const console = "%99"
func clients(lines ...string) func() (string, error) {
	return func() (string, error) { return strings.Join(lines, "\n"), nil }
}
func TestDeskIsTheLastActiveClientThatIsNoDevice(t *testing.T) { // T4
	cs := clientsOf("1794089228 /dev/pts/7 %99\n1794089220 /dev/pts/2 %99\n1794089210 /dev/pts/1 %99\n1794089229 /dev/pts/3 %4", console)
	if got := deskOf(cs, map[string]string{"/dev/pts/7": "iPad-id"}); got != "/dev/pts/2" {
		t.Errorf("desk = %q, want /dev/pts/2 (pts/7 is the iPad, pts/3 is not on the console)", got)
	}
}
func TestADeviceAloneIsItsOwnDesk(t *testing.T) { // Review Focus 1
	cs := clientsOf("1794089228 /dev/pts/7 %99", console)
	if got := deskOf(cs, map[string]string{"/dev/pts/7": "iPad-id"}); got != "/dev/pts/7" {
		t.Errorf("desk = %q", got)
	}
}
func TestADeskInAnotherSessionIsStillTheDesk(t *testing.T) { // Review Focus 2: matched by pane id
	cs := clientsOf("1794089228 /dev/pts/1 %99", console)
	if !cs[0].here {
		t.Error("the client on the console's pane is not here")
	}
}
func TestAMarkIsSpentByTheNextEvent(t *testing.T) { // Review Focus 4
	m := loaded(t, 100)
	next, _ := m.Update(uv.UnknownApcEvent(keycmd.MarkedPrefix + "/dev/pts/7\x1b\\"))
	next, _ = next.Update(keyPress("down"))
	if got := next.(Model).askedBy; got != "/dev/pts/7" {
		t.Errorf("the marked event asked as %q", got)
	}
	next, _ = next.Update(keyPress("up"))
	if got := next.(Model).askedBy; got != "" {
		t.Errorf("the next event still asked as %q", got)
	}
}
func TestAMarkLapsesAfter300ms(t *testing.T) { // Review Focus 4
	t0 := time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)
	if got := (clientMark{"/dev/pts/7", t0}).spend(t0.Add(301 * time.Millisecond)); got != "" {
		t.Errorf("a lapsed mark gave %q", got)
	}
	if got := (clientMark{"/dev/pts/7", t0}).spend(t0.Add(300 * time.Millisecond)); got != "/dev/pts/7" {
		t.Errorf("a fresh mark gave %q", got)
	}
}

// into_test.go
func jumpRig(t *testing.T, list string) (Model, *[][]string) // loaded(t,100), opt.Pane = console, opt.ClientList = clients(list),
	// opt.Clients lists every tty of list, opt.Pair = &PairHost{DeviceTTYs: {"/dev/pts/7": "iPad-id"}}, opt.Run records
func TestAMarkedEnterMovesOnlyThatClient(t *testing.T) { // T2
	m, ran := jumpRig(t, "1794089228 /dev/pts/7 %99\n1794089220 /dev/pts/1 %99")
	next, _ := m.Update(uv.UnknownApcEvent(keycmd.MarkedPrefix + "/dev/pts/7\x1b\\"))
	next, cmd := next.Update(keyPress("enter"))
	drain(cmd)
	if !slices.ContainsFunc(*ran, func(a []string) bool { return slices.Equal(a, remote.FocusArgv("/dev/pts/7", "%1")) }) {
		t.Errorf("ran %q, want FocusArgv for /dev/pts/7", *ran)
	}
}
func TestAnUnmarkedEnterMovesTheDesk(t *testing.T) { // T4: same rig, no mark → FocusArgv("/dev/pts/1", "%1")
}
func TestAMarkedKeyOfAGoneClientIsRefused(t *testing.T) { // marked, but opt.Clients no longer lists it
	// want: no FocusArgv ran, flash == "this device is not attached: reconnect"
}
func TestATmuxErrorJumpsAsBefore(t *testing.T) { // Review Focus 3: ClientList errors → ran == [["tmux","switch-client","-t","%1"]]
}
func TestAJumpToAGonePaneSaysSo(t *testing.T) { // Review Focus 5: Run fails on FocusArgv → flash starts "failed: tmux switch-client -c /dev/pts/1 -t %1", selection still %1
}
func TestTakeOverAnsweredFromTheDeskJumpsThePhone(t *testing.T) { // T21: stoker night; marked (pts/7) Enter → held; unmarked "n" → FocusArgv("/dev/pts/7", …)
}
func TestTheSelectionMovesOnWhenNoOneElseWatches(t *testing.T) { // T9, on queueState(): after ranMsg{jumped:"%1", tty:"/dev/pts/7", watched:false} the selection is %2
}
func TestTheSelectionStaysWhileTheDeskWatches(t *testing.T) { // T24: ranMsg{jumped:"%1", tty:"/dev/pts/7", watched:true} → still %1;
	// and intoCmd sets watched from othersHere: list "… /dev/pts/1 %99" after the jump of pts/7 → watched == true
}
func TestTheSignalNamesThePaneBeforeTheJump(t *testing.T) { // T15 Go half, T23 console half
	// Enter on app-feature (a claude session): the Cmd's first message is tea.Raw of logos.Passthrough(keycmd.PaneSignal("claude", "%1")),
	// and Run saw AgentMarkArgv("%1", "claude") before FocusArgv
}
func TestTheConsoleAnnouncesTheMarks(t *testing.T) { // Shell.Init and a new graphics viewers key both send keycmd.MarksSignal()
}

// remote_test.go
func TestAHostJumpCarriesTheAskingTTY(t *testing.T) { // T5 app half: onHost + marked Enter → the Local list holds "switch-client -c /dev/pts/7"
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/app -run 'Desk|DeviceAlone|AMark|Marked|Unmarked|TmuxError|GonePane|TakeOverAnswered|TheSelection|TheSignalNames|AnnouncesTheMarks|HostJumpCarries' -v`
Expected: FAIL, `undefined: clientsOf`, `askedBy undefined`.

- [ ] **Step 3: Implement**

- `Update`: an `uv.UnknownApcEvent` with `keycmd.MarkedPrefix` and a `remote.ClientTTY` tail sets `m.marked = clientMark{tty, m.now()}` and returns; for `tea.KeyPressMsg` and `tea.MouseClickMsg`, `m.askedBy, m.marked = m.marked.spend(m.now()), clientMark{}` before `m.update`.
- `jump(pane)` sets `a.tty = m.askingTTY()` (the action is built at key time, so a held take-over or a pending confirm keeps it).
- `runInto` for a home-host nav step: `tea.Sequence(sayAgent(keycmd.PaneSignal(id, pane), m.opt), m.intoCmd(*a))`, `id` = `keycmd.AgentOf(s.Agent)`, or `keycmd.OtherAgent` when `ForAgent` is empty; no signal for another host.
- `intoCmd(a)`: `a.tty == ""` → `runNav(a, run)` as today. Else, off the loop: refuse with `errNotAttached` when `m.clients()` lacks `a.tty`; check `NavAllowed(a.steps[0])` and `FocusAllowed(argv)`; run `AgentMarkArgv(a.pane, id)` when allowed (its error ignored), then `FocusArgv(a.tty, a.pane)`; `watched = othersHere(clientsOf(list()), a.tty)`; return `ranMsg{cmd: "tmux switch-client -c " + tty + " -t " + pane, jumped, tty, watched}`.
- `ranMsg` case: `errors.Is(msg.err, errNotAttached)` flashes the error's text alone; `nextAfter` only when `!msg.watched`. `focusJump` computes `watched` the same way.
- `jumpOnHost`: `j.ClientTTY = a.tty`.
- `Shell.Init` sends `sayAgent(keycmd.MarksSignal(), …)` in place of `noAgent`; `graphics` re-sends it when `msg.key != m.viewers` (in place of resetting `agentSaid`).
- `closeTheMachine`: `ClientList` returns the demo's refusal.

- [ ] **Step 4: Run the Go tests**

Run: `go test ./internal/app/... ./cmd/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app cmd/matchblox/main.go
git commit -S -m "feat(console): a jump moves only the screen that asked (#334)" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: The read-only view and the demo's stand-in; the chat's keys go

**Files:**
- Rename: `internal/app/chat.go` → `internal/app/liveview.go`; `internal/app/chat_test.go` → `internal/app/liveview_test.go`
- Delete: `internal/app/menutap.go`, `internal/app/menutap_test.go`
- Modify: `internal/app/model.go` (fields `chatKeys`, `menuSeen`, `chatTyped`, `keyQ`, `keyInFlight`, `ctrlCHeld`, `agentSaid`, `tmuxSaid`; `Update`'s `keepChat`/`noteMenu`/`tellAgent`; `PasteMsg`'s chat case; APC case), `keys.go:114-118,231-240` (`chatKeymap` keeps `Back`, `BackEnd`; `Stop` goes), `stoker.go:182-189,209-211` (the chat-key hold), `mouse.go`, `view.go:197-198,253,640`, `focus.go:60`, `scanask.go:38`, `pairpage.go:351`, `layers.go:281`, `logos.go:71`, `ships.go:489`, `demo.go`
- Modify: `internal/panes/home.go:76-96` (`CommandKeysArgv`), `internal/demo/world.go:143-200` (arrows)
- Modify: `docs/keys.md` ("The chat", "Scrolling", "An agent's menu", `:108-201`), `docs/console.md` ("The chat", `:347-503`)
- Test: `internal/app/liveview_test.go`, `internal/app/demo_test.go`, `internal/app/everyscreen_test.go`, `internal/panes/home_linux_test.go:295-397`, `internal/demo/script_test.go`, `internal/demo/world_test.go`

**Interfaces:**
- Consumes: `goInto` (Task 3), `nextAfter`, `keyName` (`keys.go:336`), `chatScreen`/`chatLines`/`chatPlacement` (kept, drawing), `watchCmd`, `homeKey`, `proto.KeysPrefix`, `proto.WheelUp/WheelDown`.
- Produces (new):
  - Model field `standIn bool` (replaces `chatKeys`): the demo's full-screen stand-in shows.
  - `func (m Model) openStandIn(pane string) Model` — `m` unchanged unless `m.opt.Demo != ""` and the pane is a home session's.
  - `func (m Model) standInKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd)` — `ctrl+a` then `m` (as `chatBacks`) leaves; any other key named by `keyName` goes as `proto.Act{RecID: proto.KeysPrefix + pane, Text, PID, Start, Seen: m.screen.At}`; nothing else, and nothing at all unless `m.opt.Demo != ""`.
  - `func (m Model) leaveStandIn() Model` — `standIn = false`, then `nextAfter(pane)`; run by the stand-in's back key, `homeKey`, and a tap on row 0 (`‹ Sessions`).
  - `const standInHint = "type 2 to answer"` — the stand-in's key line while its session waits at a menu.
  - `func (m Model) liveKeyLine(w int) (string, []span)` — `enter goes into <name>` (replaces `chatKeyLine`; a tap presses `enter`).
  - `func (m Model) wheelView(up bool) (tea.Model, tea.Cmd)` — sends `keys:<pane>` with `WheelUp`/`WheelDown` only; replaces `wheelChat`.
  - `goInto` in the demo calls `openStandIn` in place of `chatOn`.
  - `World` takes `send-keys -t <pane> Up|Down` at a menu and moves its cursor (new `func (w *World) move(pane string, up bool) error`).
  - `CommandKeysArgv`: each `User<slot>` binds `if-shell -F "#{==:#{@matchblox_agent},<agent>}" "send-keys -l <text>"`; the `atConsole` branch goes.

Delete with their tests: `chatKey`, `pressed`, `heldKeys`, `chatPaste`, `tellAgent`, `agentGone`, `noAgent`, `commandKey`, `commandPrefix`, `maxCommandID`, `pasteTexts`, `toAgent`, `lastCtrlC`, `agentIn`, `queueKey`, `sendKey`, `keyAnswered`, `keepChat`, `reopenChat`, `chatJustShut`, `chatShutHold`, `chatTypingHold`, `menuQuiet`, `menuSeen`, `seenOf`, `noteMenu`, `menuWait`, `atMenu`, `chatHushed`, `askedAgain`, `startsDigit`, `intoPane`, `ctrlCWindow`, `ctrlCLapsed`, `lapseCtrlC`, `ctrlCMark`, `openChat`, `chatOn`, `chatTarget`, `tooShort`, `chatSeen`. Keep the watch, size dwell (`sizeDwell`, `sizeDueMsg`, `chatSize`, `watchCmd` without its `chatKeys` term) and every drawing function. Keep in `liveview_test.go` only the drawing and watch tests (`TestFit*`, `TestScreenOfAnotherPaneIsIgnored`, `TestSelectionChange*`, `TestLostConnectionForgetsTheWatch`, `TestChatLines*`, `TestRefused*Watch*`, `TestRefusalOfAnOldWatchHoldsNothing`, `TestWideStacksStatsLeft`, `TestMediumSplitsListAndChat`, `TestTheChatStrip*`, `TestAShortDeskKeepsItsStats`, `TestAGoneScreenSaysTheSessionEnded`, `TestTheMoreHintNeverCoversTheSelection`, `TestACutListSaysHowManyAreBelow`, `TestTheScreenDropsLinksAndOtherControls`, `TestAFailedReadKeepsTheChat`, `TestKeyNames`); the rest go.

Layout: one column (a phone) draws no live view outside the demo: Enter goes into the real pane. Beside the list the view keeps `chatBeside`; where its question has no room (`chatNone`), the opened row's question card shows instead of a refusal.

- [ ] **Step 1: Write the failing tests (T6, T7, T8, T10, T14, T23 live)**

```go
// liveview_test.go
func TestTheLiveViewTakesNoKeys(t *testing.T) { // T6
	m, f := loadedWith(t, 100, fixtureState())
	for _, k := range []string{"2", "y", "esc", "tab", "backspace", "space", "ctrl+a", "m", "up", "down"} {
		next, cmd := key(m, k)
		drain(cmd)
		m = next.(Model)
	}
	for _, a := range f.acts() {
		if strings.HasPrefix(a.RecID, proto.KeysPrefix) {
			t.Errorf("a key reached the agent: %+v", a)
		}
	}
}
func TestTheLiveViewNamesTheWayIn(t *testing.T) {
	m := loaded(t, 100)
	if out := ansi.Strip(m.render()); !strings.Contains(out, "enter goes into app-feature") {
		t.Errorf("no way in on the key line:\n%s", out)
	}
}
func TestTheStandInIsTheDemosOnly(t *testing.T) { // T7
	m, f := loadedWith(t, 100, fixtureState())
	m.tab = tabSessions
	if nm := m.openStandIn("%1"); nm.standIn {
		t.Fatal("the stand-in opened outside the demo")
	}
	m.standIn = true // even forced, it forwards nothing
	_, cmd := m.standInKey(keyPress("2"))
	drain(cmd)
	if len(f.acts()) != 0 {
		t.Errorf("the stand-in sent %+v outside the demo", f.acts())
	}
}
func TestTheWheelScrollsTheLiveViewBack(t *testing.T) { // T14
	// a wheel over the view beside the list sends exactly one act, keys:%1 Text proto.WheelUp, and opens nothing (standIn false)
}

// demo_test.go
func TestEnterInTheDemoOpensTheStandIn(t *testing.T) {
	// demoSessions(t, 44): Enter on flint-auth → standIn, row 0 starts "‹ Sessions", key line contains "type 2 to answer"
}
func TestTheStandInsBackKeyReturnsToTheNextWait(t *testing.T) {
	// standIn on flint-auth; Update(uv.UnknownApcEvent(homeKey)) → !standIn, selection is the next waiting session (nextAfter)
}

// internal/demo/script_test.go: openQuestion presses "\r" in place of "a" and waits for "type 2 to answer"
func TestTheStandInAnswersWithADigit(t *testing.T) { // T8: rewrites TestAnAnswerEndsInDone
	d, term := openQuestion(t)
	term.send(t, "2")
	term.waitFor(t, "I rotate the token on each refresh")
	eventually(t, 6*time.Second, "Done", d.Done)
}
func TestTheStandInAnswersWithArrowsThenEnter(t *testing.T) { // T8
	d, term := openQuestion(t)
	term.send(t, "\x1b[B") // Down: the cursor moves to 2. Rotate, keep
	term.send(t, "\r")
	term.waitFor(t, "I rotate the token on each refresh")
	eventually(t, 6*time.Second, "Done", d.Done)
}
func TestTheStandInGoesBackToSessions(t *testing.T) { // T8: send "\x1b_matchblox\x1b\\" → waitFor "WAITS FOR YOU"
}
// world_test.go
func TestArrowsMoveTheMenusCursor(t *testing.T) {
	// at flint-auth's menu: Run(tmux send-keys -t %N Down) → highlighted == "2"; Up → "1"; Up at the top stays "1"
}

// home_linux_test.go: TestCommandKeyReachesTheConsoleAtHome and TestCommandKeySkipsADeadConsole go;
func TestCommandKeyTypesIntoAMarkedAgentPane(t *testing.T) { // T10, T23
	// pane in "work" runs cat -v into a file; tmux set-option -p -t <pane> @matchblox_agent claude; CommandKeysArgv;
	// type keycmd.Sequence("compact") into a client there → the file holds "/compact" and no Return
}
func TestCommandKeyDoesNothingInAnotherAgentsPane(t *testing.T) { // T10: @matchblox_agent codex, Sequence("compact") → file empty
}
// TestCommandKeyDoesNothingElsewhere stays: no @matchblox_agent → nothing typed.
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/app ./internal/demo ./internal/panes -run 'LiveView|StandIn|WheelScrollsTheLiveView|Arrows|CommandKey' -v`
Expected: FAIL, `undefined: openStandIn`, `m.standIn undefined`, keys still reach the chat.

- [ ] **Step 3: Implement**

Rename with `git mv` first, so the history follows. Cut the key code listed above; turn `chatKeys` into `standIn` only where the full-screen mode is drawn or keyed (`view.go:197`, `chatSize`'s full case, `focus`'s layer for the stand-in). `Update`'s APC case: `homeKey` → `leaveStandIn` when `standIn`; a stand-in key goes to `standInKey` from `key`. `wheel` over the view calls `wheelView`. `docs/keys.md` and `docs/console.md` lose "The chat" sections and gain a short "The live view" section: the selected agent's screen beside the list, read-only, the wheel scrolls it back, `enter goes into <name>`; and the jump keys `⏎`, `a`, `→`, `ctrl+o` (help `go in`).

- [ ] **Step 4: Run the Go tests and the docs check**

Run: `go test ./internal/... ./cmd/...`
Expected: PASS, including `TestTheDocsNameEveryKeyAndOnlyRealOnes` (no doc names `^C` as the chat's stop any more).

- [ ] **Step 5: Commit**

```bash
git add -A internal/app internal/panes internal/demo docs/keys.md docs/console.md
git commit -S -m "feat(console)!: the chat becomes a read-only live view; the demo gets a stand-in for the pane (#334)" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: The service sends keys only to the demo's World

**Files:**
- Modify: `internal/service/keys.go:59-66` (`keys`), `internal/service/watch.go:17,38-75,88-91`, `internal/service/hands.go:18,210-240` (`command`), `internal/service/actions.go:116,149-157`, `internal/service/service.go` (field), `internal/proto/proto.go:149-152` (`CommandPrefix`)
- Modify: `internal/demo/world.go:117-129` (`Wire`)
- Test: `internal/service/keys_test.go`, `internal/service/scrollback_test.go:103`, `internal/service/watch_test.go`, `internal/demo/world_test.go`

**Interfaces:**
- Consumes: `World.Wire(s *service.Service)`.
- Produces (new): `Service.KeysToWorld bool` (set only by `World.Wire`); `const keysOnlyInDemo = "not sent: keys go to an agent in its own pane"`; `const watchOnlyFromConsole = "screens go only to a console"` (replaces `keysOnlyFromConsole` for the watch act). Deleted: `keysOnlyFromConsole`, the `command` act and `proto.CommandPrefix` (their only sender, the chat's `commandKey`, went in Task 5).

- [ ] **Step 1: Write the failing tests (T25, T14 service half, T8 wiring)**

```go
// keys_test.go
func TestKeysAreRefusedOnTheRealService(t *testing.T) { // T25
	s, ran := keysTest(t)
	s.KeysToWorld = false // the real service
	res := s.act(t.Context(), key("Enter"))
	if res.Err != keysOnlyInDemo || len(*ran) != 0 {
		t.Errorf("err %q ran %q", res.Err, *ran)
	}
}
// keysTest sets s.KeysToWorld = true, so TestKeysSendsANamedKey and the rest keep testing the demo's path;
// TestKeysOnlyFromAConsole keeps its aConsole check.

// scrollback_test.go: TestAPaneThatReadsTheMouseStillGetsTheWheel becomes
func TestTheWheelScrollsBackEvenAPaneThatReadsTheMouse(t *testing.T) { // T14
	// PaneMouse says the pane reads the mouse; a WheelUp keys act is handled by scrollKey: watch back == scrollStep, nothing ran
}

// internal/demo/world_test.go
func TestTheDemosServiceSendsKeysToTheWorld(t *testing.T) { // T25, T8
	// after World.Wire, s.KeysToWorld; a keys:%N Text "2" act at flint-auth's menu picks option 2 (World.Ran has the send-keys)
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/service ./internal/demo -run 'RefusedOnTheRealService|EvenAPaneThatReadsTheMouse|SendsKeysToTheWorld' -v`
Expected: FAIL, `undefined: keysOnlyInDemo`.

- [ ] **Step 3: Implement**

`keys` returns `keysOnlyInDemo` before any other check unless `s.KeysToWorld`. `scrollKey` drops its `PaneMouse` branch, so a wheel always moves through `CaptureBack`. `watchAct` uses `watchOnlyFromConsole`. Delete `command`, its `paneActs` entry, its `actions.go` case and `proto.CommandPrefix`. `World.Wire` sets `s.KeysToWorld = true`.

- [ ] **Step 4: Run the Go tests**

Run: `go test ./internal/... ./cmd/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service internal/proto internal/demo
git commit -S -m "feat(service): keys reach a pane only in the demo's world (#334)" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: Pairing ends on the new device's first jump

**Files:**
- Modify: `internal/app/pairpage.go:370` (beat 3 line), `:380-416` (`pairActed`: the `keys` verb goes), new `pairJumped`
- Modify: `internal/app/model.go` (`ranMsg` case calls `pairJumped`)
- Test: `internal/app/pairpage_test.go:624,663,749,776-803`

**Interfaces:**
- Consumes: `ranMsg.tty`, `ranMsg.jumped` (Task 4), `PairHost.DeviceIn`, `m.sessionIn`.
- Produces (new): `func (m Model) pairJumped(tty, pane string) Model` — at beat 3, when `tty` is the new device's `DeviceIn` tty and the pane is a home session's: beat 4, line `<device> went into <session>`, fold at `pairActFold`.

- [ ] **Step 1: Write the failing tests (T22)**

```go
func TestPairingSaysGoIntoAnAgent(t *testing.T) {
	// beat 2 → 3 as today: m.pair.line == "iPad is in: go into an agent from it, or esc to go back."
}
func TestPairingEndsOnTheDevicesFirstJump(t *testing.T) { // T22
	// beat 3, DeviceIn("iPad-id") tty "/dev/pts/7"; Update(ranMsg{jumped: "%2", tty: "/dev/pts/7"})
	// → beat 4, line "iPad went into api"; ranMsg with tty "/dev/pts/1" (the desk) leaves beat 3
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/app -run 'Pairing' -v`
Expected: FAIL (the old line, no `pairJumped`).

- [ ] **Step 3: Implement** the line, `pairJumped`, its call on a successful `ranMsg` with `jumped != ""`; drop the `keys` case of `pairActed`. Rewrite the tests at `pairpage_test.go:624-803` that expect `iPad picked in api` or `Answer an agent from it`.

- [ ] **Step 4: Run the package**

Run: `go test ./internal/app/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app
git commit -S -m "feat(pair): the pair page ends on the new device's first jump (#334)" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: The app marks its own keys and drives the real pane

**Files:**
- Create: `ios/MatchbloxKit/Sources/MatchbloxKit/InputMark.swift`, `ios/MatchbloxKit/Tests/MatchbloxKitTests/InputMarkTests.swift`
- Modify: `ios/MatchbloxKit/Sources/MatchbloxKit/AgentSignal.swift` (add `ConsoleSignal`), `ios/MatchbloxKit/Tests/MatchbloxKitTests/AgentSignalTests.swift`
- Modify: `ios/Matchblox/ConsoleView.swift:183` (`onAgent`), `:243-330` (`Belts`: `chatShows` → `paneShows`), `:415-421`, `:632-648` (`serve`), `:700-705` (`send(source:data:)`)
- Modify: `ios/MatchbloxUITests/DemoConsoleTests.swift:64-80,200-212`

**Interfaces:**
- Consumes: the console's `AgentSignal` bytes as `AgentSignalFilter.feed` returns them (`agent: String?`), `keycmd.MarkKey` (Task 2), `HomeKey.labelColumns`, `ConsoleTerminalView.inTmuxPane` (`ConsoleTerminalView.swift:275`).
- Produces (new):
  - `public enum InputMark { public static let bytes: [UInt8]; public static func marks(_ data: ArraySlice<UInt8>) -> Bool }` — `bytes` = `"\u{1b}_matchblox;mark\u{1b}\\"`; `marks` is true for a CR (0x0d) or a left-button press (SGR `ESC [ < 0 ; x ; y M`, classic `ESC [ M` with button byte 32).
  - `public struct ConsoleSignal: Equatable, Sendable { public let agent: String; public let pane: String?; public let marks: Bool; public init(_ raw: String) }` — parses `id[;pane=<p>][;marks]`.
  - `public struct PaneAgent: Sendable { public static let window: Duration /* 1 s */; public init(); public mutating func marked(at: ContinuousClock.Instant); public mutating func signal(_ s: ConsoleSignal, at: ContinuousClock.Instant); public mutating func left(); public var agent: String? { get } }` — takes an agent only from a signal with a pane that comes within `window` after its own mark, once per mark; `left()` (back on the console) drops it.

- [ ] **Step 1: Write the failing tests (T15, T16, T20 app half)**

```swift
final class InputMarkTests: XCTestCase {
    func testAReturnAndAPressAreMarked() {
        XCTAssertTrue(InputMark.marks(ArraySlice([0x0d])))
        XCTAssertTrue(InputMark.marks(ArraySlice(Array("\u{1b}[<0;20;6M".utf8))))
        XCTAssertTrue(InputMark.marks(ArraySlice([0x1b, 0x5b, 0x4d, 32, 52, 38])))
    }
    func testOtherBytesAreNot() {
        for other in ["a", "2", "\u{1b}[A", "\u{1b}[<0;20;6m", "\u{1b}[<64;20;6M", "\u{1b}_matchblox\u{1b}\\"] {
            XCTAssertFalse(InputMark.marks(ArraySlice(Array(other.utf8))), other)
        }
    }
    func testTheMarkIsTheConsolesKey() {
        XCTAssertEqual(InputMark.bytes, Array("\u{1b}_matchblox;mark\u{1b}\\".utf8)) // keycmd.MarkKey
    }
    func testAnOwnJumpKeepsItsAgent() { // T15
        var p = PaneAgent(); let t0 = ContinuousClock.now
        p.marked(at: t0)
        p.signal(ConsoleSignal("claude;pane=%12"), at: t0 + .milliseconds(200))
        XCTAssertEqual(p.agent, "claude")
        p.left()
        XCTAssertNil(p.agent)
    }
    func testAnotherScreensJumpIsIgnored() { // T15: no mark of ours before it
        var p = PaneAgent(); let t0 = ContinuousClock.now
        p.signal(ConsoleSignal("codex;pane=%3"), at: t0)
        XCTAssertNil(p.agent)
        p.marked(at: t0); p.signal(ConsoleSignal("claude;pane=%12"), at: t0 + .milliseconds(100))
        p.signal(ConsoleSignal("codex;pane=%3"), at: t0 + .milliseconds(150)) // the mark is spent
        XCTAssertEqual(p.agent, "claude")
    }
    func testALateSignalIsIgnored() {
        var p = PaneAgent(); let t0 = ContinuousClock.now
        p.marked(at: t0); p.signal(ConsoleSignal("claude;pane=%12"), at: t0 + .milliseconds(1001))
        XCTAssertNil(p.agent)
    }
}
// AgentSignalTests
func testTheSignalNamesItsPaneAndTheMarks() {
    XCTAssertEqual(ConsoleSignal("claude;pane=%12"), ConsoleSignal(agent: "claude", pane: "%12", marks: false)) // add a memberwise init for tests, or compare fields
    XCTAssertTrue(ConsoleSignal(";marks").marks)
    XCTAssertEqual(ConsoleSignal(";marks").agent, "")
    XCTAssertFalse(ConsoleSignal("claude").marks) // an old console: no marks, so the app never marks (T20)
}
```

- [ ] **Step 2: Run them to see them fail (on a Mac)**

Run: `make -C ios test` (that is `swift test --package-path ios/MatchbloxKit`)
Expected: FAIL, `cannot find 'InputMark' in scope`. Not runnable on this Linux host: no `swift`.

- [ ] **Step 3: Implement**

- `InputMark`, `ConsoleSignal`, `PaneAgent` in MatchbloxKit.
- `ConsoleConnection`: `private var consoleMarks = false` and `private var paneAgent = PaneAgent()`, both reset in `serve`. Each signal from the filter is parsed with `ConsoleSignal`; `marks` sets `consoleMarks`; a signal with a pane feeds `paneAgent`; `onAgent(paneAgent.agent)`.
- `send(source:data:)`: when `consoleMarks`, `terminal.inTmuxPane == false` and `InputMark.marks(data)`, yield `InputMark.bytes` first and call `paneAgent.marked(at: .now)`; then yield `data` as today. This covers the soft and the hardware Return, belt keys (they reach `send` through `TerminalView.send`) and SwiftTerm's taps.
- `Belts`: `chatShows` becomes `paneShows` = `!(agent ?? "").isEmpty && terminal?.inTmuxPane == true`; the doc comment says commands run in a known agent's pane. When the terminal leaves the pane (`inTmuxPane` false after true, checked in `outputArrived`), the connection calls `paneAgent.left()`.

- [ ] **Step 4: Rewrite the demo UI test for the belt path (T17)**

`answerTheAgent(in:)`: `tapRow(in: app, containing: "flint-auth", until: "type 2 to answer")`; tap `app.buttons["Show keyboard"].firstMatch`; `app.typeText("2")`; expect `I rotate the token on each refresh`; the invite heading `Your agents wait the same way.` follows. Add `testTheBeltsArrowsAndReturnAnswer` (tap `Down`, then `Return`, same expectations) and `testBackToMatchbloxReturnsToSessions` (open the stand-in, tap `Back to matchblox`, expect `WAITS FOR YOU`).

- [ ] **Step 5: Run the iOS tests (on a Mac)**

Run: `make -C ios test && make -C ios uitest DESTINATION='platform=iOS Simulator,name=iPhone 16'`
Expected: PASS. On this Linux host, say in the PR that the Swift tests ran on a Mac (or in the `ios.yml` workflow), not here.

- [ ] **Step 6: Commit**

```bash
git add ios
git commit -S -m "feat(ios): the app marks its own keys, and its command keys type into the agent's pane (#334)" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 9: The docs say Go straight in

**Files:**
- Modify: `README.md:77-81`, `docs/phone.md:244-344` ("Read and steer an agent", "The key belt", "Command keys", "Go to an agent's pane and back"), `CHANGELOG.md:7` (Unreleased)

**Interfaces:**
- Consumes: the behavior of Tasks 3-8; the strings of Global Constraints.

- [ ] **Step 1: Rewrite README "Steer without leaving" as "Go straight in"**

Text to carry: `Enter` goes into the agent's own tmux pane, on this host or another, and moves only your screen. Sessions shows the selected agent's live screen read-only beside the list. `prefix m` comes back, with the next waiting agent selected.

- [ ] **Step 2: Rewrite `docs/phone.md`**

"Read and steer an agent" becomes "Go into an agent": a tap or Return on a row goes into its pane on the phone only; the belt's keys (Esc, Ctrl, arrows, Tab, the prefix) reach the agent unchanged; command keys light in a known agent's pane and type their command there; `‹`/◂ comes back. Delete the chat's menu and paste rules. Say that the demo shows a stand-in for the pane, answered with the belt (`type 2 to answer`).

- [ ] **Step 3: Add the CHANGELOG entry under `## [Unreleased]`**

```markdown
### Changed

- **Enter goes into the pane.** `Enter`, `a`, `→`, `ctrl+o` and a tap go into
  the agent's own tmux pane, on every host and device, and move only the
  screen that asked. Sessions shows the selected agent's live screen,
  read-only; the wheel scrolls it back. The phone's command keys type into
  the agent's pane. The key-giving chat is gone; the demo shows a stand-in
  for the pane. ([#334](https://github.com/blox-eng/matchblox/issues/334))
```

- [ ] **Step 4: Run the docs checks and the whole suite (T18)**

Run: `go test ./internal/app -run TestTheDocsNameEveryKeyAndOnlyRealOnes ./cmd/matchblox -run TestTheDocsShowOnlyRealCommands && scripts/validate.sh`
Expected: PASS, then `OK`. Also `rg -n "gives it the keys|The chat|chat has the keys" README.md docs --glob '!docs/superpowers/**'` prints nothing.

- [ ] **Step 5: Commit**

```bash
git add README.md docs/phone.md CHANGELOG.md
git commit -S -m "docs: Enter goes straight into the agent's pane (#334)" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 10: Tell #312's session (needs the owner's choice of channel first)

**Blocked until the owner picks the channel.** Ask: "Tell PR #312's session (matchblox-ux) through a chain and `matchblox send`, or as a comment on PR #312?" Do not send before the answer.

- [ ] **Step 1: Ask the owner for the channel and wait for the answer**

- [ ] **Step 2: Send this message on the chosen channel**

> #334 (Enter goes into the pane) lands on main first (eng review D3). It deletes the chat's key code in `internal/app/chat.go` (renamed `liveview.go`), `menutap.go`, the service's `cmd:` act and, outside the demo, the `keys:` act, and the Stoker take-over's hold for chat keys (`stoker.go`, the `chatKeys` branches of `asksTakeOver` and `hold`). `model.go`, `mouse.go` and `keys.go` change too. Whichever PR lands second rebases; #312's stoker family keeps its own behavior, and any chat-key path it adds has no home after #334.

Chain route: `matchblox chain` with the matchblox-ux session, then `matchblox send matchblox-ux "<message>"`. Comment route: `gh pr comment 312 --body "<message>"`.

- [ ] **Step 3: Note in the #334 PR body where and when #312's session was told**

---

## Self-review notes

- Spec coverage: goals 1 (Tasks 3, 4), 2 (Tasks 1, 2, 4, 8), 3 (Task 5), 4 (Tasks 4, 5), 5 (Tasks 2, 5, 8), 6 (Tasks 5, 6, 8), 7 (Task 3); the deletions (Tasks 5, 6); docs (Tasks 5, 9); order with #312 (Task 10). D1 (Task 5), D2 (Tasks 2, 4, 8), D3 (Task 10), D4 (Task 4). Outside voice 1 (Task 6), 2 (Task 2), 3 (Tasks 3, 4: the action is built at key time), 4 (Task 8), 5 (Tasks 1, 4), 6 (Task 7), 8 (Tasks 2, 4, 5).
- T1 T11 T12 T13: Task 3. T2 T4 T9 T21 T24, T15 Go: Task 4. T3 T19 T20 Go: Tasks 1, 2. T5: Tasks 1, 4. T6 T7 T8 T10 T14 T23: Tasks 5, 6. T16 T15 T17 T20 app: Task 8. T18: Tasks 5, 9. T22: Task 7. T25: Task 6.
- Calls this plan makes where the spec leaves them to the builder: a home-host jump runs at once (no confirm), another host's keeps its confirm; `→` on a pane-less row says the same as Enter; one column shows no live view outside the demo; the command binding types the text without Return; the marks announce rides the graphics viewers poll, not a tmux hook; the `cmd:` act is deleted with its only sender.
