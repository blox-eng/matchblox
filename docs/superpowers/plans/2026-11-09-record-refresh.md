# Record Refresh Before a Switch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A switch from an idle Claude Code or Codex agent first asks that agent to write its task record, waits up to 3 minutes for the turn to end, then captures and switches.

**Architecture:** The service runs the whole wait inside `switchAgent`, in `internal/service/refresh.go`. It types a fixed request with the guarded peer-typing path, polls the sampled session for the turn end, publishes the wait in `proto.State.Refreshes`, and marks the pane as switching so other input refuses fast. A `switchnow:<pane>` act ends the wait. The capture and the result notes name the outcome.

**Tech Stack:** Go, bubbletea console (`internal/app`), tmux.

**Spec:** `docs/superpowers/specs/2026-11-09-record-refresh-design.md`

## Global Constraints

- Time limit: 3 minutes (`refreshLimit = 3 * time.Minute`, a `var` so tests shorten it).
- Refresh only Claude Code (`claude`) and Codex (`codex`) sessions with a verified `Transcript` and a `MatchContext` (a managed record).
- The request is one line and names no agent. Its text is fixed once built: lead `Update your task record at `, then the record path quoted with `%q`, then the tail ` for a handover to another agent now: the goal, the latest user direction, the completed work, the decisions, the checks that ran and the next step. Do no other work.`
- The excerpt mark is `[matchblox refresh request]`.
- Copy: `<name> switches agents now: send again after` (fast refusal of other input). `switching already` (second switch or kill). `<name> took your prompt while it wrote its record: switch again` (race). `esc switches now` (console). `Ctrl-C switches now` (CLI).
- Every view fits 44 columns (the iOS app and the phone).
- Older clients omit `no_refresh` and get the refresh. An older service ignores it.
- Signed Conventional Commits, `(#336)` in each subject, no `--no-verify`.

## Review Focus

1. **A request left from an earlier switch** (timed out, refused or failed, with a different record path) shows in the next capture as the mark, never as the builder's words. Test in Task 2.
2. **The builder types into the pane during the wait** (Claude Code queues it, and a turn starts right after the refresh turn). The switch refuses with the race copy, and the bundle is removed. Test in Task 4.
3. **The agent answers in chat and never writes the file.** The switch goes ahead at once with "it ended its turn without writing the record". Test in Task 4.
4. **The esc act arrives after the wait already ended** or for a pane with no wait. It answers `no switch waits in <pane>` and changes nothing. Test in Task 5.
5. **The service stops or the act's context ends mid-wait.** The switching mark and the published `Refresh` are removed (deferred), so no pane stays blocked. Test in Task 4.

---

### Task 1: Protocol

**Files:**
- Modify: `internal/proto/proto.go` (`SwitchAgent` at ~512, `State` at ~86, the act prefixes)
- Test: `internal/proto/proto_test.go`

**Interfaces:**
- Produces: `SwitchAgent.NoRefresh bool` (`json:"no_refresh,omitempty"`). `type Refresh struct { Pane, Name, Agent string; Since, Until time.Time }` with json tags `pane,name,agent,since,until`. `State.Refreshes []Refresh` (`json:"refreshes,omitempty"`). `const SwitchNowPrefix = "switchnow:"`.

- [ ] **Step 1: Write the failing test** `TestSwitchAgentOmitsNoRefresh`: `json.Marshal(SwitchAgent{Agent: "codex"})` holds no `no_refresh`; with `NoRefresh: true` it holds `"no_refresh":true`. `TestStateCarriesRefreshes`: a `State{Refreshes: []Refresh{{Pane: "%1"}}}` round-trips.
- [ ] **Step 2: Run** `go test ./internal/proto/ -run 'NoRefresh|Refreshes'`. Expected: FAIL to compile.
- [ ] **Step 3: Add the fields, the type and the prefix.** Comment `NoRefresh` with the older-client rule, as `Account` is commented.
- [ ] **Step 4: Run the tests again.** Expected: PASS.
- [ ] **Step 5: Commit** `feat(proto): a switch can skip the record refresh, and the state shows one (#336)`.

### Task 2: The request text and its excerpt mark

**Files:**
- Modify: `internal/service/matchcontext.go` (beside `switchPrompt` and `earlierSwitch`, and where `transcriptTurn` calls `earlierSwitch`)
- Test: `internal/service/matchcontext_test.go`

**Interfaces:**
- Produces: `func refreshPrompt(record string) string`. `func earlierRefresh(text string) bool`. `const refreshLead`, `const refreshNext`, `const refreshMark = "[matchblox refresh request]"`.

- [ ] **Step 1: Write the failing test** `TestARefreshRequestNeverPassesAsTheBuilders`. `earlierRefresh(refreshPrompt("/s/m-1/handover.md"))` is true. So is a request with any other path whose base is `handover.md` (Review Focus 1). Each of these is false: the lead alone; a path whose base is not `handover.md`; the tail with one changed word; the exact request plus any trailing text; the builder's words that start with the lead. A transcript fixture with the request as a user row gives an excerpt that holds `[matchblox refresh request]` and not `Update your task record`.
- [ ] **Step 2: Run** `go test ./internal/service/ -run TestARefreshRequestNeverPassesAsTheBuilders`. Expected: FAIL.
- [ ] **Step 3: Implement.** `refreshPrompt` is `refreshLead + strconv.Quote(record) + refreshNext`. `earlierRefresh` cuts the lead, takes `strconv.QuotedPrefix`, checks `filepath.Base == "handover.md"`, and requires the rest to equal `refreshNext` exactly. In `transcriptTurn`, a user text where `earlierRefresh` holds becomes `refreshMark` and is `noGoal`, as the switch mark is.
- [ ] **Step 4: Run the tests again.** Expected: PASS. Also run `go test ./internal/service/ -run Switch`. Expected: PASS.
- [ ] **Step 5: Commit** `feat: a refresh request never passes as your words in a later switch (#336)`.

### Task 3: The switching mark and its guards

**Files:**
- Create: `internal/service/refresh.go`
- Modify: `internal/service/actions.go` (`messageable` ~467), `internal/service/stoker.go` (`seen` ~172, and the `stokable` the runner calls), `internal/service/lifecycle.go` (`kill` ~215, top of `switchAgent`), `internal/service/service.go` (Service fields)
- Test: `internal/service/refresh_test.go`

**Interfaces:**
- Produces: on `Service`, `refreshes map[string]*refreshRun` under `s.mu`. `type refreshRun struct { info proto.Refresh; skip chan struct{} }`. `func (s *Service) switching(pane string) bool`. `func (s *Service) startRefresh(x sample.Session, until time.Time) (*refreshRun, func())`: the returned func removes the run and its `State.Refreshes` entry, then broadcasts. `startRefresh` also sets `s.cur.Refreshes` and broadcasts.

- [ ] **Step 1: Write the failing tests.** Run `startRefresh` for pane `%1` (name `api`), then check each guard:
  - `TestASwitchingPaneRefusesMessages`: `s.messageable("%1", pid)` errs with exactly `api switches agents now: send again after`. A peer send to `api` returns the same error.
  - `TestASwitchingPaneSkipsTheStoker`: `s.stokable("%1", "")` errs with the same text, and `seen()` reports `%1` as waiting, so a rest trigger on `%1` is not due.
  - `TestASwitchingPaneRefusesSwitchAndKill`: `switchAgent` and `kill` on `%1` refuse with `switching already`.
  - `TestTheRefreshEndsItsMark`: after the release func, all four guards pass again and `s.cur.Refreshes` is empty.
- [ ] **Step 2: Run** `go test ./internal/service/ -run 'Switching|TheRefreshEnds'`. Expected: FAIL.
- [ ] **Step 3: Implement.** Each guard checks `s.switching(pane)` first and returns at once. None of them takes `holdPane`.
- [ ] **Step 4: Run the tests again, then `go test ./internal/service/`.** Expected: PASS.
- [ ] **Step 5: Commit** `feat: a pane that switches agents refuses other input at once (#336)`.

### Task 4: The refresh wait in the switch

**Files:**
- Modify: `internal/service/refresh.go`, `internal/service/lifecycle.go` (`switchAgent`, between the account hold and `matchLaunch`), `internal/service/matchcontext.go` (`matchLaunch` and `captureMatch` gain the outcome), `internal/service/peersend.go` (`typeLine`/`showsLine` take the prefix they wait for)
- Test: `internal/service/refresh_test.go`, `internal/service/lifecycle_test.go`

**Interfaces:**
- Consumes: Task 1 (`NoRefresh`, `Refresh`), Task 2 (`refreshPrompt`), Task 3 (`startRefresh`, `refreshRun.skip`).
- Produces: `func (s *Service) refreshRecord(ctx context.Context, x sample.Session, n proto.SwitchAgent, word string) (note string)`. `note` is `record written for this switch`, or `not refreshed: <reason>`. `captureMatch(ctx, old, note string)` writes `Refresh: <note>.` as the first line of the task record section. `matchLaunch` passes `note` through. Its other callers pass `""`, and then no line is written. `var refreshLimit = 3 * time.Minute`. `var refreshPoll = 250 * time.Millisecond`.

- [ ] **Step 1: Write the failing tests** with the fake sampler and tmux the lifecycle tests already use. Shorten `refreshLimit` and `refreshPoll`. Each test asserts the capture line and the result note:
  - `TestAnIdleAgentWritesItsRecordBeforeTheSwitch`: the pane receives `refreshPrompt(<record>)` then Enter. The fake moves `TurnEnded` after the send and touches the record. The note is `record written for this switch`, and the capture starts its record section with `Refresh: record written for this switch.`
  - `TestARefreshTurnWithNoWriteSwitchesAtOnce`: the turn ends and the mtime is unchanged. The note is `not refreshed: it ended its turn without writing the record`, and the switch runs before the limit (Review Focus 3).
  - `TestARefreshTimesOut`: the turn never ends. The note is `not refreshed: the refresh timed out after 3:00`.
  - `TestNoRefreshWhen`, one table with one note each:
    - `Anyway` with a busy session: `not refreshed: api works`.
    - `Anyway` with a session that waits: `not refreshed: api waits for you`.
    - Usage stopped: `not refreshed: out of usage until 15:04`.
    - Agent `opencode` or `cursor-agent`, or no transcript: `not refreshed: opencode gives no turn end`.
    - No `MatchContext`: `not refreshed: no managed task record`.
    - `NoRefresh`: `not refreshed: skipped with --no-refresh`.
    - Empty handoff: no request typed, and no `Refresh:` line.
    - A draft in the prompt: `not refreshed: the request did not reach api: not delivered: api has a draft in its prompt`.
  - `TestTheBuildersPromptDuringTheRefreshRefusesTheSwitch`: after the refresh turn ends, the fake shows the session busy again. The switch refuses with `api took your prompt while it wrote its record: switch again`, and no `match-*` folder is left (Review Focus 2).
  - `TestACancelledRefreshLeavesNoMark`: cancel the act's context mid-wait. `s.cur.Refreshes` is empty, and `s.switching("%1")` is false (Review Focus 5).
- [ ] **Step 2: Run** `go test ./internal/service/ -run 'Refresh|NoRefreshWhen|BuildersPromptDuring'`. Expected: FAIL.
- [ ] **Step 3: Implement `refreshRecord`.**
  - Check the reasons above in the listed order. Run it only for an empty `word` (the session is idle), unless `n.Anyway` sets the reason.
  - Type with `typeLine`, with its shown-line prefix passed as `refreshLead`.
  - Note `sent := s.now()` and the record's mtime. Call `startRefresh` with `sent.Add(refreshLimit)`.
  - Poll `s.sessionAt` every `refreshPoll` until one of these happens:
    - The turn ends: `TurnEnded.After(sent) && Resting() && !s.waitsOn(pane, cur)`.
    - The limit passes.
    - `skip` closes. The note is then `not refreshed: skipped with esc`.
    - `ctx` ends.
  - Compare the mtime only after the turn ends.
  - In `switchAgent`, append the note to `notes`. When a refresh ran, the post-capture busy check uses the race copy.
- [ ] **Step 4: Run the tests again, then `go test ./internal/service/`.** Expected: PASS.
- [ ] **Step 5: Commit** `feat: an idle agent writes its task record before you switch it (#336)`.

### Task 5: The "switch now" act

**Files:**
- Modify: `internal/service/actions.go` (the act router that dispatches `switch:`), `internal/service/refresh.go`
- Test: `internal/service/refresh_test.go`

**Interfaces:**
- Consumes: Task 1 (`SwitchNowPrefix`), Task 3 (`refreshRun.skip`).
- Produces: act `switchnow:<pane>` → `proto.Result{Ran: [][]string{{"switch now"}}}`, or `Err: "no switch waits in <pane>"`.

- [ ] **Step 1: Write the failing test** `TestSwitchNowEndsTheWait`. While a refresh waits, the act returns `Ran` and the switch completes with the note `not refreshed: skipped with esc`. A second `switchnow` act, or one sent to a pane with no wait, returns `no switch waits in %1` (Review Focus 4).
- [ ] **Step 2: Run** `go test ./internal/service/ -run TestSwitchNowEndsTheWait`. Expected: FAIL.
- [ ] **Step 3: Implement.** Close `skip` once, guarded by `s.mu`. Acts already run concurrently (`service.go` ~1118), so this act reaches a switch that is still running.
- [ ] **Step 4: Run the tests again.** Expected: PASS.
- [ ] **Step 5: Commit** `feat: esc switches now, from the evidence, while the record is written (#336)`.

### Task 6: The console shows the wait

**Files:**
- Modify: `internal/app/model.go` (`jumpAfter`, the flash, and esc handling while a switch is pending), plus the session row render that reads `snap`
- Test: `internal/app/lifeform_test.go`

**Interfaces:**
- Consumes: Task 1 (`State.Refreshes`, `SwitchNowPrefix`).

- [ ] **Step 1: Write the failing tests.**
  - `TestTheSwitchShowsTheRecordWait`: the console sent `switch:%1`, and a snapshot carries `Refresh{Pane: "%1", Name: "api", Agent: "claude", Since: now-42s}`. At width 80 the screen holds `◐ claude writes its task record 0:42` and `esc switches now`. At width 44 both lines fit, with no line wider than 44.
  - `TestEscDuringTheRecordWaitSwitchesNow`: esc sends exactly one act, `switchnow:%1`, with no confirm, and the form stays closed.
  - `TestAnotherConsoleSeesTheWait`: a console that did not send the switch shows `writes its record` on `%1`'s row.
- [ ] **Step 2: Run** `go test ./internal/app/ -run 'RecordWait|AnotherConsoleSeesTheWait'`. Expected: FAIL.
- [ ] **Step 3: Implement.** The timer reads `Since` from the snapshot and the console clock, so every console counts the same. Esc maps to `switchnow:` only while `jumpAfter.pane` has a `Refresh`. Otherwise esc keeps its current meaning.
- [ ] **Step 4: Run the tests again, then `go test ./internal/app/`.** Expected: PASS.
- [ ] **Step 5: Commit** `feat(console): the switch shows the record wait, and esc switches now (#336)`.

### Task 7: The command line, docs and changelog

**Files:**
- Modify: `cmd/matchblox/life.go` (the `switch` flags, `switchWait` ~281, the result print), `cmd/matchblox/main.go` (usage line), `docs/agents.md` (Handover evidence), `docs/console.md` if it describes the switch, `CHANGELOG.md` (`[Unreleased]`), `design/0016-agent-handover-plan.md` (a dated decision)
- Test: `cmd/matchblox/life_test.go`

**Interfaces:**
- Consumes: Task 1 (`NoRefresh`, `SwitchNowPrefix`), Task 4 (`refreshLimit` value: 3 minutes).

- [ ] **Step 1: Write the failing tests.**
  - `TestSwitchTakesNoRefresh`: `--no-refresh` sets `NoRefresh` in the act's text. Combined with `--preview` it is accepted and does nothing.
  - `TestSwitchWaitsForTheRefresh`: `switchWait` is at least `newWait(...) + 3*time.Minute`.
  - `TestSwitchPrintsTheRefreshNote`: a result whose notes hold `record written for this switch` prints that line after `switched api to codex`.
- [ ] **Step 2: Run** `go test ./cmd/matchblox/ -run 'NoRefresh|WaitsForTheRefresh|RefreshNote'`. Expected: FAIL.
- [ ] **Step 3: Implement.**
  - Add the flag.
  - Add the 3 minutes to the wait.
  - While the act runs, print `<name> writes its task record first; Ctrl-C switches now` to stderr. Print it only when the snapshot shows the pane's `Refresh`.
  - On SIGINT, send `switchnow:<pane>` once and keep waiting for the result.
  - Update the docs to describe the refresh, its reasons and `--no-refresh`.
  - Add a Changed entry under `[Unreleased]` that says what the builder can now do.
  - Add a `2026-11-09` decision to 0016.
- [ ] **Step 4: Run** `go test ./...`, `go vet ./...` and `golangci-lint run ./...`. Expected: all pass, 0 issues.
- [ ] **Step 5: Commit** `feat(cli): matchblox switch waits for the record, and --no-refresh skips it (#336)`.
