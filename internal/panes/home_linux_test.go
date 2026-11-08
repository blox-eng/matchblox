package panes

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"

	"github.com/blox-eng/matchblox/internal/keycmd"
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
	tmux("new-session", "-d", "-s", Home, "-e", decodeTo+"="+got, consoleBinary(t), "-test.run=^TestDecodePane$")
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

// pressPrefixM is prefix m on a server with tmux's default prefix, one key at
// a time as a person types: two keys within assume-paste-time are a paste to
// tmux, which binds neither. A client just attached gets a moment first.
func pressPrefixM(t *testing.T, term *os.File) {
	t.Helper()
	time.Sleep(100 * time.Millisecond)
	for _, k := range []string{"\x02", "m"} {
		if _, err := term.WriteString(k); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
	}
}

// consoleBinary is this test binary under the console's name, which is how
// tmux tells a console's pane.
func consoleBinary(t *testing.T) string {
	t.Helper()
	console := t.TempDir() + "/" + Home
	if err := os.Symlink(os.Args[0], console); err != nil {
		t.Fatal(err)
	}
	return console
}

// consolePane starts a server of its own with the console's session, whose
// pane writes what it reads to the file it returns.
func consolePane(t *testing.T) (sock string, tmux func(args ...string) string, got string) {
	t.Helper()
	return consolePaneIn(t, Home)
}

// consolePaneIn is consolePane with the console in session.
func consolePaneIn(t *testing.T, session string) (sock string, tmux func(args ...string) string, got string) {
	t.Helper()
	sock = tmuxtest.Server(t)
	tmux = func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", sock, "-f", os.DevNull}, args...)...).CombinedOutput() //nolint:gosec // test argv
		if err != nil {
			t.Fatalf("tmux %q: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	got = t.TempDir() + "/events"
	tmux("new-session", "-d", "-s", session, "-e", decodeTo+"="+got, consoleBinary(t), "-test.run=^TestDecodePane$")
	waitFor(t, func() bool {
		b, _ := os.ReadFile(got)
		return strings.HasPrefix(string(b), "ready")
	})
	return sock, tmux, got
}

// paneRead is what the pane of consolePane read, one event a line.
func paneRead(t *testing.T, got string) []string {
	t.Helper()
	b, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")[1:]
}

// pressHomeKey is the app's ◂, ⌘M or ctrl+M: the home key's bytes, whole.
func pressHomeKey(t *testing.T, term *os.File) {
	t.Helper()
	if _, err := term.WriteString(HomeKey); err != nil {
		t.Fatal(err)
	}
}

// TestTheWayBackAtAnyConsolePane: on a pane that runs a console, in the home
// session or in place in another one, prefix m and the app's home key reach
// that console, whole and alone, and the client stays where it is. The home
// session is there too, to switch to if the test were wrong.
func TestTheWayBackAtAnyConsolePane(t *testing.T) {
	for _, session := range []string{Home, "blox"} {
		for name, press := range map[string]func(*testing.T, *os.File){"prefix m": pressPrefixM, "the home key": pressHomeKey} {
			t.Run(session+"/"+name, func(t *testing.T) {
				sock, tmux, got := consolePaneIn(t, session)
				if session != Home {
					tmux("new-session", "-d", "-s", Home, "sleep", "100")
				}
				tmux(HomeKeyArgv()[1:]...)
				term := attach(t, sock, session)
				waitFor(t, func() bool { return tmux("list-clients", "-F", "#{client_session}") == session })
				time.Sleep(100 * time.Millisecond)
				press(t, term)
				want := eventLine(uv.UnknownApcEvent(HomeKey))
				waitFor(t, func() bool { return slices.Contains(paneRead(t, got), want) })
				if lines := paneRead(t, got); len(lines) != 1 {
					t.Errorf("the console read %q, want only %s", lines, want)
				}
				if on := tmux("list-clients", "-F", "#{client_session}"); on != session {
					t.Errorf("the client went to %q, want it to stay in %q", on, session)
				}
			})
		}
	}
}

// TestTheWayBackFromAShellPane: on a pane that runs no console, prefix m and
// the app's home key show the home console, also when another window of the
// home session is the one on screen, and also from that window. The console
// reads nothing.
func TestTheWayBackFromAShellPane(t *testing.T) {
	for _, from := range []string{"another session", "the home session"} {
		for name, press := range map[string]func(*testing.T, *os.File){"prefix m": pressPrefixM, "the home key": pressHomeKey} {
			t.Run(from+"/"+name, func(t *testing.T) {
				sock, tmux, got := consolePane(t)
				tmux("new-window", "-t", "="+Home+":", "sleep", "100")
				session := Home
				if from == "another session" {
					session = "work"
					tmux("new-session", "-d", "-s", session, "sleep", "100")
				}
				tmux(HomeKeyArgv()[1:]...)
				term := attach(t, sock, session)
				waitFor(t, func() bool { return tmux("list-clients", "-F", "#{client_session}") == session })
				time.Sleep(100 * time.Millisecond)
				press(t, term)
				waitFor(t, func() bool {
					return tmux("list-clients", "-F", "#{client_session}") == Home &&
						tmux("display-message", "-p", "-t", "="+Home+":", "#{pane_current_command}") == Home
				})
				if lines := paneRead(t, got); len(lines) != 0 {
					t.Errorf("the console read %q", lines)
				}
			})
		}
	}
}

// TestPrefixMGivesTheKeysBackAtHome presses prefix m in a real client in the
// console's session: the console reads the home key, whole and alone, so its
// chat gives the keys back. tmux's own m, which marks a pane, is gone.
func TestPrefixMGivesTheKeysBackAtHome(t *testing.T) {
	sock, tmux, got := consolePane(t)
	tmux(HomeKeyArgv()[1:]...)
	term := attach(t, sock, Home)
	pressPrefixM(t, term)
	want := eventLine(uv.UnknownApcEvent(HomeKey))
	waitFor(t, func() bool { return slices.Contains(paneRead(t, got), want) })
	if lines := paneRead(t, got); len(lines) != 1 {
		t.Errorf("the console read %q, want only %s", lines, want)
	}
}

// TestPrefixMTakesTheClientHome presses prefix m in a real client in another
// session: tmux moves it to the console's session, and the console reads
// nothing.
func TestPrefixMTakesTheClientHome(t *testing.T) {
	sock, tmux, got := consolePane(t)
	tmux("new-session", "-d", "-s", "work")
	tmux(HomeKeyArgv()[1:]...)
	term := attach(t, sock, "work")
	pressPrefixM(t, term)
	waitFor(t, func() bool { return tmux("list-clients", "-F", "#{client_session}") == Home })
	if lines := paneRead(t, got); len(lines) != 0 {
		t.Errorf("the console read %q", lines)
	}
}

// TestHomeKeyLeavesCopyMode: with the console's pane in copy mode, in either
// set of copy-mode keys, the home key and prefix m leave it and reach the
// console, whole. Copy mode would read the key's bytes as its own commands.
func TestHomeKeyLeavesCopyMode(t *testing.T) {
	for _, mode := range []string{"emacs", "vi"} {
		for name, press := range map[string]func(*testing.T, *os.File){
			"prefix m": pressPrefixM, "the home key": pressHomeKey,
		} {
			t.Run(mode+"/"+name, func(t *testing.T) {
				sock, tmux, got := consolePane(t)
				tmux(HomeKeyArgv()[1:]...)
				tmux("set-option", "-g", "mode-keys", mode)
				term := attach(t, sock, Home)
				tmux("copy-mode", "-t", "="+Home+":")
				waitFor(t, func() bool { return tmux("display-message", "-p", "-t", "="+Home+":", "#{pane_in_mode}") == "1" })
				press(t, term)
				want := eventLine(uv.UnknownApcEvent(HomeKey))
				waitFor(t, func() bool { return slices.Contains(paneRead(t, got), want) })
				if lines := paneRead(t, got); len(lines) != 1 {
					t.Errorf("the console read %q, want only %s", lines, want)
				}
				if in := tmux("display-message", "-p", "-t", "="+Home+":", "#{pane_in_mode}"); in != "0" {
					t.Errorf("the pane stayed in copy mode")
				}
			})
		}
	}
}

// TestTheConfigAndTheServerBindPrefixMAlike: the way back in the tmux config
// and the binding the console sets are one, so a config sourced again leaves
// prefix m as the console set it.
func TestTheConfigAndTheServerBindPrefixMAlike(t *testing.T) {
	_, tmux, _ := consolePane(t)
	tmux(HomeKeyArgv()[1:]...)
	set := tmux("list-keys", "-T", "prefix", "m")
	conf := t.TempDir() + "/tmux.conf"
	if err := os.WriteFile(conf, []byte(WayBack), 0o600); err != nil {
		t.Fatal(err)
	}
	tmux("unbind-key", "-T", "prefix", "m")
	tmux("source-file", conf)
	if got := tmux("list-keys", "-T", "prefix", "m"); got != set {
		t.Errorf("the config binds\n%s\nthe console bound\n%s", got, set)
	}
}

// agentPane is a client attached to a pane in session work that the
// console marked as agent's, which writes what reaches it, as cat -v shows
// it, to the file it returns. T10, T23.
func agentPane(t *testing.T, agent string) (client *os.File, typed string) {
	t.Helper()
	sock := tmuxtest.Server(t)
	tmux := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", sock, "-f", os.DevNull}, args...)...).CombinedOutput() //nolint:gosec // test argv
		if err != nil {
			t.Fatalf("tmux %q: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	typed = t.TempDir() + "/typed"
	tmux("new-session", "-d", "-s", "work", "sh", "-c", "stty raw -echo; exec cat -v > "+typed)
	pane := tmux("display-message", "-p", "-t", "work", "#{pane_id}")
	tmux("set-option", "-p", "-t", pane, "@matchblox_agent", agent)
	tmux(CommandKeysArgv()[1:]...)
	waitFor(t, func() bool { return tmux("display-message", "-p", "-t", pane, "#{pane_current_command}") == "cat" })
	return attach(t, sock, "work"), typed
}

// typedThen types key, then x, into client, and is what the pane read
// once the x came.
func typedThen(t *testing.T, client *os.File, typed, key string) string {
	t.Helper()
	if _, err := client.WriteString(key + "x"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		b, _ := os.ReadFile(typed)
		return strings.HasSuffix(string(b), "x")
	})
	b, _ := os.ReadFile(typed)
	return string(b)
}

// TestCommandKeyTypesIntoAMarkedAgentPane: in a pane marked as Claude
// Code's, the compact key types /compact, and no Return.
func TestCommandKeyTypesIntoAMarkedAgentPane(t *testing.T) {
	client, typed := agentPane(t, "claude")
	if got := typedThen(t, client, typed, keycmd.Sequence("compact")); got != "/compactx" {
		t.Errorf("the pane read %q, want /compact and no Return", got)
	}
}

// TestCommandKeyDoesNothingInAnotherAgentsPane: a Claude Code command in a
// pane marked as Codex's types nothing.
func TestCommandKeyDoesNothingInAnotherAgentsPane(t *testing.T) {
	client, typed := agentPane(t, "codex")
	if got := typedThen(t, client, typed, keycmd.Sequence("compact")); got != "x" {
		t.Errorf("the pane read %q, want nothing of the command", got)
	}
}

// TestCommandKeyDoesNothingElsewhere types a command key into a client in
// another session: tmux drops it, so no agent there reads it as typing.
// Without the binding tmux hands the sequence on to the pane.
func TestCommandKeyDoesNothingElsewhere(t *testing.T) {
	sock := tmuxtest.Server(t)
	tmux := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("tmux", append([]string{"-L", sock, "-f", os.DevNull}, args...)...).CombinedOutput(); err != nil { //nolint:gosec // test argv
			t.Fatalf("tmux %q: %v\n%s", args, err, out)
		}
	}
	got := t.TempDir() + "/events"
	tmux("new-session", "-d", "-s", Home)
	tmux("new-session", "-d", "-s", "work", "-e", decodeTo+"="+got, os.Args[0], "-test.run=^TestDecodePane$")
	tmux(CommandKeysArgv()[1:]...)
	term := attach(t, sock, "work")
	waitFor(t, func() bool {
		b, _ := os.ReadFile(got)
		return strings.HasPrefix(string(b), "ready")
	})
	if _, err := term.WriteString(keycmd.Sequence("compact") + "x"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		b, _ := os.ReadFile(got)
		return strings.Contains(string(b), "KeyPressEvent")
	})
	if b, _ := os.ReadFile(got); strings.Contains(string(b), "UnknownApcEvent") {
		t.Errorf("the pane read the command key: %q", b)
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

// clientTTYs are the ttys of the clients attached to sock.
func clientTTYs(tmux func(args ...string) string) []string {
	return strings.Fields(tmux("list-clients", "-F", "#{client_tty}"))
}

func TestAMarkedReturnNamesTheClientThatSentIt(t *testing.T) {
	sock, tmux, got := consolePane(t)
	tmux(MarkKeyArgv()[1:]...)
	attach(t, sock, Home)
	waitFor(t, func() bool { return len(clientTTYs(tmux)) == 1 })
	desk := clientTTYs(tmux)[0]
	phone := attach(t, sock, Home)
	waitFor(t, func() bool { return len(clientTTYs(tmux)) == 2 })
	phoneTTY := ""
	for _, tty := range clientTTYs(tmux) {
		if tty != desk {
			phoneTTY = tty
		}
	}
	if _, err := phone.WriteString(keycmd.MarkKey + "\r"); err != nil {
		t.Fatal(err)
	}
	mark := eventLine(uv.UnknownApcEvent(keycmd.MarkedPrefix + phoneTTY + "\x1b\\"))
	enter := eventLine(uv.KeyPressEvent{Code: uv.KeyEnter})
	waitFor(t, func() bool { return slices.Contains(paneRead(t, got), enter) })
	lines := paneRead(t, got)
	if i, j := slices.Index(lines, mark), slices.Index(lines, enter); i < 0 || i > j {
		t.Errorf("want %q before %q in %q", mark, enter, lines)
	}
}

func TestAMarkElsewhereTypesNothing(t *testing.T) {
	sock := tmuxtest.Server(t)
	tmux := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", sock, "-f", os.DevNull}, args...)...).CombinedOutput() //nolint:gosec // test argv
		if err != nil {
			t.Fatalf("tmux %q: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	typed := t.TempDir() + "/typed"
	tmux("new-session", "-d", "-s", "work", "cat -v > "+typed)
	tmux(MarkKeyArgv()[1:]...)
	client := attach(t, sock, "work")
	waitFor(t, func() bool { return len(clientTTYs(tmux)) == 1 })
	if _, err := client.WriteString(keycmd.MarkKey + "x\r"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { b, _ := os.ReadFile(typed); return strings.Contains(string(b), "x") })
	if b, _ := os.ReadFile(typed); strings.Contains(string(b), "matchblox") {
		t.Errorf("the mark was typed: %q", b)
	}
}
