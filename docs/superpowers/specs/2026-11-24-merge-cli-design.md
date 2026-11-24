# A builder merges a waiting ship from the CLI the same way as with `p`

Date: 2026-11-24. Spec: Part 2 of `design/0021-wedge-against-ade.md`.
Inventory: `design/0021-part2-gaps.md` (candidate 3).

## Who, and the moment

A builder (or an agent that drives matchblox) sits away from the console.
A session's pull request is green and waits for their merge.
On the desk they press `p`, read `gh pr merge …`, type `y`, and it lands.
From a shell or a script they can `matchblox ships` and see the wait, but they
have no twin that runs the same guarded merge.
Evidence: `docs/ships.md` (`p` merges; typed `y`); `matchblox -h` lists
`ships` and no merge; service already resolves `ship:<repo>#<n>` in
`internal/service/actions.go`.

## Why it matters

- Estimated: each merge from a headless host today means open the console, or
  paste `gh pr merge` by hand and lose the two-minute green pin and the
  `WaitsForYou` guard the board already holds.
- Agents and stoker already speak CLI for kill, send, hold. A missing merge
  twin breaks the "every console act that matters has a CLI twin" goal in
  the wedge design.

## The experience

Happy path:

1. A ship waits for merge on session `api` (same as `p` would offer).
2. The builder runs `matchblox merge api`.
3. matchblox sends the same act the console would: `ship:<repo>#<n>` with
   confirm, through the service. The merge argv is the one Ships last read
   (method + `--match-head-commit` when present).
4. On success the command exits 0. Stderr/stdout stay quiet when normal, in
   line with other life commands; a failure names the fix.

Wrong paths (words the person sees; each names the next act):

- No session matches `<who>` → same resolve error as `kill` / `peek`.
- Session has no ship that waits for merge →
  `no ship of <name> waits for a merge` (same words as the board notice).
- Ship wait is older than the board's two-minute read window, or
  `Merge` is empty → refuse with those board words; do not invent a fresh
  `gh pr merge`.
- Service is down → same start/connect error other CLI acts use.

## Approaches

| | Path | Trade-off |
|---|---|---|
| A | Phone buzz path spike only | Learns flaky reconnect; may ship no product this week |
| B | Stale-host wait cue | Helps multi-host; still needs a feel spike first |
| C | **Ship merge CLI twin** | Proven desk/CLI gap; no phone; reuses `ship:` act |

**Picked C** (dive-in, 2026-11-24): owner said dive in; inventory candidate 3
is the only row with a clear repro without a spike. A and B stay on the
inventory for later.

How to shape the command (builder calls inside C):

| | Shape | Trade-off |
|---|---|---|
| C1 | `matchblox merge <who>` (session twin of `p`) | One mental model with the board |
| C2 | `matchblox ships merge <n>` (by PR number) | Easy for scripts; drifts from `p`'s session lead |
| C3 | Print the argv only (`--dry-run`) as v1 | Safe; still leaves the act to paste |

**Picked C1** as the default shape (proposed with this spec).

## Non-goals

- Phone buzz spike, stale-host cue, or Pocket jump into tmux (other inventory rows).
- A new confirm prompt on the CLI (other life commands already send `Confirm: "y"`;
  the service still re-checks `WaitsForYou` + `Merge` at act time).
- Bulk merge of every green ship.
- Owning `gh` auth, or inventing merge argv the Ships report did not pin.
- ADE surfaces (diff review, Electron, harness wall).

## Anti-goals (failure even if it works)

- A merge runs for a ship that no longer waits for you (stale green, wrong PR).
- The CLI invents a `gh pr merge` line the console would not show.
- Copy claims "native IDE merge" or grows Electron-class surface for this act.
- A failed merge prints success, or an empty error with no next step.

## Surfaces

- an agent's tools — a CLI merge is a destructive tool an agent can call; it
  must only fire the same guarded `ship:` act the board already exposes.
  Anti-goal: inventing argv or merging a ship that does not wait.
- No other sensitive surface touched. Checked: authentication, money,
  uploads, webhooks, secrets, CI, personal data.

## Rulings

- 2026-11-24 — Approach B of the wedge (selective phone/remote/CLI, no ADE
  chase) — because owner approved `design/0021-wedge-against-ade.md`.
- 2026-11-24 — Part 2 v1 is candidate 3 (merge CLI), not buzz spike or
  stale-host cue — because dive-in; only candidate 3 has a clear repro
  without a device spike. — dive-in (owner), recorded by the agent.
- 2026-11-24 — Command shape `matchblox merge <who>` (C1) — because it is
  the twin of board `p` on a session. — owner (approval of this spec).
- 2026-11-24 — Landing page: unchanged for this feat unless the story
  changes; PR body holds `Landing page: unchanged because …` — because
  AGENTS.md landing guard.
- 2026-11-24: spec approved. Decided by: the owner.

## Left to the builder

- Exact wiring in `cmd/matchblox` (lifeAct-style vs ships subcommand).
- Whether stdout echoes the argv once on success or stays fully quiet.
- Test placement (`cli_test` / service act tests) and docs rows in
  `docs/ships.md` + `docs/console.md`.
- Updating `design/0021-part2-gaps.md` gate checkboxes for the pick.

## Calls to check

- Confirm C1 (`merge <who>`) over C2 (`ships merge <n>`)? Recommendation:
  C1 — because it matches `p`. Cost if wrong: scripts that only know a PR
  number need an extra `ships --json` lookup (acceptable).

## Done when

- [ ] From a shell with serve up, `matchblox merge <session>` merges the
      leading ship that waits for you, and a second run on the same session
      prints `no ship of <name> waits for a merge` (or the post-merge truth).
- [ ] `matchblox -h` names `merge`.
- [ ] `docs/ships.md` documents the CLI twin next to `p`.
- [ ] A test fails if the CLI can merge when `Kind` is not `WaitsForYou` or
      `Merge` is empty.

## Cold read

Cold read: not cold-read. Reason: Cursor harness marks start-a-subagent
not verified; cold-read forbids the author reading their own document in
its place.
