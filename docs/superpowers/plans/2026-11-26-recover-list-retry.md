# Recover list retry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Spec: docs/superpowers/specs/2026-11-26-recover-list-retry-design.md
Spec approved: 2026-11-26
Test command: go test ./internal/service ./cmd/matchblox -count=1
Plan approved: 2026-11-26 by the owner
Next: builder:build

**Goal:** After recover starts an agent, matchblox retries a failed pane list silently, then offers `⏎ retry` without starting a second session.

**Architecture:** `settled` loops `ListStrict` up to three times and stops at the first success. A list error no longer writes `reopened`. It writes `unconfirmed` keyed by conversation, with `As` and the agent command. The next recover of that key lists at once and never calls Start.

**Tech Stack:** Go, existing `TmuxOut` Fake list. No new module.

**Spec:** `docs/superpowers/specs/2026-11-26-recover-list-retry-design.md`

## Global Constraints

- Why copy, verbatim: `Recover didn’t hear back · ⏎ retry`
- Three `ListStrict` at most, 400ms between errors, stop at the first success.
- Confirm does not wait `recoverSettle`. Confirm does not run `recoverRow` cannot checks. Confirm lists `row.As`.
- Empty list with no error (no-server, missing socket) stays today’s exited-at-once path. Do not retry it.
- Signed Conventional Commits. Never `--no-verify`. Landing page unchanged (not a feat).
- Lands on PR #514 (`feat/200-mux-extract`). Do not push until the owner says yes.

## Review Focus

1. **No-server is not this CTA.** `ListStrict` empty (`no server running`) → exited-at-once, one list. → Task 1, `TestRecoverNoServerIsExitedAtOnce`.
2. **First list success lists once.** A live pane on the first `ListStrict` must not wait 400ms twice. → Task 1, `TestRecoverListSucceedsOnFirstTryListsOnce`.
3. **Confirm does not wait settle.** After the CTA, `s.settle = time.Hour` still returns. → Task 2, `TestRecoverRetryAfterListCTADoesNotWaitSettle`.
4. **Confirm lists `As`, not `Name`.** Start took `web-2`; confirm must not look for `web`. → Task 2, `TestRecoverRetryAfterListCTAListsAs`.
5. **Preview after CTA stays cannot.** `matchblox recover` with no names must not print `can reopen`. → Task 2, `TestRecoverPreviewAfterListCTAKeepsWhy`.

## File map

- `internal/service/service.go` (change) — `listPause` next to `settle`; `unconfirmed` next to `reopened`.
- `internal/service/recover.go` (change) — retry loop, `unconfirmed` map, confirm path, Why constant.
- `internal/service/recover_test.go` (change) — the recover tests this spec names.
- `cmd/matchblox/recover_test.go` (change) — CLI print of the CTA Why.

## Lens findings

- Engineering, reuse: `settled`, `TmuxOut`, `recoverLive`, `sessionRunsOf`, `printRecover`. No new package. No mux change. `listPause` uses the same zero rule as `settle`; tests set a nanosecond. Changed Task 1 Interfaces after the cold read, because a zero `listPause` would ship 0ms gaps or 400ms tests.
- Engineering, scale: at most three `list-panes` per started row, plus 0.8s. Unknown production glitch rate. Cheap check: the retry tests count `list-panes` calls. No change.
- Design, notices: `reopening web` stays (not this slice). CTA Why names the miss and the next key. Rate 9. A 10 would name the session in Why; the strip already prefixes `web not reopened:`. No change, because the spec pins Why.
- Design, CLI: same Why; name sits in column one. No change.
- Architecture: no new API, store or agent tool. Recover.json unchanged. Lens not run.

## Task 1: Silent retries then the CTA

**Files:** `internal/service/service.go`, `internal/service/recover.go`, `internal/service/recover_test.go`, `cmd/matchblox/recover_test.go`

**Interfaces:**

- Consumes: `Service.mux().ListStrict`, `s.settle`, `startedPanes` / `TmuxOut`, `paneLine`.
- Produces: `recoverListRetryWhy = "Recover didn’t hear back · ⏎ retry"`; `recoverListTries = 3`; `recoverListPause = 400 * time.Millisecond`; `Service.listPause time.Duration` with the same zero rule as `s.settle` (0 means the production constant). `recoverTest` sets `s.listPause = time.Nanosecond` so three errors add no real wait. `listPanes` retries errors only.

- [ ] **Step 1: Write the failing tests** in `internal/service/recover_test.go`. In `recoverTest`, set `s.listPause = time.Nanosecond` (do not leave it 0). Count `list-panes` in `TmuxOut`.

`TestRecoverListGlitchThenLiveReopens`: first two `ListStrict` (`list-panes`) return `&exec.ExitError{}`; the third returns `paneLine("%42", "web:1.1", 1, "/dev/pts/9", false)`. Row `web`: State `reopened`, Why `""`, three `list-panes` calls.

`TestRecoverListGlitchGivesRetryCTA`: three `&exec.ExitError{}`. Why equals `recoverListRetryWhy`, State `cannot`, Start (`new-session`) ran once, no `kill-session`, `recoverLive` is false for that conversation.

`TestRecoverListSucceedsOnFirstTryListsOnce`: first list is a live `web` pane. One `list-panes` call. State `reopened`.

`TestRecoverNoServerIsExitedAtOnce`: `TmuxOut` returns `&exec.ExitError{Stderr: []byte("no server running on /tmp/tmux-1000/default\n")}`. Why contains `exited at once`. One `list-panes` call. Why must not be `recoverListRetryWhy`.

`TestRecoverMissingAgentHasNoListRetry`: `LookPath` misses `codex`. Why contains `codex is not installed`. Zero `list-panes` calls.

Change `TestRecoverDoesNotTreatAListFailureAsExited` to expect `recoverListRetryWhy` instead of `tmux did not list panes`, still no `kill-session`, still not live.

In `cmd/matchblox/recover_test.go`, `TestRecoverPrintsListRetryCTA`: one cannot row with Why `Recover didn’t hear back · ⏎ retry`, `printRecover(..., true)` prints `cannot reopen` and that Why, and returns an error that contains `could not reopen`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/service -count=1 -run 'TestRecoverListGlitchThenLiveReopens|TestRecoverListGlitchGivesRetryCTA|TestRecoverListSucceedsOnFirstTryListsOnce|TestRecoverNoServerIsExitedAtOnce|TestRecoverMissingAgentHasNoListRetry|TestRecoverDoesNotTreatAListFailureAsExited'`

Expected: `TestRecoverListGlitchThenLiveReopens`, `TestRecoverListGlitchGivesRetryCTA`, and `TestRecoverDoesNotTreatAListFailureAsExited` FAIL (Why still `tmux did not list panes`, or one `list-panes` only). `TestRecoverListSucceedsOnFirstTryListsOnce`, `TestRecoverNoServerIsExitedAtOnce`, and `TestRecoverMissingAgentHasNoListRetry` may PASS: they already match today’s one-list path.

Run: `go test ./cmd/matchblox -count=1 -run TestRecoverPrintsListRetryCTA`

Expected: FAIL undefined test, then PASS once the test exists (printRecover already prints Why). If it passes on the first run, keep it.

- [ ] **Step 3: Implement the retry in `settled`**

In `internal/service/recover.go` add `recoverListRetryWhy`, `recoverListTries`, `recoverListPause`. In `internal/service/service.go` add `listPause time.Duration`.

`listPanes(ctx)`: for `i := 0; i < recoverListTries; i++` call `ListStrict`. On `err == nil`, return that slice. On error and not last try, wait `s.listPause` or `recoverListPause` (same pattern as `s.settle`), honour `ctx.Done`. On last error, return it.

`settled` calls `listPanes` instead of one `ListStrict`. On list error: set Why to `recoverListRetryWhy`. Do **not** write `s.reopened`. (Task 2 writes `unconfirmed`. After Task 1, a second recover may still Start. Task 2 closes that.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/service ./cmd/matchblox -count=1 -run 'TestRecoverListGlitch|TestRecoverListSucceeds|TestRecoverNoServer|TestRecoverMissingAgent|TestRecoverDoesNotTreatAListFailureAsExited|TestRecoverPrintsListRetryCTA|TestRecoverSaysWhenTheAgentExitsAtOnce'`

Expected: PASS. Exited-at-once and dead-pane tests still pass.

- [ ] **Step 5: Commit**

```bash
git add internal/service/service.go internal/service/recover.go internal/service/recover_test.go cmd/matchblox/recover_test.go
git commit -m "$(cat <<'EOF'
fix(recover): retry a failed pane list before the cannot CTA

A list glitch is often gone a beat later. Show nothing until three
ListStrict errors, then Recover didn’t hear back · ⏎ retry.

EOF
)"
```

**Done when:** those tests pass. A list error is not exited-at-once and is not live.

## Task 2: Confirm lists the session Start opened

**Files:** `internal/service/service.go`, `internal/service/recover.go`, `internal/service/recover_test.go`

**Interfaces:**

- Consumes: Task 1 `listPanes`, `recoverListRetryWhy`.
- Produces: `Service.unconfirmed map[string]unconfirmedAt` with `at time.Time`, `as`, `cmd string`. Key is `conversation(agent, id)`. `recover()` on that key copies `Name`, `Agent`, `Conversation`, `Dir` from the recovery `Entry` in the loop (`e.Session`, `e.Agent`, `e.ConversationID`, `e.Cwd`) and `As` from `unconfirmed.as`. When reopen: skip `recoverRow` Start and skip its cannot checks; call `listPanes` with no `settle` wait; resolve live / dead / CTA. Preview (no reopen): return `cannot` with `recoverListRetryWhy` and that `As`.

- [ ] **Step 1: Write the failing tests**

`TestRecoverRetryAfterListCTADoesNotStartAgain`: after Task 1’s three-error recover, a second `Recover{All: true}`: `new-session` count stays 1; Why still `recoverListRetryWhy`; six `list-panes` total (3+3).

`TestRecoverRetryAfterListCTAReopensWhenListWorks`: after the CTA, second recover’s `TmuxOut` returns a live `web` pane. State `reopened`. No second `new-session`.

`TestRecoverRetryAfterListCTAExitedAtOnce`: after the CTA, second recover lists a dead `web` pane. Why contains `exited at once`. `kill-session` of `=web`. No second `new-session`.

`TestRecoverRetryAfterListCTADoesNotWaitSettle`: after the CTA, set `s.settle = time.Hour`. Second recover must return (use `t.Context()` is enough if confirm skips `settled`’s wait; if it hangs the test times out). No `time.After(s.settle)`.

`TestRecoverRetryAfterListCTAListsAs`: same setup as `TestRecoverTakesAFreeName` (publish a pane `Target: "web:1.0"` on the new server so Start uses `web-2`). Drive three list errors on the first recover. Second recover lists a live `web-2:1.1` pane, not `web`. State `reopened`.

`TestRecoverPreviewAfterListCTAKeepsWhy`: after the CTA, `recoverAct` with `proto.Recover{}` (preview). State `cannot`, Why `recoverListRetryWhy`, `new-session` still 1.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/service -count=1 -run 'TestRecoverRetryAfterListCTA|TestRecoverPreviewAfterListCTAKeepsWhy'`

Expected: FAIL — second recover Starts again, or preview is `ready`, or confirm waits `s.settle`.

- [ ] **Step 3: Remember `unconfirmed` and confirm**

On list error in `settled`, store `s.unconfirmed[conversation(row.Agent, row.Conversation)] = {now, row.As, cmd}` instead of `reopened`. Add `unconfirmed` on `Service` in `service.go`.

In `recover`’s entry loop, if `unconfirmed` has `conversation(e.Agent, e.ConversationID)`, do not call `recoverRow`. Build the row from `e` (`Name: e.Session`, `Agent: e.Agent`, `Conversation: e.ConversationID`, `Dir: e.Cwd`, `As: unconfirmed.as`), State `cannot`, Why `recoverListRetryWhy`. If `reopen`, call `listPanes` immediately (no `s.settle`), then the same live / dead / CTA branch as `settled` (kill dead session, `reopened` on live, keep `unconfirmed` on another list error). If not `reopen`, append the cannot row and skip Start.

Delete `unconfirmed` when confirm resolves to reopened or exited-at-once.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/service -count=1 -run 'TestRecover'`

Expected: PASS, including `TestRecoverTakesAFreeName` and live-after-reopen.

- [ ] **Step 5: Commit**

```bash
git add internal/service/service.go internal/service/recover.go internal/service/recover_test.go
git commit -m "$(cat <<'EOF'
fix(recover): retry the list, not a second start, after the CTA

⏎ after Recover didn’t hear back confirms the session Start opened.
It does not wait again and it does not spawn web-2.

EOF
)"
```

**Done when:** those six tests pass. Preview after the CTA is still cannot.

## Task 3: Review the recover retry

**Files:** the Task 1–2 diff on `feat/200-mux-extract`.

- [ ] **Step 1:** Load `builder:review`. Spec `docs/superpowers/specs/2026-11-26-recover-list-retry-design.md`. Test command from the header.

- [ ] **Step 2:** Hostile: a list error must not fail open as live or as exited-at-once. Re-run Task 1’s list-failure tests.

- [ ] **Step 3:** No open Critical or Important. Do not push. Do not merge.

**Done when:** `builder:review` runs on the branch diff. No open Critical or Important. Do not push. Do not merge.

## Spec map

| Spec line | Task |
|---|---|
| First two lists fail, third live → `reopened`, no CTA | Task 1 `TestRecoverListGlitchThenLiveReopens` |
| Three list errors → CTA Why, row cannot, Start once | Task 1 `TestRecoverListGlitchGivesRetryCTA` |
| Second recover starts zero times; lists fail → same Why | Task 2 `TestRecoverRetryAfterListCTADoesNotStartAgain` |
| Second recover, live list → `reopened` | Task 2 `TestRecoverRetryAfterListCTAReopensWhenListWorks` |
| Second recover, dead list → exited-at-once | Task 2 `TestRecoverRetryAfterListCTAExitedAtOnce` |
| Missing agent, no list retry | Task 1 `TestRecoverMissingAgentHasNoListRetry` |
| CLI prints Why and fails | Task 1 `TestRecoverPrintsListRetryCTA` |
| Anti-goal: no cannot before three lists | Task 1 retry loop |
| Anti-goal: ⏎ does not start a second session | Task 2 |
| Anti-goal: list error is not exited-at-once | Task 1 `TestRecoverDoesNotTreatAListFailureAsExited` |
| Anti-goal: list error is not live | Task 1 `TestRecoverListGlitchGivesRetryCTA` |
| Confirm skips 1.5s | Task 2 `TestRecoverRetryAfterListCTADoesNotWaitSettle` |
| Confirm lists `As` | Task 2 `TestRecoverRetryAfterListCTAListsAs` |
| Preview stays cannot | Task 2 `TestRecoverPreviewAfterListCTAKeepsWhy` |
| No-server stays exited-at-once | Task 1 `TestRecoverNoServerIsExitedAtOnce` |

## Cold read

Cold read: 2026-11-26, 1 fresh reader(s). Blocker 0, major 3, nit 3. Fixed 6, contested 0, for the person 0, declined 0. Second read: 5 left.

Second read, for the person (not edited after this read):

- Major — `unconfirmed.at` is stored and never read. Apply `reopenedFor` (⏎ after a minute Starts again) or never expire.
- Major — confirm uses `e.Cwd`, not the `--dir` Start used.
- Nit — Task 1 vs Task 2: skip `recoverRow` argv vs do not call it.
- Nit — one `listPanes` per `settled` vs per started row.
- Nit — CLI test expected FAIL vs already prints Why.
