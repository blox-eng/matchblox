package demo_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/app"
	"github.com/blox-eng/matchblox/internal/demo"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/transport"
)

const tree = "../../testdata/demo"

// terminal is both ends of the view that the app gives the console.
type terminal struct {
	in  *io.PipeWriter
	out *io.PipeReader
	d   *demo.Demo // to ask for a whole frame; nil until the demo starts

	mu     sync.Mutex
	drawn  bytes.Buffer
	states []string // what each state from the service held, for a failure to name (#321)
	ended  chan struct{}
}

// heard records each state the console receives.
type heard struct {
	transport.Conn
	term  *terminal
	start time.Time
}

func (h heard) Recv() (proto.Envelope, error) {
	env, err := h.Conn.Recv()
	if err == nil && env.Kind == proto.KindSnapshot {
		var st proto.State
		line := "a state that does not parse"
		if json.Unmarshal(env.Body, &st) == nil {
			line = fmt.Sprintf("%d sessions, errors %q", len(st.Sessions), st.Errors)
		}
		h.term.mu.Lock()
		h.term.states = append(h.term.states, fmt.Sprintf("%6dms %s", time.Since(h.start).Milliseconds(), line))
		h.term.mu.Unlock()
	}
	return env, err
}

func start(t *testing.T) (*demo.Demo, *terminal) {
	t.Helper()
	return startAt(t, 44, 31)
}

func startAt(t *testing.T, cols, rows int) (*demo.Demo, *terminal) {
	t.Helper()
	d, term, err := tryStartAt(t, cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	return d, term
}

func tryStart(t *testing.T) (*demo.Demo, *terminal, error) {
	t.Helper()
	return tryStartAt(t, 44, 31)
}

func tryStartAt(t *testing.T, cols, rows int) (*demo.Demo, *terminal, error) {
	t.Helper()
	return tryStartTree(t, tree, cols, rows)
}

func tryStartTree(t *testing.T, tree string, cols, rows int) (*demo.Demo, *terminal, error) {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	term := &terminal{in: inW, out: outR, ended: make(chan struct{})}
	go term.read()
	t.Cleanup(term.close)
	start := time.Now()
	o := demo.WithConsole(demo.Options{Tree: tree, In: inR, Out: outW, Cols: cols, Rows: rows}, func(a *app.Options) {
		a.Conn = heard{Conn: a.Conn, term: term, start: start}
	})
	d, err := demo.Start(o)
	if err != nil {
		return nil, term, err
	}
	t.Cleanup(d.Stop)
	term.d = d
	return d, term, nil
}

func (term *terminal) read() {
	defer close(term.ended)
	buf := make([]byte, 64*1024)
	for {
		n, err := term.out.Read(buf)
		term.mu.Lock()
		term.drawn.Write(buf[:n])
		term.mu.Unlock()
		if err != nil {
			return
		}
	}
}

func (term *terminal) close() {
	_ = term.in.Close()
	_ = term.out.Close()
}

func (term *terminal) raw() string {
	term.mu.Lock()
	defer term.mu.Unlock()
	return term.drawn.String()
}

func (term *terminal) text() string {
	term.mu.Lock()
	defer term.mu.Unlock()
	return ansi.Strip(term.drawn.String())
}

// wholeFrame is how many whole frames the console drew, and the text from the
// start of the last one. A whole frame clears the screen first, and comes in
// one write.
func (term *terminal) wholeFrame() (int, string) {
	const clear = "\x1b[2J"
	raw := term.raw()
	i := strings.LastIndex(raw, clear)
	if i < 0 {
		return 0, ""
	}
	return strings.Count(raw, clear), ansi.Strip(raw[i:])
}

// repaintAfter is how long a wait reads what the console wrote before it
// asks for a whole frame: a frame writes only the cells that change, so
// what changed may never be written whole. A second was most of a walk's
// time on the board, whose tabs and searches change a few cells each.
const repaintAfter = 150 * time.Millisecond

// waitFor waits until the console drew every want, in what it wrote or in a
// whole frame. The console writes only the cells that change, so a count it
// drew while it waited for the service turns from "BOARD 0" to "BOARD 7"
// by one cell, and what it wrote never holds "BOARD 7" whole (#321).
func (term *terminal) waitFor(t *testing.T, want ...string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	repaint := time.Now().Add(repaintAfter)
	for {
		text := term.text()
		_, frame := term.wholeFrame()
		missing := ""
		for _, w := range want {
			if !strings.Contains(text, w) && !strings.Contains(frame, w) {
				missing = w
				break
			}
		}
		if missing == "" {
			return
		}
		if time.Now().After(deadline) {
			term.mu.Lock()
			states := strings.Join(term.states, "\n")
			term.mu.Unlock()
			t.Fatalf("the console never drew %q; it drew:\n%s\nthe states it had:\n%s", missing, text, states)
		}
		if term.d != nil && time.Now().After(repaint) {
			term.d.Repaint()
			repaint = time.Now().Add(repaintAfter)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// screen is the whole screen as the console draws it now, or what it drew
// since the last whole frame when none comes.
func (term *terminal) screen() string {
	n, frame := term.wholeFrame()
	term.d.Repaint()
	for range 200 {
		if m, f := term.wholeFrame(); m > n {
			return f
		}
		time.Sleep(5 * time.Millisecond)
	}
	return frame
}

func tempCopies(t *testing.T) []string {
	t.Helper()
	found, err := filepath.Glob(filepath.Join(os.TempDir(), "matchblox-demo-*"))
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestTheConsoleDrawsTheMachine(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	_, term := start(t)
	term.waitFor(t, "BOARD 7", "flint-auth")
}

// late holds back the console's first state until the start mark has played,
// as a slow machine does: the console then draws "BOARD 0" while it waits.
type late struct {
	transport.Conn
	held *atomic.Bool
}

func (l late) Recv() (proto.Envelope, error) {
	env, err := l.Conn.Recv()
	if err == nil && env.Kind == proto.KindSnapshot && !l.held.Swap(true) {
		time.Sleep(time.Second)
	}
	return env, err
}

func TestAStateAfterTheStartMarkStillDrawsTheCount(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	term := &terminal{in: inW, out: outR, ended: make(chan struct{})}
	go term.read()
	t.Cleanup(term.close)
	var held atomic.Bool
	d, err := demo.Start(demo.WithConsole(demo.Options{Tree: tree, In: inR, Out: outW, Cols: 44, Rows: 31}, func(a *app.Options) {
		a.Conn = late{Conn: a.Conn, held: &held}
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Stop)
	term.d = d
	term.waitFor(t, "BOARD 7", "flint-auth")
}

func TestStopLeavesNothing(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	d, term := start(t)
	term.waitFor(t, "flint-auth")
	if len(tempCopies(t)) != 1 {
		t.Fatalf("while it runs, want one temp copy, got %v", tempCopies(t))
	}

	d.Stop()

	if left := tempCopies(t); len(left) != 0 {
		t.Errorf("Stop left %v", left)
	}
	waited := make(chan error, 1)
	go func() { waited <- d.Wait() }()
	select {
	case err := <-waited:
		if err != nil {
			t.Errorf("Wait after Stop: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after Stop")
	}
}

func TestAStopWithNoOneReadingStillEnds(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	d, term := start(t)
	term.waitFor(t, "flint-auth")
	// The app has gone: nothing reads the terminal or writes to it any more.
	term.close()

	stopped := make(chan struct{})
	go func() { d.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop hung on a terminal no one reads")
	}
}

func TestAStartSweepsWhatAKilledRunLeft(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	stale := filepath.Join(os.TempDir(), "matchblox-demo-killed")
	if err := os.MkdirAll(filepath.Join(stale, "proc"), 0o700); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(os.TempDir(), "matchblox-demos")
	if err := os.Mkdir(other, 0o700); err != nil {
		t.Fatal(err)
	}

	start(t)

	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the killed run's copy is still there: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("the sweep took a folder that is not a copy: %v", err)
	}
}

func TestAStartAfterAStopIsFresh(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	d, first := start(t)
	first.waitFor(t, "BOARD 7", "start a new session")
	firstSeen := first.screen()

	if _, _, err := tryStart(t); !errors.Is(err, demo.ErrRunning) {
		t.Errorf("a second Start with no Stop: want ErrRunning, got %v", err)
	}
	if left := tempCopies(t); len(left) != 1 {
		t.Errorf("the refused Start touched the running copy: %v", left)
	}
	if _, err := first.in.Write([]byte("\t")); err != nil {
		t.Fatal(err)
	}
	first.waitFor(t, "MACHINE") // the first one still runs
	d.Stop()

	_, second := start(t)
	second.waitFor(t, "BOARD 7", "start a new session")
	if a, b := firstScreen(firstSeen), firstScreen(second.screen()); a != b {
		t.Errorf("the first screens differ:\n--- first\n%s\n--- after the restart\n%s", a, b)
	}
}

// firstScreen is the sessions list in a whole frame: the logo before it and
// the spark of a working session turn with the clock, and differ from run to
// run.
func firstScreen(text string) string {
	_, list, _ := strings.Cut(text, "BOARD 7")
	list, _, _ = strings.Cut(list, "start a new session")
	return strings.ReplaceAll(list, "✧", "✦")
}

func TestStartNamesWhatFailed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a folder's mode does not stop a write on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes to a read-only folder")
	}
	readOnly := filepath.Join(t.TempDir(), "read-only")
	if err := os.Mkdir(readOnly, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", readOnly)

	_, _, err := tryStart(t)
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), readOnly) {
		t.Errorf("the error does not name %s: %v", readOnly, err)
	}

	t.Setenv("TMPDIR", t.TempDir())
	if _, _, err := tryStart(t); err != nil {
		t.Errorf("a failed Start must not hold the single instance: %v", err)
	}
}
