package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestMachineMarkCountsANearlySpentPlan: a plan's window at 90% or more
// puts the ! on the Machine tab, as a detached busy loop does, and Machine
// names the plan and its window. Under 90% there is no mark.
func TestMachineMarkCountsANearlySpentPlan(t *testing.T) {
	st := limitsState() // OpenAI Plus's week is at 92%
	m, _ := loadedWith(t, 132, st)
	if label := ansi.Strip(m.tabLabel(tabMachine)); !strings.HasSuffix(label, " !") {
		t.Errorf("a plan at 92%% leaves the Machine tab without its mark: %q", label)
	}
	out := ansi.Strip(strings.Join(m.machinePanel(132).lines, "\n"))
	if !strings.Contains(out, "nearly spent: OpenAI Plus wk 92%") {
		t.Errorf("Machine does not name the nearly spent plan:\n%s", out)
	}

	st.Limits[1].Windows[1].UsedPct = 89
	m, _ = loadedWith(t, 132, st)
	if label := ansi.Strip(m.tabLabel(tabMachine)); strings.Contains(label, "!") {
		t.Errorf("a plan at 89%% marks the Machine tab: %q", label)
	}
	if out := ansi.Strip(strings.Join(m.machinePanel(132).lines, "\n")); strings.Contains(out, "nearly spent") {
		t.Errorf("Machine names a plan at 89%%:\n%s", out)
	}
}
