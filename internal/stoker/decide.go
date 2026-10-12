package stoker

import (
	"strconv"

	"github.com/blox-eng/matchblox/internal/sample"
)

// Mark is where a session's context and plan stood when the run began
// following its pace.
type Mark struct {
	Tokens int `json:"tokens"`
	Pct    int `json:"pct"`
}

const (
	// MinPace is how far a plan's bar moves before its pace is trusted.
	MinPace = 5
	// Lookahead is the stretch of the bar a step is taken to cover: the
	// stoker compacts at the last boundary before the line.
	Lookahead = 10
)

// Verdict is what the stoker makes of a session, and why.
type Verdict struct {
	Compact bool
	Why     string
}

// Decide weighs a session against the line (compact_at, compact_tokens):
// past it, it compacts; with a pace, it compacts at the last boundary
// before the line, and a session that finishes inside the line is left
// alone. Without a pace only the line counts.
func Decide(s sample.Session, mark *Mark, line sample.Full) Verdict {
	if line.Reached(s) {
		return Verdict{true, "past " + line.String()}
	}
	limit := lineTokens(s, line)
	if limit == 0 {
		return Verdict{}
	}
	at := pct(limit, s.Window)
	p := s.Progress
	if p == nil || mark == nil || p.Pct-mark.Pct < MinPace || s.Tokens <= mark.Tokens {
		return Verdict{}
	}
	if p.Pct >= 100 {
		return Verdict{false, "done"}
	}
	per := float64(s.Tokens-mark.Tokens) / float64(p.Pct-mark.Pct)
	finish := s.Tokens + int(per*float64(100-p.Pct))
	switch {
	case finish < limit:
		return Verdict{false, "finishes ~" + pct(finish, s.Window)}
	case s.Tokens+int(per*Lookahead) >= limit:
		return Verdict{true, "would pass " + at + " within the next step"}
	}
	return Verdict{false, "compacts near ~" + pct(limit-int(per*Lookahead), s.Window)}
}

// lineTokens is the line in tokens: the lower of compact_at of the window
// and compact_tokens.
func lineTokens(s sample.Session, line sample.Full) int {
	limit := 0
	if line.Pct > 0 && s.Window > 0 {
		limit = int(line.Pct / 100 * float64(s.Window))
	}
	if line.Tokens > 0 && (limit == 0 || line.Tokens < limit) {
		limit = line.Tokens
	}
	return limit
}

func pct(tokens, window int) string {
	if window <= 0 {
		return sample.Kilo(tokens)
	}
	return strconv.Itoa((tokens*100+window/2)/window) + "%"
}

// pace follows the session's plan: the first sample with a bar marks it,
// and a compact (the context shrank) or a new plan (the bar went back)
// marks it again.
func (m *Mode) pace(s sample.Session) bool {
	if s.Progress == nil || s.Tokens == 0 {
		return false
	}
	now := Mark{Tokens: s.Tokens, Pct: s.Progress.Pct}
	was, ok := m.Pace[s.SessionID]
	if ok && now.Tokens >= was.Tokens && now.Pct >= was.Pct {
		return false
	}
	if m.Pace == nil {
		m.Pace = map[string]Mark{}
	}
	m.Pace[s.SessionID] = now
	return true
}
