Spec: docs/superpowers/specs/2026-11-20-acts-show-at-once-design.md
Spec approved: 2026-11-20
Test command: go test ./internal/app/... (each task); go test $(go list ./... | grep -v /internal/demo$) and go test ./internal/demo (the review)
Plan approved: 2026-11-20 by the owner
Next: builder:build

# Plan: a kill, a rename or a park lands in the row at once; the console tells you things in one place

## What the code holds today

- A row: `sideRow` (`internal/app/siderow.go:37`); its group: `sideStateOf` (`sidestate.go:24`), ranked by the `sideState` constants; the cursor walks `sideOrder()` (`sidelist.go:174`); the title is `sideTitle` (`sidescreen.go:723`).
- Kill: `killAction` (`lifeform.go:1608`) goes through the generic send (`model.go` ~1868, `m.sendTo` + `startRec`). Rename: `lifeform.go:769`. Park: `model.go:1686`. `sendToID` (`model.go:742`) returns the act id `"a<n>"`; ids are unique per console.
- Every host's snapshot ends in `merge()` (`lists.go:36`): home at `model.go:1023`, peers at `peers.go:285` and `:412`.
- Results: `result()` (`model.go:1281`); unclaimed ones end in `describe` and `m.say` or `m.flash`.
- Messages: `m.flash` (~245 writes, key line `view.go:607`, list foot via `sideSays` `sidescreen.go:446`), `m.say` (header toast, `model.go:2054`), `m.notice` (`notice.go:40`), and the standing `stateNotice` (`notice.go:97`, which carries "not read:").
- Tests drive the console with `listShot`, `pressOn`, `boardFake(m).acts()` (`sideacts_test.go`).

## File map

- `internal/app/sidestate.go` (change): HELD ranks last.
- `internal/app/pendingacts.go` (create): the console's pending acts: the store, the expect at send, the settle on a sample or a result.
- `internal/app/pendingacts_test.go` (create): the tests of the pending acts.
- `internal/app/siderow.go` (change): a pending row draws faint with its word.
- `internal/app/sidelist.go` (change): the cursor skips a row that is being killed.
- `internal/app/model.go` (change): the kill and park sends expect; `result()` settles a pending act first; the reconnect drops a host's pending acts.
- `internal/app/lifeform.go` (change): the rename send expects.
- `internal/app/lists.go` (change): `merge()` settles the pending acts.
- `internal/app/tell.go` (create): the one function that shows a message, the toast stack, and the standing-error mark.
- `internal/app/tell_test.go` (create): the tests of `tell.go`.
- `internal/app/notice.go`, `view.go`, `sidescreen.go` (change): draw the toasts and the mark; the key line and the list foot keep their keys.
- All `internal/app/*.go` that write `m.flash`, `m.toast` or call `notice(` (change): call `tell` instead.
- `CHANGELOG.md` (change): the [Unreleased] entry.
- `www/` (change, owner's words): the landing line for the toasts, per AGENTS.md.

## Tasks

### Task 1: HELD is the last group of the list

- **Files:** `sidestate.go`, `sidelist_test.go` (or the nearest list test file).
- **Test first:** `TestTheListEndsInHeld`: a state with one session of each kind (needs you, working, idle, parked by you). Assert the group heads in `sideList` read NEEDS YOU, WORKING, IDLE, HELD, top to bottom.
- **Run:** `go test ./internal/app -run TestTheListEndsInHeld`: before, fails (HELD before IDLE); after, passes.
- **Do:** reorder the constants to `sideNeed, sideWork, sideIdle, sideHeld` and the `head()` words with them. Grep for `< sideHeld`, `> sideIdle` and the other rank comparisons, and keep their meaning.
- **Done when:** the test passes and `go test ./internal/app/...` is green.
- **Commit:** `feat: parked sessions sit at the end of the list`

### Task 2: the console remembers each kill, rename and park it sends

- **Files:** `pendingacts.go`, `pendingacts_test.go`, `model.go`, `lifeform.go`.
- **Test first:** `TestAConfirmedKillIsPending`: `listShot` on `boardFixture`, `x y y` on `kindle-web`. Assert one pending act: kind kill, its host and pane, and the pid and start of `kindle-web`. The same for a rename through the form, and for `z` (park).
- **Run:** fails to compile (no store); after, passes.
- **Do:** a `pendingAct{id, kind, host, pane, pid, start, name, held, since, samples}` in `Model.acts` keyed by act id. `m.expect(id, kind, s)` adds one. The kill path, `lifeform.go:769` (with `sendToID`) and `model.go:1686` call it only when an id came back (a lost connection sends nothing and gets no pending act).
- **Done when:** the three cases pass.
- **Commit:** `feat: the console remembers the kill, rename or park it just sent`

### Task 3: a row being killed reads `killing`, faint, and the cursor skips it

- **Files:** `siderow.go`, `sidelist.go`, `pendingacts_test.go`.
- **Test first:** `TestAKilledRowSaysKillingAtOnce`: after `x y y`, with no new snapshot, the row's text holds `killing` in the faint style, and `chosenSide()` is not that row.
- **Run:** fails (the row is as before); after, passes.
- **Do:** `m.pendingOf(s)` finds the act by host, pane, pid and start. `sideRow` draws a pending kill faint with `killing` as its line 2. `sidePanes(true)` leaves the row out of the walk.
- **Done when:** the test passes.
- **Commit:** `feat: a killed session reads killing at once`

### Task 4: a sample confirms or reverts a pending act

- **Files:** `pendingacts.go`, `lists.go`, `pendingacts_test.go`.
- **Test first:**
  - `TestAKillGoesWithTheSampleThatLacksIt`: after `x y y`, a snapshot without `kindle-web`: no row, no pending act.
  - `TestAKillThatDidNotTakeComesBack`: three snapshots of the host that still have it: the row is back, not faint, and `tell` said `kindle-web is still running after the kill. Press x to kill it again.`
  - `TestAReusedPidIsNotKilling`: a snapshot with the same pane and pid but another start: the row is not faint, and the pending act is gone.
- **Run:** fails; after, passes.
- **Do:** `m.settle(host)` right after `merge()` where a host's state lands (`model.go` for home, `peers.go` for a peer). Each call is one sample of that host. Kill: gone, or another start → drop; still there at 3 → drop and tell the error. Rename: the session's name equals the new one → drop. Park: the hold by you matches the wanted one → drop. Rename or park still unchanged at 3 → drop and tell "the rename did not take" / "the park did not take".
- **Done when:** the three tests pass.
- **Commit:** `feat: the next samples confirm a kill, or bring the row back with why`

### Task 5: an error result reverts the pending act and says why

- **Files:** `model.go`, `pendingacts.go`, `pendingacts_test.go`.
- **Test first:** `TestAKillTheServiceRefusesComesBack`: after `x y y`, feed `proto.Result{ActID: <the id>, Err: "no such pane"}`. The row is back and `tell` said `kill failed: no such pane`. The same for rename and park.
- **Run:** fails; after, passes.
- **Do:** `result()` calls `m.settleResult(r)` first: an `Err` drops the act and tells `<kind> failed: <Err>`; a success keeps it for the sample.
- **Done when:** the tests pass.
- **Commit:** `feat: a refused kill, rename or park comes back with the service's reason`

### Task 6: a rename shows the new name, and a park shows in HELD, at once

- **Files:** `sidescreen.go` (`sideTitle`), `sidestate.go` (`sideStateOf`), `siderow.go`, `pendingacts_test.go`.
- **Test first:** `TestARenameShowsAtOnce`: the form renames `spark-docs` to `docs-2`; before any snapshot the row's title is `docs-2`, faint. `TestAParkShowsInHeldAtOnce`: `z` on a working session puts it under HELD, faint with `parking`; `z` on a parked one moves it out with `letting go`.
- **Run:** fails; after, passes.
- **Do:** `sideTitle` takes a pending rename's name. `sideStateOf` takes a pending park's wanted hold first. `sideRow` draws both faint with their word.
- **Done when:** the tests pass.
- **Commit:** `feat: a rename and a park show in the list at once`

### Task 7: a lost connection ends the pending acts of its host

- **Files:** `model.go` / `peers.go` (where `lost` turns true), `pendingacts_test.go`.
- **Test first:** `TestALostHostEndsItsPendingActs`: `x y y`, then the conn is lost. The row is not faint, and `tell` said `not confirmed: the connection to <host> dropped. Look at the list when it is back.`
- **Run:** fails; after, passes.
- **Do:** drop the host's acts where the console marks a host lost, and tell once.
- **Done when:** the test passes.
- **Commit:** `fix: a pending act never outlives its connection`

### Task 8: one function shows every message, as a toast

- **Files:** `tell.go`, `tell_test.go`, `notice.go`, `view.go`, `sidescreen.go`.
- **Test first:** `TestATellShowsAToastAndTheKeysStay`: `m.tell(tellEvent, "sent: kill api")`. The frame shows the text in the top-right toast stack over the body, and the key line still shows its keys. It goes after its life. `TestAnErrorToastStaysLonger`: an error toast lives twice an event's life.
- **Run:** fails; after, passes.
- **Do:** `m.tell(kind, text) tea.Cmd` with kinds event and error. It stores up to 3 toasts with an end time on the console clock (`m.now()`), newest on top, and returns the tick that drops them (as `noticeEnds` does). It also sets `m.flash` to the text, so a key still clears it and the existing tests still read it. The stack draws in the body's top-right corner, as wide as the text up to half the body (the full width on Pocket's narrow screen), and never over the cursor's row or the key line. `say` and `notice` become thin calls to `tell`. A question that waits for a key (the take-over, a consent) stays on the key line: it is not a toast.
- **Done when:** both tests pass and `go test ./internal/app/...` is green.
- **Commit:** `feat: the console tells you things in one corner, and the keys stay`

### Task 9: every flash, toast and notice goes through `tell`

- **Files:** every `internal/app/*.go` that writes `m.flash =` (not `""`), `m.toast =`, or calls `notice(`.
- **Test first:** `TestOneFunctionTellsThePerson` in `tell_test.go`: parse the package's non-test files with `go/ast`. Fail on any assignment to `m.flash` or `m.toast` outside `tell.go`, except `= ""`. The message names this plan and says "call m.tell".
- **Run:** fails, listing the ~245 places; after, passes.
- **Do:** rewrite each `m.flash = X` to `cmd = m.tell(kindOf, X)` (or batch it with the cmd already returned). Errors and refusals take the error kind. The clears become `m.hush()`.
- **Done when:** the guard passes and `go test ./internal/app/...` is green.
- **Commit:** `refactor: one function tells the person every message`

### Task 10: a standing error shows once, then as a `⚠` mark until it is fixed

- **Files:** `tell.go`, `notice.go`, `view.go`, `tell_test.go`.
- **Test first:** `TestAShipReadErrorIsOneToastThenAMark`: a state whose ships report has `Errors: ["blox-eng/x: HTTP 502"]`. The first frame has the error toast with its fix. After its life, the header shows `⚠ 1`, the key line shows its keys, and the list foot shows no "not read:". A state without the error: no mark. A tap on the mark, or its key, opens the text of each standing error.
- **Run:** fails; after, passes.
- **Do:** `stateNotice` returns a list of standing errors, each with its fix (gh not signed in: `run gh auth login`; a repository not read: `ships read it again in <interval>`; no service; old data). A new one tells once with the error kind. The header shows `⚠ <n>` while any holds, as a tap target. A free key (the builder picks it and adds it to the key help) opens them as a toast list.
- **Done when:** the test passes.
- **Commit:** `feat: a lasting error shows once with its fix, then waits as a mark`

### Task 11: the changelog and the landing page

- **Files:** `CHANGELOG.md`, `www/` (the news or feature line).
- **Test first:** `.github/scripts/landing-guard.sh "feat: a kill, rename or park shows at once, and the console tells you things in one corner" <body-file> origin/main` prints OK.
- **Do:** one [Unreleased] entry, headline ≤ 8 words. A `www/` line in the owner's voice, shown to the owner before commit.
- **Done when:** the guard prints OK.
- **Commit:** `docs: the changelog and the site say acts show at once`

### Task 12: review

- Load `builder:review` on the branch against `origin/main`, with the spec above. Run `go test $(go list ./... | grep -v /internal/demo$)` and `go test ./internal/demo`, and quote both.
- **Done when:** no open Critical or Important finding.

## Changes during build

- Task 4: `settle(host)` runs where a host's state lands, not inside `merge()`, and counts calls instead of comparing snapshot times. Reason: the tests' clock is fixed, so snapshot times cannot tell samples apart, and `merge()` does not know which host sent. The `since` field is gone.

- Task 8: the one function is `say(text)` for an event and `sayError(text)` for an error, in `say.go`, with the stack in `m.saying`. Reason: `tell` and `told` already name the once-only notices of `listonly.go`; `say` already is the header toast's function, so its callers stay.

- Tasks 8 and 9 land in one commit. Reason: between them, a raw `m.flash` write draws at the list foot while `say` draws a toast, so tests see two places; fixing those tests after Task 8 would redo them after Task 9.
- Task 9: a question that waits for a key (the take-over prompt with `m.held`, the tap-again prompt with `m.armed`) keeps the key line. The guard exempts those writes by name.
- `sideSays()` is what the tab says now (the newest toast, else the foot); `footSays()` draws the list foot and skips what a toast says.

- Task 9 (fresh agent): the stack keeps off an open box (consent, form) and draws in the full-screen chat too; `unsay` ends the copy check's toast; the key line draws `m.flash` only for the tap-again prompt (`askingAgain`); `flashNotice` and the demo's two-row key line are gone; `sayEnds` is stubbed in tests like `noticeEnds`.
- Found by `internal/demo`: a key, a click or a paste ends the toasts said before it (the flash's old "a key clears it"); a toast is cut only when it is too wide; the demo's refusal replaces the failure that carries it instead of showing twice.
- Placement: on a phone (under 60 columns) the stack sits at the body's foot, since the top holds the pane's title and its way back; at the foot it keeps off the last body line (the list's foot, the chat's line). When no place is clear of the chosen row, it still draws where no box is: it never hides.
- `TestReplay`'s frames (`www/demo/frames-*.json`) regenerate for the "sent" step; `www/index.html` and the SVGs keep their committed animation phase.

## Lens findings

- Engineering: `merge()` runs on every host's snapshot. Counting "3 samples" there would count other hosts' samples. Changed Task 4: count only when the act's host has a newer snapshot time.
- Engineering: 245 `flash` writes in one task is a big diff. No split, because the guard of Task 9 proves completeness in one run, and `tell` sets `m.flash` too, so the existing tests keep reading it.
- Engineering: the side memo (`m.side`) is rebuilt per message. No change, because a pending act changes only on a message (a send, a result, a snapshot), and those already rebuild it.
- Design: a toast over the list could hide the row under the cursor. Changed Task 8: the stack never covers the cursor's row or the key line.
- Design: on Pocket the corner is narrow. Changed Task 8: full width there.
- Design: a question on the key line (take over?, a consent) is not a message. Changed Task 8: it stays on the key line.
- Architecture: no change to the service, the snapshot or the wire. No change, because the pending acts are the console's own memory (ruling 2026-10-24).

## Spec check

| Spec line | Task |
|---|---|
| Kill: `killing` and faint before any sample; the cursor skips it | Task 3 |
| Gone after a sample without the session | Task 4 |
| Comes back with `kill failed: <reason>` on `Err` | Task 5 |
| Comes back with the "still running" words after 3 samples | Task 4 |
| Rename shows at once; old name and `rename failed` on `Err` | Task 6, Task 5 |
| Park shows in HELD with `parking`; `park failed` on `Err` | Task 6, Task 5 |
| The groups read NEEDS YOU, WORKING, IDLE, HELD | Task 1 |
| A reused pid with another start is not `killing` | Task 4 |
| A ship read error: one toast, then `⚠ 1`, the key line keeps its keys; the mark goes when fixed | Task 10 |
| Grep finds one function that tells the person | Task 9 |
| Anti-goal: a failed act looks like it worked | Task 4, Task 5 |
| Anti-goal: a pending row outlives the session or a reconnect | Task 4, Task 7 |
| Anti-goal: a standing error leaves the screen while it holds | Task 10 |
| Anti-goal: a toast covers the cursor's row or the key line | Task 8 |
| Surfaces: none touched | Task 12 checks the diff for it |
