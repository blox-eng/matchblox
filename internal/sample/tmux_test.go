package sample_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/tmuxtest"
)

// TestTmuxPanesKeepTabsWhenLangUnset: with LANG unset, tmux in the C locale
// replaces tabs in list-panes -F with "_", and ParsePanes finds nothing.
// The sampler's list must still yield panes (tmux -u), or Sessions is empty.
func TestTmuxPanesKeepTabsWhenLangUnset(t *testing.T) {
	server := tmuxtest.Server(t)
	cmd := exec.Command("tmux", "-L", server, "-f", os.DevNull, "new-session", "-d", "-s", "base", "sleep", "60") //nolint:gosec // test argv
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("new-session: %v\n%s", err, out)
	}
	for _, k := range []string{"LANG", "LC_ALL", "LC_CTYPE", "LC_MESSAGES"} {
		t.Setenv(k, "")
	}
	out, err := sample.TmuxPanesOn(server)
	if err != nil {
		t.Fatal(err)
	}
	panes := sample.ParsePanes(out)
	if len(panes) == 0 {
		t.Fatalf("no panes with LANG unset; list-panes was %q", out)
	}
	if panes[0].ID == "" || panes[0].Target == "" {
		t.Fatalf("pane incomplete: %+v (raw %q)", panes[0], out)
	}
}

// TestParsePanesSkipsJumpSessions: a jump's session on either side groups
// the agent's: its listing is the same pane, and `list-panes -a` sorts it
// first by name. The pane's own session stays, and so does a builder's
// session whose name looks like a jump's: the jump's mark says which.
func TestParsePanesSkipsJumpSessions(t *testing.T) {
	got := sample.ParsePanes([]byte("%1\tmatchblox-jump-0a1b2c3d:1.1\tw\t10\tclaude\t/x\n" +
		"%9\tmatchblox-ws-1-0a1b2c3d:1.1\tw\t30\tssh\t/y\t/s\t1\t1\n" +
		"%1\twork:1.1\tw\t10\tclaude\t/x\n" +
		"%5\tmatchblox-api-deadbeef:0.0\tw\t50\tclaude\t/z\t/s\t1\t\n"))
	if len(got) != 2 || got[0].Target != "work:1.1" || got[1].Target != "matchblox-api-deadbeef:0.0" {
		t.Fatalf("got %+v", got)
	}
}

// TestCapturePaneUpOnTmux: on a real server, the lines up end that many
// lines above the bottom, and an offset past the history stops at its top.
func TestCapturePaneUpOnTmux(t *testing.T) {
	server := tmuxtest.Server(t)
	cmd := exec.Command("tmux", "-L", server, "-f", os.DevNull, "new-session", "-d", "-s", "up", "-x", "40", "-y", "10", "seq -f line%g 1 30; sleep 60") //nolint:gosec // test argv
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("new-session: %v\n%s", err, out)
	}
	out, err := exec.Command("tmux", "-L", server, "list-panes", "-t", "up", "-F", "#{pane_id}").Output() //nolint:gosec // test argv
	if err != nil {
		t.Fatal(err)
	}
	pane := sample.OnServer(strings.TrimSpace(string(out)), server)
	var text string
	for deadline := time.Now().Add(5 * time.Second); !strings.Contains(text, "line30") && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		text, _, _ = sample.CapturePaneUp(pane, 0)
	}
	text, up, ok := sample.CapturePaneUp(pane, 3)
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if !ok || up != 3 || len(lines) != 10 || lines[0] != "line19" || lines[9] != "line28" {
		t.Fatalf("up %d ok %v lines %q", up, ok, lines)
	}
	text, up, _ = sample.CapturePaneUp(pane, 500)
	if first, _, _ := strings.Cut(text, "\n"); up != 21 || first != "line1" {
		t.Fatalf("up %d, first line %q; want 21 and line1", up, first)
	}
}
