// Package stoker keeps the fires lit while the builder is away: rule steps
// that need no person and no model of their own. The only rule is compact.
// A step is decided from the sample, re-checked on the host right before it
// is sent, never answers a question and leaves a line in stoker.jsonl.
package stoker

import (
	"fmt"
	"sort"
	"time"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/panes"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

// Instruction asks the compact for what the builder types after one by hand.
const Instruction = "Keep the task, the branch and last commit, the files you changed, what to read first, every open decision and the next step. End with one line that starts with RESUME: and says how to continue."

const (
	// RuleCompact is the one rule.
	RuleCompact = "compact"
	// PerSession is the least time between two steps on one session.
	PerSession = 30 * time.Minute
	// MaxSteps bounds one run.
	MaxSteps = 30
	// ResumeWait is how long the service reads a transcript back for the
	// result of a compact.
	ResumeWait = 30 * time.Minute
)

// Results of a step.
const (
	Sent    = "sent"
	Skipped = "skipped" // + ": " and what the host said
)

// Waits tells if a queue item waits for the builder: a permission prompt
// or a question. Claude Code's idle reminder after a finished turn asks
// nothing.
func Waits(it queue.Item) bool {
	return it.State == queue.StatePermission || it.State == queue.StateQuestion && !it.Reminder
}

// Mode is what stoker.json keeps.
type Mode struct {
	On    bool      `json:"on"`
	Since time.Time `json:"since,omitzero"` // the run started; kept after it ends
	Until time.Time `json:"until,omitzero"` // a night ends itself here; zero: when turned off
	Ended time.Time `json:"ended,omitzero"`
	Acked time.Time `json:"acked,omitzero"` // the builder saw the steps up to here
	// Nightly: the run is a night; kept after it ends, for its card.
	Nightly bool `json:"night,omitempty"`

	// Start are the sessions at work when the run began; the score counts
	// how many of them are ready to go on at its end.
	Start []Seat `json:"start,omitempty"`
	// Watched, Full and Waited are the sessions seen during the run, those
	// that reached compact_at, and those that waited for the builder.
	Watched []string `json:"watched,omitempty"`
	Full    []string `json:"full,omitempty"`
	Waited  []string `json:"waited,omitempty"`
	// Card is the run's score, kept from its end until the next run.
	Card *Card `json:"card,omitempty"`
	// Asked: the builder was asked to take over in this run.
	Asked bool `json:"asked,omitempty"`
	// Rests: the builder took over here, and the stoker sends nothing
	// until they leave. Touched is their last key in a console.
	Rests   time.Time `json:"rests,omitzero"`
	Touched time.Time `json:"touched,omitzero"`
}

// RestIdle is how long the consoles stay without a key before a stoker
// that rests goes on.
const RestIdle = 15 * time.Minute

// Rest hands the sessions to the builder.
func (m Mode) Rest(now time.Time) Mode {
	m.Asked, m.Rests, m.Touched = true, now, now
	return m
}

func (m Mode) Wake() Mode {
	m.Rests, m.Touched = time.Time{}, time.Time{}
	return m
}

// Touch is a key of the builder's while the stoker rests.
func (m Mode) Touch(now time.Time) Mode {
	if !m.Rests.IsZero() && now.After(m.Touched) {
		m.Touched = now
	}
	return m
}

// Awake ends a rest when the builder left: no console is open, or none has
// had a key for RestIdle.
func (m Mode) Awake(now time.Time, consoles int) (Mode, bool) {
	if m.Rests.IsZero() || consoles > 0 && now.Sub(m.Touched) < RestIdle {
		return m, false
	}
	return m.Wake(), true
}

// TurnOn starts a run, or keeps the one that runs.
func (m Mode) TurnOn(now time.Time) Mode {
	if m.On {
		return m
	}
	return Mode{On: true, Since: now, Acked: m.Acked}
}

// Night runs the stoker until the next ends ("07:00", local time).
func (m Mode) Night(now time.Time, ends string) (Mode, error) {
	at, err := time.ParseInLocation("15:04", ends, now.Location())
	if err != nil {
		return m, fmt.Errorf("night_ends %q is not a time like 07:00", ends)
	}
	until := time.Date(now.Year(), now.Month(), now.Day(), at.Hour(), at.Minute(), 0, 0, now.Location())
	if !until.After(now) {
		until = until.AddDate(0, 0, 1)
	}
	m = m.TurnOn(now)
	m.Until, m.Nightly = until, true
	return m, nil
}

func (m Mode) Off(now time.Time) Mode {
	m.On, m.Until, m.Ended = false, time.Time{}, now
	return m.Wake()
}

// Ack folds what the builder saw: the steps up to now, and the card.
func (m Mode) Ack(now time.Time) Mode {
	m.Acked = now
	if m.Card != nil && m.Acked.Before(m.Ended) {
		m.Acked = m.Ended
	}
	return m
}

// Expire turns a night off at its end.
func (m Mode) Expire(now time.Time) (Mode, bool) {
	if !m.On || m.Until.IsZero() || now.Before(m.Until) {
		return m, false
	}
	return m.Off(m.Until), true
}

// Entry is one line of stoker.jsonl. A later line with the same ID adds
// what was read back: the context after the compact and the RESUME line.
type Entry struct {
	ID      string     `json:"id"`
	At      time.Time  `json:"at,omitzero"`
	Session string     `json:"session,omitempty"`
	Name    string     `json:"name,omitempty"`
	Pane    string     `json:"pane,omitempty"`
	Rule    string     `json:"rule,omitempty"`
	Argv    [][]string `json:"argv,omitempty"`
	Guard   string     `json:"guard,omitempty"` // what the host said right before the send
	Result  string     `json:"result,omitempty"`
	Before  int        `json:"before,omitempty"` // context tokens
	After   int        `json:"after,omitempty"`
	Resume  string     `json:"resume,omitempty"`
	// Back: the compact's result was read back from the transcript, with or
	// without counts and a RESUME line. It is not read again.
	Back time.Time `json:"back,omitzero"`
}

// Step is one planned compact.
type Step struct {
	Session, Name, Pane, Transcript string
	Tokens                          int
	Action                          advice.Action
}

// Plan picks the sessions to compact now, unless the stoker rests: Claude Code, idle and not cold,
// at or above compactAt, not waiting for the builder, not stepped on in
// the last PerSession, while the run has steps left. The fullest go first.
// waiting holds the panes the queue lists.
func Plan(m Mode, sessions []sample.Session, waiting map[string]bool, compactAt float64, recent []Entry, now time.Time) []Step {
	if !m.On || !m.Rests.IsZero() {
		return nil
	}
	left := MaxSteps
	last := map[string]time.Time{}
	for _, e := range recent {
		if !e.At.Before(m.Since) {
			left--
		}
		if e.At.After(last[e.Session]) {
			last[e.Session] = e.At
		}
	}
	ss := append([]sample.Session(nil), sessions...)
	sort.SliceStable(ss, func(i, j int) bool { return ss[i].ContextPct > ss[j].ContextPct })
	var out []Step
	for _, s := range ss {
		if len(out) >= left {
			break
		}
		at, stepped := last[s.SessionID]
		switch {
		case s.Agent != "claude", s.Pane == "", s.SessionID == "",
			s.Busy, s.Status != "idle", s.Age() != "idle",
			s.Tokens == 0, s.ContextPct < compactAt,
			waiting[s.Pane], s.Asks, s.Permits,
			stepped && now.Sub(at) < PerSession:
			continue
		}
		g := advice.Guard{IdlePane: s.Pane, StokePane: s.Pane}
		typed := g
		typed.Typed = true
		out = append(out, Step{
			Session: s.SessionID, Name: s.Name, Pane: s.Pane, Transcript: s.Transcript, Tokens: s.Tokens,
			Action: advice.Action{
				Label: RuleCompact, Steps: panes.Send(s.Pane, "/compact "+Instruction),
				Destructive: true, Guards: []advice.Guard{g, typed},
			},
		})
	}
	return out
}
