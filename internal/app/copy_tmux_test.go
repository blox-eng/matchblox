//go:build linux

package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/tmuxtest"
)

// TestAHostCopyReachesTheDeskTerminal: a host's copy, as it comes over
// matchblox's link, reaches the desk terminal through the console's own
// tmux, with set-clipboard left at external and no passthrough on any pane
// (spec R7). The terminal is script(1); its log must hold the OSC 52.
func TestAHostCopyReachesTheDeskTerminal(t *testing.T) {
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script is not installed")
	}
	desk := tmuxtest.Server(t)
	tm := func(args ...string) (string, error) {
		cmd := exec.Command("tmux", append([]string{"-L", desk}, args...)...) //nolint:gosec // a test's own server
		cmd.Env = noTmuxEnv()
		out, err := cmd.Output()
		return string(out), err
	}
	if out, err := tm("-f", "/dev/null", "new-session", "-d", "-s", "matchblox", "-x", "100", "-y", "30", "sleep", "600"); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	_, _ = tm("set-option", "-s", "set-clipboard", "external")
	pane, _ := tm("display", "-p", "-t", "=matchblox:", "#{pane_id}")

	log := filepath.Join(t.TempDir(), "terminal")
	client := exec.Command("script", "-qfc", "stty rows 30 cols 100; exec tmux -L "+desk+" attach -t '=matchblox'", log) //nolint:gosec // a test's own server
	client.Env = append(noTmuxEnv(), "TERM=xterm-256color", "SHELL=/bin/sh")
	in, err := client.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close(); _ = client.Process.Kill(); _ = client.Wait() })
	for end := time.Now().Add(5 * time.Second); ; time.Sleep(30 * time.Millisecond) {
		if out, _ := tm("list-clients", "-F", "#{client_name}"); strings.TrimSpace(out) != "" {
			break
		}
		if time.Now().After(end) {
			t.Fatal("no desk client")
		}
	}

	ws := newFake()
	m := New(Options{NoMotion: true, Pane: strings.TrimSpace(pane), Tmux: tm, Peers: []Peer{{Host: "ws-1", Conn: ws}}})
	m.besideShows = paneKey{"ws-1", "%7"}
	next, _ := m.Update(peerMsg{host: "ws-1", conn: ws, msg: copyMsg{Text: "from the host"}})
	if said := next.(Model).noticeText; !strings.HasPrefix(said, "copied: 13 characters from ws-1") {
		t.Fatalf("said %q", said)
	}
	// tmux writes ESC ] 52 ; <selection, often empty> ; <base64>.
	const payload = ";ZnJvbSB0aGUgaG9zdA==" // base64 of "from the host"
	for end := time.Now().Add(5 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		b, _ := os.ReadFile(log)
		if i := strings.Index(string(b), "\x1b]52;"); i >= 0 && strings.Contains(string(b[i:]), payload) {
			break
		}
		if time.Now().After(end) {
			t.Fatalf("the desk terminal got no OSC 52 of the copy")
		}
	}
	if v, _ := tm("show", "-sv", "set-clipboard"); strings.TrimSpace(v) != "external" {
		t.Errorf("set-clipboard is %q, want external as it was", v)
	}
	if v, _ := tm("list-panes", "-a", "-F", "#{allow-passthrough}"); strings.Contains(v, "on") {
		t.Errorf("a pane has passthrough on: %q", v)
	}
}

func noTmuxEnv() []string {
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "TMUX") {
			env = append(env, e)
		}
	}
	return env
}
