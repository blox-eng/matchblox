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
	// RuleOrder marks what the stoker agent sent through the CLI.
	RuleOrder = "order"
	// RuleGo marks a step matchblox put on the card itself: a guarded
	// command it denied in a pane the stoker drives, which waits for the
	// builder's y.
	RuleGo = "go"
	// WhyOrder marks a compact the stoker agent asked for, not the plan.
	WhyOrder = "order"
	// MaxCompacts bounds the compacts the plan sends in one run.
	MaxCompacts = 30
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

// Ordered tells if the stoker agent sent this step through the CLI, a
// compact it asked for included.
func (e Entry) Ordered() bool {
	return !e.Logged() && (e.Rule == RuleOrder || e.Why == WhyOrder)
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
	// Pace marks where each session's context and plan stood when the run
	// began following it, by session id.
	Pace map[string]Mark `json:"pace,omitempty"`
	// Lapses are the stretches the plan's limit stopped sessions.
	Lapses []Lapse `json:"lapses,omitempty"`
	// Card is the run's score, kept from its end until the next run.
	Card *Card `json:"card,omitempty"`
	// Asked: the builder was asked to take over in this run.
	Asked bool `json:"asked,omitempty"`
	// Rests: the builder took over here, and the stoker sends nothing
	// until they leave. Touched is their last key in a console.
	Rests   time.Time `json:"rests,omitzero"`
	Touched time.Time `json:"touched,omitzero"`
	// Order is the builder's order and the stoker agent's plan for it.
	Order *Order `json:"order,omitempty"`
	// Triggers are the stoker's `when`s that have not fired yet; they
	// end with the run.
	Triggers []Trigger `json:"triggers,omitempty"`
	// NextTrigger numbers the run's triggers.
	NextTrigger int `json:"next_trigger,omitempty"`
}

const (
	// RestIdle is how long the builder stays without a key before a stoker
	// that rests goes on.
	RestIdle = 15 * time.Minute
	// LeaveGrace is how long no console is open before a stoker that rests
	// goes on: a dropped link or a restart of the service is not leaving.
	LeaveGrace = 2 * time.Minute
)

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

// Awake ends a rest when the builder left: no key for RestIdle, or no
// console open since alone (zero: one is open) for LeaveGrace.
func (m Mode) Awake(now, alone time.Time) (Mode, bool) {
	left := !alone.IsZero() && now.Sub(alone) >= LeaveGrace
	if m.Rests.IsZero() || !left && now.Sub(m.Touched) < RestIdle {
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
	m.On, m.Until, m.Ended, m.Triggers = false, time.Time{}, now, nil
	if m.Order != nil {
		o := *m.Order
		o.State = OrderDone
		m.Order = &o
	}
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
	Why     string     `json:"why,omitempty"` // the verdict that called for the step
	Argv    [][]string `json:"argv,omitempty"`
	Guard   string     `json:"guard,omitempty"` // what the host said right before the send
	Result  string     `json:"result,omitempty"`
	Before  int        `json:"before,omitempty"` // context tokens
	After   int        `json:"after,omitempty"`
	Resume  string     `json:"resume,omitempty"`
	// Back: the compact's result was read back from the transcript, with or
	// without counts and a RESUME line. It is not read again.
	Back time.Time `json:"back,omitzero"`
	// Step, State, Needs and Text are a step the stoker agent logged
	// (`matchblox stoker step`): its number in the plan (0: a standing
	// rule, named by Rule), doing|done|failed|waits, the go it waits for
	// (merge:#4556) and what it says.
	Step  int    `json:"step,omitempty"`
	State string `json:"state,omitempty"`
	Needs string `json:"needs,omitempty"`
	Text  string `json:"text,omitempty"`
	// Want is the command a go step (RuleGo) waits to run, as the hook
	// read it with its values scrubbed.
	Want string `json:"want,omitempty"`
}

// Step is one planned compact.
type Step struct {
	Session, Name, Pane, Transcript string
	Tokens                          int
	Why                             string // the verdict: "past 80%", "would pass 80% within the next step"
	Action                          advice.Action
}

// Plan picks the sessions to compact now, unless the stoker rests: Claude Code, idle and not cold,
// due by Decide, not stopped by the plan's limit, not waiting for the builder, not stepped on in
// the last PerSession, while the run has steps left. The fullest go first.
// waiting holds the panes the queue lists; held, the session ids parked or held.
func Plan(m Mode, sessions []sample.Session, waiting map[string]bool, full sample.Full, recent []Entry, now time.Time, held map[string]bool) []Step {
	if !m.On || !m.Rests.IsZero() {
		return nil
	}
	left := MaxCompacts
	last := map[string]time.Time{}
	for _, e := range recent {
		if e.Rule == RuleOrder || e.Logged() {
			continue // a message the stoker agent sent, or a step it logged, is not a compact
		}
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
			!s.Resting(), s.Idle >= sample.ColdAfter, !s.Out.IsZero(),
			waiting[s.Pane], held[s.SessionID], s.Asks, s.Permits,
			stepped && now.Sub(at) < PerSession:
			continue
		}
		var mark *Mark
		if mk, ok := m.Pace[s.SessionID]; ok {
			mark = &mk
		}
		v := Decide(s, mark, full)
		// Below compact_at, only the end of a step is a good moment: a turn
		// that ended mid-step (waiting for a subagent) is not.
		if !v.Compact || !v.Past && mark != nil && !mark.Moved {
			continue
		}
		// At a rest, not idle: its subagents may still work.
		out = append(out, Step{
			Session: s.SessionID, Name: s.Name, Pane: s.Pane, Transcript: s.Transcript, Tokens: s.Tokens, Why: v.Why,
			Action: CompactAction(s.Pane),
		})
	}
	return out
}

// CompactAction types the compact and its Enter into a pane. Both steps
// are guarded: the session is at rest, and the pane, read again, shows no
// menu, no question and no draft of the builder's (before the Enter, the
// compact itself).
func CompactAction(pane string) advice.Action {
	g := advice.Guard{RestPane: pane, StokePane: pane}
	typed := g
	typed.Typed = true
	return advice.Action{
		Label: RuleCompact, Steps: panes.Send(pane, "/compact "+Instruction),
		Destructive: true, Guards: []advice.Guard{g, typed},
	}
}
