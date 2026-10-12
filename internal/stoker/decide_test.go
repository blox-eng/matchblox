package stoker

import (
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/sample"
)

var line80 = sample.Full{Pct: 80}

// building is an idle session of a 1M window at tokens, its plan at pct.
func building(tokens, pct int) sample.Session {
	return sample.Session{
		Agent: "claude", Pane: "%5", SessionID: "billed", Name: "Billed", Status: "idle",
		Tokens: tokens, Window: 1_000_000, ContextPct: float64(tokens) / 1e4,
		Progress: &sample.Progress{Pct: pct, Step: "Task"},
	}
}

func TestDecide(t *testing.T) {
	for _, c := range []struct {
		name    string
		s       sample.Session
		mark    *Mark
		compact bool
		why     string
	}{
		// Billed's night: 556k at 14%, 692k at 64%: 2.7k a point, 36 points
		// left, so it finishes near 790k, under the 800k line.
		{"finishes inside the line", building(692_000, 64), &Mark{Tokens: 556_000, Pct: 14}, false, "finishes ~79%"},
		{"the next step would cross it", building(740_000, 64), &Mark{Tokens: 300_000, Pct: 14}, true, "would pass 80% within the next step"},
		{"crosses it later", building(500_000, 50), &Mark{Tokens: 200_000, Pct: 14}, false, "compacts near"},
		{"past the line", building(810_000, 90), &Mark{Tokens: 556_000, Pct: 14}, true, "past 80%"},
		{"no pace yet", building(600_000, 16), &Mark{Tokens: 590_000, Pct: 14}, false, ""},
		{"no bar, under the line", building(600_000, 0), nil, false, ""},
		{"done", building(700_000, 100), &Mark{Tokens: 556_000, Pct: 14}, false, "done"},
	} {
		if c.s.Progress.Pct == 0 {
			c.s.Progress = nil
		}
		v := Decide(c.s, c.mark, line80)
		if v.Compact != c.compact || !strings.Contains(v.Why, c.why) || c.why == "" && v.Why != "" {
			t.Errorf("%s: %+v, want compact %v and %q", c.name, v, c.compact, c.why)
		}
	}
}

// The pace starts where the run first saw a session's plan, and starts again
// when a compact or a new plan takes the context or the bar back.
func TestThePaceStartsAgainAfterACompactOrANewPlan(t *testing.T) {
	m := on()
	m, _ = m.Observe([]sample.Session{building(556_000, 14)}, nil, line80, t0)
	m, _ = m.Observe([]sample.Session{building(692_000, 64)}, nil, line80, t0)
	if got := m.Pace["billed"]; got != (Mark{Tokens: 556_000, Pct: 14}) {
		t.Fatalf("pace %+v, want the first sample", got)
	}
	m, _ = m.Observe([]sample.Session{building(80_000, 64)}, nil, line80, t0)
	if got := m.Pace["billed"]; got != (Mark{Tokens: 80_000, Pct: 64}) {
		t.Fatalf("after a compact %+v", got)
	}
	m, _ = m.Observe([]sample.Session{building(90_000, 5)}, nil, line80, t0)
	if got := m.Pace["billed"]; got != (Mark{Tokens: 90_000, Pct: 5}) {
		t.Fatalf("after a new plan %+v", got)
	}
}

// The stoker compacts a session at the boundary its pace calls for, and
// leaves the one that finishes inside the line, though both are past 50%.
func TestThePlanFollowsThePace(t *testing.T) {
	m := on()
	early := building(300_000, 14)
	early.SessionID, early.Pane = "fast", "%6"
	m, _ = m.Observe([]sample.Session{building(556_000, 14), early}, nil, line80, t0)
	late := building(740_000, 64)
	late.SessionID, late.Pane = "fast", "%6"
	steps := Plan(m, []sample.Session{building(692_000, 64), late}, nil, line80, nil, t0)
	if len(steps) != 1 || steps[0].Session != "fast" {
		t.Fatalf("steps %+v, want only the fast one", steps)
	}
}

// The card says what the stoker made of each session: one it left alone
// says why, one past the line still needs its compact.
func TestTheCardSaysWhatTheStokerDecided(t *testing.T) {
	m := on()
	over := building(810_000, 30)
	over.SessionID, over.Name = "over", "Over"
	m, _ = m.Observe([]sample.Session{building(556_000, 14), over}, nil, line80, t0)
	m.Ended = t0
	m = m.Score([]sample.Session{building(692_000, 64), over}, nil, line80, nil)
	c := m.Card
	if c.Ready != 1 || len(c.Left) != 1 || c.Left[0].Name != "Billed" || c.Left[0].Why != "69% · finishes ~79% · left alone" {
		t.Fatalf("left %+v ready %d", c.Left, c.Ready)
	}
	if len(c.NotReady) != 1 || c.NotReady[0].Why != "needs a compact · past 80% · 810k" {
		t.Fatalf("not ready %+v", c.NotReady)
	}
}
