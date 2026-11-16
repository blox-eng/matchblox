package remote

import (
	"slices"
	"strings"
	"testing"
)

func TestSplitArgvGivesTheAgentTheRest(t *testing.T) {
	argv := SplitArgv("%1", 40, 160)
	if got := strings.Join(argv[:8], " "); got != "tmux split-window -h -d -t %1 -l 75%" {
		t.Fatalf("argv = %q", got)
	}
	for _, want := range []string{"-P", "#{pane_id}", "--"} {
		if !slices.Contains(argv, want) {
			t.Errorf("no %q in %q", want, argv)
		}
	}
	if got := SplitArgv("%1", 40, 0)[7]; got != "60%" {
		t.Errorf("unknown window: %q", got)
	}
}

func TestBesideArgvNamesBothPanesAndPaintsTheBorder(t *testing.T) {
	got := strings.Join(BesideArgv("%1", "%9", "#c3a56a"), " ")
	for _, want := range []string{"@matchblox_console %1", "@matchblox_beside %9", "pane-active-border-style fg=#c3a56a"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in %q", want, got)
		}
	}
}
