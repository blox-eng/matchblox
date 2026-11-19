Spec: docs/superpowers/specs/2026-11-19-matchhead-wheel-design.md
Spec approved: 2026-11-19
Test command: swift test --package-path ios/MatchbloxKit (CI job `ios`, macOS; then `make -C ios build`)
Plan approved: 2026-11-19 by the owner
Next: builder:build

# Plan: the owner scrolls by feel with a match head on the belt

What the code has today (read 2026-11-19):
- `KeyCatalog` (`BeltLayouts.swift`) already feeds the Mine editor with groups Steer, Edit, Digits, Ctrl and the agents' commands. `Mine.toggle` adds a key at the end. Only Mine is editable.
- `BoardBelt` holds the console keys with words (`g chain`, `m menu`, `⌃o expand` …) as `board.<token>` ids. Act keys send a tap APC that tmux hands to the console, never to the agent.
- `ScrollBack` (#436) counts tmux wheel steps; `jumpToBottom()` sends them back down, and tmux leaves copy mode at the bottom. A belt key today only calls `forgetScrollBack()`, so the pane stays in copy mode.
- `WheelScroll` turns a swipe into wheel steps (1 line a step, 5 in tmux) and coasts 0.5 s. Ticks play only while the console shows.
- This Linux host has no Swift. Every test runs in CI (`ios.yml`: `swift test`, then `make -C ios build`). Device checks run on the owner's iPhone from a TestFlight or Xcode build.

## File map

- `ios/MatchbloxKit/Sources/MatchbloxKit/MatchHead.swift` (create) — the head's physics: roll to notches, speed to lines a notch, the tap rate cap, heat, strike and cool, lines to wheel events.
- `ios/MatchbloxKit/Tests/MatchbloxKitTests/MatchHeadTests.swift` (create) — tests of `MatchHead.swift`.
- `ios/MatchbloxKit/Sources/MatchbloxKit/HeadRoute.swift` (create) — where a notch goes: mouse wheel, SwiftTerm scrollback, or nowhere, and when it is the end.
- `ios/MatchbloxKit/Tests/MatchbloxKitTests/HeadRouteTests.swift` (create) — tests of `HeadRoute.swift`.
- `ios/MatchbloxKit/Sources/MatchbloxKit/BeltLayouts.swift` (change) — the head key and the Console group in the catalog.
- `ios/MatchbloxKit/Tests/MatchbloxKitTests/BeltLayoutsTests.swift` (change) — tests of the new catalog rows.
- `ios/MatchbloxKit/Sources/MatchbloxKit/WheelScroll.swift` (change) — the fast swipe: a rested touch gets the head's speed-up.
- `ios/MatchbloxKit/Tests/MatchbloxKitTests/WheelScrollTests.swift` (change) — tests of the fast swipe.
- `ios/Matchblox/HeadView.swift` (create) — draws the head (sphere, speckles, heat) and plays its haptics.
- `ios/Matchblox/BeltView.swift` (change) — places the head piece, gives it the touches that start on it, dims console keys outside the console on every layout.
- `ios/Matchblox/ConsoleTerminalView.swift` (change) — takes the head's notches, ticks in tmux, the fast swipe.
- `ios/Matchblox/ConsoleView.swift` (change) — wires the head to the terminal; a belt key after a tmux scroll jumps to the bottom first.
- `ios/Matchblox/BeltEditor.swift` (change) — shows the head's face in the editor grid.
- `docs/phone.md`, `docs/keys.md`, `DESIGN.md` (change) — the head, its haptics and the fast swipe.
- `CHANGELOG.md`, `www/` (change) — the release line and the landing page (feat PR rule, AGENTS.md).

## Tasks

### Task 1: a roll turns into notches, and speed into lines a notch

- **Files:** `MatchHead.swift`, `MatchHeadTests.swift`.
- **Test first:**
  - `test_tenPointsIsOneNotch`: `var h = MatchHead(); h.roll(dy: 25, speed: 100)` returns 2 notches down; `h.roll(dy: 5, speed: 100)` returns 1 more (carry of 5 pt).
  - `test_upRollIsUpNotches`: `roll(dy: -20, …)` returns 2 notches up. Down shows older lines, as `WheelScroll` (finger down = wheel up).
  - `test_linesPerNotch`: `MatchHead.lines(atSpeed:)` is 1 at 0, 1 at 250, 10 at 2400, 10 at 5000; strictly rising between 250 and 2400.
- **Run:** `swift test --package-path ios/MatchbloxKit --filter MatchHeadTests` — before: does not compile (no `MatchHead`); after: passes.
- **Do:** `public struct MatchHead` with `notch: CGFloat = 10`, `mutating func roll(dy: CGFloat, speed: CGFloat) -> Roll` where `Roll` holds `notches: Int` (sign = direction, WheelScroll's sense) and `lines: Int` (`notches.magnitude * lines(atSpeed:)`). `static func lines(atSpeed:) -> Int` = `1 + round(9 * t²)`, `t = clamp((|v| − 250) / 2150, 0, 1)`.
- **Done when:** the three tests pass.
- **Commit:** `feat(ios): the match head turns a roll into notches and lines`

### Task 2: taps never come faster than 30 a second

- **Files:** `MatchHead.swift`, `MatchHeadTests.swift`.
- **Test first:** `test_tapRateCap`: notches at t = 0, 0.010, 0.020, 0.034 s (`h.tap(at:)`) give taps true, false, false, true. `test_everyNotchTapsWhenSlow`: notches 0.1 s apart all tap.
- **Run:** same filter — before: fails (no `tap(at:)`); after: passes.
- **Do:** `mutating func tap(at time: TimeInterval) -> Bool`: true when `time − lastTap ≥ 1/30`; then `lastTap = time`.
- **Done when:** both tests pass.
- **Commit:** `feat(ios): the match head caps its taps at 30 a second`

### Task 3: the head heats, strikes and cools

- **Files:** `MatchHead.swift`, `MatchHeadTests.swift`.
- **Test first:**
  - `test_heatFollowsSpeed`: after `roll(…, speed: 1300)`, `heat` is 0.5; at 0 it is 0; at 5200 it is 1.
  - `test_strikesOnce`: a roll at 2400 pt/s sets `struck` and returns `Roll.strike == true`; a second roll at 2500 returns `strike == false` (one rigid tap per strike).
  - `test_coolsWithinASecond`: after a strike, 60 calls of `cool(frame: 1/60)` give `heat < 0.05` and `struck == false`.
- **Run:** same filter — before: fails; after: passes.
- **Do:** `heat = min(1, |v| / 2600)` on each roll; `struck` turns true when heat ≥ 0.92 and was false, and `Roll.strike` reports that edge. `cool(frame:)` multiplies heat by `0.94^(frame·60)`; `struck` clears below heat 0.5.
- **Done when:** the three tests pass.
- **Commit:** `feat(ios): the match head heats, strikes and cools`

### Task 4: lines become wheel events, with tmux's 5-line step

- **Files:** `MatchHead.swift`, `MatchHeadTests.swift`.
- **Test first:** `test_eventsCarryLines`: `var b = WheelBurst(linesPerStep: 5)`; `b.events(lines: 3)` = 0, `b.events(lines: 3)` = 1, `b.events(lines: 10)` = 2. `test_consoleOneLineEach`: `linesPerStep: 1`, `events(lines: 4)` = 4. Negative lines give negative events.
- **Run:** same filter — before: fails; after: passes.
- **Do:** `public struct WheelBurst` with a carry, as `WheelScroll.move` carries points.
- **Done when:** both tests pass.
- **Commit:** `feat(ios): the match head's lines become wheel steps`

### Task 5: a notch knows where it goes and when it is the end

- **Files:** `HeadRoute.swift`, `HeadRouteTests.swift`.
- **Test first:** `HeadRoute.route(readsMouse:inTmuxPane:canScroll:position:scrollBackSteps:down:)`:
  - mouse on, not tmux → `.wheel`.
  - tmux, `scrollBackSteps == 0`, notch toward newer → `.end` (the live bottom); toward older → `.wheel`.
  - mouse off, `canScroll`, position 0, toward older → `.end`; position 0.5 → `.scrollback`.
  - mouse off, no scrollback → `.nothing`.
- **Run:** `--filter HeadRouteTests` — before: no type; after: passes.
- **Do:** a pure enum function with those rows. The tmux top cannot be known: older in tmux is always `.wheel`.
- **Done when:** all rows pass.
- **Commit:** `feat(ios): the match head knows the ends of the scrollback`

### Task 6: the editor offers the head and the console's keys

- **Files:** `BeltLayouts.swift`, `BeltLayoutsTests.swift`, `BeltEditor.swift`.
- **Test first:**
  - `test_groupsStartWithScroll`: `catalog.groups.first` is `("Scroll", [BeltKey(id: "Wheel", cells: 1)])`.
  - `test_consoleGroup`: a group `"Console"` holds `board.c, board.ctrl+o, board.s, board.g, board./, board.m, board.z, board.N, board.tab, board.x`, in `BoardBelt`'s tail order, with cells from their labels.
  - `test_mineKeepsHeadAndConsoleKeys`: `catalog.keys(.mine, agent: nil, mine: [◂, Wheel, board.g])` returns all three; encode and decode of `Mine` keeps them.
  - `test_wheelSendsNothing`: `KeyBar.bytes(for: "Wheel") == nil` (it starts with a capital, so it is no command id).
- **Run:** `--filter BeltLayoutsTests` — before: fails; after: passes.
- **Do:** add `KeyCatalog.scroll = [BeltKey(id: "Wheel", cells: 1)]` and `KeyCatalog.console` from a new `BoardBelt.tailKeys` (public, the existing `tail` mapped by `key(_:)`). Put both in `known` and in `groups` (Scroll first, Console after Ctrl). In `BeltEditor`, the `Wheel` cell shows a small match head, and its VoiceOver label is "Scroll wheel".
- **Done when:** the four tests pass, and the editor shows Scroll and Console groups (device).
- **Commit:** `feat(ios): the belt editor offers the match head and the console's keys`

### Task 7: console keys on Mine follow the console's dim rule

- **Files:** `BeltView.swift`.
- **Test first:** the dim rule is `BoardBelt.away.isDim(_:)`, already pure. Add `test_awayDimsConsoleKeys` in `BoardBeltTests`: `.away.isDim("board.g")` is true, `.plain.isDim("board.g")` is false.
- **Run:** `--filter BoardBeltTests` — before: the new test passes already (it pins the rule); the app change follows.
- **Do:** `isBoardDim(_:)` drops `layout == .board &&` for ids that start with `board.`, so on Mine, `g chain` is dim with "this key is off here" while the console is not shown.
- **Done when:** the test passes; on the device, `g chain` on Mine is dim in an agent pane and acts in the list.
- **Commit:** `feat(ios): console keys on your own layout follow the console`
- **Surface:** an agent's tools. Hostile input: a tap on `x kill` on Mine inside an agent pane sends nothing (dim), checked on the device.

### Task 8: the head draws and rolls on the belt

- **Files:** `HeadView.swift`, `BeltView.swift`, `ConsoleTerminalView.swift`, `ConsoleView.swift`.
- **Test first:** the physics and the routing are tested in Tasks 1–5. This task wires them; its check is `make -C ios build` in CI and the device checks below.
- **Do:**
  - `HeadView` (a `CALayer` piece in the belt's ring, 1 cell): a sphere with 70 speckles that turn with the roll (rotation about x by `dy / 34` rad, about y by `dx / 34`), coloured by heat from `match-head` through `match-ember`, `flame-amber` to `flame-core`; a glow when struck. With Reduce Motion: no turning speckles; heat colour stays.
  - `BeltView`: a touch that starts on the head piece is the head's. The belt's pan does not begin for it (`gestureRecognizerShouldBegin`), and a lift without a roll sends nothing. Its drag drives `MatchHead.roll`; a lift above 150 pt/s coasts with `WheelScroll.coast` for `coastTime`; a touch stops the coast. New callback `onHead: (Roll) -> HeadRoute.Result`.
  - `ConsoleTerminalView.head(_ roll:) -> HeadRoute.Result`: `.wheel` sends `WheelBurst` events through `sendWheel` (and counts them in `scrollBack` in tmux); `.scrollback` moves `scroll(toPosition:)` by `lines / scrollback lines`; `.end` and `.nothing` move nothing. Sideways: buttons 6 and 7 only with the mouse on and not in a tmux pane; else nothing.
  - `.nothing` flashes `nothing scrolls here · this program has no scrollback` on the belt (`flash`), once per touch.
- **Done when (device):** in tmux with 300 lines, a roll up reaches the first line; a roll down at the bottom stops the head; in a plain shell the head stops at the top.
- **Commit:** `feat(ios): the match head rolls the terminal from the belt`

### Task 9: you feel every notch, the strike and the end

- **Files:** `HeadView.swift`.
- **Test first:** the tap cap is Task 2's test. This task's check is the device.
- **Do:** Core Haptics (`CHHapticEngine`): a notch is one transient, intensity 0.5 + 0.4·heat, sharpness 0.6 + 0.4·heat, played only when `tap(at:)` is true; a strike is one transient at intensity 1, sharpness 0.3 (the rigid tap); `.end` is one transient at intensity 1, sharpness 0.1, once per touch (the heavy bump). With no haptics hardware, nothing plays and nothing fails. Haptics play with Reduce Motion (DESIGN.md §5).
- **Done when (device):** slow roll: a tap per notch; hard roll: distinct taps, no buzz; strike: one hard tap; end: one bump.
- **Commit:** `feat(ios): the match head taps each notch, the strike and the end`

### Task 10: VoiceOver can scroll with the head

- **Files:** `HeadView.swift`, `BeltView.swift`.
- **Test first:** device check (VoiceOver).
- **Do:** the head is an accessibility element, label "Scroll wheel", trait adjustable; `accessibilityIncrement` scrolls one page toward newer lines, `accessibilityDecrement` one page toward older (rows of the terminal, through `onHead`).
- **Done when (device):** VoiceOver reads "Scroll wheel, adjustable", and a swipe up or down on it scrolls a page.
- **Commit:** `feat(ios): VoiceOver scrolls with the match head`

### Task 11: a belt key after a tmux scroll goes to the agent, not to copy mode

- **Files:** `ConsoleView.swift`.
- **Test first:** `ScrollBackTests.test_jumpReturnsEveryStep` already pins that `jump()` returns the steps up. Add `test_jumpAfterHeadBurst`: `wheeled([.up × 7])`, then `jump()` returns 7 `.down` and `isUp` is false.
- **Run:** `--filter ScrollBackTests` — passes before and after (it pins the rule the app uses).
- **Do:** in `Belts`, `belt.onSend` calls `terminal.jumpToBottom()` when the terminal is scrolled back in a tmux pane, then sends the key. Keyboard typing keeps today's `forgetScrollBack()`.
- **Done when (device):** roll up in a tmux pane with Claude Code, tap `Esc` then `q` on the belt: the agent's line shows `q`; tmux shows no copy-mode search or exit.
- **Commit:** `fix(ios): a belt key after a scroll reaches the agent, not tmux copy mode`
- **Surface:** an agent's tools. Hostile input: `q` and `/` after a scroll, the two keys tmux copy mode acts on.

### Task 12: a swipe ticks in tmux, and a rested swipe is fast

- **Files:** `WheelScroll.swift`, `WheelScrollTests.swift`, `ConsoleTerminalView.swift`.
- **Test first:** `test_restedSwipeIsFast`: `WheelScroll.isFast(restedFor: 0.30)` is true, `0.20` false. `test_fastMultipliesSteps`: a wheel with `boost: 4` moves 4 steps for one cell of travel.
- **Run:** `--filter WheelScrollTests` — before: fails; after: passes.
- **Do:** `WheelScroll.fastRest = 0.25`; `init(…, boost: Int = 1)`; `move` returns `steps × boost`. In `ConsoleTerminalView`, record the touch-down time in `touchesBegan`; at the wheel pan's `.began`, `boost = isFast(restedFor: now − down) ? MatchHead.lines(atSpeed: velocity) : 1`, updated on each `.changed`. Drop the `showsConsole` gate on `tick` so tmux ticks too, rate-capped as Task 2. A still hold does not begin the pan, so the row menu at 0.48 s and SwiftTerm's selection keep working.
- **Done when (device):** on a layout with no head, a hold then a swipe scrolls fast; a quick swipe scrolls as today with ticks in tmux; a still hold opens the row menu.
- **Commit:** `feat(ios): a swipe ticks in tmux, and a rested swipe scrolls fast`

### Task 13: the docs, the changelog and the landing page say it

- **Files:** `docs/phone.md`, `docs/keys.md`, `DESIGN.md`, `CHANGELOG.md`, `www/`.
- **Test first:** `.github/scripts/landing-guard.sh "feat: …" <body-file> origin/main` fails before the `www/` change; passes after.
- **Do:** `phone.md`: the head (add it in the editor, roll, heat, strike, ends), the fast swipe, ticks in tmux. `keys.md`: the Console group. `DESIGN.md §5`: haptics for a notch, a strike and the end. `CHANGELOG.md` Unreleased: headline ≤ 8 words, "Roll a match head to scroll". `www/`: one line and the head in the phone picture.
- **Done when:** the landing guard passes, and the docs name every behaviour of the spec's experience.
- **Commit:** `docs: the match head in the phone guide, keys and design`

### Task 14: review the whole change

- Load `builder:review` on the branch against `main`, with the spec `docs/superpowers/specs/2026-11-19-matchhead-wheel-design.md` and the test command above. CI green (`swift test` and `make -C ios build`), then the device checks below on the owner's iPhone.

## Lens findings

- **Scope:** 9 code files and 2 new parts (`MatchHead`, `HeadRoute`), above the 8-file smell. The spec needs the editor, the belt, the terminal and the wiring; each file has one job. Cut: a separate head layout, a settings row, sideways in tmux. No new dependency (Core Haptics ships with iOS).
- **Reuse:** `WheelScroll.coast` for the flick, `ScrollBack.jump()` for copy mode, `BoardBelt` faces for the Console group, `flash` for the notice, the match tokens for heat.
- **Architecture:** the kit owns the numbers (notches, lines, heat, routes); the app owns drawing, touches and haptics. One owner for scroll state: `ConsoleTerminalView`.
- **Production failure:** a phone with haptics off or no engine — Task 9 plays nothing and fails nothing. A Mine layout saved before this change — `Mine` decoding keeps unknown-free old rows (Task 6 round-trip). A tmux pane whose top is unknown — Task 5 never bumps at the tmux top, as the spec says.
- **Regression:** the belt's pan (Task 8 gives only touches that start on the head to the head; the existing belt tests stay); the row menu and selection (Task 12 device check); `forgetScrollBack` on typing (unchanged).
- **Performance:** per frame, 70 speckles and one gradient on one layer; per notch, one haptic at most 30/s. Measured on the device in Task 9.
- **Design, every state:** head at rest (cold red), rolling (heat), struck (glow), coasting, at the end (bump, stops), nothing to scroll (notice), VoiceOver (adjustable). Notice words rated 8: they say what and why; the fix is to use the program's own keys, which needs no words.
- **Not testable here:** this host has no Swift; all tests run in CI, and the feel runs only on the iPhone. No change, because there is no other place to run them.

## Spec check

| Spec line | Task |
|---|---|
| The owner adds the head in the editor and sees it on the belt | Task 6, Task 8 |
| A roll up in tmux reaches the first line, a tap per notch | Task 8, Task 9 |
| Slow roll 1 line a notch; hard roll many lines, taps distinct | Task 1, Task 2, Task 9 |
| A hard roll lights the head with one rigid tap, dark within 1 s | Task 3, Task 9 |
| Plain shell top: one heavy bump, the head stops | Task 5, Task 8, Task 9 |
| `g` after a tmux scroll goes to the agent, not copy mode | Task 11 |
| No head: hold then swipe is fast; a still hold opens the row menu | Task 12 |
| The editor lists `g chain`, `m menu`, `ctrl+o expand`, a tap adds each | Task 6 |
| VoiceOver: "Scroll wheel", a swipe scrolls a page | Task 10 |
| Anti-goal: a tap without a scroll, or a scroll without a tap | Task 5 (`.nothing` plays no tap), Task 9 |
| Anti-goal: taps blur into a buzz | Task 2, Task 9 |
| Anti-goal: a key the owner did not tap reaches the agent | Task 7, Task 11 |
| Anti-goal: a strike that never cools | Task 3 |
| Anti-goal: a hold meant for the menu starts the fast scroll | Task 12 |
| Surface: an agent's tools | Task 7, Task 11 |

## Changes during build

- Task 5: the route's direction argument is `older:` (true: toward older lines), not `down:`. It names what the person sees, so a reader does not need WheelScroll's sign rule. (2026-11-20)
- Task 6: the head's id is `KeyBar.wheel` ("Wheel") and its bytes are `[]`, not nil, as Ctrl's are. `testEveryKeyHasBytes` pins that every catalog key has bytes; Ctrl is the precedent for a key that sends nothing itself. (2026-11-20)
- Task 12: `boost` is a settable property of `WheelScroll`, not an init argument: the pan updates it on each change from the finger's speed. `MatchHead.tapGap` is public, so the swipe's ticks share the head's 30-a-second cap. (2026-11-20)
- Task 8: a sideways roll sends xterm's wheel codes 66 and 67 by hand. SwiftTerm 1.20.0's `encodeButton` maps buttons 6 and 7 to 0, a left click. (2026-11-20)
- Review fixes (2026-11-20): the sideways wheel is cut (SwiftTerm's SGR encoding turns 67 into a wheel-up release); `press` ignores the head; a key or typed text stops the head's coast; a rested flick coasts at ×1; `Esc` after a tmux scroll only jumps to the bottom; `WheelBurst` rounds each notch up to one step instead of carrying lines, so every tap moves the pane.
