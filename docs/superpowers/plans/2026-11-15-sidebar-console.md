# The sidebar console Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The console is a list of sessions by state beside one chat, so the owner answers any agent on any host with no `⏎` into the pane.

**Architecture:** New files in `internal/app` hold the list (`side*.go`) and the chat (`chat*.go`). They replace the board's body in the Sessions tab at every size. The list is built from the same sessions, holds, ships and chains the board reads. The chat reuses the live view's watch and crop code, and it watches a peer through that peer's own connection. It sends through `sendTo(host, …)`. The board's replaced parts then go, one commit each. Pocket, the docs and the replay follow.

**Tech Stack:** Go, Bubble Tea v2 (`tea.KeyPressMsg`, `tea.PasteMsg`), tmux through the service's mux, Swift (Pocket, `ios/`).

**Spec:** `docs/superpowers/specs/2026-11-14-sidebar-console-design.md`. Mockup: `design/0021-sidebar-console.html`. Where the spec and this plan differ, the spec wins, except for the calls listed under "Calls this plan makes".

## Global Constraints

- Do not change `matchblox sessions --json`, `matchblox peek` or `matchblox send`. `panes.CheckAnswer` (`internal/panes/panes.go:128`) stays as it is, because `send` uses it.
- Fixtures, goldens and docs use only the demo fleet's names from DESIGN.md §7: host `ws-1`, sessions hearth-api, flint-auth, spark-docs, kindle-web, ember-billing, cinder-ci, wick-mobile. (The mockup's `flint-ui` and `gpu-1` are not in §7: use `wick-mobile` for the third chain member. Use the second host that the demo already seeds, or else `ws-2`.)
- Every key is a binding in `keys.go`. `keyguard_test.go` refuses key literals in code and key glyphs in render code. `keys_test.go:99` refuses collisions.
- `internal/app/docs_test.go:67`: every bound key is in a Key table in the docs, and every key the docs name is bound. So a task that adds or removes a key changes `docs/keys.md` (and any other doc that names that key) in the same commit.
- `internal/arch_test.go:46`: `internal/app` must not import procfs, host, gitscan, ships, actions, panes, hooks, setup, stoker, service or clone. New tmux work goes in the service, through the mux.
- Words on the screen and in the docs are ASD-STE100: one idea per sentence, active voice, present tense.
- An empty group draws no head and no row. A key or a tap never does nothing silently.
- Commits are signed Conventional Commits (`SSH_AUTH_SOCK=$HOME/.ssh/agent.sock`), with no `--no-verify` and no WIP commits. Allowed types: feat, fix, chore, test, docs, refactor, ci, perf, build, revert. Before each commit, `git diff --cached | rg -i -f ~/.config/builder/denylist.txt` prints nothing. `go test ./...` passes before every commit that touches Go.
- Each removal is its own `refactor(app): remove …` commit, and it takes its code, tests, goldens, keys and doc lines with it.
- Goldens: `go test ./internal/app -run TestScreens -update` rewrites `testdata/screens`. Read every changed golden before you commit it.
- Commit trailer: `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Calls this plan makes

The code map found nine places where the spec does not fit the code as it is. These are my calls. The spec gets a "Calls made in the plan" section in Task 0.

1. **A peer's chat.** Today the watch is for the home host only (`liveview.go:155`). Each peer has its own `conn` (`m.peers[i].conn`). The chat sends the same `proto.WatchPrefix` act on the conn of the chosen session's host. No new service relay.
2. **A multi-line paste.** The `message:` act refuses control characters and text over 500 runes. A new act, `paste:<pane>`, carries the line when it has a newline or is longer than that. The service puts it in through `tmux load-buffer` and `paste-buffer -p` (bracketed paste), then presses Enter. The cap is 16384 bytes. ESC and the other C0 characters, except `\n` and `\t`, are taken out. The same `messageable` guard applies. `send` does not change.
3. **A message to a working agent.** The console holds the message and sends it when the session's state leaves "works", for each agent that does not take typed input while busy. The start table is in Task 7. The builder checks each agent by hand and records the result in the table's comment.
4. **A hidden host and `s`.** A hidden host has no peer link today, so `s` cannot reach it. Hidden hosts now get a link too. "All hosts", the counts and the list leave them out, and nothing more.
5. **`⌃o` back.** Claude Code uses `⌃o` for itself, so a tmux root binding on `⌃o` would take it from the agent. `⌃o` expands. The way back stays `ctrl+a m`, as today, and the key line says so.
6. **`h`.** `h` is "fold recs" today. The tips move to the foot, so Fold goes, and `h` unhides a hidden host.
7. **`n`.** `n` is "stop night" today. A new session stays `N`. The foot says `+ new`, with `N`.
8. **The key that hides the list:** `ctrl+l`. It is not tmux's prefix (`ctrl+b`), and it is not matchblox's (`ctrl+a`).
9. **The PR badge** shows `#<Number>` and the ship's stage word. `ships.Ship` has no CI count, so there is no `CI 6/9`. **`step N of M`**: N is the place of the chain's most urgent member in team order, and M is the member count.

## Review Focus

1. **The list moves while the owner types.** A row changes state, and the list reorders under a half-typed line. The message must still go to the session named in the line. The draft is keyed by `paneKey`, not by row index. Test in Task 6: `TestLineKeepsItsSessionWhenTheListMoves`.
2. **A paste with escape sequences.** A paste holds `\x1b[201~` or `\x1b]52;…`. It must not reach the agent's pane as keys. Test in Task 8 (service): `TestPasteTakesOutControlCharacters`.
3. **Small screens.** At 80x24 and 60x20 no line is wider than the screen, the chat line is visible, and the list does not cover the chat. Goldens in Task 10: `side-80x24`, `side-60x20`.
4. **The chosen session ends.** The choice moves to the next session in the drawn order. A kept draft says `flint-auth ended · the text is not sent`. Test in Task 6: `TestChosenSessionEndsKeepsTheDraft`.
5. **A question comes while the owner types a digit.** The board's 400 ms guard (`peekDead`) carries over, so a digit typed just before a question opens does not mark an option. Test in Task 6: `TestDigitJustBeforeAQuestionTypes`.

---

### Task 0: The spec takes the plan's calls

**Files:**
- Modify: `docs/superpowers/specs/2026-11-14-sidebar-console-design.md`

- [ ] **Step 1:** Add a section `## Calls made in the plan (2026-11-15)` with calls 1–9 above, one line each. In "Where the keys go", change the way back from "`⌃o` again comes back" to "`ctrl+a m` comes back (call 5)". In "What else changes", change the www line to the owner's answer on the replay frames (see the handover question).
- [ ] **Step 2:** Commit `docs: the sidebar console's spec takes the plan's calls (#411)`.

### Task 1: Each session's state in the list

**Files:**
- Create: `internal/app/sidestate.go`, `internal/app/sidestate_test.go`

**Interfaces:**
- Produces: `type sideState int` with `sideNeed, sideWork, sideHeld, sideIdle` (in this order, so the order is the rank); `func (m Model) sideStateOf(s sample.Session) sideState`; `func (h sideState) head() string` (`NEEDS YOU`, `WORKING`, `HELD`, `IDLE`); `func (m Model) heldWhy(s sample.Session) string`.

- [ ] **Step 1: Write the failing tests.** `TestSideStateOf`, table driven with demo names:
  - flint-auth asks now → `sideNeed`
  - hearth-api works → `sideWork`
  - kindle-web parked by you → `sideHeld`, and `heldWhy` = `parked by you · z lets it go`
  - spark-docs held by the stoker → `sideHeld`, and `heldWhy` = `held by the stoker · not the priority` (the hold's `Why` when it has one)
  - cinder-ci cold or stopped → `sideIdle`
  - ember-billing with a PR that waits for the owner's merge (`mergeable`, boardcard.go:454) → `sideNeed`
  - wick-mobile with a PR whose CI runs, and an idle agent → `sideIdle`, with its badge
- [ ] **Step 2:** Run `go test ./internal/app -run TestSideStateOf`. Expected: FAIL, `sideStateOf` undefined.
- [ ] **Step 3: Implement.** Follow the precedence of `columnOf` (boardcols.go:41), with two changes: any hold (`holdOf`, sessions.go:74) gives `sideHeld` before work, and "ship running" gives the agent's own state, not a group of its own.
- [ ] **Step 4:** Run the test. Expected: PASS.
- [ ] **Step 5:** Commit `feat(app): each session has one of four states for the list (#411)`.

### Task 2: Groups and chains as units

**Files:**
- Create: `internal/app/sidelist.go`, `internal/app/sidelist_test.go`

**Interfaces:**
- Consumes: `sideStateOf`, `chainKeyOf` (fuse.go:238), `members(k)` (room.go:111, team order).
- Produces:
  - `type sideUnit struct { chain chainKey; members []sample.Session }` (a zero `chain` means a session in no chain, with one member)
  - `type sideGroup struct { state sideState; count int; units []sideUnit }`
  - `func (m Model) sideGroups() []sideGroup`
  - `func (m Model) sideOrder() []paneKey`, the order ↑↓ walk, as drawn
  - `func (u sideUnit) urgent() sample.Session`
  - `func (u sideUnit) step() (n, of int)`

- [ ] **Step 1: Write the failing tests.**
  - `TestEmptyGroupHasNoEntry`: with no session that needs you, `sideGroups()` has no `sideNeed` entry.
  - `TestChainSitsWithItsMostUrgentMember`: login = kindle-web (idle), flint-auth (needs you), wick-mobile (works). Its unit is in `sideNeed`. Its members keep team order. `count` of `sideNeed` is 1, `sideWork` does not count wick-mobile, and `step()` = (2, 3).
  - `TestChainMovesWhenItsWaitIsAnswered`: flint-auth works → the unit is in `sideWork`.
  - `TestOrderInAGroupHoldsWhileStatesHold`: two refreshes with the same states give the same `sideOrder()` (the board's fixed-order rule; reuse the order key that `boardIndex` used).
  - `TestChainTakesItsUrgentMembersPlace`: in its group, the unit sorts where `urgent()` would sort alone.
- [ ] **Step 2:** Run them. Expected: FAIL.
- [ ] **Step 3: Implement.** Hosts that are hidden are left out unless the shown host is that hidden host (Task 9 sets that).
- [ ] **Step 4:** Run them. Expected: PASS.
- [ ] **Step 5:** Commit `feat(app): the list groups sessions by state and keeps a chain as one unit (#411)`.

### Task 3: Rows, matches and links

**Files:**
- Modify: `internal/app/match.go` (add the held and idle matches)
- Create: `internal/app/siderow.go`, `internal/app/siderow_test.go`

**Interfaces:**
- Produces:
  - `func (m Model) sideRow(s sample.Session, w int, chosen bool, link string) []string`: one line, plus a second line only when the session needs you or is chosen
  - `func (m Model) chainHead(u sideUnit, w int, lit bool) string`: `≋ login   step 2 of 3 · 3`
  - `func linkOf(i, n int, twoLines bool) (first, rest string)`: `├`, `└`, `│`
  - `func sideMatch(st sideState) string`: struck red, burning, `╽`, `╵`, from the tokens

- [ ] **Step 1: Write the failing tests.**
  - `TestRowIsOneLineUnlessItNeedsYou`
  - `TestChosenRowHasASecondLine`
  - `TestRowEndsWithTheAgentsMark`: ✻ ❋ ⬢ from `agentMarks`
  - `TestRowShowsAnotherHost`: `flint-auth@ws-2` only when all hosts show
  - `TestBadgeIsNumberAndStage`
  - `TestLinksLightOnlyWhenTheChosenIsInTheChain`: compare the styled output
  - `TestRowFitsItsWidth`: widths 24..60, `ansi.StringWidth` ≤ w
- [ ] **Step 2:** Run them. Expected: FAIL.
- [ ] **Step 3: Implement.** Use the styles in match.go. Glyphs come from tokens, not literals in render code (keyguard).
- [ ] **Step 4:** Run them. Expected: PASS.
- [ ] **Step 5:** Commit `feat(app): a list row shows the state as its match and the agent at its end (#411)`.

### Task 4: The list panel, its foot, and the header

**Files:**
- Create: `internal/app/sidepanel.go`, `internal/app/sidepanel_test.go`
- Modify: `internal/app/view.go:299` (`header`)

**Interfaces:**
- Produces: `func (m Model) sidePanel(w, h int) []string` (heads `NEEDS YOU · 4  −`, a thin line between groups, units, then the foot `+ new · ✧ 2 tips · ! 1 setup`; the setup part only while a step is open). The list scrolls to keep the chosen row in view.
- Header: `● N need you` counts the hosts in view, and at zero it says `✓ nothing needs you`.

- [ ] **Step 1: Write the failing tests.**
  - `TestNothingNeedsYouTakesNoRow`: no `NEEDS YOU` in the panel, and the header has `✓ nothing needs you`.
  - `TestFootHidesSetupWhenNoStepIsOpen`
  - `TestPanelKeepsTheChosenRowInView`: 25 sessions, h=12, choose the last one.
  - `TestStatesWordsShow`: loading → `reading your matches…`; a search with no match → `Nothing matches "x". esc clears the search.`
- [ ] **Step 2:** Run them. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4:** Run them. Expected: PASS.
- [ ] **Step 5:** Commit `feat(app): the list panel draws its groups and one foot line (#411)`.

### Task 5: The chat panel and the watch on any host

**Files:**
- Create: `internal/app/chat.go`, `internal/app/chat_test.go`
- Modify: `internal/app/liveview.go:147` (`watchCmd`): it watches the chosen session on its host's conn, the home conn or `m.peers[i].conn`

**Interfaces:**
- Consumes: `fitScreen`/`cropScreen` (liveview.go:70/:122), `screenMsg`, the room's chain data for the chain line.
- Produces:
  - `func (m Model) chatPanel(w, h int) []string`
  - The head line: name, `@host`, agent, state, badge, `ctx N%`, then the keys for expand and menu (from bindings).
  - The chain line, only for a session in a chain: `chain login · step 2 of 3   kindle-web done → flint-auth needs you → wick-mobile works`.
  - The conversation, cut. Per agent the cut is the builder's call: drop the agent's own input box and status lines at the foot.
  - The question and its options.
  - The line: `› …   ⏎ send to flint-auth`.
  - `func (m Model) watchConn(host string) transport.Conn`

- [ ] **Step 1: Write the failing tests.**
  - `TestChatWatchesAPeersPane`: a fake peer conn gets the `WatchPrefix` act for a ws-2 session, and the home conn gets nothing.
  - `TestChatHasNoChainLineOutsideAChain`
  - `TestChatCutsTheAgentsInputBox`: one fixture screen each for Claude Code, Codex and OpenCode in `testdata/chat/`
  - `TestNoSessionsSaysHowToStart`: `No sessions yet. + new session starts one here (N). matchblox setup finds the agents you already run.`
- [ ] **Step 2:** Run them. Expected: FAIL.
- [ ] **Step 3: Implement.** A screen of another pane never stays. Keep that rule from `watchCmd`.
- [ ] **Step 4:** Run them. Expected: PASS.
- [ ] **Step 5:** Commit `feat(app): the chat shows the chosen session's pane on any host (#411)`.

### Task 6: Where the keys go, and the line

**Files:**
- Create: `internal/app/sidekeys.go`, `internal/app/sidekeys_test.go`
- Modify: `internal/app/model.go` (fields; `tea.PasteMsg` at :1151 goes to the line when it has the keys), `internal/app/keys.go` (`side` keymap), `docs/keys.md`
- Rename: the peek's guard in boardkeys.go:62–188 (`startPeek`, `peekOn`, `peekMarks`, `answerFromPeek`) to `chatMark…`/`answerFromChat`, and lift the peer refusal at :161 (answers go through `sendTo`)

**Interfaces:**
- Produces:
  - `type sideFocus int` with `focusLine, focusList`
  - Model fields `focus sideFocus`, `sideAt paneKey`
  - The existing `drafts` (model.go:578), keyed by `paneKey`
  - `func (m Model) sideKey(k tea.KeyPressMsg) (Model, tea.Cmd, bool)`
  - `func (m *Model) sendLine() tea.Cmd`
  - Key line words: `writing to flint-auth` or `in the list`, then at most five keys

- [ ] **Step 1: Write the failing tests.**
  - `TestLettersTypeIntoTheLine`
  - `TestEnterSendsTheLineToItsSession`: a `message:<pane>` act on that host's conn
  - `TestPasteArrivesAsOnePiece`
  - `TestDigitMarksThenSends`: `2` marks, then `2` sends `lit:2`. This is done-when line 1, at 160 columns.
  - `TestDigitWithNoQuestionTypes`
  - `TestDigitJustBeforeAQuestionTypes` (Review Focus 5)
  - `TestArrowsChooseInDrawnOrder`
  - `TestEscClearsAMarkThenMovesToTheList`
  - `TestListKeys`: s, g, /, m, c, z, N, tab act; another letter goes back to the line and is typed there
  - `TestLineKeepsItsSessionWhenTheListMoves` (Review Focus 1)
  - `TestChosenSessionEndsKeepsTheDraft` (Review Focus 4)
  - `TestParkedLineIsOff`: `parked: z lets it go`
  - `TestHeldByStokerOneReleases`
  - `TestUnreachedHostKeepsTheText`: `not sent: ws-2 is not reached`, no retry
  - `TestNoServiceSendsNothing`
- [ ] **Step 2:** Run them. Expected: FAIL.
- [ ] **Step 3: Implement.** Add the new bindings to `docs/keys.md`.
- [ ] **Step 4:** Run `go test ./internal/app`. Expected: PASS, docs_test included.
- [ ] **Step 5:** Commit `feat(app): the line has the keys, and esc gives them to the list (#411)`.

### Task 7: A message to a busy agent

**Files:**
- Modify: `internal/app/sidekeys.go`, `internal/app/sidekeys_test.go`

**Interfaces:**
- Produces: `var typesWhileBusy = map[string]bool{"claude": true, "codex": true, "opencode": false}`. The comment records how each value was checked by hand. Also `held map[paneKey]string`. The chat says `queued for its next turn`.

- [ ] **Step 1: Write the failing test.** `TestBusyAgentGetsTheMessageWhenItsTurnEnds`: OpenCode works → nothing is sent. A refresh moves it to idle or needs you → the `message:` act goes.
- [ ] **Step 2:** Run it. Expected: FAIL.
- [ ] **Step 3: Implement.** Check each agent by hand in a scratch tmux, and record the result in the comment.
- [ ] **Step 4:** Run it. Expected: PASS.
- [ ] **Step 5:** Commit `feat(app): a message to a busy agent waits for its next turn (#411)`.

### Task 8: The paste act

**Files:**
- Modify: `internal/proto/proto.go` (`PastePrefix = "paste:"`, `MaxPaste = 16384`), the service action next to `message` (`internal/service/actions.go:320/463`), and its tests
- Modify: `internal/app/sidekeys.go` (`sendLine` picks `paste:` when the text has a newline or is over 500 runes)

- [ ] **Step 1: Write the failing tests.**
  - `TestPasteGoesInAsBracketedPaste`: the fake mux sees `load-buffer`, then `paste-buffer -p -d`, then Enter.
  - `TestPasteTakesOutControlCharacters` (Review Focus 2)
  - `TestPasteOverTheCapIsRefused`: the words say the size and the cap.
  - `TestPasteKeepsTheMessageGuard`: held, parked and asking sessions are refused with today's words.
  - `TestSendCommandIsUnchanged`: the `send` CLI output for a multi-line text is the same as before.
- [ ] **Step 2:** Run them. Expected: FAIL.
- [ ] **Step 3: Implement.** Go through the mux (arch_test).
- [ ] **Step 4:** Run `go test ./...`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(service): a pasted text reaches an agent as one bracketed paste (#411)`.

### Task 9: Hosts

**Files:**
- Modify: `internal/app/boardkeys.go:430` (`turnHost` → move to `sidehosts.go`), `internal/app/peers.go:93`, `internal/app/layers.go:113–134`, `cmd/matchblox/main.go:702–712` (hidden hosts also get a link), `internal/app/keys.go` (`Unhide` = `h`; remove `Fold`), `docs/keys.md`, `docs/remote.md`
- Create: `internal/app/sidehosts_test.go`

- [ ] **Step 1: Write the failing tests.**
  - `TestAllHostsLeavesOutHidden`: ws-2 is hidden, so all hosts shows ws-1 only, and the count is for ws-1 only.
  - `TestSReachesAHiddenHost`: the header says `ws-2 · hidden from all hosts · h shows it`.
  - `TestHUnhidesAndAllHostsShowsIt`: `hosts.Show` is called.
  - `TestOneUnhiddenHostSStepsHostToHost`: no second stop on the same screen.
  - `TestSIsNeverSilent`: with one host and nothing hidden, `s` gives a notice that says so.
  - `TestUnreachedHostDimsItsRows`: `ws-2 not reached · since 2m`.
- [ ] **Step 2:** Run them. Expected: FAIL.
- [ ] **Step 3: Implement.** Machine › Hosts still ticks and unticks.
- [ ] **Step 4:** Run `go test ./...`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(app): s reaches every host, and a hidden host is only left out of all hosts (#411)`.

### Task 10: The screen at every size

**Files:**
- Modify: `internal/app/tabs.go:153` (the Sessions body is the list and the chat), `internal/app/view.go` (`keyLine`), `internal/app/mouse.go` (taps), `internal/app/keys.go` (`HideList` = `ctrl+l`; `Jump` `ctrl+o` = expand through `intoSelected`), `docs/keys.md`
- Create: `internal/app/sidescreen_test.go`, goldens `testdata/screens/side-*`

- [ ] **Step 1: Write the failing tests and goldens.**
  - Goldens `side-160x38`, `side-132x32`, `side-104x38`, `side-80x24`, `side-60x20`, `side-44x31` (phone list), `side-44x31-chat` (phone chat), `side-hidden-132x32` (list hidden, the header names each wait)
  - `TestTapOnARowOpensItsChat`
  - `TestPhoneBackReturnsToTheList`: `◂`
  - `TestTapOnAHeaderNameOpensItsChat`: list hidden
  - `TestCtrlOExpandsAndTheKeyLineSaysTheWayBack`
  - `TestDragInTheChatSelectsChatTextOnly`: the copied text has no list column in it
  - `TestTapOnAGroupsFoldMarkFoldsIt`: `−` folds the group, and the head keeps its count
- [ ] **Step 2:** Run them. Expected: FAIL.
- [ ] **Step 3: Implement.** The list is 40 columns at Wide. The builder picks it for Medium, and when the list starts hidden. Narrow and portrait sizes show the list or the chat, full width.
- [ ] **Step 4:** Run `go test ./internal/app -run TestScreens -update`, read each new golden, then run `go test ./...`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(app): the Sessions tab is a list beside one chat at every size (#411)`.

### Tasks 11–18: The removals, one commit each

Each task takes the listed code, its tests, its goldens, its key bindings and the doc lines that name it. Each one ends with `go test ./...` passing and its own commit. Run them in this order, because later ones lean on earlier ones:

| Task | Commit | What goes (from the code map) |
|---|---|---|
| 11 | `refactor(app): remove the strip and u` | strip.go:104–402, listonly.go:110–137, `boardFold`, `board.Strip`, strip_test (strip.go:22–100, the notices, stays) |
| 12 | `refactor(app): remove the one-line view and v` | `oneLine*`/`flipOneLine` listonly.go:82–103, `board.Density`, boardoneline-* goldens |
| 13 | `refactor(app): remove the live view` | liveview.go except `watchCmd`, `fitScreen`, `cropScreen`, `sendKey*` (move these to chat.go); `viewPane`, `viewStrip`, `boardLive`, `viewScreen`, `overBoardView`; liveview_test, viewcrop_test, viewwheel_test, live_bench_test; everyscreen cases |
| 14 | `refactor(app): remove the peek` | `peekHead` boardcard.go:111, and the peek fields model.go:548–562 not used by the renamed guard; boardpeek_test |
| 15 | `refactor(app): remove ←→ between columns` | `boardSide`, `boardOpen`, `boardShowCol`, `board.Left/Right` |
| 16 | `refactor(app): remove hiding a column` | `colHidden`/`flipHidden` listonly.go:60–68, `boardCrumb` "+ name", boardhidden_test, boardhidden-* goldens |
| 17 | `refactor(app): remove the board's columns and SHIPPING` | boardcols.go, board.go column code, `boardAt`/`boardPick`, `match.go` `change.from boardCol`, `copy.go:268/341–359` (moved to the list), boardcols/board/boardships/boardshort/boardport tests, `board*` goldens |
| 18 | `refactor(app): remove ⏎ and a into the pane` | `boardInto` and the `Into` binding (`a`, `right`); `Open` in the list goes back to the line; `ctrl+o` stays |

- [ ] For each: delete, run `go vet ./... && go test ./...`, grep `rg -n '<removed name>' internal cmd docs README.md` → nothing, then commit.

### Task 19: Pocket

**Files:**
- Modify: `ios/MatchbloxKit/Sources/MatchbloxKit/BoardBelt.swift` (tail :31, faces :125–145): `◂`, the chosen session's `1 2 3`, `↑ ↓ ⏎`, then `⌃o`, `s`, `g`, `/`, `m`, `c`, `z`, `N`. The `v`, `u`, `←`, `→`, `]`, `[` keys go.
- Modify: `ios/.../BoardTouch.swift` (a sideways swipe no longer sends ←/→; the edge swipe sends `◂`), `ios/Matchblox/ConsoleTerminalView.swift:90–192`
- Modify: `internal/app/boardkeys.go:1054–1160` (`beltKeys`, `cardBelt`, `beltValue`), `internal/keycmd/keycmd.go:57`
- Test: `ios/MatchbloxKit/Tests/*/BoardBeltTests.swift`, `BoardTouchTests.swift`, the Go belt tests
- Modify: the App Store text in `ios/` (`fastlane` or its metadata folder, wherever it lives)

- [ ] **Step 1:** Write the failing tests `testBeltHasTheSidebarKeys`, `testSidewaysSwipeTurnsNoPage`, `testEdgeSwipeGoesBack`, and the Go `TestBeltFollowsTheFocus` (the line's belt and the list's belt).
- [ ] **Step 2:** Run `swift test --package-path ios/MatchbloxKit` and `go test ./internal/app -run Belt`. Expected: FAIL.
- [ ] **Step 3: Implement.** The soft keyboard already types into the console, so it types into the line. An iOS paste reaches the console as a bracketed paste, so it lands as one `tea.PasteMsg`. Check both by hand on the simulator.
- [ ] **Step 4:** Run both suites, and `go test ./internal/app -run TestTokens`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(pocket): the belt and the touches follow the sidebar console (#411)`.

### Task 20: README, docs and CHANGELOG

**Files:**
- Modify: README.md, docs/console.md, docs/keys.md, docs/phone.md, docs/start.md, docs/remote.md, docs/index.md, docs/agents.md, docs/stoker.md, docs/ships.md, docs/limits.md, docs/config.md, CHANGELOG.md (Unreleased)

- [ ] **Step 1:** Rewrite each part that names the board, its columns, `SHIPPING`, the peek (not the `matchblox peek` command) or the live view, in ASD-STE100. Show screens with demo names.
- [ ] **Step 2:** Run `rg -n -i 'shipping|live view|column|the board' README.md docs --glob '!docs/superpowers/**'`. Expected: only lines about other things (read each one).
- [ ] **Step 3:** Run `go test ./internal/app -run Docs ./cmd/matchblox -run Docs`. Expected: PASS.
- [ ] **Step 4:** Commit `docs: the README and the docs show the sidebar console (#411)`.

### Task 21: The replay

**Files:**
- Modify: `cmd/matchblox/replay_test.go:637–679` (the steps: choose flint-auth, answer `2 2` in the chat, move through the list, open the login chain with `g`; no columns, no "+ work")
- Generated by `-update`: `www/demo/frames-{desk,duo,phone}.json`, `www/demo/replay-*.svg`, the still frame in `www/index.html`. **Waits on the owner's answer to the handover question.**

- [ ] **Step 1:** Change the steps.
- [ ] **Step 2:** Run `go test ./cmd/matchblox -run TestReplay -update`, then `go test ./cmd/matchblox -run TestReplay -count=3`. Expected: PASS three times. Watch for the macOS strip flake, which goes away with Task 11.
- [ ] **Step 3:** Commit `test: the replay records the sidebar console (#411)`.

### Task 22: The PR, and the landing brief

- [ ] **Step 1:** Write the PR body to the scratchpad: what the owner can now do, done-when lines with how each one was checked, `Landing page: unchanged because the page's words wait for the owner's approval of the built console (#411)`, and the 🤖 line. Run `.github/scripts/landing-guard.sh "<title>" <body-file> origin/main`. Expected: pass.
- [ ] **Step 2:** Write the brief to matchbloxrelease in the scratchpad: the two lines from the spec, the stills from the mockup's landing section, and the new replay frames. **Send it only after the owner approves the built console.**
- [ ] **Step 3:** Show the owner the PR body. Push and open the PR only after their yes.
