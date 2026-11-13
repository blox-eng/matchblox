//go:build linux || darwin

package guard

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

// helperEnv names what this test binary does when the guard runs it in
// place of an agent.
const helperEnv = "MATCHBLOX_GUARD_HELPER"

func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "":
		os.Exit(m.Run())
	case "flood":
		helperFlood()
	}
	os.Exit(0)
}

// helperFlood replays without end, as a Cursor with a very long chat, and
// writes the first three keys it reads to the file named in its argument.
func helperFlood() {
	if _, err := term.MakeRaw(os.Stdin.Fd()); err != nil {
		os.Exit(2)
	}
	fmt.Print(ClearCodes)
	go func() {
		line := []byte(strings.Repeat("replayed ", 20) + "\r\n")
		for {
			_, _ = os.Stdout.Write(line)
		}
	}()
	keys := make([]byte, 3)
	for n := 0; n < len(keys); {
		k, err := os.Stdin.Read(keys[n:])
		if err != nil {
			os.Exit(3)
		}
		n += k
	}
	if err := os.WriteFile(os.Args[1], keys, 0o600); err != nil {
		os.Exit(4)
	}
}

// screen is the builder's side of the terminal the guard runs in: what the
// pane shows, and where keys are typed.
type screen struct {
	t      *testing.T
	master *os.File
	slave  *os.File
	mu     sync.Mutex
	got    bytes.Buffer
}

func newScreen(t *testing.T, cols, rows int) *screen {
	t.Helper()
	master, slave, err := openPTY()
	if err != nil {
		t.Fatalf("no terminal for the test: %v", err)
	}
	s := &screen{t: t, master: master, slave: slave}
	s.size(cols, rows)
	go func() {
		buf := make([]byte, 64<<10)
		for {
			n, err := master.Read(buf)
			s.mu.Lock()
			s.got.Write(buf[:n])
			s.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() { _ = slave.Close(); _ = master.Close() })
	return s
}

func (s *screen) size(cols, rows int) {
	s.t.Helper()
	if err := unix.IoctlSetWinsize(int(s.master.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(cols), Row: uint16(rows)}); err != nil { //nolint:gosec // test sizes
		s.t.Fatal(err)
	}
}

func (s *screen) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.got.String()
}

func (s *screen) typeKeys(keys string) {
	s.t.Helper()
	if _, err := s.master.WriteString(keys); err != nil {
		s.t.Fatal(err)
	}
}

// waitFor waits until ok says the screen shows what it must.
func (s *screen) waitFor(what string, ok func(text string) bool) {
	s.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok(s.text()) {
		if time.Now().After(deadline) {
			text := s.text()
			s.t.Fatalf("the screen never showed %s; its last bytes: %q", what, text[max(len(text)-300, 0):])
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// run starts the guard on the screen and gives its exit status when asked.
func (s *screen) run(argv ...string) func() int {
	s.t.Helper()
	done := make(chan int, 1)
	go func() {
		status, err := Run(argv, s.slave, s.slave)
		if err != nil {
			s.t.Errorf("Run(%q): %v", argv, err)
		}
		done <- status
	}()
	return func() int {
		select {
		case status := <-done:
			return status
		case <-time.After(10 * time.Second):
			s.t.Fatalf("the guard did not end with %q", argv)
			return -1
		}
	}
}

// TestRunKeysDuringHold: while the pane holds a replay back, a key the
// builder types reaches the agent at once, and the replay does not scroll by.
func TestRunKeysDuringHold(t *testing.T) {
	keysFile := t.TempDir() + "/keys"
	t.Setenv(helperEnv, "flood")
	s := newScreen(t, 100, 30)
	wait := s.run(os.Args[0], keysFile)
	s.waitFor("the notice", func(text string) bool { return strings.Contains(text, Notice) })
	typed := time.Now()
	s.typeKeys("abc")
	var keys []byte
	for time.Since(typed) < 2*time.Second && string(keys) != "abc" {
		time.Sleep(10 * time.Millisecond)
		keys, _ = os.ReadFile(keysFile)
	}
	if string(keys) != "abc" {
		t.Fatalf("2 s after the keys abc were typed during a replay, the agent has %q", keys)
	}
	if status := wait(); status != 0 {
		t.Fatalf("the agent ended with status %d", status)
	}
	if n := strings.Count(s.text(), "replayed "); n > 20*(tailLines+1) {
		t.Fatalf("the pane got %d words of the replay: more than its last %d lines", n, tailLines)
	}
}

// TestRunCopies: every key reaches the agent as typed, and its output
// comes back as written.
func TestRunCopies(t *testing.T) {
	s := newScreen(t, 100, 30)
	const keys = "héllo\x1b[200~x\x1b[201~\r\x03\x1a" // text, a paste, Enter, ctrl+c, ctrl+z
	wait := s.run("sh", "-c", fmt.Sprintf("stty raw -echo; printf ready; head -c %d", len(keys)))
	s.waitFor("ready", func(text string) bool { return strings.HasSuffix(text, "ready") })
	s.typeKeys(keys)
	if status := wait(); status != 0 {
		t.Fatalf("the agent ended with status %d", status)
	}
	if got := s.text(); got != "ready"+keys {
		t.Fatalf("the screen shows %q, want %q", got, "ready"+keys)
	}
}

// TestRunSize: the agent's terminal has the pane's size, at its start and
// after each change.
func TestRunSize(t *testing.T) {
	s := newScreen(t, 100, 30)
	wait := s.run("sh", "-c", "stty -echo; while read -r x; do [ \"$x\" = q ] && exit 0; stty size; done")
	s.typeKeys("\n")
	s.waitFor("30 100", func(text string) bool { return strings.Contains(text, "30 100") })
	s.size(80, 30)
	if err := syscall.Kill(os.Getpid(), syscall.SIGWINCH); err != nil { // the test's screen is not this process's terminal
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	s.typeKeys("\n")
	s.waitFor("30 80", func(text string) bool { return strings.Contains(text, "30 80") })
	s.typeKeys("q\n")
	wait()
}

// TestRunStatus: the guard ends with the agent's own exit status.
func TestRunStatus(t *testing.T) {
	s := newScreen(t, 100, 30)
	if status := s.run("sh", "-c", "exit 3")(); status != 3 {
		t.Fatalf("the agent exits 3 and the guard gives %d", status)
	}
}
