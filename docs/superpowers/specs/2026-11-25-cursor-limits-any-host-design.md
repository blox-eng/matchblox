# An owner on any host sees Cursor plan usage in Machine

## Who, and the moment

An owner opens Machine → USAGE on a Linux (or any non-macOS) host where Cursor sessions already run. They want the Cursor line to show the month's included usage the way Anthropic already does — not stuck on unmeasured with a misleading "turn on cursor_usage" line when the flag is already on.

Evidence: live capture `~/.local/state/matchblox/evidence/2026-11-25-cursor-codex-gaps/` (2026-11-25). Default Cursor login on that host; token present in `~/.config/cursor/auth.json` (`accessToken`); `~/.cursor/auth.json` absent; matchblox only wires Keychain for the default login on darwin (`cmd/matchblox/main.go` `cursorUsage`). `[limits] cursor_usage = true` was already set on the host and still left Cursor unmeasured.

## Why it matters

Without figures, the owner cannot see when Cursor is about to hit the month cap on the box that actually runs the agents. Estimated: every check of Machine on a Linux host is a dead end for Cursor. Measured: Cursor provider stays `not measured` while Anthropic is measured on the same USAGE view.

## The experience

1. Owner has `cursor_usage = true` under `[limits]` (unchanged opt-in; see safety.md).
2. USAGE lists Cursor the same way it does today when a Cursor session is (or was) sampled — this work does not change how long an account stays on the list.
3. Machine → USAGE shows the Cursor account with `mo` (share of the month's included usage), same shape as today on macOS when Keychain works.
4. Token is read only when a call is due, sent only to Cursor's usage API, then dropped — same as today.

### Wrong paths (words the person sees)

Existing USAGE / limits copy (see `docs/limits.md`): with the flag on, one of `signing in` · `no Cursor login token read yet` · `Cursor's login token could not be read` · `Cursor's API has not answered yet`. With the flag off, the fix line still says to set `cursor_usage = true`.

| What happened | What they see | Fix |
|---|---|---|
| Flag off | fix line for `cursor_usage = true` | Turn the flag on |
| Flag on, no token in that login's sources | `no Cursor login token read yet` (retry ~30s) | Sign in with Cursor CLI on that host |
| Token file / Keychain denied | `Cursor's login token could not be read` (retry 5m) | Fix permissions or Keychain access |
| Token read, API not yet answered | `Cursor's API has not answered yet` | Wait one poll (5 minutes, same as today) |

## Non-goals

- Measuring OpenCode usage.
- Changing how Anthropic or Codex limits are read.
- Shipping Cursor/Codex PR ownership (separate project after this).
- Writing or displaying the access token.
- Requiring the owner to create a matchblox Cursor account folder for the default login.
- Changing the Cursor usage API or the `mo` display format.
- Changing how long USAGE keeps a Cursor account after its last session.

## Anti-goals (failure even if it works)

- The access or refresh token is written to a log, a pane, USAGE text, or state JSON.
- A host without Keychain stays forever unmeasured when a Cursor auth file with `accessToken` is present in a documented default-login path.
- A call runs while `cursor_usage` is false.
- Account-folder usage and the default login's usage are mixed (one login shows another's figures).

## Surfaces

- Credentials / login tokens — harm: token leak or reuse outside Cursor's usage API. Anti-goal above.
- Checked, not touched: pane text, SSH keys, GitHub tokens.

## Approaches

| Way | Trade-off |
|---|---|
| A — File-first for default; Keychain only as darwin fallback | Works on Linux; Keychain still a special case in code |
| **B — One try-sources mechanism; each login type has its own source list** | Slightly more refactor; one model on every OS; no cross-login mix |
| C — Only account folders | Does not help the common default CLI login on Linux |

Pick: **B**. Decided by: the owner (2026-11-25).

**Source lists (same try-in-order mechanism):**

| Login | Sources (first usable `accessToken` wins) |
|---|---|
| Default (`dir == ""`) | (1) `$XDG_CONFIG_HOME/cursor/auth.json` or `~/.config/cursor/auth.json` (2) `~/.cursor/auth.json` (3) on darwin only: Keychain service `cursor-access-token` |
| Account folder | Only `<folder>/home/.cursor/auth.json` — never host `~/.config`, never Keychain |

`Reads` is true when that login's list is non-empty (always for default once wired; for accounts when `File` is wired), not only when Keychain exists.

## Rulings

- 2026-11-25 — Approach B as above: one try-sources mechanism; default and account logins have different source lists so tokens never mix. — because the owner wants multi-machine, not Keychain-specific, and mixing is an anti-goal. — the owner (approach); source-list split — the builder from cold read.
- 2026-11-25 — Opt-in stays `[limits] cursor_usage = true`. — because safety.md already treats this as the one exception that reads a login token. — the owner (implied by keeping current safety).
- 2026-11-25 — Default file sources are the two paths in the table; files before Keychain on darwin. — because this host's live token is under `~/.config/cursor/`; files-first keeps Linux and file-backed CI simple. — the owner (files on every OS); order — builder call from Calls.
- 2026-11-25 — Ships/PR ownership for Cursor and Codex is the next project after this ships. — the owner.
- 2026-11-25: spec approved. Decided by: the owner.

## Left to the builder

- Exact wiring of `CursorUsages` / `Reads` / fix strings / tests without changing the Fetch API contract beyond token acquisition.
- Whether `$XDG_CONFIG_HOME` is consulted when set (recommended: yes, then fall back to `~/.config`).
- Doc edits for `docs/limits.md`, `docs/safety.md`, `docs/agents.md`, `docs/console.md` (drop "macOS only" for the default login; describe the source lists).

## Done when

- [ ] On a Linux host with `cursor_usage = true` and `accessToken` in `~/.config/cursor/auth.json` (or `~/.cursor/auth.json`), Machine → USAGE shows a measured Cursor `mo` within one 5-minute usage poll after the token is readable (or briefly `Cursor's API has not answered yet`, then measured).
- [ ] On darwin, with neither default auth file present but Keychain `cursor-access-token` present, USAGE still measures (no regression). How to seed Keychain is left to the builder's test harness / manual check notes.
- [ ] With `cursor_usage` false, no Cursor token read runs.
- [ ] A hostile test: flag on, Linux-style default with no Keychain and an auth file on a default path → `Reads("")` is true and a token is obtained from the file (no requirement that Keychain be non-nil).
- [ ] A hostile test: account folder source list does not call Keychain or host `~/.config/cursor/auth.json`.
- [ ] `docs/safety.md`, `docs/limits.md`, `docs/agents.md`, and `docs/console.md` describe file sources on every OS for the default login, not Keychain-only.

## Cold read

- 2026-11-25 — fresh `generalPurpose` reader on this path (agent `180b7a72-1c42-458f-b1b2-71318324fc08`).
- Blockers fixed in-doc: per-login source lists (default vs account); Keychain only on the default list.
- Majors settled in-doc: USAGE copy aligned with `docs/limits.md`; all four docs in done-when; sampler retention unchanged; opt-in anti-goal clarified; default paths fixed to the two named (+ XDG left to builder); poll interval = 5 minutes; Keychain regression seeding left to builder.
- Changed after the read: Approaches, Rulings, Experience wrong-paths, Anti-goals, Done when, Left to the builder.
