Spec: docs/superpowers/specs/2026-11-24-pocket-home-design.md
Spec approved: 2026-11-25
Test command: swift test --package-path ios/MatchbloxKit (each kit task); make -C ios test (alias); App UI / Mac build only on macOS CI (`make -C ios build`) — Linux cannot run HostsView/MatchTray UIKit tests
Plan approved: 2026-11-25 by owner
Next: builder:build

# Plan: Pocket home — the box

**Goal:** From any live Pocket console, a person reaches home (hosts, Live Activity, text size, demo/privacy/sponsor) in two taps via a `BOX` stamp, without home competing with the matches.

**Architecture:** Pure rules (first-run route, “once” flags, host-row status words, footer demo rule, VoiceOver strings) live in `MatchbloxKit` so Linux CI proves them. The app grows `HostsView` into home and adds a `BOX` stamp beside `EDIT MINE` on the tray. Island behaviour stays; copy becomes Live Activity; text size leaves that screen.

**Tech stack:** Swift, MatchbloxKit, SwiftUI/UIKit Pocket app, `DESIGN.md`.

**Spec:** `docs/superpowers/specs/2026-11-24-pocket-home-design.md`

## What the code holds today

- `Opening.first`: 0 hosts → `.scan`; 1 → console; 2+ → `.hosts` (`Opening.swift`).
- Live console has no exit to hosts; cover shows Hosts/Island only when not live (`ConsoleView`).
- `HostsView`: list, swipe-delete with no confirm, Pair / Island / Try a demo.
- `IslandView`: Follow / Show / Buzz / Console text (`TextSize`).
- Tray: `EditStamp` (10.5 pt, `contentShape` −10, VO “Edit Mine”); agent tag bottom-left.
- Kit tests: `OpeningTests`, `HostStoreTests`, `MatchTrayTests`, `TextSizeTests`, `DemoHostTests`.
- CI kit: `swift test --package-path ios/MatchbloxKit`.

## File map

- `ios/MatchbloxKit/Sources/MatchbloxKit/Opening.swift` (change) — first route is home when no host; helpers home needs.
- `ios/MatchbloxKit/Tests/MatchbloxKitTests/OpeningTests.swift` (change) — first-route and helper tests.
- `ios/MatchbloxKit/Sources/MatchbloxKit/HomeFlags.swift` (create) — “once” flags for first-run play and BOX teach (UserDefaults).
- `ios/MatchbloxKit/Tests/MatchbloxKitTests/HomeFlagsTests.swift` (create) — flag set/get tests.
- `ios/MatchbloxKit/Sources/MatchbloxKit/HomeCopy.swift` (create) — host-row status words, VoiceOver lines, remove question text, footer demo visibility, Live Activity row summary.
- `ios/MatchbloxKit/Tests/MatchbloxKitTests/HomeCopyTests.swift` (create) — those strings and rules.
- `ios/MatchbloxKit/Sources/MatchbloxKit/HostSight.swift` (create) — per-host `lastSeenAt` / `lastFailAt` in UserDefaults; no background probe.
- `ios/MatchbloxKit/Tests/MatchbloxKitTests/HostSightTests.swift` (create) — sight → row status mapping tests.
- `ios/Matchblox/MatchTray.swift` (change) — `BOX` stamp (small front-on box via scaled `MatchBox` or path), agent tag shifts right; teach-once hook.
- `ios/Matchblox/HostsView.swift` (change) — home layout, states, swipe confirm, Live Activity row, text line, footer, `‹ host` back.
- `ios/Matchblox/MatchbloxApp.swift` (change) — first-run home, BOX → `.hosts`, back to live console, Reduce Motion skip.
- `ios/Matchblox/ConsoleView.swift` (change) — live path opens home via tray; cover Hosts opens home; cover Island control removed.
- `ios/Matchblox/MatchBox.swift` (change only if needed) — allow a stamp-sized front-on mark for first-run / BOX glyph.
- `ios/MatchbloxUITests/DemoConsoleTests.swift` (change) — empty launch expects home (pair + demo), not scan-only.
- `ios/Matchblox/IslandView.swift` (change) — title/copy Live Activity; drop Console text section.
- `ios/Matchblox/BeltView.swift` (change) — wire `onBox` / teach-once if the tray callback lives here.
- `DESIGN.md` (change) — Pocket section (5.1); §9 Island → Live Activity in app words.
- `docs/phone.md` (change) — name home / BOX / Live Activity where Pocket docs speak today.
- `CHANGELOG.md` (change) — Unreleased.
- `ios/MatchbloxUITests/…` (change or create, if CI already runs UI tests for hosts/demo) — first-run lands on home; BOX opens hosts. Skip on Linux.

## Tasks

### Task 1: First open is home when no host

- **Files:** `Opening.swift`; `OpeningTests.swift`; `DemoHostTests.swift` if needed; `DemoConsoleTests.swift`.
- **Test first:** `testFirstWithNoHostIsHosts` — `Opening.first(hosts: []) == .hosts`. Update any empty→`.scan` assertion. UITest: empty launch shows home pair/demo (not scan-as-root).
- **Run:** `swift test --package-path ios/MatchbloxKit --filter OpeningTests` — before: fail (still `.scan`); after: pass. Mac CI UITest updated in the same commit or Task 5 if the test lives only there.
- **Do:** Change `case 0` to `.hosts`. Keep one-host → console, many → hosts. Scanner remains reachable via `+ pair a host` / first-run actions on home.
- **Done when:** `testFirstWithNoHostIsHosts` passes; existing one-host and many-host tests still pass; UITest expectation updated so Mac CI does not require scan-as-root.
- **Commit:** `feat(ios): opening with no host lands on home`

### Task 2: Once flags for first-run play and BOX teach

- **Files:** `HomeFlags.swift` (create); `HomeFlagsTests.swift` (create).
- **Test first:**
  - `testFirstRunPlayDefaultsFalse` — fresh suite defaults (inject `UserDefaults(suiteName:)`).
  - `testMarkFirstRunPlayPlayed` — after `markFirstRunPlayPlayed()`, `shouldPlayFirstRun` is false.
  - `testMarkBoxTaught` — same for BOX teach.
- **Run:** `swift test --package-path ios/MatchbloxKit --filter HomeFlagsTests` — before: fail (no type); after: pass.
- **Do:** `HomeFlags` with injectable `UserDefaults`. Keys under a clear prefix (`pocket.home.…`). Reduce Motion is read in the app; flags still set when motion is skipped (spec 6.2).
- **Done when:** three tests pass.
- **Commit:** `feat(ios): remember first-run and BOX teach once`

### Task 3: Home copy, VoiceOver, and host sight

- **Files:** `HomeCopy.swift`; `HomeCopyTests.swift`; `HostSight.swift`; `HostSightTests.swift`.
- **Test first:**
  - `testHostRowSpokenLive` — live → contains label, address, `live`.
  - `testHostRowSpokenNotReachable` — not reachable → `not reachable since` + time from input.
  - `testHostRowSpokenLastSeen` — last seen → relative `2h ago` style from a fixed `Date`.
  - `testRemoveQuestionLive` — live host question includes `its console closes`.
  - `testFooterHidesDemoWhenDemoPaired` — `showDemoInFooter(hosts:)` false when any `isDemo`.
  - `testVoiceOverBack` / `testVoiceOverBox` — as spec 6.3.
  - `testLiveActivityRow` — follow host label + show word (`sessions`/`usage`/`auto`) + `buzz on`/`buzz off` from prefs.
  - `testSightLiveBeatsLastSeen` — open console host is `live` even if lastSeen old.
  - `testSightFailMarksNotReachable` — `markFailed` then status is not-reachable until a later `markSeen`.
- **Run:** `swift test --package-path ios/MatchbloxKit --filter 'HomeCopyTests|HostSightTests'` — before: fail; after: pass.
- **Do:**
  1. `HomeCopy` owns Pass 2 / 6.3 words and the Live Activity one-liner: follow host (or lastOpened if follow none) · show · buzz for that host (if follow none, buzz on if any host buzzes).
  2. `HostSight` stores `lastSeenAt` / `lastFailAt` per fingerprint (UserDefaults). **No background probe in v1.** App calls `markSeen` on successful console connect and clean end; `markFailed` on connect failure. Row status: live (open console) → not-reachable (fail after last seen) → last-seen → checking is unused in v1 (keep last known line; no spinner — matches spec “checking”).
  3. Remove success notice: keep existing `Opening.removedNotice` (spec’s `removed work-1` was shorthand).
  4. Footer version: `Bundle.main` short version string, not a hard-coded `0.58.0`.
- **Done when:** those tests pass.
- **Commit:** `feat(ios): lock Pocket home copy, VoiceOver, and host sight`

### Task 4: BOX stamp on the tray

- **Files:** `MatchTray.swift` (app); `BeltView.swift` if callbacks land there; kit `BoxStampMetrics` (`minHit: 44`).
- **Test first:** `testBoxStampMinHitIs44`. Teach gate: Task 2. Teach also after first demo console land (journey step 2–3; treat “first successful console from empty” as the teach moment, pair or demo).
- **Run:** `swift test --package-path ios/MatchbloxKit --filter BoxStamp` — after: pass.
- **Do:** Mirror `EditStamp`: ink/edge, `BOX` + small front-on box (scaled `MatchBox` shut/outline or a 3-face path — builder picks the smaller draw). Hit ≥44×44. Bottom-left; agent tag to its right. `onBox`. **Two taps:** tap the drawer handle (existing) then `BOX`; a pull that opens the drawer is the same first action. Teach-once drawer open uses DESIGN §3 open **380 ms** for this teach only (not the belt’s 0.26 s close path). Skip teach when Reduce Motion; still set the flag.
- **Done when:** kit min-hit test passes; PR notes Mac/device VO for `Home, the box`.
- **Commit:** `feat(ios): BOX stamp opens home from the tray`

### Task 5: HostsView becomes home

- **Files:** `HostsView.swift`; `MatchbloxApp.swift` (wiring); optionally split a `HomeView` only if `HostsView` cannot grow cleanly — prefer one file, one job: home.
- **Test first:** Kit: `HomeCopy` already covers words. App: extend UITest if present — empty hosts shows pair + demo; with hosts, Live Activity row absent for demo-only; footer hides demo when demo paired. Otherwise document Mac CI / device as the check for layout.
- **Run:** kit filters green; UITest or manual.
- **Do:**
  1. Draw as console: Plex Mono / theme tokens, region labels + hairlines (not `List` settings chrome if it fights the anti-goal — keep density, drop iOS Form look).
  2. Order: `‹ host` (when a console is behind) → brand → HOSTS → Live Activity row (real hosts only) → text `− N pt +` → footer `demo · privacy ↗ · ♥ sponsor ↗  <version>`.
  3. Host states via `HostSight` + `HomeCopy` (Task 3). Font: same system mono as the rest of Pocket (spec’s “Plex Mono” means console mono voice; do not add a new face).
  4. Swipe left → inline remove question (`HomeCopy`); live host adds console-closes; last host stays on first-run home under `Opening.removedNotice` (no jump to scanner).
  5. Footer: privacy → docs URL; sponsor → GitHub Sponsors; demo → existing demo path; hide demo when demo paired; version from the bundle.
  6. Text size: load/save `TextSize`; pinch on terminal stays.
  7. Live Activity row → `.island` (code name stays); summary from Task 3.
  8. VO labels and ≥44 pt hits (6.3, 6.4).
- **Surface:** host list / device keys on remove — keep `DeviceKey.remove` + Island forget; test confirm→remove still forgets the key for a non-demo host.
- **Done when:** kit tests green; remove-with-confirm clears key; Mac CI UITest (or PR “Not proven” + screenshots) shows hosts-first home matching the wireframe order.
- **Commit:** `feat(ios): HostsView is Pocket home`

### Task 6: Live console reaches home; back returns; first-run mark

- **Files:** `ConsoleView.swift`; `MatchbloxApp.swift`; `BeltView.swift` / tray; `MatchBox.swift` if first-run needs a pose on home.
- **Test first:** `Opening.backToConsole(host:)` → `.console(host, open: nil)` in `OpeningTests`.
- **Run:** `swift test --package-path ios/MatchbloxKit --filter OpeningTests` — after: pass.
- **Do:** Live: BOX → `.hosts` with live host for `‹ host`. Back via `‹ host` or live row. **Cover:** keep Hosts (opens home); **remove the Island control** from the cover (entry is the home row only). First-run on empty home: play existing **`MatchBox` open/strike/stand** (pair-page sequence — Pocket’s mark). Spec’s site Fall (300 ms) is the www hero; Pocket does not reimplement Fall. Skip under Reduce Motion; set `HomeFlags`.
- **Done when:** back helper test passes; live→home→back on device or Not proven in the PR.
- **Commit:** `feat(ios): live console opens home through BOX`

### Task 7: Live Activity copy; text size leaves Island screen

- **Files:** `IslandView.swift`; user-visible Island strings in `ios/Matchblox` (not type names / file names).
- **Test first:** after change, these are empty: `rg -n 'Text\\(\"Island\"\\)|Button\\(\"Island\"' ios/Matchblox --glob '*.swift'`. Broader `"Island"` hits in comments/types may remain.
- **Run:** that rg empty; kit suite green.
- **Do:** Title and buttons say Live Activity. Remove Console text from IslandView. Buzz stays.
- **Done when:** the Text/Button rg is empty; text size only on home + pinch.
- **Commit:** `feat(ios): Island copy becomes Live Activity; text size on home`

### Task 8: Canon and docs

- **Files:** `DESIGN.md`; `docs/phone.md`; `CHANGELOG.md`.
- **Test first:** no product test. Checks: `go test ./internal/docscheck/...` if it covers DESIGN; else manual read. Landing guard: feat PR body edits `www/` only if the story changes — recommend **unchanged** because Pocket home is app UX, not the marketing page: `Landing page: unchanged because Pocket home is an app surface, not a marketing story.` (≥10 chars reason).
- **Run:** docscheck if applicable; landing-guard script on the PR title/body.
- **Do:** Add Pocket section per spec 5.1. §9: app words say Live Activity; code names stay. Phone docs name BOX and home. Changelog Unreleased entries for the feat.
- **Done when:** DESIGN Pocket section matches 5.1; landing-guard passes with the unchanged line.
- **Commit:** `docs: Pocket home and Live Activity in the canon`

### Task 9: Review

- **Files:** none new; whole diff.
- **Test first:** full `swift test --package-path ios/MatchbloxKit`; on Mac CI `make -C ios build`; landing-guard.
- **Run:** kit green; build on CI.
- **Do:** Load `builder:review`. Spec path above. No open Critical or Important. Surfaces touched: host keys on remove — confirm hostile path still safe.
- **Done when:** review clean; kit green.
- **Commit:** none (review only) — or fixup commits if review finds gaps.

## Lens findings

### Engineering

- Reuse `HostsView`, `EditStamp`, `TextSize`, `Opening`, `HostStore`, Island registration — no new settings stack. **No change to cut further:** eight app/kit touch points is the minimum for tray + home + route + canon.
- Failure in production: reachability check fails → show last-known / not-reachable line (Task 5); store save fails on remove → keep existing problem string.
- Test map:
  - empty → home route — Task 1
  - once flags — Task 2
  - VO / remove / footer demo — Task 3
  - BOX hit constant — Task 4
  - remove forgets key — Task 5
  - back to console — Task 6
  - Island user copy gone — Task 7 (grep)
- Regression: `Opening.first` one-host still console; demo first still console (`DemoHostTests`); Edit Mine stamp unchanged.

### Design

- Empty home: first-run sentence + pair + demo (Task 5/6). Rating target 9 via spec storyboard.
- Error: not reachable on row; remove confirm words from `HomeCopy` — tested Task 3.
- One job: hosts lead — Tasks 5–7. **No change,** because the spec already locks order and anti-goals.

### Architecture

- No new service boundary. UserDefaults for flags only. **No change.**

## Spec check

| Spec line | Task |
|---|---|
| Reach home from live console in two taps | Task 4, 6 |
| Home lists hosts; tap opens; remove with question | Task 3, 5 |
| Text size, Live Activity, demo, privacy, sponsor on home | Task 5, 7 |
| DESIGN.md Pocket section | Task 8 |
| Anti-goal: home competes with matches | Task 4 (stamp, not layout key) |
| Anti-goal: text size not the main thing | Task 5, 7 |
| Anti-goal: no iOS settings-list look | Task 5 |
| First-run box plays once / Reduce Motion skip | Task 2, 6 |
| BOX teach once | Task 2, 4 |
| VO + ≥44 pt taps | Task 3, 4, 5 |
| Island → Live Activity copy; behaviour unchanged | Task 7, 8 |
| Last host remove stays on home | Task 5 |
| Footer demo hides when demo paired | Task 3, 5 |

## Cold read

Fresh reader (2026-11-25): Blockers 2, Majors 10, Nits 3.

| Finding | Plan change |
|---|---|
| No last-seen / reachability | Added `HostSight` + Task 3 rules; no background probe |
| First-run §3 Fall vs MatchBox | Task 6 uses Pocket `MatchBox`; site Fall stays on www |
| Footer version 0.58.0 vs bundle | Bundle short version |
| Remove notice wording | Keep `Opening.removedNotice` |
| Teach after demo? | Yes — first console land from empty (pair or demo) |
| Cover Island stays? | Removed in Task 6 |
| Two taps vs pull | Handle tap (or pull) then BOX |
| UITest scan root | Task 1 / DemoConsoleTests update |
| Visual done-when | Mac CI / Not proven + wireframe order |
| Teach drawer 380 ms | Task 4 uses 380 ms for teach open |
| Live Activity row mapping | Task 3 `testLiveActivityRow` rules |
| Plan approved: no | Correct until owner approves this plan — no change |
| Plex vs system mono | System mono (Pocket today) |
| BOX glyph | Scaled MatchBox / path — builder |
| Island grep breadth | Narrow Text/Button pattern |

## Calls

- App UI not proven on Linux; Mac CI + device/TestFlight are the visual proof.
- First-run sentence exact words: left to the builder (spec).
- Sidebar duplicate: #506; not this plan.
- HostSight without a background probe is the v1 reading of spec states 2.1 / checking — owner accepts by approving this plan.
