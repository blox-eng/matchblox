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

// watching starts the copy watcher of s on sock and gives it two reads, so
// the buffers that were there before are its baseline.
func watching(t *testing.T, s *Service, sock string, tm func(args ...string) string) {
	t.Helper()
	ctx := run(t, s)
	go s.copyWatch(ctx, sock)
	time.Sleep(3 * copyEvery)
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

// TestTheWatcherWaitsForAServer: on a socket with no server it starts none,
// and once a server runs there its copies arrive.
func TestTheWatcherWaitsForAServer(t *testing.T) {
	name := tmuxtest.Server(t)
	sock := sample.SocketPath(name)
	s := newTest(t, time.Hour)
	ctx := run(t, s)
	go s.copyWatch(ctx, sock)
	console := connectAs(t, t.Context(), s, proto.ClientConsole)
	time.Sleep(3 * copyEvery)
	if err := exec.Command("tmux", "-L", name, "list-sessions").Run(); err == nil { //nolint:gosec // a test's own server
		t.Fatal("the watcher started a tmux server")
	}
	tm := func(args ...string) {
		if out, err := exec.Command("tmux", append([]string{"-L", name}, args...)...).CombinedOutput(); err != nil { //nolint:gosec // a test's own server
			t.Fatalf("tmux %v: %v: %s", args, err, out)
		}
	}
	tm("-f", "/dev/null", "new-session", "-d", "-s", "work", "sleep", "600")
	time.Sleep(3 * copyEvery)
	tm("set-buffer", "-b", "buffer2", "later")
	if got := copyOf(t, console); got != "later" {
		t.Fatalf("console got %q, want later", got)
	}
}

// TestTheWatcherLeavesNoSession: the watcher adds no session and no client
// to the person's tmux: a plain tmux attach lands in the person's own
// session, and tmux ls lists only theirs.
func TestTheWatcherLeavesNoSession(t *testing.T) {
	sock, tm := copyServer(t)
	s := newTest(t, time.Hour)
	watching(t, s, sock, tm)
	if got := tm("list-sessions", "-F", "#{session_name}"); got != "work" {
		t.Fatalf("tmux ls lists %q, want only work", got)
	}
	if got := tm("list-clients", "-F", "#{client_name}"); got != "" {
		t.Fatalf("the watcher added clients %q", got)
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
	waitUntil(t, "tmux tells the person", func() bool {
		return strings.Contains(messages(sock, person), "not carried: 2.0 MB is over matchblox's 1 MB copy limit")
	})
	tm("set-buffer", "-b", "buffer4", "small") // a person's next copy, later than one read
	if got := copyOf(t, console); got != "small" {
		t.Fatalf("console got %d bytes first, want the small copy", len(got))
	}
}

// TestACopyWithNoConsoleSaysNothing: with no console, tmux's own copy
// reaches a terminal attached to it, so matchblox says nothing there: a
// "not copied" would be false.
func TestACopyWithNoConsoleSaysNothing(t *testing.T) {
	sock, tm := copyServer(t)
	s := newTest(t, time.Hour)
	watching(t, s, sock, tm)
	person := attachedClient(t, sock, "work")
	tm("set-buffer", "-b", "buffer5", "fine")
	time.Sleep(3 * copyEvery)
	if got := messages(sock, person); strings.Contains(got, "copied") {
		t.Fatalf("tmux showed %q", got)
	}
}

// TestAnAgentThatSaysConsoleGetsNoCopies: a client whose process runs in an
// agent's pane is no console, whatever its hello says, as for screens: it
// gets no copy of what the person copied.
func TestAnAgentThatSaysConsoleGetsNoCopies(t *testing.T) {
	sock, tm := copyServer(t)
	s := newTest(t, time.Hour)
	s.PeerPane = func(pid int) string { return map[int]string{100: "%77"}[pid] }
	watching(t, s, sock, tm)
	waitUntil(t, "the first sample", func() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.have })
	s.mu.Lock()
	s.cur.Sessions = append(s.cur.Sessions, sample.Session{Name: "flint-auth", Pane: "%77"})
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
	tm("set-buffer", "-b", "buffer6", "secret") // %77 is no fixture session's: only the one added above
	if got := copyOf(t, person); got != "secret" {
		t.Fatalf("the person's console got %q", got)
	}
	select {
	case got := <-agentCopies:
		t.Fatalf("an agent's pane got the person's copy: %s", got)
	case <-time.After(500 * time.Millisecond):
	}
}

// TestACopyCarriesNoControlCodes: a person's tmux copy holds none, so a
// buffer with escape codes loses them before it reaches any clipboard.
func TestACopyCarriesNoControlCodes(t *testing.T) {
	sock, tm := copyServer(t)
	s := newTest(t, time.Hour)
	watching(t, s, sock, tm)
	console := connectAs(t, t.Context(), s, proto.ClientConsole)
	tm("set-buffer", "-b", "buffer7", "\x1b]52;c;eA==\x07ok\tgo\nnext\r")
	if got, want := copyOf(t, console), "]52;c;eA==ok\tgo\nnext"; got != want {
		t.Fatalf("console got %q, want %q", got, want)
	}
}

// TestAnAgentPaneFoundLaterGetsNoCopies: the console rule runs at each copy:
// a client whose pane turns out to be an agent's after it said hello gets no
// copy from then on.
func TestAnAgentPaneFoundLaterGetsNoCopies(t *testing.T) {
	sock, tm := copyServer(t)
	s := newTest(t, time.Hour)
	s.PeerPane = func(pid int) string { return map[int]string{100: "%77"}[pid] }
	watching(t, s, sock, tm)
	waitUntil(t, "the first sample", func() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.have })
	a, b := net.Pipe()
	go s.HandlePeer(t.Context(), transport.NewConn(b), 100) //nolint:errcheck // ends with the test
	agent := transport.NewConn(a)
	t.Cleanup(func() { agent.Close() })
	go func() {
		_ = agent.Send(proto.KindHello, "", proto.Hello{Version: proto.Version, Client: proto.ClientConsole})
	}()
	next(t, agent, proto.KindHello)
	next(t, agent, proto.KindSnapshot) // it has joined, as a console
	s.mu.Lock()
	s.cur.Sessions = append(s.cur.Sessions, sample.Session{Name: "flint-auth", Pane: "%77"})
	s.mu.Unlock()
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
	tm("set-buffer", "-b", "buffer8", "secret")
	if got := copyOf(t, person); got != "secret" {
		t.Fatalf("the person's console got %q", got)
	}
	select {
	case got := <-agentCopies:
		t.Fatalf("a pane found to be an agent's got the copy: %s", got)
	case <-time.After(500 * time.Millisecond):
	}
}
