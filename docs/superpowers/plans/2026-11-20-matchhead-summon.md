Spec: docs/superpowers/specs/2026-11-20-matchhead-summon-design.md
Spec approved: 2026-11-20
Test command: swift test --package-path ios/MatchbloxKit (CI; locally the Docker kit harness `scratchpad/kitlin/run.sh`), go test ./internal/privacycheck/..., make -C ios build (CI)
Plan approved: 2026-11-20 by the owner
Next: builder:build

# Plan: call up the match head anywhere, and click it for Enter

What the code says today:
- The belt head's touch, roll, coast, cooling and drawing live inside `BeltView` (`headBegan`/`headMoved`/`headEnded`/`roll`/`headStepped`). A second head needs the same loop, so it moves into one driver first.
- `BeltView.press` ignores the head (`guard piece.head == nil`), so a tap on it sends nothing.
- `ConsoleTerminalView.wheelPanned` makes a rested swipe fast (`WheelScroll.isFast`, `wheel.boost`). The wheel pan begins only where the program reads the mouse; elsewhere SwiftTerm's scroll view scrolls.
- `ConsoleView` wires `belt.onSend`, which first brings a scrolled tmux pane back to its bottom. A click's Enter takes the same path.
- `ProcessInfo.systemUptime` (boot-time API) and `UserDefaults` (app, kit, widget) are required-reason APIs. No target ships a `PrivacyInfo.xcprivacy`.

## File map

- `ios/MatchbloxKit/Sources/MatchbloxKit/HeadClick.swift` (create) — decides whether a touch on the head is a click.
- `ios/MatchbloxKit/Tests/MatchbloxKitTests/HeadClickTests.swift` (create) — its tests.
- `ios/MatchbloxKit/Sources/MatchbloxKit/HeadSummon.swift` (create) — where a called-up head sits, and when it goes.
- `ios/MatchbloxKit/Tests/MatchbloxKitTests/HeadSummonTests.swift` (create) — its tests.
- `ios/MatchbloxKit/Sources/MatchbloxKit/WheelScroll.swift` (change) — `boost` goes: no swipe scrolls fast any more. `fastRest`/`isFast` stay: they now mean "call up the head".
- `ios/MatchbloxKit/Tests/MatchbloxKitTests/WheelScrollTests.swift` (change) — the boost test goes.
- `ios/Matchblox/HeadDriver.swift` (create) — one head's touch, roll, coast, cooling, click and drawing, for any view.
- `ios/Matchblox/HeadView.swift` (change) — `HeadFeel.click()`, and a short flash on `HeadLayer`.
- `ios/Matchblox/BeltView.swift` (change) — the belt head runs on `HeadDriver`; its click sends Enter.
- `ios/Matchblox/SummonedHead.swift` (create) — the called-up head over the terminal: shows, waits, fades.
- `ios/Matchblox/ConsoleTerminalView.swift` (change) — a rested vertical move calls up the head and hands it the finger.
- `ios/Matchblox/ConsoleView.swift` (change) — one `send(_:)` for belt keys and clicks; places the called-up head on the screen.
- `ios/Matchblox/PrivacyInfo.xcprivacy`, `ios/MatchbloxIsland/PrivacyInfo.xcprivacy` (create) — the required-reason declarations.
- `internal/privacycheck/privacycheck.go`, `privacycheck_test.go` (create) — fails when a target's Swift uses a required-reason API that its manifest does not declare, or declares one it does not use.
- `.github/workflows/ios.yml` (change) — after the build, the app and the widget bundles must hold their manifests.
- `www/assets/match-head.svg` (create) — the head drawing, one file for the site and the docs.
- `www/index.html`, `www/styles.css` (change) — the Pocket section shows the head and names the click and the call-up.
- `docs/phone.md`, `docs/keys.md`, `DESIGN.md`, `CHANGELOG.md`, `.github/workflows/docs.yml` (change) — words, the drawing, the haptic, the changelog.

## Task 1: a touch on the head is a click only when the head was still

Surface: an agent's tools
- **Files:** `HeadClick.swift`, `HeadClickTests.swift`.
- **Test first:** hostile inputs first, each asserts `isClick == false`:
  - `testTouchThatStopsACoastIsNoClick`: `began(at: 10, coasting: true, lastRoll: 9.9)`, `ended(at: 10.1)`.
  - `testShortLiftAfterARollIsNoClick`: `began(at: 10, coasting: false, lastRoll: 9.95)`, `ended(at: 10.08)`.
  - `testRollDuringTheTouchIsNoClick`: `moved(by: 7)` (past the 6 pt slop), `ended` at 0.1 s.
  - `testLongPressIsNoClick`: still, `ended` 0.5 s after `began`.
  Then `testStillTapIsAClick`: `began(at: 10, coasting: false, lastRoll: 9.0)`, `moved(by: 2)`, `ended(at: 10.12)` → true.
- **Run:** the kit harness. Before: fails to compile, `HeadClick` missing. After: 5 tests pass.
- **Do:** `public struct HeadClick` with `static let still: TimeInterval = 0.3`, `slop: CGFloat = 6`, `press: TimeInterval = 0.35`. `mutating began(at:coasting:lastRoll:)`, `mutating moved(by distance: CGFloat)` (adds the absolute travel), `func isClick(endedAt:) -> Bool`: not coasting at touch, `lastRoll` older than `still` before the touch, travel under `slop`, held under `press`.
- **Done when:** the 5 `HeadClickTests` pass.
- **Commit:** `feat(ios): a still tap on the match head is a click`

## Task 2: the called-up head sits by the finger, above the keyboard, and waits 3 s

- **Files:** `HeadSummon.swift`, `HeadSummonTests.swift`.
- **Test first:**
  - `testSitsUnderTheFinger`: finger (200, 300) in bounds 400×800, size 56, no keyboard → center (200, 300).
  - `testStaysInsideTheEdges`: finger (5, 5) → center (28, 28).
  - `testStaysAboveTheKeyboard`: finger (200, 700), keyboard top 600 → center.y 572.
  - `testStaysInsideTheSafeArea`: bounds = safe area (0, 47, 390, 763), finger (200, 40) → center.y 75.
  - `testShiftsClearOfTheJumpButton`: finger on the ↓ button's frame (avoid list) → the head's circle does not overlap that frame.
  - `testWaitsThreeSecondsAfterTheLastTouch`: `isGone(lastTouch: 10, now: 12.9)` false; `now: 13.0` true.
- **Run:** kit harness. Before: compile fails. After: 4 pass.
- **Do:** `public enum HeadSummon` with `static let wait: TimeInterval = 3`, `static let size: CGFloat = 56`, `static func center(finger:bounds:keyboardTop:) -> CGPoint` (clamped by half the size), `static func isGone(lastTouch:now:) -> Bool`.
- **Done when:** the 4 `HeadSummonTests` pass.
- **Commit:** `feat(ios): the called-up match head sits by the finger`

## Task 3: one head driver for the belt and the screen

- **Files:** `HeadDriver.swift` (create), `BeltView.swift`.
- **Test first:** no unit test can see a move of UIKit code with no change in behaviour. Check instead: `rg -n 'headStepped|headCoast|func roll\(dx' ios/Matchblox/BeltView.swift` finds 3+ lines before and 0 after; the kit harness still passes; CI `make -C ios build` passes.
- **Do:** `@MainActor final class HeadDriver` owns `MatchHead`, `HeadFeel`, the turn, the coast, the display link and the touch. Input: `began(at point:, time:)`, `moved(to:time:)`, `ended()`, `cancelled()`, `stop()`, `page(older:)`. Output closures: `onLines: (Int) -> HeadRoute`, `draw: (turn: CGPoint, heat: CGFloat, struck: Bool) -> Void`, `say: (String) -> Void`. Move the code from `BeltView` unchanged; `BeltView` keeps hit-testing and VoiceOver, and calls the driver.
- **Done when:** the check above holds, and the belt head rolls, coasts and cools as before (CI build; device check in the review).
- **Commit:** `refactor(ios): the match head runs on one driver`

## Task 4: a click on the belt head sends Enter

Surface: an agent's tools
- **Files:** `HeadDriver.swift`, `HeadView.swift`, `BeltView.swift`, `ConsoleView.swift`.
- **Test first:** the click rules are Task 1's tests. Here: `HeadDriver` feeds `HeadClick` from its touch (`coasting` = a coast ran at touch down; `lastRoll` = the last notch's time; `moved` = finger travel). The hostile cases already fail a click in Task 1. Check: `rg -n 'HeadClick' ios/Matchblox/HeadDriver.swift` finds the use; CI build passes.
- **Do:** add `onClick: () -> Void` to `HeadDriver`; on `ended()`, when `isClick`, call it, play `HeadFeel.click()` (design review 4A: two transients 60 ms apart, intensity 0.9, sharpness 0.6), and flash the head (`HeadLayer.flash()`: a `flame-core` ring at the rim for 140 ms, never the strike glow). DESIGN.md §5 gains both. A touch that stops a coast plays `HeadFeel.stop()` (design review 5A: one transient, intensity 0.4, sharpness 0.1) and no flash; `docs/phone.md` says "a touch stops it; tap again for Enter". `BeltView` sets `onClick = { onSend([0x0d]) }`. In `ConsoleView`, move the `belt.onSend` body into `private func send(_ bytes: [UInt8])` and point `belt.onSend` at it. Update the `press` comment: a tap on the head goes to the driver. VoiceOver (design review 10A): the belt head's label becomes "Match head" and its hint "Swipe up or down to scroll. Double-tap to send Enter."; its activate action sends Enter through `send(_:)`. The called-up head is hidden from VoiceOver. `KeyFace` "Wheel" spoken name follows (update `KeyFaceTests`).
- **Done when:** a still tap on the belt head sends `0x0d` through `send(_:)` (device check in review); CI build passes.
- **Commit:** `feat(ios): a click on the match head sends Enter`

## Task 5: a rest, then a move, calls up the head under the finger

Surface: an agent's tools
- **Files:** `SummonedHead.swift`, `ConsoleTerminalView.swift`, `ConsoleView.swift`, `WheelScroll.swift`, `WheelScrollTests.swift`.
- **Test first:** in `WheelScrollTests`, delete `boost`'s test and keep `isFast`'s. Add `testRestThenMoveTimeline` (design review 3A) for `WheelScroll.gesture(restedFor:moved:)`: moved 10 pt after 0.1 s → `.swipe`; after 0.3 s → `.summon`; after 0.5 s → `.menu`; 5 pt (under the 6 pt slop) after 0.3 s → `.rest`. Before: the kit compiles with `boost`. After: `boost` is gone, `rg -n boost ios -g '*.swift'` finds nothing, and the kit passes.
- **Do:**
  - `SummonedHead: UIView` holds a `HeadLayer` (`HeadSummon.size`) and its own `HeadDriver`. `show(at finger:)` places it with `HeadSummon.center` and the keyboard layout guide's top; touches on it go to its driver; each touch, each roll and the end of a coast restart the wait; it goes with a 140 ms `Fade` when `HeadSummon.isGone` (design review 9A); with Reduce Motion it appears and goes at once. DESIGN.md §5 gains the row "Wait | 3 s | the called-up match head, from the last touch or the end of its coast". Its `onClick` and `onLines` come from `ConsoleView` (`send([0x0d])`, `terminal.head(lines:)`).
  - `ConsoleTerminalView`: `wheelPan` may begin when the finger rested (`isFast`) even where the program reads no mouse, never during a selection. A rested pan calls `onSummon(point)` and then forwards `moved`/`ended` to the called-up head's driver instead of sending wheel steps. Delete `fastSwipe` and the boost lines. An unrested swipe scrolls as today.
  - The timeline (design review 3A), one owner per moment: a move before 0.25 s is a swipe; a rest of 0.25–0.48 s, then a move past 6 pt, calls up the head and the hold fails; a still finger at 0.48 s opens the right-click or copy menu, and a later move never calls up the head. `WheelScroll.gesture(restedFor:moved:)` returns `.swipe`, `.rest`, `.summon` or `.menu`.
  - `terminal.onTyped` and `send(_:)` also stop the called-up head's coast.
  - Hit area (design review 2A): only the circle is the head (`point(inside:)` tests the radius). It never sits over ↓ or a notice: `HeadSummon.center` takes their frames and shifts clear (test `testShiftsClearOfTheJumpButton`). A touch outside the circle dismisses the head at once and reaches what it touched. Order: above the terminal, below ↓ and notices.
  - Edge states (design review 6A): a keyboard that rises while the head waits dismisses it at once; the head clamps to the safe area (`bounds` passed in is the safe-area frame); "nothing scrolls here · this program has no scrollback" shows in the screen's `notice`, which sits above the head; an end plays the heavy bump, as on the belt; a call-up works wherever the terminal scrolls (console, chat, pane); a second call-up moves the head, never adds one.
  - Arrival (design review 1A): the moment it appears, one light impact (`HeadFeel.seat()`, intensity 0.5, sharpness 0.5) and a 140 ms `Fade` in (DESIGN.md §5); the ball is 56 pt, so its rim shows around the thumb. DESIGN.md §5 Haptics gains the seat.
- **Done when:** the kit passes with no `boost`; CI build passes; the device checks of the review hold.
- **Commit:** `feat(ios): rest, then move, to call up the match head`

## Task 6: the app declares its required-reason APIs

Surface: personal data
- **Files:** `internal/privacycheck/privacycheck.go`, `privacycheck_test.go`, the two `PrivacyInfo.xcprivacy`, `.github/workflows/ios.yml`.
- **Test first:** `go test ./internal/privacycheck/...`
  - hostile `TestUndeclaredUseFails`: a temp target with `UserDefaults.standard` and an empty manifest → an error naming `NSPrivacyAccessedAPICategoryUserDefaults`.
  - hostile `TestDeclaredButUnusedFails`: a manifest with `NSPrivacyAccessedAPICategoryFileTimestamp` and no file-date call → an error.
  - `TestRepoTargetsMatchTheirManifests`: reads `ios/Matchblox` (+ `ios/MatchbloxKit/Sources`, linked into the app) and `ios/MatchbloxIsland`. Before: fails (no manifest). After: passes.
- **Do:** `privacycheck.Check(swiftDirs []string, manifest string) error` maps symbols to categories: `UserDefaults|@AppStorage` → UserDefaults; `systemUptime|mach_absolute_time` → SystemBootTime; `creationDate|modificationDate|attributesOfItem` → FileTimestamp; `volumeAvailableCapacity|statfs` → DiskSpace; `activeInputModes` → ActiveKeyboards. App manifest: UserDefaults `CA92.1` and `1C8F.1` (the app group), SystemBootTime `35F9.1`; no tracking, no collected data. Widget: UserDefaults `1C8F.1`. Before writing them, clone SwiftTerm 1.20.0 into the scratchpad and grep it for the same symbols; add any category it uses to the app manifest. In `ios.yml`, after the build: `test -f` each bundle's `PrivacyInfo.xcprivacy` and `plutil -lint` it.
- **Done when:** the 3 tests pass; CI's bundle check passes.
- **Commit:** `fix(ios): declare the app's required-reason APIs`

## Task 7: the site and the docs show the head

- **Files:** `www/assets/match-head.svg`, `www/index.html`, `www/styles.css`, `.github/workflows/docs.yml`, `docs/phone.md`, `docs/keys.md`, `DESIGN.md`, `CHANGELOG.md`.
- **Test first:** no test sees prose. Checks: `.github/scripts/landing-guard.sh "feat(ios): …" <body> origin/main` passes; `rg -n 'A tap on the head sends nothing|swipe scrolls fast' docs/` finds lines before and none after; `go test ./internal/docscheck/...` passes; the deny-list grep of the staged diff is empty.
- **Do:** draw the head as a still SVG (design review 7A): 40 px, one fixed turn, about 20 speckles, fills from CSS variables (the cold match-head red over an ember shadow, with the light theme's pair), `alt="The match head"`, never animated, never glowing (DESIGN.md §8: only the hero's match burns). Show it in the "Jump in, answer, out" block at 40 px, with the line (design review 8A) "Tap a session to peek, tap an option twice to answer. Rest a finger to roll the match head; tap it for Enter." The verb is "tap" on every surface. Copy it into the docs build the way `matchblox-mark-dark.svg` is copied, and show it at the top of "The match head" in `docs/phone.md`. Rewrite that section's tap line and the rested-swipe line in `docs/phone.md` and `docs/keys.md`. `DESIGN.md` §5: the click haptic. `CHANGELOG.md` Unreleased: Added "**Tap the match head for Enter.**" and "**Rest a finger to call up the match head.**"; Fixed "**The app declares the APIs Apple asks about.**"
- **Done when:** the checks pass, and both pages show the drawing in a local preview.
- **Commit:** `docs: the site and the docs show the match head`

## Task 8: review

- Load `builder:review` on the branch against `origin/main`, with this plan, the spec and the test command. Then `builder:ship`: PR, CI green, merge, a new `ios-v` tag only with the owner's yes.

## Lens findings

- Engineering: a second head copied from `BeltView` makes two loops to keep in step → added Task 3 (one driver).
- Engineering: `boost` has no use once the rested swipe calls up the head → cut it in Task 5.
- Engineering: a manifest that drifts from the code is the next Apple email → Task 6's guard test fails on both drift directions.
- Design: the called-up head under the finger hides the line under it → it sits at the finger only while touched; after the lift it fades in 3 s (spec anti-goal). No change beyond the spec, because the owner asked for it "wherever I want".
- Design: VoiceOver users cannot rest-and-move → the belt head keeps its adjustable element, and its double-tap sends Enter with a hint that says so (Task 4, design review 10A).
- Design: Reduce Motion → the called-up head appears and fades with no scale animation; the turn follows the existing Reduce Motion rule in the driver.
- Architecture: no API, service boundary or data store changes; the click enters the existing `send(_:)` path → no change.

## Design review: states and journey

| Head | Appears | Rolling | Waiting | Click | Stopped coast | End | Nothing to scroll | Goes |
|---|---|---|---|---|---|---|---|---|
| Belt | always there | notches, heat, strike | — | two taps 60 ms apart, flame-core rim 140 ms, Enter | soft dull tap, nothing sent | heavy bump | handle row message | — |
| Called up | seat tap, 140 ms Fade in, under the finger, clear of ↓ and notices | as the belt | 3 s, in the safe area | as the belt | as the belt | heavy bump | the screen's notice, above the head | 3 s after the last touch or coast; at once on a touch outside it or a rising keyboard |

| Step | The owner does | The owner feels | The plan says |
|---|---|---|---|
| 1 | reads a long reply, wants older lines | "where is it?" | rest, then move, anywhere (Task 5) |
| 2 | the head arrives under the thumb | "it caught" | seat tap, Fade in (1A) |
| 3 | rolls, flicks | control, speed | notches, heat, coast (Task 3) |
| 4 | touches to stop the flick | "stopped, nothing sent" | soft dull tap (5A) |
| 5 | taps the still head | "sent" | double tap, rim flash, Enter (4A) |
| 6 | lifts and reads on | the head gets out of the way | 3 s wait, then fade; outside tap dismisses (2A) |

## Check against the spec

| Spec line | Task |
|---|---|
| A tap on the still head sends Enter | Task 1, Task 4 |
| A flick, then a touch to stop it, sends nothing | Task 1 (`testTouchThatStopsACoastIsNoClick`) |
| Rest, then move, shows the head under the finger; a roll scrolls with notches | Task 2, Task 5 |
| After a lift the head waits, takes a roll or a tap, fades ~3 s after the last touch | Task 2, Task 5 |
| A still hold opens the right-click menu, and no head appears | Task 5 |
| The next TestFlight email names no missing API declaration | Task 6 |
| Site and docs show the drawing and name the click and the call-up | Task 7 |
| Anti-goal: a touch that stops a coast sends Enter | Task 1 |
| Anti-goal: a roll that ends in a short lift sends Enter | Task 1 |
| Anti-goal: the called-up head hides the line and stays | Task 2, Task 5 |
| Anti-goal: right-click menu and head both appear | Task 5 |
| Anti-goal: manifest declares an unused reason or misses a used one | Task 6 |

## Changes during build

- Task 7: the drawing is inline SVG on the page and in `docs/phone.md`, not `www/assets/match-head.svg`: an `<img>` cannot follow the site's light and dark tokens. `TestTheMatchHeadIsOneDrawing` (internal/docscheck) keeps the two copies identical, so the docs workflow needs no copy step.
- Task 6: three manifests, not two: the widget and the notification extension (`MatchbloxBuzz`) link MatchbloxKit, whose Swift uses UserDefaults. SwiftTerm 1.20.0 (`5d14406`) uses none of the required-reason APIs; swift-nio and swift-crypto ship their own manifests. The manifests declare no collected data: that is the App Store label's job, and the owner sets it.
- Task 5: `HeadDriver.began` takes `canClick:`. The terminal's pan begins only after about 10 pt of travel, so a call-up and a quick lift looked like a still tap and sent Enter; a grab from the terminal never clicks, only a fresh touch on the waiting head does.
- Task 5: the console screen (`HeldScreen`) owns the called-up head, places it (`headCenter`), dismisses it in `hitTest`, and gains `say(_:)` for its notice; `BeltView.enter` is shared with it. `WheelScroll.gesture` uses `BoardTouch.longPress` for the menu's moment. A call-up needs a vertical move, as the swipe does, so a sideways drag stays a selection.
- Task 2: `HeadSummon.center` takes `avoid: [CGRect]` (↓, notices) and `HeadSummon.frame(center:)` exists, for design review 2A. Tests import Foundation, not CoreGraphics (Linux has none).

## Design review: NOT in scope

- Mockups of the called-up head: the designer tool was rate limited (429, 0 of 3 saved); the device check after build replaces them.
- A setting for the wait, the size or the click strength: the spec's non-goals keep these fixed.
- A different VoiceOver path to call up the head: the belt head and its actions serve VoiceOver users.

## Design review: what already exists

- `HeadLayer` and `HeadFeel` (`ios/Matchblox/HeadView.swift`): the drawing, heat colours and haptics the called-up head reuses.
- DESIGN.md §5 Motion `Fade` (140 ms) and Haptics; §8 "one fire for each view".
- The console screen's `jump` (↓) button and `notice` label: the head stays clear of them and uses `notice` for messages.
- The editor's cold head cell (`BeltEditor.swift`): the cold gradient the site drawing follows.

## Implementation Tasks

Synthesized from this review's findings. Each one is folded into a plan task above.

- [ ] **T1 (P1, human: ~1h / CC: ~10min)** — HeadSummon — safe-area clamp and shift clear of ↓ and notices (2A, 6A) → Task 2 tests `testStaysInsideTheSafeArea`, `testShiftsClearOfTheJumpButton`.
- [ ] **T2 (P1, human: ~2h / CC: ~15min)** — SummonedHead — circle hit area, outside touch dismisses and passes through, z-order, keyboard dismiss, one head only (2A, 6A) → Task 5.
- [ ] **T3 (P1, human: ~1h / CC: ~10min)** — WheelScroll — the rest/hold timeline with `gesture(restedFor:moved:)` and `testRestThenMoveTimeline` (3A) → Task 5.
- [ ] **T4 (P1, human: ~1h / CC: ~10min)** — HeadFeel/HeadLayer — click double tap + flame-core rim flash; soft stop tap; seat tap (1A, 4A, 5A) → Tasks 4 and 5; DESIGN.md §5 rows.
- [ ] **T5 (P2, human: ~30min / CC: ~5min)** — SummonedHead — Fade in/out 140 ms, Wait 3 s from the last touch or coast end, Reduce Motion instant (9A) → Task 5; DESIGN.md §5 row.
- [ ] **T6 (P1, human: ~30min / CC: ~5min)** — BeltView/KeyFace — VoiceOver label "Match head" and the Enter hint (10A) → Task 4.
- [ ] **T7 (P2, human: ~1h / CC: ~10min)** — site and docs — still cold SVG spec, "tap" as the one verb, the shorter Pocket line (7A, 8A) → Task 7.

## Design Review Completion Summary

```
  +====================================================================+
  |         DESIGN PLAN REVIEW — COMPLETION SUMMARY                    |
  +====================================================================+
  | System Audit         | DESIGN.md present; UI scope: phone + site   |
  | Step 0               | 6/10; all 7 passes                          |
  | Pass 1  (Info Arch)  | 5/10 → 9/10 after fixes                     |
  | Pass 2  (States)     | 4/10 → 9/10 after fixes                     |
  | Pass 3  (Journey)    | 7/10 → 9/10 after fixes                     |
  | Pass 4  (AI Slop)    | 6/10 → 9/10 after fixes                     |
  | Pass 5  (Design Sys) | 6/10 → 9/10 after fixes                     |
  | Pass 6  (Responsive) | 6/10 → 9/10 after fixes                     |
  | Pass 7  (Decisions)  | 0 open, 0 deferred                          |
  +--------------------------------------------------------------------+
  | NOT in scope         | written (3 items)                           |
  | What already exists  | written                                     |
  | TODOS.md updates     | 0 items proposed                            |
  | Approved Mockups     | 0 generated (designer 429), 0 approved      |
  | Decisions made       | 10 added to plan                            |
  | Decisions deferred   | 0                                           |
  | Overall design score | 4/10 → 9/10                                 |
  +====================================================================+
```

Left below 10 on each pass: no mockup confirms the look, and the feel of each haptic is proven only on the owner's iPhone.

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
|--------|---------|-----|------|--------|----------|
| CEO Review | `/plan-ceo-review` | Scope & strategy | 0 | — | — |
| Outside Review | Claude subagent (in-host); Codex not installed | Independent 2nd opinion | 1 | unavailable (no external provider) | 10 findings, all folded into the passes |
| Eng Review | `/plan-eng-review` | Architecture & tests (required) | 0 | — | — |
| Design Review | `/plan-design-review` | UI/UX gaps | 1 | clean | score: 4/10 → 9/10, 10 decisions |
| DX Review | `/plan-devex-review` | Developer experience gaps | 0 | — | — |

- **OUTSIDE COVERAGE:** codex, design phase, unavailable (CLI not installed); the native Claude subagent completed with 10 findings. No external coverage.
- **VERDICT:** DESIGN CLEARED — eng review required.

NO UNRESOLVED DECISIONS
