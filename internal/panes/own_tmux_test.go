//go:build linux

package panes

import (
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"syscall"
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

// TestAConsoleEndsOnlyItsOwnAgentPane: the second console exits; its agent
// pane goes with it, and the first console keeps both of its panes.
func TestAConsoleEndsOnlyItsOwnAgentPane(t *testing.T) {
	s := newOwnServer(t)
	one, two := s.console(t, Home), s.console(t, Home+"-2")
	s.tmux(BesideHookArgv()[1:]...)
	flag := t.TempDir() + "/stop"
	s.tmux("respawn-pane", "-k", "-t", two.c, "sh", "-c", "until [ -e "+flag+" ]; do sleep 0.05; done")
	if err := os.WriteFile(flag, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	panes := func() []string { return strings.Fields(s.tmux("list-panes", "-a", "-F", "#{pane_id}")) }
	waitFor(t, func() bool { return !slices.Contains(panes(), two.c) })
	waitFor(t, func() bool { return !slices.Contains(panes(), two.a) })
	if p := panes(); !slices.Contains(p, one.c) || !slices.Contains(p, one.a) {
		t.Errorf("panes left %q, want the first console's %s and %s", p, one.c, one.a)
	}
}

// sessions are the sessions of sock's clients, one per line.
func (s ownServer) sessions() string { return s.tmux("list-clients", "-F", "#{client_session}") }

// TestTheWayBackStaysInItsOwnConsole: from the agent beside its list, the
// second terminal's way back shows its own list; the first console stays
// as it was.
func TestTheWayBackStaysInItsOwnConsole(t *testing.T) {
	s := newOwnServer(t)
	one, two := s.console(t, Home), s.console(t, Home+"-2")
	s.tmux(HomeKeyArgv()[1:]...)
	s.attach(t, &one)
	s.attach(t, &two)
	s.tmux("select-pane", "-t", two.a)
	s.tmux("select-pane", "-t", one.a)
	press(t, two, HomeKey)
	waitFor(t, func() bool { return s.active(two) == two.c })
	if got := strings.Fields(s.sessions()); len(got) != 2 || !slices.Contains(got, one.name) || !slices.Contains(got, two.name) {
		t.Errorf("clients are in %q, want one in each console", got)
	}
	if got := s.active(one); got != one.a {
		t.Errorf("the first console's active pane is %s, want its agent %s", got, one.a)
	}
}

// focus is a session the way Ctrl+O makes it, marked with the console it
// came from, with a terminal in it.
func (s ownServer) focus(t *testing.T, back string) ownConsole {
	t.Helper()
	f := ownConsole{name: "matchblox-in-1"}
	f.a = s.tmux("new-session", "-d", "-s", f.name, "-P", "-F", "#{pane_id}", "sleep", "600")
	s.tmux("set-option", "-t", f.a, sample.BackOption, back)
	s.attach(t, &f)
	return f
}

// TestTheWayBackFromAFocusGoesToItsConsole: a terminal that went into an
// agent from the second console comes back to the second console.
func TestTheWayBackFromAFocusGoesToItsConsole(t *testing.T) {
	s := newOwnServer(t)
	s.console(t, Home)
	two := s.console(t, Home+"-2")
	s.tmux(HomeKeyArgv()[1:]...)
	f := s.focus(t, two.name)
	press(t, f, HomeKey)
	waitFor(t, func() bool { return s.sessions() == two.name })
	if got := s.active(two); got != two.c {
		t.Errorf("the console's active pane is %s, want its list %s", got, two.c)
	}
}

// TestTheWayBackFromAFocusOfAGoneConsoleGoesToTheFirst: the console the
// terminal came from has ended: the way back goes to the first console,
// not to a dead end.
func TestTheWayBackFromAFocusOfAGoneConsoleGoesToTheFirst(t *testing.T) {
	s := newOwnServer(t)
	s.console(t, Home)
	s.tmux(HomeKeyArgv()[1:]...)
	f := s.focus(t, Home+"-9")
	press(t, f, HomeKey)
	waitFor(t, func() bool { return s.sessions() == Home })
}

// TestFlipHandsOffToItsOwnConsole: ⌥s in an agent's pane reached from the
// second console hands off to the second console, and to no other.
func TestFlipHandsOffToItsOwnConsole(t *testing.T) {
	s := newOwnServer(t)
	one, two := s.console(t, Home), s.console(t, Home+"-2")
	s.tmux(FlipKeysArgv()[1:]...)
	f := s.focus(t, two.name)
	s.tmux("set-option", "-p", "-t", f.a, MatchOption, "1")
	press(t, f, "\x1bs")
	waitFor(t, func() bool { return s.sessions() == two.name && read(two.cOut) != "" })
	time.Sleep(100 * time.Millisecond)
	if got := read(one.cOut); got != "" {
		t.Errorf("the first console read %q", got)
	}
}

// TestAClosedTerminalOrphansItsConsole reads a real server: the second
// console is not orphaned while its terminal shows it, nor while that
// terminal is in an agent it opened, nor while it looks at another session;
// once the terminal closes, it is.
func TestAClosedTerminalOrphansItsConsole(t *testing.T) {
	s := newOwnServer(t)
	one, two := s.console(t, Home), s.console(t, Home+"-2")
	s.attach(t, &one)
	s.attach(t, &two)
	seen, seenOne := map[string]bool{}, map[string]bool{}
	orphaned := func() bool { return Orphaned(two.name, seen, s.tmux(ClientsArgv()[1:]...)) }
	if orphaned() {
		t.Fatal("orphaned while its terminal shows it")
	}
	tty := strings.Fields(s.tmux("list-clients", "-t", two.name, "-F", "#{client_tty}"))[0]
	for _, to := range []string{"matchblox-in-1", "work"} {
		s.tmux("new-session", "-d", "-s", to, "sleep", "600")
		s.tmux("switch-client", "-c", tty, "-t", to)
		waitFor(t, func() bool { return strings.Contains(s.sessions(), to) })
		if orphaned() {
			t.Fatalf("orphaned while its terminal is in %s", to)
		}
	}
	pid, err := strconv.Atoi(s.tmux("list-clients", "-t", "work", "-F", "#{client_pid}"))
	if err != nil {
		t.Fatal(err)
	}
	_ = syscall.Kill(pid, syscall.SIGHUP) // the terminal closes
	waitFor(t, orphaned)
	if Orphaned(one.name, seenOne, s.tmux(ClientsArgv()[1:]...)) {
		t.Error("the first console is orphaned")
	}
}

// TestAnUnmarkedFirstConsoleKeepsItsKeys: a first console that an older
// matchblox started has no mark; a newer console's keys still flip in it.
func TestAnUnmarkedFirstConsoleKeepsItsKeys(t *testing.T) {
	s := newOwnServer(t)
	one := s.console(t, Home)
	s.tmux("set-option", "-u", "-t", one.c, HomeOption)
	s.tmux(BesideKeysArgv()[1:]...)
	s.attach(t, &one)
	press(t, one, "\x11") // ctrl+q
	waitFor(t, func() bool { return s.active(one) == one.a })
}
