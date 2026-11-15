package app

import (
	"slices"
	"strconv"
	"strings"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/charmbracelet/x/ansi"
)

// chatFacts are the lines under the chat's head, each w cells wide: the
// fix for tokens with no window known first, then a chain mate's inbox, an
// account that is not the default, the pull request when no ship names it,
// and the background work. They are one line where they fit. Else the fix
// wraps whole, so it keeps its action, and the other facts follow on its
// last line, or on one line of their own, cut. None: nothing to say.
func (m Model) chatFacts(s sample.Session, w int) []string {
	ink := m.st.muted.Render
	fix, parts := windowFix(s), m.otherFacts(s)
	all := parts
	if fix != "" {
		all = append([]string{fix}, parts...)
	}
	if len(all) == 0 {
		return nil
	}
	line := strings.Join(all, " · ")
	if fix == "" || 1+ansi.StringWidth(line) <= w {
		return []string{fit(" "+ink(line), w)}
	}
	lines := wrapPlain(fix, max(w-1, 1))
	if len(parts) > 0 {
		rest := strings.Join(parts, " · ")
		if tail := lines[len(lines)-1] + " · " + rest; 1+ansi.StringWidth(tail) <= w {
			lines[len(lines)-1] = tail
		} else {
			lines = append(lines, rest) // cut to the width below
		}
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = fit(" "+ink(l), w)
	}
	return out
}

// otherFacts are the facts of s after the window fix, in their order.
func (m Model) otherFacts(s sample.Session) []string {
	var parts []string
	if v, ok := m.mateOf(s); ok && v.Inbox > 0 {
		inbox := "✉ " + strconv.Itoa(v.Inbox) + " in its inbox"
		if v.Full {
			inbox += ", the oldest dropped"
		}
		parts = append(parts, inbox)
	}
	if tag := m.accountTag(s); tag != "" {
		parts = append(parts, strings.TrimSpace(providerMarks[s.Provider]+" "+tag))
	}
	if len(m.shipsOf(s)) == 0 { // the head names the leading ship
		if p := m.prOf(s.Cwd); p != nil {
			pr := "#" + strconv.Itoa(p.Number)
			if p.Draft {
				pr += " draft"
			}
			parts = append(parts, pr)
		}
	}
	if s.Agents > 0 && s.Status != sample.StatusAgents { // the state word says agents already
		parts = append(parts, plural(s.Agents, "subagent")+" running")
	}
	if len(s.Shells) > 0 {
		shells := plural(len(s.Shells), "shell")
		if old := s.Shells[0].Started; !old.IsZero() {
			shells += " · oldest " + sample.Human(m.now().Sub(old))
		}
		parts = append(parts, shells)
	}
	if n := m.descendants(s.Host, sample.ProcRef{PID: s.PID, Start: s.Start}); n > 0 {
		parts = append(parts, plural(n, "child run"))
	}
	return parts
}

// prOf is the open pull request of the worktree a directory is in: the
// worktree with the longest path that holds it.
func (m Model) prOf(cwd string) *proto.PR {
	if m.git == nil || cwd == "" {
		return nil
	}
	var pr *proto.PR
	best := 0
	for _, r := range m.git.Repos {
		for _, wt := range r.Worktrees {
			if (cwd == wt.Path || strings.HasPrefix(cwd, wt.Path+"/")) && len(wt.Path) > best {
				pr, best = wt.PR, len(wt.Path)
			}
		}
	}
	return pr
}

// mateOf is what s's chain says of s as a mate: its inbox.
func (m Model) mateOf(s sample.Session) (state.MateView, bool) {
	cv := m.chainsOf(s.Host)
	if cv == nil {
		return state.MateView{}, false
	}
	mb := memberOf(s)
	i := slices.IndexFunc(cv.Mates, func(v state.MateView) bool { return v.Member.Same(mb) })
	if i < 0 {
		return state.MateView{}, false
	}
	return cv.Mates[i], true
}

// windowFix is what to do about tokens with no window known, "" when the
// window is known or nothing is spent. For Claude Code the status line
// gives the window, so the fix names setup first.
func windowFix(s sample.Session) string {
	switch {
	case s.Tokens == 0 || s.Window > 0:
		return ""
	case s.Agent == "claude":
		return sample.Kilo(s.Tokens) + " tokens · no window known: run matchblox setup, or add the model to [sessions.windows]"
	}
	return sample.Kilo(s.Tokens) + " tokens · no window known: add the model to [sessions.windows]"
}
