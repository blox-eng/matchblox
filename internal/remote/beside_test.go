package remote

import (
	"strings"
	"testing"
)

func TestBesidePercentGivesTheAgentTheRest(t *testing.T) {
	if got := BesidePercent(40, 160); got != 75 {
		t.Errorf("40 of 160: %d%%, want 75%%", got)
	}
	if got := BesidePercent(40, 0); got != 60 {
		t.Errorf("unknown window: %d%%, want 60%%", got)
	}
	if got := BesideShell(); len(got) != 3 || got[0] != "/bin/sh" || got[1] != "-c" {
		t.Errorf("BesideShell = %q", got)
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

func TestChainArgvSplitsTheAgentPaneOnly(t *testing.T) {
	views := [][]string{{"v0"}, {"v1"}, {"v2"}, {"v3"}}
	two := strings.Join(ChainArgv("%9", views[:2]), " ")
	for _, want := range []string{"tmux respawn-pane -k -t %9 -- v0 ;", "set-option -w -t %9 @matchblox_chain %9", "split-window -v -t %9 -P -F 1 #{pane_id} -- v1"} {
		if !strings.Contains(two, want) {
			t.Errorf("no %q in %q", want, two)
		}
	}
	if strings.Count(two, "split-window") != 1 {
		t.Errorf("2 views: %q", two)
	}
	four := strings.Join(ChainArgv("%9", views), " ")
	if strings.Count(four, "split-window") != 3 || strings.Count(four, "set-option -awF @matchblox_chain") != 3 {
		t.Errorf("4 views: %q", four)
	}
	if three := strings.Join(ChainArgv("%9", views[:3]), " "); strings.Contains(three, "v3") || strings.Count(three, "split-window") != 2 {
		t.Errorf("3 views: %q", three)
	}
}
