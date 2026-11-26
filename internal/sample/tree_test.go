package sample

import (
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/procfs"
)

// procTable is a process table under construction, with each process's cpu
// and argv.
type procTable struct {
	procs map[int]procfs.Proc
	cpu   map[int]float64
	argv  map[int][]string
}

func newProcTable() *procTable {
	return &procTable{procs: map[int]procfs.Proc{}, cpu: map[int]float64{}, argv: map[int][]string{}}
}

func (t *procTable) add(pid, ppid int, comm string, cpu float64, argv ...string) {
	t.procs[pid] = procfs.Proc{PID: pid, PPID: ppid, Comm: comm}
	t.cpu[pid] = cpu
	if len(argv) == 0 {
		argv = []string{comm}
	}
	t.argv[pid] = argv
}

func (t *procTable) argvOf(pid int) []string { return t.argv[pid] }

// busyTree adds xargs (13) and its 16 gcloud, each with 15 python at 7%,
// under parent.
func (t *procTable) busyTree(parent int) {
	t.add(13, parent, "xargs", 0)
	for g := range 16 {
		gpid := 100 + g
		t.add(gpid, 13, "gcloud", 0)
		for p := range 15 {
			t.add(1000+15*g+p, gpid, "python", 7)
		}
	}
}

// paneTree is pane pid 10 (zsh) → 11 claude → 12 bash -c → the busy tree.
func paneTree(paneParent int) *procTable {
	t := newProcTable()
	t.add(10, paneParent, "zsh", 0)
	t.add(11, 10, "claude", 1)
	t.add(12, 11, "bash", 0, "bash", "-c", "xargs -P16 -n1 gcloud compute ssh")
	t.busyTree(12)
	return t
}

func wantBusyTree(t *testing.T, got *Tree, agent int) {
	t.Helper()
	if got == nil {
		t.Fatal("heaviest = nil, want the xargs tree")
	}
	want := Tree{PID: 13, Comm: "xargs", Next: "gcloud", Procs: 257, CPU: 240 * 7, Agent: agent}
	if *got != want {
		t.Fatalf("heaviest = %+v, want %+v", *got, want)
	}
}

var agent11 = map[int]bool{11: true}

func TestHeaviestCutsUnderThePane(t *testing.T) {
	pt := paneTree(5)
	got := heaviest(pt.procs, pt.cpu, []Pane{{ID: "%1", PID: 10}}, agent11, pt.argvOf)
	wantBusyTree(t, got, 11)
}

func TestHeaviestOutsideAPane(t *testing.T) {
	pt := newProcTable()
	pt.add(1, 0, "systemd", 0)
	pt.busyTree(1)
	wantBusyTree(t, heaviest(pt.procs, pt.cpu, nil, nil, pt.argvOf), 0)

	pt = newProcTable()
	pt.add(1, 0, "systemd", 0)
	pt.add(900, 1, "systemd", 0, "systemd", "--user")
	pt.busyTree(900)
	wantBusyTree(t, heaviest(pt.procs, pt.cpu, nil, nil, pt.argvOf), 0)
}

func TestHeaviestKeepsPanesOutOfTheirServer(t *testing.T) {
	pt := paneTree(500)
	pt.add(1, 0, "systemd", 0)
	pt.add(500, 1, "tmux", 2)
	got := heaviest(pt.procs, pt.cpu, []Pane{{ID: "%1", PID: 10}}, agent11, pt.argvOf)
	wantBusyTree(t, got, 11)
}

func TestHeaviestUnderAStokerPane(t *testing.T) {
	pt := paneTree(700)
	pt.add(1, 0, "systemd", 0)
	pt.add(500, 1, "tmux", 0) // the main server, its pane idle
	pt.add(20, 500, "zsh", 0)
	pt.add(700, 1, "tmux", 0) // the stoker's server
	panes := []Pane{{ID: "%1", PID: 20}, {ID: OnServer("%1", "stoker"), PID: 10, Server: "700"}}
	wantBusyTree(t, heaviest(pt.procs, pt.cpu, panes, agent11, pt.argvOf), 11)
}

func TestHeaviestPicksTheMostCPU(t *testing.T) {
	pt := newProcTable()
	pt.add(1, 0, "systemd", 0)
	pt.add(20, 1, "make", 30)
	pt.add(21, 20, "cc", 30)
	pt.add(22, 20, "cc", 30)
	pt.add(30, 1, "node", 1)
	for i := range 39 {
		pt.add(31+i, 30, "esbuild", 1)
	}
	got := heaviest(pt.procs, pt.cpu, nil, nil, pt.argvOf)
	want := Tree{PID: 20, Comm: "make", Next: "cc", Procs: 3, CPU: 90}
	if got == nil || *got != want {
		t.Fatalf("heaviest = %+v, want %+v", got, want)
	}
}

func TestHeaviestWithNoLoad(t *testing.T) {
	pt := paneTree(5)
	for pid := range pt.cpu {
		pt.cpu[pid] = 0
	}
	if got := heaviest(pt.procs, pt.cpu, []Pane{{ID: "%1", PID: 10}}, agent11, pt.argvOf); got != nil {
		t.Fatalf("heaviest = %+v, want nil", *got)
	}
}

// TestShellWrapper: a shell is a wrapper only when it runs a command string,
// with -c alone or among other short flags.
func TestShellWrapper(t *testing.T) {
	for _, c := range []struct {
		argv []string
		want bool
	}{
		{[]string{"bash", "-c", "make"}, true},
		{[]string{"/bin/zsh", "-lc", "make"}, true},
		{[]string{"sh", "-e", "-c", "make"}, true},
		{[]string{"zsh"}, false},
		{[]string{"bash", "script.sh", "-c"}, false},
		{[]string{"bash", "--rcfile", "x"}, false},
	} {
		if got := runsCommand(c.argv); got != c.want {
			t.Errorf("runsCommand(%q) = %v, want %v", c.argv, got, c.want)
		}
	}
}

func BenchmarkHeaviest(b *testing.B) {
	pt := newProcTable()
	pt.add(1, 0, "systemd", 0)
	pt.add(500, 1, "tmux", 0)
	var panes []Pane
	agents := map[int]bool{}
	for tr := range 50 {
		pane, agent := 10000+60*tr, 10001+60*tr
		pt.add(pane, 500, "zsh", 0)
		pt.add(agent, pane, "claude", 1)
		pt.add(agent+1, agent, "bash", 0, "bash", "-c", "make -j")
		pt.add(agent+2, agent+1, "make", 0)
		for w := range 56 {
			pt.add(agent+3+w, agent+2, "cc", float64(tr%7))
		}
		panes = append(panes, Pane{ID: "%" + string(rune('a'+tr%26)), PID: pane})
		agents[agent] = true
	}
	if len(pt.procs) < 3000 {
		b.Fatalf("%d processes, want 3000", len(pt.procs))
	}
	b.ReportAllocs()
	for b.Loop() {
		heaviest(pt.procs, pt.cpu, panes, agents, pt.argvOf)
	}
}

// TestSampleCarriesHeavy: a scan finds the heaviest tree, and every
// snapshot carries it.
func TestSampleCarriesHeavy(t *testing.T) {
	root := copyFixture(t)
	s := newFixtureSampler(root)
	s.ProcEvery = 30 * time.Second
	s.Sample()

	// Pane %1's agent's child burns half a core over the next ProcEvery.
	s.Now = func() time.Time { return fixtureNow.Add(s.ProcEvery) }
	setTicks(t, root, 210, 100+1500)
	snap := s.Sample()
	agent := byPane(snap)["%1"].PID
	if h := snap.Heavy; h == nil || h.PID != 210 || h.Comm != "node" || h.Agent != agent || h.Agent == 0 {
		t.Fatalf("heavy = %+v, want node 210 under agent %d", h, agent)
	}
}
