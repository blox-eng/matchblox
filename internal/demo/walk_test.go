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
	"ctrl+a": "\x01", "ctrl+c": "\x03", "ctrl+o": "\x0f",
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

// quits are the keys that end the program: the walk presses them last.
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

// walkTheDoors takes the doors a person meets on purpose: the launch sheet,
// plain, with a repository to clone and with a branch for a worktree; a rename;
// the stoker's night and its order; a merge; and the chat's own keys.
func (w *walker) walkTheDoors() {
	w.t.Helper()
	w.press("1", "N")
	w.send("a folder", "/work/flint-auth")
	w.press("tab", "tab")
	w.send("a name", "walked-one")
	w.press("enter")
	time.Sleep(100 * time.Millisecond)
	w.reset()

	w.press("1", "N")
	w.send("a repository", "example/cloned")
	w.press("enter")
	w.reset()

	w.press("1", "N")
	w.send("a folder", "/work/flint-auth")
	w.press("tab")
	w.send("a branch", "a-branch")
	w.press("enter", "y")
	time.Sleep(100 * time.Millisecond)
	w.reset()

	w.press("1", "/")
	w.send("a search", "spark-docs")
	w.press("enter", "r", "backspace")
	w.send("a name", "walked-two")
	w.press("enter")
	time.Sleep(100 * time.Millisecond)
	w.reset()

	w.press("7", "n", "enter", "enter", "y")
	w.reset()
	w.press("7", "o", "enter", "enter")
	w.reset()

	w.press("2", "k", "k", "k", "k", "j", "x", "y")
	w.reset()

	w.press("1", "enter", "ctrl+o", "y")
	w.reset()
}

// walk is everything a person can press: a fresh demo for each tab, so that a
// kill on one leaves the next its sessions, on a phone and on a wide screen,
// where the hints and the taps differ. The keys are the same on both.
func walk(t *testing.T) (saw []string, ran [][]string) {
	t.Helper()
	for _, size := range [][2]int{{44, 31}, {132, 32}} {
		for _, tab := range []string{"1", "2", "3", "4", "5", "6", "7"} {
			w := startWalk(t, size[0], size[1])
			if size[0] == 44 {
				w.walkKeys(tab)
			}
			w.walkTaps(tab)
			w.walkTheDoors()
			time.Sleep(200 * time.Millisecond) // what the service still answers
			w.alive()
			saw, ran = append(saw, w.spy.seen()...), append(ran, w.d.Ran()...)
			w.d.Stop()
		}
	}
	return saw, ran
}

// theWalk is what the one walk of a test run found. It takes a while, and two
// tests read it.
var theWalk struct {
	sync.Once
	finished            bool
	saw, resolved, open []string
	ran                 [][]string
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
		saw, ran := walk(t)
		left, err := os.ReadDir(m.temp)
		if err != nil {
			t.Fatal(err)
		}
		theWalk.saw, theWalk.ran, theWalk.leftInTemp = saw, ran, left
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
	for _, want := range []string{"gh pr merge", "new-session", "kill-pane", "rename-window", "git -C /work/hearth worktree remove"} {
		if !slices.ContainsFunc(theWalk.ran, func(argv []string) bool { return strings.Contains(strings.Join(argv, " "), want) }) {
			t.Errorf("the walk never took %q, so it proves little", want)
		}
	}
	if theWalk.resolved != nil {
		t.Errorf("the demo looked up a name: %v", theWalk.resolved)
	}
	if theWalk.open != nil {
		t.Errorf("the demo held a socket open: %v", theWalk.open)
	}
}

// TestQEnds: q is the one key that ends the program, and it ends it cleanly.
func TestQEnds(t *testing.T) {
	newMachine(t)
	w := startWalk(t, 44, 31)
	w.press("q")
	select {
	case <-w.ended:
	case <-time.After(5 * time.Second):
		t.Fatal("q did not end the program")
	}
	if err := w.d.Wait(); err != nil {
		t.Errorf("q ended it with %v", err)
	}
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

// TestTheDemoOpensNoNetwork runs the walk again in a network namespace with no
// interface up. A walk that needs the network fails there.
func TestTheDemoOpensNoNetwork(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("a network namespace is Linux's")
	}
	if err := exec.Command("unshare", "-rn", "true").Run(); err != nil {
		t.Skipf("unshare -rn is not permitted here, so the walk does not run with no route out: %v", err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("unshare", "-rn", self, "-test.run", "^TestNoDoorOrKeyLeavesTheDemo$", "-test.count=1").CombinedOutput() //nolint:gosec // this test binary
	if err != nil {
		t.Errorf("the walk failed with no route out: %v\n%s", err, out)
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

// clear takes the line off the screen and waits for a frame to show it gone:
// the console draws only what changed, so the next refusal draws again.
func (w *walker) clear() {
	w.t.Helper()
	w.press("esc")
	time.Sleep(300 * time.Millisecond)
}

// saysAfter presses keys and waits for want in what the console then draws.
func (w *walker) saysAfter(what, want string, keys ...string) {
	w.t.Helper()
	from := len(w.term.text())
	w.press(keys...)
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(w.term.text()[from:], want) {
		if time.Now().After(deadline) {
			w.t.Fatalf("%s: never drew %q; it drew:\n%s", what, want, w.term.text()[from:])
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestWhatThePhoneCannotDoSaysSo(t *testing.T) {
	m := newMachine(t)
	w := startWalk(t, 132, 32)
	for _, tab := range []string{"1", "2", "3", "4", "5", "6", "7", "1"} {
		w.press(tab)
		w.flashes("ctrl+o on tab "+tab, "ctrl+o")
		w.clear()
	}
	w.flashes("ctrl+o in a chat", "enter", "ctrl+o")
	w.reset()
	w.clear()

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

	// The new session takes a message like any other.
	w.press("/")
	w.send("a search", "fresh-one")
	w.press("enter", "enter")
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

	w.press("/")
	w.send("a search", "renamed-one")
	w.press("enter", "x", "y", "y")
	w.press("esc", "esc")
	eventually(t, 3*time.Second, "the kill to leave tmux's list", func() bool { return !strings.Contains(paneList(t), "renamed-one") })
	if got := m.spawns(t); got != "" {
		t.Errorf("a real command ran:\n%s", got)
	}
	w.alive()
}
