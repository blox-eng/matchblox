package app

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/sample"
)

// The desk's frame: the head row over the list and the agent, the bar row
// under them (design/0022-the-desk.html, "The canon, in cells").

// piece is a part of a frame row, drawn in its inks, and what a tap on it
// presses ("" none).
type piece struct{ s, press string }

func gap(n int) piece { return piece{s: strings.Repeat(" ", n)} }

func cells(ps []piece) int {
	n := 0
	for _, p := range ps {
		n += lipgloss.Width(p.s)
	}
	return n
}

// lay draws left at the left and right against the right edge of w cells,
// with the span of each piece that presses.
func lay(left, right []piece, w int) (string, []span) {
	var b strings.Builder
	var spans []span
	x := 0
	draw := func(ps []piece) {
		for _, p := range ps {
			pw := lipgloss.Width(p.s)
			if p.press != "" {
				spans = append(spans, span{x, x + pw, p.press})
			}
			b.WriteString(p.s)
			x += pw
		}
	}
	draw(left)
	if fill := w - x - cells(right); fill > 0 {
		draw([]piece{gap(fill)})
	}
	draw(right)
	return fit(b.String(), w), spans
}

// frameHead is the desk's head row: the mark and the name, the spaces 4
// cells apart, then who needs the owner, what is done, and ⚙. The calm
// head says nothing at its right but ⚙ (design review D4). Spaces that do
// not fit fold into +N ▾.
func (m Model) frameHead(w int) (string, []span) {
	st := m.st
	left := []piece{{s: " " + st.accent.Render("▰") + " " + st.text.Render("matchblox")}}
	var right []piece
	if n := m.needCount(); n > 0 {
		right = append(right, piece{st.accent.Render(fmt.Sprintf("● %d need you", n)), "waits"})
	}
	if d := m.countIn(sideDone); d > 0 {
		if len(right) > 0 {
			right = append(right, piece{s: st.faint.Render(" · ")})
		}
		right = append(right, piece{s: lipgloss.NewStyle().Foreground(m.marks().Ember).Render(fmt.Sprintf("✓ %d done", d))})
	}
	if len(right) > 0 {
		right = append(right, gap(4))
	}
	right = append(right, piece{st.muted.Render("⚙"), "cog:usage"})
	if m.busyLoop() {
		right = append(right, gap(1), piece{st.neg.Render("!"), "cog:loops"})
	}
	right = append(right, gap(1))

	sps := m.spaces()
	if len(sps) == 0 {
		return lay(left, right, w)
	}
	names := []space{{name: "all"}}
	names = append(names, sps...)
	for k := len(names); ; k-- {
		row := slices.Clone(left)
		for _, sp := range names[:k] {
			row = append(row, gap(4), m.spacePiece(sp))
		}
		if k < len(names) {
			row = append(row, gap(4), piece{s: st.faint.Render("+" + strconv.Itoa(len(names)-k) + " ▾")})
		}
		if k == 1 || cells(row)+4+cells(right) <= w {
			return lay(row, right, w)
		}
	}
}

// spacePiece is a space's name and its waits: the open one uppercase in
// the accent, bold, the others faint; the count muted, accent when open.
func (m Model) spacePiece(sp space) piece {
	st, name, count := m.st, sp.name, ""
	open := m.space == sp.name || m.space == "" && sp.name == "all"
	ink, countInk := st.faint, st.muted
	if open {
		name, ink, countInk = strings.ToUpper(name), st.accent.Bold(true), st.accent
	}
	if sp.need > 0 {
		count = countInk.Render(" ●" + strconv.Itoa(sp.need))
	}
	return piece{ink.Render(name) + count, "space:" + sp.name}
}

// frameBar is the desk's bar row: the mode word, each host's chip, the
// usage of each plan with its resets, and the stoker, 3 cells apart; at
// the right the way out of the agent or the shell, and ? keys. Only
// trouble takes colour, on its own chip. Short of room the hosts fold
// into N hosts s▸, but a host with trouble keeps its chip.
func (m Model) frameBar(mode string, w int) (string, []span) {
	st := m.st
	modeInk := st.accent.Bold(true)
	if mode == "LIST" {
		modeInk = st.muted
	}
	left := []piece{{s: " " + modeInk.Render(mode)}}
	var right []piece
	if mode == "AGENT" || mode == "SHELL" {
		right = append(right, piece{s: st.text.Render(flipKey) + st.muted.Render(" list")}, gap(3))
	}
	right = append(right, piece{st.text.Render("?") + st.muted.Render(" keys"), "keys"}, gap(1))

	row := func(groups ...[][]piece) []piece {
		r := slices.Clone(left)
		for _, g := range groups {
			for _, c := range g {
				r = append(append(r, gap(3)), c...)
			}
		}
		return r
	}
	if m.lost {
		return lay(row([][]piece{{{s: st.warn.Render("no service · it redials")}}}), right, w)
	}
	var hosts []string
	for _, h := range m.hosts() {
		if m.inView(h) {
			hosts = append(hosts, h)
		}
	}
	turn := piece{st.faint.Render("s▸"), m.keys.board.Host.Keys()[0]}
	var chips, kept [][]piece
	for _, h := range hosts {
		c, trouble := m.hostChip(h)
		chips = append(chips, c)
		if trouble {
			kept = append(kept, c)
		}
	}
	folded := chips
	if len(hosts) > 1 {
		chips[len(chips)-1] = append(chips[len(chips)-1], gap(2), turn)
		folded = append(slices.Clone(kept), []piece{{st.muted.Render(strconv.Itoa(len(hosts)) + " hosts"), "cog:health"}, gap(1), turn})
	}
	var stoker [][]piece
	if word := m.stokerBadge(false); word != "" {
		stoker = [][]piece{{{word, "cog:stoker"}}}
	}
	// Short of room: fold the hosts, then drop the resets, the week, the
	// usage, the stoker and the host count. A trouble chip stays.
	tries := [][]piece{
		row(chips, m.usageChips(0), stoker),
		row(folded, m.usageChips(0), stoker),
		row(folded, m.usageChips(1), stoker),
		row(folded, m.usageChips(2), stoker),
		row(folded, stoker),
		row(folded),
		row(kept),
	}
	for _, r := range tries {
		if cells(r)+3+cells(right) <= w {
			return lay(r, right, w)
		}
	}
	return lay(tries[len(tries)-1], right, w)
}

// hostChip is host's chip and whether it shows trouble: its cpu and
// memory, or a busy loop in neg, or since when it is not reached in warn.
func (m Model) hostChip(host string) ([]piece, bool) {
	st, mc := m.st, m.machineOf(host)
	name := m.nameOf(host)
	switch {
	case m.stale(host):
		return []piece{{st.warn.Render(m.unreachedWord(host)), "cog:health"}}, true
	case len(m.orphansOn(host)) > 0:
		return []piece{{st.neg.Render(fmt.Sprintf("%s cpu %.0f%% ⚠ busy loop", name, mc.CPU)), "cog:loops"}}, true
	}
	cpu, mem := m.pulse(), m.pulse()
	if slices.Contains(m.statedHosts(), host) {
		_, memPct := memUse(mc)
		cpu, mem = st.muted.Render(fmt.Sprintf("%.0f%%", mc.CPU)), st.muted.Render(fmt.Sprintf("%.0f%%", memPct))
	}
	return []piece{{st.muted.Render(name), "cog:health"},
		{s: st.faint.Render(" cpu ") + cpu + st.faint.Render(" mem ") + mem}}, false
}

// usageChips are a chip for each plan, named once though several hosts
// read it: each window's percent and the time to its reset. A plan within
// 10% of a limit wears warn; one not read yet has the Pulse for a value,
// and one never measured says so (the ⚙ sheet says how to fix it). Short
// of room, short drops the resets (1), then all but the first window (2).
func (m Model) usageChips(short int) [][]piece {
	rows, _ := m.usageRows(false)
	now := m.now()
	var out [][]piece
	var named []string
	for _, r := range rows {
		if slices.Contains(named, r.name) {
			continue
		}
		named = append(named, r.name)
		uses := r.a.Uses(now)
		label, value := m.st.faint, m.st.muted
		if slices.ContainsFunc(uses, func(u limits.Use) bool { return u.Pct >= spentAt }) {
			label, value = m.st.warn, m.st.warn
		}
		mark := cmp.Or(providerMarks[r.a.Provider], "·")
		name := strings.TrimSpace(mark + " " + strings.TrimSpace(strings.TrimPrefix(r.name, cmp.Or(providerNames[r.a.Provider], r.a.Provider))))
		chip := []piece{{value.Render(name), "cog:usage"}}
		if r.a.State == limits.Unmeasured {
			out = append(out, []piece{{value.Render(name) + m.st.faint.Render(" usage not measured"), "cog:usage"}})
			continue
		}
		if len(uses) == 0 {
			out = append(out, append(chip, piece{s: " " + m.pulse()}))
			continue
		}
		var s strings.Builder
		for i, u := range uses {
			sep := " "
			if i > 0 {
				sep = " · "
			}
			s.WriteString(label.Render(sep+u.Name+" ") + value.Render(fmt.Sprintf("%.0f%%", u.Pct)))
			if short >= 2 {
				break
			}
			if !u.Resets.IsZero() && short == 0 {
				s.WriteString(label.Render(" ↻" + sample.Human(u.Resets.Sub(now))))
			}
		}
		out = append(out, append(chip, piece{s: s.String()}))
	}
	return out
}

// orphansOn are the detached busy loops host last sampled.
func (m Model) orphansOn(host string) []sample.Orphan {
	if host == m.opt.Host {
		return m.all.orphans
	}
	return m.stateOf(host).Orphans
}

// busyLoop: a detached busy loop burns on a host in view.
func (m Model) busyLoop() bool {
	return slices.ContainsFunc(m.hosts(), func(h string) bool { return m.inView(h) && len(m.orphansOn(h)) > 0 })
}

// pulse stands where a value goes before it is read (DESIGN.md §5,
// Pulse): … steps between faint and muted each 0.8 s, still in faint
// without motion. Never a zero.
func (m Model) pulse() string {
	if !m.opt.NoMotion && m.now().UnixMilli()%1600 >= 800 {
		return m.st.muted.Render("…")
	}
	return m.st.faint.Render("…")
}
