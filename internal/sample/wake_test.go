package sample

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestANewPaneIsReadAtTheNextSample: tmux is listed at every sample; a pane
// not seen before reads the processes at once instead of after ProcEvery.
func TestANewPaneIsReadAtTheNextSample(t *testing.T) {
	root := copyFixture(t)
	smp := newFixtureSampler(root)
	smp.ProcEvery = time.Hour
	smp.Sample()

	// Positive control for the negative below: a new agent in a pane the
	// sampler knows waits for ProcEvery.
	addProc(t, root, 700, 500, "claude", []string{"claude"}, "%4")
	if _, ok := byPane(smp.Sample())["%4"]; ok {
		t.Fatal("an agent in a known pane read before ProcEvery: nothing woke the scan")
	}

	addProc(t, root, 800, 1, "claude", []string{"claude"}, "%9")
	addPane(t, root, "%9\tnew:1.1\tnew\t800\tclaude\t/work/new")
	got := byPane(smp.Sample())
	if _, ok := got["%9"]; !ok {
		t.Fatalf("the new pane's agent is missing: %v", got)
	}
}

// TestAnAgentStartedInAPaneIsReadAtTheNextSample: `tmux new`, then claude
// typed in its shell: the pane's command changes to an agent.
func TestAnAgentStartedInAPaneIsReadAtTheNextSample(t *testing.T) {
	root := copyFixture(t)
	smp := newFixtureSampler(root)
	smp.ProcEvery = time.Hour
	smp.Sample()

	addProc(t, root, 700, 500, "claude", []string{"claude"}, "%4")
	panes := filepath.Join(root, "tmux-panes.txt")
	b, _ := os.ReadFile(panes)
	_ = os.WriteFile(panes, []byte(strings.Replace(string(b), "500\tzsh", "500\tclaude", 1)), 0o644)
	if _, ok := byPane(smp.Sample())["%4"]; !ok {
		t.Fatal("the agent started in %4 is missing")
	}
}
