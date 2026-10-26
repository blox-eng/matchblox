package sample_test

import (
	"os"
	"os/exec"
	"testing"

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
// first by name. The pane's own session stays.
func TestParsePanesSkipsJumpSessions(t *testing.T) {
	got := sample.ParsePanes([]byte("%1\tmatchblox-jump-0a1b2c3d:1.1\tw\t10\tclaude\t/x\n" +
		"%9\tmatchblox-ws-1-0a1b2c3d:1.1\tw\t30\tssh\t/y\n" +
		"%1\twork:1.1\tw\t10\tclaude\t/x\n"))
	if len(got) != 1 || got[0].Target != "work:1.1" {
		t.Fatalf("got %+v", got)
	}
}
