package demo_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/app"
	"github.com/blox-eng/matchblox/internal/demo"
	"github.com/blox-eng/matchblox/internal/panes"
)

// spies records what the console would give the operating system: every seam
// of app.Options that runs a command.
type spies struct {
	mu   sync.Mutex
	argv []string
}

func (s *spies) saw(argv ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.argv = append(s.argv, strings.Join(argv, " "))
}

func (s *spies) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.argv)
}

func (s *spies) on(o *app.Options) {
	spied := errors.New("spied")
	o.Exec = func(argv []string, done func(error) tea.Msg) tea.Cmd {
		s.saw(argv...)
		return func() tea.Msg { return done(spied) }
	}
	o.Run = func(argv []string) error {
		s.saw(argv...)
		return spied
	}
	o.Clients = func() ([]string, error) {
		s.saw("tmux", "list-clients")
		return nil, spied
	}
	o.LastClient = func() (string, time.Time, error) {
		s.saw("tmux", "list-clients", "-t", "=matchblox")
		return "", time.Time{}, spied
	}
	o.Check = func(_ context.Context, argv []string, _ func(string)) (string, int) {
		s.saw(argv...)
		return "spied", 255
	}
	o.SSH = func(_ context.Context, argv, _ []string, _ func(string)) (string, int) {
		s.saw(argv...)
		return "spied", 255
	}
}

// outsideEnv are the folders of the person's own machine.
var outsideEnv = []string{"HOME", "XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME", "XDG_RUNTIME_DIR", "XDG_CACHE_HOME"}

// planted are the commands a real spawn of the service, the sampler or the
// console would find on PATH. Each only writes down that it ran.
var planted = []string{"git", "gh", "tmux", "ssh", "ssh-add", "ssh-keygen", "claude", "codex", "opencode", "cursor-agent",
	"docker", "nvidia-smi", "tailscale", "matchblox", "kill", "env", "sh", "curl", "xdg-open", "open"}

// machine is a person's machine with nothing on it: empty folders, and on PATH
// only commands that tell on whoever runs them.
type machine struct {
	outside string // holds a folder for each of outsideEnv
	temp    string
	spawned string // the planted commands' log
}

func newMachine(t *testing.T) machine {
	t.Helper()
	m := machine{outside: t.TempDir(), temp: t.TempDir()}
	for _, env := range outsideEnv {
		dir := filepath.Join(m.outside, strings.ToLower(env))
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv(env, dir)
	}
	t.Setenv("TMPDIR", m.temp)
	bin := t.TempDir()
	m.spawned = filepath.Join(t.TempDir(), "spawned")
	shim := fmt.Sprintf("#!/bin/sh\necho \"$0 $*\" >> %q\n", m.spawned)
	for _, name := range planted {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(shim), 0o700); err != nil { //nolint:gosec // an executable the test plants
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	return m
}

// spawns are the planted commands that ran.
func (m machine) spawns(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(m.spawned)
	if errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// written lists what is in the folders of the person's own machine.
func (m machine) written(t *testing.T) []string {
	t.Helper()
	var found []string
	for _, env := range outsideEnv {
		root := filepath.Join(m.outside, strings.ToLower(env))
		err := filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
			if p != root {
				found = append(found, p)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return found
}

// walker presses every key at a running demo.
type walker struct {
	t          *testing.T
	d          *demo.Demo
	term       *terminal
	spy        *spies
	cols, rows int
	ended      chan struct{}
	steps      []string
}

func startWalk(t *testing.T, cols, rows int) *walker {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	term := &terminal{in: inW, out: outR, ended: make(chan struct{})}
	go term.read()
	t.Cleanup(term.close)
	spy := &spies{}
	d, err := demo.Start(demo.WithConsole(demo.Options{Tree: tree, In: inR, Out: outW, Cols: cols, Rows: rows}, spy.on))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Stop)
	w := &walker{t: t, d: d, term: term, spy: spy, cols: cols, rows: rows, ended: make(chan struct{})}
	go func() {
		_ = d.Wait()
		close(w.ended)
	}()
	term.waitFor(t, "hearth-api")
	return w
}

// keyBytes are what a terminal sends for each key a binding names. A key that
// has no entry and is no single character fails the walk: it is a key the walk
// does not press yet. Esc is its CSI u form, which takes effect at once.
var keyBytes = map[string]string{
	"enter": "\r", "tab": "\t", "shift+tab": "\x1b[Z", "esc": "\x1b[27u", "space": " ", "backspace": "\x7f",
	"up": "\x1b[A", "down": "\x1b[B", "right": "\x1b[C", "left": "\x1b[D", "pgup": "\x1b[5~", "pgdown": "\x1b[6~",
	"ctrl+a": "\x01", "ctrl+c": "\x03", "ctrl+o": "\x0f", "alt+s": "\x1bs", "alt+n": "\x1bn",
}

func bytesOf(t *testing.T, key string) string {
	t.Helper()
	if b, ok := keyBytes[key]; ok {
		return b
	}
	if name, ok := strings.CutPrefix(key, "shift+"); ok && len(name) == 1 {
		return strings.ToUpper(name)
	}
	if len(key) != 1 {
		t.Fatalf("the walk has no bytes for the key %q", key)
	}
	return key
}

func (w *walker) press(keys ...string) {
	w.t.Helper()
	for _, k := range keys {
		w.send(k, bytesOf(w.t, k))
	}
}

// pressLast presses a key that ends the program: the end may come before
// the write returns, and that is the key's answer, not a failure.
func (w *walker) pressLast(key string) {
	w.t.Helper()
	w.steps = append(w.steps, key)
	go func() { _, _ = w.term.in.Write([]byte(bytesOf(w.t, key))) }()
}

// send writes to the terminal. A program that ended reads no more, so it fails
// here and not at the test's time limit. The steps it keeps name what ended it.
func (w *walker) send(what, s string) {
	w.t.Helper()
	w.steps = append(w.steps, what)
	sent := make(chan error, 1)
	go func() {
		_, err := w.term.in.Write([]byte(s))
		sent <- err
	}()
	select {
	case err := <-sent:
		if err != nil {
			w.t.Fatal(err)
		}
	case <-w.ended:
		w.dead()
	}
}

func (w *walker) dead() {
	w.t.Helper()
	w.t.Fatalf("the program ended (%v) after %v", w.d.Wait(), w.steps[max(len(w.steps)-12, 0):])
}

func (w *walker) alive() {
	w.t.Helper()
	select {
	case <-w.ended:
		w.dead()
	default:
	}
}

// tap is a press and a release at a cell, counted from 1.
func (w *walker) tap(x, y int) {
	w.t.Helper()
	w.send(fmt.Sprintf("tap %d,%d", x, y), fmt.Sprintf("\x1b[<0;%d;%dM\x1b[<0;%d;%dm", x, y, x, y))
}

// reset leaves whatever the last key opened: the chat's way back, then esc for
// a confirm, a form, a sheet or the search line.
func (w *walker) reset() {
	w.t.Helper()
	w.press("ctrl+a", "m", "esc", "esc")
}

// quits are the keys that end the program, as on any host. The walk does not
// press them: TestQAndCtrlCEnd does.
var quits = []string{"q", "ctrl+c"}

// everyKey is each key of defaultKeys but those that quit.
func everyKey() []string {
	return slices.DeleteFunc(app.KeyNames(), func(k string) bool { return slices.Contains(quits, k) })
}

// rowsWalked are how many rows down each tab's row keys are pressed on, and
// rowKeys the keys whose act depends on the row.
const rowsWalked = 4

var rowKeys = []string{"enter", "x", "X", "Q", "z", "A", "r", "a", "d", "space", "y", "ctrl+o", "o"}

// walkKeys presses every key on a tab, each followed by the keys that consent
// to what it opened: Enter twice, then y twice, as a kill of a session that
// works asks twice. Then it takes the keys of a row to each row.
func (w *walker) walkKeys(tab string) {
	w.t.Helper()
	top := slices.Repeat([]string{"k"}, rowsWalked)
	for _, k := range everyKey() {
		for _, consent := range []string{"enter", "y"} {
			w.press(tab, k, consent, consent)
			w.reset()
		}
	}
	for _, k := range rowKeys {
		for row := 1; row < rowsWalked; row++ {
			w.press(tab)
			w.press(top...)
			w.press(slices.Repeat([]string{"j"}, row)...)
			w.press(k, "y", "y")
			w.reset()
		}
	}
}

// walkTaps taps each hint of the key line's two rows, and each row of the
// screen twice, as a second tap acts.
func (w *walker) walkTaps(tab string) {
	w.t.Helper()
	for x := 1; x <= w.cols; x += 2 {
		for _, y := range []int{w.rows, w.rows - 1} {
			w.press(tab)
			w.tap(x, y)
			w.press("y")
			w.reset()
		}
	}
	for y := 1; y <= w.rows; y++ {
		w.press(tab)
		w.tap(20, y)
		w.tap(20, y)
		w.press("y")
		w.reset()
	}
}

// refused presses keys, waits for the line and clears it.
func (w *walker) refused(what string, keys ...string) {
	w.t.Helper()
	w.says(what, keys...)
	w.clear()
}

// says presses keys and waits for the line. A phone draws it on two rows, so
// there the stream holds the end of it whole. In a chat esc is the agent's, so
// the line stays until the chat is left.
func (w *walker) says(what string, keys ...string) {
	w.t.Helper()
	want := demo.Refusal
	if len(want)+1 > w.cols {
		_, want, _ = strings.Cut(want, ". ")
	}
	w.saysAfter(what, want, keys...)
}

// takes presses keys and waits for an act of the world that holds want.
func (w *walker) takes(what, want string, keys ...string) {
	w.t.Helper()
	before := len(w.d.Ran())
	w.press(keys...)
	eventually(w.t, 3*time.Second, what+" to take "+want, func() bool {
		return slices.ContainsFunc(w.d.Ran()[before:], func(argv []string) bool { return strings.Contains(strings.Join(argv, " "), want) })
	})
}

// scrolls opens the chat of the second session from the list, then turns the
// wheel over the chat, at the screen's right where a wide screen draws it, and
// presses the page keys. The chat is a window on the agent's pane, so each goes
// to the agent: the world takes it, and tmux is never asked if the agent reads
// the mouse.
func (w *walker) scrolls(where string) {
	w.t.Helper()
	before, from := len(w.d.Ran()), len(w.term.text())
	defer func() {
		if w.t.Failed() {
			w.t.Logf("it drew:\n%s", w.term.text()[from:])
		}
	}()
	w.press("j", "enter")
	x := w.cols - 5
	w.send("wheel up "+where, fmt.Sprintf("\x1b[<64;%d;10M", x))
	w.send("wheel down "+where, fmt.Sprintf("\x1b[<65;%d;10M", x))
	w.press("pgup", "pgdown")
	eventually(w.t, 30*time.Second, "the wheel and the page keys "+where+" to reach the agent", func() bool {
		wheels, pages := 0, 0
		for _, argv := range w.d.Ran()[before:] {
			switch {
			case slices.Contains(argv, "-H"):
				wheels++
			case slices.Contains(argv, "PPage"), slices.Contains(argv, "NPage"):
				pages++
			}
		}
		return wheels >= 2 && pages >= 2
	})
}

// walkTheDoors takes each door a person meets on purpose, on one demo, and each
// must end in its own act or the line. A stoker that runs asks to take over
// before any act on a session, so the kill comes before it, and the night after
// the order.
func (w *walker) walkTheDoors() {
	w.t.Helper()
	w.press("1")
	w.scrolls("in a chat")
	w.reset()
	w.clear()
	w.refused("a pane", "6", "enter")
	w.refused("a worktree's shell", "4", "enter")
	w.press("3")
	w.press(slices.Repeat([]string{"j"}, 9)...)
	w.refused("Pair a device", "enter")

	w.press("1", "N")
	w.send("a repository", "example/cloned")
	w.refused("a clone", "enter")
	w.press("1", "N")
	w.send("a folder", "/work/flint-auth")
	w.press("tab")
	w.send("a branch", "a-branch")
	w.refused("a new worktree", "enter")

	w.press("1", "N")
	w.send("a folder", "/work/flint-auth")
	w.press("tab", "tab")
	w.send("a name", "walked-one")
	w.takes("a new session", "new-session", "enter")
	w.clear()

	w.press("1", "/")
	w.send("a search", "spark-docs")
	w.press("enter", "r", "backspace")
	w.send("a name", "walked-two")
	w.takes("a rename", "rename-window", "enter")
	w.clear()
	w.press("esc", "esc")

	w.press("2", "k", "k", "k", "k", "j")
	w.takes("a merge", "gh pr merge", "x", "y")
	w.clear()

	w.press("4", "X")
	w.takes("a merged worktree's removal", "worktree remove", "x", "y")
	w.clear()

	w.press("1", "/")
	w.send("a search", "hearth-api")
	w.takes("a kill", "kill-pane", "enter", "x", "y", "y")
	w.press("esc", "esc")
	w.clear()

	w.press("7", "f", "enter")
	w.clear()
	w.opens("the order sheet", "How should I run?", "o")
	w.send("an order", "keep the tests green")
	w.saysAfter("an order", "“keep the tests green”", "enter")
	w.clear()
	w.press("f")
	w.clear()
	w.press("n")
	w.saysAfter("a night", "NIGHT", "enter")
}

// walkOnePhone asks the walk for the phone's size alone, in a child that runs
// with a tracer or in a namespace.
const walkOnePhone = "MATCHBLOX_DEMO_WALK_ONE_SIZE"

// walk is everything a person can press. The doors first, on one demo for each
// size. Then a sweep: a fresh demo for each tab, so that a kill on one leaves
// the next its sessions, on a phone and on a wide screen, where the hints and
// the taps differ. The keys are the same on both.
func walk(t *testing.T) (saw []string) {
	t.Helper()
	sizes := [][2]int{{44, 31}, {132, 32}}
	if os.Getenv(walkOnePhone) != "" {
		sizes = sizes[:1]
	}
	for _, size := range sizes {
		w := startWalk(t, size[0], size[1])
		w.walkTheDoors()
		w.alive()
		saw = append(saw, w.spy.seen()...)
		w.d.Stop()
		for _, tab := range []string{"1", "2", "3", "4", "5", "6", "7"} {
			w := startWalk(t, size[0], size[1])
			if size[0] == 44 {
				w.walkKeys(tab)
			}
			w.walkTaps(tab)
			time.Sleep(200 * time.Millisecond) // what the service still answers
			w.alive()
			saw = append(saw, w.spy.seen()...)
			w.d.Stop()
		}
	}
	return saw
}

// theWalk is what the one walk of a test run found. It takes a while, and two
// tests read it.
var theWalk struct {
	sync.Once
	finished            bool
	saw, resolved, open []string
	spawned             string
	written             []string
	leftInTemp          []os.DirEntry
}

// walked runs the walk once, with the person's machine empty.
func walked(t *testing.T) {
	t.Helper()
	theWalk.Do(func() {
		m := newMachine(t)
		resolved := noResolver(t)
		held := watchSockets(t)
		saw := walk(t)
		left, err := os.ReadDir(m.temp)
		if err != nil {
			t.Fatal(err)
		}
		theWalk.saw, theWalk.leftInTemp = saw, left
		theWalk.resolved, theWalk.open = resolved(), held()
		theWalk.spawned, theWalk.written = m.spawns(t), m.written(t)
		theWalk.finished = true
	})
	if !theWalk.finished {
		t.Fatal("the walk did not finish: the test that ran it says why")
	}
}

func TestNoDoorOrKeyLeavesTheDemo(t *testing.T) {
	walked(t)
	if saw := theWalk.saw; len(saw) > 0 {
		t.Errorf("%d commands reached the console's seams, the first of them:\n%s", len(saw), strings.Join(saw[:min(len(saw), 20)], "\n"))
	}
	if theWalk.spawned != "" {
		t.Errorf("a real command ran:\n%s", theWalk.spawned)
	}
	if theWalk.resolved != nil {
		t.Errorf("the demo looked up a name: %v", theWalk.resolved)
	}
	if theWalk.open != nil {
		t.Errorf("the demo held a socket open: %v", theWalk.open)
	}

	// The edge swipe, the ‹ and ⌘M all write the home key. In a chat q is the
	// agent's, so q ending the program proves the chat gave back the keys.
	t.Run("the home key leaves a chat", func(t *testing.T) {
		newMachine(t)
		w := startWalk(t, 44, 31)
		w.press("enter")
		w.saysAfter("ctrl+c in a chat", "This is a demo.", "ctrl+c") // the keys are in the chat
		w.send("the home key", panes.HomeKey)
		w.press("q")
		select {
		case <-w.ended:
		case <-time.After(5 * time.Second):
			t.Fatal("q did not end the program after the home key: the chat kept the keys")
		}
		if err := w.d.Wait(); err != nil {
			t.Errorf("q ended it with %v", err)
		}
	})
}

// TestQAndCtrlCEnd: the two keys that end the program, as on any host, and they
// end it cleanly. In a chat ctrl+c is the agent's, which a phone cannot send: the
// line answers and the program runs on.
func TestQAndCtrlCEnd(t *testing.T) {
	newMachine(t)
	for _, key := range quits {
		w := startWalk(t, 44, 31)
		w.pressLast(key)
		select {
		case <-w.ended:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s did not end the program", key)
		}
		if err := w.d.Wait(); err != nil {
			t.Errorf("%s ended it with %v", key, err)
		}
	}
	w := startWalk(t, 132, 32)
	w.press("1", "enter")
	w.flashes("ctrl+c in a chat", "ctrl+c")
	w.alive()
}

func TestTheDemoWritesOnlyItsTempFolder(t *testing.T) {
	walked(t)
	if got := theWalk.written; len(got) > 0 {
		t.Errorf("the demo wrote outside its temp folder:\n%s", strings.Join(got, "\n"))
	}
	if len(theWalk.leftInTemp) > 0 {
		t.Errorf("the temp folder keeps %v after the demo stopped", theWalk.leftInTemp)
	}
}

// childLimit stops a child's walk well inside the Linux job's ten minutes.
const childLimit = "-test.timeout=4m"

// walkInChild is the arguments and environment of this test binary running the
// walk at one size.
func walkInChild(t *testing.T) (self string, args, env []string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return self, []string{"-test.run", "^TestNoDoorOrKeyLeavesTheDemo$", "-test.count=1", childLimit}, append(os.Environ(), walkOnePhone+"=1")
}

// TestTheDemoOpensNoNetwork runs the walk in a network namespace with no
// interface up. It proves the demo does not need the network: a dial fails at
// once there, and the demo would tolerate that. TestTheDemoMakesNoSocketOrCommand
// proves none is tried.
func TestTheDemoOpensNoNetwork(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("a network namespace is Linux's")
	}
	if err := exec.Command("unshare", "-rn", "true").Run(); err != nil {
		// In CI a skip would pass unseen, and the spec asks for this test.
		if os.Getenv("CI") != "" {
			t.Fatalf("unshare -rn is not permitted here, so the walk does not run with no route out: %v", err)
		}
		t.Skipf("unshare -rn is not permitted here, so the walk does not run with no route out: %v", err)
	}
	self, args, env := walkInChild(t)
	cmd := exec.Command("unshare", slices.Concat([]string{"-rn", self}, args)...) //nolint:gosec // this test binary
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("the walk failed with no route out: %v\n%s", err, out)
	}
}

// TestTheDemoMakesNoSocketOrCommand traces the walk: it fails on any socket or
// connect call, tried or done, and on any execve but the test binary's own.
func TestTheDemoMakesNoSocketOrCommand(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("strace is Linux's")
	}
	// In CI a skip would pass unseen, and this is the proof that matters.
	skip := t.Skipf
	if os.Getenv("CI") != "" {
		skip = t.Fatalf
	}
	if _, err := exec.LookPath("strace"); err != nil {
		skip("strace is not on the path, so the walk is not traced")
	}
	self, args, env := walkInChild(t)
	if err := exec.Command("strace", "-f", "-qq", "-o", os.DevNull, "-e", "trace=execve", self, "-test.run=^$", childLimit).Run(); err != nil { //nolint:gosec // this test binary
		skip("strace cannot trace here (ptrace refused?), so the walk is not traced: %v", err)
	}
	trace := filepath.Join(t.TempDir(), "trace")
	cmd := exec.Command("strace", slices.Concat([]string{"-f", "-qq", "-o", trace, "-e", "trace=socket,connect,execve", "-e", "signal=none", self}, args)...) //nolint:gosec // this test binary
	cmd.Env = env
	started := time.Now()
	out, err := cmd.CombinedOutput()
	t.Logf("the traced walk took %v", time.Since(started).Round(time.Second))
	if err != nil {
		t.Fatalf("the traced walk failed: %v\n%s", err, out)
	}
	b, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	if bad := traceFindings(string(b), self); len(bad) > 0 {
		t.Errorf("the walk left the demo:\n%s", strings.Join(bad, "\n"))
	}
}

// traceFindings reads strace's log of socket, connect and execve: each line of
// them but the one execve of self, and that one's absence, which means the trace
// saw nothing.
func traceFindings(trace, self string) []string {
	var bad []string
	own := 0
	for _, line := range strings.Split(trace, "\n") {
		switch {
		case strings.Contains(line, `execve("`+self+`"`) && own == 0:
			own++
		case strings.Contains(line, "execve("), strings.Contains(line, "socket("), strings.Contains(line, "connect("):
			bad = append(bad, line)
		}
	}
	if own == 0 {
		bad = append(bad, "the trace holds no execve of the test binary, so it saw nothing")
	}
	return bad
}

func TestATraceIsReadForWhatLeaves(t *testing.T) {
	own := `1 execve("/t/demo.test", ["x"], 0x1) = 0` + "\n"
	for name, c := range map[string]struct {
		trace string
		bad   int
	}{
		"only its own":   {own + "2 +++ exited with 0 +++\n", 0},
		"nothing at all": {"", 1},
		"a socket":       {own + "2 socket(AF_INET, SOCK_STREAM, 0) = 3\n", 1},
		"a failed dial":  {own + "2 connect(3, {}, 16) = -1 ENETUNREACH\n", 1},
		"a command":      {own + `2 execve("/usr/bin/git", ["git"], 0x1) = -1 ENOENT` + "\n", 1},
		"its own, again": {own + own, 1},
	} {
		if got := traceFindings(c.trace, "/t/demo.test"); len(got) != c.bad {
			t.Errorf("%s: %q", name, got)
		}
	}
}

// noResolver makes the default resolver tell on every name it is asked for,
// and answers with a failure.
func noResolver(t *testing.T) func() []string {
	t.Helper()
	var mu sync.Mutex
	var dialed []string
	was := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(_ context.Context, network, address string) (net.Conn, error) {
		mu.Lock()
		defer mu.Unlock()
		dialed = append(dialed, network+" "+address)
		return nil, errors.New("no network in the demo")
	}}
	t.Cleanup(func() { net.DefaultResolver = was })
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(dialed)
	}
}

// watchSockets looks every few milliseconds for a socket this process holds
// open, beyond any it held at the start. net.Pipe, which joins the console and
// the service, is no socket. Linux only: elsewhere it sees nothing.
func watchSockets(t *testing.T) func() []string {
	t.Helper()
	before := sockets()
	var mu sync.Mutex
	var seen []string
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case <-time.After(5 * time.Millisecond):
			}
			for _, s := range sockets() {
				if !slices.Contains(before, s) {
					mu.Lock()
					seen = append(seen, s)
					mu.Unlock()
				}
			}
		}
	}()
	var once sync.Once
	finish := func() { once.Do(func() { close(stop); <-done }) }
	t.Cleanup(finish)
	return func() []string {
		finish()
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(seen)
	}
}

func sockets() []string {
	fds, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return nil
	}
	var out []string
	for _, fd := range fds {
		if l, err := os.Readlink("/proc/self/fd/" + fd.Name()); err == nil && strings.HasPrefix(l, "socket:") {
			out = append(out, l)
		}
	}
	return out
}

// flashes presses keys and waits for the line in what the console then draws.
func (w *walker) flashes(what string, keys ...string) {
	w.t.Helper()
	w.saysAfter(what, demo.Refusal, keys...)
}

// screen is the whole screen as the console draws it now, or what it drew
// since the last whole frame when none comes.
func (w *walker) screen() string {
	n, frame := w.term.wholeFrame()
	w.d.Repaint()
	for range 200 {
		if m, f := w.term.wholeFrame(); m > n {
			return f
		}
		time.Sleep(5 * time.Millisecond)
	}
	return frame
}

// clear takes the line off the screen and waits for a whole frame to show it
// gone: the console draws only what changed, so the next refusal draws again.
// A wait of a set time let a slow frame keep the line, and the next refusal
// then drew nothing (#318).
func (w *walker) clear() {
	w.t.Helper()
	_, line, _ := strings.Cut(demo.Refusal, ". ")
	w.press("esc")
	deadline, again := time.Now().Add(30*time.Second), time.Now().Add(2*time.Second)
	for {
		n, _ := w.term.wholeFrame()
		w.d.Repaint()
		m, frame := w.term.wholeFrame()
		for m == n && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
			m, frame = w.term.wholeFrame()
		}
		if m > n && !strings.Contains(frame, line) {
			return
		}
		if time.Now().After(deadline) {
			w.t.Fatalf("esc never took the line off the screen after %q; the last whole frame:\n%s", w.steps[max(len(w.steps)-3, 0):], frame)
		}
		// The frame still holds the line: the esc is not in yet, or the
		// line answers an act the world ran before it and came after the esc.
		if time.Now().After(again) {
			w.press("esc")
			again = time.Now().Add(2 * time.Second)
		}
	}
}

// opens presses key until the console draws want. The key can come before
// the state it needs, such as the stoker that the last Enter starts through
// the service: then it only flashes a hint, and keys typed next run as list
// keys (#344). A flash is cleared and the key pressed again.
func (w *walker) opens(what, want, key string) {
	w.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		from := len(w.term.text())
		w.press(key)
		for wait := time.Now().Add(2 * time.Second); time.Now().Before(wait); time.Sleep(5 * time.Millisecond) {
			if strings.Contains(w.term.text()[from:], want) {
				return
			}
		}
		if time.Now().After(deadline) {
			w.t.Fatalf("%s: %q never drew %q; the screen:\n%s", what, key, want, w.screen())
		}
		w.clear()
	}
}

// saysAfter presses keys and waits for want in what the console then draws.
func (w *walker) saysAfter(what, want string, keys ...string) {
	w.t.Helper()
	from := len(w.term.text())
	w.press(keys...)
	deadline := time.Now().Add(30 * time.Second)
	for !strings.Contains(w.term.text()[from:], want) {
		if time.Now().After(deadline) {
			w.t.Fatalf("%s: never drew %q after %q; the screen:\n%s", what, want, w.steps[max(len(w.steps)-12, 0):], w.screen())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestWhatThePhoneCannotDoSaysSo(t *testing.T) {
	m := newMachine(t)
	w := startWalk(t, 132, 32)
	w.flashes("a pane", "6", "enter")
	w.clear()
	w.flashes("a worktree's shell", "4", "enter")
	w.clear()

	// The Machine tab's last row is its device row.
	w.press("3", "j", "j", "j", "j", "j", "j", "j", "j", "j")
	w.flashes("Pair a device", "enter")
	w.clear()

	w.press("2", "j") // #214 waits for its merge
	w.flashes("a merge", "x", "y")
	w.clear()

	w.press("1", "N")
	w.send("a folder", "/work/flint-auth")
	w.press("tab")
	w.send("a branch", "a-branch")
	w.flashes("a new worktree", "enter", "y")
	w.clear()

	w.press("N")
	w.send("a repository", "example/cloned")
	w.flashes("a clone", "enter")
	w.clear()

	if saw := w.spy.seen(); len(saw) > 0 {
		t.Errorf("commands reached the console's seams: %v", saw)
	}
	if got := m.spawns(t); got != "" {
		t.Errorf("a real command ran:\n%s", got)
	}
	w.alive()
}

func TestANightStartsAndRunsNothing(t *testing.T) {
	m := newMachine(t)
	w := startWalk(t, 132, 32)
	w.press("7", "n")
	w.saysAfter("a night", "the stoker runs the night", "enter")
	if saw := w.spy.seen(); len(saw) > 0 {
		t.Errorf("commands reached the console's seams: %v", saw)
	}
	if got := m.spawns(t); got != "" {
		t.Errorf("a real command ran:\n%s", got)
	}
}

// paneList is tmux's list on the demo's temp copy.
func paneList(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(tempCopies(t)[0], "tmux-panes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSessionsCanBeStartedAndKilled(t *testing.T) {
	m := newMachine(t)
	w := startWalk(t, 132, 32)

	w.press("N")
	w.send("a folder", "/work/flint-auth")
	w.press("tab", "tab")
	w.send("a name", "fresh-one")
	w.saysAfter("a new session", "fresh-one", "enter")
	w.term.waitFor(t, "started fresh-one")
	if got := paneList(t); !strings.Contains(got, "fresh-one") {
		t.Errorf("tmux's list has no new session:\n%s", got)
	}
	// The search finds the session only once the console's list holds it: on
	// a slow machine its sample comes after the word that it started.
	eventually(t, 30*time.Second, "the list to show fresh-one", func() bool {
		return slices.ContainsFunc(strings.Split(w.screen(), "\n"), func(l string) bool {
			return strings.Contains(l, "fresh-one") && !strings.Contains(l, "started fresh-one")
		})
	})

	// The new session takes a message like any other.
	w.press("/")
	w.send("a search", "fresh-one")
	w.press("enter", "enter")
	w.term.waitFor(t, "keys go to fresh-one")
	time.Sleep(500 * time.Millisecond) // its screen comes: a key answers only what the console showed
	w.send("a message", "hello")
	from := len(w.term.text())
	w.press("enter")
	time.Sleep(300 * time.Millisecond)
	if drawn := w.term.text()[from:]; strings.Contains(drawn, "no such file") || strings.Contains(drawn, "failed") {
		t.Errorf("a message to a new session failed:\n%s", drawn)
	}
	w.reset()

	// spark-docs rests, so it renames at once.
	w.press("/")
	w.send("a search", "spark-docs")
	w.press("enter", "r")
	for range len("spark-docs") + 10 {
		w.press("backspace")
	}
	w.send("a name", "renamed-one")
	w.press("enter")
	eventually(t, 3*time.Second, "the rename to reach tmux's list", func() bool { return strings.Contains(paneList(t), "renamed-one") })
	w.press("esc")
	w.saysAfter("the list", "renamed-one", "/")
	w.press("esc")

	// The search finds the new name once a sample has read it: until then it finds
	// no row, and x does nothing.
	eventually(t, 5*time.Second, "the kill to leave tmux's list", func() bool {
		w.press("/")
		w.send("a search", "renamed-one")
		w.press("enter", "x", "y", "y", "esc", "esc")
		time.Sleep(300 * time.Millisecond)
		return !strings.Contains(paneList(t), "renamed-one")
	})
	// Another tab and back draws the list anew: it has the new session, not the
	// killed one.
	eventually(t, 5*time.Second, "the drawn list to lose the killed session", func() bool {
		w.press("2")
		time.Sleep(200 * time.Millisecond)
		from := len(w.term.text())
		w.press("1")
		time.Sleep(200 * time.Millisecond)
		drawn := w.term.text()[from:]
		return strings.Contains(drawn, "fresh-one") && !strings.Contains(drawn, "renamed-one")
	})
	if got := m.spawns(t); got != "" {
		t.Errorf("a real command ran:\n%s", got)
	}
	w.alive()
}
