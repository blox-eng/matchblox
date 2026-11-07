# Chain Room Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The owner works each chain in its own room, with its stoker one key away (#326).

**Architecture:** The room is a new console view, `internal/app/room*.go`, built on today's chain index (`chainIx`, `chainKeyOf`), the chain stokers in `state.Doc.Stokers`, and #261's chat. The flip is a tmux binding that hands the console an APC, like the home key (`internal/panes/home.go`). Step 1 (Tasks 1–11) adds the room, the flip, the island and the haptics on today's tabs. Step 2 (Tasks 12–14) folds the tabs to `chains · machine` and removes the groupings.

**Tech Stack:** Go 1.x with Bubble Tea (console, service), tmux, Swift/UIKit/ActivityKit (iOS), bash (CI guard).

**Spec:** `docs/superpowers/specs/2026-11-07-chain-room-design.md`. Read it whole before any task. Its section names are cited as "spec § …".

**Base:** one branch, one PR: `design/chain-room` (the spec, this plan and Task 1 are on it now). After PR #312 (#304) merges, rebase it onto `main`, then do Tasks 2–15 on it. This plan uses #304's names. matchbloxrelease commits the landing page onto this branch (Task 15).

## Global Constraints

- No sibling product names in code, docs, the site or commit messages. Grep the staged diff before each commit.
- Signed Conventional Commits, with the trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Never `--no-verify`.
- Goldens update in the commit that changes the screen.
- DESIGN.md changes in the same PR as each step (spec § DESIGN.md changes).
- Copy is ASD-STE100, and every string the spec quotes is used verbatim.
- One accent per view (DESIGN.md rule 4). `✦` stays the busy match. `≋` means "has a stoker".
- A hook keeps 30 ms (`hookTimeout`). The flip is a tmux binding and never reaches a pane as typing.
- A chain is a team. A match may be in more than one team (owner, 2026-11-07). Multi-team membership in `internal/chains` belongs to matchblox-chain's work. The room reads membership only through `Model.chainKeyOf` and `chainIx`, so it shows a match in every team that `chainIx` reports.
- The landing page changes before a `feat` PR merges (owner, 2026-11-07). matchbloxrelease owns `www/index.html`; Task 15 coordinates.

## Review Focus

1. A peer host's chain: ⌥s on its row opens its room here. ⌥s pressed in a pane of another tmux server does nothing, and never types into that pane. Test: Task 7, `TestFlipFromAPaneOfAnotherServerSendsNothing`.
2. The came-from pane is gone when the owner flips back: the room stays, and the toast says `That session ended.`. Test: Task 7, `TestFlipBackToAGonePaneStays`.
3. Ten matches change state in one sample: the phone plays one haptic. Test: Task 11, `testBurstPlaysOnce`.
4. A 47-character chain name at 40 columns: the room title truncates with `…`, and the need line wraps to two lines at most. Test: Task 4, `TestRoomAt40ColumnsTruncatesLongNames`.
5. The owner types while the stoker's order act is still in flight: the second ⏎ waits for the first result and does not send twice. Test: Task 5, `TestTalkSendsOneOrderAtATime`.

---

### Task 1: The landing page guard

**Files:**
- Create: `.github/scripts/landing-guard.sh`, `.github/scripts/landing-guard_test.sh`, `AGENTS.md`, `CLAUDE.md`
- Modify: `.github/workflows/ci.yml` (job `conventional-commits`, and the shellcheck line of `lint`), `.claude/settings.json`, `CONTRIBUTING.md`

**Interfaces:**
- Produces: `landing-guard.sh` usage `landing-guard.sh <title> <body-file> <base-ref>`; exit 0 = OK, exit 1 = FAIL with the fix on stderr.

Rule: a pull request whose title starts `feat` (`^feat(\([^)]+\))?!?: `) must change a file under `www/`. Alternatively, its body has a line `Landing page: unchanged because <reason>` with a reason of at least 10 characters. Any other title passes.

- [ ] **Step 1: Write the failing test** `landing-guard_test.sh`. It builds a temporary git repo with a `main` commit and a branch, then asserts:
  - `feat: x` + a change only in `internal/` + empty body → exit 1, stderr contains `change www/ or add "Landing page: unchanged because …"`
  - `feat: x` + a change in `www/index.html` → exit 0
  - `feat(app): x` + body `Landing page: unchanged because the room has no public face yet` → exit 0
  - `feat: x` + body `Landing page: unchanged because no` → exit 1 (reason too short)
  - `fix: x` + a change only in `internal/` → exit 0
- [ ] **Step 2:** Run `bash .github/scripts/landing-guard_test.sh`. Expected: FAIL, because `landing-guard.sh` does not exist.
- [ ] **Step 3: Implement `landing-guard.sh`** in the style of `changelog-guard.sh`: `set -euo pipefail`, `git diff --name-only <base-ref>...HEAD`, and fail closed when the base ref is missing (`FAIL: cannot read <base-ref>`).
- [ ] **Step 4:** Run the test. Expected: every case prints `ok`, exit 0. Run `shellcheck .github/scripts/landing-guard*.sh`. Expected: no output.
- [ ] **Step 5: Wire CI.** Add a step `Landing page` to `conventional-commits`. It runs only when `github.event_name == 'pull_request'`, after `actions/checkout` with `fetch-depth: 0`. It writes `github.event.pull_request.body` to a file through `env:` (never interpolated into the script) and runs the guard with `origin/${{ github.base_ref }}`. Add the test script to the `lint` job, and add both files to its shellcheck line.
- [ ] **Step 6: Wire agents.**
  - `AGENTS.md`: one section, "Before a feat PR merges", with the rule, the escape line and the command to run it locally.
  - `CLAUDE.md`: one line, `@AGENTS.md`.
  - `CONTRIBUTING.md`: the same rule, one paragraph.
  - `.claude/settings.json`: add a `PreToolUse` hook on `Bash` that runs `.github/scripts/landing-guard-hook.sh`. That script reads the hook JSON from stdin. When `tool_input.command` matches `gh pr (create|merge|ready)`, it runs the guard with the current PR's title and body (`gh pr view --json title,body`, or the `--title`/`--body` of the command) against `origin/main`. It exits 2 with the guard's message to block, else exits 0. Add `landing-guard-hook.sh` to the test script with a fake JSON input for each of the two outcomes, and to shellcheck.
- [ ] **Step 7: Commit.** `ci: a feat pull request changes the landing page or says why not`

### Task 2: The key check

**Files:**
- Modify: `docs/superpowers/specs/2026-11-07-chain-room-design.md` (spec § The flip), `internal/panes/home.go`
- Test: `internal/panes/home_test.go`

**Interfaces:**
- Produces: `panes.FlipKey`, `panes.NextKey` (string, the tmux key names, `M-s` and `M-n` unless the check changes them), `panes.FlipAPC(pane string) string`, `panes.NextAPC(pane string) string`, `panes.OpenAPC(host, chain string) string`.

- [ ] **Step 1: Check the keys.** Read the current key maps of Claude Code, Codex and OpenCode (their docs or `--help`), and readline's `M-s`/`M-n` defaults. Record each tool's use of `M-s` and `M-n` in a table in spec § The flip, with the source and its date. If any tool uses one of them, pick the nearest free Option key and replace it in the spec, DESIGN.md and this plan.
- [ ] **Step 2: Write the failing test** `TestFlipAPCsRoundTrip`. It asserts:
  - `FlipAPC("%171") == "\x1b_matchblox;flip=%171\x1b\\"`
  - `NextAPC("%171") == "\x1b_matchblox;next=%171\x1b\\"`
  - `OpenAPC("ws-1","c7") == "\x1b_matchblox;open=ws-1/c7\x1b\\"`
  - the console's parser (Task 7) reads each one back.
- [ ] **Step 3: Implement** the three functions next to `HomeKey`.
- [ ] **Step 4:** Run `go test ./internal/panes/ -run TestFlipAPCs`. Expected: PASS.
- [ ] **Step 5: Commit.** `feat(panes): the flip and next keys and their APCs`

### Task 3: The room's rows

**Files:**
- Create: `internal/app/room.go`, `internal/app/room_test.go`
- Modify: `internal/app/model.go` (a `room *room` field)

**Interfaces:**
- Consumes: `chainKey`, `Model.chainKeyOf`, `m.chainIx`, `Model.unitOf(k) (state.ChainStoker, bool)`, `Model.unitGoes(u)`.
- Produces:
  - `type room struct{ k chainKey; at int; typing bool; draft string; from string; unfolded bool; act string }`
  - `type roomKind int`, with kinds `roomNeed, roomStep, roomMore, roomMatch, roomShips, roomGit, roomTalk`
  - `type roomRow struct{ kind roomKind; id string; text string; acts []string }`
  - `func (m Model) roomRows(k chainKey) []roomRow`
  - `func (m Model) openRoom(k chainKey) Model`, which puts the cursor on the need row, else on the talk row
  - `func (m Model) needOf(k chainKey) (row roomRow, more int, ok bool)`

- [ ] **Step 1: Write the failing tests:**
  - `TestRoomRowsLeadWithTheOldestNeed`: two matches ask, at 14:02 and 14:05. Then `rows[0].kind == roomNeed`, its text names the 14:02 one, and `more == 1`.
  - `TestRoomCardShowsTheActiveStepAndOneEachSide`: a plan of 8 steps with step 4 active gives roomStep rows for 3, 4 and 5, then one `roomMore` with text `↓ 5 more`. With `unfolded` set, all 8.
  - `TestRoomWithNoNeedHasNoNeedRow`.
  - `TestRoomOfAChainOfOneNamesTheSharedStoker`: the card title is `≋ the stoker · every session in no chain`, and the talk text is `› tell the stoker…`.
  - `TestRoomWithNoStokerAsksForTheOrder`: the talk text is `› what should hearth's stoker do?`.
  - `TestOpenRoomPutsTheCursorOnTheNeedElseTheTalkLine`.
  - `TestMatchRowCarriesMarkRepoAndInbox`: the row holds the agent's mark, `✉ 2` for two notices, and the repo only when the team's matches span two repos.
  - `TestAnUnseenMateStaysFaint`: a member with no sample row shows `not seen · 4m` in `faint`, and the room still lists its team.
  - `TestAMatchInTwoTeamsSaysAlsoIn`: `· also in kindle`. This test is skipped with `t.Skip("multi-team membership: matchblox-chain")` until `chainIx` reports a match in two teams.
- [ ] **Step 2:** Run `go test ./internal/app/ -run 'TestRoom|TestOpenRoom'`. Expected: FAIL (undefined).
- [ ] **Step 3: Implement** `room.go`. ships and git rows filter `m.snap.Ships` and `m.snap.Git` to the panes of the chain's matches. Their text is spec § The room 3 and 4, with `no ships yet` and `no worktrees` when the chain has none.
- [ ] **Step 4:** Run the same command. Expected: PASS.
- [ ] **Step 5: Commit.** `feat(app): a chain's room knows its rows, the need first`

### Task 4: The room's look

**Files:**
- Create: `internal/app/roomview.go`, `internal/app/roomview_test.go`, goldens in `internal/app/testdata/` (the golden harness of `screens_test.go`)
- Modify: `internal/app/theme.go` (the glyph `≋`, if glyphs live there), `internal/app/view.go` (draw the room when `m.room != nil`)

**Interfaces:**
- Consumes: `roomRows`, the chat pane of #261 (`chat.go`, the right side of Sessions).
- Produces: `func (m Model) roomView(w, h int) string`.

- [ ] **Step 1: Write the failing golden tests** for the desk (110 cols), Medium (80) and the phone (40) with the Hearth demo data:
  - `TestRoomGoldenDesk`: the room on the left at 40%, and the selected match's chat on the right.
  - `TestRoomGoldenPhone`: matches spec § The phone's first frame cell for cell.
  - `TestRoomAt40ColumnsTruncatesLongNames`: a 47-character chain name ends in `…` inside 40 columns, and the need line takes 2 lines at most.
  - `TestRoomHasOneAccent`: count the accent token in a frame where the need line, an asking match row and a ship that waits for merge all show. Expected: exactly one run, on the need line.
- [ ] **Step 2:** Run `go test ./internal/app/ -run TestRoom -update=false`. Expected: FAIL (missing goldens and missing view).
- [ ] **Step 3: Implement** `roomView`. Match rows keep the fuse column of `fuse.go` between the match and the name. Regions follow DESIGN.md §6 Regions: no boxes, labels in `faint`, hairlines. `≋` takes `muted`, and the `night` token while that chain's stoker is in charge. On Narrow, `⏎` on a match draws the chat full screen, titled `‹ hearth · ember-billing · asks`.
- [ ] **Step 4:** Run with `-update`, read every golden by eye against the spec's frames, then run without `-update`. Expected: PASS.
- [ ] **Step 5: Commit.** `feat(app): the room draws its need, card, matches, ships, git and talk line`

### Task 5: The room's keys and the talk line

**Files:**
- Create: `internal/app/roomkeys.go`, `internal/app/roomkeys_test.go`
- Modify: `internal/app/keys.go` (a `roomKeys` binding set, like `sheetKeys`)

**Interfaces:**
- Consumes: `roomRows`; the acts `stoker:new` and `stoker:order-chain` with body `{"chain","words","grants"}`, as `sendChain` sends them (`sheet.go`); `stoker:grant` (`stokertab.go`); the consent box (`m.pending = &action{…}`).
- Produces: `func (m Model) roomKey(msg tea.KeyMsg) (Model, tea.Cmd)` and `func (m Model) roomResult(r proto.Result) (Model, bool)`.

- [ ] **Step 1: Write the failing tests:**
  - `TestEnterActsOnTheSelectedRow`: on a match it opens the chat; on `↓ N more` it unfolds; on the talk row it starts typing.
  - `TestYGivesAGoOnTheNeedLine`: sends `stoker:grant` with the unit's id and the step.
  - `TestAGoThatMergesAsksInTheConsentBox`: a step whose act is a merge sets `m.pending`, with the exact argv shown, and sends nothing before a typed `y`.
  - `TestTypingTakesEveryKeyUntilEnterOrEsc`: while typing, `y`, `]` and `q` go into the draft, the key line is `typing to hearth's stoker · ⏎ sends · esc stops`, and esc keeps the draft.
  - `TestTalkStartsAStokerWithNoSheet`: on a chain with no unit, ⏎ sends `stoker:new` with `grants: []` and the typed words, and no sheet opens.
  - `TestTalkToARunningStokerSendsAnOrder`: it sends `stoker:order-chain`.
  - `TestTalkSendsOneOrderAtATime`: a second ⏎ while `room.act != ""` sends nothing.
  - `TestTalkRefusalKeepsTheDraft`: the result's `Err` shows as `not sent: <why>`, and the draft stays.
  - `TestBracketsWalkTheChains`: `]` and `[` move to the next and the previous chain in list order.
  - `TestEscStepsBackOneLevel`: typing → room → no room.
- [ ] **Step 2:** Run `go test ./internal/app/ -run 'TestEnterActs|TestYGives|TestAGo|TestTyping|TestTalk|TestBrackets|TestEsc'`. Expected: FAIL.
- [ ] **Step 3: Implement.** After a send, the toast is `sent to hearth's stoker`; after a go, `go given: <step>`. While typing, the key line is as above. Else it shows the selected row's keys, and at 40 columns its three keys and `? keys`.
- [ ] **Step 4:** Run the same command. Expected: PASS.
- [ ] **Step 5: Commit.** `feat(app): the room's keys give gos, open chats and talk to the stoker`

### Task 6: The way in (step 1)

**Files:**
- Modify: `internal/app/fuse.go` (the chain head row), `internal/app/lists.go`
- Test: `internal/app/room_test.go`

**Interfaces:**
- Consumes: `openRoom`.
- Produces: ⏎ on a chain's head row in Sessions calls `openRoom`. On a session in no chain, `o` opens its chain-of-one room.

- [ ] **Step 1: Write the failing tests:** `TestEnterOnAChainHeadOpensItsRoom` and `TestOOnALooseSessionOpensItsChainOfOne`.
- [ ] **Step 2:** Run them. Expected: FAIL.
- [ ] **Step 3: Implement** the two keys. The sheet stays reachable from the card with `e` (edit the run's goes), as spec § Waking a stoker says.
- [ ] **Step 4:** Run them. Expected: PASS.
- [ ] **Step 5: Commit.** `feat(app): a chain's head opens its room`

### Task 7: The flip

**Files:**
- Modify: `internal/panes/home.go` (a `FlipKeysArgv() []string`, run where `HomeKeyArgv` runs), `internal/app/chat.go` (parse the three APCs next to `commandPrefix`), `internal/app/room.go`
- Create: `internal/app/flip_test.go`
- Modify: `ios/Matchblox/BeltView.swift` (a flip and a next button)

**Interfaces:**
- Consumes: `panes.FlipKey`, `NextKey`, `FlipAPC`, `NextAPC`, `OpenAPC`; the console's switch to a pane (the act behind `Switch` in `keys.go`/`lifeform.go`).
- Produces: `func (m Model) flipFrom(pane string) (Model, tea.Cmd)`, `func (m Model) nextFrom(pane string) (Model, tea.Cmd)`, `func (m Model) openFromAPC(host, chain string) Model`.

tmux binding: on a pane that runs the console, the key goes on to the console as the APC with an empty pane. On a session pane of this server: `switch-client -t =matchblox`, then `send-keys -t <the console's pane> -H <FlipAPC(#{pane_id})>`. On a server without a console: nothing.

- [ ] **Step 1: Write the failing tests:**
  - `TestFlipArgvBindsBothKeys`: argv equality, in the style of `home_test.go`.
  - `TestFlipFromASessionOpensItsChainsRoomOnTheNeed`: `room.from == "%171"`.
  - `TestFlipInTheRoomGoesBackToWhereItCameFrom`: a switch act to `%171`.
  - `TestFlipInTheRoomWithNoCameFromStartsTyping`.
  - `TestFlipBackToAGonePaneStays`: the toast says `That session ended.`, and the room stays.
  - `TestFlipFromALooseSessionOpensTheChainOfOne`.
  - `TestNextWalksMatchesThenTheStokerThenTheFirst`.
  - `TestFlipFromAPaneOfAnotherServerSendsNothing`: no `send-keys` to a non-console pane appears in the argv.
  - `TestOpenAPCOpensThatChainsRoom`.
- [ ] **Step 2:** Run `go test ./internal/panes/ ./internal/app/ -run 'TestFlip|TestNext|TestOpenAPC'`. Expected: FAIL.
- [ ] **Step 3: Implement.** Keep `room.from` per console client (the console is one client), so no service state changes. On iOS, the belt's two buttons send `FlipAPC("")` and `NextAPC("")`, which the console reads as "from the selected session". They are 44 × 44 pt, with VoiceOver labels `Flip to the stoker` and `Next match`, and a `.light` haptic (Task 11's `Haptics.flip()`).
- [ ] **Step 4:** Run the same command, then `cd ios && make test` (or the repo's iOS test target). Expected: PASS.
- [ ] **Step 5: Commit.** `feat: Option-s flips between a session and its chain's room`

### Task 8: The states

**Files:**
- Modify: `internal/app/room.go`, `internal/app/roomview.go`
- Test: `internal/app/roomstates_test.go`

**Interfaces:**
- Consumes: Tasks 3–5.

- [ ] **Step 1: Write one failing test for each cell** of spec § States that names text. Each asserts the exact copy:
  - `≋ waking…`
  - `the stoker did not start: <why>`, with the draft kept
  - `≋ hearth's stoker is quiet since 14:02 · ⏎ asks it`, after 60 s with no step
  - `the chain has no matches · + start one here`
  - `reading…`
  - `no ships yet`, `no worktrees`
  - the §6 GitHub strip text on the ships line
  - `not sent: <why>`
  - a peer out of reach: its chains stay dimmed under `ws-1 is not reachable · <why>`
  - before the first state: names only, no zero figures
- [ ] **Step 2:** Run `go test ./internal/app/ -run TestRoomState`. Expected: FAIL.
- [ ] **Step 3: Implement** the states.
- [ ] **Step 4:** Run the same command. Expected: PASS.
- [ ] **Step 5: Commit.** `feat(app): the room names every state and its action`

### Task 9: The header and the night

**Files:**
- Modify: `internal/app/view.go` (the header's stoker word), `internal/app/night.go`
- Test: `internal/app/night_test.go`, `internal/app/room_test.go`

- [ ] **Step 1: Write the failing tests:**
  - `TestHeaderCountsRunningStokers`: 2 units running → `≋ 2`; none and the host off → `stoker off`.
  - `TestAChainStokerInChargePaintsOnlyItsGlyph`: no night palette on the console, and `≋` in the `night` token.
  - `TestTheHostRunStillWearsTheNight`: unchanged behaviour.
- [ ] **Step 2:** Run them. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4:** Run them. Expected: PASS.
- [ ] **Step 5: Commit.** `feat(app): the header counts the stokers that run`

### Task 10: The island and the buzz name the chain

**Files:**
- Modify: `internal/push/payload.go` (`ContentState` gains `Chain *TopChain` with `json:"chain,omitempty"`), `internal/push/state.go` (`Build`), `internal/push/numbers.go` (`func (n *Numbers) Chain(id string) int`)
- Modify: `ios/MatchbloxKit/Sources/MatchbloxKit/IslandState.swift` (`TopChain`), `ios/MatchbloxIsland/IslandWidget.swift`, `ios/Matchblox/IslandSettings.swift` (`Show`: `Chains`, `Usage`, `Auto`), `ios/MatchbloxBuzz/NotificationService.swift`
- Test: `internal/push/state_test.go`, `internal/push/payload_test.go`, the MatchbloxKit tests

**Interfaces:**
- Produces: `type TopChain struct{ N int `json:"n"`; State string `json:"state"`; Done int `json:"done"`; Steps int `json:"steps"`; Stokers int `json:"stokers"` }`. State is one of `asks`, `go`, `waits`, `burns`, `quiet`. Names travel only as numbers, as sessions do today.

- [ ] **Step 1: Write the failing tests:**
  - `TestBuildPicksTheNeediestChain`: `Chain.N` is hearth's number, `State == "asks"`, `Done == 1`, `Steps == 3`.
  - `TestAnOldPayloadDecodesWithoutChain` (Go and Swift): a JSON without `chain` decodes, and the widget draws today's Sessions face.
  - `TestBuzzNamesTheChain`: the sealed body's text is `hearth: merge #4552? needs your go`.
  - Swift: `testUnknownChainSaysChainN`, which renders `chain 3`.
- [ ] **Step 2:** Run `go test ./internal/push/` and the Swift tests. Expected: FAIL.
- [ ] **Step 3: Implement** the faces of spec § The phone:
  - lock screen: `▰ ws-1  ≋ 2 ╿ 1` over `hearth · ember-billing asks`;
  - compact: the match and `hearth`;
  - minimal: the front of the match, with `╿` while anything waits;
  - expanded: the need, `plan 1/3`, and `Answer` or `Open`.

  Tapping opens the app on that host and sends `OpenAPC(host, chain)`. A wait still breaks through on Usage for 60 s.
- [ ] **Step 4:** Run the same tests. Expected: PASS.
- [ ] **Step 5: Commit.** `feat: the island and the buzz name the chain that needs you`

### Task 11: Haptics

**Files:**
- Modify: `ios/Matchblox/Haptics.swift`, `ios/Matchblox/ConsoleTerminalView.swift`, `ios/Matchblox/ConsoleView.swift` (or wherever the app reads the console's state, for state changes), the app settings view (a `Haptics` toggle, `@AppStorage("haptics")`, default `true`)
- Test: the Matchblox unit tests (a `HapticsPolicy` value type tested without a device)

**Interfaces:**
- Produces:
  - `struct HapticsPolicy { mutating func onStates(_ old: [Int: String], _ new: [Int: String], at: Date) -> HapticEvent? }`
  - `enum HapticEvent { case needsYou, finished }`
  - `Haptics.needsYou()` (`.warning`), `Haptics.finished()` (`.soft`, intensity 0.6), `Haptics.tick()` (`UISelectionFeedbackGenerator`), `Haptics.edge()` (`.rigid`), `Haptics.flip()` (`.light`), `Haptics.given()` (`.success`), and `Haptics.failed()`, which exists already.

- [ ] **Step 1: Write the failing tests:**
  - `testAskPlaysNeedsYou`
  - `testFinishPlaysFinished`
  - `testBusyIdleChurnPlaysNothing`
  - `testBurstPlaysOnce`: ten changes in one sample give one event.
  - `testTwoSecondGap`: a second event within 2 s gives nil.
  - `testSettingOffPlaysNothing`
  - `testScrollTicksAtMost30PerSecond`: a `ScrollTicker` value type over row offsets and timestamps.
  - `testEdgePlaysOnceAtTop`
- [ ] **Step 2:** Run the iOS unit tests. Expected: FAIL.
- [ ] **Step 3: Implement.**
  - The terminal view's wheel pan and scrollback call `ScrollTicker`. Each row moved ticks, and the top or bottom plays `edge()` once until the scroll leaves that edge.
  - State changes come only from the foreground app, for the host on screen.
  - Generators stay prepared, as `Haptics.prepare()` does.
  - With Reduce Motion on, haptics still play.
- [ ] **Step 4:** Run the iOS unit tests. Expected: PASS. Then try it on a device: scroll the scrollback, flip, give a go. Record what you felt in the PR body under "Not proven" if no device is at hand.
- [ ] **Step 5: Commit.** `feat(ios): haptics for a match that needs you, a finish, scrolling and the flip`

### Task 12: Step 1's docs and DESIGN.md

**Files:**
- Modify: `DESIGN.md` §4 (`≋`), §6 (the room, the flip keys, the need line, the header's `≋ N`, the go consent rule), §9 (the Chains face, the haptics); `docs/console.md`, `docs/stoker.md`, `docs/keys.md`, `docs/phone.md`; `CHANGELOG.md` [Unreleased] → Added
- Test: `internal/app/docs_test.go` (if it checks docs against keys), `mkdocs build --strict`

- [ ] **Step 1:** Write the text, copying the spec's strings verbatim.
- [ ] **Step 2:** Run `go test ./internal/app/ -run TestDocs` and `mkdocs build --strict`. Expected: PASS.
- [ ] **Step 3: Commit.** `docs: the chain room, the flip and the island's Chains face`

### Task 13: The fold to `chains · machine` (step 2)

**Files:**
- Modify: `internal/app/tabs.go` (the consts and `views`: `tabChains`, `tabMachine`), `internal/app/sessions.go` (remove `groupNeed`…`groupNone` and the `sessGroup` keys), `internal/app/lists.go`, `internal/app/stokertab.go` (its card moves into the room; its host controls into Machine), the Ships, Git, History and Panes views (now regions of the room or of Machine), `config.example.toml` and the config reader (drop the grouping key, and warn once with `sessions.group is gone: chains are the only grouping` if a config still sets it)
- Test: `internal/app/tabs_test.go`, `internal/app/stokertab_test.go` (move its cases to the room tests), the goldens

**Interfaces:**
- Produces: `tabChains = iota`, `tabMachine`. The console opens on `tabChains`. Digits `1` and `2`; `p` is gone.

- [ ] **Step 1: Write the failing tests:**
  - `TestTabsAreChainsAndMachine`: the tab row golden is `CHAINS  machine`.
  - `TestChainListOrderHoldsWithinATier`: two chains in `works` swap their sample order, and the list order stays. A chain that starts to ask moves to the top, and the cursor stays on its chain.
  - `TestChainListHoldsDoorsGhostAndRecommendations`: the rows are in spec order.
  - `TestMachineRegionsInOrder`: usage, the machine, the busy loops, the top processes, the stoker, DEVICES, history, panes.
  - `TestBackSelectsTheLastChain`.
  - `TestOldGroupingConfigWarnsOnce`.
- [ ] **Step 2:** Run `go test ./internal/app/`. Expected: FAIL.
- [ ] **Step 3: Implement.** Delete the dead code: groupings, the tab views that moved, and their keys. Every test of a removed view either moves to its new home or is deleted with a line in the commit body saying which.
- [ ] **Step 4:** Run `go test -race ./...` and the demo tests (`go test ./internal/demo/`). Expected: PASS. Update the goldens with `-update` and read each one.
- [ ] **Step 5: Commit.** `feat(app)!: the console opens on chains, and its tabs are chains and machine`. The commit body lists the removed tabs, the removed groupings and the removed keys.

### Task 14: Step 2's docs and DESIGN.md

**Files:**
- Modify: `DESIGN.md` §6 Tabs, Sessions, Ships, Git and the Machine tab; `docs/console.md`, `docs/keys.md`, `docs/ships.md` (folded into `docs/console.md` with a redirect in `mkdocs.yml`, if the docs use redirects, else deleted from `nav`); `CHANGELOG.md` [Unreleased] → Changed and Removed
- [ ] **Step 1:** Write the text.
- [ ] **Step 2:** Run `mkdocs build --strict` and `go test ./internal/app/ -run TestDocs`. Expected: PASS.
- [ ] **Step 3: Commit.** `docs: chains and machine are the console's two tabs`

### Task 15: The landing page (pre-merge must-have)

**Files:**
- None by this session: matchbloxrelease owns `www/index.html`.

- [ ] **Step 1:** When Tasks 1–14 are green, send matchbloxrelease (`matchblox send`, at most 500 characters) the line and the visual for "The chain" beat: a session, ⌥s, the stoker's room, ⌥s back.
- [ ] **Step 2:** matchbloxrelease commits the page change to `design/chain-room`. Pull it, then run `.github/scripts/landing-guard.sh "<PR title>" <body-file> origin/main`. Expected: `OK`.
- [ ] **Step 3:** The PR's CI shows the `Landing page` step green. Only then is the PR ready to merge.
