# Answer Any Menu From the Chat — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every numbered menu an agent draws, a permission prompt or any question, is answered from the chat with two digit presses, on a real machine and on a peer.

**Architecture:** One menu reader in `internal/panes` reads the last run of numbered lines on a pane's screen. The service gets a console-only act `choose:<pane>`: it reads the pane again under the pane's lock and types the digit only if that line still starts with the text the console saw. The console reads the menu from the chat's screen and sends every digit answer by this act, Claude Code's question tool too (v0.46.0 sent `keys:`, which the real service refuses).

**Tech Stack:** Go, Bubble Tea v2, tmux, `github.com/charmbracelet/x/ansi`.

**Spec:** `docs/superpowers/specs/2026-11-16-answer-any-menu-design.md`

## Global Constraints

- Signed Conventional Commits, never `--no-verify`, no WIP commit. Before each commit: `git diff --cached | rg -v -i 'blox-eng' | rg -i -f ~/.config/builder/denylist.txt` prints nothing.
- `go vet ./...`, `gofmt -l .` (empty), `staticcheck ./...` and `go test ./...` pass before every commit that touches Go.
- Do not change `matchblox sessions --json`, `peek` or `send`. `keys:` stays demo-only (`KeysToWorld`).
- Fixtures and docs use the demo fleet's names (DESIGN.md §7): `flint-auth`, `kindle-web`, `hearth-api`, `wick-mobile`.
- Refusal words, verbatim: `not sent: <name>'s menu changed — look again`; `not sent: <name> shows no menu`; `not sent: <pane> runs a different agent now`.
- The line's words when the session needs the owner and no menu is read: `answer in its pane · ctrl+o` (the key from `m.keys.main.Jump.Help().Key`).
- Docs and comments in ASD-STE100: one idea per sentence, active voice, present tense.

## Review Focus

1. The agent's reply above the menu holds its own numbered list (`1. Add tests`). Only the last run of numbered lines, with nothing but blank or box lines below it before the input, is the menu. Task 1 pins it.
2. The menu changes between the two presses (a new prompt with other options). The service refuses, and nothing reaches the pane. Task 2 pins it.
3. A peer's session: the act goes on the peer's own conn, and a refusal from the peer shows on the line. Task 3 pins it.
4. A menu with 10 or more options: digits pick 1 to 9 only, and the line says `1-9 only`. Task 3 pins it.
5. The digit is typed into a text box, not a menu (the session no longer asks): the console sends nothing, and the line types the digit. Task 3 pins it (it is today's `digitAnswers` rule, kept).

---

### Task 1: One menu reader

**Files:**
- Create: `internal/panes/menu.go`, `internal/panes/menu_test.go`
- Modify: `internal/app/chat.go` (`agentMenuLine`, `paneAsks`: use the reader)

**Interfaces:**
- Produces: `func Menu(screen string) map[int]string`: in `screen` (styled or plain), the options of the last run of numbered lines, by number, the text trimmed of the marker (`❯ › >`) and spaces. The run is consecutive lines that match `^\s*[›❯>]?\s*(\d+)\.\s+(.*?)\s*$` (ANSI stripped). A wrapped line of an option (indented, no number) does not end the run. A run ends where a line of other text follows it, unless only blank lines and box-drawing lines (`─│╭╮╰╯` and spaces) follow up to the screen's end or to the agent's input line (`❯ ` or `> ` with no number). Nil when there is no run that ends the screen this way.

- [ ] **Step 1: Write the failing tests** in `internal/panes/menu_test.go`:
  - `TestMenuReadsAPermissionPrompt`: a Claude Code prompt screen (`Do you want to proceed?`, `❯ 1. Yes`, `  2. Yes, and don't ask again for go test commands`, `  3. No, and tell Claude what to do differently (esc)`, then the box foot) gives `{1: "Yes", 2: "Yes, and don't ask again for go test commands", 3: "No, and tell Claude what to do differently (esc)"}`.
  - `TestMenuSkipsANumberedReplyAboveIt`: a reply with `1. Add tests` / `2. Ship`, then prose `Which way?`, then a menu `❯ 1. Fast` / `2. Safe` at the foot gives `{1: "Fast", 2: "Safe"}`.
  - `TestMenuNoneWhenTextFollows`: a numbered list followed by a prose paragraph and the input box gives nil.
  - `TestMenuReadsStyledText`: the same prompt with SGR colour codes gives the same map.
- [ ] **Step 2:** `go test ./internal/panes -run TestMenu` fails: `Menu` is undefined.
- [ ] **Step 3:** Implement `Menu` in `internal/panes/menu.go`. Then make `paneAsks` in `internal/app/chat.go` read `panes.Menu(chatCut(s.Agent, m.screen.Text))`, and delete `agentMenuLine`.
- [ ] **Step 4:** `go test ./internal/panes ./internal/app` passes.
- [ ] **Step 5:** Commit `refactor: one reader finds an agent's menu on its screen`.

### Task 2: The service's `choose:` act

**Files:**
- Modify: `internal/proto/proto.go` (add `ChoosePrefix`), `internal/service/actions.go` (dispatch, beside `pick`), `internal/service/hands.go` (`paneActs`, `typedPane`)
- Create: `internal/service/choose.go`, `internal/service/choose_test.go`

**Interfaces:**
- Consumes: `panes.Menu` (Task 1); `s.Capture`, `s.holdPane`, `s.sessionAt`, `sameAgent`, `s.aConsole`, `panes.Literal`, `s.kickScreens`, `s.undrive`.
- Produces: `const ChoosePrefix = "choose:"`. The act `proto.Act{RecID: ChoosePrefix + pane, Text: "<n>\x00<option text>", PID, Start, Confirm: "y", Console: true}`. `func (s *Service) choose(ctx context.Context, pane string, a proto.Act) proto.Result`. `Ran` has one entry when the digit went.

- [ ] **Step 1: Write the failing tests** in `internal/service/choose_test.go`, with a fake `Capture` and a recording runner (the pattern of the paste tests):
  - `TestChooseTypesTheDigitWhenTheMenuStands`: the screen holds the prompt of Task 1, the act is `2\x00Yes, and don't ask again for go test commands`. The runner gets one `send-keys -t <pane> -l -- 2`, and `Ran` has one entry.
  - `TestChooseRefusesAChangedMenu`: the screen's option 2 now says `No`. `Err == "not sent: flint-auth's menu changed — look again"`, and the runner gets nothing.
  - `TestChooseRefusesNoMenu`: the screen has no menu. `Err == "not sent: flint-auth shows no menu"`.
  - `TestChooseRefusesAnotherAgent`: the act's PID differs. `Err == "not sent: <pane> runs a different agent now"`.
  - `TestChooseIsConsoleOnly`: `Console: false`, or `From` an agent pane: refused, runner gets nothing.
  - `TestChooseRefusesABadText`: text `x\x00Yes`, `0\x00Yes`, `10\x00Yes` or no `\x00`: refused, runner gets nothing.
- [ ] **Step 2:** `go test ./internal/service -run TestChoose` fails.
- [ ] **Step 3:** Implement `choose` in `internal/service/choose.go`: check console and confirm, parse `n` (1 to 9) and the text, take `s.holdPane(pane)`, check `sameAgent`, capture the pane, read `panes.Menu`, compare `strings.HasPrefix(menu[n], text)` after the same trimming, then run `panes.Literal(pane, strconv.Itoa(n))` through `s.Actions.Do` with `advice.Guard{PID, StartTicks}`. After it ran: `s.undrive(pane)` and `s.kickScreens(pane)`. Add `ChoosePrefix` to `paneActs` and `typedPane`, and dispatch it in `actions.go` next to `pick`. A peer conn takes it as it takes `paste:` (check `peersend.go`: it must pass the same gate).
- [ ] **Step 4:** `go test ./internal/service` passes.
- [ ] **Step 5:** Commit `feat: the service types a menu pick only while the menu the console saw stands`.

### Task 3: The console answers every menu by `choose:`

**Files:**
- Modify: `internal/app/boardkeys.go` (`answerFromChat`, `sendAnswer`, `chatMarks`), `internal/app/sidekeys.go` (`digitAnswers`, `answerResult`), `internal/app/boardcard.go` (the options the card and chat draw)
- Test: `internal/app/menu_test.go` (create)

**Interfaces:**
- Consumes: `panes.Menu` (Task 1); `proto.ChoosePrefix` and its text form (Task 2).
- Produces: `func (m Model) chatMenu(s sample.Session) (map[int]string, bool)`: the menu on the chat's screen of s, when the chat watches s (the `paneAsks` checks) and `m.asksNow(s)`. `oneChoice` options still win when the question tool's options match the screen; else the screen menu is the menu. `digitAnswers(s)` is true when `chatMenu` is. `sendAnswer(s, n, text)` sends the `choose:` act by `m.sendToID(s.Host, …)` on both hosts, and `answerResult` handles its result for both.

- [ ] **Step 1: Write the failing tests** in `internal/app/menu_test.go` (helpers: `sideConsole`, `pickSide`, `pressOn`, a screen message for the watched pane as the chat tests use):
  - `TestTwoPressesAnswerAPermissionPrompt`: `kindle-web` (claude, `Permits`), its screen has the prompt. `2` marks (`❯ 2.` on the chat, nothing sent), `2` again sends one act `choose:<pane>` with text `2\x00Yes, and don't ask again for go test commands`, `Confirm: "y"`, PID and Start of the session.
  - `TestTheQuestionToolGoesByChoose`: `flint-auth` with `askingMatch(..., "Yes", "No")` and the same menu on screen: `1`, `1` sends `choose:` with `1\x00Yes`. No `keys:` act goes.
  - `TestARefusedPickSaysWhy`: the result `Err: "not sent: kindle-web's menu changed — look again"` shows on the line, and the mark clears.
  - `TestNoMenuSaysAnswerInItsPane`: the session asks (`Permits`), and the screen shows no menu. The chat line's right side says `answer in its pane · ctrl+o`. A digit types into the line as text, and no act goes.
  - `TestAPeersMenuGoesOnItsConn`: `hearth-api` on host `gpu-1`: the act goes on gpu-1's conn, and its refusal shows on the line.
  - `TestTenOptionsPickOneToNine`: a menu of 10 lines: `1-9 only` for `0`; `9`, `9` sends option 9.
- [ ] **Step 2:** `go test ./internal/app -run 'Menu|Choose|TwoPresses|QuestionTool|RefusedPick|TenOptions'` fails.
- [ ] **Step 3:** Implement as the Interfaces say. The mark keys on the menu's text, not on `askOf(s)`, so a new menu clears the mark (`chatMarks`). Keep the 400 ms dead time (`peekDead`) after the menu first shows.
- [ ] **Step 4:** `go test ./...` passes. Regenerate goldens only if a frame changed for a reason this task names (`go test ./internal/app -run TestScreens -update`), and say which in the commit.
- [ ] **Step 5:** Commit `fix: a digit answers any menu in the chat, on a real machine too`.

### Task 4: Docs and changelog

**Files:**
- Modify: `docs/console.md` (the digits paragraph after "### The line"), `docs/phone.md` (answering), `CHANGELOG.md` (`## [Unreleased]`, `### Fixed`)

- [ ] **Step 1:** In `docs/console.md`, replace "The digits answer a question with one choice … a choice of several options." with: the digits answer any numbered menu the agent draws: a permission prompt, a question, a Codex or OpenCode choice. The service types the digit only while that menu stands. With no menu read, the line says `answer in its pane · ctrl+o`.
- [ ] **Step 2:** `CHANGELOG.md` Fixed entry: **A digit answers any agent's menu from the chat.** v0.46.0's digits went as keys, which the service refuses outside the demo. Name the issue number.
- [ ] **Step 3:** `rg -n 'question with one choice' docs README.md` prints nothing.
- [ ] **Step 4:** Commit `docs: the digits answer any menu`.
