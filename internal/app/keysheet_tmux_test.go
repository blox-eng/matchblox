//go:build linux

package app

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/tmuxtest"
)

// TestCommaTypesIntoTheAgent: with the agent pane focused, "," and "?"
// reach that pane. The keys sheet does not open, and Machine does not.
func TestCommaTypesIntoTheAgent(t *testing.T) {
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script is not installed")
	}
	sock := tmuxtest.Server(t)
	tm := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("tmux", append([]string{"-L", sock}, args...)...) //nolint:gosec // a test's own server
		cmd.Env = noTmuxEnv()
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %q: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	console := tm("-f", "/dev/null", "new-session", "-d", "-s", "matchblox", "-x", "100", "-y", "30", "-P", "-F", "#{pane_id}", "sleep", "600")
	agent := tm("split-window", "-d", "-h", "-t", console, "-P", "-F", "#{pane_id}", "cat")
	tm("set-option", "-g", "status", "off")
	tm("set-option", "-w", "-t", console, sample.ConsoleOption, console)
	tm("set-option", "-w", "-t", console, sample.BesideOption, agent)
	tm("set-option", "-t", console, panes.HomeOption, "1")
	tm(panes.BesideKeysArgv()[1:]...)

	log := filepath.Join(t.TempDir(), "terminal")
	client := exec.Command("script", "-qfc", "stty rows 30 cols 100; exec tmux -L "+sock+" attach -t matchblox", log) //nolint:gosec // a test's own server
	client.Env = append(noTmuxEnv(), "TERM=xterm-256color", "SHELL=/bin/sh")
	in, err := client.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close(); _ = client.Process.Kill(); _ = client.Wait() })
	waitTMUX(t, func() bool { return tm("list-clients", "-F", "#{client_session}") == "matchblox" })
	tm("select-pane", "-t", agent)
	if _, err := in.Write([]byte(",?")); err != nil {
		t.Fatal(err)
	}

	var capAgent string
	waitTMUX(t, func() bool {
		cmd := exec.Command("tmux", "-L", sock, "capture-pane", "-p", "-t", agent) //nolint:gosec // a test's own server
		cmd.Env = noTmuxEnv()
		out, err := cmd.Output()
		if err != nil {
			return false
		}
		capAgent = string(out)
		return strings.Contains(capAgent, ",") && strings.Contains(capAgent, "?")
	})
	listCmd := exec.Command("tmux", "-L", sock, "capture-pane", "-p", "-t", console) //nolint:gosec // a test's own server
	listCmd.Env = noTmuxEnv()
	listOut, _ := listCmd.Output()
	if strings.Contains(string(listOut), ",") || strings.Contains(string(listOut), "anywhere") || strings.Contains(string(listOut), "USAGE") {
		t.Fatalf("the list pane took the keys or opened ⚙:\n%s", listOut)
	}
	binds := tm("list-keys", "-T", "root")
	for _, line := range strings.Split(binds, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[0] == "bind-key" && fields[2] == "root" && (fields[3] == "," || fields[3] == "?") {
			t.Fatalf("root binds %s: %s", fields[3], line)
		}
	}
	if strings.Contains(capAgent, "⚙") {
		t.Fatalf("agent capture shows ⚙:\n%s", capAgent)
	}
}

func waitTMUX(t *testing.T, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(4 * time.Second); time.Now().Before(end); time.Sleep(30 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatal("timed out")
}
