# Sessions as a tree: design

Date: 2026-10-24. Status: built. Issue: #206. Live-checks and closes: #157
(already fixed in code by #187).

## Changed while planning (2026-10-24)

- Children live in `Snapshot.Children`, never in `Sessions`, so a child is
  never a full session.
- A child with a pane of its own cannot be proven (tmux, not the agent,
  starts it), so that case is dropped. Every child has an empty `Pane`.
- The walk stops at a pane's shell or a tmux server.
- "A family sorts by its most urgent member" falls away, because a child
  never waits.
- A parent whose child works reads Busy with `StatusAgents`, the same
  invariant as a session working through subagents, and its compact and
  clear suggestion is recomputed.
- A parent's `Agents` stays the in-process subagent count. Children show on
  their own `children` line.
- A top-level `codex mcp-server` gets no row (the engine table's `Server`
  rule).
- `matchblox sessions` prints children under their parent, and `--json`
  nests them in `children` with `name`, `agent`, `via` and `pid`.

## Intent

A builder's Claude Code session sends out subagents, and a subagent often
runs another agent's CLI: `codex exec`, `agent -p`, `claude -p`. Today that
child is either invisible (the sampler drops any agent below another agent)
or, when its process chain breaks, a stray top-level row that nobody can
explain. The builder cannot tell which session started it, nor that a
parent which looks quiet still has work running through its children.

Sessions draws each child under the session that started it, with a branch
line, like `git log --graph`. The link is measured, never guessed: a row is
a child only when the process facts prove it.

The mechanism must scale to new agents (engines) and new places they run
(runtimes: tmux today, herdr #200 next), and it must be harder to fool than
a screen-rule manifest.

## Goals

1. **Every agent process gets a role.** Each agent process the sampler sees
   is one of: a *session* (top-level row), a *child* of a session, or *none*
   (a daemon, server or helper that is no one's session). Today's two
   outcomes (row or dropped) become three.
2. **The parent link is a measured fact.** `Session` carries its parent as
   a process identity (pid and start time), plus the evidence that proved
   it. The sampler is its one owner. The console, `matchblox sessions`,
   `status --json`, the phone and the stoker all read the same field.
3. **The link survives re-parenting.** A child whose shell detached or was
   re-parented to init or a subreaper still finds its parent, through an
   engine's own marker in its environment, verified against `/proc`.
4. **Sessions draws the tree.** A child row sits under its parent with
   `├─` / `└─`, in every group mode. It shows its own agent, status and
   context. The family sorts by its most urgent member.
5. **Servers are *none*, through the engine table.** `codex app-server`
   (#157, fixed by #187), `codex mcp-server` and Cursor's `worker-server`
   are *none*: their engine's `Server` rule says so. #157's repro is run
   live in this work and the issue closed on it.
6. **One engine table.** What each engine needs for this lives in one typed
   Go table: its commands, which argv is a batch run, which is a server,
   and which environment variable names its parent. Adding an engine is one
   entry plus its fixture test. Adding a runtime touches no engine entry.

## Non-goals

- **Agent-tool subagents as rows.** In-process subagents that start no CLI
  keep today's count in the opened row (`3 subagents running`). Rows are
  for things that run as their own process. Decided 2026-10-24 with the
  builder.
- **Screen rules (#199).** Which screen means `asks`, and which digit picks
  which option, is #199's work in `internal/said`. Lineage never reads a
  screen.
- **Other tmux servers (#96)** and **the herdr adapter (#200, #205).** This
  design keeps lineage runtime-neutral so they plug in later; it builds
  neither.
- **Lineage across hosts or into containers.** Each host links only its own
  processes. A container's agents keep today's behaviour.
- **Hook-based labels** (naming the subagent that launched a child). A
  possible later layer on top of this one.

## Anti-goals

- **A false family.** Drawing a link nobody measured is worse than drawing
  none. When the evidence is missing or does not verify, the row stays
  top-level.
- **Lineage that widens an action.** A key on a parent never acts on its
  children, and a key on a child never acts on its parent. `kill` keeps its
  exact target and its existing guards.
- **A child that hides a wait.** A child that waits for the builder still
  reaches the queue (see Goal 4: its family sorts as waiting).

## Constraints

- The sampler runs every 2 s by default and is the most heavily tested
  part of matchblox (#193 shows how fragile Codex attribution is). Each
  change lands behind a fixture test that fails first.
- `/proc` is the source of truth. `procfs.Proc` has pid, ppid, process
  group, comm, state and start time today. This design adds the session id
  (`stat` field 6), the controlling tty (`stat` field 7) and the real uid
  (`status` `Uid:`), all read in the same pass.
- The environment of a process is written by that process's ancestors and
  can be set by the process itself. It is a claim, never proof on its own.
- Linux first, as the sampler is today.

## Approaches

Considered on 2026-10-24. The builder picked **A**.

- **A. The sampler records the parent.** Keep nested agents instead of
  dropping them. Link each one by the process tree first, and by a verified
  engine marker in its environment once re-parented. One owner, every
  surface reads it, and #157 is fixed at the same spot. Cost: it changes
  the sampler, and children that were hidden become visible.
- **B. The console groups rows by guessing** (same pane, folder, start
  time). Cheap, but it guesses. It cannot show the children that are
  hidden today, and a wrong guess draws a false family. Dropped: it breaks
  the first anti-goal.
- **C. Hooks record each spawn.** Claude Code's hooks log each subagent and
  each Bash call that launches an agent. Richest detail, but only for a
  Claude Code parent on a host with the hooks, and it does nothing for
  #157. Kept as a possible later label layer on top of A.

## The design

### Roles

For each live agent process `p` (an `agentName` match, as today):

1. If `p` is a server or daemon by its engine's `Server` rule: **none**.
   A process that uses CPU still appears in Top, as today.
2. If `p`'s direct parent is a process of the *same* engine with no shell
   between (a launcher starting its native binary, as `opencodeIn` and
   `codexIn` already assume): it is the **same session** as that parent.
   The launcher keeps the row, as today.
3. Else, if a parent is proven (next section): **child** of that session.
4. Else: **session**, as today.

`underAgent`'s silent drop goes away. Its walk becomes step 1 of the
parent proof. Cursor's `agent -p` stays no agent at top level (as
`cursorPrint` has it today); under a session it is a batch child.

### Proving a parent

Evidence is tried in order. The first that verifies wins and is recorded
in `Via`.

1. **`process`: the process tree.** Walk `PPID` from `p` (at most 64 hops,
   as today) to the nearest ancestor that is an agent process with a role
   of session or child. That ancestor is the parent. A tree walk cannot be
   forged by the child.
2. **`env`: an engine marker.** Only for a process that the walk proves
   *detached*: it reached init or a subreaper without passing a tmux
   server or a pane's shell, and `p` has no controlling tty. A pane
   session never takes the env path: a tmux server started from a Claude
   Bash call seeds every pane with that call's `CLAUDE_PID`, and those are
   honest facts that would draw a false family. Then read the variable that `p`'s *suspected* parent engine exports to
   its shells (Claude Code: `CLAUDE_PID`). Accept the named pid only if all
   hold:
   - it is alive, and it is an agent process of the engine that exports
     that variable;
   - its start time is before `p`'s start time (a reused pid fails here);
   - it has the same real uid as `p`;
   - it has the role session or child.
   Otherwise the marker is ignored, never half-trusted, and `p` stays a
   session.

The parent is stored as `ProcRef{PID, Start}`, so a reused pid can never
inherit a family. Cycles cannot form: a parent always started earlier.

Depth is unbounded in the data. The console draws four levels and folds
deeper ones into `└─ … N more`.

### The engine table

`internal/sample/engine.go` holds one entry per engine, in typed Go:

```go
type engine struct {
	Name      string              // as in config agents: claude, codex, cursor-agent …
	Batch     func([]string) bool // argv runs it with no person at it: exec, -p
	Server    func([]string) bool // argv is a daemon or server: never a session
	ParentEnv string              // the variable it exports to its shells naming its own pid; "" if none
}
```

Today's ad-hoc helpers move into it: `codexBatch`, `codexServer`,
`cursorLauncher`'s worker-server rule. Engines without a known marker
(Codex, Cursor, OpenCode, the others in `docs/agents.md`) set `ParentEnv`
to `""` and are linked by the process tree only. Any configured agent with
no entry gets a zero entry: process tree only, no batch or server rule.

A contract test runs every entry against the same lineage fixtures: a
direct child, a re-parented child with a valid marker, one with a forged
marker, one with a reused pid, and a stray daemon.

**Runtimes stay out of the table.** Which pane a process belongs to
(`TMUX_PANE` today, `HERDR_PANE_ID` with #200) is the runtime's question,
answered where `ownerPane` answers it now. Lineage reads only `/proc`, so
herdr and other runtimes get it for free.

### Why this is harder to fool than a manifest

herdr detects agents with per-agent TOML manifests fetched from its site
and overridable locally. Those match what a pane shows and runs. They do
not record who started whom, as far as #200's check of its API shows.
Here:

- The rules are compiled into the binary, with nothing fetched at run
  time, so there is no remote rule file to attack.
- Lineage reads kernel facts (ppid, start time, uid, tty) first. An
  environment claim only counts after `/proc` verifies it.
- Lineage only places a row. It never chooses a key to type, never widens
  an action, and never changes what the stoker may touch beyond "a child
  is not a stoker target".

What a forged marker can do: a process of the same user can put *itself*
under a real session of that user, or fail to and stay top-level. It
cannot move another process, cross users, or reach a session started after
it. A same-uid process could already do worse, so this design adds no new
power.

### Snapshot shape

```go
type ProcRef struct {
	PID   int    `json:"pid"`
	Start uint64 `json:"start_ticks"`
}

// in Session:
Parent *ProcRef `json:"parent,omitempty"` // the session that started this one
Via    string   `json:"via,omitempty"`    // process | env: how the link was proven
```

A child's identity is its `ProcRef`, never its parent's pane. Two kinds:

- **A child with a pane of its own**: its controlling tty is a pane's tty
  that differs from its parent's (an agent started in a new tmux window or
  pane). It is a full session: pane, hooks, peek, answer, jump and the
  queue work as for any session.
- **A child in its parent's pane**: no controlling tty, or its parent's
  (a `codex exec` or `claude -p` from a Bash call, which Claude Code runs
  under `setsid` with no tty). Its `Pane` is empty, so no map keyed by
  pane can confuse it with its parent. It is shown, never acted on: no
  keys, no peek of a screen, no queue item, no hook match. Nobody could
  answer it anyway. `matchblox sessions` prints children indented under their
parent. `--json` carries `parent` and `via`.

### Console

- In every group mode, a child is drawn right after its parent, never on
  its own under another group label. The family's group is that of its
  most urgent member: a child with its own pane that waits for you puts
  the family in WAITS FOR YOU, with the child row marked.
- The child row: `├─` / `└─` in the tracks where the name starts, then its
  own word, agent, status and context bar. A narrow (phone) layout keeps
  the branch glyph and drops columns as it does today.
- The opened parent row adds a line: `children  codex · busy, claude -p · idle`.
- When a child ends, its row goes at the next sample. Nothing lingers.
- `Agents` (in-process subagents) keeps its current count line.

### Queue and stoker

- A child with its own pane that waits is a queue item like any other,
  labelled with its parent: `claude (under app)`. A child in its
  parent's pane is never a queue item.
- **The sampler** sets a parent whose turn ended but whose child works to
  `StatusAgents`, and `Agents` counts its working children with its
  in-process subagents. So the console, the phone, the queue and
  `status --json` all show the parent working, and `Resting`/`Works` keep
  their meaning. The stoker never targets a child: no compact, no clear,
  no go.

### Short-lived helpers

A batch child shows once it has lived 2 s, measured from its start time,
so a hook's one-second `claude -p` does not flash a row. An interactive
child shows at once.

## Done when

- A `codex exec` started from a Claude Code Bash call shows under that
  session by the next sample, in the console and in `matchblox sessions`.
- The same after its shell detached (`setsid`, `nohup … &`), proven through
  `CLAUDE_PID` with `via: env`.
- A process that forges `CLAUDE_PID` to a session started after it, to
  another user's session, or to a dead pid stays top-level.
- #157's live repro shows one claude row and no codex row on the pane.
- A child in its parent's pane has an empty `Pane`; no key reaches the
  parent through it.
- An agent started in a pane of a tmux server that inherited `CLAUDE_PID`
  stays top-level.
- `kill` on a parent targets only the parent. The stoker never acts on a
  child, and waits while a child works.
- Every engine entry passes the contract test. Adding a fake engine in the
  test needs no change outside its entry.

## Calls made in this design (for review)

- The child-row lifetime threshold of 2 s for batch children.
- Four drawn levels, then `… N more`.
- A family sorts by its most urgent member.
- `procfs` gains session id, tty and uid.
- A child in its parent's pane is shown only, never acted on.
- The env proof only for a detached process with no tty.
- Same engine, direct parent, no shell between: one session, launcher's row.
- `codex mcp-server` is a server (*none*), not a batch child.
- A working child makes its parent `StatusAgents` in the sampler.
- The new design lives in `design/`, following the repo's numbered docs,
  not `docs/superpowers/specs/`.

## Left to the builder

- Names and placement of helpers inside `internal/sample`.
- Exact glyphs and colours, within DESIGN.md.
- How to fold `opencodeIn` / `codexIn` counting into the role pass, as long
  as #193's tests keep passing.
