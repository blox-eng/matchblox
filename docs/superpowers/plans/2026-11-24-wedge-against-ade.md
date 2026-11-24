Spec: design/0021-wedge-against-ade.md
Spec approved: 2026-11-24
Test command: go test ./internal/sample/... ./internal/app/... (each code task); go test $(go list ./... | grep -v /internal/demo$) and go test ./internal/demo (the review)
Plan approved: 2026-11-24 by owner
Next: builder:build

# Plan: prove the wedge; then only the gaps that fit

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A builder sees matchblox’s own light footprint on Machine, reads one defended category line in README, sees the user-first landing in `www/`, then a gap inventory for phone/remote/CLI — with no ADE chase.

**Architecture:** The service already samples the host into `sample.Machine`. Add `SelfRSS` (serve process) there. Machine’s `machineAbout` draws one quiet `matchblox` line; the local console may add its own RSS beside serve when the Machine host is this machine. Landing copy lands in `www/` in the same feat PR (`design/0021-landing-brief.md`).

**Tech Stack:** Go, existing `internal/procfs` + `internal/sample` + `internal/app` (Bubble Tea console).

**Spec:** `design/0021-wedge-against-ade.md`

## Global Constraints

- Calm by default; one meaning per sign (`DESIGN.md` §1).
- Not an ADE: no prompt editor, diff review, Electron, bots platform (`design/0001-v0.1.md`).
- Feat PRs edit `www/` when the story changes (`AGENTS.md`); else `Landing page: unchanged because …`.
- Footprint numbers are measured, with host/OS/date, or not claimed.
- ASD-STE100 for user-facing words.
- Conventional Commits; no WIP; no `--no-verify`.

## Review Focus

1. Older console vs newer serve: missing `self_rss_bytes` must not break Machine (omitempty / zero → no matchblox line or “—”).
2. Fixture / non-Linux sample: SelfRSS read fails → leave 0; Machine stays usable.
3. Remote Machine: show that host’s serve RSS only; never this laptop’s console RSS on a remote host section.
4. Narrow width: the matchblox line still fits via existing `fit`.
5. Copy never says “native TUI” or “GPU terminal” for matchblox.

---

## What the code holds today

- Machine sample: `sample.Machine` in `internal/sample/sampler.go` (~155); filled by `sampleMachine` (~545).
- Process RSS already exists on `procfs.Proc.RSS` via `/proc/<pid>/stat` (`internal/procfs/proc.go`).
- Machine draw: `machineAbout` / `machineLines` in `internal/app/dock.go` (~343); mem line already uses GB.
- Dock test pattern: `TestTheDockSaysTheDisk` in `internal/app/layout_test.go` (~237).
- Docs: `docs/console.md` § Machine (~648); README lead (~23).

## File map

- `internal/sample/sampler.go` (change) — `Machine.SelfRSS`; fill from this process’s RSS.
- `internal/sample/self_rss_test.go` (create) — sampler / Machine SelfRSS tests (or nearest sample test file).
- `internal/procfs/proc.go` (change, if needed) — `Proc(pid)` helper to read one pid’s RSS without listing all.
- `internal/app/dock.go` (change) — `matchblox` line in `machineAbout`.
- `internal/app/dock_self_test.go` or `layout_test.go` (change) — Machine shows serve / console MiB.
- `internal/app/selfrss.go` (create, if needed) — local console RSS reader, injectable for tests.
- `README.md` (change) — defended line + measured footnote.
- `docs/console.md` (change) — Machine names the matchblox line.
- `docs/index.md` (change, if it restates the promise) — same category line.
- `design/0021-landing-brief.md` (create) — landing copy applied in the feat PR under `www/`.
- `scripts/measure-footprint.sh` (create) — dated measurement recipe (or `docs/superpowers/…` note).
- `CHANGELOG.md` (change) — Unreleased.
- `design/0021-part2-gaps.md` (create) — Part 2 inventory (no product code until owner picks).

## Tasks

### Task 1: Serve SelfRSS on the sample

- **Files:** `internal/sample/sampler.go`; `internal/procfs/proc.go` (only if a one-pid read is cleaner); `internal/sample/self_rss_test.go` (create).
- **Test first:** `TestSampleMachineSetsSelfRSS`: with a live `procfs.FS{Root: "/proc"}` on Linux (skip on other OS), `Sample()` (or `sampleMachine` via a small exported/test hook) sets `Machine.SelfRSS` within ±50% of `/proc/self/status` VmRSS×1024, and `SelfRSS > 0`. Second test: when the pid’s `stat` is missing, `SelfRSS` stays 0 and Sample still returns.
- **Run:** `go test ./internal/sample/ -run TestSampleMachineSetsSelfRSS` — before: fail (no field / always 0); after: pass.
- **Do:**
  1. Add `SelfRSS uint64 \`json:"self_rss_bytes,omitempty"\`` to `Machine`.
  2. In `sampleMachine`, set `m.SelfRSS` from this process (`os.Getpid()`) through `procfs` (reuse stat RSS parsing).
  3. Do not scan every process for “matchblox” binaries.
- **Done when:** both tests pass; `go test ./internal/sample/...` green.
- **Commit:** `feat(sample): report the serve process RSS on each snapshot`

### Task 2: Machine draws the matchblox line

- **Files:** `internal/app/dock.go`; `internal/app/layout_test.go` (or `dock_self_test.go`); `internal/app/selfrss.go` if local console RSS needs a seam.
- **Test first:**
  - `TestMachineSaysMatchbloxServe`: snapshot `SelfRSS = 34 << 20`; `machineLines` contain a line with `matchblox` and `34 MiB` (or the project’s existing size word if one fits better — prefer integer MiB for this line).
  - `TestMachineOmitsMatchbloxWhenSelfRSSZero`: `SelfRSS = 0` → no `matchblox` memory claim on that block (or a single quiet absence — pick absence; document in test name).
  - `TestLocalMachineAddsConsoleRSS`: when drawing the local host, with injectable console RSS `38 << 20` and serve `34 << 20`, line names both; when `host` is a remote name, console MiB is absent.
- **Run:** `go test ./internal/app/ -run 'TestMachineSaysMatchblox|TestMachineOmitsMatchblox|TestLocalMachineAddsConsoleRSS'` — before fail; after pass.
- **Do:**
  1. After the `mem` line in `machineAbout`, add one `matchblox` line from `mc.SelfRSS`.
  2. Local console RSS: read only when `host` is this machine; inject via a var/func for tests.
  3. Keep `fit`; use `faint`/`label` tokens like neighbouring rows.
- **Done when:** the three tests pass; `go test ./internal/app/...` green.
- **Commit:** `feat(console): show matchblox RSS on Machine`

### Task 3: Measure script and README wedge

- **Files:** `scripts/measure-footprint.sh` (create); `README.md`; `CHANGELOG.md`.
- **Test first:** no Go test. Check: script exits 0 on Linux and prints serve MiB and (if a console pid given) console MiB with host, kernel, date. README contains the defended line and does not contain `native TUI` / `GPU-rendered` / `GPU terminal` as claims about matchblox.
- **Run:** `bash -n scripts/measure-footprint.sh`; `rg -n 'native TUI|GPU-rendered|GPU terminal' README.md` → no matches claiming matchblox is those.
- **Do:**
  1. Script: sample `matchblox serve` / console RSS (pgrep/exe basename), print measured lines.
  2. README: after the promise, add one builder-facing category line (tmux control plane — not another IDE) and a quiet measured footnote from the script. Do not lead the README with MiB or stack. Landing copy follows `design/0021-landing-brief.md` (user-first; no tech in hero).
  3. CHANGELOG Unreleased: Machine self footprint + README wedge.
- **Done when:** script parses; README has the line + footnote; CHANGELOG updated.
- **Commit:** `docs: defend the light wedge and record measured footprint`

### Task 4: Docs + apply landing brief to `www/`

- **Files:** `docs/console.md`; `docs/index.md` if needed; `design/0021-landing-brief.md`; `www/index.html` (hero sub + section ledes per the brief).
- **Test first:** none. Check: Machine docs name the matchblox RSS line; hero sub matches the brief’s user-first line; first viewport has no stack/MiB/ADE claims; `rg -n 'MiB|Electron|GPU|ADE|native TUI' www/index.html` finds none in hero/marketing copy.
- **Run:** read the files; `bash .github/scripts/landing-guard.sh` will see `www/` changed.
- **Do:** ASD-STE100 docs; apply `design/0021-landing-brief.md` to `www/index.html` (deslop, user-first).
- **Done when:** docs + `www/` match the brief; landing guard can pass via the `www/` diff.
- **Commit:** `docs: Machine footprint and user-first landing copy`

### Task 5: Part 2 gap inventory (no product code)

- **Files:** `design/0021-part2-gaps.md` (create).
- **Test first:** none. Done-when is a written inventory.
- **Do:**
  1. Walk desk vs phone (`docs/phone.md`), remote (`docs/remote.md`, 0017), and CLI (`matchblox -h` vs console acts in `docs/keys.md`).
  2. List only gaps where desk works and phone/remote/CLI lags, each with repro or “unknown — needs spike”.
  3. Cap: recommend at most three fix candidates; do not implement them in this plan.
  4. Non-goals stay listed: bots, Electron, diff review, etc.
- **Done when:** `design/0021-part2-gaps.md` exists with the inventory and explicit “owner picks before any Part 2 build”.
- **Commit:** `docs: inventory phone/remote/CLI gaps after the wedge`

### Task 6: Review the whole change

- **Files:** none new.
- **Do:**
  1. Load `builder:review` on the branch against `origin/main`, with `design/0021-wedge-against-ade.md`.
  2. Run `go test $(go list ./... | grep -v /internal/demo$)` and `go test ./internal/demo`; quote both.
  3. Confirm `www/` changed per the brief, or the PR body has a valid `Landing page: unchanged because …` line; run landing-guard.
  4. Confirm anti-goals: no Electron, no bots surface, copy has no false “native TUI”.
- **Done when:** review has no open Critical or Important on touched surfaces; tests green.
- **Commit:** none (review only).

## Lens findings

### Engineering

- Reuse `procfs` RSS parsing — no new dependency. **Change:** Task 1 uses existing stat RSS.
- Minimum for Part 1 is SelfRSS + Machine line + copy; Part 2 is inventory only. **Change:** cut implementing phone/remote/CLI fixes from this plan (Task 5 only lists them).
- File count stays under eight product files plus docs. **No change.**

### Design

- Every state: SelfRSS 0 → omit line (no “0 MiB” alarm). **Change:** Task 2 `TestMachineOmitsMatchbloxWhenSelfRSSZero`.
- Error: read failure is silent zero, Machine remains useful. **No change** beyond Task 1.
- One job per section: one `matchblox` line, not a card wall. **No change.**

### Architecture

- One owner of serve RSS: the sampler on that host. Console RSS owned by the local console process, never written onto a remote host’s snapshot. **Change:** Task 2 remote case.
- Wire: additive JSON field with omitempty. **No change** (project already evolves snapshots this way).

## Spec coverage

| Spec line | Task |
|---|---|
| Machine names matchblox’s own footprint | Task 2 (data: Task 1) |
| Defended category line on README | Task 3 |
| Landing copy in `www/` (same PR) | Task 4 |
| Measured footnote | Task 3 |
| Selective phone/remote/CLI only as later tickets | Task 5 |
| Anti-goal: no Electron-class RAM to look modern | Task 1–2 keep sampling only; review Task 6 |
| Anti-goal: no false “native” / “GPU terminal” | Task 3 check; Task 6 |
| Anti-goal: no dead ends / no weak gate | Task 5 invents no port; Task 6 |
| Non-goal: bots / ADE chrome | Task 5 + Task 6 |

## Cold read

A builder with only this plan and the repo can implement Part 1 without chat history: files, tests, and commits are named. Part 2 correctly refuses to invent fixes. Open risk: exact MiB formatting helper (`size` today takes float rate) — Task 2 may add a tiny `rssMiB(uint64) string` next to `machineAbout`; that is an allowed call.

## Approaches (from spec)

A messaging-only — rejected. **B wedge then selective inventory** — this plan. C ADE chase — rejected.

## Changes during build

- **2026-11-24 — Multi-host `machineAbout` passes host `h`.** Was `""` under
  `hostHead`. Needed so console RSS is omitted on remote sections. When
  `hostCount > 1`, the MACHINE about line no longer repeats the host name.
- **2026-11-24 — Console RSS does not import `procfs`.** `TestArchBoundaries`
  forbids `internal/app` → `procfs`. `consoleRSSFromProc` reads
  `/proc/self/status` VmRSS instead.
- **2026-11-24 — README host label `ws-1`.** Measured on this machine; private
  hostname blocked by private-terms hook.
