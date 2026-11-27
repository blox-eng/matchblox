Spec: docs/superpowers/specs/2026-11-27-one-match-head-design.md
Spec approved: 2026-11-27
Test command: go vet ./... && go test -race ./... (host, from a shell outside tmux with SSH_AUTH_SOCK unset) · swift test --package-path ios/MatchbloxKit and make -C ios build (macOS only: the `iOS` workflow on the pushed branch; this host has no Swift toolchain) · make -C ios uitest DESTINATION="id=<udid>" (the `iOS demo` workflow)
Plan approved: 2026-11-27 by the owner (Tasks 1–16); revision 2 (Tasks 17–27): 2026-11-27 by the owner
Next: builder:build

# Plan: the owner answers an agent with one thumb on one match head

What exists (read 2026-11-27 on origin/main fe4e332d, after #508 merged):
- `MatchHead` (`ios/MatchbloxKit/Sources/MatchbloxKit/MatchHead.swift:6`) rolls on `dy` with speed-based lines (1…10, `lines(atSpeed:)` :72), heat and one strike. `HeadClick.swift` decides a click (still 0.3 s, slop 6 pt). `HeadRoute.swift` routes lines to `.wheel/.scrollback/.end/.nothing`. Tests: `MatchHeadTests`, `HeadClickTests`, `HeadRouteTests`.
- The head is a belt key `KeyBar.wheel = "Wheel"` (`KeyBar.swift:9`), only in Mine through the Scroll group (`BeltLayouts.swift:27,75`). `BeltView` draws it with `HeadLayer` (`BeltView.swift:483-492`) and drives it with `HeadDriver`. `HeadFeel` (CoreHaptics, `HeadView.swift:116-176`) plays notch, strike, end, click.
- The head called up under the finger is already gone (commit 6e4dc95c, #475). Leftovers: `HeadDriver.swift:4` comment, `canClick` (:54-62, always true), `HeadDriver.seat()` and `HeadFeel.seat()` with no callers, `docs/phone.md:459-460`.
- Lines reach the screen through `terminal.head(lines:)` (`ConsoleTerminalView.swift:589`): wheel steps (5 lines a step in tmux) or SwiftTerm scrollback. Enter goes `Belts.send` → `terminal.send` after `jumpToBottom()` (`ConsoleView.swift:508-518`).
- The dock: `Belts.dock` sits under the terminal in a `UIStackView`; `Belts.lifted` rides the keyboard as `inputAccessoryView`; `focusChanged` shows one of them (`ConsoleView.swift:185,214-216,381-382,496-505`). `BeltGeometry` height 60, keys 44 (`Belt.swift:14-32`).
- The phone gets no menu data. It gets terminal bytes and two APC signals (`agent=`, `board=`) that `AgentSignalFilter` strips (`AgentSignal.swift`). The host's `board=` goes from `tellBelt` (`internal/app/boardkeys.go:608`) to every console viewer, and only while the console shows; in an agent's tmux pane, where the owner answers most, no host signal reaches the phone. `menu.Read` (`internal/menu/menu.go:31`) reads text only and drops the `❯` mark. The phone already holds the same screen text in SwiftTerm.
- Session names reach the phone only as `IslandNames` (a number→name map from registration, stale, no current session, empty in the demo). A session opens with `matchblox console --open <n>` (`OpenLink.swift:36`, `cmd/matchblox/gate.go:139`). `SSHSession.open(command:pty:)` (`SSHSession.swift:37`) runs a host command with no pty. `matchblox sessions --json` lists sessions with `name`, `word`, `status`, and no `n`.
- Home (`HostsView.swift`): `region(_:content:)`; LIVE ACTIVITY is inside `if !realHosts.isEmpty` (:81-93), so the demo has none; the text − + row follows at :95; `TextSize` stores under `console.text-size`.
- The checked pick: `service.choose` (`internal/service/choose.go:59`) takes a `choose:<pane>` act with `Confirm: "y"` and `digit\x00text\x00sign`, re-reads the live screen under the pane's lock, and types the digit only when `menu.Read` and `menu.Sign` still match; its refusal is `<name>'s menu changed — look again`. It needs `a.Console`, which the service sets only for `ClientConsole` hellos and never inside an agent's pane (`service.go:1241`, `aConsole` in `watch.go:24`). Today only the console's board sends it (`internal/app/boardkeys.go:191`). `send --pick` is another path: it answers the transcript's question tool (`pickAct`, `cli.go:840`).
- `peek` reads the transcript (`internal/stoker/peek.go`), not the screen.
- The iOS demo is one byte stream through `cmd/mbdemolib` (`MBDemoStart`, `MBDemoWrite/Read`; `ios/Matchblox/DemoSession.swift`). It has no command channel.
- VoiceOver: the head is `.adjustable`, Enter on double-tap (`BeltView.swift:1022-1072`), name "Match head" (`KeyFace.swift:25`).

## File map

Host (Go):
- `internal/menu/menu.go` (change) — `Chosen(screen) int`: the number of the option the menu marks, 0 when none.
- `internal/menu/menu_test.go`, `testdata/menus/marked-*.txt` (change, create) — tests and fixtures of `Chosen`.
- `internal/proto/proto.go` (change) — `ClientPocket`; `MenuPrefix` (`menu:<pane>`), a read act; its reply `Menu{Options []Option, Chosen int, Sign string}`.
- `internal/service/menu.go` (create) — answers `menu:<pane>` from `liveScreen` with `menu.Read`, `menu.Chosen` and `menu.Sign`.
- `internal/service/service.go` (change) — a `ClientPocket` hello counts as a console for `choose:` and `menu:` only, with the same pane checks.
- `cmd/matchblox/menu.go` (create) — `matchblox menu <who> --json` and `matchblox choose <who> <n> --text <t> --sign <s>`, both saying hello as `ClientPocket`.
- `cmd/matchblox/` sessions printer (change) — each `sessions --json` row gains `n`, the number `console --open <n>` takes.
- `internal/demo/*` (change) — the demo world asks one question with a three-option menu, has three sessions, and long output with `✦` steps.
- `cmd/mbdemolib/main.go` (change) — `MBDemoRun(cmd) -> stdout`: runs `menu`, `choose` and `sessions --json` against the demo world.

MatchbloxKit (pure, tested):
- `MatchHead.swift` (change) — the turn: angle to notches (15°), pace by notches since the crossing (1/3/8/20 at 10/24/48), 0.35 s pause reset, one strike a roll, heat from the pace.
- `HeadZone.swift` (create) — a touch point to `.middle` (inside 48% of the radius) or `.edge(mark)` with the nearest of top/left/right/bottom; `isPress(moved:)` under 10 pt.
- `HeadClick.swift` (change) — the middle's lift: under 0.35 s `.enter`, 0.35–0.6 s `.nothing`, from 0.6 s `.hold`.
- `HeadWalk.swift` (create) — owns one roll: lines up, notches since the crossing, the last menu seen; one notch gives `.highlight(i)/.wait/.cross/.scroll(lines)/.home/.bump(reason)`; a tap gives `.pick(n, text, sign)`, `.enter` or `.home`.
- `HeadMarks.swift` (create) — the five settings (top, left, right, bottom, hold) as key ids, the first marks, undo, as it came, `^C` refused, Codable under `head.marks`.
- `HeadHint.swift` (create) — the handle row's line for each head state.
- `StepMark.swift` (create) — the nearest `✦` or `⏺` line at the top of the visible lines.
- `MenuSight.swift` (create) — decode `menu <who> --json` into `MenuSight(options, chosen, sign)`; strict.
- `HostCommand.swift` (create) — one way to run a host command: `SSHSession.open(command:pty: nil)` on a host, `MBDemoRun` in the demo.
- `SessionList.swift` (create) — decode `sessions --json` rows into `(n, name, word)`.
- `BeltLayouts.swift` (change) — no Scroll group, no Wheel key; `Mine` drops a stored `Wheel` on load.
- `KeyBar.swift`, `KeyFace.swift` (change) — no `wheel`; the words of each mark key.
- `Tests/MatchbloxKitTests/*` (change/create) — one test file for each file above.

App:
- `ios/Matchblox/DockHeadView.swift` (create) — the big head over the belt in `Belts.dock`: ring and marks on touch, zones, hold ring, sessions card, step card; draws with `HeadLayer`, plays `HeadFeel`.
- `ios/Matchblox/HeadDriver.swift` (change) — drives the turn from angles; leftovers cut.
- `ios/Matchblox/HeadView.swift` (change) — `HeadFeel.cross()` and `bump()`; `seat()` cut.
- `ios/Matchblox/BeltView.swift` (change) — no Wheel cell; the belt comes in front while dragged and slips back 1.4 s after it stops.
- `ios/Matchblox/ConsoleView.swift` (change) — dock height grows by the head; the keyboard folds after a keyboard Enter; the sessions card opens `--open N`.
- `ios/Matchblox/BeltEditor.swift` (change) — no Scroll group.
- `ios/Matchblox/HostsView.swift` (change, on top of #508) — region `THE HEAD`: drawing, list, undo, as it came, its colour.
- `ios/MatchbloxKit/Sources/MatchbloxKit/HeadPigment.swift` (create) — the five colours of the cold head, their names, the stored key `head.colour`, and match red for an unknown or missing value.
- `ios/MatchbloxKit/Tests/MatchbloxKitTests/HeadPigmentTests.swift` (create) — the tests of `HeadPigment`.
- `ios/Matchblox/HeadView.swift` (change) — `HeadLayer` draws the real material: the pigment, the grain, the touch of depth; no gloss.
- `ios/MatchbloxUITests/HeadTests.swift` (create) — UI tests on the demo console.
- `docs/phone.md`, `docs/keys.md`, `DESIGN.md`, `www/index.html` (change) — one head.

## Tasks

PR #508 (Pocket home) merged as e62b70c1. Branch `build/one-match-head` from main. Swift tasks prove their tests on the `iOS` workflow of the pushed branch; each Swift task's Run line names the test, and the build pushes once per task (see Calls).

### Task 1: the host reads the live menu, its mark and its sign
- Files: `internal/menu/menu.go`, `internal/menu/menu_test.go`, `testdata/menus/marked-*.txt`, `internal/proto/proto.go`, `internal/service/menu.go`, its test.
- Test first: `TestChosen` — fixtures: `❯ 2.` of 3 → 2; `› 1.` of 4 → 1; `> 3.` → 3; a wrapped option; a numbered list in a reply with text under it → 0; `❯ 9.` inside an option's text → not the mark. `TestMenuActReadsLiveScreen` — a pane whose `ScreenOf` holds a 3-option menu answers `Menu{Options: 3, Chosen: 2, Sign: menu.Sign(screen)}` with option texts equal to `menu.Read`'s; no menu → an empty `Menu`; an unreadable pane → an error.
- Run: `go test ./internal/menu/ ./internal/service/ -run 'TestChosen|TestMenuAct'` — fails: `undefined: Chosen`; then passes.
- Do: capture the mark in `optionLine`; `Chosen` uses the last-run rule of `Read` (which stays as it is). `menu:` is a read act: it types nothing.
- Done when: both tests pass and `TestRead*` still pass.
- Commit: `feat(service): read a pane's live menu, its mark and its sign`

### Task 2: Pocket picks through the checked path
- Files: `internal/proto/proto.go`, `internal/service/service.go`, `cmd/matchblox/menu.go`, their tests.
- Test first: `TestPocketChooses` — a `ClientPocket` act `choose:<pane>` with `2\x00<text>\x00<sign>` and `Confirm: "y"` types `2`. `TestPocketChooseStale` (Surface: an agent's tools) — an older sign types nothing and returns `<name>'s menu changed — look again`. `TestPocketHelloFromAgentPaneRefused` (Surface) — the same act whose `From` is an agent's pane, or whose PID is in an agent's tree, is refused with `only the console picks from a menu`. `TestPocketIsNoConsoleElsewhere` — a `ClientPocket` `message:` act is not a console act. CLI: `TestChooseCommand` — `matchblox choose s 2 --text "Yes" --sign abc` builds that act; `TestMenuCommandJSON` — `matchblox menu s --json` prints `{"options":[{"n":1,"text":"Yes"},…],"chosen":2,"sign":"…"}`.
- Run: `go test -race ./internal/service/ ./cmd/matchblox/ -run 'TestPocket|TestChooseCommand|TestMenuCommandJSON'` — fails: no `ClientPocket`; then passes.
- Do: at `service.go:1241`, `a.Console` is also true for `ClientPocket` when `a.RecID` starts with `choose:` or `menu:`, with the same `agentsPane` check. The two commands say hello as `ClientPocket`. The handle row's refusal text on the phone is `the question changed · roll again` (Task 9 maps the service's words).
- Done when: the tests pass, and `go vet ./... && go test -race ./...` passes.
- Commit: `feat(cli): matchblox menu and choose, for Pocket's head`
- Surface: an agent's tools.

### Task 3: sessions name their open number, and the demo answers commands
- Files: the `sessions --json` printer in `cmd/matchblox/`, `internal/demo/*`, `cmd/mbdemolib/main.go`, their tests; `MenuSight.swift`, `SessionList.swift`, `HostCommand.swift`, their tests; `ios/Matchblox/DemoSession.swift`.
- Test first: Go — `TestSessionsJSONHasOpenNumber` (each row's `n` is the number the service focuses for `console --open <n>`, through the same lookup `gateFocus` uses); `TestDemoShowsAMenu` (3 options); `TestDemoHasThreeSessions`; `TestDemoRun` (`MBDemoRun("menu 1 --json")` gives the demo menu; `MBDemoRun("choose 1 2 --text … --sign …")` answers it; a stale sign is refused; `MBDemoRun("sessions --json")` gives three rows; `MBDemoRun("rm -rf /")` gives `not a demo command`). Swift — `testMenuSight` (3 options, chosen 2, a sign); `testMenuHostile` (chosen 0 or 4 of 3, 12 options, a missing sign, an option over 500 characters → nil); `testSessionRows` (no `n`, or a name over 200 characters, is dropped).
- Run: `go test ./cmd/matchblox/ ./internal/demo/ ./cmd/mbdemolib/ -run 'TestSessionsJSONHasOpenNumber|TestDemo'` and `swift test --package-path ios/MatchbloxKit --filter 'MenuSightTests|SessionListTests'` — fail; then pass.
- Do: add `n`; a demo turn that asks with a three-option menu and has `✦` steps in its output; three demo sessions; `MBDemoRun` takes only the three commands; `HostCommand` calls it in the demo and `SSHSession.open(command:pty: nil)` on a host.
- Done when: the tests pass.
- Commit: `feat(demo): the demo answers Pocket's menu, choose and sessions`
- Surface: an agent's tools (`testMenuHostile`, `MBDemoRun` refuses other commands).

### Task 3a: the phone's key runs the head's three commands
- Files: `cmd/matchblox/gate.go`, `cmd/matchblox/gate_test.go`, `cmd/matchblox/menu.go`, `cmd/matchblox/menu_test.go`.
- Surface: an agent's tools, and the phone's key.
- Test first (hostile): `TestGatePhoneHeadCommandsOnly` — in phone mode, `matchblox menu %2 --json`, `matchblox choose %2 2 --text 'Yes' --sign 0a1b`, `matchblox sessions --json` pass as exactly those argv; refused: `matchblox menu %2 --json; rm -rf /`, `matchblox menu $(id) --json`, an open quote, a backtick, a newline, `matchblox send %2 hi`, `matchblox sessions`, flags in another order, a sign that is not hex, a non-phone key line. Then `TestChooseStaleExits3`: a refused pick because the menu changed exits 3 and prints `the question changed` on stdout (SSH drops stderr); any other refusal exits 1.
- Run: `go test ./cmd/matchblox -run 'TestGatePhoneHead|TestChooseStale'` — fails: the gate refuses all three; choose exits 1 on a stale menu. Then passes.
- Do: in `gateDecision`, phone mode only, parse `orig` with the single-quote rules of `demo.words` (shared, not copied) and accept only the three exact shapes; return their argv. In `choose`, map the service's "menu changed" refusal to exit 3 with one stdout line.
- Done when: both tests and the full Go suite pass.
- Commit: `feat(gate): the phone's key runs menu, choose and sessions for the head`

### Task 4: the turn counts notches round the head
- Files: `MatchHead.swift`, `MatchHeadTests.swift`.
- Test first: `testFifteenDegreesIsOneNotch` (angles 0→0.27 rad give one clockwise notch; back gives one anticlockwise); `testAngleWrapsAtPi` (from 3.1 to -3.1 rad is a small clockwise move, not a full turn); `testPaceSteps` (notches since the crossing 1…10 → 1 line, 11…24 → 3, 25…48 → 8, 49+ → 20); `testPauseResetsPace` (0.35 s with no notch → 1 line); `testStrikesOnceARoll`; `testHeatFollowsPace` (heat = lines a notch ÷ 20: 0.05, 0.15, 0.4, 1). Angles are `atan2(dy, dx)` in UIKit points; a growing angle is clockwise.
- Run: `swift test --package-path ios/MatchbloxKit --filter MatchHeadTests` — fails on the new tests; then passes.
- Do: replace `roll(dy:speed:)` and `lines(atSpeed:)` with `turn(angle:at:) -> Roll` and `pace(notchesSinceCross:)`. Delete the dy tests that test the old grammar (testTenPointsIsOneNotch, testLinesPerNotch, testUpRollIsUpNotches, testHeatFollowsSpeed); keep `WheelBurst` and its test.
- Done when: `MatchHeadTests` passes.
- Commit: `feat(ios): the match head turns round its edge`

### Task 5: where the finger lands decides the meaning
- Files: `HeadZone.swift`, `HeadClick.swift`, `HeadZoneTests.swift`, `HeadClickTests.swift`.
- Test first: `testZones` (radius 66: (0,20) middle, (0,-50) top, (-50,5) left, (50,0) right, (3,50) bottom); `testPressUnderTenPoints` (9.9 pt press, 10 pt not); `testMiddleLift` (0.2 s → `.enter`, 0.4 s → `.nothing`, 0.2 s after a roll → `.nothing`, 0.2 s with `coasting: true` → `.nothing`); `testHoldDue` (`holdDue(after: 0.59)` false, `0.6` true: the hold starts during the press, not on the lift); `testLiftAfterHold` (a lift after the hold began → `.hold`, never `.enter`).
- Run: `swift test --package-path ios/MatchbloxKit --filter 'HeadZoneTests|HeadClickTests'` — fails: no `HeadZone`; then passes.
- Do: `HeadZone.at(point:radius:) -> Zone`; `HeadClick` gains `lift(after:rolled:coasting:) -> Lift` and `holdDue(after:) -> Bool`.
- Done when: both test classes pass.
- Commit: `feat(ios): the head's middle and edge mean different things`

### Task 6: one notch walks the options, then the output
- Files: `HeadWalk.swift`, `HeadWalkTests.swift`.
- Test first: `testUpThroughMenuThenCross` (menu of 3, chosen 3, three anticlockwise notches → `.highlight(2)`, `.highlight(1)`, `.cross`); `testCrossThenScroll` (next notch `.scroll(1)`); `testNoMenuScrollsAtOnce`; `testDownStopsAtBottom` (lines up 2, three clockwise notches → `.scroll(-1)`, `.home` (0 up, firm tap), `.bump(.bottom)`; no `.highlight` in that roll); `testNewRollMovesHighlight` (after the lift, clockwise → `.highlight` next); `testLastOptionBumps`; `testTopOfScrollbackBumps` (route `.end` → `.bump(.top)`); `testRollStartingUpPaces` (a roll that starts 30 lines up counts its pace from its first notch); `testWaitsForMenu` (no peek reply yet → `.wait`; a waited notch is dropped); `testTapPicks` (highlight 2 → `.pick(2, "Yes, and open a pull request", sign)`; no menu → `.enter`; lines up → `.home`).
- Run: `swift test --package-path ios/MatchbloxKit --filter HeadWalkTests` — fails: no `HeadWalk`; then passes.
- Do: `struct HeadWalk { mutating func menu(_: MenuSight?); mutating func notch(_ dir: Dir, route: HeadRoute) -> Step; mutating func tap() -> Tap; mutating func lift() }`. It owns the highlight, lines up and notches since the crossing, and asks `MatchHead.pace` for the lines of each output notch; `.scroll` carries lines. It never yields an arrow key.
- Done when: `HeadWalkTests` passes.
- Commit: `feat(ios): a roll walks the options, then the output`
- Surface: an agent's tools (`testTapPicks`: the pick carries text and sign; no arrow ever goes).

### Task 7: the owner's marks, with undo
- Files: `HeadMarks.swift`, `KeyFace.swift`, `HeadMarksTests.swift`.
- Test first: `testFirstMarks` (top `◂ back`, left `← cursor`, right `→ cursor`, bottom `⌨ type`, hold `⇄ sessions`); `testSetAndUndo`; `testAsItCameThenUndo`; `testInterruptRefused` (setting `^C` on any mark throws `.staysOnBelt` and keeps the mark); `testSessionsOnlyOnHold`; `testDecodesLeniently` (a stored `{"left":"ctrlc"}` or an unknown id loads the first mark for that place); `testWords` (`Esc` → `cancel`, `Tab` → `tab`, `⇧Tab` → `mode`, `⇞` → `page up`, `⇟` → `page down`, `/c` → `compact`).
- Run: `swift test --package-path ios/MatchbloxKit --filter HeadMarksTests` — fails; then passes.
- Do: `struct HeadMarks: Codable` with `subscript(place)`, `set(_:at:) throws`, `undo()`, `asItCame()`; `load/save` under `head.marks`. `◂` sends `HomeKey.bytes`; word moves send `ESC b` / `ESC f`.
- Done when: `HeadMarksTests` passes.
- Commit: `feat(ios): the owner sets the head's four marks and its hold`
- Surface: an agent's tools (`testInterruptRefused`).

### Task 8: the belt has no head key (build it after Task 10, so the app always has a head)
- Files: `BeltLayouts.swift`, `KeyBar.swift`, `BeltEditor.swift`, `BeltView.swift`, `BeltLayoutsTests.swift`.
- Test first: `testNoScrollGroup`; `testMineDropsStoredWheel` (stored `{keys:[{id:"Wheel",cells:1},{id:"Esc",cells:1}]}` loads as `[Esc]`, and a move of index 0 moves Esc). Delete `testWheelSendsNothingAndSaysWhatItIs`.
- Run: `swift test --package-path ios/MatchbloxKit --filter BeltLayoutsTests` — fails on the two tests; then passes.
- Do: remove `KeyCatalog.scroll` and `KeyBar.wheel`; drop only the id `"Wheel"` in `Mine.init(from:)` (agent command keys stay); remove the Wheel drawing in `BeltEditor.swift:115-118` and in `BeltView.swift:483-492`.
- Done when: `BeltLayoutsTests` passes and `make -C ios build` passes on the workflow.
- Commit: `refactor(ios): the belt has no head key`

### Task 9: the handle row says what the head does, and names the step
- Files: `HeadHint.swift`, `StepMark.swift`, `HeadHintTests.swift`, `StepMarkTests.swift`.
- Test first: `testRestLine` (`roll: options, then the output · hold: sessions · BOX sets them`, with the hold's word from `HeadMarks`); `testUpLine` (`12 lines up · tap the head to come back`); `testOptionLine` (`2 of 3 · Yes, and open a pull request`); `testStaleLine` (`the question changed · roll again`); `testBumpLines` (top: `top of the scrollback · roll the other way to come down`; last option: `last option · roll back, or tap to answer`; nothing: `nothing scrolls here · this program has no scrollback`); `testNearestStep` (visible lines with `✦ Reading a.go` at row 2 and `⏺ Bash(go test)` at row 5 → `✦ Reading a.go`; none → nil).
- Run: `swift test --package-path ios/MatchbloxKit --filter 'HeadHintTests|StepMarkTests'` — fails; then passes.
- Do: pure functions; no UIKit.
- Done when: both pass.
- Commit: `feat(ios): the handle row says what a roll and a hold do`

### Task 10: one big head over the belt
- Files: `DockHeadView.swift`, `HeadDriver.swift`, `HeadView.swift`, `BeltView.swift`, `ConsoleView.swift`, `ConsoleTerminalView.swift` (the highlight overlay), `HeadTests.swift` (UI).
- Test first (UI, demo console): `testHeadIsOnEveryLayout` (the element "Match head" exists on Steer, tmux and Mine; no belt key named "Match head"); `testTapAnswers` (roll one notch up at the demo question, tap → the demo shows option 2 answered); `testStalePickRefused` (a UI-test launch flag makes the demo redraw its question after the roll → nothing answered, the handle row reads `the question changed · roll again`); `testStepCardShows` (roll 30 notches up the demo output → a card names a `✦` step; it goes 0.6 s after the roll slows below 3 lines a notch); `testTouchShowsMarks` (press 0.2 s → "back", "cursor", "type" exist; 1.2 s after the lift they do not); `testRollEndingOnEdgeSendsNothing` (drag along the edge from right to bottom, lift → the demo's line is unchanged); `testPressTypeRaisesKeyboard`.
- Run: `make -C ios uitest DESTINATION="id=<udid>"` (the `iOS demo` workflow) — fails: no "Match head" element; then passes.
- Do: `DockHeadView` in `Belts.dock` only, above the belt, 96 pt, fixed, its lower part over the belt; the dock grows by its height; the lifted belt has no head. On touch-down at an agent, `HostCommand` runs `matchblox menu <pane> --json` and feeds `HeadWalk.menu` (the pane from the `agent=…;pane=` signal; in the console's chat, the chosen session's pane; on the console's list, no menu: the roll scrolls and the tap is Enter). Touches: `HeadZone` → middle (`HeadClick` → `HeadWalk.tap`: `.pick` runs `matchblox choose <pane> <n> --text <t> --sign <s>`; a refusal shows `the question changed · roll again`) or edge (turn via `HeadDriver` → `HeadWalk` → `terminal.head(lines:)` and the highlight overlay on the screen row whose text matches the option; or a mark's key on a press). Ring and marks fade in on touch-down, out 0.9 s after lift. `HeadFeel.cross()` on `.cross` and `.home`, `bump()` on `.bump`. Cut the `HeadDriver` leftovers (`seat`, `canClick`, the summon comment). Reduce Motion stops the turn, as today. `HeadLayer` draws the material of the spec: the cold colour from `HeadPigment`; a grain made once per size (a noise height field lit from the top left, multiplied over the colour so it only darkens; medium grain, about 1.7 pt features at 96 pt) that turns with `turnX`/`turnY` while the light stays fixed; flatter shading than today, and no white highlight; a shallow well (3 pt, inset shadow), a soft drop shadow (3 pt down, 6 pt blur), and a 1 pt sink with a tighter shadow on press. The speckles stay, as darker grit.
- Done when: the seven UI tests pass. (The owner's phone check is in Task 16.)
- Commit: `feat(ios): one match head over the belt, on every layout`
- Surface: an agent's tools (`testRollEndingOnEdgeSendsNothing`).

### Task 10a: the board names the chat's pane
- Files: `internal/app/boardkeys.go`, `internal/app/boardkeys_test.go`, `BoardBelt.swift`, `BoardBeltTests.swift`, `ConsoleView.swift`, `DockHeadView.swift`.
- Test first: Go `TestTheChatNamesItsPane` (the chat of a chosen session ends its value in `;chat=<pane>`; the list names none; a pane that is not `%` and digits, such as `%3@matchblox` or `%2;rm`, leaves plain `;chat`), `TestTheBeltValueKeepsItsLimit` with a pane, and `TestTheBeltSaysWhenTheChatTakesTheScreen` now wants `1,2;;chat=%2` (flint-auth's pane); Kit `testTheChatNamesItsPane` (`chatPane` is `%2` for `;chat=%2`; nil for `;chat`, `;chat=%2;rm`, `;chat=abc`, `;chat=`, `;chat=%`; the keys stay the same) and `testTheChatsPaneGoesWithTheBoard` (no pane while a pane shows or off the board).
- Run: `go test ./internal/app/` — fails: `beltWord` takes no pane; then passes. `swift test --package-path ios/MatchbloxKit` (the `iOS` workflow) — fails: `BoardBelt` has no `chatPane`; then passes. Task 10's `testTapAnswers` and `testStalePickRefused` (the `iOS demo` workflow) pass with it.
- Do: `beltWord(ks, armed, chat, pane)` adds `=<pane>` after `;chat` when the pane is a bare pane id of the service's session (`paneID`); never text from a pane. `BoardBelt(said:)` reads it strictly into `chatPane`. `Belts` gives the head the pane of the `agent=…;pane=` signal, else `board.belt.chatPane` (nil off the board and in a pane).
- Done when: the Go and Kit tests pass, and the two Task 10 UI tests pass on the `iOS demo` workflow.
- Commit: `feat(console): the board signal names the chat's pane`
- Surface: an agent's tools (a hostile pane in the value reads as none).

### Task 11: the hold switches session
- Files: `DockHeadView.swift`, `ConsoleView.swift`, `HeadTests.swift`.
- Test first (UI): `testHoldShowsSessions` (go into demo session 1's pane, press 0.7 s → a card lists the three demo sessions, session 1 marked); `testHostListFails` (the list command fails → the card says `sessions not reachable · try again` and the lift does nothing); `testLiftOnCurrentChangesNothing`; `testHoldEndsEarlySendsNothing` (press 0.45 s → no answer, no card).
- Run: as Task 10 — fails; then passes.
- Do: the hold ring fills from 0.15 s to 0.6 s; then the card from `matchblox sessions --json` run through `HostCommand`; the current one is the row whose `pane` is the pane of the last `agent=…;pane=` signal, else the `n` this console opened, else none is marked; each 30° round the head's centre steps one session, counted once the finger is outside the middle 25% of the radius, where an angle is steady; lift opens the chosen one through `OpenLink`'s `--open N` reconnect, as a tap on a session in the Live Activity does. No list (one session or none): the card says `no other session on this host` and the lift does nothing.
- Done when: the four UI tests pass.
- Commit: `feat(ios): hold the head to switch session`

### Task 12: the belt comes forward, and the keyboard folds
- Files: `BeltView.swift`, `ConsoleView.swift`, `HeadTests.swift`.
- Test first (UI): `testBeltDragBringsItForward` (drag the belt → the head is not hittable; 2 s later it is); `testKeyboardFoldsAfterSend` (raise with `⌨`, type `ok`, Return → the demo receives `ok`, then the keyboard is gone and the head is hittable); `testFoldKeepsText` (Return sends the whole line before `resignFirstResponder`); `testLiftedEnterFolds` (with the keyboard up, the lifted belt's `⏎` sends Enter and folds the keyboard).
- Run: as Task 10 — fails; then passes.
- Do: belt drag sets `dock.headBehind = false`; settle + 1.4 s sets it back. After a Return from the system keyboard or the lifted belt's `⏎` is sent, `terminal.resignFirstResponder()`.
- Done when: the four UI tests pass.
- Commit: `feat(ios): the belt comes forward, and the keyboard folds after a send`

### Task 13: home sets the head
- Files: `HostsView.swift`, `HeadPigment.swift`, `HeadPigmentTests.swift`, `HeadView.swift`, `HeadTests.swift`.
- Test first (UI): `testHomeSetsLeftMark` (open home, tap the left mark, tap `Esc cancel` → the console's left mark reads `Esc cancel`; `undo` → `cursor`); `testInterruptDim` (`^C interrupt` is not enabled and shows `stays on the belt: a roll can brush the edge`); `testHomeSetsColour` (tap `blue tip` → the dock's head has the value `blue tip`; relaunch → still `blue tip`). Unit, `HeadPigmentTests`: `testUnknownIsMatchRed` (stored `"pink"` → `.matchRed`), `testFiveColours` (five cases, in the home order, each with its name), `testColdOnly` (heat 0.5 gives the same ember for every pigment).
- Run: as Task 10 — fails; then passes.
- Do: region `THE HEAD` after the `if !realHosts.isEmpty` block (`HostsView.swift:93`), outside it, so the demo shows it too: a drawn head with five buttons, the list, `undo`, `as it came`, `its colour`: five 28 pt heads in a radio row (44 pt targets), each named; a tap stores `HeadPigment` under `head.colour` and the dock's `HeadLayer` redraws at once.
- Done when: the three UI tests and the three `HeadPigmentTests` pass.
- Commit: `feat(ios): home sets what the head's marks and hold do`

### Task 14: VoiceOver reaches the head
- Files: `DockHeadView.swift`, `HeadTests.swift`.
- Test first (UI): `testHeadAccessibility` — label "Match head", trait adjustable; custom actions named after the four marks and "sessions"; at a menu, increment moves the highlight down one option and announces it; with no menu, it pages.
- Run: as Task 10 — fails; then passes.
- Do: move the accessibility code of `BeltView.swift:1022-1072` to `DockHeadView`, menu-aware.
- Done when: the UI test passes.
- Commit: `feat(ios): VoiceOver reaches the head's marks and sessions`

### Task 15: the docs and the landing page say one head
- Files: `docs/phone.md`, `docs/keys.md`, `DESIGN.md`, `www/index.html`.
- Test first: `.github/scripts/landing-guard.sh "feat(ios): one match head over the belt" <body> origin/main` passes with `www/` changed; `rg -n "Scroll group|comes under it" docs/phone.md docs/keys.md DESIGN.md www` finds nothing.
- Run: the two commands — the rg finds `docs/phone.md:417,460`; then nothing.
- Do: rewrite "#### The match head" for the roll round the edge, the marks, the hold and home; cut the summon text; the landing's Pocket line names the roll, the tap and the hold.
- Done when: both checks pass.
- Commit: `docs: Pocket has one match head`

### Task 16: review
- Load `builder:review` on the whole branch against the spec, with `go vet ./... && go test -race ./...`, the `iOS` and `iOS demo` workflows green, and the owner's phone checks of the spec's done-when.
- Done when: no open Critical or Important finding, and the owner has run the phone checks.

## Lens findings

- Scope: 15 files touched in the app and Kit, and 6 new parts (`HeadZone`, `HeadWalk`, `HeadMarks`, `HeadHint`, `StepMark`, `DockHeadView`) — over the smell line. Kept, because each is a pure function the UI tests cannot reach in timing detail, and the spec asks for each behaviour. Cut: a new session API (the card reads `sessions --json` and opens with `--open <n>`); a new haptic engine (reuse `HeadFeel`).
- Architecture: the agent owns its menu; the host is its only reader (`menu.Read`, `menu.Chosen`, `menu.Sign` on the live screen) and the only one that types a pick, under the pane's lock (`service.choose`). Pocket asks over its SSH link with two commands that say hello as `ClientPocket` (`menu`, `choose`); the service trusts that hello for those two acts only, and refuses them from an agent's pane (Task 2). The demo answers the same commands in-process (`MBDemoRun`, Task 3). Cut: a host signal over the terminal stream (it never reaches a tmux pane); `peek` (it reads the transcript, so no sign and other labels); `send --pick` (it answers the question tool, not the screen). A failure in production: the screen changes between the roll and the tap; the host refuses the stale sign (Task 2, Task 10 `testStalePickRefused`). Another: the menu command is slow on a poor link; notches before its reply are dropped (`testWaitsForMenu`).
- Tests: the timing paths (0.35/0.6 s, 10 pt, 15°, pace) are unit tests; the UI tests prove what the owner sees. Gap: the feel of the haptics — no test sees it; the owner's phone check in Task 16.
- Regression: today's scroll to the bottom before a key (`jumpToBottom`) stays, kept by the existing `Belts.send` path; the right-click hold on the terminal (`BoardTouch.longPress`) is untouched — no change, because the head no longer lives on the terminal. Old `belt.mine` with `Wheel`: Task 8 `testMineDropsStoredWheel`.
- Performance: one notch is one arrow or one `head(lines:)` call; 20 lines a notch at up to 30 notches a second is 600 lines/s, the same order as today's 10 × 30. No change.
- Design: every state has words (Task 9). The empty sessions state has its line (Task 11). The keyboard-up state: the head stays in the dock, which hides under the keyboard (Task 10). Home's dim `^C` says why (Task 13).
- Architecture lens (a tool for an agent: keys into a live pane): the pick is checked by the host against text and sign, and only `ClientPocket` acts `menu:` and `choose:` are trusted (Task 2); Pocket decodes the menu and sessions strictly, and `MBDemoRun` takes three commands only (Task 3); no head path sends `^C` (Task 7).

## Spec check

| Spec line | Task |
|---|---|
| One head above the belt on every layout, no head key | Task 10, Task 8 |
| Anticlockwise moves the highlight up, a tap answers it, in console and pane | Task 1, Task 6, Task 10 |
| A changed screen: the tap sends nothing, the line says so | Task 2, Task 10 |
| Past the first option scrolls up, firm tap, tap brings back | Task 6, Task 10 |
| 48 notches → 20 lines, strike, step card | Task 4, Task 9, Task 10 `testStepCardShows` |
| Clockwise from the output stops, the choice does not move | Task 6 |
| Tap on the middle answers, no wait | Task 5, Task 10 |
| A roll ending on the edge sends no key; `⌨` raises the keyboard; Enter sends and folds | Task 10, Task 12 |
| Hold → sessions; lift on current changes nothing | Task 11 |
| Home sets left mark to Esc, undo | Task 13 |
| Home sets the colour; it stays after restart; still warms | Task 13 `testHomeSetsColour`, `testColdOnly` |
| The head reads as a dry, grainy match head; grain turns, light stays | Task 10 (`HeadLayer`) and Task 16 phone check |
| Belt comes in front and goes back | Task 12 |
| VoiceOver | Task 14 |
| Docs and landing name one head | Task 15 |
| Anti: a roll brushing the edge sends a key | Task 5, Task 10 |
| Anti: a late lift or a lift after a roll sends Enter | Task 5, Task 11 |
| Anti: a touch on the head changes settings | Task 13 (settings only on home) |
| Anti: the head hides the question or the input line | Task 10 (head in its own row) and Task 16 phone check |
| Anti: the card goes to another session | Task 11 |
| Anti: `^C` from the head | Task 7, Task 13 |
| Anti: the keyboard folds before the line is sent | Task 12 `testFoldKeepsText` |
| Surface: an agent's tools | Tasks 2, 3, 6, 7, 10 |

## Calls

- Swift runs only on macOS CI. The owner gave a standing yes (2026-11-27) to push `build/one-match-head` as a draft PR and push after each task, so each Swift task proves its test on the `iOS` workflow.
- The spec says the called-up head "goes"; it already went in #475. No task removes it; Task 10 cuts its leftovers.

## Cold read

Cold read: 2026-11-27, 1 fresh reader. Blocker 2, major 10, nit 5. Fixed 16, contested 0, for the person 1, declined 0. Second read: 17 left (blocker 3, major 6, nit 8): the CLI pick never reaches `service.choose` (it needs a console act and the screen text); peek reads the transcript, not the screen, so it has no sign and its labels differ; the iOS demo has no command channel for peek, pick or sessions; plus the home region place, the step card UI, the pane on the console list, the current session, the pace in a roll that starts up, and 8 nits. For the owner.

- For the owner, answered: the menu path — the host-checked pick, on a peer session's advice.
- For the owner, answered: the second read's blockers — the owner chose one PR, redesigned now, reviewed by the owner (no third read). Changed after the read: What exists, the file map, Tasks 1–3, 5, 6, 8, 10–13, Lens findings, Spec check. Fixed with it: the home region place, the step card UI, the pane on the console's list, the current session, the pace in a roll that starts up, and the 8 nits.

## Changes during build

- 2026-11-27, Task 3: the demo keeps its fixture (7 sessions; pane %2's menu has 5 options) because the walk, app goldens and `DemoConsoleTests` share it; tests check every session with an id has a unique `n`, not "three sessions". The `✦` steps move to Task 10 next to `testStepCardShows`. `MBDemoRun` takes a pane (`menu %2`), as the host does.
- 2026-11-27, added Task 3a: the phone's key gate (`gateDecision`) refused `menu`, `choose` and `sessions --json`, so the head could not reach a real host; and SSH drops stderr, so a stale pick needs its own exit code (3) for Pocket to say `the question changed · roll again`. Task 9 maps exit 3, not stderr text.
- 2026-11-27, Task 3: a session started after the phone last registered has no `n` and is left off the sessions card until the next register, because `console --open <n>` can only open a numbered session.
- 2026-11-27, Task 6: `MatchHead` counted its heat and strike from the roll's start, not from the crossing into the output, so menu notches heated the head. `MatchHead.cross()` starts the count again (`testPaceAndStrikeCountFromTheCross`); the driver calls it on `.cross` (Task 10). `HeadWalk.notch` takes `at time:` so its own pace keeps the 0.35 s pause. Its bumps add `.nothing` (a program with no scrollback), and a fresh read of the same menu (same sign) keeps Pocket's highlight.
- 2026-11-27, Task 10: the demo reports a stale pick as a host does. `demo.Run` returns matchblox's exit status, 3 with `the question changed` for a refused pick; `MBDemoRun`'s out-param is that status, and `DemoSession.run` passes it into `HostReply`, so `HeadHint.afterPick` reads the demo as a host. `Demo.MoveCursor` (`MBDemoMoveCursor`) moves the asking session's cursor, as its agent does: the `-demoQuestionChanges` launch flag runs it before each pick (`testStalePickRefused`, `TestDemoRunPickAfterTheAgentMovedIsStale`).
- 2026-11-27, Task 10: done after the owner allowed the fixture edit (a local allow rule, removed after merge). The `✦`/`⏺` step lines for `testStepCardShows` go at the top of hearth-api's pane, `testdata/demo/panes/1.txt`: no replay frame shows that pane, so no golden changes. Not written yet.
- 2026-11-27, Task 10: taken by Task 10a. In the console's chat the app knows no pane: the board signal says `;chat` and no pane, and only a jump into a tmux pane sends `agent=…;pane=`. So the head reads a menu only in a pane, and `testTapAnswers` and `testStalePickRefused` (in flint-auth's chat) cannot pass yet. Proposed: the board value names the chosen session's pane while the chat shows (`…;chat=%2`, `beltWord` in Go, `BoardBelt.chatPane` in Kit), and the head takes it as its pane.
- 2026-11-27, Task 10: a UI test rolls with one straight drag, which turns the head about 11 notches at most, so `testStepCardShows` rolls 11 (the 11th notch is at 3 lines) and reads the card from the head's DEBUG value, as the card stays 0.6 s. The finger's angle counts from 15% of the radius (Task 11 keeps 25% for the sessions). A roll walks only from a touch that began on the edge; one from the middle turns the head and sends nothing. The head has no coast: a flick that went on after the lift would walk a roll that had ended. At rest the dock's handle row shows `HeadHint.rest`; the head's lines hold until the next one.
- 2026-11-27, Task 11: the card's logic is a Kit type, `SessionCard` (`SessionCardTests`), so the anti-goal has a unit test: a lift opens the number of the row shown chosen, never the current row's, and none when two rows share a number. `SessionRow` gains `pane`, read strictly as a bare pane id. The current session is the row of the head's pane (the `agent=…;pane=` pane, else the chat's pane of Task 10a), else the number this console opened. With none marked, nothing is chosen until the first step clockwise. The UI tests hold 1.0 s, not 0.7 s, so a slow runner still shows the card, and read it from the head's DEBUG value, as it goes at the lift; they hold in flint-auth's chat. `-demoSessionsFail` makes the demo's list fail. In the demo a lift on another session restarts the demo, as `jump(to:)` does there: the demo has no `--open`.
- 2026-11-27, Task 12: the dock's flag is `beltInFront` (true while dragged and for 1.4 s after it stops), not `headBehind = false`, which read backwards. The fold comes from `ConsoleTerminalView.onReturnSent`, called after a Return from the system keyboard reached SwiftTerm; it replaces `onTyped`, which nothing used once the head lost its coast. The belt's Enter folds the same way, and so does the board's `⏎ do` (its byte is Return too). Both fold on the next run-loop turn, after the bytes have gone. Open for the owner: a Return on a hardware keyboard also folds if SwiftTerm passes it through `insertText`, which would take the keys away from an iPad keyboard after each line.

- 2026-11-27, Task 13: the heat colours move into `HeadPigment` with the cold head and its shade, so `testColdOnly` reaches them in Kit; `HeadLayer.cold` became `HeadLayer.pigment`. The dock head's accessibility value starts with the pigment's name (`blue tip`), before the DEBUG card lines, and the dock reads `head.colour` again on `UserDefaults.didChangeNotification`, as the text size does. The UI tests start from `-resetHead` (DEBUG: forget `head.marks` and `head.colour`) and read the left mark from its label `head-mark-left`.
- 2026-11-27, Task 14: XCUITest has no way to swipe as VoiceOver does (no increment on a plain adjustable element) nor to list custom actions, so the swipe's logic is a Kit type, `HeadVoice` (`HeadVoiceTests`), and `testHeadAccessibility` checks the label and the actions' names from a DEBUG `actions=` line of the head's value, before and after home changes the left mark. At a menu a swipe stops at the first and the last option, never crossing into the output, so the double-tap after it answers the option it named. The actions are named `cursor, left mark` (the word, then the place: left and right are both `cursor`) and `sessions`; a `·` mark is no action. The sessions action shows a sheet of the other sessions. After a swipe the head's value is the line it said, so VoiceOver reads it; it is the colour again at the next touch or focus. `head.page` is wired to `headPage`.
- 2026-11-27, Task 17: Task 10, not only Task 8, took the belt head's wiring out, so main's also comes back where the file list did not name it: `ConsoleView` sets the belts' `onHead`/`onHeadPage`, `ConsoleTerminalView.onTyped` stops the belt head's coast again, and `HeadFeel.stop()` returns. In `MatchHead` the roll's constants take their own names (`rollNotch`, `rollMaxLines`) beside the turn's `notch` and `maxLines`. `BeltHeadDriver` draws one turn round the centre, as `HeadLayer` now draws, and drops main's unused `seat()` and `canClick`. Main's `testRollReportsLines` is not named in the task and stays out.
- 2026-11-27, Task 18: the head follows the dock's layout in `BeltView` (`placeHead` on every layout change), not only `layoutPicked`, so a pick, a turn to Board and back, and Done in the Mine editor all carry or drop it. Joystick's `⌨` is `KeyBar.keyboard`: it sends no bytes, as Ctrl and Wheel do (`testEveryKeyHasBytes` walks every layout), its press raises or hides the keyboard as the handle row's key does, and VoiceOver says "Keyboard". `MatchTrayTests` count six matches, Joystick last. The HeadTests pick Joystick in `launched()`, not `setUp`, since each test launches with its own arguments; `testHeadOnlyOnJoystick` gives the demo a Mine with the small head key by the `-belt.mine` launch argument, as the demo opens no Mine editor. `testLiftedEnterFolds` goes: Joystick's belt has no `⏎`, and no other layout folds. `DockHeadView` needs no change: its `willMove(toWindow:)` already drops a touch when the head leaves.
- 2026-11-27, Task 19: `began(at:)` takes the time only, so `moved(to:at:)` takes the finger's travel from where it landed. `Send.key`'s `word` is set at any place from the tenth repeat, and the mark decides: `HeadMarks.wordBytes` gives `Esc b`/`Esc f` for `←` `→` and nil for `↑` `↓`, which repeat the arrow. A ring filled at 0.6 s stays filled through the pushes that step the sessions, so that lift is `.hold`. The hold refuses `↑` `↓` as `.notAMark` (they are not in its list). `load(from:)` saves the moved build-9 marks under `head.marks.2` and removes `head.marks`; `HeadMarks.buildNineKey` is public so `-resetHead` forgets both. The UI HeadTests that read build 9's first marks (`testTouchShowsMarks`' back and type, `testPressTypeRaisesKeyboard`, `raiseTheKeyboard` in `testKeyboardFoldsAfterSend` and `testFoldKeepsText`, `testHeadAccessibility`'s action names) fail in `iOS demo` until Task 20 rewrites HeadTests for the push: Task 19's files do not name `HeadTests.swift`.
- 2026-11-28, Task 20: `testPushDownMovesTheAgent`, `testPushHeldRepeats` and `testPushUpPastFirstReadsUp` run in flint-auth's pane (the demo's stand-in, by `go in`), not in its chat: in the console's chat `↑` `↓` choose the session, as a keyboard's do (`lineKey` in `internal/app/sidekeys.go`), so a push there never reaches the agent's `❯`. In a pane the demo already moves `❯` on arrow bytes and takes the marked option on Enter (`TestTheStandInAnswersWithArrowsThenEnter`): no Go change. Open for the owner: the chat is where the owner answers most, and a chat that passes `↑` `↓` to an agent at its menu changes the desk console, a non-goal.
- 2026-11-28, Task 20: `ScreenMenu` keeps `internal/menu`'s rules and also skips one status line at the very bottom of the screen (a jumped tmux pane's status line, the stand-in's key line), never the agent's `>` input; it needs a marked option. Without the skip no jumped pane would read. The handle row's `n of m` names the option the push moves to, from the read before the push.
- 2026-11-28, Task 20: `HeadVoice.swipe` and its menu steps go now, not in Task 23: with the host's menu read cut, a swipe's highlight would name one option while the double-tap's Enter answers the agent's own `❯`. Until Task 23 a swipe pages and the double-tap is Enter. `HeadHint` loses the walk's and the pick's lines (`waiting`, `crossed`, `lastOption`, `questionChanged`, `notSent`, `unreadable`, `afterPick`); its rest line is `roll: the output · hold: … · BOX sets them` and its up line `… up · push ↓ or tap to come back`. Pocket's highlight goes with the walk (`ConsoleTerminalView.highlightRow`, `onHighlight`). `MenuSight`, the `menu`/`choose` CLI, `HeadZone.isPress`, `MatchHead.cross` and `-demoQuestionChanges` stay for Task 24; `docs/phone.md` for Task 25.
- 2026-11-28, Task 20: a press on the edge sends no mark's key any more: the edge is only the wheel. Only `↑ ↓ ← →` repeat while pushed; `⌨`, `◂`, `Esc`, `/c` and the rest send once a push. `HeadStick` shows `still` and `held`, so the dock ends the ring at a push and steps the sessions after the ring filled; the dock asks `tick(at:)` every 15 ms on `CACurrentMediaTime`, the touches' clock. The coast (×0.93 a 60 Hz frame) starts from a lift that moved in its last 0.05 s at 4 notches a second or more, stops below that, and stops at a bump. A touch that stops a coast, or comes 0.3 s after a notch, is never Enter, as before.
- 2026-11-28, Task 20: the HeadTests that read build 9's marks now read Task 19's: `testTouchShowsMarks` looks for `❯ up`, `cursor` and `❯ down`, with a 0.45 s press that sends nothing; `testPressTypeRaisesKeyboard` became `testKeyboardKeyRaisesKeyboard` (Joystick's belt `⌨`), and `raiseTheKeyboard` (`testKeyboardFoldsAfterSend`, `testFoldKeepsText`) taps that key; `testHeadAccessibility` reads `❯ up, top mark` and `❯ down, bottom mark`. `testTapAnswers` and `testStalePickRefused` go with the pick. `testStepCardShows` rests 0.15 s before its lift, so no coast scrolls past the step it reads. The fold after Enter on Joystick stays proved by `testKeyboardFoldsAfterSend`: while the keyboard is up the dock, and so the head, is hidden, so the keyboard's Return is the Enter that folds there.
- 2026-11-28, Task 20, CI on `d9cef9b0` and `d76bc797`: `HeadStick`'s `CGPoint = .zero` does not build in Kit (`swift test` on macOS: Foundation's `CGPoint` has no `.zero`), so neither iOS job ran Tasks 19–20; Kit writes `CGPoint(x: 0, y: 0)`, as its other files do, and `HeadStickTests` compare the offset's `x` and `y`. `testHomeSetsLeftMark`: home is made again each time it opens and reads the marks from the store, and the undo steps lived only in that view, so `undo` after reopening home had nothing to take back; `HeadMarks.save` now writes them under `head.marks.2.history` and `load` reads them (`testUndoAfterLoad`; `-resetHead` forgets them). `testStepCardShows` was wrong: CI's card read `⏺ Read internal/limit/store.go · 13 lines up`, exactly the 11 notches' 13 lines, so no coast went past the step; hearth-api's pane draws `⏺ Read` over `✦ Running`, and the card names the first step from the top of the view (`StepMark`), so the test now expects `⏺ Read`. `testHeadOnlyOnJoystick` (the head exists but is not hittable after Mine → Joystick): no cause found by reading; ruled out `beltInFront` (Steer's drag cannot set it), the screen hold (`drawerOpen` 96 ≠ the row's 72, so it lets go), the cached accessibility list (`placeHead` and the rebuild clear it) and the head's z-order (added last).
- 2026-11-28, Task 20, review: the dock kept its own `touch.held` from a timer beside `HeadStick.held`, so a push near 0.6 s could step the sessions with a lift of nothing, or send a key with a lift that opens the hold. The stick alone says the hold now: the dock starts the hold when `held` turns true at a move or a tick (`holdBegan`), pushes step the sessions only then, and the list's answer waits for it (`testHeldOnlyFromTheStick`).
- 2026-11-28, Task 20, the owner's ruling of 2026-11-28: in the console's chat (the board says `;chat`, and the app jumped into no pane) a push `↑` `↓` moves Pocket's own highlight (`ChatPick`, `ChatPickTests`) over the options the screen shows, from the agent's `❯`, at most to option 9 (the chat answers 1–9); `↑` at the first option reads up; the handle row says `n of m · text`, and `ConsoleTerminalView.highlightRow` comes back to draw it. A tap types the option's number twice, as the chat's first digit marks an option and the same digit again sends it (`answerFromChat`); a tap with no push answers the `❯`. With no options on the screen a push is the chat's arrow and a tap Enter. `ScreenMenu` already numbers each option as the screen does (it reads only an unbroken run N down to 1), but read as a pane the chat's own line `›` under the question is text, so `ScreenMenu.read(chat:)` reads above the chat's last `›` line (`testTheChatsQuestion`). UI: `testChatPushSendsNoArrow`, `testChatPushHighlightsThenTapAnswers`. A line that holds text in the chat would take the digits as text: Pocket cannot see that, and no test covers it.

## Revision 2: the joystick, as a layout to pick

The spec changed after TestFlight build 9 (rulings of 2026-11-27, "after TestFlight build 9"): the head is a joystick, `Joystick` is a layout on the Layouts switch, every other layout keeps today's belt and its small head key, the tap is plain `⏎`, and motion matches the mockup. Tasks 1–16 stay built; these tasks change them. Built with the plan's test command; Swift runs only in CI (`iOS`, `iOS demo`).

### Task 17: the other layouts keep their small head
- Files: `KeyBar.swift`, `KeyFace.swift`, `BeltLayouts.swift`, `BeltView.swift`, `BeltEditor.swift`, `MatchHead.swift`, `ios/Matchblox/BeltHeadDriver.swift` (create), `BeltLayoutsTests.swift`, `MatchHeadTests.swift`.
- Test first: bring back main's belt-head tests as they are on `origin/main` (`testWheelSendsNothingAndSaysWhatItIs`, `testMineKeepsHeadAndConsoleKeys`, the Scroll group in `testGroups`; `MatchHeadTests`' `roll(dy:speed:)` tests `testTenPointsIsOneNotch`, `testUpRollIsUpNotches`, `testLinesPerNotch`, `testHeatFollowsSpeed`, and main's `testStrikesOnce` renamed `testRollStrikesOnce` since the branch keeps a turn version), next to the dock head's turn tests. Delete `testMineDropsStoredWheel` and `testNoScrollGroup` (Task 8's).
- Run: `swift test --package-path ios/MatchbloxKit --filter 'BeltLayoutsTests|MatchHeadTests'` (CI) — fails: no `KeyBar.wheel`, no `roll(dy:speed:)`; then passes.
- Do: undo Task 8 (`refactor(ios): the belt has no head key`) for the belt; `MatchHead` keeps both `roll(dy:speed:)` (the belt key, as on main) and `turn(angle:at:)` (the dock head); main's belt-head driver comes back as `BeltHeadDriver` (dy rolls, coast, `HeadLayer`), so `HeadDriver` stays the dock's. `Mine` keeps a stored `"Wheel"`.
- Done when: the belt-head tests from main pass (the UI check is Task 18's `testHeadOnlyOnJoystick`).
- Commit: `fix(ios): Steer, tmux and Mine keep the belt's small match head`

### Task 18: `Joystick` is a layout
- Files: `BeltLayouts.swift`, `BeltView.swift`, `ConsoleView.swift`, `DockHeadView.swift`, `BeltLayoutsTests.swift`, `HeadTests.swift`.
- Test first: Kit `testJoystickLayout` (`BeltLayout.joystick` exists, its keys are exactly `◂ ⌨ Esc ^C ⇞ ⇟ Tab ⇧Tab` — the head does `↑ ↓ ⏎ ← →` —, its name is `Joystick`; a stored unknown layout loads Steer, as today). UI `testHeadOnlyOnJoystick` replaces `testHeadIsOnEveryLayout`: in one run, Layouts → Joystick ("Match head" shows above the belt), → Steer (no dock head; the terminal is `DockHeadView.row` taller; no fold after Enter: the keyboard stays up; dragging the belt does not hide anything), → Mine (the belt's small head key shows), → Joystick again (the head is back).
- Run: Kit filter `BeltLayoutsTests`; UI in `iOS demo` — fails: no `joystick` case; then passes.
- Do: add `case joystick` to `BeltLayout` with the row `◂ ⌨ Esc ^C ⇞ ⇟ Tab ⇧Tab`. `layoutPicked` (`ConsoleView`) carries the dock head when the layout becomes `joystick` and drops it when it leaves (`headRow.constant` 0, the terminal re-lays out); the head is carried at start only when the stored layout is `joystick`. The fold after Enter (revision 1 Task 12) and the belt coming forward run only while the layout is `joystick`. `HeadTests` open `Joystick` first in `setUp`.
- Done when: both tests pass.
- Commit: `feat(ios): Joystick is a layout, and the head comes with it`

### Task 19: the head is a thumbstick
- Files: `ios/MatchbloxKit/Sources/MatchbloxKit/HeadStick.swift` (create), `HeadStickTests.swift` (create), `HeadMarks.swift`, `HeadMarksTests.swift`, `KeyFace.swift`.
- Surface: an agent's tools.
- Test first (hostile first): `testOnePushOneKey` (a thumb that stays pushed past 16 pt sends one key until 0.42 s; wobbling between 16 and 11 pt sends nothing more); `testRearmUnderTen` (back under 10 pt, then a push the other way sends that key, no lift); `testRepeatAndWords` (repeats every 0.11 s after 0.42 s; `←`/`→` give `Esc b`/`Esc f` after 9 repeats); `testTravelClamps` (the ball offset is at most 12 pt in the push's direction); `testTapIsEnterOnlyStill` (under 10 pt and 0.35 s is Enter; any push before the lift makes the lift nothing); `testFirstMarks` (top `↑`, bottom `↓`, left `←`, right `→`, hold `⇄`; `↑` `↓` in `keys(for:)` with words `❯ up`, `❯ down`); `testOldMarksMove` (stored `head.marks` from build 9 with left set to `Esc`: left stays `Esc`, top becomes `↑`, bottom `↓`; saved under `head.marks.2`).
- Run: Kit filter `HeadStickTests|HeadMarksTests` — fails: no `HeadStick`; then passes.
- Do: `HeadStick` is a pure model: `began(at:)`, `moved(to:at:) -> Send?`, `tick(at:) -> Send?` (repeats), `lifted(at:) -> Lift`, `offset` (clamped). `Send` is `.key(Place, word: Bool)` (the mark at that place, a word move after 9 repeats). `Lift` is `.enter` (still, under 10 pt and 0.35 s), `.hold` (still for 0.6 s: the ring filled), `.nothing`. The dominant axis wins a diagonal (`|dx| > |dy|` is left/right). Any push before 0.6 s cancels the hold ring. Thresholds 16 / 10 / 12 pt, 0.35 / 0.42 / 0.11 / 0.6 s, 9 repeats. `HeadMarks`: `edgeKeys` gains `↑` and `↓` (allowed at every edge place, refused for the hold like the others); first marks top `↑`, left `←`, right `→`, bottom `↓`, hold `⇄`. Stored marks move to the key `head.marks.2`; the build-9 key `head.marks` is read once: a place the owner set to anything but the build-9 first mark keeps it, the rest take the new first marks.
- Done when: the tests pass.
- Commit: `feat(ios): the head is a thumbstick: one push, one key`

### Task 20: the dock head pushes, taps Enter, and rolls to scroll
- Files: `DockHeadView.swift`, `HeadDriver.swift`, `HeadView.swift`, `ios/MatchbloxKit/Sources/MatchbloxKit/ScreenMenu.swift` (create), `ScreenMenuTests.swift` (create), `HeadWalk.swift` → `HeadScroll.swift` (the output walk only), `HeadWalkTests.swift` → `HeadScrollTests.swift`, `SessionCard.swift`, `SessionCardTests.swift`, `HeadTests.swift`.
- Surface: an agent's tools.
- Test first (UI, demo, on `Joystick`; hostile first): `testRollSendsNoKey` (a roll on the edge and a flick send no byte; the click count stays); `testPushDownMovesTheAgent` (flint-auth's chat: one push `↓` moves `❯` to option 2; a tap answers it: the demo shows option 2 taken); `testPushHeldRepeats`; `testPushUpPastFirstReadsUp` (then a tap comes back down and sends nothing); `testFlickCoasts` (a flick on the edge scrolls on after the lift and stops at the bottom with the bump line). Kit: `ScreenMenuTests` — `ScreenMenu.read(lines:)` finds the menu at the foot of the visible screen (the same rules as `internal/menu`: numbered options, a `›❯>` mark) and returns `(chosen n, of count, text)`; `❯ 1.` of 3 → (1, 3, …); a numbered list with text under it → nil; `❯ 9.` inside an option's text is not the mark. `HeadScrollTests` — the output walk only: lines up, home, the bumps, the pace (no menu, no pick). `SessionCardTests.testPushSteps` — `step(by: +1)` chooses the next session, `step(by: -1)` the one before, clamped at both ends; a lift on the current one stays nil; the anti-goal tests stay; the angle API goes.
- Run: UI in `iOS demo`, Kit filter — fail: a push sends nothing today; then pass.
- Do: the middle feeds `HeadStick`; the ball moves by `offset` and springs back (0.24 s, damping 0.6); `↑` `↓` send the arrow bytes (`KeyBar.bytes`), `←` `→` the cursor; `↑` at the first option scrolls the view up (the terminal's `head(lines:)`): "the first option" is `ScreenMenu.read` of the terminal's visible lines, read locally on each push, never from the host; with no menu on screen `↑` `↓` are plain arrows. The handle row shows `2 of 3 · text` from the same read, after each push. The tap sends Enter, or comes home when scrolled up. The edge feeds `HeadDriver` → scroll with pace, and a flick coasts (×0.93 a frame, stops at an end with `HeadFeel.bump()`). The hold's card steps on push. Cut the host menu read, the pick and `HeadHint` pick lines from the dock head. `HeadWalk` becomes `HeadScroll` (menu cases deleted).
- Done when: the UI and Kit tests pass, and the six fixed HeadTests from revision 1 still pass where they still apply.
- Commit: `feat(ios): push the head for arrows, tap it for Enter, roll its edge to scroll`

### Task 21: the head moves as the mockup does
- Files: `ios/MatchbloxKit/Sources/MatchbloxKit/HeadMotion.swift` (create), `HeadMotionTests.swift` (create), `DockHeadView.swift`, `BeltView.swift`, `ConsoleView.swift`.
- Test first: `testMotionIsTheMockups` pins the numbers only (marks in 0.18 s from scale 0.86, out after 0.9 s; spring 0.24 s; belt forward 0.25 s, back after 1.4 s; keyboard 0.25 s; home 0.38 s; card 0.6 s; Reduce Motion keeps fades and drops scales and the spring). UI `testMarksFadeNotCut`: right after touch-down the marks exist with alpha between 0 and 1 (a fade in progress), and 0.5 s after the lift they still exist. The real check of feel is Task 27's side-by-side.
- Run: Kit filter `HeadMotionTests` — fails: no `HeadMotion`; then passes.
- Do: one `HeadMotion` holds every timing; each view reads it. Marks and ring fade with a scale (`UIViewPropertyAnimator`), never a cut.
- Done when: the test passes, and the owner's side-by-side check (Task 27) finds the same motion.
- Commit: `feat(ios): the head's reveals and hides move as the mockup does`

### Task 22: the phone's key runs no menu or choose (built after Task 24)
- Files: `cmd/matchblox/gate.go`, `cmd/matchblox/gate_test.go`.
- Surface: the phone's key.
- Test first: `TestGatePhoneHeadCommandsOnly` flips: `matchblox menu …` and `matchblox choose …` are refused in phone mode; `matchblox sessions --json` still passes; all hostile lines stay refused.
- Run: `go test ./cmd/matchblox -run TestGate` — fails: menu and choose pass; then passes.
- Do: `headCommand` accepts only `sessions --json`. The CLI keeps `menu` and `choose`.
- Done when: the gate tests and the full Go suite pass.
- Commit: `fix(gate): the phone's key runs only sessions for the head`

### Task 23: VoiceOver pushes the head
- Files: `DockHeadView.swift`, `HeadVoice.swift`, `HeadVoiceTests.swift`, `HeadTests.swift`.
- Test first: Kit `HeadVoiceTests` — a swipe up sends `↑`, down sends `↓`, at every screen; a double-tap is Enter; custom actions are the four marks and "sessions". UI `testHeadAccessibility` reads the new `actions=`.
- Run: Kit filter `HeadVoiceTests` — fails: a swipe moves Pocket's highlight; then passes.
- Do: the adjustable head sends arrows; `HeadVoice.swipe` takes no `MenuSight` and returns the arrow to send; the menu read leaves.
- Done when: the tests pass.
- Commit: `feat(ios): VoiceOver swipes push the head`

### Task 24: what the joystick no longer needs goes
- Files: the dock's menu read (`DockHeadView`), `MenuSight.swift`/`MenuSightTests.swift` if nothing else calls them, `HeadHint` pick and stale lines, `HostCommand` `menu`/`choose` callers, docs strings.
- Check first: `rg -n 'MenuSight|questionChanged|choose ' ios/` finds callers before and none after; the `iOS` build fails if a deleted symbol is still called; the Kit and UI suites stay green.
- Do: delete the dead paths (deslop). Keep `matchblox menu`/`choose` in the CLI and its docs.
- Commit: `refactor(ios): the head reads no menu`

### Task 25: docs and the landing say Joystick
- Files: `docs/phone.md`, `docs/keys.md`, `docs/console.md`, `www/index.html`, `CHANGELOG.md`.
- Do: `docs/phone.md` describes `Joystick` as a layout to pick, its push/tap/roll/hold, and that other layouts keep the small head; the phone's key runs the stream, the console and `sessions --json`; the landing line names the joystick; the changelog entry says so (≤ 7-word headline). `mkdocs build --strict`, the docs tests and the landing guard pass.
- Commit: `docs: Joystick is a layout you pick`

### Task 26: rebase and full CI
- Rebase on `origin/main` (signed; force-push with lease needs the owner's yes), then CI green: `CI`, `iOS`, `iOS demo` on the PR.
- Done when: every check on PR #539 is green.

### Task 27: review, then TestFlight
- Load `builder:review` on the whole branch against the spec, with the test command; fix each Critical and Important finding; then a TestFlight build from the branch and the owner's phone checks, the mockup beside the phone.
- Done when: no open Critical or Important finding, and the owner has run the done-when checks.

### Revision 2: spec check

| Spec line | Task |
|---|---|
| Layouts offers Joystick; other layouts as before | Task 17, Task 18 |
| Push ↓ moves ❯; tap answers | Task 19, Task 20 `testPushDownMovesTheAgent` |
| Held push repeats; rearm without lift | Task 19 `testRearmUnderTen`, Task 20 `testPushHeldRepeats` |
| ↑ past the first option reads up; tap comes back | Task 20 `testPushUpPastFirstReadsUp` |
| Roll scrolls, flick coasts, no key | Task 20 `testRollSendsNoKey`, `testFlickCoasts` |
| ← → cursor; ⌨ on the belt; fold after Enter | Task 18, Task 20, revision 1 Task 12 |
| Hold → sessions, push to choose | Task 20 `testPushSteps` |
| Looks and moves as the mockup | Task 21, Task 27 phone check |
| Home sets a mark and the colour | revision 1 Task 13 (marks list gains ↑ ↓ in Task 19) |
| VoiceOver | Task 23 |
| Docs and landing say Joystick | Task 25 |
| Anti: a roll sends a key, or a tap after a push sends Enter | Task 19 `testTapIsEnterOnlyStill`, Task 20 `testRollSendsNoKey` |
| Anti: a push sends a second key | Task 19 `testOnePushOneKey` |
| Anti: the owner loses the old belt | Task 17 |
| Surface: an agent's tools | Tasks 19, 20 |
| Surface: the phone's key | Task 22 |

### Revision 2: cold read

Cold read: 2026-11-27, 1 fresh reader. Blocker 1, major 8, nit 1. Fixed 10: the agent's mark comes from `ScreenMenu` on the visible screen (Task 20); `HeadWalk` becomes `HeadScroll` and `HeadVoice.swipe` drops `MenuSight` (Tasks 20, 23); fold and belt-forward only on `Joystick` (Task 18); live layout switching carries and drops the head (Task 18); the exact `Joystick` key row (Task 18); `testHeadIsOnEveryLayout` replaced (Task 18); `Send`/`Lift`, dominant axis, push cancels hold, `testPushSteps` defined (Tasks 19, 20); build-9 marks kept where the owner set them, `↑ ↓` in `edgeKeys` (Task 19); motion test pins numbers plus a fade check (Task 21); Task 22 after Task 24, Task 24's check named, Task 17's UI check named.
