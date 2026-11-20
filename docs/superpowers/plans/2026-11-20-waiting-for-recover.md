Spec: https://github.com/blox-eng/matchblox/issues/444 (mockup: https://claude.ai/artifact/DgkavtoGzdTA4D9cg8rFWQ)
Spec approved: 2026-11-20
Test command: SSH_AUTH_SOCK=~/.ssh/agent.sock go test ./...
Plan approved: 2026-11-20 by owner
Next: builder:build

# A match that waits for recover — Implementation Plan

**Goal:** After a lost tmux server, the owner sees each chain member that waits for `matchblox recover` in its team, in its own color, and can reopen it or take it out. A mate that sends to it hears that the line waits.

**Builds on:** PR #440 (branch `fix/391-chains-survive-reboot`). The service already keeps a waiting member in its teams and knows its name (`waitNames`).

## Rulings

- 2026-11-20: spec approved (issue #444 and the mockup). Decided by: owner.
- 2026-11-20: waiting matches stay under their team's head, in their own color. Decided by: owner.
- 2026-11-20: `⏎` on a waiting row reopens that one match (default; the owner did not pick the sheet).

## Global constraints

- Signed Conventional Commits, never `--no-verify`, no WIP commit. Before each commit: `git diff --cached | rg -v -i 'blox-eng' | rg -i -f ~/.config/builder/denylist.txt` prints nothing.
- `go vet ./...`, `gofmt -l .` (empty) and the test command pass before every commit that touches Go.
- Fixtures and docs use the demo fleet's names: `flint-auth`, `kindle-web`, `hearth-api`, `wick-mobile`.
- Docs and comments in ASD-STE100: one idea per sentence, active voice, present tense.
- Do not change `matchblox sessions --json` or `peek`.

## File map

- `internal/state/state.go` (change) — `MateView.Wait`: the facts of a member that waits for recover.
- `internal/service/chains.go` (change) — keep each waiting member's recovery entry; fill `MateView.Wait`; take out by session id.
- `internal/service/actions.go` / `peersend.go` (change) — `message:sid:<id>` puts a line in a waiting member's inbox and says so.
- `internal/service/chains_recover_test.go` (change) — the service tests of the tasks below.
- `cmd/matchblox/cli.go` (change) — `send <name>` resolves a waiting mate by its name and sends by session id.
- `cmd/matchblox/cli_test.go` (change) — the CLI test.
- `internal/app/theme.go` (change) — the `wait` color, one value for dark and light and night.
- `internal/app/sidelist.go` (change) — a chain unit carries its waiting members; a team of only waiting members still draws.
- `internal/app/siderow.go` (change) — `waitRow`: the row and the chosen second line of a waiting member; the head's `1 + 2 wait`.
- `internal/app/sidepanel.go` (change) — the foot part `↻ N to reopen`; a waiting row is choosable (key `wait:<sid>`).
- `internal/app/boardmenu.go` (change) — the menu of a waiting row.
- `internal/app/sidekeys.go` (change) — `⏎` on a waiting row reopens it.
- `internal/app/chat.go` (change) — the chat's place for a chosen waiting row, where the chat is drawn.
- `internal/app/model.go` (change) — show a recover result as a notice.
- `internal/app/*_test.go` (change) — the app tests of the tasks below.
- `docs/console.md`, `docs/keys.md`, `docs/agents.md` (change) — say it.
- `www/index.html` (change) — the landing page line (AGENTS.md: a feat PR changes `www/`).
- `CHANGELOG.md` (change) — the `[Unreleased]` entry.

## Tasks

### Task 1: the doc names each waiting member

- **Files:** `internal/state/state.go`, `internal/service/chains.go`, `internal/service/chains_recover_test.go`.
- **Test first:** `TestTheDocShowsAWaitingMate`: `rebootPair`, `loseServer`, then the doc's `Chains.Mates` entry of `two` has `Wait.Name == "two"`, `Wait.Agent == "claude"`, `Wait.Conversation == "s2"`, `Wait.Forget == Wait.Since + 14 days`. The entry of a live mate has `Wait == nil`.
- **Run:** `go test ./internal/service/ -run TestTheDocShowsAWaitingMate` — fails to build (no `Wait`), then passes.
- **Do:** add `type WaitView struct { Name, Agent, Conversation string; Since, Forget time.Time }` and `Wait *WaitView \`json:"wait,omitempty"\`` to `MateView`. `waitingLocked` returns the recovery entries by conversation; `pruneChainsLocked` keeps them in `s.waiting` (name as `waitNames` picks it). `chainsView` takes the waiting map and fills `Wait` (`Since = e.Seen`, `Forget = e.Seen + recovery.ExpireAfter`).
- **Done when:** the test passes, and every older chain test passes.
- **Commit:** `feat(service): the doc names each chain member that waits for recover (#444)`

### Task 2: out of team by session id

- **Files:** `internal/service/chains.go`, `internal/service/chains_recover_test.go`.
- **Test first:** `TestTakeOutAWaitingMateBySessionID`: three mates (`one`, `two`, `three`), lose the server, reopen `one` and `three`; act `chain:out:sid:s2@one` from the console. Result `took two out of one`; `two` is in no team; the team still has `one` and `three`. A non-console act gets the existing refusal.
- **Run:** fails (`no match in sid:s2 is in one`), then passes.
- **Do:** in `takeOutLocked`, a target `sid:<id>` matches the member whose `SessionID` is `<id>`; a pane target works as today.
- **Done when:** the test passes.
- **Commit:** `feat(service): the owner takes a waiting match out of its team (#444)`

### Task 3: a line to a waiting mate waits, and the sender hears it

- **Files:** `internal/service/peersend.go`, `internal/service/actions.go`, `internal/service/chains_recover_test.go`.
- **Test first:** `TestALineToAWaitingMateWaits`: lose the server, reopen `one`; act `message:sid:s2` from `one`'s pane with text `the fix is on main`. Result note `two waits for recover: your line waits in its inbox (1 now)`. Reopen `two` (same session id): the line is in `two`'s inbox (`chainInbox`). A send from a match not chained with `two` is refused: `you are not chained with two`.
- **Hostile input:** the same act with text holding a newline is refused by `checkPeerText`, as for a live mate.
- **Run:** fails (unknown target), then passes.
- **Do:** in the `message:` act, a target `sid:<id>` that names a waiting member: check the text, check the sender shares a team, `PostLine(member, sender, text)`, return the note with `InboxCount`. A `sid:` target that waits for nothing: `no match waits for recover as <id>`.
- **Done when:** the test passes.
- **Commit:** `feat(service): a line to a match that waits for recover waits in its inbox (#444)`

### Task 4: `matchblox send` reaches a waiting mate by its name

- **Files:** `cmd/matchblox/cli.go`, `cmd/matchblox/cli_test.go`.
- **Test first:** `TestSendResolvesAWaitingMate`: a doc with no session named `hearth-api` and a mate with `Wait.Name == "hearth-api"`, conversation `c-3`: `resolve` gives the act `message:sid:c-3`. A name that is neither keeps `no session named nope`.
- **Run:** fails, then passes.
- **Do:** `resolve` falls back to `doc.Chains.Mates` with `Wait.Name == who`; the hand act uses `sid:<conversation>`; `handOutcome` prints the service's note.
- **Done when:** the test and `cli_test.go:134`'s test pass.
- **Commit:** `feat(cli): send reaches a match that waits for recover (#444)`

### Task 5: the list draws waiting members in their team, in their color

- **Files:** `internal/app/theme.go`, `internal/app/sidelist.go`, `internal/app/siderow.go`, `internal/app/sidepanel.go`, `internal/app/siderow_test.go`, `internal/app/sidepanel_test.go`.
- **Test first:** `TestAWaitingMateDrawsInItsTeam`: `loginChain` with one member's session dropped and its `MateView.Wait` set (`Name: "kindle-web"`). `sidePanel(60,20)` holds `◌ kindle-web` and `waits for recover` under `≋ login`, with the last link `└`; the head reads `· 2 + 1 wait`. The waiting line carries the `wait` color (assert the style, as `TestChainLinksLightWhenAMemberIsChosen` does). `TestATeamOfWaitingMatesStillDraws`: every member waits; the head and its rows still draw, in the IDLE group. `TestNoWaitingMateNoChange`: a doc with no `Wait` renders byte for byte as before.
- **Run:** fails (no row), then passes.
- **Do:** add `wait` to `styles` (lavender `#a99cf2` dark, `#6b5bd6` light; night uses its own ink). `sideUnit` gets `waiting []state.MateView`; `buildSideGroups` fills it from `members(k)`'s unseen that have `Wait`, and keeps a unit whose members all wait. `sideRow` draws `waitRow` after the live members. `chainHead` counts `n + m wait`.
- **Done when:** the three tests pass, and every existing `internal/app` test passes.
- **Commit:** `feat(app): a match that waits for recover shows in its team (#444)`

### Task 6: a waiting row is choosable; its second line and the chat's place

- **Files:** `internal/app/sidepanel.go`, `internal/app/sidelist.go`, `internal/app/sidekeys.go`, `internal/app/chat.go`, `internal/app/sidepanel_test.go`.
- **Test first:** `TestAWaitingRowIsChosen`: `↓` from the last live member chooses the waiting row; its second line reads `its server stopped 22:14 · ⏎ reopen · m menu`. `TestTheChatShowsAWaitingMate` (no tmux): the chat's place holds `waits for recover`, `claude c-3`, the forget date, `⏎ reopen it` and `m out of login`.
- **Run:** fails, then passes.
- **Do:** `buildSidePanes` lists `wait:<conversation>` keys after a unit's live members; `sideChosen` returns the waiting mate for such a key; `chatPanel` gets a branch for it. On a desk the console draws no chat: the second line carries the facts.
- **Done when:** both tests pass.
- **Commit:** `feat(app): choose a waiting match and see why it waits (#444)`

### Task 7: `⏎` reopens; the menu takes it out

- **Files:** `internal/app/sidekeys.go`, `internal/app/boardmenu.go`, `internal/app/model.go`, `internal/app/boardchain_test.go`.
- **Test first:** `TestEnterReopensAWaitingMatch`: `⏎` on the waiting row sends `proto.Act{RecID: "recover", Text: {"names":["kindle-web"]}, Confirm: "y"}`; a result row `cannot` with `Why` shows as the notice `kindle-web not reopened: <why>`; `reopened` shows `kindle-web reopened`. `TestTheMenuOfAWaitingMatch`: `m` lists `reopen it`, `copy its name`, `copy its conversation id`, `out of login`, with the room closed; `out of login` asks for a typed `y`, then sends `chain:out:sid:c-3@login`.
- **Hostile input:** a waiting name with a quote or a newline (`kindle"web`) goes into the act as JSON, never as a shell word: assert the act's `Text` decodes to the same name.
- **Run:** fails, then passes.
- **Do:** `listKey` Enter on a `wait:` key builds the recover act with `json.Marshal(proto.Recover{Names: []string{name}})`; `result` handles `Result.Recover` rows into one notice. `menuFor` gets a waiting branch; `outOfTeam` takes a target (`pane` or `sid:<id>`).
- **Done when:** both tests pass.
- **Commit:** `feat(app): reopen a waiting match, or take it out of its team (#444)`

### Task 8: `↻ N to reopen` in the foot

- **Files:** `internal/app/sidepanel.go`, `internal/app/sidescreen.go`, `internal/app/sidepanel_test.go`.
- **Test first:** `TestTheFootCountsMatchesToReopen`: two waiting mates in two teams (one in both counts once): the foot holds `↻ 2 to reopen`; with none it holds no `↻`. `TestTheFootReopensThemAll`: pressing it asks `reopen 2 matches: kindle-web, wick-mobile? y`, and a typed `y` sends one recover act with both names.
- **Run:** fails, then passes.
- **Do:** `footParts` adds `footReopen` with the count of distinct waiting conversations; `openFoot(footReopen)` sets `m.pending` to a destructive action whose `say` names the matches.
- **Done when:** both tests pass.
- **Commit:** `feat(app): the foot reopens every waiting match (#444)`

### Task 9: docs, landing page, changelog

- **Files:** `docs/console.md` (The list, The menu), `docs/keys.md`, `docs/agents.md` (chains), `www/index.html`, `CHANGELOG.md`.
- **Test first:** `.github/scripts/landing-guard.sh "feat: a match that waits for recover shows in its team" <body-file> origin/main` passes; the repo's doc checks (`go test ./internal/demo` if docs feed it) pass.
- **Do:** document the row, the color, the head count, `⏎`, the menu lines, the foot part, and what a sender hears. Changelog `Added`, headline 8 words or fewer.
- **Done when:** the guard passes.
- **Commit:** `docs: say how a match that waits for recover shows and comes back (#444)`

### Task 10: review

- Run `builder:review` on the whole branch against `fix/391-chains-survive-reboot` (then `main` once #440 merges), with the spec (#444) and the test command. No open Critical or Important finding.

## Lens findings

- **Engineering — a waiting member's pane id is stale and can name a live match (found in #440's review).** Changed: every act on a waiting member targets it by session id (`sid:<id>`), never by pane (Tasks 2, 3, 7). Its list key is `wait:<id>` (Task 6).
- **Engineering — `chains.Member` has no name.** Changed: the doc carries `Wait.Name` from the service (Task 1); the app never guesses a name from a pane.
- **Design — on a desk the right side is the agent's real pane; the console draws no chat there.** Changed: the chosen waiting row's second line carries the facts and both keys (Task 6). The chat's-place view shows only where the chat is drawn (demo, phone, no tmux). The mockup drew the desk case with a chat; this plan follows the real layout.
- **Design — no recover sheet exists in the console.** Changed: `↻ N to reopen` asks one typed-`y` confirm that names the matches (Task 8), instead of a new sheet. Reopening starts agents, so it confirms first, as recover does in the CLI.
- **Design — a refusal must name the fix.** Task 7's notice passes recover's own `Why`, which already names the fix (`… is not installed here: install it, then run matchblox recover again`).
- **Design — an empty group takes no room (owner, 2026-11-14).** Task 5's `TestNoWaitingMateNoChange` pins that nothing changes when nothing waits; the foot part takes no room at zero (Task 8).
- **Architecture — the doc schema grows (`MateView.Wait`, `omitempty`).** An older console ignores the field and shows what it shows today. A newer console with an older service sees no `Wait` and draws nothing new. No change needed.
- **Architecture — a new act target `sid:` on `message:` and `chain:out:`.** Only the console (out) and a chained match (message) may use them, under the existing guards. Task 3 tests the unchained refusal.

## Spec check

| Spec line | Task |
|---|---|
| The service marks each waiting member in the chains view | Task 1: the doc names each waiting member |
| The list draws waiting members in their team in the waiting color; no row when nothing waits | Task 5: the list draws waiting members in their team |
| `⏎` reopens that match; a refusal says why and names the fix | Task 7: `⏎` reopens; the menu takes it out |
| `out of <team>` takes a waiting member out after a typed `y` | Task 2 (service) and Task 7 (menu) |
| `↻ N to reopen` opens the recover sheet | Task 8 (a typed-`y` confirm, not a sheet: see Lens findings) |
| A send tells the sender once that the line waits | Task 3 (service) and Task 4 (CLI) |
| Tests cover each line; docs and landing page say it | Tasks 1–8 tests; Task 9 docs and `www/` |
| The chat's place shows the facts and the two acts | Task 6 (chat where drawn; the second line on a desk) |

## Calls

- Plan path `docs/superpowers/plans/`, as the repo's other plans.
- The branch stacks on #440; it rebases on `main` once #440 merges.

## Changes during build

- Task 5: a team whose members all wait sits last in IDLE; its waiting members count in the IDLE head. Its head drops `step` and reads `3 wait`. Night uses the dark lavender. (builder's call)
- Task 5 check: `sideCount` lost a branch no list reaches; a test pins that a gone member with no `Wait` draws no row.
- Task 6: the list key of a waiting row is `wait:<session id>`, not `wait:<conversation>`: every act on it goes by session id (Tasks 2, 3, 7). `sideChosen()` stays false on a waiting row, so no session act runs there; `x`, `p`, `r`, `c`, typed text and (on a desk) `⏎` show `<name> waits for recover · matchblox recover reopens it` until Task 7.
- Task 4: `send` now prints the service's note when it typed nothing; a live mate's `queued for its next turn` shows too.
- Task 6 check: a member that waits in two teams is one row, in the first team (as a live match in two chains); a right-click or a long press on a waiting row opens its menu.
