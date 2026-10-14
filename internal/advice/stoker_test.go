package advice

import (
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/sample"
)

// The stoker's own server is not a lane: no rec names a session there, and
// none offers a jump the console would refuse. The header carries its waits.
func TestNoRecNamesTheStokersOwnServer(t *testing.T) {
	now := time.Date(2026, 10, 14, 2, 0, 0, 0, time.UTC)
	stoker := sample.OnServer("%9", sample.StokerServer)
	acct := limits.Account{Provider: "anthropic", Account: "a@example.com · Max", State: limits.Measured,
		Windows:  []limits.Window{{UsedPct: 72, Minutes: limits.WeekMinutes, ResetsAt: now.Add(48 * time.Hour)}},
		Forecast: &limits.Outlook{Out: now.Add(5 * time.Hour)}}
	snap := sample.Snapshot{
		At:     now,
		Limits: []limits.Account{acct},
		Orphans: []sample.Orphan{{PID: 42, Comm: "bash", CPU: 99, HotFor: time.Hour, Parent: "init",
			Pane: stoker, PaneAlive: true, Target: "stoker:0.0", Kill: []string{"kill", "42"}}},
		Alerts: []sample.Alert{{Key: "load1", Evidence: "load 9"}},
		Top:    []sample.ProcRow{{PID: 43, Comm: "node", CPU: 300, Owner: "pane " + stoker}},
		Sessions: []sample.Session{{
			Pane: stoker, Name: "stoker", Status: "idle", Do: "compact", ContextPct: 95, Why: "context 95%",
			Provider: "anthropic", Account: acct.Account, Burn30m: 90000, Tokens: 190000,
			Shells: []sample.Shell{{PID: 7, Start: 1, Group: true, Started: now.Add(-3 * time.Hour), Command: "sleep 99999"}},
		}},
	}
	for _, r := range Build(snap, nil) {
		if strings.Contains(r.Title, "stoker") || strings.Contains(r.Title, stoker) {
			t.Errorf("a rec names the stoker's session: %q", r.Title)
		}
		for _, a := range []*Action{r.Primary, r.Second} {
			if a == nil {
				continue
			}
			for _, s := range a.Steps {
				if strings.Contains(strings.Join(s, " "), "@"+sample.StokerServer) {
					t.Errorf("%q acts on the stoker's server: %q", r.Title, s)
				}
			}
		}
	}
}
