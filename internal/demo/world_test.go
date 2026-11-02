package demo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/service"
)

func newTestWorld(t *testing.T, kill func(int)) *World {
	t.Helper()
	root, err := copyTree("../../testdata/demo")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	w, err := NewWorld(root, time.Now, kill)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestAWorldRefusesWithTheLine(t *testing.T) {
	w := newTestWorld(t, func(int) {})
	err := w.Run([]string{"rm", "-rf", "/"})
	if err == nil || err.Error() != Refusal {
		t.Fatalf("got %v, want the refusal line", err)
	}
}

func TestAWorldKillsTheOrphanOnly(t *testing.T) {
	var killed []int
	w := newTestWorld(t, func(pid int) { killed = append(killed, pid) })
	if err := w.Run([]string{"kill", "4242"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Run([]string{"kill", "1"}); err == nil {
		t.Error("a kill of any other process was taken")
	}
	if len(killed) != 1 || killed[0] != OrphanPID {
		t.Fatalf("killed %v, want [%d]", killed, OrphanPID)
	}
}

// TestWireAnswersWhatTheServiceWouldAskTheMachine: with the seams wired, a
// service asks the world for the pane above its screen, tmux, the agents and
// the folders, and never the machine it runs on.
func TestWireAnswersWhatTheServiceWouldAskTheMachine(t *testing.T) {
	w := newTestWorld(t, func(int) {})
	s := service.New(config.Default(), nil, w.Git)
	w.Wire(s)

	text, up, ok := s.CaptureUp("%1", 40)
	if want, _ := os.ReadFile(filepath.Join(w.root, "panes", "1.txt")); !ok || up != 0 || text != string(want) {
		t.Errorf("CaptureUp: %q, %d, %v; want the fixture's pane", text, up, ok)
	}
	if _, _, ok := s.CaptureUp("%99", 40); ok {
		t.Error("CaptureUp read a pane the fixture lacks")
	}
	if _, err := s.TmuxOut([]string{"tmux", "list-sessions"}); err == nil || err.Error() != Refusal {
		t.Errorf("TmuxOut: %v, want the refusal line", err)
	}
	if err := s.Actions.Check(advice.Guard{}); err != nil {
		t.Errorf("Check: %v", err)
	}
	for _, cmd := range []string{"claude", "codex", "opencode"} {
		if got, err := s.LookPath(cmd); err != nil || !strings.HasSuffix(got, "/"+cmd) {
			t.Errorf("LookPath(%s): %q, %v", cmd, got, err)
		}
	}
	for _, cmd := range []string{"tmux", "/usr/local/bin/tmux", "gh"} {
		if _, err := s.LookPath(cmd); err == nil {
			t.Errorf("LookPath(%s) found a program the service would run", cmd)
		}
	}
	if !s.IsDir("/work/flint-auth") || s.IsDir(w.root) {
		t.Error("IsDir: the demo's folders are /work/ and nothing else")
	}
	if pane := s.PeerPane(1); pane != "" {
		t.Errorf("PeerPane: %q", pane)
	}
}

func TestTwoNewSessionsAreTwoSessions(t *testing.T) {
	w := newTestWorld(t, func(int) {})
	start := func(name string) error {
		return w.Run([]string{"env", "-u", "GIT_TERMINAL_PROMPT", "tmux", "new-session", "-d", "-s", name, "-n", name, "-c", "/work/flint-auth", "claude", "--name", name, "go"})
	}
	for _, name := range []string{"one", "two"} {
		if err := start(name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	panes, err := os.ReadFile(filepath.Join(w.root, "tmux-panes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"%9\tone:1.1\tone\t2000\t", "%10\ttwo:1.1\ttwo\t2001\t"} {
		if !strings.Contains(string(panes), want) {
			t.Errorf("tmux-panes.txt lacks %q:\n%s", want, panes)
		}
	}
	if w.sessions["%9"].name != "one" || w.sessions["%10"].name != "two" {
		t.Errorf("sessions: %+v %+v", w.sessions["%9"], w.sessions["%10"])
	}
}

// A session the demo started has no transcript yet: its first answer makes one.
func TestAStartedSessionTakesAnAnswer(t *testing.T) {
	w := newTestWorld(t, func(int) {})
	if err := w.Run([]string{"env", "HOME=/home/user", "tmux", "new-session", "-d", "-s", "one", "-n", "one", "-c", "/work/one", "claude"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Run([]string{"tmux", "send-keys", "-t", "%9", "-l", "--", "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Run([]string{"tmux", "send-keys", "-t", "%9", "Enter"}); err != nil {
		t.Errorf("the answer: %v", err)
	}
	if err := w.Finish("%9", 1000, "done"); err != nil {
		t.Errorf("its turn's end: %v", err)
	}
}

func TestAWorldAnswersWhatItCannotTake(t *testing.T) {
	w := newTestWorld(t, func(int) {})
	if err := w.Run(nil); err == nil || err.Error() != Refusal {
		t.Errorf("an empty argv: %v, want the refusal line", err)
	}
	if err := w.Finish("%99", 1000, "done"); err == nil {
		t.Error("Finish of a pane with no session passed")
	}
}

func TestAStartMissingATokenIsRefusedNotPanicked(t *testing.T) {
	w := newTestWorld(t, func(int) {})
	for _, argv := range [][]string{
		{"env", "new-session"},
		{"env", "tmux", "new-session", "-d", "-c", "/tmp", "claude"},
		{"env", "tmux", "new-session", "-d", "-s", "x", "claude"},
		{"env", "tmux", "new-session", "-d", "-s", "x", "-c"},
	} {
		if err := w.Run(argv); err == nil || err.Error() != Refusal {
			t.Errorf("%v: got %v, want the refusal line", argv, err)
		}
	}
}
