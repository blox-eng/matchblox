# The island on Pocket Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The island goes live when Pocket is collapsed, and one tap brings back the same console, fast, with no Face ID inside 5 minutes.

**Architecture:** Part 1 is operations: the five relay secrets on the existing `matchblox.sh` Worker, then a check on a phone. Part 2 is three small rules in MatchbloxKit (unlock grace, link keeps console, cover on resume), each with XCTest, wired into `ConsoleView.swift` and `MatchbloxApp.swift`.

**Tech Stack:** Swift 6 / SwiftUI / LocalAuthentication / SwiftTerm; Cloudflare Worker (`wrangler`).

**Spec:** `docs/superpowers/specs/2026-11-25-island-on-pocket-design.md`

## Global Constraints

- Unlock grace: exactly 300 s (`UnlockGrace.window`), measured from the `.background` phase.
- New `LAContext`: `touchIDAuthenticationAllowableReuseDuration = 300`.
- Relay: production APNs, no `APNS_ENV` variable (TestFlight signs production).
- No host, user or real names in committed text (local commit hook).
- Swift tests run only on macOS: `swift test --package-path ios/MatchbloxKit` (CI job `ios.yml`). This Linux host has no Swift.
- Commits: signed Conventional Commits; never `--no-verify`.

## Review Focus

1. Pocket comes back after more than 5 minutes: Face ID must ask; the old context must be invalidated, not reused. (Task 2 test `testAfterTheWindowItAsks`.)
2. A reconnect fails while the old screen shows: the cover with its reason must show, not the stale screen. (Task 4 test `testAFailureShowsTheCoverOnResume`.)
3. A tap on the island of host B while host A's console is open: B's console opens. (Task 3 test `testALinkForAnotherHostOpensIt`.)
4. Pocket goes to the background and back inside a second (Control Centre pull): no Face ID, no reset. (Task 2 test `testAShortTripHolds`.)
5. The phone was locked and unlocked with Face ID in the window: no second prompt. Only a device can prove it (Task 5 step 3).

---

### Task 0: Track it

- [ ] **Step 1:** Create the GitHub issue "A tap on the island brings back the same console, with no second Face ID", milestone v0.1, link the spec. Add it to project 2, Status Started, Work on 2026-11-25 (commands in the global CLAUDE.md).
- [ ] **Step 2:** Branch `feat/island-on-pocket` from `origin/main`.

### Task 1: Relay setup (owner + agent)

**Files:** none in the repository. RELEASING.md lines 54–82 are the steps.

- [ ] **Step 1 (owner):** Apple Developer → Keys → + → APNs, Sandbox & Production → download `AuthKey_<KEYID>.p8`; note the Key ID and the Team ID.
- [ ] **Step 2 (owner, or agent with a Cloudflare token that has Workers Scripts: Edit):** set `APNS_KEY`, `APNS_KEY_ID`, `APNS_TEAM_ID`, `APNS_BUNDLE_ID` (the value of `MATCHBLOX_BUNDLE_ID`), `GRANT_SECRET` with `npx wrangler secret put`. Node is not on the agent's host; the owner runs it, or the agent installs Node with the owner's yes.
- [ ] **Step 3: Verify the relay.** Run: `curl -s -X POST https://matchblox.sh/push/grant -H 'content-type: application/json' -d '{}'`. Expected: a 4xx about the request (attestation), **not** `relay not configured`.
- [ ] **Step 4: Verify a phone.** Owner opens Pocket on the iPhone once. Run: `matchblox devices`. Expected: the iPhone row shows `buzz on  island yes`.
- [ ] **Step 5: Verify live.** Owner collapses Pocket; an agent session changes state. Expected: the Dynamic Island and the lock screen show the new state within 10 s. Record the measured time.

### Task 2: Unlock grace

**Files:**
- Create: `ios/MatchbloxKit/Sources/MatchbloxKit/UnlockGrace.swift`
- Test: `ios/MatchbloxKit/Tests/MatchbloxKitTests/UnlockGraceTests.swift`
- Modify: `ios/Matchblox/ConsoleView.swift` (`unlockKey()` ~line 915, `stop()` ~668, the `scenePhase` switch ~64)

**Interfaces:**
- Produces: `public struct UnlockGrace: Equatable, Sendable { public static let window: Duration /* .seconds(300) */; public init(); public mutating func backgrounded(at: ContinuousClock.Instant); public mutating func returned(at: ContinuousClock.Instant) -> Bool /* true: the unlock holds; false: ask again */ }`. With no `backgrounded` call, `returned` answers true (the app never left).

- [ ] **Step 1: Write the failing tests.** `testAShortTripHolds` (back after 1 s → true), `testAtTheWindowItHolds` (back after exactly 300 s → true), `testAfterTheWindowItAsks` (back after 301 s → false), `testEachTripCountsFromItsOwnStart` (leave, back at 200 s, leave again, back 200 s later → true).
- [ ] **Step 2:** Run `swift test --package-path ios/MatchbloxKit --filter UnlockGraceTests` (macOS or CI). Expected: FAIL, `UnlockGrace` not found.
- [ ] **Step 3:** Implement `UnlockGrace`.
- [ ] **Step 4:** Run the same command. Expected: PASS.
- [ ] **Step 5: Wire it.** In `ConsoleConnection`: add `func paused()` for `.background`: `cancel()` the connection, cancel the registration and set `registered = false` (each return renews the island, as each open does today), call `grace.backgrounded(at: .now)`, and keep `unlocked`. Add `func resumed()` for `.active`: if `!grace.returned(at: .now)`, invalidate `unlocked` and set it to nil; then `start()`. `stop()` (onDisappear) keeps today's behaviour. In `unlockKey()` set `touchIDAuthenticationAllowableReuseDuration = 300` (was 10). The `scenePhase` switch calls `paused()` and `resumed()`.
- [ ] **Step 6: Commit.** `feat(ios): the Face ID unlock holds for 5 minutes after Pocket leaves the screen`

### Task 3: A link for the open host keeps its console

**Files:**
- Modify: `ios/MatchbloxKit/Sources/MatchbloxKit/IslandPrefs.swift` (the `Opening` extension, near `route(for:hosts:)` line 191)
- Test: `ios/MatchbloxKit/Tests/MatchbloxKitTests/OpeningTests.swift`
- Modify: `ios/Matchblox/MatchbloxApp.swift` (`open(_:)`, ~line 182)

**Interfaces:**
- Consumes: `Opening.Route` (has `.console(Host, open: Int?)`), `OpenLink`, `host8(_:)`.
- Produces: `public static func keepsConsole(for link: OpenLink, current: Route) -> Bool`. True when `current` is `.console(h, _)` and `host8(h.fingerprint) == link.host`, whatever `link.session` is.

- [ ] **Step 1: Write the failing tests.** `testALinkForTheOpenHostKeepsItsConsole` (same host, session 3 → true), `testALinkForAnotherHostOpensIt` (other host → false), `testALinkOnTheHostsListOpensTheConsole` (current `.hosts` → false).
- [ ] **Step 2:** Run `swift test --package-path ios/MatchbloxKit --filter OpeningTests`. Expected: FAIL.
- [ ] **Step 3:** Implement `keepsConsole(for:current:)`.
- [ ] **Step 4:** Run it. Expected: PASS.
- [ ] **Step 5: Wire it.** In `open(_:)`: when `keepsConsole(for: link, current: route)` is true, return before `linkOpens += 1`. Update the doc comment on `open(_:)` to say so.
- [ ] **Step 6: Commit.** `feat(ios): a tap on the island goes back to the open console as it was`

### Task 4: The screen stays while the console comes back

**Files:**
- Modify: `ios/MatchbloxKit/Sources/MatchbloxKit/Opening.swift`
- Test: `ios/MatchbloxKit/Tests/MatchbloxKitTests/OpeningTests.swift`
- Modify: `ios/Matchblox/ConsoleView.swift` (body ~line 50; `serve` ~851)

**Interfaces:**
- Produces: `public static func coverShows(live: Bool, failed: Bool, wasLive: Bool) -> Bool`. False when `live`. True when `failed`. Else `!wasLive`.

- [ ] **Step 1: Write the failing tests.** `testTheFirstConnectShowsTheCover` (false, false, false → true), `testAResumeKeepsTheScreen` (false, false, true → false), `testAFailureShowsTheCoverOnResume` (false, true, true → true), `testLiveShowsNoCover` (true, false, true → false).
- [ ] **Step 2:** Run `swift test --package-path ios/MatchbloxKit --filter OpeningTests`. Expected: FAIL.
- [ ] **Step 3:** Implement it.
- [ ] **Step 4:** Run it. Expected: PASS.
- [ ] **Step 5: Wire it.** `ConsoleConnection` gets `private(set) var wasLive = false`, set true when `phase` becomes `.live`, and kept across `paused()`/`resumed()`. The body shows `cover` when `Opening.coverShows(live: phase == .live, failed: <phase is .failed>, wasLive: connection.wasLive)`. In `serve`, skip `resetToInitialState()` and `forgetScrollBack()` when `wasLive` is true: the tmux attach repaints the screen in place.
- [ ] **Step 6: Commit.** `feat(ios): Pocket keeps the console's screen while it connects again`

### Task 5: Device check

- [ ] **Step 1:** CI `ios.yml` green on the branch (quote the `swift test` summary line).
- [ ] **Step 2:** A TestFlight or Xcode build on the iPhone. Open a console, collapse, wait 1 min, tap the island. Expected: no Face ID, no `Connecting` cover, same pane, back in under 2 s (measure).
- [ ] **Step 3:** Lock the phone, unlock with Face ID, tap the lock-screen activity within 5 min. Expected: one Face ID (the phone's), none from Pocket. If Pocket asks, record it: the reuse duration did not apply, and the fallback is Face ID on that path.
- [ ] **Step 4:** Collapse for 6 min, return. Expected: Face ID asks.
- [ ] **Step 5:** Turn on airplane mode, return. Expected: the cover with the failure reason.

### Task 5b: An idle account does not lead the glance (report, 2026-11-26)

- [x] **Step 1: Failing tests.** Go `TestAnAccountWithNoWorkingSessionIsIdle` (`AccountUse.Idle`, and `usageMoved` sends a flip); Swift `testAnIdleAccountDoesNotLeadTheGlance` (`tightest` skips idle accounts, nil when all are idle, a payload with no `idle` decodes as false) and `testAMonthNamesItsResetNotTheWeek` (`resets Dec 17`).
- [x] **Step 2:** Run them. Expected: FAIL (`Idle` undefined; CI `ios`).
- [x] **Step 3:** `push.Build` sets `Idle` when no busy session has the account's provider, label and folder. `tightest` filters idle accounts. `IslandLook.forecast` names the latest reset for an account with no `wk` window.
- [x] **Step 4:** `go test ./...` and CI `ios` green.
- [x] **Step 5: Commit.** `fix: an idle account no longer fills the island's glance`

### Task 6: Docs, review, ship

- [ ] **Step 1:** `docs/phone.md` "The buzz and the island": one line on the 5-minute unlock and the tap back. `CHANGELOG.md` Unreleased: one line, headline ≤7 words.
- [ ] **Step 2:** Landing: run `.github/scripts/landing-guard.sh "<title>" <body-file> origin/main`. Expected: pass, with a `www/` line on the one-tap return, or `Landing page: unchanged because <reason>` in the body.
- [ ] **Step 3:** Review the whole branch (builder:review). No open Critical or Important finding.
- [ ] **Step 4:** PR, title `feat(ios): …`, body links the issue; merge when green.
