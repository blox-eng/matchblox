package sample

import (
	"slices"
	"strings"

	"github.com/blox-eng/matchblox/internal/procfs"
)

// Tree is the process tree that adds the most load to a host: the first
// command a session or the system started, past its agent and shell wrappers.
type Tree struct {
	PID   int     `json:"pid"`
	Comm  string  `json:"comm"`
	Next  string  `json:"next,omitempty"` // the root's child with the most processes under it
	Procs int     `json:"procs"`
	CPU   float64 `json:"cpu_pct"`         // summed, 100 = one core
	Agent int     `json:"agent,omitempty"` // the session above the root, 0: none
}

// cuts are where trees start: their roots are the first processes under them.
// An interactive shell is one, like a pane, so a job typed in a terminal
// window is its own tree, not the terminal app's.
type cuts struct{ panes, servers, shells map[int]bool }

func (c cuts) at(p procfs.Proc) bool {
	return p.PID == 1 || c.panes[p.PID] || c.servers[p.PID] || c.shells[p.PID] || p.Comm == "systemd" || p.Comm == "launchd"
}

// place is a process's spot in its tree: root is 0 while the walk down from
// the cut still skips agents and wrappers, and branch is the root's child it
// lies under.
type place struct{ root, branch int }

// heaviest returns the tree with the most summed cpu, or nil when no tree
// has any. Each process is placed once, from its parent's place, so the walk
// is linear in the processes; argv is read only for shells, once each.
func heaviest(procs map[int]procfs.Proc, cpu map[int]float64, panes []Pane, agents map[int]bool, argv func(int) []string) *Tree {
	c := cuts{panes: make(map[int]bool, len(panes)), servers: map[int]bool{}, shells: map[int]bool{}}
	for _, p := range panes {
		c.panes[p.PID] = true
		if pp, ok := procs[p.PID]; ok {
			c.servers[pp.PPID] = true
		}
	}
	argvs := map[int][]string{}
	for pid, p := range procs {
		if shell(p) {
			argvs[pid] = argv(pid)
		}
	}
	wraps := map[int]bool{} // the shells that run a command string
	for pid, a := range argvs {
		switch up, isShell := argvs[procs[pid].PPID]; {
		case runsCommand(a):
			wraps[pid] = true
		case interactive(a) && !(isShell && slices.Equal(up, a)): // a forked subshell keeps its parent's argv
			c.shells[pid] = true
		}
	}
	places := make(map[int]*place, len(procs))
	var placeOf func(pid int) *place
	placeOf = func(pid int) *place {
		if pl, ok := places[pid]; ok {
			return pl // a pid met again mid-walk is a ppid loop: its zero place ends it
		}
		pl := &place{}
		places[pid] = pl
		p := procs[pid]
		if c.at(p) {
			return pl
		}
		up := &place{}
		if pp, ok := procs[p.PPID]; ok && !c.at(pp) {
			up = placeOf(p.PPID)
		}
		switch {
		case up.root != 0:
			pl.root, pl.branch = up.root, up.branch
			if pl.branch == 0 {
				pl.branch = pid
			}
		case !agents[pid] && !wrapper(p, wraps):
			pl.root = pid
		}
		return pl
	}

	type sum struct {
		cpu   float64
		procs int
	}
	trees := map[int]*sum{}
	branches := map[int]int{}
	for pid, p := range procs {
		if c.at(p) && !(c.panes[pid] && agents[pid]) && !c.shells[pid] {
			continue // an agent that is its pane's process, or an interactive shell, is a tree: its place is its own
		}
		pl := placeOf(pid)
		root := pl.root
		if root == 0 {
			root = pid // a wrapper with no command under it is its own tree
		}
		t := trees[root]
		if t == nil {
			t = &sum{}
			trees[root] = t
		}
		t.cpu += cpu[pid]
		t.procs++
		if pl.branch != 0 {
			branches[pl.branch]++
		}
	}

	best := 0
	for root, t := range trees {
		if b := trees[best]; t.cpu > 0 && (b == nil || t.cpu > b.cpu || t.cpu == b.cpu && root < best) {
			best = root
		}
	}
	if best == 0 {
		return nil
	}
	tree := &Tree{PID: best, Comm: procs[best].Comm, Procs: trees[best].procs, CPU: trees[best].cpu}
	next, most := 0, 0
	for branch, n := range branches {
		if places[branch].root == best && (n > most || n == most && branch < next) {
			next, most = branch, n
		}
	}
	if next != 0 {
		tree.Next = procs[next].Comm
	}
	for pid, steps := best, 0; steps < len(procs); pid, steps = procs[pid].PPID, steps+1 {
		if agents[pid] {
			tree.Agent = pid
			break
		}
		if _, ok := procs[pid]; !ok {
			break
		}
	}
	return tree
}

// wrapper is a process that only carries the tree: a shell that runs a
// command string (in wraps), or the server of a login, under which the
// login shell is a cut.
func wrapper(p procfs.Proc, wraps map[int]bool) bool {
	switch p.Comm {
	case "sshd", "sshd-session", "login":
		return true
	}
	return wraps[p.PID]
}

// shell: p is sh, bash, zsh or dash; a login shell's name may lead with -.
func shell(p procfs.Proc) bool {
	switch strings.TrimPrefix(p.Comm, "-") {
	case "sh", "bash", "zsh", "dash":
		return true
	}
	return false
}

// interactive reports whether a shell that runs no command string is one a
// person types in: a login shell (argv[0] leads with -) or one with no
// operand after its options, and not -s, which reads its script on stdin.
// bash script.sh is the script's command; an argv not read is no shell's.
func interactive(argv []string) bool {
	if len(argv) == 0 || shortFlag(argv, 's') {
		return false
	}
	if strings.HasPrefix(argv[0], "-") {
		return true
	}
	args := argv[min(1, len(argv)):]
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case takesArg[a]:
			i++
		case !strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "+"):
			return false
		}
	}
	return true
}

// takesArg are the shell options whose value is the next argument.
var takesArg = map[string]bool{"-o": true, "+o": true, "-O": true, "+O": true, "--rcfile": true, "--init-file": true}

// runsCommand reports whether a shell's argv holds -c, alone or among other
// short flags, before its first operand.
func runsCommand(argv []string) bool { return shortFlag(argv, 'c') }

// shortFlag reports whether a shell's argv holds the flag f, alone or among
// other short flags, before its first operand.
func shortFlag(argv []string, f rune) bool {
	args := argv[min(1, len(argv)):]
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case takesArg[a]:
			i++
		case strings.HasPrefix(a, "--"):
		case !strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "+"):
			return false
		case strings.HasPrefix(a, "-") && strings.ContainsRune(a, f):
			return true
		}
	}
	return false
}

// sessionAbove is the session at the top of agent's line: agent itself when
// it is a session, the session that a child agent descends from, or 0.
func sessionAbove(agent int, sessions, children []Session) int {
	parent := make(map[int]int, len(children))
	for _, c := range children {
		if c.Parent != nil {
			parent[c.PID] = c.Parent.PID
		}
	}
	for hops := 0; agent != 0 && hops < maxHops; hops++ {
		if slices.ContainsFunc(sessions, func(s Session) bool { return s.PID == agent }) {
			return agent
		}
		agent = parent[agent]
	}
	return 0
}
