# The agent's own pane beside the list: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On a desk, the console's right side is the chosen agent's real tmux pane. `⌥m` flips the keys between the list and the agent. `g` on a chain tiles up to 4 member panes.

**Architecture:** The console in its home session (`MATCHBLOX_HOME=1`, inside tmux) splits its window once. The right pane runs a nested tmux client (`TMUX` unset) of a session grouped with the agent's. For a peer, it runs the existing `JumpSSH`. A row choice respawns that pane. tmux key bindings flip the focus and keep the zoom on narrow screens. The console's own pane id and the agent pane's id live in window options, so the user-key bindings find the console when the agent pane has the keys.

**Tech Stack:** Go, bubbletea v2, tmux ≥ 3.2 (CI floor), real-tmux tests via `internal/tmuxtest`.

**Spec:** `docs/superpowers/specs/2026-11-16-agent-beside-the-list-design.md` (approved by the owner 2026-11-16 with the toggle key, "3 is what i want").

## Global Constraints

- Do not change `matchblox sessions --json`, `peek`, `send`, or the `ctrl+o` jump.
- Never `join-pane`, `move-pane` or `swap-pane` an agent's pane. The agent keeps its own session, window, pane id and name.
- The toggle key is `M-m` (`⌥m`). It acts only in the `matchblox` session's window that has `@matchblox_beside` set. Everywhere else it is `send-keys M-m`.
- Window options: `@matchblox_console` = the console's pane id; `@matchblox_beside` = the agent pane's id; `@matchblox_chain` = the member panes' ids, space-separated (chain view only).
- The list is 40 columns (`listWide`), 32 on a mid screen (`listMedium`). The agent pane takes the rest. Where `window_width - listW - 1 < chatMin` (40) or the client is Pocket upright, one pane shows (zoom). The **window's** width decides, not the console pane's.
- The way back (`prefix m`, APC 900, a tap on "◂ matchblox", `⌥m` from the agent) always ends on the list. Where one pane shows, it keeps the window zoomed (`select-pane -Z`).
- Beside mode is on only for a console inside tmux in its home session (`MATCHBLOX_HOME=1`) with no demo and no fixtures. The demo, the iOS demo and a console with no tmux keep the drawn chat and the line.
- The chain view shows at most 4 members: the 4 first in the list's urgency order (`sideOrder`).
- `app` must not import `panes` (arch test). tmux argv builders that `app` needs go in `internal/remote`. Bindings that `main` installs go in `internal/panes`.
- Fixtures and docs use the demo fleet of DESIGN.md §7: `hearth-api`, `flint-auth`, `spark-docs`, `kindle-web`, `ember-billing`, `cinder-ci`, `wick-mobile`, and the host `ws-1`. The chain in examples is `login` (`kindle-web`, `flint-auth`, `hearth-api`).
- Commits: signed Conventional Commits. `go test ./...` passes before each Go commit. Run the deny-list grep on the staged diff before each commit. Write the docs in ASD-STE100. The CHANGELOG headline has 8 words or fewer.
- `www/` belongs to matchbloxrelease. The landing change goes to it as a brief.

## Review Focus

1. **Two devices on one console** (desk + Pocket): a tap on the phone respawns the desk's right side, and the phone's zoom must not leave the desk zoomed after the phone detaches. Expected: the zoom follows the window width that `window-size latest` gives. Test: Task 3 `TestTheZoomFollowsTheWindowWidth`.
2. **The agent pane's command fails** (agent gone between the choice and the respawn, peer ssh refused): the pane must say why and name `⌥m`, never stay blank. Test: Task 1 `TestAViewOfAGonePaneSaysSo`.
3. **The person kills the agent pane** by hand (`prefix x`) or the console restarts: the next ⏎ must make a new agent pane, never respawn a dead id. Test: Task 4 `TestAChoiceRemakesAMissingAgentPane`.
4. **`⌥m` in an agent pane outside the console's window** (a `ctrl+o` jump, a plain tmux window): the agent must get `⌥m` as typed. Test: Task 2 `TestTheFlipKeyTypesOutsideTheConsoleWindow`.
5. **The chain view while a member ends**: its pane shows the ended line. A new `g` makes the view from the members that still run. Test: Task 6 `TestAChainViewSkipsAnEndedMember`.

---

### Task 0: Start from main with #423 and #425

- [ ] **Step 1:** When #423 and #425 are merged, run `git merge origin/main` in `/home/blox-master/dev/matchblox-beside`. The branch holds only docs, so expect no conflict.
- [ ] **Step 2:** Run `go test ./...`. Expected: ok.

### Task 1: The view: a nested client of the agent's pane

**Files:**
- Create: `internal/remote/view.go`, `internal/remote/view_tmux_test.go`
- Modify: `internal/remote/jump.go`: let `Remote()` take a Jump with `Host == ""` for this machine (only `JumpAllowed` checks the host).

**Interfaces:**
- Produces: `func ViewArgv(j Jump, name string) []string`. This is the argv that a pane runs to show `j.Pane` of this machine. It is `/bin/sh -c <wrap> sh <name> <session> env -u TMUX <j.Remote()...>`. `wrap` runs the list. It kills `=matchblox-jump-<id>` when the list fails. When the client ends, it prints `<name> ended. ⌥m: back to the list` (or, on a failure, `<name> cannot be shown: <tmux's error>. ⌥m: back to the list`) and then waits for a key, without exiting.
- Produces: `func ViewAllowed(j Jump) bool`. It is `JumpAllowed` with `Host == ""` allowed, `Pane` set and `Dir` empty.
- Consumes: `Jump.Remote()`, `JumpSessionName`, `tmuxtest.Server`.

- [ ] **Step 1: Write the failing tests** in `view_tmux_test.go` (`//go:build unix`, real server from `tmuxtest.Server`). Each test sets `TMUX` for the nested client to the test socket. The view runs `tmux -L <sock>`, so `ViewArgv` needs the socket. Add an unexported `tmuxBin []string` package var that defaults to `{"tmux"}`, and set it in the tests.
  - `TestAViewTypesIntoTheAgentsOwnPane`: create session `kindle-web`, split the console window, and run `ViewArgv` in the right pane. `send-keys -t <view> 'echo typed-in-view' Enter`. `capture-pane -t kindle-web` contains `typed-in-view`. `list-sessions` shows `matchblox-jump-<id>` with `session_group == kindle-web`.
  - `TestARespawnEndsTheOldView`: respawn the view pane with a view of `flint-auth`. Within 2 s, `matchblox-jump-<id1>` is gone and `kindle-web` still has 0 attached clients.
  - `TestAViewEndsWithItsAgent`: `send-keys -t flint-auth exit Enter`. The view pane shows `flint-auth ended. ⌥m: back to the list`. No `matchblox-jump-*` session is left.
  - `TestAViewOfAGonePaneSaysSo`: run a view of `%999`. The pane shows `cannot be shown` and `⌥m`. No `matchblox-jump-*` session is left.
- [ ] **Step 2:** Run `go test ./internal/remote -run View`. Expected: FAIL (`ViewArgv` undefined).
- [ ] **Step 3:** Implement `ViewArgv` and `ViewAllowed` in `view.go`. `Remote()` stays the same list. `Outer` is `true` for a view (status off).
- [ ] **Step 4:** Run `go test ./internal/remote`. Expected: ok.
- [ ] **Step 5:** Commit `feat: a pane can show an agent's own pane as a nested client`.

### Task 2: The flip key and a way back that finds the console

**Files:**
- Create: `internal/panes/beside.go`, `internal/panes/beside_tmux_test.go`
- Modify: `internal/panes/home.go`: `atConsole`, `goTo`, and the targets of `MarkKeyArgv`, `BeltKeyArgv`, `TapKeysArgv` and `ViewerHookArgv`. Modify `cmd/matchblox/main.go:306-323`: install `BesideKeysArgv()` beside `FlipKeysArgv()`.

**Interfaces:**
- Produces in `panes`: `const BesideKey = "M-m"`, `const ConsoleOption = "@matchblox_console"`, `const BesideOption = "@matchblox_beside"`, `const ChainOption = "@matchblox_chain"`, `func BesideKeysArgv() []string`.
- Produces in `remote` (so `app` can use them): the same three option names as `remote.ConsoleOption`, `remote.BesideOption` and `remote.ChainOption`. `panes` refers to the `remote` ones, so there is one owner.
- `BesideKeysArgv` binds `bind-key -n M-m if-shell -F "#{&&:#{==:#{session_name},matchblox},#{@matchblox_beside}}" <flip> "send-keys M-m"`. `<flip>` runs through `run-shell -C`. On the console pane it runs `last-pane -Z`, and when there is no last pane it runs `select-pane -Z -t #{@matchblox_beside}`. On any other pane it runs `select-pane -Z -t #{@matchblox_console}`.
- `atConsole` becomes true in the `matchblox` session when the active pane runs the console, **or** when the window has `@matchblox_console`. The key hands go to `-t #{?@matchblox_console,#{@matchblox_console},#{pane_id}}`.
- `goTo` selects the console pane with `select-pane -Z`.

- [ ] **Step 1: Write the failing tests** (real tmux):
  - `TestTheFlipKeyGoesToTheAgentAndBack`: a `matchblox` window with a pane `C` running `sleep` (named so that `pane_current_command` is not the console) and an agent pane `A`. Set both options and attach a client. `send-keys -K M-m` (or type `\x1bm` with the client helper): the active pane is `A`. Again: the active pane is `C`.
  - `TestTheFlipKeepsTheZoom`: zoom `C` (`resize-pane -Z`), then press M-m. `A` is active, and `window_zoomed_flag` is 1.
  - `TestTheFlipKeyTypesOutsideTheConsoleWindow`: in a session `kindle-web` with a `cat` pane, M-m reaches the pane (`capture-pane` shows `^[m` or the `cat -v` form).
  - `TestTheWayBackSelectsTheConsolePane`: with `A` active and zoomed, run the HomeKey binding. `C` is active and the window stays zoomed.
  - `TestAMarkReachesTheConsoleFromTheAgentPane`: with `A` active, send `keycmd.MarkKey`. The bytes arrive in `C` (C runs `cat -v > file`), not in `A`.
- [ ] **Step 2:** Run `go test ./internal/panes -run 'Flip|WayBack|Mark'`. Expected: the new tests FAIL.
- [ ] **Step 3:** Implement the changes. Keep the existing home.go tests green, and update their expected strings where the target changes.
- [ ] **Step 4:** Run `go test ./internal/panes ./cmd/...`. Expected: ok.
- [ ] **Step 5:** Commit `feat: ⌥m flips between the list and the agent`.

### Task 3: Beside mode: the console splits its window

**Files:**
- Create: `internal/app/beside.go`, `internal/app/beside_test.go`, `internal/remote/beside.go`
- Modify: `internal/app/model.go` (Options, Model fields, `tea.WindowSizeMsg`), `internal/app/sidescreen.go` (`onePane`, `sideBody`), `cmd/matchblox/main.go` `runConsole` (set `Beside`).

**Interfaces:**
- `Options.Beside bool`: true in `runConsole` when inside tmux, `MATCHBLOX_HOME=1`, no demo and no fixtures.
- `Model.beside string`: the agent pane's id, or "" before the split.
- `remote.SplitArgv(console string, listW int) []string`: `tmux split-window -h -d -t <console> -l <rest>% -P -F '#{pane_id}' -- /bin/sh -c 'printf "⏎ on a row shows its agent here.\n"; read _'`. Then it sets the two window options, and `pane-active-border-style fg=<accent>` on the window, so the pane that has the keys shows (spec anti-goal). It returns the new pane id through `Options.Tmux`.
- `remote.ListWidthArgv(console string, listW int) []string`: `tmux resize-pane -t <console> -x <listW>`.
- `remote.ZoomArgv(pane string, zoom bool) []string`: selects the pane with `-Z` and zooms or unzooms it to match `zoom`.
- `func (m Model) windowWidth() int`: from `Options.Tmux("display", "-p", "-t", m.opt.Pane, "#{window_width}")`. 0 when it does not read, and then the pane's width counts.
- `onePane(w)` uses `windowWidth()` while `m.beside != ""`.
- `sideBody` in beside mode draws only the list, at the pane's full width. It draws no chat place and no line.

- [ ] **Step 1: Write the failing tests** with a fake `Options.Tmux` and `Options.Run` that record argv:
  - `TestTheConsoleSplitsItsWindowOnceAtStart`: init with `Beside: true`, `Pane: "%1"`. The recorded argv has one `split-window -h -d -t %1`, and `set-option -w @matchblox_console %1` and `@matchblox_beside %9` (the fake returns `%9`). A second `WindowSizeMsg` makes no second split.
  - `TestABesideConsoleDrawsOnlyTheList`: at 40×50 with window width 160, the view has no `│` chat border and no line prompt, and it has the list's rows (`kindle-web`).
  - `TestTheZoomFollowsTheWindowWidth`: window width 160 gives `ZoomArgv(…, false)`. Window width 70 gives `ZoomArgv(%1, true)` (the list is in use).
  - `TestTheListKeepsItsWidth`: a WindowSizeMsg at window width 200 runs `resize-pane -t %1 -x 40`. At 110, it runs `-x 32`.
  - `TestTheDemoKeepsTheDrawnChat`: `Beside: false` makes no tmux call, and the chat place is drawn as today.
- [ ] **Step 2:** Run `go test ./internal/app -run 'Beside|Zoom|ListKeeps|DemoKeeps'`. Expected: FAIL.
- [ ] **Step 3:** Implement. The split runs once, in `Init` as a `tea.Cmd`, and its pane id comes back as a message (`besideMsg{pane string, err error}`). A failed split leaves `m.beside == ""` and the drawn chat, with the flash `the agent pane did not open: <err>`.
- [ ] **Step 4:** Run `go test ./...`. Expected: ok.
- [ ] **Step 5:** Commit `feat: the desk console keeps a pane for the agent`.

### Task 4: A choice shows the agent

**Files:**
- Modify: `internal/app/beside.go`, `internal/app/sidekeys.go` (`listKey` ⏎), `internal/app/sidescreen.go` (`chooseSide`), `internal/app/remote.go` (factor the peer checks), `internal/app/into.go`.
- Test: `internal/app/beside_test.go`; update the existing tests that expect `toLine` on ⏎ or a click in beside mode.

**Interfaces:**
- `func (m Model) peerJump(host, pane string, a *action) (remote.Jump, []string, *action)`: the checks of `jumpOnHost` (`JumpAllowed`, the tmux floor, the hello tmux, the host key, the master). It returns the Jump and its `JumpSSH` argv, or a refused action. `jumpOnHost` calls it, and its behaviour stays the same.
- `func (m Model) showBeside(host, pane string) (tea.Model, tea.Cmd)`: the beside form of `goIntoAsking`. The take-over question comes first (`takeOverOn`). A peer's consent box comes first (`m.pending`), as in `runInto`. Then it runs `respawn-pane -k -t <beside> -- <ViewArgv or JumpSSH argv>`, then `ZoomArgv(beside, onePane)` (this also selects the agent pane).
- `action.beside bool` marks this action kind for `runInto`.
- In beside mode, ⏎ in the list and a click or tap on a row call `showBeside`. ↑↓ change only the choice. `ctrl+o` stays the full jump.
- When `@matchblox_beside` names no live pane (`display -p -t <id> '#{pane_id}'` fails), the console runs `SplitArgv` again before the respawn.

- [ ] **Step 1: Write the failing tests:**
  - `TestEnterShowsTheChosenAgentBeside`: at `kindle-web`, ⏎ records `respawn-pane -k -t %9 --` with `/bin/sh` and `matchblox-jump-` in the argv, then `select-pane -Z -t %9`.
  - `TestArrowsDoNotRespawn`: ↓↓ records no `respawn-pane`.
  - `TestAPeerAgentAsksBeforeItShows`: a session on `ws-1`. ⏎ opens the consent box. `y` records `respawn-pane` with `ssh` and `ws-1` in the argv.
  - `TestAStokerHostAsksToTakeOver`: with `takeOverOn` true, ⏎ sets `m.held`, and no respawn happens until the answer.
  - `TestAChoiceRemakesAMissingAgentPane`: the fake `Tmux` fails on `%9`. ⏎ records `split-window` before `respawn-pane`.
  - `TestOnePaneZoomsTheAgent`: window width 70, ⏎ records `ZoomArgv(%9, true)`.
- [ ] **Step 2:** Run them. Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run `go test ./...`. Expected: ok. Run `go test ./internal/replay -run TestReplay -update` only if a replay frame changes because of the beside mode. The replay runs the demo, so expect no change.
- [ ] **Step 5:** Commit `feat: a row's ⏎ or a tap shows its agent beside the list`.

### Task 5: By hand on this machine (desk and Pocket)

- [ ] **Step 1:** `go build -o /tmp/claude-1000/-home-blox-master-dev-matchblox/32b14168-47b7-4fd7-b2e7-4f584405105a/scratchpad/matchblox ./cmd/matchblox`. Start it on a test socket (`tmux -L beside new -s matchblox -e MATCHBLOX_HOME=1 <bin>`), with two sessions that run `claude` (or `bash`). Check every "Done when" line of the spec that does not need a peer, and record what you see in the ledger.
- [ ] **Step 2:** Fix what fails, each fix with a test first. Commit `fix: …` per finding.

### Task 6: The chain view

**Files:**
- Modify: `internal/app/beside.go`, `internal/app/keys.go` (`g`, `G`, `esc` in beside mode), `internal/app/sidelist.go` (a mark on the members on screen), `internal/remote/beside.go`.
- Test: `internal/app/beside_test.go`.

**Interfaces:**
- `remote.ChainArgv(beside string, views [][]string) []string`: one tmux list. It respawns `beside` with `views[0]`. For 2 views it runs `split-window -v -t beside`. For 3 or 4 it makes a 2 × 2 grid: split `beside` with `-v`, then `-h` on each half, and the 4th cell only with a 4th view. It sets `@matchblox_chain` to the pane ids and returns them (`-P -F '#{pane_id}'`).
- `func (m Model) chainBeside(k chainKey) (tea.Model, tea.Cmd)`: up to 4 members of `k`, in `sideOrder`, that still run. It makes one view each: a `ViewArgv`, or a `JumpSSH` through `peerJump` (one consent box for all of a peer's members).
- `func (m Model) closeChainBeside() tea.Cmd`: `kill-pane` for every id in `@matchblox_chain` except `beside`, then unsets the option.
- Keys in beside mode: `g` on a chain's row or a member's row opens the chain view, and `g` again closes it. `esc` with no filter closes it. `G` opens the chain's room (today's `g`).
- The list draws `▸` before each member on screen.

- [ ] **Step 1: Write the failing tests:**
  - `TestGOpensTheChainBeside`: on the chain `login`, `g` records one `respawn-pane` and `split-window` ×3 with `kindle-web`, `flint-auth` and `hearth-api` in the views, and `set-option -w @matchblox_chain`.
  - `TestAChainViewShowsAtMostFour`: a chain of 6 shows the first 4 of `sideOrder`.
  - `TestAChainViewSkipsAnEndedMember`: a member with no pane is left out, so 2 members give 1 split.
  - `TestGAgainClosesTheChainView`: it records `kill-pane` for the extra ids, and the list shows no `▸`.
  - `TestBigGOpensTheRoom`: `G` on `login` opens the room, as `g` does today on the drawn chat.
- [ ] **Step 2:** Run them. Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run `go test ./...`. Expected: ok. By hand: on the test socket, `g` on a chain of 3 shows 3 panes, a click types into one, `prefix z` zooms it, and `⌥m` goes to the list and back to the member used last.
- [ ] **Step 5:** Commit `feat: g shows a chain's agents side by side`.

### Task 7: Docs, CHANGELOG and the landing brief

**Files:**
- Modify: `README.md` ("## Quick start", "## What it shows", "## Keys", "## From a phone"), `docs/console.md` ("## The chat", "### The line", "### Go in", "## A chain's room"), `docs/keys.md` ("## The list and the chat", "## A chain's room", "### The flip from a pane", "## A phone"), `docs/phone.md` ("### Use the app", "## Go into an agent", "### The key belt"), `docs/start.md` (:60, :119), `docs/tmux.md` ("## The way back without config"), `DESIGN.md` §6 (Chat, Chains, Keys), `CHANGELOG.md` (Unreleased, Added).

- [ ] **Step 1:** Write the docs in ASD-STE100.
  - README first: the right side is the agent's own pane, `⌥m` flips, and `g` shows a chain.
  - Name where the drawn chat and line remain: the demo, no tmux, a console outside its home session.
  - In `docs/keys.md`, add `⌥m` to the key table and to the terminal note on Option as Meta (the same note as `⌥s`). Add `G`, and the new meaning of `g` and `esc` in beside mode.
- [ ] **Step 2:** CHANGELOG Added headline, 8 words or fewer: `**Type straight into an agent beside the list.**` (7 words). Add a second bullet for the chain view.
- [ ] **Step 3:** Run `go test ./internal/docscheck/... ./...`. Expected: ok (docscheck holds the docs to the code's keys).
- [ ] **Step 4:** Commit `docs: the console shows the agent's own pane and ⌥m flips`.
- [ ] **Step 5:** Send matchbloxrelease the landing brief through the fleet (a peer message, no yes needed):
  - the words: "Type straight into an agent beside the list. ⌥m flips between them. g shows a whole chain at once.";
  - the visual: a list beside a real Claude Code pane, and the 2 × 2 chain view of `login`.

  The feat PR waits for its `www/` change, or carries `Landing page: unchanged because …` only if matchbloxrelease declines.

### Final

- [ ] Whole-branch review on the most capable model. Then `superpowers:finishing-a-development-branch`: push and open the PR, **after the owner's yes**, with "Closes" lines for the bug-2 issue.
