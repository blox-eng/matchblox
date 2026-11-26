# Cursor limits on any host — implementation plan

Spec: `docs/superpowers/specs/2026-11-25-cursor-limits-any-host-design.md`  
Spec approved: 2026-11-25  
Test command: `go test ./internal/limits/ ./cmd/matchblox/ ./internal/sample/ -count=1`  
Plan approved: 2026-11-25 by the owner  
Next: builder:build

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** With `cursor_usage = true`, Machine → USAGE measures the default Cursor login on Linux (and every OS) from auth files, without requiring macOS Keychain; account folders stay isolated; darwin Keychain still works as a default-login fallback.

**Architecture:** Replace the `dir == ""` → Keychain-only branch with a try-in-order source list. Default login: config/cursor auth.json → ~/.cursor/auth.json → (darwin) Keychain. Account login: only `<folder>/home/.cursor/auth.json`. Same Fetch/API path as today.

**Tech Stack:** Go, existing `internal/limits` + `cmd/matchblox` wiring.

## Global Constraints

- Opt-in remains `[limits] cursor_usage = true`.
- Never log, print, or persist the access/refresh token.
- Account sources never include host `~/.config` or Keychain.
- Files before Keychain on darwin for the default login.

## Review Focus

- Empty `accessToken` in a present JSON file (treat as missing login).
- Both default auth files present with different tokens (first wins).
- `XDG_CONFIG_HOME` set vs unset.
- Flag off: no token function invoked.
- Stale file vs fresh Keychain on darwin (files win by ruling).

## File map

- `internal/limits/cursor_sources.go` (create) — builds and tries default/account token source lists; no API calls.
- `internal/limits/cursor_sources_test.go` (create) — hostile tests for path order and account isolation.
- `internal/limits/cursorusage.go` (change) — `CursorUsages` uses source lists via `Default` (+ keep `File` for accounts); `Reads`/`Reading` no longer require Keychain for default.
- `internal/limits/cursorusage_test.go` (change) — default login tests use `Default` sources, not Keychain-only.
- `cmd/matchblox/main.go` (change) — `cursorUsage` wires `Default` on every OS; Keychain only appended on darwin.
- `cmd/matchblox/main_test.go` (change) — assert linux wiring exposes default sources without Keychain.
- `internal/sample/cursor_test.go` / `limits_test.go` (change) — fixtures that constructed `Keychain:` for default move to `Default:`.
- `docs/limits.md`, `docs/safety.md`, `docs/agents.md`, `docs/console.md` (change) — drop macOS-only for default login; document source lists.

---

## Task 1: Default and account source lists (no API)

**Files:** `internal/limits/cursor_sources.go` (create), `internal/limits/cursor_sources_test.go` (create)

**Surface:** Credentials / login tokens — hostile tests must never require printing the token; assert equality only inside the test process.

**Test first:**

- `TestDefaultCursorSourcesLinuxFilesOnly` — `goos=linux`, home with only `~/.config/cursor/auth.json` containing `accessToken=acc`; `TryTokenSources` returns `acc`. Keychain stub must not be called (`t.Fatal` if invoked); pass a non-nil stub anyway to prove linux never appends it.
- `TestDefaultCursorSourcesPrefersConfigOverDotCursor` — both files present; config path wins.
- `TestDefaultCursorSourcesDarwinFallsBackToKeychain` — `goos=darwin`, no auth files; Keychain returns `from-kc`; result is `from-kc`.
- `TestDefaultCursorSourcesFilesBeforeKeychain` — darwin, file and Keychain both present; file wins; Keychain not called.
- `TestDefaultCursorSourcesDarwinNilKeychain` — `goos=darwin`, `keychain == nil`, no auth files; `TryTokenSources` returns `os.ErrNotExist` (nil Keychain is **not** appended).
- `TestDefaultCursorSourcesEmptyAccessTokenIsMiss` — auth file JSON with `"accessToken":""`; treated as miss → `os.ErrNotExist` when it is the only source.
- `TestDefaultCursorSourcesXDGConfigHome` — set `XDG_CONFIG_HOME` to a temp dir for the test process (or pass config dir explicitly if the helper takes it); token is read from `$XDG_CONFIG_HOME/cursor/auth.json`, not `~/.config/...`.
- `TestAccountCursorSourceOnlyUnderFolder` — account dir `…/work`; only `work/home/.cursor/auth.json` is tried; a host `~/.config/cursor/auth.json` with another token is ignored.

**Run:** `go test ./internal/limits/ -run 'TestDefaultCursorSources|TestAccountCursorSource' -count=1` — fails (symbols missing) then passes.

**Do:**

1. Add `type TokenSource func(context.Context) (string, error)`.
2. Add `DefaultCursorSources(home, goos string, keychain TokenSource) []TokenSource` — builds the default list per spec table. Append Keychain **only** when `goos == "darwin"` **and** `keychain != nil`. Never append a nil source.
3. Config path: if `os.Getenv("XDG_CONFIG_HOME")` is non-empty, use `filepath.Join(that, "cursor", "auth.json")`; else `filepath.Join(home, ".config", "cursor", "auth.json")`. Also always include `filepath.Join(home, ".cursor", "auth.json")` after the config path.
4. Add `AccountCursorSources(folder string, file func(path string) (string, error)) []TokenSource` — single source at `filepath.Join(folder, "home", ".cursor", "auth.json")` via `file`.
5. Add `TryTokenSources(ctx, sources) (token string, err error)` — try in order; `os.ErrNotExist` or empty token → next; first other error returns; if all miss → `os.ErrNotExist`. Empty token from a successful read counts as miss (missing login).

**Done when:** the eight tests pass.

**Commit:** `feat(limits): ordered Cursor token sources for any host`

---

## Task 2: Wire `CursorUsages` to source lists

**Files:** `internal/limits/cursorusage.go`, `internal/limits/cursorusage_test.go`

**Surface:** Credentials / login tokens.

**Test first:**

- `TestReadsDefaultWithoutKeychain` — `Default` non-nil, `Keychain` nil → `Reads("") == true`.
- `TestReadingDefaultUsesFileSource` — temp auth file in a fake home path injected via `Default`; Fetch called with that token; reading measured.
- Update `TestEachCursorAccountHasItsOwnReading` — default uses a `Default` source returning `tok-keychain` (or rename to `tok-default`); accounts still isolated.
- Keep `TestCursorAccountNotSignedInAsksNothing` behaviour.

**Run:** `go test ./internal/limits/ -count=1` — fail on old Keychain assumptions, then pass.

**Do:**

1. Add field `Default []TokenSource` on `CursorUsages`.
2. Change `Reads("")` to `len(u.Default) > 0` (not `Keychain != nil`).
3. In `Reading`, for `dir == ""`, set `c.Token` to `func(ctx) { return TryTokenSources(ctx, u.Default) }`.
4. For account dirs, set `c.Token` to `TryTokenSources` over `AccountCursorSources(dir, u.File)` (or inline the single file source).
5. Remove use of `Keychain` from `Reading`. Either delete the field or keep it unused and delete in this task once tests migrate — prefer **delete** `Keychain` and fix all call sites in this package and sample tests.
6. Ensure empty `accessToken` → signed-out path (via `TryTokenSources`).

**Done when:** `go test ./internal/limits/ -count=1` passes; no production references to `Keychain` remain in `internal/limits`.

**Commit:** `feat(limits): CursorUsages reads default login from source list`

---

## Task 3: Wire `cursorUsage` on every OS + cmd tests

**Files:** `cmd/matchblox/main.go`, `cmd/matchblox/main_test.go`, `internal/sample/cursor_test.go`, `internal/sample/limits_test.go` (and any other `Keychain:` constructors)

**Surface:** Credentials / login tokens.

**Test first:**

- Rewrite `TestAccountUsageRunsOffMac` (same name or rename to `TestCursorUsageReadsDefaultOnEveryGOOS`): for `linux` and `darwin`, `cursorUsage(goos).Reads("") == true`, and account dir `Reads` stays true. (Today this test asserts linux default `false` — that expectation is wrong after this work.)
- Update sample tests that set `Keychain:` to set `Default: []limits.TokenSource{func…}`.

**Run:** `go test ./cmd/matchblox/ ./internal/sample/ -count=1` — fail then pass.

**Do:**

1. Rewrite `cursorUsage(goos)`:
   - Always set `File: limits.FileToken`, `Fetch: …`, `Every: 5m`.
   - `home, _ := os.UserHomeDir()` (same as the rest of `cmd/matchblox`; do not invent `homeDir()` here).
   - Build `keychain` closure only when `goos == "darwin"` (same `security` command + `keychainErr`); otherwise pass `nil`.
   - `u.Default = limits.DefaultCursorSources(home, goos, keychainOrNil)`.
2. Update comment: default login is file sources on every OS; Keychain is darwin fallback.
3. Fix sample tests constructing `CursorUsages`.

**Done when:** `TestAccountUsageRunsOffMac` (or its rename) expects default read on linux and darwin; sample Cursor usage tests pass; `go test ./cmd/matchblox/ ./internal/sample/ ./internal/limits/ -count=1` passes.

**Commit:** `feat(matchblox): enable Cursor usage token sources on all GOOS`

---

## Task 4: Docs match the behaviour

**Files:** `docs/limits.md`, `docs/safety.md`, `docs/agents.md`, `docs/console.md`

**Test first:** none (docs). Done-when is a human grep.

**Do:**

1. Replace “macOS Keychain only” / “works on macOS only” for the default Cursor login with the source list from the spec.
2. Keep the opt-in `cursor_usage = true` and the “token never kept” language.
3. Note account folders still use `<folder>/home/.cursor/auth.json` only.

**Done when:** each of the four files describes the default-login file sources (and Keychain only as a darwin fallback). Grep must find no leftover that claims the default login works on macOS only — including phrases like `(macOS)`, `macOS Keychain` as the sole default source, and `works on macOS only`. Suggested check: `rg -n 'macOS only|works on macOS|\\(macOS\\)|default login.*Keychain|Keychain.*default' docs/limits.md docs/safety.md docs/agents.md docs/console.md` and fix every hit that still describes default login as Keychain-only.

**Commit:** `docs: Cursor plan usage from auth files on any host`

---

## Task 5: Review

**Files:** none new — read the diff of Tasks 1–4.

**Do:**

1. Run `go test ./internal/limits/ ./cmd/matchblox/ ./internal/sample/ -count=1`.
2. Grep the diff for `accessToken`, `refreshToken`, log/format of tokens — none outside `FileToken` / test asserts.
3. Confirm anti-goals: no mix of account and default; flag off still nil `CursorUsage` in sampler setup.
4. Manual check note for the owner (not automated): on this Linux host, with flag on, after service restart, USAGE shows Cursor `mo` within 5 minutes.

**Done when:** no open Critical/Important finding; test command green.

**Commit:** none unless review fixes (then `fix(limits): …` as needed).

---

## Lens findings

**Engineering — scope:** Reuses `FileToken`, `FetchCursorUsage`, `CursorUsage` polling. Minimum is source lists + wiring + docs. No new dependency. Files ≤ 8. No change.

**Engineering — architecture:** Owner of token acquisition stays `internal/limits`; `cmd/matchblox` only builds the default list. Data flow: source → token in memory → Authorization header → drop. Production failure “auth file half-written”: already `FileToken` → ErrNotExist; covered by TryTokenSources miss. No change.

**Engineering — tests:** Map:

```
linux default from ~/.config/cursor/auth.json     — Task 1, 3
config preferred over ~/.cursor/auth.json         — Task 1
darwin Keychain fallback when no files            — Task 1
files before Keychain                             — Task 1
account folder ignores host config + Keychain     — Task 1, 2
Reads("") without Keychain                        — Task 2, 3
flag off → no CursorUsage                         — existing sample test + Task 3 unchanged wiring
empty accessToken                                 — Task 1 TryTokenSources
```

**Engineering — regression:** Account isolation test updated in Task 2; darwin Keychain path kept in DefaultCursorSources. No change.

**Design — every state:** USAGE copy unchanged (limits.md four why-lines). Empty/missing login already has words. Rating: existing copy ≥ 8 for this work. No change, because the person already sees those strings; we only make “measured” reachable on Linux.

**Architecture lens:** not required (no new API/service boundary beyond existing Cursor HTTP call).

## Spec coverage

| Spec line | Task |
|---|---|
| Linux + auth file → measured `mo` (done-when) | Task 1–3; manual note in Task 5 |
| darwin Keychain-only still measures | Task 1, 3 |
| Flag off → no token read | Task 3 (nil CursorUsage) + existing tests |
| Hostile: Reads("") without Keychain | Task 2 |
| Hostile: account list skips host/Keychain | Task 1 |
| Docs: four files | Task 4 |
| Anti-goal: no token leak | Task 5 grep; FileToken unchanged |
| Anti-goal: no forever-unmeasured with file present | Task 1–3 |
| Anti-goal: no call when flag false | Task 3 |
| Anti-goal: no mix of logins | Task 1, 2 |

## Cold read

- 2026-11-25 — fresh `generalPurpose` reader (agent `5b190b30-655e-418d-a8ff-18a6814d26e4`).
- Majors fixed in-doc: rewrite `TestAccountUsageRunsOffMac`; clarify Task 3 asserts; use `os.UserHomeDir()` not `homeDir()`; named tests for empty token and XDG; stronger docs grep; darwin+nil Keychain not appended; fix `~/.config` typo.
- Changed after the read: Task 1 tests/Do, Task 3 tests/Do, Task 4 done-when.
