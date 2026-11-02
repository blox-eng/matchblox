package demo_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/demo"
)

const tree = "../../testdata/demo"

// terminal is both ends of the view that the app gives the console.
type terminal struct {
	in  *io.PipeWriter
	out *io.PipeReader

	mu    sync.Mutex
	drawn bytes.Buffer
	ended chan struct{}
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
	d, err := demo.Start(demo.Options{Tree: tree, In: inR, Out: outW, Cols: cols, Rows: rows})
	if err != nil {
		return nil, term, err
	}
	t.Cleanup(d.Stop)
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

func (term *terminal) waitFor(t *testing.T, want ...string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		text := term.text()
		missing := ""
		for _, w := range want {
			if !strings.Contains(text, w) {
				missing = w
				break
			}
		}
		if missing == "" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the console never drew %q; it drew:\n%s", missing, text)
		}
		time.Sleep(10 * time.Millisecond)
	}
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
	term.waitFor(t, "SESSIONS 7", "hearth-api")
}

func TestStopLeavesNothing(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	d, term := start(t)
	term.waitFor(t, "hearth-api")
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
	term.waitFor(t, "hearth-api")
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
	first.waitFor(t, "SESSIONS 7", "start a new session")

	if _, _, err := tryStart(t); !errors.Is(err, demo.ErrRunning) {
		t.Errorf("a second Start with no Stop: want ErrRunning, got %v", err)
	}
	if left := tempCopies(t); len(left) != 1 {
		t.Errorf("the refused Start touched the running copy: %v", left)
	}
	if _, err := first.in.Write([]byte("2")); err != nil {
		t.Fatal(err)
	}
	first.waitFor(t, "SHIPS") // the first one still runs
	d.Stop()

	_, second := start(t)
	second.waitFor(t, "SESSIONS 7", "start a new session")
	if a, b := firstScreen(first.text()), firstScreen(second.text()); a != b {
		t.Errorf("the first screens differ:\n--- first\n%s\n--- after the restart\n%s", a, b)
	}
}

// firstScreen is the sessions list as first drawn: the logo before it and the
// spark of a working session turn with the clock, and differ from run to run.
func firstScreen(text string) string {
	_, list, _ := strings.Cut(text, "SESSIONS 7")
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
