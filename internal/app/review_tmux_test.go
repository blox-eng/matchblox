//go:build linux

package app

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/tmuxtest"
)

// TestCtrlQReachesTheReviewPane: ctrl+q in REVIEW or COMMENT is esc, so the
// root bind sends it to the review pane. In every other mode it still goes
// to the list.
func TestCtrlQReachesTheReviewPane(t *testing.T) {
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script is not installed")
	}
	for _, c := range []struct {
		mode      string
		toReview  bool
		wantFocus string
	}{
		{"REVIEW", true, "seat"},
		{"COMMENT", true, "seat"},
		{"AGENT", false, "console"},
		{"LIST", false, "console"},
	} {
		t.Run(c.mode, func(t *testing.T) {
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
			// The seat shows what reaches it: -ixon so that ^Q is not eaten by the tty.
			seat := tm("split-window", "-d", "-h", "-t", console, "-P", "-F", "#{pane_id}", "sh", "-c", "stty -ixon; exec cat -v")
			tm("set-option", "-g", "status", "off")
			tm("set-option", "-w", "-t", console, sample.ConsoleOption, console)
			tm("set-option", "-w", "-t", console, sample.BesideOption, seat)
			tm("set-option", "-w", "-t", console, sample.ModeOption, c.mode)
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
			tm("select-pane", "-t", seat)
			waitTMUX(t, func() bool {
				return strings.Contains(tm("capture-pane", "-p", "-t", seat)+tm("display", "-p", "-t", seat, "#{pane_current_command}"), "cat")
			})
			if _, err := in.Write([]byte{0x11}); err != nil {
				t.Fatal(err)
			}
			if c.toReview {
				waitTMUX(t, func() bool { return strings.Contains(tm("capture-pane", "-p", "-t", seat), "^Q") })
				return
			}
			waitTMUX(t, func() bool { return tm("display", "-p", "-t", console, "#{pane_active}") == "1" })
			if got := tm("capture-pane", "-p", "-t", seat); strings.Contains(got, "^Q") {
				t.Fatalf("in %s ctrl+q reached the seat:\n%s", c.mode, got)
			}
		})
	}
}
