# Handover: agent switch (#143)

Kickoff prompt for an agent continuing this work. The design is approved
in outline; implementation has not started. Ship against the design, not
against this file when they disagree — update the design if a ruling
changes.

## Read first

1. GitHub issue: https://github.com/blox-eng/matchblox/issues/143
2. Design: [`design/0011-agent-switch.md`](../design/0011-agent-switch.md)
3. Pattern: [`design/0009-cursor-agent.md`](../design/0009-cursor-agent.md) and
   [`design/0010-cursor-agent-plan.md`](../design/0010-cursor-agent-plan.md)
4. Lifecycle: `internal/service/lifecycle.go` (`New` / `Kill` / `Rename`,
   `launchArgv`, `agentBin`)
5. Console lifeform: `internal/app/lifeform.go`, `docs/keys.md`
6. Holds: `internal/holds/holds.go` (today: hold follows SessionID; switch
   needs a pending pane rebind — see design)

## What to build

Issue #143: switch an existing tmux-backed session from one agent harness
to another (claude ↔ codex ↔ opencode ↔ cursor-agent) without losing the
matchblox place.

Also in this design (same PR unless you split with a note):

- Cursor launches from matchblox use `--trust` (`new` and `switch`).
- Cursor account usage on Sessions when a cheap cached source exists (no
  per-tick `agent status`).
- Outside-tmux agents stay listed but are clearly marked; refuse switch.

Out of scope: full foreign transcript import; switching non-tmux sessions
(#86).

## Locked rulings (do not re-litigate without updating the design)

- **Place:** name + cwd + same tmux pane. `respawn-pane` (not `kill-pane`
  + `new-session`).
- **Confirm:** typed `y`; mid-turn same gravity as `kill --anyway`.
- **Handoff default:** *Read the working tree and any local notes.
  Summarise progress so far, then continue the work from here.* Editable;
  empty allowed. Not a transcript import.
- **After success:** jump tmux client to that pane (console always; CLI
  when `TMUX` is set — plan may confirm).
- **Holds:** pending rebind by pane until new SessionID; seat must not
  look briefly free.
- **Driven:** builder switch clears stoker driven state for that pane.
- **Failure:** one respawn step; on failure no success / jump / rebind.
- **Canonical UI:** Sessions form (proposed key `S`) +  
  `matchblox switch <who> --agent <name> [--handoff '…'] [--anyway]`.

## How matchblox ships this

1. Write `design/0012-agent-switch-plan.md` (task checkboxes, tests first,
   files, commits) following `0010-cursor-agent-plan.md`.
2. Implement against the plan. Reuse lifecycle; do not invent a parallel
   start/kill path.
3. Tests first: each behaviour failed before it passed. Fixtures stay
   anonymous (`you@example.com`, `/work/app`, `/home/u`).
4. Docs: `docs/agents.md`, `docs/keys.md` (and limits/console if needed),
   README if user-facing, `CHANGELOG.md` under `## [Unreleased]`.
5. `go test ./...` and `golangci-lint run` before each commit.
6. Push and open/update the PR with conventional title (`feat: …`).
   Squash-only merges. Live-check confirm + switch on a real pane if you
   can.

## Likely touch points

- `internal/service/lifecycle.go` — add `switch`
- `cmd/matchblox/main.go`, `life.go`, `cli.go` — CLI surface
- `internal/app/lifeform.go`, Sessions keys — Switch form
- `internal/holds` / service hold lookup — pending pane rebind
- `launchArgv` — Cursor `--trust`
- Sampler / limits — Cursor account usage (spike first; defer if no cheap
  source, still ship `--trust` + switch)
- Sessions row rendering — mark `Pane == ""` as outside tmux

## Done when

- Design rulings above are covered by tests and docs.
- Draft/ready PR links #143.
- Handoff confirm + switch works on a real pane, or the PR says why not.

## Kickoff one-liner (paste into a new chat)

```text
Read the handover prompt from docs/handover-agent-switch.md and continue issue #143 from there.
```
