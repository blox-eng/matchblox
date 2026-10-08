package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// series keeps the last n machine samples for the header sparklines.
type series struct {
	n                            int
	cpu, mem, net, io, lat, load []float64
}

func newSeries(n int) *series { return &series{n: n} }

func (s *series) push(m sample.Machine) {
	add := func(xs *[]float64, v float64) {
		*xs = append(*xs, v)
		if len(*xs) > s.n {
			*xs = (*xs)[len(*xs)-s.n:]
		}
	}
	add(&s.cpu, m.CPU)
	if m.MemTotal > 0 {
		add(&s.mem, float64(m.MemTotal-m.MemAvail))
	}
	add(&s.net, m.NetRx+m.NetTx)
	add(&s.io, m.DiskRead+m.DiskWrite)
	add(&s.lat, float64(m.Latency))
	add(&s.load, m.Load[0])
}

var blocks = []rune("▁▂▃▄▅▆▇█")

// spark draws the last w values scaled to their own max (or ceil if larger).
func spark(xs []float64, w int, ceil float64) string {
	if len(xs) > w {
		xs = xs[len(xs)-w:]
	}
	top := ceil
	for _, x := range xs {
		top = max(top, x)
	}
	var b strings.Builder
	for _, x := range xs {
		i := 0
		if top > 0 {
			i = min(int(x/top*float64(len(blocks)-1)+0.5), len(blocks)-1)
		}
		b.WriteRune(blocks[i])
	}
	return b.String()
}

func (m Model) render() string {
	if m.quitting {
		return ""
	}
	w := max(m.width, minWidth)
	if m.splashing() {
		return m.splashView(w)
	}
	var out []string
	out = append(out, m.header(w), m.tabs(w), m.st.hair.Render(strings.Repeat("─", w)))
	if m.mismatch {
		return strings.Join(append(out, "", m.mismatchText(w)), "\n")
	}
	if !m.have {
		why := m.waitingWhy()
		if len(out)+markH/2+3 <= m.height {
			out = append(append(out, ""), m.markLines(w)...)
		}
		return strings.Join(append(out, "", strings.Repeat(" ", max((w-lipgloss.Width(why))/2, 0))+m.st.faint.Render(why)), "\n")
	}
	// The line the person acts on sits under the tabs: on a phone the
	// keyboard covers the bottom of the screen.
	out = append(out[:2], m.footer(w), out[2])
	out = append(out, m.visible(w).lines...)
	for len(out) < m.height {
		out = append(out, "")
	}
	return strings.Join(out, "\n")
}

// header shows every metric with its trend when the terminal is wide, drops
// the trends when it is not, then drops metrics from the right.
func (m Model) header(w int) string {
	mc, h, st := m.snap.Machine, m.history, m.st
	sep := st.hair.Render("  │  ")
	type metric struct{ label, value, trend string }
	ms := []metric{
		{"cpu", fmt.Sprintf("%3.0f%%", mc.CPU), spark(h.cpu, 8, 100)},
		{"mem", fmt.Sprintf("%s/%s", gib(mc.MemTotal-mc.MemAvail), gib(mc.MemTotal)), spark(h.mem, 8, float64(mc.MemTotal))},
		{"load", fmt.Sprintf("%.1f", mc.Load[0]), spark(h.load, 8, 0)},
		{"net", "↓" + size(mc.NetRx) + " ↑" + size(mc.NetTx), spark(h.net, 8, 0)},
		{"io", "r " + size(mc.DiskRead) + " w " + size(mc.DiskWrite), spark(h.io, 8, 0)},
	}
	if mc.LatencyErr != "" {
		ms = append(ms, metric{"lat", st.neg.Render("! down"), ""})
	} else if mc.Latency > 0 {
		ms = append(ms, metric{"lat", fmt.Sprintf("%dms", mc.Latency.Milliseconds()), spark(h.lat, 8, 0)})
	}
	build := func(n int, trends bool) string {
		parts := []string{m.brand()}
		for _, x := range ms[:n] {
			p := st.label.Render(x.label) + " " + st.text.Render(x.value)
			if trends && x.trend != "" {
				p += " " + st.faint.Render(x.trend)
			}
			parts = append(parts, p)
		}
		return " " + strings.Join(parts, sep)
	}
	for _, trends := range []bool{true, false} {
		for n := len(ms); n > 0; n-- {
			if line := build(n, trends); lipgloss.Width(line) <= w {
				return fit(line, w)
			}
		}
	}
	return fit(build(1, false), w)
}

// tabs tries the full names, then a narrower gap, then the short names,
// then digits only (a phone), so every tab and the alert marker stay on the
// line.
func (m Model) tabs(w int) string {
	alert := m.alert()
	line := m.tabLine(m.tabTier(w))
	if alert != "" {
		line += strings.Repeat(" ", max(w-lipgloss.Width(line)-lipgloss.Width(alert)-1, 2)) + alert
	}
	return fit(line, w)
}

func (m Model) alert() string {
	n := len(m.snap.Alerts)
	if n == 0 {
		return ""
	}
	if layout(m.width) == Narrow {
		return m.st.warn.Render(fmt.Sprintf("▲ %d", n))
	}
	alert := m.st.warn.Render(fmt.Sprintf("▲ %d alert", n))
	if n > 1 {
		alert += m.st.warn.Render("s")
	}
	return alert
}

type tabTier struct {
	names []string // nil: the open tab by its short name, the others by digit
	gap   string
}

func (m Model) tabTier(w int) tabTier {
	alert := lipgloss.Width(m.alert())
	tiers := []tabTier{{tabNames, "   "}, {tabNames, "  "}, {tabShort, "  "}, {nil, "  "}}
	for _, t := range tiers {
		if lipgloss.Width(m.tabLine(t))+alert+2 <= w {
			return t
		}
	}
	return tiers[len(tiers)-1]
}

func (m Model) tabLine(t tabTier) string {
	return " " + strings.Join(m.tabParts(t), t.gap)
}

// tabParts are the tab labels; a tap finds its tab by their widths.
func (m Model) tabParts(t tabTier) []string {
	var parts []string
	for i := range tabNames {
		label := fmt.Sprint(i + 1)
		switch {
		case t.names != nil:
			label += " " + strings.ToUpper(t.names[i])
		case i == m.tab:
			label += " " + strings.ToUpper(tabShort[i])
		}
		switch {
		case i == m.tab:
			parts = append(parts, m.st.tabActive.Render(label))
		case i == tabProcs && len(m.snap.Orphans) > 0:
			parts = append(parts, m.st.neg.Render(label+" !"))
		case i == tabQueue && len(m.queue) > 0 && m.tab != tabQueue && t.names == nil:
			parts = append(parts, m.st.accent.Render(fmt.Sprintf("%s·%d", label, len(m.queue))))
		case i == tabQueue && len(m.queue) > 0 && m.tab != tabQueue:
			parts = append(parts, m.st.accent.Render(fmt.Sprintf("%s %d", label, len(m.queue))))
		default:
			parts = append(parts, m.st.faint.Render(label))
		}
	}
	return parts
}

func (m Model) footer(w int) string {
	st := m.st
	if m.input != nil {
		return fit(" "+st.label.Render("ANSWER ")+st.muted.Render(m.input.pane+" ")+
			st.text.Render(m.input.text)+st.accent.Render("▏")+"  "+st.faint.Render("⏎ review  esc cancel"), w)
	}
	if m.searching {
		return fit(" "+st.label.Render("/ ")+st.text.Render(m.filter)+st.accent.Render("▏")+"  "+st.faint.Render("⏎ keep  esc clear"), w)
	}
	if m.pending != nil {
		confirm := "⏎ run  esc cancel"
		if m.pending.destructive {
			confirm = st.neg.Render("y") + st.muted.Render(" run  any other key cancels")
		}
		return fit(" "+st.label.Render("RUN ")+st.text.Render(m.pending.String())+"   "+st.muted.Render(confirm), w)
	}
	tabKeys := m.view().keys
	if _, ok := m.selectedRec(); ok {
		tabKeys = "↑↓ select  ⏎ do  x the other action"
	}
	keys := tabKeys + "  / find  1-7 panel  q quit"
	if m.filter != "" {
		keys = "/ " + m.filter + " · esc clears  " + tabKeys
	}
	if layout(w) == Narrow {
		keys = strings.NewReplacer("↑↓ select  ", "↑↓ ", "1-7 panel", "1-7", "the other action", "other", "space mark  X all safe  ", "␣ X ", "  r rescan", "").Replace(keys)
	}
	left := " " + st.muted.Render(keys)
	if m.flash != "" {
		left = " " + st.text.Render(m.flash)
	}
	right := st.faint.Render("sampled " + sample.Human(m.now().Sub(m.snap.At)) + " ago ")
	if lipgloss.Width(left)+lipgloss.Width(right)+1 > w {
		return fit(left, w) // the keys matter more than the age
	}
	return left + strings.Repeat(" ", w-lipgloss.Width(left)-lipgloss.Width(right)) + right
}

const (
	colPane = 11 // session:window.pane; widened to show the tab name when there is room
	colTab  = 12
	colName = 18
	colSt   = 7 // the match, a space, the word
	colIdle = 6
	colBar  = 10
	colPct  = 9
	colCPU  = 6
	colDo   = 10
)

func (m Model) sessions(w, h int) body {
	st := m.st
	ss := m.snap.Sessions
	busy := 0
	for _, s := range ss {
		if s.Busy {
			busy++
		}
	}
	region := fmt.Sprintf(" %d AGENTS  ·  BUSY %d  ·  IDLE %d", len(ss), busy, len(ss)-busy) + m.sortNote(w, m.sessSort, sessCols)
	var b body
	b.add(-1, "", st.label.Render(region))
	if layout(w) == Narrow {
		// A phone: two lines for each session, no columns, no detail.
		sel := m.selIndex()
		for i, s := range ss {
			b.addRow(i, i == sel, w, st, m.narrowRow(s, i == sel)...)
		}
		return b
	}

	tab := 0
	if w >= 100 {
		tab = colTab
	}
	tree := min(max(w-(1+colPane+tab+colName+colSt+colIdle+colBar+1+colPct+colCPU+colDo+1), 12), 48)
	head, _ := m.sessHeader(tab)
	b.add(headerRow, st.label.Render(fit(head, w)))

	detail := m.detail(w)
	room := max(h-len(b.lines)-len(detail)-2, 3)
	sel := m.selIndex()
	first := max(min(sel-room/2, len(ss)-room), 0)
	for i := first; i < len(ss) && i < first+room; i++ {
		b.add(i, m.row(ss[i], i == sel, w, tab, tree))
	}
	if n := len(m.snap.IdlePanes); n > 0 {
		var ids []string
		for _, p := range m.snap.IdlePanes {
			ids = append(ids, place(p.Target, p.Pane)+" "+p.Command)
		}
		b.add(-1, st.faint.Render(fit(fmt.Sprintf(" %d without an agent: %s", n, strings.Join(ids, ", ")), w)))
	}
	b.add(-1, "", st.hair.Render(strings.Repeat("─", w)))
	b.add(-1, detail...)
	return b
}

func (m Model) row(s sample.Session, selected bool, w, tab, tree int) string {
	st := m.st
	state, idle := m.matchCell(s)+st.text.Render(pad("busy", colSt-2)), pad("", colIdle)
	if !s.Busy {
		state = m.matchCell(s) + st.faint.Render(pad("idle", colSt-2))
		idle = st.muted.Render(pad(sample.Human(s.Idle), colIdle))
	}
	ctx := st.faint.Render(pad(s.Context, colBar+1+colPct))
	if s.Context == "known" {
		pctStyle := st.text
		switch {
		case s.ContextPct >= m.compactAt():
			pctStyle = st.neg
		case s.ContextPct >= 70:
			pctStyle = st.warn
		}
		win := ""
		if s.Window >= 1_000_000 {
			win = " 1M"
		}
		ctx = st.muted.Render(bar(s.ContextPct, colBar)) + " " + pctStyle.Render(pad(fmt.Sprintf("%3.0f%%%s", s.ContextPct, win), colPct))
	}
	cpu := st.faint.Render(pad(fmt.Sprintf("%3.0f%%", s.CPU), colCPU))
	if s.CPU >= 5 {
		cpu = st.text.Render(pad(fmt.Sprintf("%3.0f%%", s.CPU), colCPU))
	}
	do := pad("", colDo+1)
	if s.Progress != nil {
		do = pad(m.progressCell(s.Progress, 5), colDo+1)
	}
	switch s.Do {
	case "compact":
		do = st.neg.Render(pad("! compact", colDo+1))
	case "clear":
		do = st.warn.Render(pad("▲ clear", colDo+1))
	}
	name := st.text.Render(pad(s.Name, colName))
	where := place(s.Target, s.Pane)
	if selected {
		where = "▌" + where
	}
	pane := st.muted.Render(pad(where, colPane))
	if selected {
		pane = st.accent.Render(pad(where, colPane))
	}
	if tab > 0 {
		pane += st.muted.Render(pad(s.Tab, tab))
	}
	line := " " + pane + name + state + idle + ctx + cpu + do + st.muted.Render(pad(worktree(s.Cwd), tree))
	if selected {
		return st.selected.Render(fit(line, w))
	}
	return fit(line, w)
}

func (m Model) compactAt() float64 { return m.opt.CompactAt }

func (m Model) detail(w int) []string {
	st := m.st
	s, ok := m.selected()
	if !ok {
		return []string{st.faint.Render(m.nothing(" no agent sessions found in tmux"))}
	}
	lines := []string{st.label.Render(" " + strings.ToUpper(place(s.Target, s.Pane)+" "+s.Tab+"  "+s.Name))}
	facts := []string{fmt.Sprintf("pid %d", s.PID)}
	if s.Pane != "" {
		facts = append(facts, "pane "+s.Pane)
	}
	if !s.Started.IsZero() {
		facts = append(facts, "started "+sample.Human(m.now().Sub(s.Started))+" ago")
	}
	if s.Model != "" {
		facts = append(facts, s.Model)
	}
	if s.Tokens > 0 {
		facts = append(facts, fmt.Sprintf("%s / %s tokens", kTok(s.Tokens), kTok(s.Window)), "+"+kTok(s.Burn30m)+" in 30 min")
	}
	facts = append(facts, fmt.Sprintf("%d processes", s.Procs))
	lines = append(lines, fit(" "+st.text.Render(strings.Join(facts, st.faint.Render("  ·  "))), w))
	lines = append(lines, fit(" "+st.muted.Render(s.Cwd), w))
	if p := s.Progress; p != nil {
		lines = append(lines, fit(" "+st.label.Render("PROGRESS ")+m.progressCell(p, 10)+"  "+st.text.Render(p.Step), w))
	}
	if s.Why != "" {
		lines = append(lines, fit(" "+st.label.Render("WHY ")+st.text.Render(s.Do+": "+s.Why), w))
	}
	if s.Pane != "" {
		lines = append(lines, fit(" "+st.label.Render("⏎ ")+st.muted.Render("tmux switch-client -t "+s.Pane), w))
	}
	return lines
}

func bar(pct float64, w int) string {
	full := min(int(pct/100*float64(w)+0.5), w)
	return strings.Repeat("█", full) + strings.Repeat("░", w-full)
}

// worktree shortens a cwd to what tells sessions apart: the checkout name,
// or the worktree name for paths under a */worktrees/* directory.
func worktree(cwd string) string {
	if cwd == "" {
		return "?"
	}
	dir, base := filepath.Split(filepath.Clean(cwd))
	if strings.Contains(filepath.Base(dir), "worktrees") {
		return "wt/" + base
	}
	return base
}

// place names a pane the way the tmux status bar does (session:window.pane),
// falling back to its id when tmux did not list it.
func place(target, id string) string {
	if target != "" {
		return target
	}
	return id
}

func pad(s string, w int) string {
	s = ansi.Truncate(s, w-1, "…")
	return s + strings.Repeat(" ", max(w-lipgloss.Width(s), 0))
}

func fit(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(w-lipgloss.Width(s), 0))
}

func gib(b uint64) string { return fmt.Sprintf("%.0fG", float64(b)/(1<<30)) }

func kTok(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	}
	return fmt.Sprintf("%dk", (n+500)/1000)
}

// size renders bytes or bytes per second compactly: 512B, 12K, 3.4M, 1.2G.
func size(b float64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1fG", b/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1fM", b/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.0fK", b/(1<<10))
	}
	return fmt.Sprintf("%.0fB", b)
}

// brand is the mark, the name and the host the console shows.
func (m Model) brand() string {
	b := m.st.accent.Render("▰") + " " + m.st.text.Render("matchblox")
	if m.host.Host != "" {
		b += m.st.faint.Render(" · ") + m.st.muted.Render(m.host.Host)
	}
	return b
}

// now is the console's clock.
func (m Model) now() time.Time {
	if m.opt.Now != nil {
		return m.opt.Now()
	}
	return time.Now()
}

// mismatchText names both versions and the command for the older side
// (design §6).
func (m Model) mismatchText(w int) string {
	host := m.host.Host
	if host == "" {
		host = "the host"
	}
	side := "update the console"
	if m.host.Version < proto.Version {
		side = "update " + host
	}
	lines := []string{
		fit(" "+m.st.text.Render(fmt.Sprintf("the console is version %d, the host is version %d", proto.Version, m.host.Version)), w),
		"",
		fit(" "+m.st.label.Render(side+": ")+m.st.text.Render(installCmd), w),
		"",
		fit(" "+m.st.muted.Render("q quit"), w),
	}
	return strings.Join(lines, "\n")
}
