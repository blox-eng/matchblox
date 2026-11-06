package app

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// A fuse joins the matches of a chain in Sessions, in a column between the
// match and the name. By need and by repo a chain stays together and its
// fuse is whole: ╮ first, ┤ between, ╯ last. Status and none split it, and
// each of its matches keeps a stub ─╴ that lights with the selected one.

// sparkFor is how long the spark takes to run down a fuse.
const sparkFor = 600 * time.Millisecond

const sameRepoHint = " · same repo · chain?"

// chainKey is a chain on a host: chains live on one host.
type chainKey struct{ host, id string }

// memberOf is a session as the chains name it.
func memberOf(s sample.Session) chains.Member {
	return chains.Member{SessionID: s.SessionID, Pane: s.Pane, PID: s.PID, Start: s.Start}
}

// chainsOf are the chains host sent last.
func (m Model) chainsOf(host string) *state.ChainsView {
	if host == m.opt.Host {
		return m.chains
	}
	return m.stateOf(host).Chains
}

// chainOf is the chain a session is in. A chain of one is no chain.
func (m Model) chainOf(s sample.Session) (chains.Chain, bool) {
	cv := m.chainsOf(s.Host)
	if cv == nil {
		return chains.Chain{}, false
	}
	me := memberOf(s)
	for _, c := range cv.Chains {
		if len(c.Members) > 1 && slices.ContainsFunc(c.Members, me.Same) {
			return c, true
		}
	}
	return chains.Chain{}, false
}

func (m Model) chainKeyOf(s sample.Session) (chainKey, bool) {
	c, ok := m.chainOf(s)
	return chainKey{s.Host, c.ID}, ok
}

// needGroup is the need group a session is listed in: its own, or the
// neediest of its chain's matches in the list.
func (m Model) needGroup(s sample.Session) int {
	g := m.groupOf(s)
	k, ok := m.chainKeyOf(s)
	if !ok {
		return g
	}
	for _, o := range m.snap.Sessions {
		if ko, ok := m.chainKeyOf(o); ok && ko == k {
			g = min(g, m.groupOf(o))
		}
	}
	return g
}

// together moves each chain's matches in a group next to the first of
// them, in need order, so its fuse is whole.
func (m Model) together(ss []sample.Session) []sample.Session {
	type block struct {
		k     chainKey
		label string
	}
	out := make([]sample.Session, 0, len(ss))
	done := map[block]bool{}
	for _, s := range ss {
		k, ok := m.chainKeyOf(s)
		if !ok {
			out = append(out, s)
			continue
		}
		b := block{k, m.groupLabel(s)}
		if done[b] {
			continue
		}
		done[b] = true
		var mates []sample.Session
		for _, o := range ss {
			if ko, ok := m.chainKeyOf(o); ok && ko == k && m.groupLabel(o) == b.label {
				mates = append(mates, o)
			}
		}
		slices.SortStableFunc(mates, func(a, b sample.Session) int { return m.groupOf(a) - m.groupOf(b) })
		out = append(out, mates...)
	}
	return out
}

// chainsIn is what a need group's label adds for the chains it holds: how
// many matches each has, and how many of them a filter hides.
func (m Model) chainsIn(label string, ss []sample.Session) string {
	if m.sessGroup != groupNeed {
		return ""
	}
	out := ""
	seen := map[chainKey]bool{}
	for _, s := range ss {
		k, ok := m.chainKeyOf(s)
		if !ok || seen[k] || m.groupLabel(s) != label {
			continue
		}
		seen[k] = true
		shown, all := 0, 0
		for _, o := range ss {
			if ko, ok := m.chainKeyOf(o); ok && ko == k {
				shown++
			}
		}
		for _, o := range m.all.sessions {
			if ko, ok := m.chainKeyOf(o); ok && ko == k && !m.isStoker(o) {
				all++
			}
		}
		all = max(all, shown)
		out += fmt.Sprintf(" · chain of %d", all)
		if all > shown {
			out += fmt.Sprintf(" · %d hidden", all-shown)
		}
	}
	return out
}

// fuseWidth is the fuse column of a list w wide: none while no session in
// it is chained, two cells on a wide list, one below.
func (m Model) fuseWidth(w int) int {
	if !slices.ContainsFunc(m.snap.Sessions, func(s sample.Session) bool { _, ok := m.chainOf(s); return ok }) {
		return 0
	}
	if w >= wideList {
		return 2
	}
	return 1
}

// fuseRuns tells if the fuse of row i runs up to the row above and down to
// the row below: by need and by repo, between chain-mates in one group.
func (m Model) fuseRuns(i int, ss []sample.Session) (up, down bool) {
	if m.sessGroup != groupNeed && m.sessGroup != groupRepo {
		return false, false
	}
	k, ok := m.chainKeyOf(ss[i])
	if !ok {
		return false, false
	}
	joins := func(j int) bool {
		if j < 0 || j >= len(ss) {
			return false
		}
		o, ok := m.chainKeyOf(ss[j])
		return ok && o == k && m.groupLabel(ss[j]) == m.groupLabel(ss[i])
	}
	return joins(i - 1), joins(i + 1)
}

// fuseCell is the fuse on the line of row i of ss, sel the selected row,
// in a list w wide that has a fuse column: "" on a row in no chain.
func (m Model) fuseCell(i int, ss []sample.Session, sel, w int) string {
	k, ok := m.chainKeyOf(ss[i])
	if !ok {
		return ""
	}
	lead, glyph, style := "─", "╴", m.st.faint
	switch up, down := m.fuseRuns(i, ss); {
	case up && down:
		glyph, style = "┤", m.st.accent
	case down:
		glyph, style = "╮", m.st.accent
	case up:
		glyph, style = "╯", m.st.accent
	default: // a stub: lit while a chain-mate is selected
		if sel >= 0 && sel < len(ss) {
			if ks, ok := m.chainKeyOf(ss[sel]); ok && ks == k {
				style = m.st.accent
			}
		}
		if i == sel {
			lead, glyph = "━", "╸"
		}
	}
	cell := glyph
	if w >= wideList {
		cell = lead + glyph
	}
	if m.sparkOn(i, k, ss) {
		style = m.sparkStyle()
	}
	return style.Render(cell)
}

// fuseThrough is the fuse on a line between rows, fw cells: │ where it
// runs on.
func (m Model) fuseThrough(runs bool, fw int) string {
	if !runs {
		return strings.Repeat(" ", fw)
	}
	return padLeft(m.st.accent.Render("│"), fw)
}

// fused puts the fuse into the lines under a row, after their first two
// cells, which are blank.
func (m Model) fused(lines []string, runs bool, fw int) []string {
	if fw == 0 {
		return lines
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = "  " + m.fuseThrough(runs, fw) + ansi.TruncateLeft(l, 2, "")
	}
	return out
}

// hairline sets the phone's opened block apart: after the fuse when there
// is one.
func (m Model) hairline(w, fw int, runs bool) string {
	if fw == 0 {
		return m.st.hair.Render(strings.Repeat("─", w))
	}
	return "  " + m.fuseThrough(runs, fw) + m.st.hair.Render(strings.Repeat("─", max(w-2-fw, 0)))
}

// sparkKey is where a chain's spark is kept among the changes: apart from
// every pane, whose ids have no colon.
func sparkKey(k chainKey) paneKey { return paneKey{k.host, "chain:" + k.id} }

// noteSparks plays the spark of each chain whose spark moved since the
// console last saw it.
func (m *Model) noteSparks(host string, next *state.ChainsView, now time.Time) {
	if next == nil {
		return
	}
	was := map[string]time.Time{}
	if cv := m.chainsOf(host); cv != nil {
		for _, c := range cv.Chains {
			was[c.ID] = c.Spark
		}
	}
	for _, c := range next.Chains {
		if !c.Spark.IsZero() && !c.Spark.Equal(was[c.ID]) {
			m.changes[sparkKey(chainKey{host, c.ID})] = change{changeSpark, now}
		}
	}
}

// sparkOn tells if the spark of chain k is on row i now: it runs down the
// chain's rows once, in sparkFor.
func (m Model) sparkOn(i int, k chainKey, ss []sample.Session) bool {
	c, ok := m.changes[sparkKey(k)]
	if m.opt.NoMotion || !ok || c.kind != changeSpark {
		return false
	}
	e := m.now().Sub(c.at)
	if e < 0 || e >= sparkFor {
		return false
	}
	var rows []int
	for j, s := range ss {
		if o, ok := m.chainKeyOf(s); ok && o == k {
			rows = append(rows, j)
		}
	}
	return rows[int(e*time.Duration(len(rows))/sparkFor)] == i
}

func (m Model) sparkStyle() lipgloss.Style { return lipgloss.NewStyle().Foreground(m.marks().Core) }

// mayChain tells if a row offers a chain: by need or in one list, a
// session in no chain whose repo another session on its host works in.
func (m Model) mayChain(s sample.Session) bool {
	if m.sessGroup != groupNeed && m.sessGroup != groupNone || !m.repoMates[keyOf(s)] {
		return false
	}
	_, chained := m.chainOf(s)
	return !chained
}

// sharingRepos are the sessions of ss that share their repo with another
// on their host.
func (m Model) sharingRepos(ss []sample.Session) map[rowKey]bool {
	type in struct{ host, repo string }
	n, of := map[in]int{}, map[rowKey]in{}
	for _, s := range ss {
		if r, _, ok := proto.RepoOf(m.gitOf(s.Host), s.Cwd); ok {
			n[in{s.Host, r}]++
			of[keyOf(s)] = in{s.Host, r}
		}
	}
	out := map[rowKey]bool{}
	for k, r := range of {
		out[k] = n[r] > 1
	}
	return out
}
