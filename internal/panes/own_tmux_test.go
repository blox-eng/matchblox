//go:build linux

package panes

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/tmuxtest"
)

// ownServer is one tmux server with a console session per terminal.
type ownServer struct {
	sock string
	tmux func(args ...string) string
}

// ownConsole is one console session: the list pane c and the agent pane a
// beside it, each a raw cat into its own file, and the terminal attached.
type ownConsole struct {
	name       string
	c, a       string
	cOut, aOut string
	term       *os.File
}

func newOwnServer(t *testing.T) ownServer {
	t.Helper()
	sock := tmuxtest.Server(t)
	s := ownServer{sock: sock}
	s.tmux = func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", sock, "-f", os.DevNull}, args...)...).CombinedOutput() //nolint:gosec // test argv
		if err != nil {
			t.Fatalf("tmux %q: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	return s
}

// console makes the console session name, marked as a console, without a
// terminal.
func (s ownServer) console(t *testing.T, name string) ownConsole {
	t.Helper()
	dir := t.TempDir()
	o := ownConsole{name: name, cOut: dir + "/c", aOut: dir + "/a"}
	o.c = s.tmux("new-session", "-d", "-s", name, "-x", "100", "-y", "30", "-P", "-F", "#{pane_id}", "sh", "-c", rawCat+o.cOut)
	o.a = s.tmux("split-window", "-d", "-h", "-t", o.c, "-P", "-F", "#{pane_id}", "sh", "-c", rawCat+o.aOut)
	s.tmux("set-option", "-w", "-t", o.c, sample.ConsoleOption, o.c, ";", "set-option", "-w", "-t", o.c, sample.BesideOption, o.a,
		";", "set-option", "-t", o.c, HomeOption, "1")
	for _, p := range []string{o.c, o.a} {
		waitFor(t, func() bool { return s.tmux("display-message", "-p", "-t", p, "#{pane_current_command}") == "cat" })
	}
	return o
}

// attach gives o a terminal.
func (s ownServer) attach(t *testing.T, o *ownConsole) {
	t.Helper()
	o.term = attach(t, s.sock, o.name)
	waitFor(t, func() bool { return strings.Contains(s.tmux("list-clients", "-F", "#{client_session}"), o.name) })
}

func (s ownServer) active(o ownConsole) string {
	return s.tmux("display-message", "-p", "-t", "="+o.name+":", "#{pane_id}")
}

func press(t *testing.T, o ownConsole, keys string) {
	t.Helper()
	if _, err := o.term.WriteString(keys); err != nil {
		t.Fatal(err)
	}
}

func read(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

// TestTwoConsolesKeepTheirKeysApart: the second terminal flips to its own
// agent and types; the text reaches that agent alone, and the first
// terminal's keys stay where they were.
func TestTwoConsolesKeepTheirKeysApart(t *testing.T) {
	s := newOwnServer(t)
	one, two := s.console(t, Home), s.console(t, Home+"-2")
	s.tmux(BesideKeysArgv()[1:]...)
	s.attach(t, &one)
	s.attach(t, &two)

	press(t, two, "\x11") // ctrl+q
	waitFor(t, func() bool { return s.active(two) == two.a })
	press(t, two, "rm -rf x")
	waitFor(t, func() bool { return strings.Contains(read(two.aOut), "rm -rf x") })
	time.Sleep(100 * time.Millisecond)
	if got := read(one.aOut) + read(one.cOut); strings.Contains(got, "rm") {
		t.Errorf("the first console's panes read %q", got)
	}
	if got := s.active(one); got != one.c {
		t.Errorf("the first console's active pane is %s, want its list %s", got, one.c)
	}
}

// TestAConsoleMarksItsOwnSession: the console's mark lands on the session
// of its pane, and on no other.
func TestAConsoleMarksItsOwnSession(t *testing.T) {
	s := newOwnServer(t)
	s.tmux("new-session", "-d", "-s", "work")
	pane := s.tmux("new-session", "-d", "-s", Home+"-2", "-P", "-F", "#{pane_id}")
	s.tmux(HomeMarkArgv(pane)[1:]...)
	if got := s.tmux("display-message", "-p", "-t", pane, "#{"+HomeOption+"}"); got != "1" {
		t.Errorf("the console's session has %s = %q, want 1", HomeOption, got)
	}
	if got := s.tmux("display-message", "-p", "-t", "work:", "#{"+HomeOption+"}"); got != "" {
		t.Errorf("work has %s = %q, want none", HomeOption, got)
	}
}
