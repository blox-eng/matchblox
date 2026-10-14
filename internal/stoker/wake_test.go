package stoker

import (
	"reflect"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/ships"
)

func worker(pane, name string, ended time.Time) sample.Session {
	return sample.Session{Pane: pane, Name: name, Target: "app:1." + pane, TurnEnded: ended}
}

func TestWakeSkipsTheStokerPane(t *testing.T) {
	sessions := []sample.Session{
		worker("%1", "Weather", t0),
		{Pane: "%9@mbtest", Name: "stoker", Target: "stoker:1.1", TurnEnded: t0},
	}
	prev := Look(sessions[:1], nil, nil, nil, nil, t0, "mbtest")
	sessions[0].TurnEnded, sessions[1].TurnEnded = t0.Add(time.Minute), t0.Add(time.Minute)
	cur := Look(sessions, nil, nil, nil, nil, t0, "mbtest")
	got := Changes(prev, cur, sessions, "mbtest")
	if want := []string{"Weather's turn ended: idle"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
	// Even a Seen that carries the stoker's pane yields no line about it.
	both := Seen{Turns: map[string]time.Time{"%9@mbtest": t0.Add(time.Minute)}, Why: map[string]string{}}
	if got := Changes(Seen{Turns: map[string]time.Time{"%9@mbtest": t0}}, both, sessions, "mbtest"); len(got) != 0 {
		t.Fatalf("got %q", got)
	}
}

func usage(pct float64, window int) []limits.Account {
	return []limits.Account{{Provider: "claude", Account: "a", Windows: []limits.Window{{Minutes: window, UsedPct: pct, ResetsAt: t0.Add(time.Hour)}}}}
}

func TestChangesTurnShipUsage(t *testing.T) {
	sessions := []sample.Session{worker("%1", "Weather", time.Time{})}
	rep := func(stage int, extra ...ships.Ship) *ships.Report {
		sh := []ships.Ship{{Repo: "blox/blox", Number: 4551, Stage: stage, Session: ships.SessionRef{Pane: "%1", Name: "Weather"}}}
		return &ships.Report{Ships: append(sh, extra...)}
	}
	prev := Look(sessions, nil, nil, rep(ships.StageCI), usage(75, limits.WeekMinutes), t0, "mbtest")

	sessions[0].TurnEnded = t0
	why := map[string]string{"%1": "idle, #4556 waits your merge"}
	cur := Look(sessions, why, nil, rep(ships.StageGreen, ships.Ship{Repo: "blox/blox", Number: 4570, Stage: ships.StageOpen, Session: ships.SessionRef{Name: "Takeoff"}}), usage(81, limits.WeekMinutes), t0, "mbtest")
	want := []string{
		"Weather's turn ended: idle, #4556 waits your merge",
		"#4551 moved to green",
		"new PR #4570 by Takeoff",
		"claude week passed 80%",
	}
	if got := Changes(prev, cur, sessions, "mbtest"); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if got := Changes(cur, cur, sessions, "mbtest"); len(got) != 0 {
		t.Fatalf("nothing changed, got %q", got)
	}
}

func TestChangesUsageEveryTenPercentOfTheFiveHours(t *testing.T) {
	prev := Look(nil, nil, nil, nil, usage(41, 300), t0, "")
	for _, c := range []struct {
		pct  float64
		want []string
	}{{49, nil}, {52, []string{"claude 5h passed 50%"}}, {10, nil}} {
		got := Changes(prev, Look(nil, nil, nil, nil, usage(c.pct, 300), t0, ""), nil, "")
		if !reflect.DeepEqual(got, c.want) {
			t.Fatalf("%v%%: got %q", c.pct, got)
		}
	}
}

// TestCoalesceMergesRepeats: F7, one wake says each thing once: a line
// that repeats, and a session's turns that ended more than once, become
// one line with a count, the newest words kept.
func TestCoalesceMergesRepeats(t *testing.T) {
	got := Coalesce([]string{
		"Notifications's turn ended: idle",
		"#4551 moved to green",
		"Notifications's turn ended: Pushed.",
		"go given for step 1",
		"#4551 moved to green",
		"Notifications's turn ended: Tests pass.",
		"Weather asks: Which branch?",
	})
	want := []string{
		"Notifications's turn ended ×3: Tests pass.",
		"#4551 moved to green ×2",
		"go given for step 1",
		"Weather asks: Which branch?",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if got := Coalesce([]string{"a", "b"}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("got %q", got)
	}
}
