package sample

import (
	"strings"
	"testing"
	"time"
)

// TestAPaneShowsBusyIdleAndWhatItAsks: an agent without an adapter is busy
// while its pane changes, idle from the last change, and its last line and
// question come from the pane. Context use is not measured.
func TestAPaneShowsBusyIdleAndWhatItAsks(t *testing.T) {
	root := copyFixture(t)
	addProc(t, root, 800, 1, "codex", []string{"codex"}, "%10")
	addPane(t, root, "%10\tapi:1.1\tapi\t800\tcodex\t/work/api")
	screen := "• Working (3s • esc to interrupt)\n"
	var asked [][]string
	now := fixtureNow
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "codex"}
	smp.Now = func() time.Time { return now }
	smp.Capture = func(panes []string) map[string]string {
		asked = append(asked, panes)
		return map[string]string{"%10": screen}
	}

	if s := byPane(smp.Sample())["%10"]; s.Status != "" || s.Busy {
		t.Fatalf("first sight: nothing is known yet, got %+v", s)
	}
	now = now.Add(2 * time.Second)
	screen = "• Working (5s • esc to interrupt)\n"
	if s := byPane(smp.Sample())["%10"]; !s.Busy || s.Status != "busy" {
		t.Fatalf("the pane changed: busy, got %+v", s)
	}
	now = now.Add(2 * time.Second)
	screen = "  Would you like to run the following command?\n  $ make test\n"
	smp.Sample()
	now = now.Add(40 * time.Second)
	s := byPane(smp.Sample())["%10"]
	if s.Busy || s.Status != "idle" || s.Idle != 40*time.Second {
		t.Fatalf("unchanged for 40 s: idle 40s, got busy=%v status=%q idle=%v", s.Busy, s.Status, s.Idle)
	}
	if !s.Asks || s.LastLine != "Would you like to run the following command?" {
		t.Fatalf("the pane asks: got asks=%v line=%q", s.Asks, s.LastLine)
	}
	if s.Context != "unmeasured" {
		t.Fatalf("no adapter: context %q, want unmeasured", s.Context)
	}
	for _, panes := range asked {
		if strings.Join(panes, ",") != "%10" {
			t.Fatalf("only panes of agents without an adapter are read, got %q", panes)
		}
	}
}
