package testhome

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	os.Exit(Run(m))
}

// envSentinel names the session of the sentinel server to the child test.
const envSentinel = "MATCHBLOX_TEST_SENTINEL"

// TestATestCannotEndTheBuildersTmux: a builder runs the tests inside tmux.
// A cleanup that runs tmux with no socket then reaches the builder's server
// (#215): the one $TMUX names, or else the default one. A sentinel stands in
// for that server. A child test binary inherits the builder's environment
// and runs a cleanup with no socket. The sentinel must survive.
func TestATestCannotEndTheBuildersTmux(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	t.Run("the server $TMUX names", func(t *testing.T) {
		sock := filepath.Join(shortDir(t), "s")
		sentinelSurvives(t, sock, "TMUX="+sock+",1,0", "TMUX_PANE=%0")
	})
	t.Run("the default server", func(t *testing.T) {
		dir := shortDir(t)
		sentinelSurvives(t, filepath.Join(dir, fmt.Sprintf("tmux-%d", os.Getuid()), "default"), "TMUX_TMPDIR="+dir)
	})
}

// shortDir is a new folder under /tmp, not macOS $TMPDIR: a socket path
// in it must fit sun_path.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "mbs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// sentinelSurvives starts a sentinel server on sock, proves that a tmux
// with no socket in the builder's environment reaches it, and runs the
// child test in that environment.
func sentinelSurvives(t *testing.T, sock string, builders ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	session := fmt.Sprintf("sentinel-%d-%s", os.Getpid(), filepath.Base(filepath.Dir(sock))) // a name only this sentinel has
	sentinel := func(args ...string) error {
		return exec.Command("tmux", append([]string{"-S", sock}, args...)...).Run() //nolint:gosec // test argv
	}
	if err := sentinel("-f", os.DevNull, "new-session", "-d", "-s", session, "sleep", "300"); err != nil {
		t.Fatalf("the sentinel does not start: %v", err)
	}
	t.Cleanup(func() { _ = sentinel("kill-server") })
	env := append(os.Environ(), builders...) // the last value of a name wins

	// The control: tmux with no socket in this environment reaches the
	// sentinel. Without it, the test below proves nothing.
	control := exec.Command("tmux", "has-session", "-t", "="+session) //nolint:gosec // no socket on purpose: the control
	control.Env = env
	if err := control.Run(); err != nil {
		t.Fatalf("tmux with no socket and %q does not reach the sentinel: %v", builders, err)
	}

	child := exec.Command(os.Args[0], "-test.run=^TestCleanupWithNoSocket$", "-test.count=1") //nolint:gosec // this test binary
	child.Env = append(env, envSentinel+"="+session)
	if out, err := child.CombinedOutput(); err != nil {
		t.Fatalf("the child test fails: %v\n%s", err, out)
	}
	if err := sentinel("has-session", "-t", "="+session); err != nil {
		t.Fatalf("a test with %q ended the sentinel: testhome.Run must clear $TMUX and set its own $TMUX_TMPDIR", builders)
	}
}

// TestCleanupWithNoSocket is the child of TestATestCannotEndTheBuildersTmux.
// It runs the cleanup of the incident: tmux with no socket. It kills only the
// sentinel's session, by its unique name, so a broken Run ends the sentinel
// and nothing of the builder's.
func TestCleanupWithNoSocket(t *testing.T) {
	session := os.Getenv(envSentinel)
	if session == "" {
		t.Skip("runs as the child of TestATestCannotEndTheBuildersTmux")
	}
	for _, v := range []string{"TMUX", "TMUX_PANE"} {
		if got, ok := os.LookupEnv(v); ok {
			t.Errorf("$%s is %q in a test: testhome.Run must clear it", v, got)
		}
	}
	_ = exec.Command("tmux", "kill-session", "-t", "="+session).Run() //nolint:gosec // no socket on purpose: the incident's command
}

// TestBuildersTmuxIsTheInheritedSocket: Run keeps the socket the builder's
// $TMUX named, for the test that reads the builder's session names.
func TestBuildersTmuxIsTheInheritedSocket(t *testing.T) {
	for in, want := range map[string]string{
		"/tmp/tmux-501/default,4242,0": "/tmp/tmux-501/default",
		"/tmp/tmux-501/a,b,4242,0":     "/tmp/tmux-501/a,b",
		"":                             "",
		"garbage":                      "",
	} {
		if got := socketOf(in); got != want {
			t.Errorf("socketOf(%q) = %q, want %q", in, got, want)
		}
	}
	if strings.Contains(os.Getenv("TMUX"), ",") {
		t.Fatal("$TMUX is set in a test")
	}
}

// envLeave asks the child test to leave a tmux server running.
const envLeave = "MATCHBLOX_TEST_LEAVE_SERVER"

// TestRunEndsTheServersItsTestsLeave: Run removes its $TMUX_TMPDIR when the
// tests end. A server that a test leaves would keep running with its socket
// gone, and no tmux could reach it any more. So Run ends it first.
func TestRunEndsTheServersItsTestsLeave(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	child := exec.Command(os.Args[0], "-test.run=^TestLeaveAServer$", "-test.count=1") //nolint:gosec // this test binary
	child.Env = append(os.Environ(), envLeave+"=1")
	out, err := child.Output()
	if err != nil {
		t.Fatalf("the child test fails: %v\n%s", err, out)
	}
	var pid int
	if _, err := fmt.Sscanf(string(out), "server pid %d", &pid); err != nil || pid <= 1 {
		t.Fatalf("no server pid from the child: %q", out)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	deadline := time.Now().Add(5 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatalf("the server a test left (pid %d) still runs after Run removed its socket", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestLeaveAServer is the child of TestRunEndsTheServersItsTestsLeave. It
// starts a server and ends no server.
func TestLeaveAServer(t *testing.T) {
	if os.Getenv(envLeave) == "" {
		t.Skip("runs as the child of TestRunEndsTheServersItsTestsLeave")
	}
	out, err := exec.Command("tmux", "-L", "left", "-f", os.DevNull, "new-session", "-d", "-s", "left", "sleep", "300", ";", "display-message", "-p", "#{pid}").Output() //nolint:gosec // test argv
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("server pid %s", out)
}
