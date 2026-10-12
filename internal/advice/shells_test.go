package advice

import (
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/sample"
)

// A background shell that ran for hours after its session's turn ended is
// named with its command, and ending it is x and a typed y, guarded so a
// reused pid is never killed.
func TestALongBackgroundShellCanBeEnded(t *testing.T) {
	now := time.Date(2026, 10, 12, 7, 45, 0, 0, time.UTC)
	snap := sample.Snapshot{At: now, Sessions: []sample.Session{{
		Pane: "%5", Name: "Billed", Status: "idle", Agent: "claude",
		Shells: []sample.Shell{
			{PID: 2240632, Start: 99, Started: now.Add(-37 * time.Hour), Command: `while pgrep -f "node walk.mjs" >/dev/null; do sleep 15; done`},
			{PID: 194234, Start: 98, Started: now.Add(-20 * time.Minute), Command: "npm run build"},
		},
	}}}
	var got []Rec
	for _, r := range Build(snap, nil) {
		if strings.Contains(r.Title, "background shell") {
			got = append(got, r)
		}
	}
	if len(got) != 1 {
		t.Fatalf("recs %+v, want one for the long shell", got)
	}
	r := got[0]
	if r.Title != "End a background shell of Billed (%5) that has run 1d13h" || !strings.Contains(r.Evidence, "node walk.mjs") || !strings.Contains(r.Evidence, "pid 2240632") {
		t.Fatalf("rec %q · %q", r.Title, r.Evidence)
	}
	if r.Primary == nil || !r.Primary.Nav || r.Second == nil || !r.Second.Destructive ||
		len(r.Second.Steps) != 1 || strings.Join(r.Second.Steps[0], " ") != "kill 2240632" ||
		len(r.Second.Guards) != 1 || r.Second.Guards[0] != (Guard{PID: 2240632, StartTicks: 99}) {
		t.Fatalf("actions %+v %+v", r.Primary, r.Second)
	}
}
