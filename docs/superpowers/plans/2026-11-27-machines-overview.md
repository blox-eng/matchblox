Spec: docs/superpowers/specs/2026-11-27-machines-overview-design.md
Spec approved: 2026-11-27
Test command: go test ./...
Plan approved: 2026-11-27 by the owner
Next: builder:build

# Plan: an owner sees in one look which machine is in trouble, why, and which accounts it runs

What exists (read 2026-11-27):

- `procfs.Host` (`internal/procfs/host.go`) is the sampler's source. `procfs.FS` reads `/proc`. `host.Host` (`internal/host/host.go`) reads macOS through gopsutil. `host_ios.go` is the iOS build. `demoHost` embeds `FS`. No other implementer exists.
- `host.read` sets `Proc.State` to `'R'` or `'S'` only. gopsutil's `convertStatusChar` maps the `ps` state `U` and `D` to `process.Blocked`, and `read` already calls `Status`. So the waiting state costs no new call on macOS.
- `host.Host.Uptime()` returns seconds since the epoch, not since boot (`host.go:171`). Boot time needs its own method.
- `Sampler.Sample` (`internal/sample/sampler.go:466`) calls `sampleMachine` on each sample. It calls `scan` only every `ProcEvery`, or when the panes change. `scan` reads the processes, builds `l.Sessions` (`sampleSessions`, line 533), then calls `sampleProcs` (line 535). `Sample` copies a fixed list of fields from `s.last` into the snapshot (lines 478–479).
- `Session` has `PID` (the agent's pid) and `Pane`. Pane ids can repeat across tmux servers (the main one and the stoker's), and pids cannot repeat on one host.
- The console keeps each peer in `link` (`internal/app/peers.go:33`): `lost`, `silent`, `mismatch`, `blocked`, `dialErr`, `at`, `have`, `st`. `blockedWords` (`internal/app/hostrows.go:212`) owns the reason and fix of an unreached host. It has no case of its own for a lost link.
- `region()` (`internal/app/panels.go:13`) upper-cases its label.
- `machinePanel` (`internal/app/dock.go:470`) draws views, USAGE, then the sections. Machine's selectable rows are views, orphans, devices, hosts. The row base is summed by hand in `machineRows`, `machineIndex`, `machinePick`, `selectedOrphan`, `selectedDevice`, `devicesBody`, `procs` and `hostsBody`. `windowed` (`internal/app/view.go:272`) follows the selected row. The Machine tab calls it at `view.go:230`.
- Fixtures: `hostState`, `sess`, `twoHosts` in `internal/app/merge_test.go`; `machineHosts`, `heads` in `internal/app/tabs_hosts_test.go`; `loadedWith` in `internal/app/app_test.go`; `/proc` fixtures under `testdata/machine/proc`.

## File map

- `internal/host/host.go` (change) — the waiting state on macOS, and boot time from gopsutil.
- `internal/host/host_ios.go` (change) — boot time on iOS: an error.
- `internal/procfs/host.go` (change) — the `Booted` method on the `Host` interface.
- `internal/procfs/proc.go` (change) — `FS.Booted` from the `btime` line of `/proc/stat`.
- `testdata/iowait/proc/` (create) — a `/proc` with a `stat` that has `btime`, and three processes, two in state `D`.
- `internal/sample/sampler.go` (change) — `Machine` gains `Booted` and `IOWait`; `Snapshot` gains `Heavy`; `Sample` copies them.
- `internal/sample/tree.go` (create) — the heaviest process tree of a scan.
- `internal/sample/tree_test.go` (create) — the tests of `tree.go`.
- `internal/sample/procs.go` (change) — `sampleProcs` counts the waiting processes and finds the heaviest tree.
- `internal/app/machines.go` (create) — the `machines · N` region: its rows, order, marks, accounts and words.
- `internal/app/machines_test.go` (create) — the tests of `machines.go` and its keys, and the `fourHosts` fixture.
- `internal/app/dock.go` (change) — `machinePanel` draws the region and records the line of each section head.
- `internal/app/devices.go` (change) — one `rowBase` for Machine's row kinds; ⏎ on a machine row.
- `internal/app/panels.go` (change) — `procs` uses `rowBase`.
- `internal/app/layout.go` (change) — `body` gains `heads`, the line of each host's section head.
- `internal/app/view.go` (change) — the Machine tab's window starts at a jumped-to head.
- `internal/app/tabs_hosts_test.go` (change) — `heads` and `TestMachineTabSectionsInHostsOrder` read only the lines after USAGE.
- `docs/console.md`, `docs/phone.md`, `docs/remote.md` (change) — name the region where they describe Machine.
- `CHANGELOG.md` (change) — one line under the next release.

## Task 1: a macOS process waiting on I/O reads as waiting

- **Files:** `internal/host/host.go`, `internal/host/host_test.go`.
- **Test first:** `TestStateOfStatus`: `stateOf([]string{process.Blocked})` is `'D'`; `stateOf([]string{process.Running})` is `'R'`; `stateOf([]string{process.Sleep})`, `stateOf(nil)` and `stateOf([]string{""})` are `'S'`.
- **Run:** `go test ./internal/host -run TestStateOfStatus`. Before: does not compile, `stateOf` undefined. After: passes.
- **Do:** add `stateOf(st []string) byte`. In `read`, set `pr.State = stateOf(st)` when `Status` has no error, in place of the `Running` check.
- **Done when:** `TestStateOfStatus` passes.
- **Commit:** `feat(host): a macOS process that waits on I/O reads as waiting`

## Task 2: each host knows when it booted

- **Files:** `internal/procfs/host.go`, `internal/procfs/proc.go`, `internal/procfs/proc_test.go`, `internal/host/host.go`, `internal/host/host_ios.go`, `testdata/iowait/proc/stat` (with `cpu` and `cpu0` lines too, so `Stat()` reads it).
- **Test first:** `TestBootedReadsBtime`: `FS` over `testdata/iowait` (the root form that `proc_test.go` uses; `FS` joins `proc` itself) returns `time.Unix(1764000000, 0)` and no error. `FS` over `testdata/machine/proc`, which has no `btime`, returns an error.
- **Run:** `go test ./internal/procfs -run TestBootedReadsBtime`. Before: does not compile. After: passes.
- **Do:** add `Booted() (time.Time, error)` to `procfs.Host`. `FS.Booted` reads `btime` from `stat`. `host.Host.Booted` calls gopsutil `host.BootTime()`. The iOS host returns an error that says boot time is not read on iOS.
- **Done when:** `TestBootedReadsBtime` passes, and `go build ./...`, `GOOS=darwin go vet ./internal/host ./internal/sample` and `GOOS=ios GOARCH=arm64 go vet ./internal/host` pass.
- **Commit:** `feat(procfs): each host knows when it booted`

## Task 3: a snapshot carries the boot time and the count of processes waiting on I/O

- **Files:** `internal/sample/sampler.go`, `internal/sample/procs.go`, `internal/sample/sampler_test.go`, `testdata/iowait/proc/` (the three process dirs).
- **Test first:**
  - `TestMachineCountsIOWait`: a `Sampler` over the root `testdata/iowait`. After one `Sample()`, `*snap.Machine.IOWait == 2` and `snap.Machine.Booted` is `time.Unix(1764000000, 0)`. A second `Sample()` before `ProcEvery` passes still has `IOWait == 2`.
  - `TestOldSnapshotHasNoIOWait`: `json.Unmarshal` of `{"load":[1,1,1]}` into `Machine` leaves `IOWait` nil, and `{"io_wait":0}` gives a pointer to 0.
- **Run:** `go test ./internal/sample -run 'IOWait'`. Before: does not compile. After: passes.
- **Do:** add `Booted time.Time` (`json:"booted,omitzero"`) and `IOWait *int` (`json:"io_wait,omitempty"`) to `Machine`. In `sampleProcs`, count `State == 'D'` and store it in a new `Sampler.ioWait *int` field. When `Procs()` fails in `scan`, set `s.ioWait = nil`. `Sample` copies `s.ioWait` into `snap.Machine.IOWait` after `scan`, so a failed read shows `unknown`, never `0`. Add `TestIOWaitIsNilWhenProcsFail`: a sampler over a root with no `proc` dir has `IOWait == nil`. `sampleMachine` sets `Booted` from `s.FS.Booted()`. On error it leaves `Booted` zero and adds `boot time: <err>` to `snap.Errors`. `proto.Version` stays 1. No extra `Procs()` read.
- **Done when:** both tests pass.
- **Commit:** `feat(sample): a snapshot counts the processes that wait on I/O`

## Task 4: a snapshot names the process tree that adds the most load

- **Files:** `internal/sample/tree.go`, `internal/sample/tree_test.go`, `internal/sample/procs.go`, `internal/sample/sampler.go`.
- **Test first**, on `heaviest(procs map[int]procfs.Proc, cpu map[int]float64, panes []Pane, agents map[int]bool, argv func(int) []string) *Tree`:
  - `TestHeaviestCutsUnderThePane`: pane pid 10 (`zsh`) → 11 `claude` (an agent) → 12 `bash` with argv `bash -c "xargs …"` → 13 `xargs` → 16 `gcloud`, each with 15 `python` children at 7% cpu. Wants `PID 13`, `Comm "xargs"`, `Next "gcloud"`, `Procs 257` (xargs, 16 gcloud, 240 python), `CPU` = the sum, `Agent 11`.
  - `TestHeaviestOutsideAPane`: 13 `xargs` and its tree, with parent pid 1, and no pane above it. Wants `PID 13`, `Agent 0`. A second case with parent `systemd` (pid 900, under pid 1) gives the same.
  - `TestHeaviestKeepsPanesOutOfTheirServer`: pid 1 → 500 `tmux` (the server) → pane pid 10 → the tree of the first test. Wants the tree under the pane (`PID 13`, `Agent 11`), not a tree rooted at `tmux`.
  - `TestHeaviestUnderAStokerPane`: the tree of the first test sits under a pane of the stoker's server (`Pane.Server` set). Wants `PID 13`, `Agent 11`, not a tree rooted at pid 1.
  - `TestHeaviestPicksTheMostCPU`: two trees, one with 3 processes at 90% and one with 40 at 1%. Wants the 90% tree.
  - `TestHeaviestWithNoLoad`: every process at 0% cpu. Wants nil.
  - `BenchmarkHeaviest`: 3,000 processes in 50 trees.
- **Run:** `go test ./internal/sample -run Heaviest -bench Heaviest`. Before: does not compile. After: passes. The benchmark is under 2 ms an op on the build host, recorded in the commit body.
- **Do:** each process belongs to the nearest pane pid among its ancestors, or to no pane. A tree's root is the first process under its pane pid that is not an agent (`agents` holds each `Session.PID` of the scan) and not a shell (`sh`, `bash`, `zsh`, `dash`) whose argv holds `-c`. A process in no pane has its root at the first process under pid 1 or under the user's service manager (`systemd`, `launchd`), skipping the same wrappers. The tmux server and each pane pid are never roots. Sum `cpu` and count the processes under each root. `Next` is the `Comm` of the root's child with the most processes under it. `Agent` is the nearest agent pid among the root's ancestors, or 0. Return `*Tree{PID, Comm, Next, Procs, CPU, Agent}`. `scan` passes `all` (the main and the stoker's panes) to `heaviest`, not `panes`. The tmux server is the parent of a pane pid; `heaviest` marks each such parent and never roots a tree there. In `sampleProcs`, call it with the scan's sessions and `s.FS.Argv`, and set `l.Heavy`. Add `Heavy *Tree` (`json:"heavy,omitempty"`) to `Snapshot`, and copy `l.Heavy` in `Sample` with the other fields.
- **Done when:** the six tests pass, the benchmark is under 2 ms an op, and `TestSampleCarriesHeavy` passes: two `Sample()` calls over a fixture with a busy tree, with the tree's ticks raised and `Now` moved on by `ProcEvery` between them. The second snapshot has a non-nil `Heavy`.
- **Commit:** `feat(sample): a snapshot names the process tree that adds the most load`

## Task 5: the machines region draws each host as one row, trouble first

- **Files:** `internal/app/machines.go`, `internal/app/machines_test.go`.
- **Surface:** personal data.
- **Test first**, on `m.machinesBody(w int) body` at width 104. `fourHosts(t)` builds the home host and the peers `gpu-1`, `box-3`, `ws-1`, in that Hosts order, from `twoHosts`'s pattern.
  - `TestMachinesRowsInTroubleOrder`: home healthy, `gpu-1` lost, `box-3` at load 50 on 16 cores, `ws-1` at load 4 on 32 cores with memory 95%. The row order is `gpu-1`, `box-3`, `ws-1`, home. The region head reads `MACHINES · 4`.
  - `TestMachineRowShowsLoadAgainstCores`: the `box-3` row holds `load 50 / 16 cores` and `!`, both styled with `st.neg`. The home row has no mark and no neg style.
  - `TestMachineRowCauseNamesTheSession`: `box-3` has `Heavy{Comm "xargs", Next "gcloud", Procs 257, Agent 11}` and a session with `PID 11` named `fetch-laws`. The line under the row holds `xargs → gcloud`, `257 processes` and `session fetch-laws`. With `Agent 0` it holds `no session`. On a host at or under its cores, there is no cause line.
  - `TestLostMachineShowsNoFigures`: the lost `gpu-1` row holds `not reached since 14:02`, and no `load`, `cpu` or `mem`. Its line under it holds the `why` of `blockedWords(blockedHost{host: "gpu-1", err: cmp.Or(l.blocked, l.dialErr)})` and `run: matchblox connect gpu-1`. A second case: lost with `l.blocked = remote.ErrHostKeyChanged` shows `host key changed: this can be an attack` and `try again`.
  - `TestSilentAndOtherVersionRows`: a silent host and a mismatched host each show `✗`, no figures, and the `why` and fix of `blockedWords` for them.
  - `TestOlderServeSaysUnknown`: a peer whose `Machine.IOWait` is nil and `Booted` is zero, at load over its cores with no `Heavy`. Its row holds `io wait unknown` and `up unknown`, the cause line says `cause unknown`, and the line `ws-1 runs an older matchblox · on ws-1: matchblox update`. Its row never holds `io wait 0`.
  - `TestNewServeOverCoresWithNoTree`: a new serve (IOWait set) over its cores with `Heavy` nil. With `IOWait` 5 the cause line reads `no busy process tree · the load is I/O wait`. With `IOWait` 0 it reads `cause not sampled yet`.
  - `TestNewServeWithoutBootSaysWhy`: a peer with `IOWait` set and `Booted` zero and `Errors` holding `boot time: no btime` shows `up unknown` and the line `ws-1 did not read its boot time: no btime`, and no update line.
  - `TestMachineRowAccountsHideTheEmail` (hostile input): `Limits` with `Account "attacker@example.com · Max"` (anthropic), `Account "x@example.com · team · Pro"` (openai) and `Account "x@example.com"` (cursor). The row holds `Claude Max`, `OpenAI Pro` and `Cursor`, and no `@` anywhere in the region. A fourth account with `Provider "newco"` shows `newco`.
  - `TestAccountsFoldWhenTheRowIsFull`: at width 104, a host with every figure and three accounts. When the accounts do not fit on the row, they go on their own line under it, and every account word shows whole.
  - `TestMachineRowNoPlanSessions`: a host with no `Limits` shows `no plan sessions`.
- **Run:** `go test ./internal/app -run 'Machine(s|Row)|LostMachine|SilentAnd|OlderServe|NewServeWithout'`. Before: does not compile. After: passes.
- **Do:** `machinesBody` gives `m.region(fmt.Sprintf("machines · %d", n), w)`, then one row for each host in `machineOrder()`, then the cause line or the problem line under it. A reached host's row: mark, name, `up <age>`, `load L / C cores` (C = `len(Cores)`), `cpu`, `mem`, the worst `disk`, `io wait`, the accounts. A host's data comes from `viewOf(host)`. Its state comes from its `link` or the home fields: lost (`l.lost`), never reached (`!l.have` with `l.dialErr` or `l.blocked`), silent, mismatch. The problem line uses `blockedWords` with `err: cmp.Or(l.blocked, l.dialErr)` for a lost or never-reached host: its `why`, then its fix, where the fix `connect` reads `run: matchblox connect <host>`, and every other fix reads as `blockedWords` gives it. `accounts(st)` maps each `limits.Account` to `cmp.Or(providerNames[Provider], Provider)` plus the part after the last ` · `, or the provider word alone with no ` · `. Memory and disk at 90% or more draw in `st.neg`. `machineOrder` sorts by group (`✗`, over cores, memory or disk at 90% or more, the rest), then Hosts order.
- **Done when:** the twelve tests pass.
- **Commit:** `feat(app): the machines region shows each host in one row, trouble first`

## Task 6: Machine opens with the region, and its rows are selectable

- **Files:** `internal/app/dock.go`, `internal/app/devices.go`, `internal/app/panels.go`, `internal/app/machines_test.go`, `internal/app/tabs_hosts_test.go`.
- **Test first:**
  - `TestMachineOpensWithTheRegion`: with `twoHosts`, `MACHINES · 2` and the two rows come after the view rows of `machinePanel`, and `USAGE` comes after them.
  - `TestOneHostMachineHasOneRow`: with one host, the region shows `MACHINES · 1` and one row.
  - `TestOneBlockedHostHasNoRegion`: one host, never reached: the blocked screen shows, and no `MACHINES ·` line.
  - `TestDownFromTheViewsReachesTheMachines`: on Machine, `↓` from the last view row selects the first machine row. `↓` again selects the second.
  - The existing tests `TestMachineOpensGitHistoryAndPanes`, `TestAHostRowOnMachineOpensHosts` and `TestOneHostMachineTabHasNoSections` still pass. `heads` in `tabs_hosts_test.go` skips the lines of the `MACHINES` region (from its head to the first blank line after it), so History, which has no region, reads as today. `TestMachineTabSectionsInHostsOrder` counts the lines whose text is ` MACHINE` alone or starts with ` MACHINE ` (the sections), not the region head. With that change, it passes with its `want` unchanged.
- **Run:** `go test ./internal/app -run 'Machine|HostRow'`. Before: the new tests fail (no region). After: all pass.
- **Do:** `machinePanel` adds `machinesBody` after `viewsBody`, for one host too, but not when the console shows the blocked screen (`m.hostCount() == 1 && !m.have`). Add `rowBase(kind int) int` with the kinds `rowViews`, `rowMachines`, `rowOrphans`, `rowDevices`, `rowHosts`, in that screen order. Replace the hand sums in `machineRows`, `machineIndex`, `machinePick`, `selectedOrphan`, `selectedDevice`, `devicesBody`, `procs` and `hostsBody` with it. Add the fields `machineHost string` and `machineHostPicked bool` for the picked machine row. `machinePick` sets them.
- **Done when:** the new tests and the four existing tests pass.
- **Commit:** `feat(app): Machine opens with every machine in the first lines`

## Task 7: Enter on a machine row moves to that host's section

- **Files:** `internal/app/layout.go`, `internal/app/dock.go`, `internal/app/devices.go`, `internal/app/view.go`, `internal/app/machines_test.go`.
- **Test first:**
  - `TestEnterOnAMachineMovesToItsSection`: three hosts, a window of 20 lines. `⏎` on the `ws-1` row: the first line that the Machine tab draws is the `ws-1` section head. The selection stays on the `ws-1` row.
  - `TestAKeyAfterTheJumpFollowsTheSelectionAgain`: after that `⏎`, `↓` moves the selection, and the window follows it as before.
  - `TestEnterOnOneHostMovesToMachine`: one host, `⏎` on its row puts the ` MACHINE` line first.
  - `TestEnterOnAnUnreachedHostMovesToItsHostRow`: `⏎` on a host that was never reached (no state, so no section) selects its row in the hosts list (`hostPicked`, `hostPick`). With `opt.Layered` false, `⏎` changes nothing.
- **Run:** `go test ./internal/app -run 'TestEnterOn|TestAKeyAfterTheJump'`. Before: fail, the window does not move. After: pass.
- **Do:** `body` gains `heads map[string]int`, the line of each host's section head, keyed by `cmp.Or(host, m.opt.Host)` (the ` MACHINE` line, under the key `m.opt.Host`, for one host). `machinePanel` fills it. `⏎` on a machine row sets the field `machineJump` to that host, when it has a head. Otherwise, it picks the host's hosts-list row when `hostRows()` holds it. With no hosts list (a console that is not layered), ⏎ does nothing, and the row's fix line already names the fix. Every other key clears `machineJump`. At `view.go:230`, when the tab is Machine and `machineJump` has a head in `b.heads`, the window starts at that line. Otherwise `windowed` runs as today. `windowStart` does not change, so no other tab changes.
- **Done when:** the four tests pass.
- **Commit:** `feat(app): Enter on a machine moves to its section`

## Task 8: the docs name the machines region

- **Files:** `docs/console.md`, `docs/phone.md`, `docs/remote.md`, `CHANGELOG.md`.
- **Test first:** the docs check in `internal/docscheck`.
- **Run:** `go test ./internal/docscheck`. Before and after: passes.
- **Do:** where each doc describes Machine, add the region: one row for each host, trouble first, the cause line, the provider and plan words without the account name, and `⏎` to the section. Add a line to `CHANGELOG.md` under the next release, in the form the file uses.
- **Done when:** `rg -n -i "machines ·" docs/console.md docs/phone.md docs/remote.md` finds a line in each file, and `go test ./internal/docscheck` passes.
- **Commit:** `docs: Machine names the machines region`

## Task 9: review the whole change

- **Files:** `docs/superpowers/plans/2026-11-27-machines-overview.md` (the record below).
- **Do:** load `builder:review` on the branch against `origin/main`, with the spec and `go test ./...`. Then run the checks that no unit test can see, and write each result and its output under "Manual checks" at the end of this plan:
  1. On this host, from a session, `seq $(( $(nproc) + 4 )) | xargs -P $(( $(nproc) + 4 )) -I{} sh -c 'while :; do :; done'` for 3 minutes. The row turns neg, shows `load N / C cores`, and the cause line names `xargs` and that session. Then kill it.
  2. Run the same command as `setsid -f sh -c '<the command>'`, so its parent is init. The cause line says `no session`.
  3. Stop serve on a macOS peer. Its row loses its figures at once, and its words match its Sessions row.
  4. Before that peer updates, its older serve shows `io wait unknown` and the update line. When no peer runs an older serve, record "not run" and the reason.
  5. Measure serve's cpu on this idle host for 5 minutes on `origin/main` and on the branch (`pidstat -p <serve pid> 60 5`, or `ps -o %cpu` once a minute when `pidstat` is missing). The branch rises by less than 1 percentage point.
  6. At width 60, the region's rows fold and cut no figure.
  7. `.github/scripts/landing-guard.sh "<title>" <body-file> origin/main` passes.
- **Done when:** the review has no open Critical or Important finding, and each manual check has its result and output in the plan.
- **Commit:** `docs: record the manual checks of the machines region`, and `fix(...)` commits for findings.

## Lens findings

- Engineering: the new fields keep older consoles and serves working only if a missing field differs from zero. Changed: Task 3 uses a pointer for `IOWait` and `omitzero` for `Booted`, and tests both decodes.
- Engineering: processes are read only in `scan`. An io-wait count read in `sampleMachine` would add a `Procs()` read on every sample. Changed: Task 3 counts in `sampleProcs`, and `Sample` copies the count.
- Engineering: `Sample` copies a fixed list of fields from `s.last`. Changed: Task 4 copies `Heavy` there, and `TestSampleCarriesHeavy` proves it.
- Engineering: the tree walk runs on every scan over every process. Changed: Task 4 adds a benchmark with a 2 ms budget, and Task 9 measures serve's cpu.
- Engineering: macOS `Status` already shells out to `ps` once for each process in each scan. That cost is not this change's. No change, because Task 1 reuses the call and adds none, and Task 9 check 5 measures the total.
- Engineering: Machine's row base is summed by hand in eight places. Changed: Task 6 adds one `rowBase(kind)` and replaces each sum.
- Correctness: `TMUX_PANE` names a pane id that can repeat on the stoker's tmux server, so the cause line could name the wrong session. Changed: Task 4 names the session by the agent pid among the tree's ancestors, which is unique on a host.
- Design: success is quiet. A healthy row has no mark and no color. Only a trouble figure draws in neg. No change, because Task 5 tests it.
- Design: making section heads selectable rows would put them out of screen order, because home's orphans sit between the heads. Changed: Task 7 moves the window with `machineJump` and leaves the selection on the machine row.
- Design: a narrow terminal must fold the row, not cut a figure. Changed: Task 9 check 6.
- Architecture: the snapshot is the protocol between serve and console, and the iOS app decodes it too. Swift's `Codable` ignores unknown keys. No change, because `proto.Version` stays 1 and every new field is optional.

## Check against the spec

| Spec line | Task |
|---|---|
| Done: two hosts, `machines · 2` above USAGE, trouble order | Task 6: Machine opens with the region; Task 5: rows in trouble order |
| Done: busy `xargs` from a session turns neg in 3 minutes, cause names `xargs` and the session | Task 4: heaviest tree; Task 5: cause names the session; Task 9 check 1 |
| Done: the same tree outside tmux says `no session` | Task 4: outside a pane; Task 5: `no session`; Task 9 check 2 |
| Done: a stopped peer says `not reached since`, the fix, no figures | Task 5: lost machine shows no figures; Task 9 check 3 |
| Done: an older serve shows `io wait unknown` and the update line | Task 3: old snapshot has no IOWait; Task 5: older serve says unknown; Task 9 check 4 |
| Done: a lost peer loses its figures at once, its words match Sessions | Task 5: lost machine shows no figures; Task 9 check 3 |
| Done: serve's idle cpu rises by less than 1 point | Task 4: benchmark; Task 9 check 5 |
| Done: io-wait is right on Linux and macOS | Task 1: macOS state; Task 3: Linux count |
| Done: providers and plans, no account name, `no plan sessions` | Task 5: accounts hide the email; no plan sessions |
| Done: Enter moves to the section or the hosts-list row | Task 7: Enter on a machine |
| Done: the three docs name the region | Task 8: the docs |
| Experience: a figure that cannot be read says `unknown` and the reason | Task 5: older serve says unknown; new serve without boot says why |
| Experience: a single unreached host keeps the blocked screen | Task 6: one blocked host has no region |
| Anti-goal: an unreached host looks live | Task 5: lost machine shows no figures |
| Anti-goal: an unknown figure shows as 0 | Task 3: old snapshot; Task 5: older serve says unknown |
| Anti-goal: the cause names the wrong session | Task 4: cuts under the pane, keeps panes out of their server; Task 5: cause names the session |
| Anti-goal: a host in trouble sits below a healthy host | Task 5: rows in trouble order |
| Anti-goal: tree sampling makes serve a load | Task 4: benchmark; Task 9 check 5 |
| Anti-goal: an account name shows in the region | Task 5: accounts hide the email (hostile input) |
| Surface: personal data | Task 5 |

## Calls

- The landing page: recommend the line `Landing page: unchanged because the region lives inside Machine, which the landing page does not show`, because `www/` shows the console's sessions, not Machine. Checked in Task 9 check 7. Cost if wrong: CI's `Landing page` step blocks the merge until `www/` changes.
- The fix word `connect` from `blockedWords` reads `run: matchblox connect <host>` in the region, because the region has no ⏎ that runs connect. The reason and every other fix are word for word. Cost if wrong: one wording differs from the Sessions row.
- The spec's example row says `claude Max · codex Pro`, but its approved rule says `providerNames`. The build follows the rule (`Claude Max · OpenAI Pro`), and the spec example is fixed to match.

## Cold read

Cold read: 2026-11-27, 1 fresh reader. Blocker 3, major 10, nit 8. Fixed 20, contested 0, for the person 0, declined 1. Second read: 14 left (blocker 1, major 8, nit 5), each given to the owner with a recommendation.

- Declined (nit): "gopsutil's `U`→`Blocked` mapping could not be verified". The reader could not open the module cache. The mapping is `convertStatusChar` in `github.com/shirou/gopsutil/v4/process/process.go`, read in this session: `case "D", "U": return Blocked`.

## Changes during build

- 2026-11-27, Task 1: `stateOf` also maps `process.Zombie` to `'Z'`, with a sixth test case. Because the Task 1 checker found that a macOS zombie read as `'S'`, so `sampler.go` and `lineage.go`, which skip only `Z` and `X`, counted an exited agent as alive. The bug was older than Task 1, but it sits in the function Task 1 wrote. — the builder.
- 2026-11-27, Task 1: `TestStateOfStatus` sits in `host_test.go`, which has no `!ios` tag, while `stateOf` is in the `!ios` file. No change, because CI never runs Go tests for iOS (only `go build ./cmd/mbdemolib`). — the builder.
- 2026-11-27, Task 2: the test reads `FS{Root: "../../testdata/iowait/proc"}`, not `testdata/iowait`. Because `FS.Root` is the `/proc` directory itself (`proc_test.go`'s `fixture` is `testdata/machine/proc`); `FS` does not join `proc`. — the builder.
- 2026-11-27, Task 2: `GOOS=ios GOARCH=arm64 go vet ./internal/host` cannot run here: ios/arm64 needs cgo linking and there is no clang. `GOOS=darwin GOARCH=arm64 go build -tags ios ./internal/host` passes in its place; vet with the tag fails only in purego, pulled in by `host_test.go`. — the builder.
- 2026-11-27, Task 2: the done-when check `GOOS=ios GOARCH=arm64 go vet ./internal/host` needs cgo and clang, which this host lacks. It is replaced by `GOOS=darwin GOARCH=arm64 go build -tags ios ./internal/host`, which passes. — the builder.
- 2026-11-27, Task 3: `testdata/machine/proc/stat` gains a `btime` line, and `TestBootedReadsBtime` takes its no-`btime` case from a temp-dir `stat`. Because a missing boot time now adds `boot time: …` to `snap.Errors`, and `TestSessionsMapToPanes` wants no errors from the machine fixture; a real `/proc/stat` always has `btime`. — the builder.
- 2026-11-27, Task 3: the sampler helper `newFixtureSampler` does join `proc` to its root, as the plan says, so the test passes `../../testdata/iowait`. `testdata/demo/proc/stat` still has no `btime`, so each demo snapshot carries `boot time: no btime in …`, which `matchblox status` prints as an error line; Task 5, which draws boot time, decides the demo's boot time. — the builder.
- 2026-11-27, after Task 3: a host that cannot read its boot time at all (iOS: `errors.ErrUnsupported`) leaves `Booted` zero and adds no error, and the demo's `movingFS` boots its fixture's uptime before the demo starts. Because the Task 3 builder found that the demo and the iOS demo would carry `boot time: …` in `Errors` on every sample, and `matchblox status` prints each as an `error:` line. A failed read on a host that can read it is still an error. — the builder.
- 2026-11-27, after Task 3: `testdata/demo/proc/stat` gets a `btime` line, as a real `/proc/stat` has. Because the Task 3 checker found that `matchblox status --fixtures testdata/demo`, which CONTRIBUTING.md runs, reads a plain `FS` (not `movingFS`) and printed `error: boot time: no btime …`. — the builder.
- 2026-11-27, Task 4: `agents` holds the pid of each of the scan's `Children` as well as each `Session.PID`. Because a child agent sits between its parent's agent and the command it started, and is no more the tree than its parent is. — the builder.
- 2026-11-27, Task 4: an agent or a shell wrapper with no command under it is a tree of its own, and `Agent` is the nearest agent among the root and its ancestors. Because a busy `claude` or a `bash -c 'while :; do :; done'` is the load itself, and a busy agent's own tree names its own session, not `no session`. — the builder.
- 2026-11-27, Task 4: a shell is a wrapper when `-c` sits in any short-flag group before its first operand (`bash -lc`, `sh -e -c`); long options are passed over. Tested by `TestShellWrapper`, a seventh test. Because a login shell running a command string reads `bash -lc …`, which a match on a lone `-c` would miss. — the builder.
- 2026-11-27, Task 4: `TestSampleCarriesHeavy` uses the machine fixture's `node` (pid 210) under pane `%1`'s agent and asserts `Agent` is that session's pid. Because it is the fixture's process under a pane with an agent above it, so the one test also proves `scan` passes the sessions. — the builder.
- 2026-11-27, after Task 4: `Heavy.Agent` is the session at the top of the agent's line (`sessionAbove`), not the nearest agent. Because the Task 4 checker found that a tree started by a child agent (`claude -p` in a session's shell) carried the child's pid, which no session has, so Task 5 would say `no session`. A child line that reaches no session gives 0. — the builder.
- 2026-11-27, after Task 4: shell options that take a value (`-o`, `+o`, `-O`, `+O`, `--rcfile`, `--init-file`) skip it, so `bash -o pipefail -c …` is a wrapper; and `sshd`, `sshd-session` and `login` pass their tree to the login shell. Because the Task 4 checker found that such a CI run was named `bash`, and that every ssh login summed into one `sshd` tree. — the builder.
