// Package limits reads the plan limits of each account an agent runs
// under, counts them as matches (the 5-hour window) and sparks (the week),
// and forecasts whether the week lasts.
package limits

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	BoxMinutes  = 5 * 60      // the 5-hour window: 5 matches, 1 = 20%
	WeekMinutes = 7 * 24 * 60 // the week: 10 sparks, 1 = 10%

	// StaleAfter: a reading older than this, with no reset since, is shown
	// with its time.
	StaleAfter = time.Hour
)

// The states of an account's limits.
const (
	Measured   = "measured"
	Stale      = "stale"
	Unmeasured = "not measured"
)

// The sources an account's limits come from.
const (
	SourceClaude   = "claude-statusline"
	SourceCodex    = "codex"
	SourceOpenCode = "opencode"
)

// Window is one limit as the agent reported it.
type Window struct {
	UsedPct  float64   `json:"used_pct"`
	Minutes  int       `json:"window_minutes"`
	ResetsAt time.Time `json:"resets_at"`
}

// Reading is what one source reported at one time.
type Reading struct {
	At      time.Time `json:"at"`
	Windows []Window  `json:"windows,omitempty"`
}

// Other is a window that is neither 5 hours nor a week (a plan's month).
type Other struct {
	Window
	Text string `json:"text"` // "53% left this month"
}

// Outlook is the forecast of the week.
type Outlook struct {
	Lasts bool      `json:"lasts"`
	Out   time.Time `json:"out,omitempty"` // when the week runs out, to the hour
}

// Account is the limits of one account, as the console shows them.
type Account struct {
	Provider string    `json:"provider"`
	Account  string    `json:"account"`
	Source   string    `json:"source"`
	State    string    `json:"state"`
	Fix      string    `json:"fix,omitempty"`
	At       time.Time `json:"at,omitzero"`
	Windows  []Window  `json:"windows,omitempty"`

	Matches      *int      `json:"matches,omitempty"`
	MatchesReset time.Time `json:"matches_reset,omitzero"`
	Sparks       *int      `json:"sparks,omitempty"`
	SparksReset  time.Time `json:"sparks_reset,omitzero"`
	Other        []Other   `json:"other,omitempty"`
	Forecast     *Outlook  `json:"forecast,omitempty"`
}

// Build turns a reading into what the console shows. A nil reading, or one
// without windows, is not measured and says how to make it measured.
func Build(provider, account, source string, r *Reading, now time.Time, q Quiet) Account {
	a := Account{Provider: provider, Account: account, Source: source}
	if r == nil || len(r.Windows) == 0 {
		a.State, a.Fix = Unmeasured, fixFor(source, account, r != nil)
		return a
	}
	a.State, a.At, a.Windows = Measured, r.At, r.Windows
	if now.Sub(r.At) > StaleAfter {
		a.State = Stale
	}
	for _, w := range r.Windows {
		if !w.ResetsAt.IsZero() && !w.ResetsAt.After(now) {
			w = Window{Minutes: w.Minutes} // reset since the reading: full again
		}
		left := math.Max(0, 100-w.UsedPct)
		switch w.Minutes {
		case BoxMinutes:
			n := count(left, 20)
			a.Matches, a.MatchesReset = &n, w.ResetsAt
		case WeekMinutes:
			n := count(left, 10)
			a.Sparks, a.SparksReset = &n, w.ResetsAt
			a.Forecast = Forecast(w, now, q)
		default:
			a.Other = append(a.Other, Other{w, fmt.Sprintf("%.0f%% left %s", left, span(w.Minutes))})
		}
	}
	return a
}

// count is how many units are left: a part of one still counts.
func count(leftPct, unit float64) int {
	return int(math.Ceil(leftPct/unit - 1e-9))
}

func span(minutes int) string {
	switch d := time.Duration(minutes) * time.Minute; {
	case d >= 28*24*time.Hour:
		return "this month"
	case d >= 24*time.Hour:
		return fmt.Sprintf("in %d days", int(d.Hours()/24))
	default:
		return fmt.Sprintf("in %d hours", int(d.Hours()))
	}
}

func fixFor(source, account string, read bool) string {
	switch {
	case account == "API key":
		return "an API key has no plan limits"
	case source == SourceClaude:
		return "open the door “Show your limits”"
	case source == SourceCodex && read:
		return "this Codex login reports no limits"
	case source == SourceCodex:
		return "Codex reports them after its next turn"
	default:
		return "OpenCode keeps no limits"
	}
}

// quietWeight is how much an hour the builder does not usually work counts
// against an hour they do.
const quietWeight = 0.1

// Quiet is when the builder does not usually work: hours of each day and
// whole days.
type Quiet struct {
	From, To int // minutes of the day; From == To: no quiet hours
	Days     [7]bool
}

var weekdays = map[string]time.Weekday{"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday,
	"wed": time.Wednesday, "thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday}

// ParseQuiet reads "23:00-08:00" and day names ("Sat", "sun"). Empty hours:
// none.
func ParseQuiet(hours string, days []string) (Quiet, error) {
	var q Quiet
	if hours != "" {
		from, to, ok := strings.Cut(hours, "-")
		var err error
		if !ok {
			return q, fmt.Errorf("quiet hours %q: want HH:MM-HH:MM", hours)
		}
		if q.From, err = clock(from); err != nil {
			return q, err
		}
		if q.To, err = clock(to); err != nil {
			return q, err
		}
	}
	for _, d := range days {
		wd, ok := weekdays[strings.ToLower(strings.TrimSpace(d))]
		if !ok {
			return q, fmt.Errorf("quiet day %q: want Mon, Tue, … Sun", d)
		}
		q.Days[wd] = true
	}
	return q, nil
}

func clock(s string) (int, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("quiet hours: %q is not HH:MM", s)
	}
	return t.Hour()*60 + t.Minute(), nil
}

// Quiet says whether t falls in a quiet hour or on a quiet day, in t's zone.
func (q Quiet) Quiet(t time.Time) bool {
	if q.Days[t.Weekday()] {
		return true
	}
	m := t.Hour()*60 + t.Minute()
	switch {
	case q.From == q.To:
		return false
	case q.From < q.To:
		return m >= q.From && m < q.To
	default: // over midnight
		return m >= q.From || m < q.To
	}
}

const step = 15 * time.Minute

// weighted counts the hours from a to b, a quiet hour as quietWeight.
func (q Quiet) weighted(a, b time.Time) float64 {
	var h float64
	for t := a; t.Before(b); {
		next := t.Truncate(step).Add(step)
		if next.After(b) {
			next = b
		}
		w := 1.0
		if q.Quiet(t) {
			w = quietWeight
		}
		h += w * next.Sub(t).Hours()
		t = next
	}
	return h
}

// Forecast says whether the week lasts at the rate of its hours so far,
// with quiet hours discounted. Nil in the first hour of a week, or for a
// window without a reset.
func Forecast(w Window, now time.Time, q Quiet) *Outlook {
	if w.ResetsAt.IsZero() || !w.ResetsAt.After(now) {
		return nil
	}
	if w.UsedPct >= 100 {
		return &Outlook{Out: now}
	}
	start := w.ResetsAt.Add(-time.Duration(w.Minutes) * time.Minute)
	if now.Sub(start) < time.Hour {
		return nil
	}
	rate := w.UsedPct / q.weighted(start, now)
	used := w.UsedPct
	for t := now; t.Before(w.ResetsAt); t = t.Add(step) {
		end := t.Add(step)
		if end.After(w.ResetsAt) {
			end = w.ResetsAt
		}
		used += rate * q.weighted(t, end)
		if used >= 100 {
			return &Outlook{Out: end.Round(time.Hour)}
		}
	}
	return &Outlook{Lasts: true}
}

// The tones of a part of the line.
const (
	Muted = "muted"
	Warn  = "warn"
)

// Part is a piece of an account's line and how loud it is.
type Part struct {
	Text string
	Tone string // "", Muted or Warn
}

// Parts is an account's line: the matches, the sparks, the forecast, and
// the age of an old reading, or what to do when there is none.
func Parts(a Account, now time.Time) []Part {
	if a.State == Unmeasured {
		return []Part{{Text: "not measured · " + a.Fix, Tone: Warn}}
	}
	var ps []Part
	if a.Matches != nil {
		ps = append(ps, Part{Text: units(*a.Matches, "match", "matches", a.MatchesReset, 1, now)})
	}
	if a.Sparks != nil {
		ps = append(ps, Part{Text: units(*a.Sparks, "spark", "sparks", a.SparksReset, 2, now)})
	}
	for _, o := range a.Other {
		ps = append(ps, Part{Text: o.Text})
	}
	switch f := a.Forecast; {
	case f == nil:
	case f.Lasts:
		ps = append(ps, Part{Text: "lasts the week", Tone: Muted})
	default:
		ps = append(ps, Part{Text: "out " + When(f.Out, now, "~"), Tone: Warn})
	}
	if a.State == Stale {
		ps = append(ps, Part{Text: "as of " + When(a.At, now, ""), Tone: Warn})
	}
	return ps
}

// units is "4 matches", and "1 match until 16:40" when few are left.
func units(n int, one, many string, reset time.Time, near int, now time.Time) string {
	s := fmt.Sprintf("%d %s", n, many)
	if n == 1 {
		s = "1 " + one
	}
	if n <= near && !reset.IsZero() {
		s += " until " + When(reset, now, "")
	}
	return s
}

// When is "16:40" today, else "Mon 09:00", in now's zone.
func When(t, now time.Time, approx string) string {
	t = t.In(now.Location())
	if y, m, d := t.Date(); y == now.Year() && m == now.Month() && d == now.Day() {
		return approx + t.Format("15:04")
	}
	return t.Format("Mon") + " " + approx + t.Format("15:04")
}

// Plain joins the parts as text.
func Plain(ps []Part) string {
	s := make([]string, len(ps))
	for i, p := range ps {
		s[i] = p.Text
	}
	return strings.Join(s, " · ")
}
