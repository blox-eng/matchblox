Spec: docs/superpowers/specs/2026-11-21-unknown-screen-design.md
Spec approved: 2026-11-21
Test command: go vet ./... && go test ./...  (CI adds -race on Linux and runs ./internal/demo apart: .github/workflows/ci.yml:142,155)
Plan approved: 2026-11-21 by the owner (with the permission-only trigger)
Next: builder:build

# Plan: a builder sees when matchblox cannot read a waiting agent's screen (#199, first slice)

## What the code says

- A hooked session (Claude Code, Cursor) gets only `readMenus` (internal/sample/sampler.go:904-917, :1204-1219): `said.Menu` sets `Permits` and the question line. Nothing marks a screen that no rule reads.
- The hook's wait lives in the queue: `queue.StatePermission` (internal/queue/queue.go:18, set at :148). The service merges it in `mergeQueue` (internal/service/service.go:784-794).
- Digits are offered only when `menu.Read` parses the screen (internal/app/boardcard.go:132-148, internal/app/chat.go:283-288). The service's `choose` re-reads the live screen and refuses "shows no menu" (internal/service/choose.go:86-88). So an unreadable menu already gets no digit; this plan pins that with tests, and closes the raw `keys:` path.
- The phone's console is the same TUI over SSH (ios/Matchblox/ConsoleView.swift:20): what the console draws, the phone shows. No iOS change.
- No version is read today. argv is read and dropped (sampler.go:995-1050). procfs has no exe reader (internal/procfs/proc.go:153-166). Codex's rollout carries `cli_version` (internal/sample/codex_test.go:21), not kept by `rolloutMeta` (internal/sample/codex.go:350-354).
- `matchblox sessions/ships/usage` read the service doc with `cliRead` (cmd/matchblox/cli.go:55-67) and print with `writeJSON` or a table (cli.go:274, :331-357). Commands are listed at cmd/matchblox/main.go:363.

## File map

- `internal/procfs/proc.go` (change) — adds `Exe(pid)`: the target of /proc/<pid>/exe.
- `internal/sample/version.go` (create) — reads an agent's version from its exe path or argv, never by running it; names each agent for people (`Claude Code`).
- `internal/sample/version_test.go` (create) — the tests of version.go.
- `internal/sample/sampler.go` (change) — keeps `Version` and `VersionWhy` on each Session, cached per process with the name.
- `internal/sample/codex.go` (change) — keeps the rollout's `cli_version` as the Codex session's version.
- `internal/service/unread.go` (create) — marks a session `UnknownScreen` when its hook waits for a permission and no rule reads a menu, after the grace time.
- `internal/service/unread_test.go` (create) — the tests of unread.go.
- `internal/service/keys.go` (change) — refuses a `keys:` act to a session with an unknown screen.
- `internal/app/sessions.go` (change) — the state word `unknown screen`.
- `internal/app/chat.go` (change) — the chat's note line with the agent, its version and the fix.
- `testdata/menus/unknown-wording.txt` (create) — a Claude Code approval menu in words no rule knows.
- `cmd/matchblox/agents.go` (create) — `matchblox agents`: the table and `--json`.
- `cmd/matchblox/agents_test.go` (change) — the tests of the command (the file exists for the sampler's agent list).
- `cmd/matchblox/main.go` (change) — registers `agents`.
- `docs/console.md`, `CHANGELOG.md` (change) — the note and the command, in the docs and under [Unreleased].

## Tasks

### Task 1: an agent's version comes from its path, never from running it

- **Files:** `internal/procfs/proc.go`, `internal/sample/version.go`, `internal/sample/version_test.go`.
- **Surface:** an agent's tools (the anti-goal: reading a version runs no agent binary).
- **Test first:** `TestAVersionComesFromThePath`, a table on `versionOf(agent, exe string, argv []string) (version, why string)`:
  - `claude`, exe `/home/u/.local/share/claude/versions/2.3.1` → `2.3.1`, `""`.
  - `cursor-agent`, argv `[node /home/u/.local/share/cursor-agent/versions/2025.11.06-8fe3a/index.js]` → `2025.11.06-8fe3a`.
  - `claude`, argv `[node /usr/lib/node_modules/@anthropic-ai/claude-code/cli.js]` → `""`, `the install path names no version`.
  - Hostile: exe `/x/claude/versions/../../etc/passwd`, `/x/claude/versions/2.3.1;rm -rf ~`, a 300-byte segment, a segment with `\x1b[2J` → `""` and the same why. A version matches `^[0-9A-Za-z][0-9A-Za-z.+_-]{0,39}$` or it is no version.
  - `TestAWhyNamesNoPath`: no why string contains `/`.
  - `TestAVersionRunsNothing`: `version.go` imports no `os/exec` (read the file's imports with go/parser, as tmuxtest does).
- **Run:** `go test ./internal/sample/ -run 'Version|Why'` → fails: `versionOf` undefined. After: passes.
- **Do:** add `procfs.Exe(pid) (string, error)` (os.Readlink of /proc/<pid>/exe). Add `versionOf` and `AgentLabel(agent string) string` (`claude` → `Claude Code`, `cursor-agent` → `Cursor`, `codex` → `Codex`, `opencode` → `OpenCode`, else the name).
- **Done when:** the four tests pass.
- **Commit:** `feat(sample): read an agent's version from its install path (#199)`

### Task 2: each session carries its agent's version

- **Files:** `internal/sample/sampler.go`, `internal/sample/codex.go`, `internal/sample/version_test.go`.
- **Test first:** `TestASessionCarriesItsVersion`: a fake FS whose Argv/Exe give a Claude Code under `versions/2.3.1` → the sampled Session has `Version == "2.3.1"`. `TestACodexSessionTakesTheRolloutVersion`: the existing rollout fixture with `cli_version` → `Version` is that value. `TestAnUnreadVersionSaysWhy`: an npm Claude Code → `Version == ""`, `VersionWhy` set.
- **Run:** `go test ./internal/sample/` → fails: no field `Version`. After: passes.
- **Do:** add `Version string \`json:"version,omitempty"\`` and `VersionWhy string \`json:"version_why,omitempty"\`` to Session. Compute them with the name in `agentRef` (sampler.go:995-1050) and cache them in `s.names[key]`: one read per process. Add `Exe` to the sampler's FS interface. In `rolloutMeta` keep `cli_version`.
- **Done when:** the three tests pass, and `go test ./internal/sample/` passes.
- **Commit:** `feat(sample): each session carries its agent's version (#199)`

### Task 3: a permission wait that no rule reads becomes an unknown screen, after the grace time

- **Files:** `internal/service/unread.go`, `internal/service/unread_test.go`, `internal/service/service.go`, `internal/sample/sampler.go`.
- **Test first** (a fake clock and samples, as the service tests do):
  - `TestAnUnreadPermissionIsAnUnknownScreen`: a queue `StatePermission` item for the pane, a session with `Permits == false` and `Ask == nil`. At +1 s: `UnknownScreen == false`. At +3 s with a second sample: `true`.
  - `TestOneSampleIsNotEnough`: +5 s but only one sample since the hook → `false`.
  - `TestAReadMenuIsNeverUnknown` (anti-goal: no false alarm): `Permits == true` at every sample for 10 s → `false` at every sample.
  - `TestAMenuThatReadsClearsIt`: true, then a sample with `Permits == true` → `false` in that sample.
  - `TestTheWaitEndingClearsIt`: true, then the queue item goes (PostToolUse) → `false`.
  - `TestAQuestionIsNeverUnknown`: `StateQuestion` (idle_prompt) with no menu → `false`: a question waits for text, not for a menu.
  - `TestNoBlink` (anti-goal): once true, a sample with no capture of the pane (the capture failed) keeps it true; only a read menu or the end of the wait clears it.
- **Run:** `go test ./internal/service/ -run 'Unknown|Unread|Blink|NotEnough|Clears'` → fails: no field `UnknownScreen`. After: passes.
- **Do:** add `UnknownScreen bool \`json:"unknown_screen,omitempty"\`` to Session. In unread.go, `markUnread(now)`, called in `mergeQueue` after the merge: per pane, keep `{since, samples}` while a `StatePermission` item exists and the session reads no menu; set `UnknownScreen` when `now-since >= unreadGrace (3 s)` and `samples >= 2`; keep it set until a menu reads or the item goes; drop the entry when the pane goes.
- **Done when:** the seven tests pass.
- **Commit:** `feat(service): a permission wait that no rule reads is an unknown screen (#199)`

### Task 4: no digit or key reaches an unknown screen

- **Files:** `internal/service/keys.go`, `internal/service/unread_test.go`, `internal/app/` test file of `digitMenu`, `testdata/menus/unknown-wording.txt`.
- **Surface:** an agent's tools (the anti-goal: a digit reaches a session that shows `unknown screen`).
- **Test first:**
  - `TestNoDigitsOnAnUnknownWording` (app): the console model with `unknown-wording.txt` as the screen and a permission item → `digitMenu` offers nothing, and pressing `1` sends no act.
  - Hostile: `TestAQuotedOldMenuGivesNoDigits` (app): `unknown-wording.txt` with an old-style `1. Yes` menu quoted in the agent's reply above it → no digits. If this fails, the plan stops and tells the owner: it is a parser bug outside this slice.
  - `TestChooseRefusesAnUnknownScreen` (service): a `choose:` act to that pane → refused, the text names the fix.
  - `TestKeysRefuseAnUnknownScreen` (service): a `keys:` act `1` to a session with `UnknownScreen` → refused with `matchblox cannot read this screen: answer it in its pane`; nothing is typed (the fake tmux records no send-keys).
- **Run:** `go test ./internal/app/ ./internal/service/ -run 'UnknownWording|QuotedOldMenu|RefusesAnUnknown|RefuseAnUnknown'` → the keys test fails (keys: types today); the others pass and pin today's behaviour. After: all pass.
- **Do:** in `keys.go`, before the send, refuse when the target session has `UnknownScreen`. In `choose.go`, make the "shows no menu" refusal name the fix: `answer it in its pane`.
- **Done when:** the four tests pass.
- **Commit:** `fix(service): no key reaches a screen matchblox cannot read (#199)`

### Task 5: the console says `unknown screen`, with the agent, its version and the fix

- **Files:** `internal/app/sessions.go`, `internal/app/chat.go`, their tests.
- **Test first:**
  - `TestTheRowSaysUnknownScreen`: a session with `UnknownScreen` and an ask → the state word is `unknown screen`, in the warn tone.
  - `TestTheChatNamesTheVersionAndTheFix`: the chat of that session shows `unknown screen · Claude Code 2.3.1 — answer it in its pane: ⏎ opens it`.
  - `TestTheChatSaysVersionNotRead`: `Version == ""` → `unknown screen · Claude Code, version not read — answer it in its pane: ⏎ opens it`.
  - `TestTheNoteGoesWithTheFlag`: the same session with `UnknownScreen == false` shows its question as today.
  - Phone width: the chat line at 44 columns wraps and keeps `⏎ opens it` whole.
- **Run:** `go test ./internal/app/ -run 'UnknownScreen|VersionAndTheFix|VersionNotRead|GoesWithTheFlag'` → fails. After: passes.
- **Do:** in `stateWord` (sessions.go:~118), when `asks && s.UnknownScreen`, return `unknown screen` in `st.warn`. In chat.go, where the ask line is drawn, draw the note instead when `UnknownScreen`. Use `sample.AgentLabel`.
- **Done when:** the five tests pass.
- **Commit:** `feat(app): the console says when it cannot read a waiting screen (#199)`

### Task 6: `matchblox agents` lists each agent, its version and its unknown screens

- **Files:** `cmd/matchblox/agents.go`, `cmd/matchblox/agents_test.go`, `cmd/matchblox/main.go`, `docs/console.md`, `CHANGELOG.md`.
- **Test first** (on `writeAgents(w, doc, asJSON)` with a built doc):
  - `TestAgentsListsEachAgent`: two Claude Code sessions on 2.3.1 (one `UnknownScreen`), one Cursor → rows `Claude Code 2.3.1 2 1` and `Cursor <v> 1 0`, and the fix line under the table.
  - `TestAgentsSaysNotRead`: a Claude Code with `VersionWhy` → `not read` in VERSION and `Claude Code: the install path names no version` under the table.
  - `TestAgentsCodexHasNoHooks`: a Codex session → `no hooks: screen only` in the last column.
  - `TestAgentsTwoVersionsAreTwoRows`: Claude Code 2.3.1 and 2.4.0 sessions → two rows, never one guessed version.
  - Hostile (anti-goal: no home path, host or user name): sessions with `Name` and `Target` holding the home path, `os.Hostname()` and the user name → the output, table and `--json`, contains none of them. `--json` carries only agent, version, why, sessions, unknown.
  - `TestAgentsIsACommand`: `parseCommand("agents")` is ok; `--help` lists it.
- **Run:** `go test ./cmd/matchblox/ -run Agents` → fails: `writeAgents` undefined. After: passes.
- **Do:** add `agents` to `commands` (main.go:363) and to the `cliRead` case (main.go:202). Group `doc.Sessions` by `(Agent, Version)`; add installed agents from `doc.Agents` with no session as rows with 0. Write the table, or `writeJSON`. Docs: a short section in docs/console.md. CHANGELOG under [Unreleased], `### Added`, headline of 7 words or fewer: `**The console says when it cannot read.**`.
- **Done when:** the six tests pass, and `go test ./cmd/matchblox/` passes (docscheck included).
- **Commit:** `feat(cli): matchblox agents lists each agent and its version (#199)`

### Task 7: the review

- Run `builder:review` on the branch against the spec `docs/superpowers/specs/2026-11-21-unknown-screen-design.md`, with `go vet ./... && go test ./...`.
- A run on the host: start the built binary with HOME and XDG_* in a temp dir (never replace the running service), a tmux test server, a Claude Code-like pane that shows `unknown-wording.txt`, and a hook event for permission. See the row, the chat, the refused digit and `matchblox agents`. Quote what it shows.
- Landing page: the PR body holds `Landing page: unchanged because <reason>` (the page sits at its 480-word cap), checked with `.github/scripts/landing-guard.sh`.
- **Done when:** the review has no open Critical or Important finding.

## Lens findings

- Engineering: the spec's happy path says "permission or question". A question (idle_prompt) waits for typed text, with no menu on screen, so it would always look unread: a false alarm on every question. Changed: Task 3 triggers on `StatePermission` only, and `TestAQuestionIsNeverUnknown` pins it. The spec needs this one-word correction; it is listed under Calls.
- Engineering: the digit block already holds (`menu.Read` gates digits, `choose` re-reads). Changed: Task 4 pins it with tests and closes only the raw `keys:` path, instead of new code in the console.
- Engineering: reading a version per sample costs a readlink per process. Changed: Task 2 caches it with the name, one read per process.
- Design: the note must fit a phone's 44 columns. Changed: Task 5 adds the phone-width test.
- Design: the word `unknown screen` in the state column and the full note in the chat. No change, because the row has room for two words and the chat for the fix, as the spec says.
- Architecture: `Session` gains three fields on the wire (`version`, `version_why`, `unknown_screen`). Old consoles ignore unknown JSON fields; a new console against an old service sees them empty, so no note shows. No change, because that is the safe side.
- Architecture: an agent's tools — the rule never changes the keys; it only withholds them. No change.

## Spec check

| Spec line | Task |
|---|---|
| Done: an unread menu shows `unknown screen · Claude Code <version>` within the grace time, and a digit sends nothing | Task 3, Task 4, Task 5, Task 7 (run on the host) |
| Done: a menu that a rule reads shows its choices and no note, at every sample | Task 3: `TestAReadMenuIsNeverUnknown` |
| Done: `matchblox agents` shows each agent, version, sessions, unknown count, and `not read` with a reason | Task 6 |
| Done: `matchblox agents` output holds no home path, host name or user name | Task 6 hostile test, Task 1 `TestAWhyNamesNoPath` |
| Done: the phone shows the same note | Task 5 (the phone's console is the same TUI), Task 5 phone-width test |
| Anti-goal: a digit reaches a session that shows `unknown screen` | Task 4 |
| Anti-goal: the note on a screen that reads well | Task 3 `TestAReadMenuIsNeverUnknown`, `TestAQuestionIsNeverUnknown` |
| Anti-goal: the note blinks | Task 3 `TestNoBlink` |
| Anti-goal: a guessed version shows as fact | Task 1 hostile table, Task 6 `TestAgentsTwoVersionsAreTwoRows` |
| Anti-goal: home path, host or user name in the output | Task 6 hostile test |
| Anti-goal: reading a version runs an agent binary | Task 1 `TestAVersionRunsNothing` |
| Surface: an agent's tools | Task 1, Task 4 (both carry `Surface:`) |

## Calls

- 2026-11-21: the trigger is a permission wait only (see Lens findings). Decided by: the owner, with the plan approval.
- macOS: the sampler reads processes through procfs only, so on macOS `Exe` gives nothing and the version shows `not read`, with the why `this system gives no process path`. Codex still reads its version from the rollout.
- The plan lives in docs/superpowers/plans/, beside the earlier plans.
