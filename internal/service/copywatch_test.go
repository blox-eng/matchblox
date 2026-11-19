package service

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/tmuxtest"
	"github.com/blox-eng/matchblox/internal/transport"
)

// copyServer is a private tmux server with one session, 100 by 30; tm runs
// a command of it.
func copyServer(t *testing.T) (sock string, tm func(args ...string) string) {
	t.Helper()
	name := tmuxtest.Server(t)
	tm = func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", name}, args...)...).CombinedOutput() //nolint:gosec // a test's own server
		if err != nil {
			t.Fatalf("tmux %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	tm("-f", "/dev/null", "new-session", "-d", "-s", "work", "-x", "100", "-y", "30", "sleep", "600")
	return sample.SocketPath(name), tm
}

// copyOf waits for the next copy on c.
func copyOf(t *testing.T, c transport.Conn) string {
	t.Helper()
	var cp proto.Copy
	if err := json.Unmarshal(next(t, c, proto.KindCopy).Body, &cp); err != nil {
		t.Fatal(err)
	}
	return cp.Text
}

// watching starts the copy watcher of s on sock and waits until it is
// attached.
func watching(t *testing.T, s *Service, sock string, tm func(args ...string) string) {
	t.Helper()
	ctx := run(t, s)
	go s.copyWatch(ctx, sock)
	waitUntil(t, "the copy watcher attaches", func() bool {
		return strings.Contains(tm("list-clients", "-F", "#{client_control_mode} #{session_name}"), "1 "+copyWatchSession)
	})
}

func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(waitLimit); time.Now().Before(end); time.Sleep(30 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("timeout: %s", what)
}

// TestANewBufferGoesToEveryConsole: a tmux copy reaches each console, and
// no matchblox command.
func TestANewBufferGoesToEveryConsole(t *testing.T) {
	sock, tm := copyServer(t)
	s := newTest(t, time.Hour)
	watching(t, s, sock, tm)
	ctx := t.Context()
	console := connectAs(t, ctx, s, proto.ClientConsole)
	cli := connectAs(t, ctx, s, proto.ClientCLI)
	cliCopies := make(chan string, 1)
	go func() {
		for {
			env, err := cli.Recv()
			if err != nil {
				return
			}
			if env.Kind == proto.KindCopy {
				cliCopies <- string(env.Body)
				return
			}
		}
	}()
	tm("set-buffer", "-b", "buffer0", "hello")
	if got := copyOf(t, console); got != "hello" {
		t.Fatalf("console got %q, want hello", got)
	}
	select {
	case got := <-cliCopies:
		t.Fatalf("a matchblox command got a copy: %s", got)
	case <-time.After(500 * time.Millisecond):
	}
}

// TestTheConsolesOwnBufferIsNotSentBack: the buffer a console loads to put
// a copy on its screen does not come round again.
func TestTheConsolesOwnBufferIsNotSentBack(t *testing.T) {
	sock, tm := copyServer(t)
	s := newTest(t, time.Hour)
	watching(t, s, sock, tm)
	console := connectAs(t, t.Context(), s, proto.ClientConsole)
	tm("set-buffer", "-b", proto.CopyBuffer, "mine")
	tm("set-buffer", "-b", "buffer1", "theirs")
	if got := copyOf(t, console); got != "theirs" {
		t.Fatalf("console got %q first, want theirs", got)
	}
}

// TestTheWatcherComesBack: the watcher's control client can end (a person
// kills its session); a later copy still arrives.
func TestTheWatcherComesBack(t *testing.T) {
	sock, tm := copyServer(t)
	s := newTest(t, time.Hour)
	watching(t, s, sock, tm)
	console := connectAs(t, t.Context(), s, proto.ClientConsole)
	tm("kill-session", "-t", "="+copyWatchSession)
	waitUntil(t, "the copy watcher attaches again", func() bool {
		out, _ := exec.Command("tmux", "-S", sock, "list-clients", "-F", "#{client_control_mode} #{session_name}").Output() //nolint:gosec // a test's own server
		return strings.Contains(string(out), "1 "+copyWatchSession)
	})
	tm("set-buffer", "-b", "buffer2", "again")
	if got := copyOf(t, console); got != "again" {
		t.Fatalf("console got %q, want again", got)
	}
}

// TestTheWatcherLeavesWindowSizesAlone: its control client sizes nothing,
// and its session is not one the sampler lists.
func TestTheWatcherLeavesWindowSizesAlone(t *testing.T) {
	sock, tm := copyServer(t)
	s := newTest(t, time.Hour)
	watching(t, s, sock, tm)
	if got := tm("display", "-p", "-t", "=work:", "#{window_width}x#{window_height}"); got != "100x30" {
		t.Fatalf("the work window is %s, want 100x30", got)
	}
	if got := tm("show-option", "-qv", "-t", "="+copyWatchSession+":", sample.JumpMark); got != "1" {
		t.Fatalf("the watcher's session has %s %q, want 1: the sampler would list it", sample.JumpMark, got)
	}
}

// attachedClient attaches a real client to session through script, for a
// message tmux shows a person.
func attachedClient(t *testing.T, sock, session string) string {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the client runs on util-linux script -qfc")
	}
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script is not installed")
	}
	cmd := exec.Command("script", "-qfc", "stty rows 30 cols 100; exec tmux -S "+sock+" attach -t '="+session+"'", "/dev/null") //nolint:gosec // a test's own server
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "TMUX") {
			env = append(env, e)
		}
	}
	cmd.Env = append(env, "TERM=xterm-256color")
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() })
	var name string
	waitUntil(t, "a client of "+session, func() bool {
		out, _ := exec.Command("tmux", "-S", sock, "list-clients", "-F", "#{client_control_mode} #{client_name}").Output() //nolint:gosec // a test's own server
		for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if n, ok := strings.CutPrefix(l, "0 "); ok {
				name = n
				return true
			}
		}
		return false
	})
	return name
}

// messages is what tmux showed client.
func messages(sock, client string) string {
	out, _ := exec.Command("tmux", "-S", sock, "show-messages", "-t", client).Output() //nolint:gosec // a test's own server
	return string(out)
}

// TestABigCopyIsCut: a copy over 1 MiB goes to no console, and tmux tells
// the person who made it.
func TestABigCopyIsCut(t *testing.T) {
	sock, tm := copyServer(t)
	s := newTest(t, time.Hour)
	watching(t, s, sock, tm)
	console := connectAs(t, t.Context(), s, proto.ClientConsole)
	person := attachedClient(t, sock, "work")
	big := filepath.Join(t.TempDir(), "big")
	if err := os.WriteFile(big, []byte(strings.Repeat("x", 2<<20)), 0o600); err != nil {
		t.Fatal(err)
	}
	tm("load-buffer", "-b", "buffer3", big)
	tm("set-buffer", "-b", "buffer4", "small")
	if got := copyOf(t, console); got != "small" {
		t.Fatalf("console got %d bytes first, want the small copy", len(got))
	}
	waitUntil(t, "tmux tells the person", func() bool {
		return strings.Contains(messages(sock, person), "not copied: 2.0 MB is over the 1 MB copy limit")
	})
}

// TestACopyWithNoConsoleSaysSo: with no console to put it on a screen, tmux
// says so where the copy was made.
func TestACopyWithNoConsoleSaysSo(t *testing.T) {
	sock, tm := copyServer(t)
	s := newTest(t, time.Hour)
	watching(t, s, sock, tm)
	person := attachedClient(t, sock, "work")
	tm("set-buffer", "-b", "buffer5", "lost")
	waitUntil(t, "tmux tells the person", func() bool {
		return strings.Contains(messages(sock, person), "not copied: no matchblox console is connected · open the console")
	})
}

// TestAnAgentThatSaysConsoleGetsNoCopies: a client whose process runs in an
// agent's pane is no console, whatever its hello says, as for screens: it
// gets no copy of what the person copied.
func TestAnAgentThatSaysConsoleGetsNoCopies(t *testing.T) {
	sock, tm := copyServer(t)
	s := newTest(t, time.Hour)
	s.PeerPane = func(pid int) string { return map[int]string{100: "%1"}[pid] }
	watching(t, s, sock, tm)
	waitUntil(t, "the first sample", func() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.have })
	s.mu.Lock()
	s.cur.Sessions = append(s.cur.Sessions, sample.Session{Name: "flint-auth", Pane: "%1"})
	s.mu.Unlock()

	a, b := net.Pipe()
	go s.HandlePeer(t.Context(), transport.NewConn(b), 100) //nolint:errcheck // ends with the test
	agent := transport.NewConn(a)
	t.Cleanup(func() { agent.Close() })
	go func() {
		_ = agent.Send(proto.KindHello, "", proto.Hello{Version: proto.Version, Client: proto.ClientConsole})
	}()
	next(t, agent, proto.KindHello)
	person := connectAs(t, t.Context(), s, proto.ClientConsole)
	agentCopies := make(chan string, 1)
	go func() {
		for {
			env, err := agent.Recv()
			if err != nil {
				return
			}
			if env.Kind == proto.KindCopy {
				agentCopies <- string(env.Body)
				return
			}
		}
	}()
	tm("set-buffer", "-b", "buffer6", "secret")
	if got := copyOf(t, person); got != "secret" {
		t.Fatalf("the person's console got %q", got)
	}
	select {
	case got := <-agentCopies:
		t.Fatalf("an agent's pane got the person's copy: %s", got)
	case <-time.After(500 * time.Millisecond):
	}
}
