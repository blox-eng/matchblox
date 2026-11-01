package panes

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"

	"github.com/blox-eng/matchblox/internal/tmuxtest"
)

// TestHomeKeyTakesTheClientHome types the home key into a real client that
// is in another session, after the key was set: tmux moves it to the
// console's session.
func TestHomeKeyTakesTheClientHome(t *testing.T) {
	sock := tmuxtest.Server(t)
	tmux := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", sock, "-f", os.DevNull}, args...)...).CombinedOutput() //nolint:gosec // test argv
		if err != nil {
			t.Fatalf("tmux %q: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	session := func() string { return tmux("list-clients", "-F", "#{client_session}") }
	tmux("new-session", "-d", "-s", Home)
	tmux("new-session", "-d", "-s", "work")

	term := attach(t, sock, "work")
	waitFor(t, func() bool { return session() == "work" })
	tmux(HomeKeyArgv()[1:]...)
	if _, err := term.WriteString(HomeKey); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return session() == Home })
}

// TestHomeKeyReachesTheConsoleAtHome types the home key, raw, into a real
// client that is in the console's session already: the console there still
// reads it, whole, through the input decoder it runs on (#261). Moving the
// client home would do nothing, and the chat would keep the keys.
func TestHomeKeyReachesTheConsoleAtHome(t *testing.T) {
	sock := tmuxtest.Server(t)
	tmux := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("tmux", append([]string{"-L", sock, "-f", os.DevNull}, args...)...).CombinedOutput(); err != nil { //nolint:gosec // test argv
			t.Fatalf("tmux %q: %v\n%s", args, err, out)
		}
	}
	got := t.TempDir() + "/events"
	tmux("new-session", "-d", "-s", Home, "-e", decodeTo+"="+got, os.Args[0], "-test.run=^TestDecodePane$")
	tmux(HomeKeyArgv()[1:]...)
	term := attach(t, sock, Home)
	want := eventLine(uv.UnknownApcEvent(HomeKey))
	waitFor(t, func() bool {
		b, _ := os.ReadFile(got)
		return strings.HasPrefix(string(b), "ready")
	})
	if _, err := term.WriteString(HomeKey); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		b, _ := os.ReadFile(got)
		return strings.Contains(string(b), want)
	})
	b, _ := os.ReadFile(got)
	if lines := strings.Split(strings.TrimSpace(string(b)), "\n"); len(lines) != 2 {
		t.Errorf("the console read %q, want only %s", lines[1:], want)
	}
}

// decodeTo names the file TestDecodePane writes the events it reads to.
const decodeTo = "MATCHBLOX_TEST_DECODE_TO"

// TestDecodePane is the pane of TestHomeKeyReachesTheConsoleAtHome: a raw
// terminal read by the decoder the console runs on, one line per event.
func TestDecodePane(t *testing.T) {
	path := os.Getenv(decodeTo)
	if path == "" {
		t.Skip("the pane of TestHomeKeyReachesTheConsoleAtHome")
	}
	if _, err := term.MakeRaw(os.Stdin.Fd()); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan uv.Event)
	go func() {
		for e := range events {
			_, _ = fmt.Fprintln(f, eventLine(e))
		}
	}()
	_, _ = fmt.Fprintln(f, "ready")
	_ = uv.NewTerminalReader(os.Stdin, "xterm-256color").StreamEvents(context.Background(), events)
}

func eventLine(e uv.Event) string { return fmt.Sprintf("%T %q", e, fmt.Sprint(e)) }

// attach runs `tmux attach -t target` on a new pseudo-terminal and returns
// the terminal's side, where the test types.
func attach(t *testing.T, sock, target string) *os.File {
	t.Helper()
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no pseudo-terminals: %v", err)
	}
	t.Cleanup(func() { _ = ptmx.Close() })
	if err := unix.IoctlSetPointerInt(int(ptmx.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(int(ptmx.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	pts, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pts.Close() }()
	cmd := exec.Command("tmux", "-L", sock, "attach", "-t", target) //nolint:gosec // test argv
	cmd.Stdin, cmd.Stdout, cmd.Stderr = pts, pts, pts
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	cmd.SysProcAttr = &unix.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	go drain(ptmx)
	return ptmx
}

// drain reads what tmux draws, so a full terminal never blocks it.
func drain(f *os.File) {
	b := make([]byte, 4096)
	for {
		if _, err := f.Read(b); err != nil {
			return
		}
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatal("timed out")
}
