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
// match and the name. By need a chain stays together and its fuse is whole:
// ╮ first, ┤ between, ╯ last; by repo too, for its matches in one repo.
// Status, none, and a repo group a chain only partly sits in split it, and
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

// chainIndex is what the chains say of the listed sessions, worked out once
// in refresh: the frames only look it up.
type chainIndex struct {
	of      map[rowKey]chainKey // the chain a session is in; a chain of one is no chain
	need    map[chainKey]int    // the neediest group of the chain's listed matches
	present map[chainKey]int    // the matches the host has, whatever the filter hides
	shown   map[chainKey]int    // the matches in the list
	rows    map[chainKey][]int  // the list rows of each chain, top down
}

// chainScan is the chain, as the chains name it, that a session is in.
func (m Model) chainScan(s sample.Session, views map[string]*state.ChainsView) (chains.Chain, bool) {
	cv, ok := views[s.Host]
	if !ok {
		cv = m.chainsOf(s.Host)
		views[s.Host] = cv
	}
	if cv == nil {
		return chains.Chain{}, false
	}
	me := memberOf(s)
	for _, c := range cv.Chains {
		if slices.ContainsFunc(c.Members, me.Same) {
			return c, true
		}
	}
	return chains.Chain{}, false
}

// indexChains works out which sessions are chained, and how needy their
// chains are, from the sessions the host has and the ones listed. A chain
// with one match present is no chain.
func (m *Model) indexChains() {
	ix := chainIndex{of: map[rowKey]chainKey{}}
	views := map[string]*state.ChainsView{}
	has := func(s sample.Session) (chainKey, bool) {
		c, ok := m.chainScan(s, views)
		return chainKey{s.Host, c.ID}, ok
	}
	present := map[chainKey]int{}
	of := map[rowKey]chainKey{}
	for _, s := range m.all.sessions {
		if k, ok := has(s); ok && !m.isStoker(s) {
			present[k]++
			of[keyOf(s)] = k
		}
	}
	for _, s := range m.snap.Sessions {
		if k, ok := has(s); ok {
			of[keyOf(s)] = k
		}
	}
	for r, k := range of {
		if present[k] > 1 {
			ix.of[r] = k
		}
	}
	ix.present, ix.need, ix.shown = present, map[chainKey]int{}, map[chainKey]int{}
	for _, s := range m.snap.Sessions {
		if k, ok := ix.of[keyOf(s)]; ok {
			g := m.groupOf(s)
			if n, seen := ix.need[k]; !seen || g < n {
				ix.need[k] = g
			}
			ix.shown[k]++
		}
	}
	m.chainIx = ix
}

// indexChainRows notes the list rows of each chain: the spark runs down
// them.
func (m *Model) indexChainRows() {
	m.chainIx.rows = map[chainKey][]int{}
	for i, s := range m.snap.Sessions {
		if k, ok := m.chainKeyOf(s); ok {
			m.chainIx.rows[k] = append(m.chainIx.rows[k], i)
		}
	}
}

func (m Model) chainKeyOf(s sample.Session) (chainKey, bool) {
	k, ok := m.chainIx.of[keyOf(s)]
	return k, ok
}

// chainOf tells if a session is in a chain.
func (m Model) chainOf(s sample.Session) bool {
	_, ok := m.chainKeyOf(s)
	return ok
}

// needGroup is the need group a session is listed in: its own, or the
// neediest of its chain's matches in the list.
func (m Model) needGroup(s sample.Session) int {
	g := m.groupOf(s)
	if k, ok := m.chainKeyOf(s); ok {
		if n, ok := m.chainIx.need[k]; ok {
			return min(g, n)
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

// chainsIn is what a group's label adds for the chains whose first match in
// view it holds: under need, how many matches each has; in every grouping,
// how many of them a filter hides.
func (m Model) chainsIn(label string, ss []sample.Session) string {
	out := ""
	seen := map[chainKey]bool{}
	for _, s := range ss {
		k, ok := m.chainKeyOf(s)
		if !ok || seen[k] {
			continue
		}
		seen[k] = true
		if m.groupLabel(s) != label {
			continue
		}
		shown, all := m.chainIx.shown[k], max(m.chainIx.present[k], m.chainIx.shown[k])
		if m.sessGroup == groupNeed {
			out += fmt.Sprintf(" · chain of %d", all)
		}
		if all > shown {
			out += fmt.Sprintf(" · %d hidden", all-shown)
		}
	}
	return out
}

// fuseWidth is the fuse column of a list w wide: none while no session in
// it is chained, two cells on a wide list, one below.
func (m Model) fuseWidth(w int) int {
	if !slices.ContainsFunc(m.snap.Sessions, m.chainOf) {
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
	rows := m.chainIx.rows[k]
	return rows[int(e*time.Duration(len(rows))/sparkFor)] == i
}

func (m Model) sparkStyle() lipgloss.Style { return lipgloss.NewStyle().Foreground(m.marks().Core) }

// mayChain tells if a row offers a chain: by need or in one list, a
// session with a repo mate on its host that it is not chained with.
func (m Model) mayChain(s sample.Session) bool {
	if m.sessGroup != groupNeed && m.sessGroup != groupNone {
		return false
	}
	return m.repoMates[keyOf(s)]
}

// sharingRepos are the sessions of ss that share their repo, on their host,
// with another session they are not chained with.
func (m Model) sharingRepos(ss []sample.Session) map[rowKey]bool {
	type in struct{ host, repo string }
	byRepo := map[in][]sample.Session{}
	for _, s := range ss {
		if r, _, ok := proto.RepoOf(m.gitOf(s.Host), s.Cwd); ok {
			byRepo[in{s.Host, r}] = append(byRepo[in{s.Host, r}], s)
		}
	}
	out := map[rowKey]bool{}
	for _, mates := range byRepo {
		for _, s := range mates {
			k, chained := m.chainKeyOf(s)
			out[keyOf(s)] = slices.ContainsFunc(mates, func(o sample.Session) bool {
				ko, ok := m.chainKeyOf(o)
				return keyOf(o) != keyOf(s) && !(chained && ok && ko == k)
			})
		}
	}
	return out
}
