package testhome

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	os.Exit(Run(m))
}

// envSentinel names the session of the sentinel server to the child test.
const envSentinel = "MATCHBLOX_TEST_SENTINEL"

// TestATestCannotEndTheBuildersTmux: a builder runs the tests inside tmux,
// so the test binary inherits $TMUX. A cleanup that runs tmux without a
// socket then reaches the builder's server (#215). The sentinel stands in
// for that server: a child test binary inherits a $TMUX that names it, and
// runs a cleanup with no socket. The sentinel must survive.
func TestATestCannotEndTheBuildersTmux(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	dir, err := os.MkdirTemp("/tmp", "mbs") // short, not macOS $TMPDIR: the socket path must fit sun_path
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")
	session := fmt.Sprintf("sentinel-%d", os.Getpid()) // a name only the sentinel has
	sentinel := func(args ...string) error {
		return exec.Command("tmux", append([]string{"-S", sock}, args...)...).Run() //nolint:gosec // test argv
	}
	if err := sentinel("-f", os.DevNull, "new-session", "-d", "-s", session, "sleep", "300"); err != nil {
		t.Fatalf("the sentinel does not start: %v", err)
	}
	t.Cleanup(func() { _ = sentinel("kill-server") })
	inherited := append(os.Environ(), "TMUX="+sock+",1,0", "TMUX_PANE=%0")

	// The control: tmux with no socket and this $TMUX reaches the sentinel.
	// Without it, the test below proves nothing.
	control := exec.Command("tmux", "has-session", "-t", "="+session) //nolint:gosec // no socket on purpose: the control
	control.Env = inherited
	if err := control.Run(); err != nil {
		t.Fatalf("tmux with an inherited $TMUX does not reach the sentinel: %v", err)
	}

	child := exec.Command(os.Args[0], "-test.run=^TestCleanupWithNoSocket$", "-test.count=1") //nolint:gosec // this test binary
	child.Env = append(inherited, envSentinel+"="+session)
	if out, err := child.CombinedOutput(); err != nil {
		t.Fatalf("the child test fails: %v\n%s", err, out)
	}
	if err := sentinel("has-session", "-t", "="+session); err != nil {
		t.Fatal("a test with an inherited $TMUX ended the sentinel: testhome.Run must clear $TMUX")
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
