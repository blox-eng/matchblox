package stoker

import (
	"strconv"

	"github.com/blox-eng/matchblox/internal/sample"
)

// Mark is where a session's context and plan stood when the run began
// following its pace, and whether its bar moved over its last turn.
type Mark struct {
	Tokens int `json:"tokens"`
	Pct    int `json:"pct"`
	// Last is the bar when the session was last idle (Seen); Busy: a turn
	// ran since. Moved: the turn that ended last moved the bar, so the
	// session sits at the end of a step.
	Last  int  `json:"last,omitempty"`
	Seen  bool `json:"seen,omitempty"`
	Busy  bool `json:"busy,omitempty"`
	Moved bool `json:"moved,omitempty"`
}

const (
	// MinPace is how far a plan's bar moves before its pace is trusted.
	MinPace = 5
	// Lookahead is the stretch of the bar a step is taken to cover: the
	// stoker compacts at the last boundary before the line.
	Lookahead = 10
)

// Verdict is what the stoker makes of a session, and why. Past: the session
// is at compact_at, where Claude Code would compact it inside a turn.
type Verdict struct {
	Compact bool
	Past    bool
	Why     string
}

// Decide weighs a session against the line. compact_at of the window is the
// hard line: past it, it compacts. Below it, a session with a pace is
// compacted at the last boundary before the line and left alone when it
// finishes inside it. compact_tokens is a line only for a session without a
// pace: on a large window a pace says more than a fixed count.
func Decide(s sample.Session, mark *Mark, line sample.Full) Verdict {
	if s.Tokens == 0 {
		return Verdict{}
	}
	if line.Pct > 0 && s.ContextPct >= line.Pct {
		return Verdict{true, true, "past " + sample.Full{Pct: line.Pct}.String()}
	}
	limit := lineTokens(s, line)
	p := s.Progress
	if limit == 0 || p == nil || mark == nil || p.Pct-mark.Pct < MinPace || s.Tokens <= mark.Tokens {
		if line.Reached(s) {
			return Verdict{true, false, "past " + line.String()}
		}
		return Verdict{}
	}
	if p.Pct >= 100 {
		return Verdict{false, false, "done"}
	}
	at := pct(limit, s.Window)
	per := float64(s.Tokens-mark.Tokens) / float64(p.Pct-mark.Pct)
	finish := s.Tokens + int(per*float64(100-p.Pct))
	switch {
	case finish < limit:
		return Verdict{false, false, "finishes ~" + pct(finish, s.Window)}
	case s.Tokens+int(per*Lookahead) >= limit:
		return Verdict{true, false, "would pass " + at + " within the next step"}
	}
	return Verdict{false, false, "compacts near ~" + pct(limit-int(per*Lookahead), s.Window)}
}

// lineTokens is the line a pace is weighed against: compact_at of the
// window, or compact_tokens when the window is unknown.
func lineTokens(s sample.Session, line sample.Full) int {
	if line.Pct > 0 && s.Window > 0 {
		return int(line.Pct / 100 * float64(s.Window))
	}
	return line.Tokens
}

func pct(tokens, window int) string {
	if window <= 0 {
		return sample.Kilo(tokens)
	}
	return strconv.Itoa((tokens*100+window/2)/window) + "%"
}

// pace follows the session's plan: the first sample with a bar marks it,
// and a compact (the context shrank) or a new plan (the bar went back)
// marks it again. Each idle stretch after a turn records whether that turn
// moved the bar.
func (m *Mode) pace(s sample.Session) bool {
	if s.Progress == nil || s.Tokens == 0 {
		return false
	}
	was, ok := m.Pace[s.SessionID]
	now := was
	if !ok || s.Tokens < was.Tokens || s.Progress.Pct < was.Pct {
		now = Mark{Tokens: s.Tokens, Pct: s.Progress.Pct}
	}
	switch {
	case s.Busy || s.Status != "idle":
		now.Busy = true
	case now.Busy || !now.Seen:
		// The first idle stretch the run sees counts as a step's end: the
		// stoker cannot know better.
		now.Moved = !now.Seen || s.Progress.Pct > now.Last
		now.Last, now.Seen, now.Busy = s.Progress.Pct, true, false
	}
	if ok && now == was {
		return false
	}
	if m.Pace == nil {
		m.Pace = map[string]Mark{}
	}
	m.Pace[s.SessionID] = now
	return true
}
