package sample

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/blox-eng/matchblox/internal/procfs"
)

// ProcRef names a process so that a reused pid is told apart.
type ProcRef struct {
	PID   int    `json:"pid"`
	Start uint64 `json:"start_ticks"`
}

// maxHops bounds every walk up a line of processes or children.
const maxHops = 64

type role int

const (
	roleSession role = iota
	roleChild
	roleSame // a launcher's own binary: the launcher holds the row
	roleNone // a server or daemon
)

// roleOf is p's role and, for a child, its parent's pid and how it was
// proven. byPID is the panes by their shell's pid.
func (s *Sampler) roleOf(pid int, p procfs.Proc, procs map[int]procfs.Proc, byPID map[int]Pane) (r role, parent int, via string) {
	agent := s.agentName(pid, p)
	if s.server(pid, agent) {
		return roleNone, 0, ""
	}
	if pp, ok := procs[p.PPID]; ok && s.agentName(p.PPID, pp) == agent {
		return roleSame, 0, ""
	}
	// The process tree: the nearest agent above p, short of a pane's shell
	// or a tmux server, whose own panes are sessions of their own.
	crossed := false
	for cur, hops := p.PPID, 0; cur > 1 && hops < maxHops; hops++ {
		q, ok := procs[cur]
		if !ok {
			break
		}
		ref := s.agentRef(cur, q)
		if ref.daemon { // Codex's app-server: its `codex exec` is its own work
			return roleNone, 0, ""
		}
		if a := ref.name; a != "" && !s.server(cur, a) {
			for ; hops < maxHops; hops++ { // a launcher's binary: the launcher holds the row
				pp, ok := procs[q.PPID]
				if !ok || s.agentName(q.PPID, pp) != a {
					break
				}
				cur, q = q.PPID, pp
			}
			return roleChild, cur, "process"
		}
		if _, ok := byPID[cur]; ok || strings.HasPrefix(q.Comm, "tmux") {
			crossed = true
			break
		}
		cur = q.PPID
	}
	// A detached batch run (no tty, re-parented past any pane) may name its
	// parent in a marker, trusted only once /proc verifies it.
	if crossed || p.TTY != 0 {
		return roleSession, 0, ""
	}
	if b := engineOf(agent).Batch; b == nil || !b(s.FS.Argv(pid)) {
		return roleSession, 0, ""
	}
	uid, ok := s.FS.UID(pid)
	if !ok {
		return roleSession, 0, ""
	}
	for _, name := range slices.Sorted(maps.Keys(engines)) {
		e := engines[name]
		if e.ParentEnv == "" {
			continue
		}
		v, _ := s.FS.Environ(pid, e.ParentEnv)
		q, err := strconv.Atoi(v)
		if err != nil || q == pid {
			continue
		}
		qp, ok := procs[q]
		if !ok || qp.State == 'Z' || qp.State == 'X' || s.agentName(q, qp) != e.Name || qp.StartTime >= p.StartTime {
			continue
		}
		if qu, ok := s.FS.UID(q); !ok || qu != uid {
			continue
		}
		// q started before p, so this ends.
		if r, _, _ := s.roleOf(q, qp, procs, byPID); r == roleSession || r == roleChild {
			return roleChild, q, "env"
		}
	}
	return roleSession, 0, ""
}

// server: the agent pid runs is a daemon or server by its engine's rule.
func (s *Sampler) server(pid int, agent string) bool {
	e := engineOf(agent)
	return e.Server != nil && e.Server(s.FS.Argv(pid))
}

// child is the row of an agent that parent started, or false for a batch
// run younger than 2 s: a hook's short `claude -p` does not flash a row.
func (s *Sampler) child(pid int, p procfs.Proc, agent string, parent procfs.Proc, via string, procs map[int]procfs.Proc, children map[int][]int, dt, uptime float64, now time.Time) (Session, bool) {
	argv := s.FS.Argv(pid)
	age := uptime - float64(p.StartTime)/procfs.ClockTicks
	if e := engineOf(agent); e.Batch != nil && e.Batch(argv) && age < 2 {
		return Session{}, false
	}
	cpu, n := s.treeCPU(pid, procs, children, dt)
	return Session{
		PID: pid, Start: p.StartTime, Agent: agent, Cwd: s.FS.Cwd(pid), CPU: cpu, Procs: n,
		Started: now.Add(-time.Duration(age * float64(time.Second))),
		Name:    childName(agent, argv), Busy: true, Status: "busy", Context: "unmeasured",
		Parent: &ProcRef{parent.PID, parent.StartTime}, Via: via,
	}, true
}

// childName is the agent, and how it was run when that says more: `codex
// exec`, `claude -p`.
func childName(agent string, argv []string) string {
	argv = codexArgv(argv)
	if len(argv) < 2 {
		return agent
	}
	a := argv[1]
	word := a != "" && !strings.HasPrefix(a, "-") && strings.Trim(a, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-") == ""
	if a == "-p" || a == "--print" || word {
		return agent + " " + a
	}
	return agent
}

// linkChildren keeps the children whose line reaches a session, sorted by
// parent, then start. A session reads as it would without them.
func linkChildren(sessions, children []Session) []Session {
	top := map[ProcRef]bool{}
	for _, x := range sessions {
		top[ProcRef{x.PID, x.Start}] = true
	}
	up := map[ProcRef]ProcRef{}
	for _, c := range children {
		up[ProcRef{c.PID, c.Start}] = *c.Parent
	}
	kept := children[:0]
	for _, c := range children {
		at := *c.Parent
		for hops := 0; hops < maxHops; hops++ {
			if top[at] {
				kept = append(kept, c)
				break
			}
			next, ok := up[at]
			if !ok {
				break
			}
			at = next
		}
	}
	slices.SortFunc(kept, func(a, b Session) int {
		if c := cmp.Compare(a.Parent.PID, b.Parent.PID); c != 0 {
			return c
		}
		return cmp.Compare(a.Start, b.Start)
	})
	return kept
}
