# Tailscale SSH gate bypass warn (#185) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When Tailscale SSH is on (`RunSSH: true`), warn builders that paired keys are not gated on the MagicDNS/:22 path, and document the honest fixes.

**Architecture:** Reuse `#184` `pairProbe` prefs injection. Add host-wide `TailscaleSSHOn()` (prefs only) plus a shared warn string. Emit it non-fatally from `serve`, `status`, and `setup`. Docs cover `tailscale set --ssh=false` / OpenSSH address-port. No pair-preflight changes; no connect-time hook (gate never sees the bypass).

**Tech Stack:** Go, existing `testdata/pair` Tailscale fixtures, `SetPairProbe` fakes.

**Spec:** `docs/superpowers/specs/2026-10-22-tailscale-ssh-gate-bypass-design.md` ([issue #185](https://github.com/blox-eng/matchblox/issues/185))

## Global Constraints

- Reuse `pairProbe` / `SetPairProbe` / prefs fixtures; do not reinvent Tailscale probing.
- Probe failure → no warn (skip), never a hard error.
- Do not change `CheckPairPath` / pair refusal strings unless a clear gap remains.
- Do not set `pairDoor.Problem` (blocks door open).
- Do not invent pairing design or weaken ForcedCommand.
- CHANGELOG under `## [Unreleased]` (Added or Fixed as fits — this is a warn + docs; prefer `### Added` or `### Changed` for visibility; use `### Fixed` only if framing as residual of #120). Prefer `### Fixed` with “builders are told…” to match #184 voice, or `### Added` for the warning — use **Added** for the warn surface and docs.
- Branch off latest `origin/main`; PR closes #185.
- Commits: repo style; plan includes commit steps (user asked implement + PR).

## Review Focus

- `RunSSH: false` / no `tailscale` / bad prefs JSON → no warning.
- `status` JSON mode keeps JSON on stdout; warn goes to stderr (like `stokerPaneGone`).
- `status --text` may put the warn on the text writer (same pattern as stoker).
- Detached `serve` still writes the warn where stderr goes (`serve.log` when spawned) — do not require TTY.
- Pair path still refuses only when address matches Self on :22 (`CheckPairPath` unchanged).

---

### Task 1: Host-wide Tailscale SSH detection + warn string

**Files:**
- Modify: `internal/setup/paircheck.go`
- Modify: `internal/setup/paircheck_test.go`

**Interfaces:**
- Consumes: `pairProbe.LookPath`, `pairProbe.TailscalePrefs` (existing)
- Produces:
  - `func TailscaleSSHOn() bool` — true only when prefs parse and `RunSSH` is true; false when tool/prefs missing or fail
  - `const WarnTailscaleSSHGate` — exact copy pinned by tests:
    `"Tailscale SSH is on: paired keys are not gated on this host's MagicDNS name or Tailscale IP on port 22 (tailscaled answers, not OpenSSH). Run tailscale set --ssh=false, or connect on an address and port OpenSSH serves."`

- [ ] **Step 1: Write failing tests** in `paircheck_test.go`

  - `TestTailscaleSSHOnTrue` — prefs fixture `tailscale-prefs-ssh-on.json` → `TailscaleSSHOn()` true
  - `TestTailscaleSSHOnFalse` — `tailscale-prefs-ssh-off.json` → false
  - `TestTailscaleSSHOnNoTool` — LookPath fails → false
  - `TestTailscaleSSHOnBadPrefs` — prefs returns truncated JSON → false
  - `TestWarnTailscaleSSHGateCopy` — const contains `tailscale set --ssh=false`, `MagicDNS` or `port 22`, and `not gated` (or equivalent pinned substring set)

- [ ] **Step 2: Run tests; expect FAIL**

  Run: `go test ./internal/setup/ -run 'TailscaleSSHOn|WarnTailscaleSSHGate' -count=1`

- [ ] **Step 3: Implement `TailscaleSSHOn` + `WarnTailscaleSSHGate` in `paircheck.go`**

  Extract prefs `RunSSH` read shared with `checkTailscaleSSH` if it stays DRY without rewriting pair logic; otherwise duplicate the small prefs parse (YAGNI: a tiny shared `runSSHFromPrefs() (bool, bool)` is fine).

- [ ] **Step 4: Run tests; expect PASS**

  Run: `go test ./internal/setup/ -run 'TailscaleSSHOn|WarnTailscaleSSHGate|CheckPairPath' -count=1`

- [ ] **Step 5: Commit**

```bash
git add internal/setup/paircheck.go internal/setup/paircheck_test.go
git commit -m "$(cat <<'EOF'
feat: detect Tailscale SSH for gate-bypass warnings

EOF
)"
```

---

### Task 2: Emit warn from status, serve, setup

**Files:**
- Modify: `cmd/matchblox/main.go` (`status`, `serve`)
- Modify: `cmd/matchblox/setup.go` (`setupDoors`)
- Modify: `cmd/matchblox/main_test.go` and/or new `cmd/matchblox/tailscale_warn_test.go`
- Touch if needed: `cmd/matchblox/setup` tests in `main_test.go`

**Interfaces:**
- Consumes: `setup.TailscaleSSHOn()`, `setup.WarnTailscaleSSHGate`
- Produces: non-fatal warn lines on those commands

- [ ] **Step 1: Write failing tests**

  - Inject `SetPairProbe` with SSH-on prefs; call `status(..., text=false)` → stderr contains `WarnTailscaleSSHGate`, stdout JSON unchanged (no warn prefix in JSON).
  - Same with `text=true` → warn appears on the text writer (mirror stoker pattern).
  - Probe SSH-off → no warn.
  - `setupDoors` with SSH-on probe → output contains warn before doors (or at least somewhere in output).

- [ ] **Step 2: Run tests; expect FAIL**

  Run: `go test ./cmd/matchblox/ -run 'TailscaleSSH|Status.*Warn|Setup.*Tailscale' -count=1`

- [ ] **Step 3: Implement**

  - Helper in cmd package: `func warnTailscaleSSHGate(w io.Writer)` — if `setup.TailscaleSSHOn()` then `fmt.Fprintln(w, setup.WarnTailscaleSSHGate)` (optionally prefix `matchblox:` to match other stderr; pin in test). Prefer prefix `matchblox: ` for consistency with other CLI lines.
  - Update const if prefix is part of the printed line — either put `matchblox: ` in the const or in the helper; tests pin the full printed form.
  - `status`: after stoker warn, call helper (stderr vs text writer like stoker).
  - `serve`: before `s.Serve`, warn to `os.Stderr`.
  - `setupDoors`: warn to `out` once before listing doors.

- [ ] **Step 4: Tests PASS**

  Run: `go test ./cmd/matchblox/ -run 'Tailscale|Status|SetupCommand' -count=1`

- [ ] **Step 5: Commit**

```bash
git add cmd/matchblox/main.go cmd/matchblox/setup.go cmd/matchblox/*tailscale* cmd/matchblox/main_test.go
git commit -m "$(cat <<'EOF'
feat: warn when Tailscale SSH bypasses the gate

EOF
)"
```

---

### Task 3: Docs + CHANGELOG

**Files:**
- Modify: `docs/phone.md` (after pair Tailscale paragraph ~L44–54)
- Modify: `docs/remote.md` (Keep the key private / devices section)
- Modify: `CHANGELOG.md` Unreleased

**Interfaces:** none

- [ ] **Step 1: Docs** — After the pair-preflight note, add that with Tailscale SSH still on, permanent laptop/phone keys are not gated on MagicDNS/:22; fix with `tailscale set --ssh=false` or connect via an OpenSSH address/port; ACLs can limit Tailscale SSH but do not run ForcedCommand. Mirror a shorter note in `remote.md`.

- [ ] **Step 2: CHANGELOG** under `### Added` (or Fixed if matching residual-#120 voice):

  - Bold headline + prose + `(#185)` link.

- [ ] **Step 3: Commit**

```bash
git add docs/phone.md docs/remote.md CHANGELOG.md
git commit -m "$(cat <<'EOF'
docs: Tailscale SSH bypasses the gate on MagicDNS :22

EOF
)"
```

---

### Task 4: Validate + PR

**Files:** none new

- [ ] **Step 1:** `scripts/validate.sh` (or at least `go test ./internal/setup/ ./cmd/matchblox/ -count=1`)

- [ ] **Step 2:** Push branch; `gh pr create` closing #185

  Title: warn when Tailscale SSH bypasses paired-key gate

  Body: Summary + Test plan; `Closes #185`.

- [ ] **Step 3:** Confirm PR URL
