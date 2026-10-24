package sample

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/blox-eng/matchblox/internal/procfs"
)

// startedAgo is the start time, in ticks, of a process that started secs
// before the fixture's now.
func startedAgo(root string, secs float64) uint64 {
	up := procfs.FS{Root: filepath.Join(root, "proc")}.Uptime()
	return uint64((up - secs) * procfs.ClockTicks)
}

// lineageSample samples root with Claude and Codex both configured.
func lineageSample(root string) Snapshot {
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "codex"}
	return smp.Sample()
}

func sessionOf(snap Snapshot, pid int) (Session, bool) {
	for _, x := range snap.Sessions {
		if x.PID == pid {
			return x, true
		}
	}
	return Session{}, false
}

func childOf(snap Snapshot, pid int) (Session, bool) {
	for _, x := range snap.Children {
		if x.PID == pid {
			return x, true
		}
	}
	return Session{}, false
}

func TestABatchAgentUnderASessionIsItsChild(t *testing.T) {
	root := copyFixture(t)
	addProcAt(t, root, 220, 200, "bash", []string{"bash"}, startedAgo(root, 10), 0)
	addProcAt(t, root, 221, 220, "codex", []string{"codex", "exec", "fix"}, startedAgo(root, 10), 0, "TMUX_PANE=%1")
	snap := lineageSample(root)
	if len(snap.Children) != 1 {
		t.Fatalf("children = %+v, want one", snap.Children)
	}
	c := snap.Children[0]
	if c.PID != 221 || c.Parent == nil || c.Parent.PID != 200 || c.Via != "process" || c.Pane != "" || c.Name != "codex exec" {
		t.Fatalf("child = %+v", c)
	}
	n := 0
	for _, x := range snap.Sessions {
		if x.Pane == "%1" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("pane %%1 has %d sessions, want 1", n)
	}
	// The fixture's claude on %1 is busy: a child leaves that as it is.
	if s, _ := sessionOf(snap, 200); !s.Busy || s.Status != "busy" {
		t.Fatalf("the busy parent reads %q", s.Status)
	}
}

// TestARestingParentWithAWorkingChildReadsAgents: pane %2's claude rests
// in the fixture; a child that works makes it read as working.
func TestARestingParentWithAWorkingChildReadsAgents(t *testing.T) {
	root := copyFixture(t)
	addProcAt(t, root, 320, 310, "bash", []string{"bash"}, startedAgo(root, 10), 0)
	addProcAt(t, root, 321, 320, "codex", []string{"codex", "exec", "fix"}, startedAgo(root, 10), 0, "TMUX_PANE=%2")
	snap := lineageSample(root)
	if _, ok := childOf(snap, 321); !ok {
		t.Fatalf("children = %+v, want 321", snap.Children)
	}
	if s, _ := sessionOf(snap, 310); !s.Busy || s.Status != StatusAgents {
		t.Fatalf("the parent reads %q, want %q while its child works", s.Status, StatusAgents)
	}
}

func TestADetachedChildIsProvenByItsMarker(t *testing.T) {
	root := copyFixture(t)
	addProcAt(t, root, 230, 1, "codex", []string{"codex", "exec", "x"}, startedAgo(root, 10), 0, "CLAUDE_PID=200")
	snap := lineageSample(root)
	c, ok := childOf(snap, 230)
	if !ok || c.Parent.PID != 200 || c.Via != "env" {
		t.Fatalf("children = %+v, want 230 under 200 by env", snap.Children)
	}
}

func TestAForgedMarkerIsIgnored(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, root string) string // the pid the marker names
	}{
		{"dead", func(t *testing.T, root string) string { return "99998" }},
		{"started after", func(t *testing.T, root string) string {
			addProcAt(t, root, 231, 1, "claude", []string{"claude"}, startedAgo(root, 5), 1)
			return "231"
		}},
		{"another uid", func(t *testing.T, root string) string {
			addProcAt(t, root, 232, 1, "claude", []string{"claude"}, startedAgo(root, 60), 1)
			other := os.Getuid() + 1
			status := "Name:\tclaude\nUid:\t" + strconv.Itoa(other) + "\t" + strconv.Itoa(other) + "\t" + strconv.Itoa(other) + "\t" + strconv.Itoa(other) + "\n"
			if err := os.WriteFile(filepath.Join(root, "proc", "232", "status"), []byte(status), 0o644); err != nil {
				t.Fatal(err)
			}
			return "232"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := copyFixture(t)
			marker := tc.setup(t, root)
			addProcAt(t, root, 230, 1, "codex", []string{"codex", "exec", "x"}, startedAgo(root, 10), 0, "CLAUDE_PID="+marker)
			snap := lineageSample(root)
			if len(snap.Children) != 0 {
				t.Fatalf("children = %+v, want none", snap.Children)
			}
			if _, ok := sessionOf(snap, 230); !ok {
				t.Fatal("230 must stay a session")
			}
		})
	}
}

func TestAPaneOnAServerStartedByClaudeIsASession(t *testing.T) {
	root := copyFixture(t)
	addProcAt(t, root, 240, 200, "tmux: server", []string{"tmux"}, startedAgo(root, 60), 0)
	addProcAt(t, root, 241, 240, "zsh", []string{"zsh"}, startedAgo(root, 50), 34825, "TMUX_PANE=%9")
	addPane(t, root, "%9\tother:1.1\tother\t241\tclaude\t/work/other")
	addProcAt(t, root, 242, 241, "claude", []string{"claude"}, startedAgo(root, 40), 34825, "TMUX_PANE=%9", "CLAUDE_PID=200")
	snap := lineageSample(root)
	if _, ok := sessionOf(snap, 242); !ok {
		t.Fatal("242 must be a session")
	}
	if _, ok := childOf(snap, 242); ok {
		t.Fatal("242 must not be a child")
	}
}

func TestALaunchersBinaryIsTheSameSession(t *testing.T) {
	root := copyFixture(t)
	addProcAt(t, root, 220, 200, "bash", []string{"bash"}, startedAgo(root, 10), 0)
	addProcAt(t, root, 250, 220, "node", []string{"node", "/usr/local/bin/codex", "exec", "y"}, startedAgo(root, 10), 0)
	addProcAt(t, root, 251, 250, "codex", []string{"/x/codex/vendor/codex", "exec", "y"}, startedAgo(root, 10), 0)
	snap := lineageSample(root)
	if len(snap.Children) != 1 || snap.Children[0].PID != 250 {
		t.Fatalf("children = %+v, want only 250", snap.Children)
	}
	if _, ok := sessionOf(snap, 251); ok {
		t.Fatal("251 is the launcher's own binary: no row")
	}
}

func TestACodexParentLinksItsClaudeChild(t *testing.T) {
	root := copyFixture(t)
	addProcAt(t, root, 260, 1, "codex", []string{"codex"}, startedAgo(root, 60), 34826, "TMUX_PANE=%8")
	addPane(t, root, "%8\tcx:1.1\tcx\t260\tcodex\t/work/cx")
	addProcAt(t, root, 261, 260, "bash", []string{"bash"}, startedAgo(root, 10), 0)
	addProcAt(t, root, 262, 261, "claude", []string{"claude", "-p", "q"}, startedAgo(root, 10), 0)
	snap := lineageSample(root)
	c, ok := childOf(snap, 262)
	if !ok || c.Parent.PID != 260 || c.Via != "process" || c.Name != "claude -p" {
		t.Fatalf("children = %+v, want 262 under 260 by process", snap.Children)
	}
}

func TestAChildWhoseParentEndedIsASession(t *testing.T) {
	root := copyFixture(t)
	addProcAt(t, root, 270, 1, "claude", []string{"claude", "-p", "q"}, startedAgo(root, 10), 0, "CLAUDE_PID=99999")
	snap := lineageSample(root)
	if _, ok := sessionOf(snap, 270); !ok {
		t.Fatalf("270 must be a session, children = %+v", snap.Children)
	}
}

func TestAYoungBatchChildIsNotShownYet(t *testing.T) {
	root := copyFixture(t)
	addProcAt(t, root, 220, 200, "bash", []string{"bash"}, startedAgo(root, 10), 0)
	addProcAt(t, root, 221, 220, "codex", []string{"codex", "exec", "fix"}, startedAgo(root, 1), 0, "TMUX_PANE=%1")
	snap := lineageSample(root)
	if len(snap.Children) != 0 {
		t.Fatalf("children = %+v, want none yet", snap.Children)
	}
	if _, ok := sessionOf(snap, 221); ok {
		t.Fatal("221 must not be a session either")
	}
}

func TestCodexMCPServerIsNoOnesChild(t *testing.T) {
	root := copyFixture(t)
	addProcAt(t, root, 280, 200, "codex", []string{"codex", "mcp-server"}, startedAgo(root, 10), 0)
	snap := lineageSample(root)
	if _, ok := sessionOf(snap, 280); ok {
		t.Fatal("a server is no session")
	}
	if _, ok := childOf(snap, 280); ok {
		t.Fatal("a server is no child")
	}
}

func TestChildName(t *testing.T) {
	for _, tc := range []struct {
		agent string
		argv  []string
		want  string
	}{
		{"codex", []string{"codex", "exec", "fix"}, "codex exec"},
		{"claude", []string{"claude", "-p", "q"}, "claude -p"},
		{"claude", []string{"claude", "--print", "q"}, "claude --print"},
		{"codex", []string{"node", "/x/codex.js", "exec", "y"}, "codex exec"},
		{"claude", []string{"claude", "--model", "x"}, "claude"},
		{"claude", []string{"claude", "fix this"}, "claude"},
		{"claude", []string{"claude"}, "claude"},
	} {
		if got := childName(tc.agent, tc.argv); got != tc.want {
			t.Errorf("childName(%q, %q) = %q, want %q", tc.agent, tc.argv, got, tc.want)
		}
	}
}

// TestAParentWorkingThroughAChildIsSuggestedAsWorking: pane %2's claude
// rests an hour holding context, so alone it is suggested a clear; with a
// child working it gets what a busy session with StatusAgents gets, no
// clear.
func TestAParentWorkingThroughAChildIsSuggestedAsWorking(t *testing.T) {
	alone, _ := sessionOf(lineageSample(copyFixture(t)), 310)
	if alone.Do != "clear" {
		t.Fatalf("the fixture's resting claude on %%2 is suggested %q, want clear", alone.Do)
	}
	root := copyFixture(t)
	addProcAt(t, root, 320, 310, "bash", []string{"bash"}, startedAgo(root, 10), 0)
	addProcAt(t, root, 321, 320, "codex", []string{"codex", "exec", "fix"}, startedAgo(root, 10), 0, "TMUX_PANE=%2")
	s, _ := sessionOf(lineageSample(root), 310)
	alone.Busy, alone.Status = true, StatusAgents
	do, why := DefaultRules.suggest(alone)
	if s.Do != do || s.Why != why || s.Do == "clear" {
		t.Fatalf("suggested %q (%s), want %q (%s) as a busy session with StatusAgents", s.Do, s.Why, do, why)
	}
}
