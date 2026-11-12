package app

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
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
	of      map[rowKey]chainKey          // the chain a session is in; a chain of one is no chain
	need    map[chainKey]int             // the neediest group of the chain's listed matches
	present map[chainKey]int             // the matches the host has, whatever the filter hides
	shown   map[chainKey]int             // the matches in the list
	rows    map[chainKey][]int           // the list rows of each chain, top down
	heads   map[rowKey]state.ChainStoker // the head rows: the chain stoker of each
	repo    map[chainKey]string          // the repo label of a chain with a head
}

// indexChains works out which sessions are chained, and how needy their
// chains are, from the sessions the host has and the ones listed. A chain
// with one match present is no chain.
func (m *Model) indexChains() {
	ix := chainIndex{of: map[rowKey]chainKey{}}
	present := map[chainKey]int{}
	for _, s := range m.all.sessions {
		if m.isStoker(s) {
			continue
		}
		for _, c := range m.teamsOf(s) { // a match in two teams is present in both
			present[chainKey{s.Host, c.ID}]++
		}
	}
	// A match draws in the first of its teams that has another match here.
	has := func(s sample.Session) (chainKey, bool) {
		var first chainKey
		for i, c := range m.teamsOf(s) {
			k := chainKey{s.Host, c.ID}
			if present[k] > 1 {
				return k, true
			}
			if i == 0 {
				first = k
			}
		}
		return first, first.id != ""
	}
	of := map[rowKey]chainKey{}
	for _, s := range m.all.sessions {
		if k, ok := has(s); ok && !m.isStoker(s) {
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
			for _, c := range m.teamsOf(s) { // shown under its first team, not hidden from the others
				if o := (chainKey{s.Host, c.ID}); o != k && ix.present[o] > 1 {
					ix.shown[o]++
				}
			}
		}
	}
	m.chainIx = ix
}

// headOf is the chain stoker that heads chain k's fuse: its run is on or
// its agent is alive. An ended unit, or one that rests with no agent,
// heads nothing.
func (m Model) headOf(k chainKey) (state.ChainStoker, bool) {
	us := m.stateOf(k.host).Stokers
	for i := len(us) - 1; i >= 0; i-- {
		if u := us[i]; u.Chain == k.id && chainAlive(u) {
			return u, true
		}
	}
	return state.ChainStoker{}, false
}

// unitOf is the last chain stoker host lists for chain k, ended or not.
func (m Model) unitOf(k chainKey) (state.ChainStoker, bool) {
	us := m.stateOf(k.host).Stokers
	for i := len(us) - 1; i >= 0; i-- {
		if us[i].Chain == k.id {
			return us[i], true
		}
	}
	return state.ChainStoker{}, false
}

// hasUnit tells if host lists a chain stoker for chain k: a new run then
// goes to it as an order.
func (m Model) hasUnit(k chainKey) bool {
	_, ok := m.unitOf(k)
	return ok
}

// agentOf is the session of chain stoker id's agent on host: tmux session
// stoker-<id> on the stoker's server.
func (m Model) agentOf(host, id string) (sample.Session, bool) {
	for _, s := range m.all.sessions {
		if s.Host == host && m.isStoker(s) && s.TmuxSession() == sample.StokerSession+"-"+id {
			return s, true
		}
	}
	return sample.Session{}, false
}

// headless is the head row of a chain stoker whose run is on and whose
// agent is gone: no pane, and a PID no process has.
func headless(host, id string) sample.Session {
	n, _ := strconv.Atoi(strings.TrimPrefix(id, "s"))
	return sample.Session{Host: host, Name: sample.StokerSession, PID: -1 - n, Status: "idle"}
}

// addHeads puts the head row of each chain in ss into ss, its need into
// the chain's and its group's rank and wait into need and since.
func (m *Model) addHeads(ss []sample.Session, need map[rowKey]int, since map[rowKey]time.Time) []sample.Session {
	ix := &m.chainIx
	ix.heads, ix.repo = map[rowKey]state.ChainStoker{}, map[chainKey]string{}
	keys := slices.Collect(maps.Keys(ix.shown))
	slices.SortFunc(keys, func(a, b chainKey) int { return cmp.Or(cmp.Compare(a.host, b.host), cmp.Compare(a.id, b.id)) })
	for _, k := range keys {
		u, ok := m.headOf(k)
		if !ok {
			continue
		}
		for _, s := range ss {
			if ko, ok := ix.of[keyOf(s)]; ok && ko == k {
				ix.repo[k] = m.repoOf(s)
				break
			}
		}
		s, alive := m.agentOf(k.host, u.ID)
		if !alive {
			s = headless(k.host, u.ID)
		}
		r := keyOf(s)
		ix.of[r], ix.heads[r] = k, u
		g := m.groupOf(s)
		need[r] = g
		if it, ok := m.waitOf(s); ok {
			since[r] = it.Since
		}
		ix.need[k] = min(ix.need[k], g)
		ss = append(ss, s)
	}
	return ss
}

// headRow is the chain stoker whose head row s is.
func (m Model) headRow(s sample.Session) (state.ChainStoker, bool) {
	u, ok := m.chainIx.heads[keyOf(s)]
	return u, ok
}

// chainName is the name of chain k, as the service names it: its first
// member's that host lists.
func (m Model) chainName(k chainKey) string {
	cv := m.chainsOf(k.host)
	if cv == nil {
		return k.id
	}
	for _, c := range cv.Chains {
		if c.ID != k.id {
			continue
		}
		for _, mb := range c.Members {
			for _, s := range m.sessionsOf(k.host) {
				if s.Name != "" && memberOf(s).Same(mb) {
					return s.Name
				}
			}
		}
	}
	return k.id
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
		slices.SortStableFunc(mates, func(a, b sample.Session) int {
			_, ha := m.headRow(a)
			_, hb := m.headRow(b)
			switch { // the head leads its chain
			case ha && !hb:
				return -1
			case hb && !ha:
				return 1
			}
			return m.groupOf(a) - m.groupOf(b)
		})
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

// chainedTogether tells if a and b are in one chain.
func (m Model) chainedTogether(a, b sample.Session) bool {
	ka, okA := m.chainKeyOf(a)
	kb, okB := m.chainKeyOf(b)
	return okA && okB && ka == kb
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
			out[keyOf(s)] = slices.ContainsFunc(mates, func(o sample.Session) bool {
				return keyOf(o) != keyOf(s) && !m.chainedTogether(s, o)
			})
		}
	}
	return out
}

// teamsOf are the teams s is in, in the order they formed: it draws in the
// first one's fuse.
func (m Model) teamsOf(s sample.Session) []chains.Chain {
	cv := m.chainsOf(s.Host)
	if cv == nil {
		return nil
	}
	me := memberOf(s)
	var out []chains.Chain
	for _, c := range cv.Chains {
		if slices.ContainsFunc(c.Members, me.Same) {
			out = append(out, c)
		}
	}
	return out
}

// alsoIn is what a row adds for each team but the one whose fuse it draws
// in: " · also in web", or "".
func (m Model) alsoIn(s sample.Session) string {
	teams := m.teamsOf(s)
	if len(teams) < 2 {
		return ""
	}
	drawn, ok := m.chainKeyOf(s)
	if !ok {
		drawn.id = teams[0].ID
	}
	var names []string
	for _, c := range teams {
		if c.ID != drawn.id {
			names = append(names, c.Name)
		}
	}
	return " · also in " + strings.Join(names, ", ")
}

// joinNews is the toast for a change of the teams from the console's last
// view to next: a match that joined a team, or a team that formed. "" for
// none, and on the first view.
func (m Model) joinNews(next *state.ChainsView, sessions []sample.Session) string {
	if next == nil || m.chains == nil && !m.have {
		return ""
	}
	name := func(mb chains.Member) string {
		for _, s := range sessions {
			if memberOf(s).Same(mb) {
				return s.Name
			}
		}
		return mb.Pane
	}
	was := map[string]chains.Chain{}
	if m.chains != nil {
		for _, c := range m.chains.Chains {
			was[c.ID] = c
		}
	}
	for _, c := range next.Chains {
		old, ok := was[c.ID]
		if !ok {
			var names []string
			for _, mb := range c.Members {
				names = append(names, name(mb))
			}
			return c.Name + " formed: " + strings.Join(names, ", ")
		}
		for _, mb := range c.Members {
			// a /clear or switch renames a member in its pane: no join
			if !slices.ContainsFunc(old.Members, func(o chains.Member) bool { return o.Same(mb) || o.Pane == mb.Pane }) {
				out := " · u in its room takes it out"
				if m.width < 50 {
					out = " · u: out"
				}
				return name(mb) + " joined " + c.Name + out
			}
		}
	}
	return ""
}

// outOfTeam is the act that takes s out of team, from the console only:
// it asks first, as the match did not ask to leave.
func (m Model) outOfTeam(s sample.Session, team string) action {
	what := "take " + s.Name + " out of " + team
	return action{
		label:       what,
		say:         what, // the confirm's words: it has no command of its own
		rec:         proto.ChainPrefix + "out:" + s.Pane + "@" + team,
		destructive: true,
		host:        s.Host,
	}
}

// sealedFamily is what a sealed night says to a start or an order of the
// person's.
const sealedFamily = "the night is sealed: answer the stoker's prompts, or give the order in the morning"

// headed tells if chain k has a head row in the list.
func (m Model) headed(k chainKey) bool {
	for r := range m.chainIx.heads {
		if m.chainIx.of[r] == k {
			return true
		}
	}
	return false
}

// headIn is the head row of a chain, in the columns t: its stoker's match,
// the chain's name, the steps of its plan done and what it asks. On a
// phone the selected head opens in place, as any row does.
func (m Model) headIn(s sample.Session, u state.ChainStoker, sel bool, w int, t tracks, fuse string, runs bool) []string {
	st := m.st
	mark := " "
	if sel {
		mark = st.accent.Render("▌")
	}
	cell := m.matchCell(s)
	if t.fuse > 0 {
		cell = strings.TrimSuffix(cell, " ") + padTo(fuse, t.fuse) + " "
	}
	done, all := u.View.PlanProgress()
	line := mark + cell + st.ink(sel).Render("stoker") + "   " + st.text.Render(u.Name) + st.muted.Render(fmt.Sprintf(" · plan %d/%d", done, all))
	if q, asks := m.headAsks(s); asks {
		if q != "" {
			q = ": " + q
		}
		line += st.warn.Render(" · asks" + q)
	}
	rows := []string{fit(line, w)}
	if !sel || !m.inPlace() {
		return rows
	}
	return append(rows, m.fused(m.opened(s, w-t.fuse), runs, t.fuse)...)
}

// headAsks tells if a chain stoker's agent waits for the person, and its
// question.
func (m Model) headAsks(s sample.Session) (string, bool) {
	if !m.asks(s) && !s.Asks && !s.Permits {
		return "", false
	}
	if s.Ask != nil && len(s.Ask.Questions) > 0 {
		return s.Ask.Questions[0].Question, true
	}
	return m.lastWords(s), true
}

// sealedNight tells if host's stoker runs a night: the person answers
// prompts only.
func (m Model) sealedNight(host string) bool {
	v := m.stokerOf(host)
	return v != nil && v.On && v.Nightly
}

// headPane tells if a on head row s shows the stoker's pane: its run is
// on, or a night runs, and its agent has a pane.
func (m Model) headPane(s sample.Session, u state.ChainStoker) bool {
	return s.Pane != "" && (u.View.On || m.sealedNight(s.Host))
}

// headVerb is what a does on head row s.
func (m Model) headVerb(s sample.Session, u state.ChainStoker) string {
	if m.headPane(s, u) {
		return "go in"
	}
	return "stoke"
}

// headAct is a on the selected head row s: the stoker's pane in a popup,
// else the start sheet of its chain. A sealed night starts nothing.
func (m Model) headAct(s sample.Session, u state.ChainStoker) (tea.Model, tea.Cmd) {
	switch {
	case m.headPane(s, u):
		// The stoker's pane is on matchblox's own tmux server, which no
		// switch-client reaches: it opens in a popup, as on the STOKER tab.
		return m.openUnit(u)
	case m.sealedNight(s.Host):
		m.flash = sealedFamily
		return m, nil
	}
	return m.openChainSheet(m.chainIx.of[keyOf(s)]), nil
}

// stokeChain is F on the selected row: the start sheet of its chain, or a
// on its head when it has one.
func (m Model) stokeChain() (tea.Model, tea.Cmd) {
	s, ok := m.selectedSession()
	if !ok {
		return m, nil
	}
	if u, head := m.headRow(s); head {
		return m.headAct(s, u)
	}
	k, chained := m.chainKeyOf(s)
	if !chained {
		m.flash = s.Name + " is in no chain"
		return m, nil
	}
	for i, h := range m.snap.Sessions {
		if u, head := m.headRow(h); head && m.chainIx.of[keyOf(h)] == k {
			m.selectRow(len(m.doors) + i)
			return m.headAct(h, u)
		}
	}
	return m.startChain(k), nil
}

// startChain opens the start sheet of chain k, which has no head row. A
// sealed night starts nothing.
func (m Model) startChain(k chainKey) Model {
	if m.sealedNight(k.host) {
		m.flash = sealedFamily
		return m
	}
	return m.openChainSheet(k)
}

// titleOf names a row in the detail and the chat: a head row by its
// chain, not by its agent's tmux session.
func (m Model) titleOf(s sample.Session) string {
	if u, head := m.headRow(s); head {
		return "stoker of chain " + u.Name
	}
	return s.Name
}

// onFuse tells if column x of a row of a list w wide is on its fuse:
// after the mark and the match.
func (m Model) onFuse(x, w int) bool {
	fw := m.fuseWidth(w)
	return fw > 0 && x >= 2 && x < 2+fw
}

// openChainSheet opens the start sheet of chain k: a run of its stoker
// needs an order and its goes.
func (m Model) openChainSheet(k chainKey) Model {
	name := m.chainName(k)
	if u, ok := m.unitOf(k); ok {
		name = u.Name // the service's name for it
	}
	m.sheet = &sheet{host: k.host, chain: k.id, chainName: name, matches: m.chainIx.present[k]}
	m.pending = nil
	return m
}
