package demo

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/history"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/ships"
)

// historySpan is how far back the machine's trend reaches when the demo
// opens.
const historySpan = 2 * time.Hour

// writeHistory writes two hours of the machine's trend, one point every
// history.Every and the last one a step before the demo's clock, so the live
// samples carry on from it. It is a function of the fixture's time: the same
// trend on every run.
func writeHistory(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	_ = w.Write(history.Columns)
	n := int(historySpan / history.Every)
	f1 := func(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }
	for i := range n {
		at := fixtureNow.Add(-time.Duration(n-i) * history.Every)
		t := float64(i)
		work := 0.5 + 0.5*math.Sin(t/17) // the agents' turns come and go
		cpu := 18 + 40*work + 6*math.Sin(t/3)
		_ = w.Write([]string{at.Format(time.RFC3339), f1(cpu), f1(3.5 + 3.5*work), f1(math.Max(0, 12*work-3)),
			f1(math.Max(0, 5*math.Sin(t/23))), f1(52 + 16*work), f1(40 - 6*work), f1(0.1 + 0.2*work)})
	}
	w.Flush()
	return errors.Join(w.Error(), f.Close())
}

// demoShips follows the pull requests of the sessions the demo machine runs:
// one of each kind. It reads no GitHub, so it is the same on every poll.
func demoShips(_ context.Context, sessions []sample.Session, _ *gitscan.Report) ships.Report {
	owner := map[string]ships.SessionRef{}
	for _, s := range sessions {
		owner[s.Name] = ships.SessionRef{Pane: s.Pane, ID: s.SessionID, Name: s.Name}
	}
	ship := func(n int, session, title, kind, note string, stage int, age time.Duration) ships.Ship {
		repo := "hearth/" + session
		since := fixtureNow.Add(-age)
		sh := ships.Ship{Repo: repo, Number: n, Title: title, URL: fmt.Sprintf("https://example.com/%s/pull/%d", repo, n),
			Session: owner[session], Evidence: ships.Created, Stage: stage, Kind: kind, Note: note, Since: since}
		for i := 0; i <= stage; i++ {
			sh.Stages[i].At = since.Add(-time.Duration(stage-i) * 25 * time.Minute)
		}
		sh.Stages[stage].Note = note
		return sh
	}
	waits := ship(214, "ember-billing", "Invoice export as CSV and PDF", ships.WaitsForYou, "green · waits for your merge", ships.StageGreen, 6*time.Minute)
	waits.Merge = []string{"gh", "pr", "merge", "214", "--squash", "-R", waits.Repo}
	return ships.Report{At: fixtureNow, InFlight: 3, Ships: []ships.Ship{
		ship(219, "hearth-api", "Rate limits on the public API", ships.Running, "CI · 6 of 9 jobs", ships.StageCI, 4*time.Minute),
		waits,
		ship(205, "flint-auth", "Refresh tokens that rotate", ships.Stuck, "conflicts with main", ships.StageCI, 40*time.Minute),
		ship(198, "spark-docs", "The 1.0 docs", ships.Done, "live", ships.StageLive, 2*time.Hour),
	}}
}
