Spec: docs/superpowers/specs/2026-11-27-one-match-head-design.md
Spec approved: 2026-11-27
Test command: go vet ./... && go test -race ./... (host, from a shell outside tmux with SSH_AUTH_SOCK unset) · swift test --package-path ios/MatchbloxKit and make -C ios build (macOS only: the `iOS` workflow on the pushed branch; this host has no Swift toolchain) · make -C ios uitest DESTINATION="id=<udid>" (the `iOS demo` workflow)
Plan approved: 2026-11-27 by the owner
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
