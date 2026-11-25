# Mux extract Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Spec: docs/superpowers/specs/2026-11-25-mux-extract-design.md
Spec approved: 2026-11-25
Test command: go test ./internal/mux ./internal/sample ./internal/service ./internal/app ./internal/panes ./internal/proto ./internal ./cmd/matchblox -count=1
Plan approved: 2026-11-25 by the owner
Next: builder:build

**Goal:** A builder keeps today's native tmux desk while every pane act goes through `internal/mux`.

**Architecture:** One `mux.Mux` backend is one multiplexer socket. `Tmux.Server` is the `-L` label (`""` builder, `matchblox` stoker). `Tmux.Socket` is an optional `-S` path (copy-watch already keys servers by socket path). Mux owns pane-id helpers (`OnServer`, `SplitPane`, argv prefix). **Mux does not import sample** (today it does; Task 1 ends that). Sample, proto, service and app call mux. `internal/remote` hop scripts and `internal/setup`'s install-tmux door stay. No vt wrapper. No herdr.

**Tech Stack:** Go, tmux via `exec` (existing). No new module.

**Spec:** `docs/superpowers/specs/2026-11-25-mux-extract-design.md`

## Global Constraints

- Freeze UX: list, beside, `ctrl+q`, peek, switch, `x` kill, recover, stoker behave as `origin/main` at extract start.
- Drag, wheel and copy-mode stay tmux's. Do not add `charmbracelet/x/vt` or a nested-client redraw of the agent.
- One backend per socket. Pane ids keep today's `@server` suffix (helpers move from `sample` to `mux` in Task 1).
- Type, Keys, Paste, Start, Rename, Focus, Attach, Respawn return argv **steps** the service still runs behind the confirm/`Actions.Do` gate. List, Caller, LastClient, Split (needs the new pane id) may run. Kill returns `[][]string` as `KillSteps` does today.
- Mux never imports `internal/sample`. Sample may import mux.
- Commits: signed Conventional Commits. Never `--no-verify`.
- Landing page: unchanged (refactor, not a feat the builder can see).
- No herdr setting. Console starts with no herdr installed.

## Review Focus

1. **Kill of pane A must not TERM pane B's pid.** Hostile: Kill(`%1`, pidOf`%2`) must not include `kill -TERM` of `%2`. → Task 2, `TestKillStepsTermOnlyThePanePid`.
2. **Start env must not appear in logs or error strings.** Hostile: env `ANTHROPIC_API_KEY=sk-test` in Start; error/log text must not contain `sk-test`. → Task 2, `TestStartErrorOmitsEnvValues`.
3. **No tmux on PATH:** List is empty, not an error (today `TmuxPanesOn` returns `nil, nil`). → Task 1, `TestListWithNoTmuxIsEmpty`.
4. **Stoker socket is not the builder's:** List on `Server:"matchblox"` must not see a pane that exists only on `""`. → Task 1, `TestListIsPerSocket`.
5. **A send/mark test still uses Fake with no tmux.** → Task 9, existing `go test ./internal/service ./internal/mux`.

## File map

- `internal/mux/mux.go` (change) — the Mux interface grows the pane acts.
- `internal/mux/tmux.go` (change) — tmux adapter; `Server` is the socket label.
- `internal/mux/tmux_test.go` (change) — adapter tests against a real tmux when present.
- `internal/mux/muxtest/fake.go` (change) — Fake implements every new method.
- `internal/mux/list.go` (create) — `Pane` and list field set (today's `PaneFormat`).
- `internal/arch_test.go` (change) — `TestPaneActsNameNoTmux` denylist.
- `internal/sample/tmux.go` (change) — `TmuxPanesOn` reads `mux.List`; keeps ParsePanes.
- `internal/proto/session.go` (change) — `KillSteps` delegates to mux.
- `internal/service/mux.go` (change) — one backend per socket (`mux()` / stoker mux).
- `internal/service/lifecycle.go` (change) — new, rename, kill, respawn through mux.
- `internal/service/recover.go` (change) — list/start/kill through mux; still `new-session`, not respawn.
- `internal/service/stokeragent.go` (change) — stoker socket through mux.
- `internal/service/peer.go` (change) — pane/tty lookup through mux.
- `internal/service/copywatch.go` (change) — paste buffers through mux.
- `internal/service/size.go` (change) — window sizes through mux.
- `internal/app/beside.go` (change) — split/focus/respawn through mux; still a real tmux pane, not vt.
- `internal/app/lastclient.go` (change) — last client through mux.
- `internal/panes/*.go` (change) — production Type/Keys argv live in mux; panes keep tests or thin wrappers only mux uses.

## Task 1: List panes per socket

**Files:** `internal/mux/mux.go`, `internal/mux/list.go`, `internal/mux/tmux.go`, `internal/mux/tmux_test.go`, `internal/mux/muxtest/fake.go`

**Surface:** an agent's tools (read: which panes exist)

**Test first:** `TestListWithNoTmuxIsEmpty` — Mux.List with LookPath failing / no server → `len==0, err==nil`.
`TestListIsPerSocket` — two servers; a pane on `""` is absent from `Tmux{Server: sample.StokerServer}.List()`.
`TestListFieldsMatchPaneFormat` — listed pane has ID, Target, Window, PID, Command, Path, Socket, Server (tmux pid), Jump (the `@matchblox-jump` option), Start (unix seconds), Dead.

**Run:** `go test ./internal/mux -count=1 -run 'TestList'`
Before: FAIL undefined List. After: PASS.

**Do:** Stop mux importing sample: copy pane-id helpers and the list format into mux (`OnServer`, `SplitPane`, `-L`/`-S` argv, UTF-8 `-u`). Add to `Mux`:

```go
type Pane struct {
    ID, Target, Window, Command, Path, Socket, ServerPID, TTY, Jump, ServerStart string
    PID int
    Dead bool
}
List() ([]Pane, error)
```

`Tmux.Server` selects `-L`. `Tmux.Socket` selects `-S` when set (copy-watch). List argv is today's `tmux -u [-L server] list-panes -a -F <format>`. Format is today's `PaneFormat` plus `#{pane_tty}` and `#{pane_dead}` at the end (peer needs tty; recover needs dead). No tmux / tmux exit → empty list, nil error. Stub `LookPath` or `Run` on `Tmux` so `TestListWithNoTmuxIsEmpty` runs in CI when tmux is installed.

`sample.OnServer` / `sample.Tmux` become wrappers that call mux, or move call sites in later tasks. After Task 1, mux compiles with no sample import.

**Done when:** the three tests pass.

**Commit:** `refactor(mux): list panes on one socket`

## Task 2: Start, rename, kill, respawn

**Files:** `internal/mux/mux.go`, `internal/mux/tmux.go`, `internal/mux/tmux_test.go`, `internal/mux/muxtest/fake.go`

**Surface:** an agent's tools; secrets

**Test first:**
- `TestKillStepsTermOnlyThePanePid` — `Kill("%1", 99)` argv is `kill-pane -t %1` then `kill -TERM 99`. `Kill("%1", 0)` is pane only.
- `TestStartErrorOmitsEnvValues` — Start with env `ANTHROPIC_API_KEY=sk-test`; any error string omits `sk-test`.
- `TestRespawnKeepsPaneID` — Respawn of `%3` targets `%3` (`respawn-pane`), does not `new-session`.

**Run:** `go test ./internal/mux -count=1 -run 'TestKillSteps|TestStartError|TestRespawn'`
Before: FAIL. After: PASS.

**Do:** Add:

```go
type Start struct {
    Name, Dir string
    Env []string
    Argv []string
    Detached bool
    EmptyExit bool // stoker: remain-on-exit / exit-empty as today
}
Start(Start) [][]string // argv steps for Actions.Do, not an immediate run
Rename(pane, name string) [][]string // today's panes.RenameSteps: send-keys, rename-window, rename-session as needed
Kill(pane string, pid int) [][]string // tmux kill-pane -t id, then kill -TERM pid when pid>0
KillSession(name string) []string
Respawn(pane, dir string, env []string, argv []string) [][]string // respawn-pane -k, env -u as SwitchAction today
```

`Kill("%1", 99)` first step is `tmux kill-pane -t %1` (with `-L` when the pane carries a server), second is `kill -TERM 99`. Recover (Task 6) uses `Start` + `KillSession`, not `Respawn`. Env values may appear on tmux `-e` as today; they must not appear in error strings or logs this package writes.

**Done when:** the three tests pass.

**Commit:** `refactor(mux): start, rename, kill and respawn through mux`

## Task 3: Split, focus, last client, attach, caller

**Files:** `internal/mux/mux.go`, `internal/mux/tmux.go`, `internal/mux/tmux_test.go`, `internal/mux/muxtest/fake.go`

**Test first:**
- `TestSplitReturnsNewPaneID` — Split of a live session returns a `%` id.
- `TestFocusArgvSelectsThePane` — Focus(`%3`) contains `select-pane` and `%3` (and switch/select-window as today).
- `TestCallerReadsTMUX_PANE` — with `TMUX_PANE=%9`, Caller is `%9`.

**Run:** `go test ./internal/mux -count=1 -run 'TestSplit|TestFocus|TestCaller'`
Before: FAIL. After: PASS.

**Do:** Add `Split(target string, argv []string) (pane string, error)` (runs, returns `%` id), `Focus(pane string) [][]string` (today's jump: select-window, select-pane; attach is separate), `LastClient(session string) (tty string, at time.Time, err error)` (`list-clients -t =<session>` as `lastClientArgv` today), `Attach(session string) []string` (`attach-session -t`), `Caller() string` (`$TMUX_PANE`).

**Done when:** the three tests pass.

**Commit:** `refactor(mux): split, focus, last client and caller`

## Task 4: Sample lists through mux

**Files:** `internal/sample/tmux.go`, `internal/sample/tmux_test.go`, `internal/service/mux.go`

**Test first:** existing `TestParsePanes` (or equivalent) still passes. Add `TestTmuxPanesOnUsesMuxList` if none asserts empty-on-missing-tmux: no tmux → nil, nil.

**Run:** `go test ./internal/sample -count=1 -run TmuxPanes`
Before: still pass on old path. After: `TmuxPanesOn` calls mux; tests pass.

**Do:** `TmuxPanesOn(server)` uses `mux.Tmux{Server: server}.List()` and maps to `sample.Pane` + ParsePanes skip rules (jump sessions). Do not change Session JSON.

**Done when:** sample tmux tests pass; `TmuxPanesOn` does not exec `list-panes` itself.

**Commit:** `refactor(sample): list panes through mux`

**Cycle:** mux still imported panes (Type/Keys/HomeEnv) so sample cannot import mux until that import is gone. Task 4 first copies Type/Keys/HomeEnv into mux (`targetArgv`, const `MATCHBLOX_HOME`) and drops the panes import. Two commits: `refactor(mux): type and keys without panes` then the sample subject. Task 9 still deletes leftover panes argv.

## Task 5: KillSteps through mux

**Files:** `internal/proto/session.go`, proto tests if present, `internal/mux` already has Kill

**Surface:** an agent's tools

**Test first:** add `TestKillStepsMatchMuxKill` in `internal/proto`: `KillSteps("%1", 99)` equals `mux.Tmux{}.Kill("%1", 99)`. Service `TestKillEndsTheAgentAndItsPane` still passes.

**Run:** `go test ./internal/proto ./internal/service -count=1 -run 'TestKillStepsMatchMuxKill|TestKillEndsTheAgentAndItsPane'`
Before: proto still names `sample.Tmux`. After: `KillSteps` calls mux.Kill.

**Do:** `KillSteps` becomes a thin wrapper on mux.Kill.

**Done when:** `TestKillStepsMatchMuxKill` passes.

**Commit:** `refactor(proto): kill steps go through mux`

## Task 6: Service life, recover, stoker, peer

**Files:** `internal/service/lifecycle.go`, `internal/service/recover.go`, `internal/service/stokeragent.go`, `internal/service/peer.go`, `internal/service/mux.go`, matching `*_test.go`

**Surface:** an agent's tools; secrets

**Test first:** existing lifecycle/recover/stoker/peer tests. Recover stays `new-session`/`kill-session` (Start+Kill), not Respawn. Stoker uses `Tmux{Server: "matchblox"}`.

**Run:** `go test ./internal/service -count=1 -run 'TestNewSession|TestKill|TestRecover|TestStoker|TestPeer'`
After: PASS. Those files do not contain `"tmux"` except comments.

**Do:** `Service.mux()` stays default builder socket. Add `Service.stokerMux()` for `-L matchblox`. Replace argv builds with mux methods. Peer uses `List()` (now includes TTY). Recover uses `Start` + `KillSession`. Stoker start uses `Start{EmptyExit: true}` and the same extra flags it uses today. Do not change user-facing strings that mention tmux in English.

**Done when:** named tests pass; those four files go through `s.mux()` / `s.stokerMux()`.

**Commit:** `refactor(service): life, recover, stoker and peer use mux`

## Task 7: Copy-watch and window size through mux

**Files:** `internal/mux/mux.go`, `internal/mux/tmux.go`, `internal/mux/muxtest/fake.go`, `internal/service/copywatch.go`, `internal/service/size.go`, tests

**Test first:** existing copywatch and size tests. Add mux methods only as those files need: list/load/drop buffers, list/restore window sizes. Names left to the builder; they must not keep `"tmux"` in copywatch.go/size.go.

**Run:** `go test ./internal/service -count=1 -run 'TestTheWatcherWaitsForAServer|TestTheWatcherLeavesNoSession|TestSize'`
After: PASS.

**Do:** Grow Mux for list/load/drop buffers and list/restore window sizes. Copy-watch keeps `-S <socket path>` via `Tmux{Socket: path}`. Fake records calls.

**Done when:** copywatch.go and size.go name no tmux.

**Commit:** `refactor(service): copy-watch and sizes through mux`

## Task 8: Console beside, jump, last client through mux

**Files:** `internal/app/beside.go`, `internal/app/lastclient.go`, `internal/app/model.go` (jump/attach), `internal/app/stokerdock.go`, `internal/panes/home.go`, tests

**Test first:** existing beside and last-client tests (`TestBeside`, last client). After: beside still `split-window` via mux.Split, still respawns the nested **tmux client** of today's view (`ViewArgv` stays in `internal/remote` — that is today's native pane, not the #504 vt wrapper).

**Run:** `go test ./internal/app -count=1 -run 'Beside|LastClient|Jump'`
After: PASS. `internal/app/beside.go` does not contain `"tmux"`.

**Do:** Those files call mux for split, focus, respawn, last client, attach, kill-session, home binds. Nested-client detect (`cmd == "tmux"`) becomes a mux helper (`ViewCmd` or comparing Split argv), so beside.go need not contain the argv word `"tmux"`. Do not import `charmbracelet/x/vt`. `internal/remote/view.go` `ViewArgv` stays: that is today's real pane, not the #504 vt wrapper. Move zoom/bind/options that those files still issue into mux methods this task names if Split/Focus/Respawn are not enough.

**Done when:** beside tests pass; agent pane is still a real tmux pane.

**Commit:** `refactor(app): beside and jump go through mux`

## Task 9: Denylist and Fake still works

**Files:** `internal/arch_test.go`, `internal/panes/*.go` as needed so Type/Keys argv live in mux

**Test first:** `TestPaneActsNameNoTmux` in `internal/arch_test.go`:
- Scan production `.go` files of `sample`, `panes`, `service`, `app`, `proto`, `advice`, `cmd/matchblox` (not tests, not `internal/mux`, not `internal/setup`, not `internal/remote`, not `internal/demo`, not `internal/tmuxtest`).
- Fail on `sample.Tmux`, `TmuxOn`, `tmuxOut`, `[]string{"tmux"` (argv). Do **not** fail on English copy (`tmux answered`, `waits for recover`) or JSON field names.
- Task 9 also moves leftover production argv in `advice.go` and `cmd/matchblox` (home/attach/graphics) through mux, or the denylist names those files as still-open with a reason in the commit body — default: move them.
- Panes production files must not emit tmux argv; mux does not call panes.

Also run: `go test ./internal/service ./internal/mux -count=1` — Fake still satisfies Mux; paste/send/mark tests pass with no tmux.

**Run:** `go test ./internal -count=1 -run TestPaneActsNameNoTmux` → FAIL until callers moved; after Task 8, PASS.
`go test ./internal/service ./internal/mux -count=1` → PASS throughout.

**Do:** Move leftover argv in `internal/panes` into mux so panes production files meet the denylist. Keep `TestChainAndSendNameNoTmux`.

**Done when:** `TestPaneActsNameNoTmux` passes; Fake tests pass.

**Commit:** `test(mux): pane acts name no tmux outside mux`

## Task 10: No herdr, no vt, native mouse

**Files:** none new unless a test file `internal/mux/extract_guard_test.go`

**Test first:** `TestNoVtDependency` — `go list -m all` / go.mod does not contain `charmbracelet/x/vt` or `creack/pty` added by this work.
`TestConsoleStartsWithoutHerdr` — default mux is tmux; constructing `service.Service` / `mux.Tmux{}` does not look up herdr.

**Run:** `go test ./internal/mux -count=1 -run 'TestNoVt|TestConsoleStartsWithoutHerdr'`
After: PASS.

**Do:** Guards only. If a task added those modules, remove them.

**Done when:** both tests pass.

**Commit:** `test(mux): no vt wrapper and no herdr required`

## Task 11: Review the extract

**Files:** none (reviewer reads the branch)

**Do:** Run `builder:review` on the branch vs `origin/main`. Spec: `docs/superpowers/specs/2026-11-25-mux-extract-design.md`. Test command: the header's Test command, plus `go test ./internal/demo -count=1 -timeout 20m` once. Confirm drag/wheel still belong to tmux (beside is a split, not vt). No open Critical or Important.

**Done when:** review gate passes.

**Commit:** none (review only). If the review finds a fix, that fix is a new commit, not this task.

## Lens findings

**Engineering — files:** more than eight files. No cut, because the spec is the remaining tmux callers. Chrome, herdr, control mode and the vt wrapper stay cut.

**Engineering — reuse:** keep `PaneFormat` field order, `KillSteps` behaviour, `TmuxPanesOn` empty-on-missing-tmux, `@server` pane ids, `ViewArgv` nested client (today's beside).

**Engineering — failure in production:** tmux missing → empty list (Task 1). Kill wrong pid → Task 2 hostile. Start env in errors → Task 2 hostile. Recover still new-session → Task 6.

**Design:** no new screens. Existing notices stay (`the agent pane did not open:`, `waits for recover`). No change, because the spec freezes copy.

**Architecture:** contract is `internal/mux/mux.go` next to the adapter. Guard is `TestPaneActsNameNoTmux`. Mux owns pane acts; sample owns Session; service owns matchblox acts; remote owns SSH hops. Kill/Start are dangerous pane tools: hostile tests in Task 2. No new HTTP API, no new store.

**Test map**

- list empty without tmux — Task 1
- list per socket (stoker vs builder) — Task 1
- list fields = today's format — Task 1
- kill TERM only that pid — Task 2
- start error omits env — Task 2
- respawn keeps pane id — Task 2
- recover uses start+kill not respawn — Task 6
- beside still real split — Task 8
- denylist — Task 9
- Fake without tmux — Task 9
- no vt / no herdr — Task 10

## Spec map

| Spec line | Task |
|---|---|
| List, beside, ctrl+q, peek, switch, x kill, recover, stoker as origin/main | Tasks 4–8 (peek is CLI, not mux; unchanged) |
| Drag, wheel, copy-mode belong to tmux, not vt | Tasks 8, 10 |
| TestPaneActsNameNoTmux denylist | Task 9 |
| Fake still runs chain/send/paste/mark | Task 9 |
| No herdr installed, console starts | Task 10 |
| Anti-goal: lose native select/wheel/fidelity | Tasks 8, 10 |
| Anti-goal: session missing with no reason | Task 1 empty list; freeze sample behaviour |
| Anti-goal: key/send wrong pane | Task 2 kill pid; existing send tests |
| Anti-goal: secrets in logs/argv | Task 2 `TestStartErrorOmitsEnvValues` |
| Anti-goal: kill/respawn without asking | Task 2; recover still explicit Start+Kill |
| Surface: agent's tools | Tasks 1, 2, 5, 6, 8 |
| Surface: secrets | Task 2 |

## Calls

- Peek stays CLI (`matchblox peek`); not a mux method. Second cold-read of the spec flagged this; the plan follows the code.
- `ViewArgv` nested tmux client stays: that is today's beside, not the #504 vt wrapper.
- Extra beside/home tmux (bind, zoom, options) move with Task 8.
- Recover does **not** use Respawn (spec table said “restart”; the code is new-session + kill-session). Owner freeze UX wins.
- `cmd/matchblox` and `advice` pane argv move in Task 9 so the denylist can pass.

## Changes during build

- 2026-11-25 — Task 4 blocked: sample cannot import mux while mux imports panes (mux → panes → sample). Ruling: Task 4 first copies Type/Keys/HomeEnv into mux and drops the panes import, two commits, then TmuxPanesOn. Task 9 still deletes leftover panes argv. Cost if wrong: short duplicate of send-keys argv until Task 9.

## Cold read

Cold read: 2026-11-25, 1 fresh reader. Blocker 5, major 15, nit 3. Fixed 20, contested 0, for the person 0, declined 0. Second read: 7 left.

First read: broke sample↔mux import cycle; KillSession; Start as steps; TTY/Dead; denylist argv vs English copy.

Second read (not fixed; builder settles at the task, unless you lock them now): TmuxPanesOn bytes vs List; Rename agent/`one`; Start env-u and stoker `-f`; has-session / pane_id after new-session; Server vs `@server` suffix; copy-watch reads vs Load; denylist vs `lists.go` column `"tmux"`; LastClient timestamps; doors.GuideArgv.
