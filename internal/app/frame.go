package app

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/hosts"
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
	// The marks that last (owner, 2026-11-24): the standing errors, which
	// a tap says again, and data no longer fresh.
	if n := len(m.standing()); n > 0 && m.have {
		right = append(right, gap(4), piece{st.warn.Render(fmt.Sprintf("⚠ %d", n)), "warn"})
	}
	if m.hostCount() <= 1 && m.stale(m.opt.Host) {
		stale := "stale"
		if name := m.nameOf(m.opt.Host); name != hosts.ThisMachine { // a host's console names it
			stale = name + " · stale"
		}
		right = append(right, gap(4), piece{s: st.warn.Render(stale)})
	}
	if len(right) > 0 {
		right = append(right, gap(4))
	}
	right = append(right, piece{st.muted.Render("⚙"), "cog:usage"})
	if m.busyLoop() {
		right = append(right, gap(1), piece{st.neg.Render("!"), "cog:loops"})
	}
	right = append(right, gap(1))
	if m.needCount() > 0 { // the names follow the count, in the room the spaces leave when folded
		names := m.headNamePieces(w - cells(left) - cells(right) - 12)
		right = slices.Concat(right[:1], names, right[1:])
	}

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

// headNamePieces are the names of the sessions that need the owner, in at
// most room cells, while the list is hidden: a tap on one opens it. The
// names that do not fit are counted.
func (m Model) headNamePieces(room int) []piece {
	names := m.headNames()
	for k := len(names); k > 0; k-- {
		ps := []piece{gap(2)}
		for i, name := range names[:k] {
			if i > 0 {
				ps = append(ps, piece{s: m.st.faint.Render(" · ")})
			}
			ps = append(ps, piece{m.st.text.Render(name), "name:" + strconv.Itoa(i)})
		}
		if k < len(names) {
			ps = append(ps, piece{s: m.st.muted.Render(" · +" + strconv.Itoa(len(names)-k))})
		}
		if cells(ps) <= room || k == 1 {
			return ps
		}
	}
	return nil
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

// frameBar is the desk's bar row: the mode word (and in AGENT the session
// the keys go to), each host's chip, the usage of each plan with its
// resets, and the stoker, 3 cells apart; at the right the way out of the
// agent or the shell, and ? keys. Only trouble takes colour, on its own
// chip. Short of room the hosts fold into N hosts s▸, but a host with
// trouble keeps its chip.
func (m Model) frameBar(mode string, w int) (string, []span) {
	st := m.st
	modeInk := st.accent.Bold(true)
	if mode == "LIST" {
		modeInk = st.muted
	}
	left := []piece{{s: " " + modeInk.Render(mode)}}
	if name := m.keysGoTo(); mode == "AGENT" && name != "" {
		left = append(left, gap(2), piece{s: st.text.Render(name)})
	}
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
	var hidden []piece // all hosts leaves a hidden host out: each fold still says so
	if n := len(m.hiddenHosts); n > 0 && len(chips) > 0 {
		hidden = []piece{{s: m.st.faint.Render(" · ") + st.muted.Render(fmt.Sprintf("%d hidden", n))}}
		chips[len(chips)-1] = append(chips[len(chips)-1], hidden...)
	}
	folded := chips
	switch {
	case len(hosts) > 1:
		chips[len(chips)-1] = append(chips[len(chips)-1], gap(2), turn)
		count := append([]piece{{st.muted.Render(strconv.Itoa(len(hosts)) + " hosts"), "cog:health"}}, hidden...)
		folded = append(slices.Clone(kept), append(count, gap(1), turn))
	case m.hostCount() > 1 && len(chips) > 0: // one host shown of several: s▸ turns on
		chips[0] = append(chips[0], gap(2), turn)
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

// keysGoTo names the session the agent's keys go to: the one whose
// screen the region shows, or the chosen one where matchblox draws its
// chat.
func (m Model) keysGoTo() string {
	if m.opt.Beside {
		return m.agent.name
	}
	if s, ok := m.sideChosen(); ok {
		return m.cardName(s)
	}
	return ""
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
		return []piece{{st.neg.Render(fmt.Sprintf("%s cpu %.0f%% ⚠ busy loop", name, mc.CPU)), "cog:loops:" + host}}, true
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

// pulses: the bar shows the Pulse, for a host or a plan not read yet.
func (m Model) pulses() bool {
	if m.lost {
		return false
	}
	for _, h := range m.hosts() {
		if m.inView(h) && !m.stale(h) && len(m.orphansOn(h)) == 0 && !slices.Contains(m.statedHosts(), h) {
			return true
		}
	}
	rows, _ := m.usageRows(false)
	return slices.ContainsFunc(rows, func(r usageRow) bool { return r.a.State != limits.Unmeasured && len(r.a.Uses(m.now())) == 0 })
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

// pressFrame runs a tap on the desk's head row or its bar: a space opens
// it, who needs the owner clears the filters, ? keys opens the keys, and
// a part of the machine opens the ⚙ sheet on it.
func (m Model) pressFrame(press string) (tea.Model, tea.Cmd) {
	switch {
	case press == tapAgain: // a prompt's own taps, as on the key line
		return m.startAgain()
	case press == tapBack:
		return m.leaveStandIn(), nil
	case press == "waits":
		m.clearBoardFilters()
		m.flash = ""
		return m, nil
	case press == "keys":
		return m.openKeys(), nil
	case press == "warn":
		said := m.sayStanding()
		return m, said
	case strings.HasPrefix(press, "name:"):
		i, _ := strconv.Atoi(strings.TrimPrefix(press, "name:"))
		if waits := m.sideWaits(); i < len(waits) {
			return m.chooseSide(paneOf(waits[i])), nil // the list stays hidden
		}
		return m, nil
	case strings.HasPrefix(press, "space:"):
		m.space, m.peekAt = strings.TrimPrefix(press, "space:"), m.now()
		if m.space == "all" {
			m.space = ""
		}
		return m, nil
	case press == "cog:stoker" && m.hasAgent(): // the stoker agent, as the header's word opened it
		return m.openAgent()
	case strings.HasPrefix(press, "cog:"):
		return m.openCog(press), nil
	}
	return m.keyed(keyPress(press))
}

// drawDesk lays out the desk, top to bottom: the head row, a rule, the
// list and the agent with the edge between them, a rule and the bar. The
// rules meet the edge with ┬ and ┴, each in accent on the side that has
// the keys. A box asks over it all; the ? sheet floats over the agent.
func (m Model) drawDesk(w int) drawn {
	head, headSpans := m.frameHead(w)
	mode := m.deskMode()
	bar, barSpans := m.frameBar(mode, w)
	prompt, said, spans, prompting := m.deskPrompt(w)
	if prompting {
		bar, barSpans = m.promptBar(prompt, said, spans, w)
	}
	box, room := m.sideBoxAt(w), m.bodyRows()
	rule := func(join string, lit bool) string {
		if box.listW == 0 || box.chatW == 0 {
			return m.st.rule.Render(strings.Repeat("─", w))
		}
		ink := m.st.rule
		if lit {
			ink = m.st.accent
		}
		return m.st.rule.Render(strings.Repeat("─", box.listW)) + ink.Render(join) + m.st.rule.Render(strings.Repeat("─", box.chatW))
	}
	sc := drawn{lines: []string{head, rule("┬", mode != "AGENT" && mode != "SHELL")}, head: headSpans, bodyTop: 2, noteAt: -1}
	waits := m.sideWaits()
	for _, s := range headSpans {
		switch i, err := strconv.Atoi(strings.TrimPrefix(s.press, "name:")); {
		case s.press == "waits":
			sc.waits = s
		case err == nil && i < len(waits):
			sc.names, sc.namePanes = append(sc.names, s), append(sc.namePanes, paneOf(waits[i]))
		}
	}
	b := m.sideBody(w, room)
	sc.lines, sc.rows = append(sc.lines, b.lines...), b.rows
	sc.keysAt, sc.keys = len(sc.lines)+1, barSpans
	sc.lines = append(sc.lines, rule("┴", mode == "AGENT" || mode == "SHELL"), bar)
	switch {
	case m.consenting():
		m.overlayConsent(&sc, w)
	case m.sheet != nil:
		m.overlaySheet(&sc, w)
	case m.form != nil:
		m.overlayForm(&sc, w)
	case m.menu != nil:
		m.overlayMenu(&sc, w)
	case (m.over != "" || m.cogShown()) && box.chatW > 0:
		m.overlayOver(&sc, box, room)
	}
	if prompting { // a box dims the screen, never the prompt that answers it
		sc.lines[sc.keysAt] = bar
	}
	m.deskNotice(&sc, w)
	return sc
}

// deskPrompt is what waits for a key on the desk, as the key line says it
// off the desk, and the mode word the bar wears for it: a confirm, a
// question, the room's typing line, a menu's or a place's keys, the way
// back from ⌃o, a newer release, the copy check, the record's wait. ok is
// false while nothing waits.
func (m Model) deskPrompt(w int) (mode, line string, spans []span, ok bool) {
	s, chosen := m.sideChosen()
	waits := !m.have && m.opt.Layered || m.searching || m.held != nil || m.pending != nil || m.pair.strips() || m.askingAgain() || m.lostLine() != "" ||
		m.room != nil && m.room.typing || m.menu != nil || m.newer != "" || m.copyCode != "" || m.sidePlace != placeChat ||
		m.footAt > 0 && m.inList() || chosen && m.sideIn == paneOf(s)
	wait, keys, keySpans, rec := m.waitRows(w)
	if !waits && !rec {
		return "", "", nil, false
	}
	mode = m.deskMode()
	if m.held != nil || m.pending != nil {
		mode = "CONFIRM"
	}
	if rec {
		lead := strings.TrimSpace(ansi.Strip(wait)) + "  "
		for i := range keySpans {
			keySpans[i].x0, keySpans[i].x1 = keySpans[i].x0+lipgloss.Width(lead), keySpans[i].x1+lipgloss.Width(lead)
		}
		return mode, " " + m.st.text.Render(lead) + strings.TrimPrefix(keys, " "), keySpans, true
	}
	if m.pending != nil { // over a box too: the bar says what waits for its yes
		return mode, " " + m.confirmSays(), nil, true
	}
	line, spans = m.keyLine(w)
	return mode, line, spans, true
}

// confirmSays is the bar's confirm: the step, then its keys, each key in
// fg and its word muted, a faint · between (owner, 2026-11-24).
func (m Model) confirmSays() string {
	st, ck := m.st, m.keys.confirm
	step := st.text.Render(m.pending.String()) + "   "
	if !m.previewSeen() {
		return step + st.text.Render(ck.ScrollDown.Help().Key) + st.muted.Render(" the rest, then ") + st.text.Render(ck.Yes.Help().Key)
	}
	keys := []string{st.text.Render(ck.Enter.Help().Key) + st.muted.Render(" "+ck.Enter.Help().Desc),
		st.text.Render(m.keys.main.Back.Help().Key) + st.muted.Render(" cancel")}
	if m.pending.destructive {
		keys = []string{st.text.Render(ck.Yes.Help().Key) + st.muted.Render(" run"), st.muted.Render("any other key cancels")}
	}
	return step + strings.Join(keys, st.faint.Render(" · "))
}

// promptBar is the bar while a prompt waits: the mode word first, then the
// prompt and its keys in place of the chips (owner, 2026-11-24).
func (m Model) promptBar(mode, line string, spans []span, w int) (string, []span) {
	ink := m.st.accent.Bold(true)
	if mode == "LIST" {
		ink = m.st.muted
	}
	lead := " " + ink.Render(mode) + "  "
	shift := lipgloss.Width(lead) - 1
	moved := make([]span, len(spans))
	for i, s := range spans {
		moved[i] = span{s.x0 + shift, s.x1 + shift, s.press}
	}
	return fit(lead+strings.TrimRight(strings.TrimPrefix(line, " "), " "), w), moved
}

// overlayOver floats the open sheet over the agent region: the region less
// 4 columns and a line on each side, so the agent's screen shows around
// it, and at least 60 columns: a narrower region gets a sheet that fills
// it.
func (m Model) overlayOver(sc *drawn, box sideBox, room int) {
	dx, dy := 4, 1
	if box.chatW-2*dx < 60 || room-2*dy < 4 {
		dx, dy = 0, 0
	}
	sw, sh := box.chatW-2*dx, room-2*dy
	title, lines, foot := "? keys", m.keysLines(), m.keysFoot(sw-4)
	if m.cogShown() {
		var rows []int
		title, foot = m.cogTitle(), m.cogFoot(sw-4)
		lines, rows = m.cogWindow(sw-4, sh-3)
		sc.rows = slices.Repeat([]int{-1}, len(sc.rows))
		for i, r := range rows {
			sc.rows[dy+1+i] = r
		}
		sc.overX0, sc.overX1 = box.chatX+dx, box.chatX+dx+sw
	}
	for i, l := range m.floatBox(m.st.text.Bold(true).Render(title), lines, foot, sw, sh) {
		y := sc.bodyTop + dy + i
		sc.lines[y] = overlayAt(sc.lines[y], box.chatX+dx, l)
	}
}

// floatBox is a sheet that floats over the agent, w by h: a rounded edge
// in faint, title in its top edge, 1 cell of padding inside, lines from
// the top and foot on its last line.
func (m Model) floatBox(title string, lines []string, foot string, w, h int) []string {
	f := m.st.faint
	top := f.Render("╭─ ") + title + " "
	out := []string{top + f.Render(strings.Repeat("─", max(w-lipgloss.Width(top)-1, 0))+"╮")}
	for i := range h - 2 {
		l := ""
		switch {
		case i == h-3:
			l = foot
		case i < len(lines):
			l = lines[i]
		}
		out = append(out, f.Render("│")+" "+fit(l, w-4)+" "+f.Render("│"))
	}
	return append(out, f.Render("╰"+strings.Repeat("─", max(w-2, 0))+"╯"))
}

// overlayAt is line with s drawn over it from column x.
func overlayAt(line string, x int, s string) string {
	return fit(ansi.Truncate(line, x, ""), x) + s + ansi.TruncateLeft(line, x+lipgloss.Width(s), "")
}

// deskNoticeAt is what the desk says, in the list's columns and muted,
// and the body lines it takes: the list's last ones, over its foot (spec,
// notices), never over the agent.
func (m Model) deskNoticeAt(w int) (rows []string, y0, y1 int) {
	text, lw, room := m.sideSays(), m.sideBoxAt(w).listW, m.bodyRows()
	if text == "" || lw < 2 {
		return nil, 0, 0
	}
	lines := []string{text}
	if ansi.StringWidth(text) > lw-1 { // the list is narrow: its fix takes the lines it needs, never cut
		lines = upTo(cutNotice(text, lw-1), max(noticeLines, room/3), lw-1)
	}
	lines = lines[max(len(lines)-room, 0):]
	for _, l := range lines {
		rows = append(rows, fit(" "+m.st.muted.Render(l), lw))
	}
	return rows, room - len(rows), room
}

// deskNotice draws deskNoticeAt over the body.
func (m Model) deskNotice(sc *drawn, w int) {
	rows, y0, _ := m.deskNoticeAt(w)
	for i, r := range rows {
		y := sc.bodyTop + y0 + i
		sc.lines[y] = overlayAt(sc.lines[y], 0, r)
	}
}
