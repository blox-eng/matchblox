# The island puts what needs you first — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The island and the lock screen lead with the sessions that need the person, count them, and keep one limit on the right.

**Architecture:** The host adds the sessions that need the person (at most 3, ordered) and their count to the content state. MatchbloxKit turns them into the compact text and the lock-screen rows. The widget's Sessions face draws them. The Usage face and the Show setting stay.

**Tech Stack:** Go (`internal/push`), Swift (MatchbloxKit, WidgetKit/ActivityKit).

**Spec:** `docs/superpowers/specs/2026-11-27-island-attention-first-design.md`

## Global Constraints

- One Live Activity per host. No rotation. No new setting.
- The payload holds numbers, states, times and percents only (`internal/push/payload.go` package doc).
- Copy, verbatim: `asks to run a command`, `asks you a question`, `finished its turn`, `+ N more`, `╿ N`.
- Old host + new phone, and new host + old phone, behave as today: a missing `needs` decodes as `[]`, `need` as 0.
- Push rates of design 0010 §3.6 stay; a change in the needs goes at once (priority 10).
- Branch `feat/island-ranking`, stacked on #513; its PR targets `main` after #513 merges.

## Review Focus

1. A finished turn and a question at the same time: the question leads (asks before finished), the count says 2.
2. Four or more that need you: three rows and `+ 1 more`; the compact says `╿ 4`.
3. A session that needs you and is in a chain: the chain face (`Chain`) keeps its place; the needs list still names the session.
4. All quiet with no working account: the compact right shows nothing, not `0%`.
5. A need that ends while the top stays the same (count 2 → 1): the change goes at once.

---

### Task 1: The host sends what needs you

**Files:**
- Modify: `internal/push/payload.go` (ContentState), `internal/push/state.go` (Build), `internal/push/sender.go:505` (changed)
- Modify: `testdata/push/content-state.json`
- Test: `internal/push/state_test.go`, `internal/push/sender_test.go`

**Interfaces:**
- Produces: `type Need struct { N int \`json:"n"\`; Kind string \`json:"kind"\` }` with Kind `command` (queue.StatePermission), `question` (queue.StateQuestion), `done` (queue.StateFinished); `ContentState.Needs []Need \`json:"needs,omitempty"\`` (at most 3, asks before done, oldest first) and `ContentState.Need int \`json:"need,omitempty"\`` (all of them).

- [ ] **Step 1: Failing tests.** `TestWhatNeedsYouLeadsInOrder`: sessions `w1` (question, since t0-2m), `d1` (finished, since t0-5m), `p1` (permission, since t0-1m), `d2` (finished, t0-4m), plus a busy session. Assert `Need == 4`, `Needs` kinds `question, command, done` with numbers of `w1, p1, d1`, and `Top.N == n.Session("w1")`. `TestANeedThatEndsGoesAtOnce` (sender): two needs, then one; the second doc sends a channel update at priority 10 within the 3 minutes.
- [ ] **Step 2:** `go test ./internal/push/ -run 'TestWhatNeedsYou|TestANeedThatEnds'`. Expected: FAIL (`Need` undefined).
- [ ] **Step 3:** Build fills `Needs`/`Need` from the queue items it already reads. `changed` adds `cs.Need != l.Need` and a change of the `Needs` numbers. Add `"need"` and `"needs"` to the fixture so `TestBuildMatchesTheFixture` holds.
- [ ] **Step 4:** `go test ./internal/push/`. Expected: ok.
- [ ] **Step 5: Commit.** `feat(push): the island state names what needs you`

### Task 2: The phone reads it and words it

**Files:**
- Modify: `ios/MatchbloxKit/Sources/MatchbloxKit/IslandState.swift`, `IslandLook.swift`
- Test: `ios/MatchbloxKit/Tests/MatchbloxKitTests/IslandLookTests.swift`, `Fixture.swift` users

**Interfaces:**
- Consumes: Task 1's JSON (`need`, `needs[].n`, `needs[].kind`).
- Produces: `IslandState.Need { n: Int; kind: String }`, `IslandState.needs: [Need]`, `IslandState.need: Int`; `IslandLook.ask(_ kind: String) -> String`; `IslandLook.attention(_ s: IslandState, names: IslandNames) -> String?` (the compact right: the name for 1, `╿ N` for 2+, nil for 0); `IslandLook.rows(_ s: IslandState, names: IslandNames) -> (rows: [(name: String, ask: String)], more: String?)`; `IslandLook.limitsLine(_ s: IslandState) -> String?` (`wk 30% · 5h 62%`, the fullest unspent window of each working account, nil when none).

- [ ] **Step 1: Failing tests.** `testWhatNeedsYouIsWorded`: `ask("command") == "asks to run a command"`, `ask("question") == "asks you a question"`, `ask("done") == "finished its turn"`. `testTheCompactNamesOneAndCountsMore`: need 1 → the session's name; need 4 → `"╿ 4"`; need 0 → nil. `testThreeRowsThenMore`: need 4 with 3 needs → 3 rows, `more == "+ 1 more"`. `testTheLimitsLineLeavesOutIdleAndSpent`: an idle account and a spent window are not in it; no working account → nil. `testAnOldStateHasNoNeeds`: the fixture without `need`/`needs` decodes with `need == 0`, `needs == []`.
- [ ] **Step 2:** Push the test commit; CI `ios`. Expected: FAIL (no member `needs`).
- [ ] **Step 3:** Implement them in `IslandState.swift` (decodeIfPresent) and `IslandLook.swift`.
- [ ] **Step 4:** CI `ios` green.
- [ ] **Step 5: Commit.** `feat(ios): the island words what needs you`

### Task 3: The Sessions face draws it

**Files:**
- Modify: `ios/MatchbloxIsland/SessionsFace.swift`, `IslandPreviews.swift`

**Interfaces:**
- Consumes: Task 2's `attention`, `rows`, `limitsLine`, `ask`.

- [ ] **Step 1:** `SessionsCompactTrailing`: `attention(...)` in the accent when non-nil; else, while work runs, `usageCompact(...)?.text`; else nothing. `SessionsCompactLeading`: the match; `▰` in the accent when the top is quiet.
- [ ] **Step 2:** `SessionsLock`: with `need > 0`, `HostLine` + `Counts`, then each row as `WhoLine(name:detail:)` with the ask, then `more`, then `limitsLine` in `Tokens.muted`; with `need == 0`, today's view plus `limitsLine`.
- [ ] **Step 3:** Previews: `needsOne`, `needsFour`, `quietWithLimits` in `IslandPreviews.swift`.
- [ ] **Step 4:** CI `ios`: `** BUILD SUCCEEDED **` (the check for views; no unit test sees them).
- [ ] **Step 5: Commit.** `feat(ios): the island leads with what needs you`

### Task 5: The match burns while any session lives (owner ruling 2026-11-27)

**Files:** `internal/push/state.go` (Build), `internal/push/state_test.go`, `ios/MatchbloxKit/Sources/MatchbloxKit/IslandLook.swift`, `ios/Shared/Match.swift`, `ios/MatchbloxKit/Tests/MatchbloxKitTests/IslandLookTests.swift`

**Interfaces:** Top state `idle` (host) and `TopState.idle` (phone). Rank: waits, done, out, burns, idle; of the idle ones, `Session.Started` newest first. `Burn` counts `burns` only.

- [ ] **Step 1: Failing tests.** Go `TestAnIdleSessionKeepsTheMatchLit`: one session, not busy, no queue row → `Top.State == "idle"`, `Burn == 0`; with a busy one too → the busy one is the top. `TestNoSessionIsQuiet`: no session → `Top == nil`. Swift `testTheMatchIsLitWhileASessionLives`: `lit(.burns|.waits|.done|.idle) == true`, `lit(.out|.quiet) == false`; `state` of `"idle"` is `.idle`.
- [ ] **Step 2:** Run both. Expected: FAIL (`idle` unknown).
- [ ] **Step 3:** Build adds idle candidates; the content-state fixture regex test accepts `idle`. `IslandLook.lit(_:)`; Match draws the flame when `lit`, the smoke over it for waits, a small flame for done and idle; the word for idle is `· idle`, the compact right is the limit (as burns).
- [ ] **Step 4:** `go test ./internal/push/` ok; CI `ios` green.
- [ ] **Step 5: Commit.** `feat: the island match burns while any session lives`

### Task 6: The buzz hisses (design 0009 §3, owner ruling 2026-11-27)

**Files:** `internal/push/sender.go` (buzz), `internal/push/sender_test.go`, `ios/Matchblox/Sounds/pinch-ember.wav`, `ios/project.yml` (resources)

**Interfaces:** `aps.sound` is `pinch-ember.wav` for every buzz, a wait or a finished turn. A finished turn buzzes like a wait (the same 30 s window; a wait wins inside one window).

- [x] **Step 0:** The owner heard both and picked `pinch-ember` for every buzz (2026-11-27).
- [ ] **Step 1: Failing tests.** `TestAWaitBuzzesWithThePinch`: `sound == "pinch-ember.wav"`. `TestAFinishedTurnBuzzes`: a `finished` item → one device push, `sound == "pinch-ember.wav"`. `TestAWaitWinsTheWindow`: a wait and a finish in one window → one push, and its line is the wait's.
- [ ] **Step 2:** Run them. Expected: FAIL (`default`).
- [ ] **Step 3:** The buzz picks the item and its sound; the file goes in the app bundle (a notification sound must be in the main bundle).
- [ ] **Step 4:** `go test ./internal/push/` ok; CI `ios` green, and the archive holds the file.
- [ ] **Step 5: Commit.** `feat: the buzz hisses, for a wait and a finished turn`

### Task 4: Docs, review, ship (last, after Tasks 5 and 6)

- [ ] **Step 1:** `docs/phone.md` "The island" bullet; `CHANGELOG.md` Unreleased, headline ≤ 7 words.
- [ ] **Step 2:** `.github/scripts/landing-guard.sh "<title>" <body> origin/main`: pass, or `Landing page: unchanged because <reason>`.
- [ ] **Step 3:** Review the branch (builder:review). No open Critical or Important.
- [ ] **Step 4:** TestFlight from the branch and the host build; the owner checks it on the phone; PR to `main` after #513.
