# One-way chains

Issue: #333 follow-up (no issue yet). Status: in design review, 2026-11-08.
Mockup: "One-way Chains Console" artifact (owner's, 2026-11-07).
Builds on: #299 (chains), #309 (chains across repos), #326 (chain room, matchblox-ux), #304 (a stoker per chain).

## Why

The owner wants a chain to form from one command. Today a chain needs both
sides to run `matchblox chain <peer>`, and a match that asks waits up to an
hour for the other side. The owner said: "its just easier". The owner also
wants the console to show chains across agents (Claude Code, Codex, Cursor,
OpenCode) and across repos as plainly as same-repo chains, with sending
working the same way, whatever multiplexer runs the panes.

## Owner's decisions

- A chain is a team: every member hears every member. (2026-11-07, relayed by matchblox-ux, confirmed in the room spec)
- One command joins a team. No handshake, no requests. (2026-11-07, option 1)
- A match may join more than one team. Two teams that share a match do not hear each other. (2026-11-07, relayed by matchblox-ux)
- A send to a chain-mate works like an inbox: it is queued, never refused, for the mate's state. (2026-11-07)
- Chains and sending go through a small backend-neutral mux layer (option A), the first slice of #200's `internal/mux`. (2026-11-07)
- The fuse keeps its column between the match and the name. No chain marker moves to the right of the row. (2026-11-07)
- The landing page: matchbloxrelease owns `www/index.html`; this work sends it the chain section's lede, three beats and any demo change, which it applies verbatim. (2026-11-07)

## Goals

1. `matchblox chain <who>` joins at once: the team of `<who>`, or a new team with it.
2. A match can be in several teams; news and sends stay inside each team.
3. `matchblox send` to a mate is accepted whatever the mate is doing, and reaches it at its next rest or next step.
4. The room (#326) shows every mate with its agent, its repo and its inbox count, including mates matchblox cannot see right now.
5. No chain or send code path calls tmux directly.
6. The landing page's chain section matches this behaviour.

## Non-goals

- Directed follows or pairwise links (rejected 2026-11-07: a chain is a team).
- Chains across hosts (#299).
- The herdr backend itself (#205), and moving Sessions, peek, switch and the stoker behind `internal/mux` (#200 step one).
- The chain list, the room layout and a stoker per team: matchblox-ux's #326.

## Anti-goals

This fails, even if it runs, when:

- A chain exists that the console does not show. (#299, #326)
- A send is lost: queued and never delivered, with nobody told.
- One command pulls two whole teams together.
- A match that turned chains off is joined.
- A session outside a team can type into one inside it.

## Approaches

- **1. Join the team (picked).** One command joins `<who>`'s team. Keeps "a chain is a team"; one command can never merge two teams.
- **2. Merge both chains at once.** One command can make a team of ten nobody agreed to. Rejected.
- **3. Directed follows / 4. pairwise links.** Fit a hub agent, but a team of four needs up to six commands, and the fuse must show a graph. Rejected: the owner's first real use was a team.

## The design

### Joining

- A team takes the name of the match that started it (`hearth`), with `-2`, `-3` on a clash among this host's teams. It keeps that name when that match leaves. The name is what `matchblox chain <team>`, `unchain <team>`, the room title and `· also in <team>` show. (D1, 2026-11-08) When a name is both a team's and a match's, `matchblox chain <name>` joins the team of that name. (Builder's call.)
- `matchblox chain <who>`: when `<who>` is in exactly one team, the caller joins it. When `<who>` is in none, the two form a new team. Answer: `joined <team>: N matches. To leave it, run: matchblox unchain <team>`.
- When `<who>` is in two or more teams: refused, listing each team and its exact command. matchblox never picks one. The answer reads:
  ```
  web-cart is in 2 chains. Run one:
    matchblox chain hearth
    matchblox chain web
  ```
- `matchblox unchain` with no name leaves the match's only team. In several teams it is refused the same way: `you are in hearth and release. Run one:` then `matchblox unchain hearth`, `matchblox unchain release`, `matchblox unchain --all`. (D12, 2026-11-08)
- A match that is off cannot be joined, and cannot join: `<who> is off: it takes no chains. It can run: matchblox chain on`.
- Each mate hears, at its next step: `[matchblox] <name> joined the chain (in <repo>)`.
- A team that drops to one match stays while its stoker has a live plan. It keeps its name, and its room says `hearth · 1 match: run matchblox chain hearth from another match to join`. A team of one with no plan ends, as today. (D5, 2026-11-08)
- A mate whose pane still exists but shows no agent stays in its teams, shown `not seen · 12m`, then `not seen · 3h`. When its pane is gone, it leaves, and its mates hear `<name> left the chain`. A sample that did not read the panes changes nothing. This also covers the gap in #333's switch, where the pane outlives the agent. (D6, 2026-11-08)
- The stoker is in no team, can be joined by no one, and joins no one.
- Requests, their TTL and "asks to chain with you" notices go away.

### Sending

- Refusals left: not in a shared team; sends off; not one line; the rate limit; to itself; to the stoker.
- Every other send lands in the mate's inbox; the sender hears `queued for <mate>: <why it waits>. It gets the line at its next rest.`, or `sent` when typed at once.
- Delivery: typed at the next rest with an empty prompt (Claude Code, Codex); heard at the next step through the hook (Claude Code, Cursor). A question, a permission prompt, a park or a draft delays, never refuses.
- On delivery of a queued line, the sender hears at its next step `[matchblox] <mate> got your line (typed 14:02)` or `(heard 14:02)`, and the team's fuse sparks once. A line sent at once gets no second notice. (D3, 2026-11-08)
- OpenCode: a send lands in the inbox and answers `kept for <mate>: OpenCode takes no typed lines. It shows in matchblox peek <mate>.` Its inbox count shows in `faint`, and no D3 delivery notice follows. (D7, 2026-11-08)

### Taking a match out

- In the console, `u` on a match row acts on that match, so the key line asks `take docs-bot out of hearth? y` first. An agent's own `matchblox unchain <team>` leaves at once. The removed match hears `[matchblox] the owner took you out of hearth`. It can rejoin with one command. (D9, 2026-11-08)

### The human sees a join

- When a match joins a team, the console shows the toast `docs-bot joined hearth · u in its room takes it out`, and the team's fuse sparks once. The phone shows the same toast. (D8, 2026-11-08)

### Narrow screens and access

- Under 50 columns the join toast reads `docs-bot joined hearth · u: out`. CLI answers wrap at word bounds, and their first line always holds the outcome (`queued for web-cart`, then `it asks a question now; next rest`). (D11, 2026-11-08)
- Colour is never the only signal: `✉ 20+` carries its number, `not seen · Nm` its words, and the spark follows `--no-motion` as every spark does. (Restates DESIGN.md rules 1 and 5.)

### Journey

| Step | Who does | Feels | Plan specifies |
|---|---|---|---|
| 1 | An agent runs `matchblox chain hearth` | relief: one command | `joined hearth: 5 matches …` |
| 2 | The owner glances at the console | aware, not interrupted | the join toast and spark (D8) |
| 3 | A mate asks a question; another sends to it | confident: nothing bounces | `queued for web-cart: it asks a question now …` |
| 4 | The asking mate rests | closure | the line is typed; the sender hears `got your line` (D3) |
| 5 | An agent restarts or a sample misses | calm: the team holds | `not seen · Nm` while the pane lives (D6) |
| 6 | The owner wants a match out | in control | `u` in the room (D9) |

5 seconds: the room shows who is in the team and what each runs. 5 minutes: messages arrive without retries. Over months: teams keep their names and plans through restarts and switches.

### States

| Feature | Loading | Empty | Error | Success | Partial |
|---|---|---|---|---|---|
| Join (CLI) | none: the service answers at once | `<who>` in no team: a new team forms | off: `<who> is off: it takes no chains. It can run: matchblox chain on`; two teams: the list of teams and commands | `joined <team>: N matches. To leave it, run: matchblox unchain <team>` | n/a |
| Send | none | n/a | not in a shared team; sends off; not one line; the rate limit; self; stoker | `sent` (typed now) or `queued for <mate>: <why>` | OpenCode: `kept for <mate>: …` (D7) |
| Delivery | queued | n/a | dropped: `1 line to <mate> dropped: its inbox is full` (D4) | `<mate> got your line (typed 14:02)` (D3) | n/a |
| Team | the next state broadcast shows a new mate; no interim state (builder's call) | one match with a plan: `hearth · 1 match: run matchblox chain hearth …` (D5) | n/a | the room lists every mate | `not seen · Nm` while the pane lives (D6) |
- The inbox keeps 20 items; past that the oldest go and a line counts them. When a dropped item is a sent line, its sender hears `[matchblox] 1 line to <mate> dropped: its inbox is full`, and the mate's inbox count reads `✉ 20+` (the room row is #326's: matchblox-ux is told). (D4, 2026-11-08)

### Where it shows

```
 matchblox chain <who>  ──►  service joins the team  ──►  state broadcast
        │                          │                          │
        ▼                          ▼                          ▼
 CLI answer: joined …     mates' next step:          Sessions fuse (until #326 step 2)
                          "[matchblox] … joined"     and the team's room (#326)
```

### Sessions (until #326 step 2 removes the groupings)

- A match in two teams draws in the fuse of the team it joined first. Its row adds a faint `· also in <team>` for each other team. The other team's fuse skips that row. (D2, 2026-11-08)

### The room (#326 owns it)

- #326 adopted these rows (its §The room, item 2, 2026-11-07): the fuse column, the agent's mark, the name, the repo when the team spans repos, the state or ask, `✉ N`, a faint `not seen · Nm` for a mate the sampler cannot see, and `· also in <team>`. This plan supplies the data: team membership, inbox counts and last-seen times.
- #326 leaves team naming to this plan (its §Names).
- `✉ N` takes `muted`, then `warn` from 10 and at `✉ 20+`; OpenCode's count takes `faint` (D7). The need line keeps the one accent (DESIGN.md rule 4). matchblox-ux applies this to #326's row. (D10, 2026-11-08)
- Keys on a match row, proposed by this plan: `m` writes a line to it, `u` takes it out of this team.

### The mux layer

- One interface for what chains and sending do to a pane: type a line, press a key, read the screen, tell copy mode. tmux implements it. A fake implements it in tests, which prove no chain or send path names tmux.

### The landing page

- The chain section drops "Nothing is shared until both sides chain" and the one-repo framing; the new lede and three beats go to matchbloxrelease.


## NOT in scope

- The room's layout, the chain list, a stoker per team and the room rows at 40 columns: #326 (matchblox-ux).
- The landing page's layout and cuts: matchbloxrelease. This plan sends only the chain section's words.
- Renaming a team: no owner need yet (D1 chose zero-effort names).
- Directed follows, links, chains across hosts, the herdr backend (#205) and #200 step one.

## What already exists

- The fuse `╮┤╯`, its stub `─╴` and its spark (`internal/app/fuse.go`, DESIGN.md).
- The provider marks `✻ ❋ ⬢ ▣` of the dock (DESIGN.md §dock).
- The 2.5 s toast and the key-line confirm (`y`) of DESIGN.md §6.
- The news inbox and `[matchblox] since your last turn:` delivery (`internal/chains`, `internal/service/notice.go`).
- Typing at rest and the Codex at-rest queue (`internal/service/peersend.go`).
- #333's move on switch (`chains.Move`), which D6 now backs with the pane rule.

## Implementation Tasks

Synthesized from this review's findings. Effort ratio assumed ~30x for features, ~50x for tests.

- [ ] **T1 (P1, human: ~1d / CC: ~30min)** — internal/chains — one-command join, many teams per match, team names (D1), no requests
  - Surfaced by: Pass 1 — D1; owner decisions (option 1, multi-team)
  - Files: internal/chains/chains.go, internal/service/chains.go, cmd/matchblox/cli.go
  - Verify: `go test ./internal/chains ./internal/service ./cmd/...`
- [ ] **T2 (P1, human: ~4h / CC: ~15min)** — join and unchain answers — the ambiguous join and bare unchain refusals (D12), off refusal
  - Surfaced by: Pass 7 — D12; outside voice item 7
  - Files: internal/service/chains.go, cmd/matchblox/cli.go
  - Verify: CLI golden tests for each answer
- [ ] **T3 (P1, human: ~1d / CC: ~30min)** — sending — queue instead of refuse; delivery notice (D3); drop notice and `✉ 20+` (D4); OpenCode kept (D7)
  - Surfaced by: Pass 2 — D3, D4, D7
  - Files: internal/service/peersend.go, internal/chains/chains.go, internal/service/actions.go
  - Verify: `go test ./internal/service -run Peer`
- [ ] **T4 (P1, human: ~4h / CC: ~20min)** — team life — a team of one with a plan stays (D5); a mate stays while its pane lives (D6)
  - Surfaced by: Pass 2 — D5, D6
  - Files: internal/chains/chains.go, internal/service/chains.go
  - Verify: tests for a sample miss, an empty pane, a closed pane, a team of one with and without a plan
- [ ] **T5 (P1, human: ~1d / CC: ~30min)** — internal/mux slice — type, key, read screen, copy mode; tmux and a fake
  - Surfaced by: owner decision (option A)
  - Files: internal/mux/ (new), internal/service/peersend.go
  - Verify: a test that no chain or send path names tmux
- [ ] **T6 (P2, human: ~4h / CC: ~20min)** — console — Sessions fuse for a shared match (D2); join toast and spark (D8); `u` confirm (D9); narrow toast (D11)
  - Surfaced by: Pass 1 D2, Pass 3 D8 D9, Pass 6 D11
  - Files: internal/app/fuse.go, internal/app/model.go, goldens
  - Verify: app goldens at 80 and 40 columns
- [ ] **T7 (P2, human: ~30min / CC: ~5min)** — handoffs — send matchblox-ux D4's `✉ 20+`, D7's faint count and D10's tokens; send matchbloxrelease the chain section's lede and beats
  - Surfaced by: Pass 5 — D10; owner decision on the landing page
  - Files: none (messages)
  - Verify: each peer confirms
- [ ] **T8 (P2, human: ~2h / CC: ~10min)** — docs — docs/agents.md, docs/console.md, CHANGELOG for one-way teams
  - Surfaced by: Pass 3 journey
  - Files: docs/agents.md, docs/console.md, CHANGELOG.md
  - Verify: `go test ./cmd/...` (docs tests)

## Design Review Completion Summary

```
  +====================================================================+
  |         DESIGN PLAN REVIEW — COMPLETION SUMMARY                    |
  +====================================================================+
  | System Audit         | DESIGN.md present; UI: CLI answers, notices, |
  |                      | Sessions fuse, toasts, room data (#326)     |
  | Step 0               | 5/10; all 7 dimensions                      |
  | Pass 1  (Info Arch)  | 6/10 → 10/10 after fixes                   |
  | Pass 2  (States)     | 4/10 → 10/10 after fixes                   |
  | Pass 3  (Journey)    | 5/10 → 10/10 after fixes                   |
  | Pass 4  (AI Slop)    | 9/10 → 9/10 (no issues; no designer images)|
  | Pass 5  (Design Sys) | 7/10 → 10/10 after fixes                   |
  | Pass 6  (Responsive) | 6/10 → 9/10 (room rows at 40 cols: #326)   |
  | Pass 7  (Decisions)  | 1 resolved, 0 deferred                     |
  +--------------------------------------------------------------------+
  | NOT in scope         | written (4 items)                           |
  | What already exists  | written                                     |
  | TODOS.md updates     | 0 items proposed                            |
  | Approved Mockups     | 0 generated (rate limited), 0 approved      |
  | Decisions made       | 12 added to plan                            |
  | Decisions deferred   | 0                                           |
  | Overall design score | 4/10 → 9/10                                 |
  +====================================================================+
```

Outside voices: Claude subagent completed (9 findings, all folded into D1–D12). Codex unavailable: its sandbox could not start here (bwrap loopback). Missing coverage, not a pass.

### Unresolved Decisions

None.

## GSTACK REVIEW REPORT

| Run | Status | Findings |
|---|---|---|
| plan-design-review (in-host) | issues_found → resolved | 12 decisions (D1–D12), all approved by the owner |
| outside voice: Claude subagent | completed | 9 findings, folded into D1–D12 |
| outside voice: Codex | unavailable | sandbox failed to start (bwrap loopback) |

VERDICT: design-complete at 9/10. Pass 4 holds at 9 for want of designer images; Pass 6 holds at 9 because the room rows at 40 columns belong to #326.
OUTSIDE COVERAGE: single-model (Claude); Codex missing.

NO UNRESOLVED DECISIONS
