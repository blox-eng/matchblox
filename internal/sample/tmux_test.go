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
