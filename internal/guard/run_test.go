//go:build linux || darwin

package guard

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/testhome"
	"github.com/blox-eng/matchblox/internal/tmuxtest"
	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

// helperEnv makes this test binary stand in for an agent, or for the
// matchblox command: its first argument names which.
const helperEnv = "MATCHBLOX_GUARD_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(helperEnv) == "" {
		os.Exit(testhome.Run(m))
	}
	switch os.Args[1] {
	case "flood":
		helperFlood(os.Args[2])
	case "cursor":
		helperCursor()
	case "guard":
		os.Exit(Main(os.Args[2:], "web", os.Stdin, os.Stdout))
	}
	os.Exit(0)
}

// standInLines is how long the stand-in's chat is: over 1 MiB a replay.
const standInLines = 20000

// helperCursor stands in for Cursor with a long chat. It prints the clear
// codes, every line and its input box at its start and at each change of
// columns, and nothing at a change of rows. It shows each key in its box.
func helperCursor() {
	if _, err := term.MakeRaw(os.Stdin.Fd()); err != nil {
		os.Exit(2)
	}
	var mu sync.Mutex
	var input []byte
	replaying := false
	replay := func() {
		mu.Lock()
		replaying = true
		mu.Unlock()
		var b bytes.Buffer
		b.WriteString(ClearCodes)
		for i := 1; i <= standInLines; i++ {
			fmt.Fprintf(&b, "line %d| %s\r\n", i, strings.Repeat("chat ", 12))
			if i%200 == 0 { // a burst, not one write: as a renderer at work
				_, _ = os.Stdout.Write(b.Bytes())
				b.Reset()
				time.Sleep(time.Millisecond)
			}
		}
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintf(&b, "╭── ask ──╮\r\n│ → %s", input)
		_, _ = os.Stdout.Write(b.Bytes())
		replaying = false
	}
	go func() {
		key := make([]byte, 64)
		for {
			n, err := os.Stdin.Read(key)
			if err != nil {
				os.Exit(0)
			}
			mu.Lock()
			input = append(input, key[:n]...)
			if !replaying {
				_, _ = os.Stdout.Write(key[:n])
			}
			mu.Unlock()
		}
	}()
	winch := make(chan os.Signal, 8)
	signal.Notify(winch, syscall.SIGWINCH)
	cols, _, _ := term.GetSize(os.Stdout.Fd())
	replay()
	for range winch {
		if now, _, _ := term.GetSize(os.Stdout.Fd()); now != cols {
			cols = now
			replay()
		}
	}
}

// helperFlood replays without end, as a Cursor with a very long chat, and
// writes the first three keys it reads to the file named in its argument.
func helperFlood(keysFile string) {
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
	if err := os.WriteFile(keysFile, keys, 0o600); err != nil {
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
	t.Setenv(helperEnv, "1")
	s := newScreen(t, 100, 30)
	wait := s.run(os.Args[0], "flood", keysFile)
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

// TestGuardCannotStart: with no terminal to give the agent, the agent still
// runs, and the pane says why it has no guard.
func TestGuardCannotStart(t *testing.T) {
	open := openTerminal
	openTerminal = func() (*os.File, *os.File, error) { return nil, nil, errors.New("no ptys left") }
	t.Cleanup(func() { openTerminal = open })
	s := newScreen(t, 100, 30)
	if _, err := term.MakeRaw(s.slave.Fd()); err != nil { // as tmux leaves it: what is written is what shows
		t.Fatal(err)
	}
	if status := Main([]string{"sh", "-c", "printf ran; exit 4"}, "web", s.slave, s.slave); status != 4 {
		t.Fatalf("the agent exits 4 without the guard, and the guard gives %d", status)
	}
	want := "Cursor runs without the redraw guard: no ptys left\r\nran"
	s.waitFor("the words and the agent's output", func(text string) bool { return text == want })
}

// TestGuardStops: a guard that breaks ends the agent, gives the terminal
// back as it was, and says how to start again.
func TestGuardStops(t *testing.T) {
	cp := copyOutput
	copyOutput = func(*Filter, io.Reader) { panic("boom") }
	t.Cleanup(func() { copyOutput = cp })
	s := newScreen(t, 100, 30)
	modes := func() string {
		stty := exec.Command("stty", "-g")
		stty.Stdin = s.slave
		out, err := stty.Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	before := modes()
	began := time.Now()
	status := Main([]string{"sh", "-c", "sleep 30"}, "web", s.slave, s.slave)
	if status == 0 || time.Since(began) > 5*time.Second {
		t.Fatalf("a broken guard gave status %d after %v: want a failure, at once", status, time.Since(began))
	}
	if after := modes(); after != before {
		t.Fatalf("the terminal's modes are %q, want them as they were: %q", after, before)
	}
	want := "matchblox's redraw guard stopped: boom. Start again: matchblox switch web --agent cursor-agent"
	s.waitFor("the words", func(text string) bool { return strings.Contains(text, want) })
}

// TestGuardInTmux: in a tmux window, behind the guard, a change of width
// shows the agent's last screen and never its chat; a change of rows shows
// nothing new; a key typed during the replay is in the agent's input box.
// The same agent with no guard is the measure beside it.
func TestGuardInTmux(t *testing.T) {
	server, dir := tmuxtest.Server(t), t.TempDir()
	tmux := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", server, "-f", os.DevNull}, args...)...).CombinedOutput() //nolint:gosec // a test's own server
		if err != nil {
			t.Fatalf("tmux %v: %v %s", args, err, out)
		}
		return string(out)
	}
	// Each pane waits a moment before its agent starts, so its pipe sees
	// every byte from the first.
	start := func(name string, argv ...string) {
		t.Helper()
		cmd := "sleep 0.5; exec env " + helperEnv + "=1 " + os.Args[0] + " " + strings.Join(argv, " ")
		tmux("new-session", "-d", "-s", name, "-x", "216", "-y", "71", "sh", "-c", cmd)
		tmux("pipe-pane", "-O", "-t", name, "cat >> "+filepath.Join(dir, name))
	}
	start("direct", "cursor")
	start("guarded", "guard", os.Args[0], "cursor")
	printed := func(name string) []byte {
		b, _ := os.ReadFile(filepath.Join(dir, name))
		return b
	}
	// settled waits until both panes show the input box with typed in it and
	// print no more, and gives each one's screen.
	settled := func(typed string) (direct, guarded string) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for last := -1; ; {
			time.Sleep(400 * time.Millisecond) // over the guard's end window
			direct, guarded = tmux("capture-pane", "-p", "-t", "direct"), tmux("capture-pane", "-p", "-t", "guarded")
			size := len(printed("direct")) + len(printed("guarded"))
			box := strings.TrimRight("│ → "+typed, " ")
			if size == last && strings.HasSuffix(strings.TrimRight(direct, "\n "), box) && strings.HasSuffix(strings.TrimRight(guarded, "\n "), box) {
				return direct, guarded
			}
			if last = size; time.Now().After(deadline) {
				t.Fatalf("the panes never settled on %q.\ndirect ends: %q\nguarded ends: %q", box, tail(direct), tail(guarded))
			}
		}
	}
	same := func(when, direct, guarded string) {
		t.Helper()
		if direct != guarded {
			t.Fatalf("%s the guarded pane does not show the agent's own last screen.\ndirect ends: %q\nguarded ends: %q", when, tail(direct), tail(guarded))
		}
	}
	direct, guarded := settled("")
	same("at the start", direct, guarded)

	typed := ""
	for _, cols := range []string{"180", "216"} {
		d0, g0 := len(printed("direct")), len(printed("guarded"))
		for _, name := range []string{"direct", "guarded"} {
			tmux("resize-window", "-t", name, "-x", cols, "-y", "71")
			tmux("send-keys", "-t", name, "-l", "ab") // typed while the agent replays
		}
		typed += "ab"
		direct, guarded = settled(typed)
		same("at "+cols+" columns", direct, guarded)
		d, g := len(printed("direct"))-d0, len(printed("guarded"))-g0
		t.Logf("to %s columns: the agent alone printed %d bytes, behind the guard %d", cols, d, g)
		if d < 1<<20 {
			t.Fatalf("to %s columns the stand-in printed %d bytes: not the replay this test is about", cols, d)
		}
		if g > 200<<10 {
			t.Fatalf("to %s columns the guarded pane got %d bytes, want under 200 KiB: the replay scrolled by", cols, g)
		}
	}
	if bytes.Contains(printed("guarded"), []byte("line 1|")) || !bytes.Contains(printed("direct"), []byte("line 1|")) {
		t.Fatal("the guarded pane got the first line of the chat, or the measure beside it did not")
	}

	g0 := len(printed("guarded"))
	tmux("resize-window", "-t", "guarded", "-x", "216", "-y", "70")
	time.Sleep(600 * time.Millisecond)
	if g := len(printed("guarded")) - g0; g != 0 {
		t.Fatalf("a change of rows alone printed %d bytes in the guarded pane, want 0", g)
	}
}

// tail is the end of a screen, for a failure's words.
func tail(screen string) string {
	screen = strings.TrimRight(screen, "\n ")
	return screen[max(len(screen)-200, 0):]
}

// TestGuardWritesNoFile: what passes through the guard, a token the agent
// prints in a replay and outside one and a password the builder types,
// lands in no file and in no line of the guard's own.
func TestGuardWritesNoFile(t *testing.T) {
	const printed, typedSecret = "tok-9f3c-SECRET", "pw-77aa-SECRET"
	home := t.TempDir()
	for _, v := range []string{"HOME", "XDG_STATE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "TMPDIR"} {
		t.Setenv(v, home)
	}
	s := newScreen(t, 100, 30)
	stderr, err := os.Create(filepath.Join(t.TempDir(), "stderr")) // outside home: read apart from it
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = stderr
	t.Cleanup(func() { os.Stderr = old })
	agent := fmt.Sprintf(`stty raw -echo; printf '%s before\r\n'; printf '\033[H\033[2J\033[3J%s in a replay\r\n'; printf ready; head -c %d >/dev/null`, printed, printed, len(typedSecret))
	done := make(chan int, 1)
	go func() { done <- Main([]string{"sh", "-c", agent}, "web", s.slave, s.slave) }()
	s.waitFor("ready", func(text string) bool { return strings.Contains(text, "ready") })
	s.typeKeys(typedSecret)
	if status := <-done; status != 0 {
		t.Fatalf("the agent ended with status %d", status)
	}
	if !strings.Contains(s.text(), printed+" in a replay") {
		t.Fatalf("the pane did not get the agent's output: %q", s.text())
	}
	said, _ := os.ReadFile(stderr.Name())
	for _, secret := range []string{printed, typedSecret} {
		if bytes.Contains(said, []byte(secret)) {
			t.Fatalf("the guard's own output holds %q", secret)
		}
	}
	err = filepath.WalkDir(home, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		for _, secret := range []string{printed, typedSecret} {
			if bytes.Contains(b, []byte(secret)) {
				t.Errorf("%s holds %q", path, secret)
			}
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestGuardEndsWhenPaneGoes: when the pane goes, an agent that ignores the
// hangup does not live on behind its guard. With no guard its terminal
// would be gone; the guard takes its terminal away too, and ends it.
func TestGuardEndsWhenPaneGoes(t *testing.T) {
	wait := killWait
	killWait = 200 * time.Millisecond
	t.Cleanup(func() { killWait = wait })
	pidFile := filepath.Join(t.TempDir(), "pid")
	s := newScreen(t, 100, 30)
	done := s.run("sh", "-c", "trap '' HUP; echo $$ > "+pidFile+"; printf ready; while :; do sleep 1; done")
	s.waitFor("ready", func(text string) bool { return strings.Contains(text, "ready") })
	if err := syscall.Kill(os.Getpid(), syscall.SIGHUP); err != nil { // as tmux sends the pane's program
		t.Fatal(err)
	}
	began := time.Now()
	done()
	if took := time.Since(began); took > 3*time.Second {
		t.Fatalf("the guard took %v to end after the hangup", took)
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(2 * time.Second); syscall.Kill(pid, 0) == nil; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("the agent (pid %d) lives on after its pane went", pid)
		}
	}
}
