package app

import (
	"strconv"
	"strings"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// chatFacts is the line under the chat's head, w cells wide: the fix for
// tokens with no window known first, then a chain mate's inbox, an account
// that is not the default, the pull request when no ship names it, and the
// background work. It is cut, never wrapped. false: nothing to say.
func (m Model) chatFacts(s sample.Session, w int) (string, bool) {
	var parts []string
	if fix := windowFix(s); fix != "" {
		parts = append(parts, fix)
	}
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
	if len(parts) == 0 {
		return "", false
	}
	return fit(" "+m.st.muted.Render(strings.Join(parts, " · ")), w), true
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
