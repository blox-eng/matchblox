# Sessions as a Tree Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An agent process that another agent started (a `codex exec` or `claude -p` from a Claude Code Bash call) shows as a branch under the session that started it, proven from `/proc`, instead of being hidden or a stray row.

**Architecture:** The sampler gives every agent process a role: session, child, same (a launcher's binary), or none (a server). A child's parent is proven by the process tree, or for a detached process by a verified engine marker (`CLAUDE_PID`). Children go in a new `Snapshot.Children` list, never in `Snapshot.Sessions`, so no queue, stoker or action path can reach them. The console and `matchblox sessions` draw them under their parent.

**Tech Stack:** Go, `/proc` via `internal/procfs`, Bubble Tea console in `internal/app`.

**Spec:** `design/0015-session-tree.md`

## Global Constraints

- Linux `/proc` is the only source of the parent link. No screen is read for it.
- A link is drawn only when proven. Unproven: the row stays top-level.
- Lineage never widens an action: children are not in `Snapshot.Sessions`, so `resolve`, the queue, the stoker and every service loop never see them.
- The env proof applies only to a process with no controlling tty whose walk up reached init or a subreaper without passing a tmux server or a pane's shell.
- Env marker accepted only if the named pid is alive, is an agent of the engine that exports it, started before the child (`StartTime` lower), has the same real uid, and is a session or child.
- Batch children show once they have lived 2 s (from `StartTime`). Interactive children show at once.
- The console draws four levels of children, then `└─ … N more`.
- Every existing test in `internal/sample` keeps passing, #193's Codex tests included.
- Commits: signed Conventional Commits (`git commit -S`), never `--no-verify`. No real machine names, people or companies in code, tests or docs.
- Comment style: the repo's (a short sentence above a non-obvious rule, present tense).

## Review Focus

1. A tmux server started from a Claude Bash call: every pane agent on it carries `CLAUDE_PID` and descends from that claude. It must stay a top-level session (walk stops at the pane shell / tmux server). Test in Task 3.
2. A pid reused after the parent exited: a detached child whose `CLAUDE_PID` names a newer process stays top-level. Test in Task 3.
3. A launcher and its native binary (same engine, direct parent, no shell between): one row, not a parent with a child of itself. Test in Task 3.
4. A codex parent that runs `claude -p`: the link works whatever the parent engine (process tree, no marker). Test in Task 3.
5. A child that outlives its parent (parent exits, child re-parented, `CLAUDE_PID` now dead): the child becomes a top-level session, never an orphan child pointing at nothing. Test in Task 3.

---

## File Structure

- `internal/procfs/proc.go` — `Proc` gains `SID` and `TTY` (stat fields 6 and 7).
- `internal/sample/engine.go` (new) — the engine table: batch, server, parent env per engine.
- `internal/sample/lineage.go` (new) — roles and the parent proof.
- `internal/sample/sampler.go` — `Session.Parent`/`Via`, `ProcRef`, `Snapshot.Children`; the main loop uses roles instead of `underAgent`'s drop; parents of working children read `StatusAgents`.
- `internal/app/sessions.go` — child rows under each session; a `children` line in the opened row.
- `cmd/matchblox/cli.go` — `matchblox sessions` prints children indented; `--json` rows carry them.
- `docs/console.md`, `CHANGELOG.md`, `design/0015-session-tree.md` — what changed.

---

### Task 1: `/proc` reads a process's session and tty

**Files:**
- Modify: `internal/procfs/proc.go:41-51` (`Proc`), `:105-124` (`parseStat`)
- Test: `internal/procfs/proc_test.go`

**Interfaces:**
- Produces: `procfs.Proc.SID int` (stat field 6, session id) and `procfs.Proc.TTY int` (stat field 7, `tty_nr`; 0 = no controlling tty).

- [ ] **Step 1: Write the failing test**

```go
// TestStatReadsSessionAndTTY: a process's session id and controlling tty
// come from stat fields 6 and 7; 0 is no tty.
func TestStatReadsSessionAndTTY(t *testing.T) {
	p, ok := parseStat(9, []byte("9 (codex) S 8 9 9 34817 9 0 0 0 0 0 1 0 0 0 20 0 1 0 555 1 1"), 4096)
	if !ok || p.SID != 9 || p.TTY != 34817 {
		t.Fatalf("got %+v", p)
	}
	p, _ = parseStat(10, []byte("10 (claude) S 1 10 10 0 -1 0 0 0 0 0 1 0 0 0 20 0 1 0 556 1 1"), 4096)
	if p.SID != 10 || p.TTY != 0 {
		t.Fatalf("detached: got %+v", p)
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `go test ./internal/procfs -run TestStatReadsSessionAndTTY`
Expected: FAIL, `p.SID undefined`.

- [ ] **Step 3: Add `SID int // the session` and `TTY int // tty_nr: the controlling terminal, 0 for none` to `Proc`; parse `f[3]` and `f[4]` in `parseStat`.**

- [ ] **Step 4: Run the package**

Run: `go test ./internal/procfs`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/procfs
git commit -S -m "feat: procfs reads a process's session and controlling tty (#206)"
```

---

### Task 2: One engine table

**Files:**
- Create: `internal/sample/engine.go`
- Modify: `internal/sample/sampler.go` (`codexBatch`, `codexServer` stay as functions; the table points at them)
- Test: `internal/sample/engine_test.go`

**Interfaces:**
- Produces:

```go
type engine struct {
	Name      string              // as in config agents
	Batch     func([]string) bool // argv runs it with no person at it
	Server    func([]string) bool // argv is a daemon or server: never a session
	ParentEnv string              // the variable it exports to its shells naming its own pid
}

// engineOf is name's entry, or a zero entry with only Name: linked by the
// process tree only, no batch or server rule.
func engineOf(name string) engine
```

Entries:
- `claude`: `Batch` = argv has `-p` or `--print`; `ParentEnv` = `CLAUDE_PID`.
- `codex`: `Batch` = `codexBatch` on `codexArgv(argv)`; `Server` = `codexWords(codexArgv(argv))` starts with `app-server` or `mcp-server`.
- `cursor-agent` (`CursorAgent`): `Batch` = argv has `-p` or `--print`.
- `opencode`: `Batch` = first word of argv after argv0 is `run`.
- Every other agent: zero entry.

- [ ] **Step 1: Write the failing tests**

```go
func TestEngineRules(t *testing.T) {
	for _, c := range []struct {
		name          string
		argv          []string
		batch, server bool
	}{
		{"claude", []string{"claude", "-p", "hi"}, true, false},
		{"claude", []string{"claude", "--resume", "x"}, false, false},
		{"codex", []string{"codex", "exec", "fix"}, true, false},
		{"codex", []string{"codex", "-m", "gpt-5", "exec", "fix"}, true, false},
		{"codex", []string{"codex", "mcp-server"}, true, true},
		{"codex", []string{"codex", "app-server", "--managed-daemon"}, true, true},
		{"codex", []string{"codex"}, false, false},
		{CursorAgent, []string{"agent", "-p", "hi"}, true, false},
		{"opencode", []string{"opencode", "run", "hi"}, true, false},
		{"aider", []string{"aider"}, false, false},
	} {
		e := engineOf(c.name)
		if got := e.Batch != nil && e.Batch(c.argv); got != c.batch {
			t.Errorf("%v: batch %v", c.argv, got)
		}
		if got := e.Server != nil && e.Server(c.argv); got != c.server {
			t.Errorf("%v: server %v", c.argv, got)
		}
	}
	if engineOf("claude").ParentEnv != "CLAUDE_PID" || engineOf("codex").ParentEnv != "" {
		t.Fatal("parent env")
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `go test ./internal/sample -run TestEngineRules`
Expected: FAIL, `undefined: engineOf`.

- [ ] **Step 3: Write `engine.go`** with `var engines = map[string]engine{...}` and `engineOf`. Leave `codexBatch`/`codexServer` call sites as they are: this task only adds the table.

- [ ] **Step 4: Run the package**

Run: `go test ./internal/sample`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sample/engine.go internal/sample/engine_test.go
git commit -S -m "feat: one table says how each agent runs in batch, as a server, and names its parent (#206)"
```

---

### Task 3: Roles and the parent proof, and `Snapshot.Children`

**Files:**
- Create: `internal/sample/lineage.go`
- Modify: `internal/sample/sampler.go` — `Session` (after `Agents`), `Snapshot` (after `Sessions`), `sampleSessions` main loop (`if s.underAgent(p, procs) { continue }` at about line 627), the end of `sampleSessions` (after `s.readPanes`)
- Modify: `internal/sample/agents_test.go` — `addProc` gains a variant that writes tty, start time and extra env
- Test: `internal/sample/lineage_test.go`

**Interfaces:**
- Consumes: `procfs.Proc.SID`, `procfs.Proc.TTY` (Task 1); `engineOf` (Task 2).
- Produces:

```go
type ProcRef struct {
	PID   int    `json:"pid"`
	Start uint64 `json:"start_ticks"`
}

// in Session, after Agents:
Parent *ProcRef `json:"parent,omitempty"` // the process that started this one: set on children only
Via    string   `json:"via,omitempty"`    // process | env: how the link was proven

// in Snapshot, after Sessions:
// Children are agents another agent started, each under its Parent. They
// are shown, never acted on: no key, queue item or stoker step reaches one.
Children []Session `json:"children,omitempty"`

type role int
const (
	roleSession role = iota
	roleChild
	roleSame // a launcher's own binary: the launcher holds the row
	roleNone // a server or daemon
)

// roleOf is p's role and, for a child, its parent's pid and how it was
// proven. byPID is the panes by their shell's pid.
func (s *Sampler) roleOf(pid int, p procfs.Proc, procs map[int]procfs.Proc, byPID map[int]Pane) (r role, parent int, via string)
```

Algorithm of `roleOf` (the design's "Roles" and "Proving a parent"):
1. `engineOf(agent).Server(argv)` → `roleNone`.
2. Direct parent is the same agent name → `roleSame`.
3. Walk `PPID` up from `p` (≤ 64 hops, stop at pid ≤ 1). Stop with *no parent* when the current pid is a pane's shell (`byPID`) or its comm starts with `tmux`; mark `crossed`. When the current pid is an agent (`agentName != ""`) that is not `roleNone`, climb its `roleSame` chain to the row holder and return `roleChild, holder, "process"`.
4. Walk ended without an agent, `!crossed`, `p.TTY == 0`: for each engine with a `ParentEnv`, read it with `s.FS.Environ(pid, …)`; accept pid `q` only if `procs[q]` exists, `agentName(q) == engine.Name`, `procs[q].StartTime < p.StartTime`, `s.FS.UID(q) == s.FS.UID(pid)`, and `q`'s own role is session or child (one level of recursion; `q` started earlier, so it terminates). Return `roleChild, q, "env"`.
5. Else `roleSession`.

In the main loop, replace the `underAgent` drop with `roleOf`: `roleNone`/`roleSame` → `continue` (after `alive[key] = true` for `roleSame`). `roleChild` → build a child (below) and `continue`. `roleSession` → as today. `underAgent` stays for the `opencodeIn`/`codexIn` counts, unchanged.

A child is `Session{PID, Start, Agent, Cwd, CPU/Procs (treeCPU), Started, Name: childName(agent, argv), Busy: true, Status: "busy", Context: "unmeasured", Parent: &ProcRef{parent, procs[parent].StartTime}, Via: via}`, with an empty `Pane`. A batch child (`engineOf(agent).Batch(argv)`) younger than 2 s is skipped. `childName(agent string, argv []string) string`: the agent, then the first argument after argv[0] (after an interpreter's script) if it is `-p`, `--print`, or a subcommand word of letters and dashes: `codex exec`, `claude -p`, else the agent alone.

After `s.readPanes`, drop any child whose `Parent` is in neither `Sessions` nor `Children`. For each remaining child, find its root session (follow `Parent` through `Children`). If that root is not `Busy` and its status is not `StatusAgents`, set its `Status = StatusAgents`. Sort `Children` by parent pid, then `Start`.

- [ ] **Step 1: Write the failing tests** in `lineage_test.go`, each on `copyFixture` with pane `%1`'s agent at pid 200, the existing claude. Add a helper in `agents_test.go`:

```go
// addProcAt is addProc with a start time, a controlling tty and more
// environment ("K=V" each).
func addProcAt(t *testing.T, root string, pid, ppid int, comm string, argv []string, start uint64, tty int, env ...string)
```

Tests (name: setup → assertion):
- `TestABatchAgentUnderASessionIsItsChild`: 220 bash (ppid 200), 221 codex `codex exec fix` (ppid 220, start well over 2 s ago, tty 0, `TMUX_PANE=%1`) → `len(snap.Children)==1`, child `PID 221`, `Parent.PID 200`, `Via "process"`, `Pane ""`, `Name "codex exec"`; pane `%1` still has exactly one entry in `Sessions`; the claude on `%1` reads `StatusAgents` when the fixture had it idle.
- `TestADetachedChildIsProvenByItsMarker`: 230 codex `codex exec x` (ppid 1, tty 0, `CLAUDE_PID=200`, start after 200's) → child of 200, `Via "env"`.
- `TestAForgedMarkerIsIgnored`, table of three: `CLAUDE_PID` = a dead pid; = a claude that started after the child; = a claude of another uid (write a `status` with another `Uid:`) → `len(Children)==0` and pid 230 is in `Sessions`.
- `TestAPaneOnAServerStartedByClaudeIsASession`: 240 `tmux: server` (ppid 200), 241 zsh (ppid 240, listed as pane `%9`'s shell via `addPane`), 242 claude (ppid 241, tty non-zero, `CLAUDE_PID=200`) → 242 is in `Sessions`, not `Children`.
- `TestALaunchersBinaryIsTheSameSession`: 250 node `node /x/codex/bin/codex.js exec y` named codex by `agentName` (ppid 220), 251 codex (ppid 250) → one child (250), no entry for 251.
- `TestACodexParentLinksItsClaudeChild`: 260 codex TUI on a pane, 261 bash (ppid 260), 262 claude `claude -p q` (ppid 261) → child of 260, `Via "process"`.
- `TestAChildWhoseParentEndedIsASession`: 270 claude `claude -p q` (ppid 1, tty 0, `CLAUDE_PID=99999` with no process 99999) → in `Sessions`.
- `TestAYoungBatchChildIsNotShownYet`: 221 as in the first test, started 1 s before `fixtureNow` → `len(Children)==0`, and 221 is not in `Sessions` either.
- `TestCodexMCPServerIsNoOnesChild`: 280 codex `codex mcp-server` (ppid 200) → in neither list.
- Update `TestNestedAgentFoldsIntoTopSession` (sampler_test.go:290): keep its assertion, and add that `snap.Children` has pid 221 under 200.

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/sample -run 'Child|Marker|Server|Launcher|Nested|Young'`
Expected: FAIL, `snap.Children undefined`.

- [ ] **Step 3: Add `ProcRef`, the `Session` fields, `Snapshot.Children`, `role`, `roleOf`, `childName` in `lineage.go`, and wire the main loop and the post-`readPanes` pass in `sampleSessions`.**

- [ ] **Step 4: Run the whole module**

Run: `go test ./...`
Expected: PASS, the #193 Codex tests included.

- [ ] **Step 5: Commit**

```bash
git add internal/sample
git commit -S -m "feat: an agent that another agent started is its child, proven from /proc (#206)"
```

---

### Task 4: The console draws the tree

**Files:**
- Modify: `internal/app/sessions.go` — `sessionRows` (about line 329), `opened` (the `agents` field at about line 693)
- Test: `internal/app/tree_test.go`

**Interfaces:**
- Consumes: `sample.Snapshot.Children`, `sample.Session.Parent`, `sample.ProcRef` (Task 3).
- Produces: `func (m Model) childRows(parent sample.ProcRef, depth, w int) []string`: the lines under one session or child, depth-first, ordered as `Children`. Each line: two spaces, `│ ` per level above, then `├─ ` or `└─ ` (the last sibling), then the child's `Name` in `st.text` and its word (`busy`) in the faint style, fitted to `w`. At `depth == 4` the remaining descendants become one line `└─ … N more`.

In `sessionRows`, after `b.addRow(...)` for a session, add each `childRows(sample.ProcRef{s.PID, s.Start}, 1, w)` line with `b.add(-1, line)`: not selectable, so no key acts on a child. In `opened`, after the `agents` field, add `field("children", …)` with the direct children's names joined by ` · ` when there are any.

- [ ] **Step 1: Write the failing tests**

```go
// A child shows under its parent with a branch, wide and on a phone, and
// the cursor never lands on it.
func TestAChildShowsUnderItsParent(t *testing.T) {
	for _, w := range []int{120, 46} {
		st := queueState()
		p := sessionOn(st, "%1") // helper: the session on a pane
		st.Children = []sample.Session{
			{PID: 901, Start: 5, Agent: "codex", Name: "codex exec", Busy: true, Status: "busy", Parent: &sample.ProcRef{PID: p.PID, Start: p.Start}, Via: "process"},
			{PID: 902, Start: 6, Agent: "claude", Name: "claude -p", Busy: true, Status: "busy", Parent: &sample.ProcRef{PID: p.PID, Start: p.Start}, Via: "env"},
		}
		m, _ := loadedWith(t, w, st)
		out := screen(m)
		// the two lines come right after the parent's line, in order
		for _, want := range []string{"├─ codex exec", "└─ claude -p"} {
			if !strings.Contains(out, want) {
				t.Fatalf("w=%d: no %q in\n%s", w, want, out)
			}
		}
	}
}
```

Plus:
- `TestTheCursorSkipsChildren`: with the two children above under `%1`, press down from `%1` (the existing key helpers in `keys_test.go`) → the selected session is the next session in the list, not a child.
- `TestTheOpenedRowNamesChildren`: `selectPane(t, m, "%1")` at width 160 → screen contains `children` and `codex exec · claude -p`.
- `TestDeepChildrenFold`: a chain of six children (each the parent of the next) under `%1` → screen contains `… 2 more`, and does not contain the fifth child's name.

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/app -run 'Child|Children'`
Expected: FAIL, no `├─ codex exec`.

- [ ] **Step 3: Implement `childRows` and the two call sites.**

- [ ] **Step 4: Run the package**

Run: `go test ./internal/app`
Expected: PASS (golden or still tests that render `%1` with no children are unchanged).

- [ ] **Step 5: Commit**

```bash
git add internal/app
git commit -S -m "feat: Sessions draws each child under the session that started it (#206)"
```

---

### Task 5: `matchblox sessions` shows children, and the docs say so

**Files:**
- Modify: `cmd/matchblox/cli.go` — `sessionJSON` (line 96), `sessionRows` (line 142), `writeSessions` (line 164)
- Modify: `docs/console.md` (the Sessions section), `CHANGELOG.md` (`## [Unreleased]`), `design/0015-session-tree.md`
- Test: `cmd/matchblox/cli_test.go` (or the existing test file that covers `writeSessions`)

**Interfaces:**
- Consumes: `Snapshot.Children` (Task 3).
- Produces: `sessionJSON.Children []childJSON \`json:"children,omitempty"\`` with `type childJSON struct { Name, Agent, Via string; PID int; Children []childJSON }` (`json` tags: `name`, `agent`, `via`, `pid`, `children,omitempty`), nested by `Parent`.

Text output: after a session's line, each child on its own line, `     └─ codex exec (via process)`, indented two spaces per level.

- [ ] **Step 1: Write the failing test**

```go
func TestSessionsPrintsChildrenUnderTheirParent(t *testing.T) {
	doc := state.Doc{}
	doc.Sessions = []sample.Session{{PID: 200, Start: 1, Name: "app", Pane: "%1", Target: "app:1.1"}}
	doc.Children = []sample.Session{{PID: 221, Start: 2, Agent: "codex", Name: "codex exec", Via: "process", Parent: &sample.ProcRef{PID: 200, Start: 1}}}
	var b strings.Builder
	if err := writeSessions(&b, doc, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "└─ codex exec (via process)") {
		t.Fatalf("got\n%s", b.String())
	}
	b.Reset()
	writeSessions(&b, doc, true)
	if !strings.Contains(b.String(), `"children":[{"name":"codex exec","agent":"codex","via":"process","pid":221}]`) {
		t.Fatalf("json\n%s", b.String())
	}
}
```

(If `writeJSON` indents, compare after `json.Unmarshal` instead of by string.)

- [ ] **Step 2: Run it to see it fail**

Run: `go test ./cmd/matchblox -run TestSessionsPrintsChildren`
Expected: FAIL.

- [ ] **Step 3: Implement, then update the docs.**
  - `docs/console.md`: one short paragraph under Sessions, in the docs' voice: a child shows under the session that started it, with `├─`/`└─`; it is shown, never acted on; a link is drawn only when `/proc` proves it.
  - `CHANGELOG.md` under `## [Unreleased]`, `### Added`: `- **Sessions shows which session started which.** A \`codex exec\` or \`claude -p\` that an agent starts shows as a branch under that session, instead of hidden or a stray row. The link is proven from /proc, never guessed. (#206)`
  - `design/0015-session-tree.md`: Status `built`. Add under the design a "Changed while planning (2026-10-24)" note: children live in `Snapshot.Children`, never in `Sessions`, so a child is never a full session; a child with a pane of its own cannot be proven (tmux, not the agent, starts it), so that case is dropped; the walk stops at a pane's shell or a tmux server; "a family sorts by its most urgent member" falls away because a child never waits.

- [ ] **Step 4: Run everything**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/matchblox docs/console.md CHANGELOG.md design/0015-session-tree.md
git commit -S -m "feat: matchblox sessions lists each session's children, and the docs say how (#206)"
```

---

### Task 6: Live check on this host

Not a code task. The controller runs it after Task 5.

- [ ] Build: `go build -o /tmp/claude-1000/mbx ./cmd/matchblox` (scratch path).
- [ ] In a scratch tmux session with a Claude Code session, have it run `codex exec "say hi"` through Bash (or `claude -p "say hi"` if Codex is not signed in), and run `mbx status | jq '.children'` while it runs: one child, `via: "process"`, parent pid = that claude.
- [ ] #157: with `codex app-server --managed-daemon` alive and inheriting a pane, `mbx sessions` shows no codex row for it. Record the outcome in the PR; close #157 from the PR if it holds.
