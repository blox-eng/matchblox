package app

import (
	"maps"
	"slices"
	"strconv"
	"time"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
)

// noticeFor is how long a notice shows in the board's notice row.
const noticeFor = 2500 * time.Millisecond

// noticeMsg comes when a notice ends: the frame drops it.
type noticeMsg struct{}

// notice shows text in the board's notice row, over the key line, for
// noticeFor. The header never shows it.
func (m *Model) notice(text string) tea.Cmd {
	m.noticeText, m.noticeUntil, m.goArmed = text, m.now().Add(noticeFor), paneKey{}
	return tea.Tick(noticeFor, func(time.Time) tea.Msg { return noticeMsg{} })
}

// noticeLines is the notice row while a notice shows: one line, two on
// Narrow when it does not fit, so its fix is never cut.
func (m Model) noticeLines(w int) []string {
	if m.noticeText == "" || !m.now().Before(m.noticeUntil) {
		return nil
	}
	lines := []string{m.noticeText}
	if m.layoutAt(w) == Narrow && ansi.StringWidth(m.noticeText) > w-1 {
		lines = upTo(wrapPlain(m.noticeText, w-1), 2, w-1)
	}
	for i, l := range lines {
		lines[i] = fit(" "+m.st.text.Render(l), w)
	}
	return lines
}

// once is the notice of a key that changed its job: first, the first time
// ever, else said ("" says nothing).
func (m *Model) once(key, first, said string) tea.Cmd {
	if !m.told(key) {
		m.tell(key)
		return m.notice(first)
	}
	if said == "" {
		return nil
	}
	return m.notice(said)
}

// boardSelected is the chosen card's match.
func (m Model) boardSelected() (sample.Session, bool) {
	b, _ := m.boardDrawOf()
	return b.chosen, b.picked
}

// boardChooseCard makes s the chosen card of column c, and opens c. A move
// to another match starts its peek.
func (m *Model) boardChooseCard(c boardCol, s sample.Session) {
	if was, ok := m.boardSelected(); !ok || paneOf(was) != paneOf(s) {
		m.peekAt, m.peekAsk, m.goArmed = m.now(), "", paneKey{}
		if s.Ask != nil {
			m.peekAsk = s.Ask.ID
		}
	}
	pick := maps.Clone(m.boardPick)
	if pick == nil {
		pick = map[boardCol]paneKey{}
	}
	pick[c] = paneOf(s)
	m.boardAt, m.boardPick = c, pick
}

// boardMove chooses the card d cards down the open column.
func (m *Model) boardMove(d int) {
	b, _ := m.boardDrawOf()
	if !b.picked {
		return
	}
	ss := b.idx[b.open]
	i := slices.IndexFunc(ss, func(s sample.Session) bool { return paneOf(s) == paneOf(b.chosen) })
	m.boardChooseCard(b.open, ss[clampMove(i, d, len(ss))])
}

// boardSide opens the next shown column that has a card, at the left (d -1)
// or the right (d 1), on the card it had chosen.
func (m *Model) boardSide(d int) {
	b, shown := m.boardDrawOf()
	for i := slices.Index(shown, b.open) + d; i >= 0 && i < len(shown); i += d {
		c := shown[i]
		if ss := b.idx[c]; len(ss) > 0 {
			k, ok := m.boardPick[c]
			at := slices.IndexFunc(ss, func(s sample.Session) bool { return ok && paneOf(s) == k })
			m.boardChooseCard(c, ss[max(at, 0)])
			return
		}
	}
}

// boardActed chooses, after an act on the chosen card, the first other card
// of NEEDS YOU; with none, the choice stays.
func (m Model) boardActed() Model {
	if !m.boardShown() {
		return m
	}
	b, shown := m.boardDrawOf()
	if !slices.Contains(shown, colNeed) {
		return m
	}
	for _, s := range b.idx[colNeed] {
		if !b.picked || paneOf(s) != paneOf(b.chosen) {
			m.boardChooseCard(colNeed, s)
			break
		}
	}
	return m
}

// onCard is m with the selection of Sessions on s, so a key that acts on
// that selection acts on the chosen card.
func (m Model) onCard(s sample.Session) Model {
	if i := slices.IndexFunc(m.snap.Sessions, func(x sample.Session) bool { return x.PID == s.PID && x.Host == s.Host }); i >= 0 {
		m.selectRow(len(m.doors) + i)
	}
	return m
}

// boardViewSize is the room of the agent's screen in the live view beside
// the board, worked out as boardPanel lays it out; 0, 0 with no live view.
func (m Model) boardViewSize() (cols, rows int) {
	if !m.boardLive() {
		return 0, 0
	}
	w := max(m.width, minWidth)
	b, shown := m.boardDrawOf()
	_, live := boardWidths(Wide, w, shown)
	if !b.picked || len(shown) == 0 || live <= 0 {
		return 0, 0
	}
	colH := max(m.boardRows(w, m.bodyRows(), false)-1, 0) // under the heads
	return live - 1, colH - len(m.viewStrip(b.chosen, live, colH))
}

// boardLive: the board has the live view of its chosen card at its right.
func (m Model) boardLive() bool {
	w := max(m.width, minWidth)
	return m.layoutAt(w) == Wide && w >= liveFrom
}

// boardKey is k on the board: its own keys, and the keys that act on a
// match, on the chosen card. false: the tabs take k.
func (m Model) boardKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if !m.boardShown() || m.standIn || m.viewFull() {
		return m, nil, false
	}
	bk, mk := m.keys.board, m.keys.main
	s, ok := m.boardSelected()
	m.flash = ""
	var cmd tea.Cmd
	switch {
	case bkey.Matches(k, mk.Up):
		m.boardMove(-1)
	case bkey.Matches(k, mk.Down):
		m.boardMove(1)
	case bkey.Matches(k, bk.Left):
		m.boardSide(-1)
	case bkey.Matches(k, bk.Right):
		m.boardSide(1)
	case bkey.Matches(k, mk.Back):
		// The menu (Task 9) closes first; the chain filter (Task 6) after the search.
		m.goArmed = paneKey{}
		if m.filter != "" {
			m.filter = ""
			m.refresh()
		}
	case bkey.Matches(k, bk.Copy):
		cmd = m.boardCopy(s, ok)
	case bkey.Matches(k, bk.Density):
		cmd = m.once("density", bk.Density.Help().Key+" changes the cards now: verbose, or one line.", "")
		m.flipOneLine()
	case bkey.Matches(k, bk.Host): // Task 6 turns the hosts
		cmd = m.once("host", bk.Host.Help().Key+" changes the host now. The order of a column is fixed.", "")
	case bkey.Matches(k, bk.Chain): // Task 6 narrows to the chain
		cmd = m.once("chain", bk.Chain.Help().Key+" shows one chain now. "+mk.Back.Help().Key+" shows all.", "")
	case bkey.Matches(k, bk.Strip):
		cmd = m.once("strip", bk.Strip.Help().Key+" turns the strip now.", "")
		m.turnStrip()
	case !ok && len(m.snap.Sessions) > 0 && bkey.Matches(k, bk.Merge, mk.Second, mk.Grant, mk.Open, mk.Into, mk.Jump,
		mk.Park, mk.Rename, mk.Switch, mk.Dismiss, mk.Fold, mk.Stoke, mk.StokeChain, mk.Night, mk.Order):
		// Every column is hidden: never a row of Sessions the person cannot see.
		cmd = m.notice("no card is chosen · tap a + name to show a column")
	case !ok && bkey.Matches(k, bk.Merge, mk.Second, mk.Grant):
		// no match at all: nothing to act on
	case !ok:
		return m, nil, false
	case bkey.Matches(k, bk.Merge):
		cmd = m.boardMerge(s)
	case bkey.Matches(k, mk.Second):
		cmd = m.boardKill(s)
	case bkey.Matches(k, mk.Grant):
		next, cmd := m.boardGo(s)
		return next, cmd, true
	case bkey.Matches(k, mk.Open, mk.Into, mk.Jump):
		next, cmd := m.onCard(s).intoSelected()
		if nm, ok := next.(Model); ok && nm.pending == nil {
			next = nm.boardActed() // a jump that waits for its y chooses on the y
		}
		return next, cmd, true
	case bkey.Matches(k, mk.Park, mk.New, mk.Rename, mk.Switch, mk.Exited, mk.Dismiss, mk.Fold,
		mk.Stoke, mk.StokeChain, mk.Night, mk.Order, mk.Search):
		next, cmd := m.onCard(s).key(k)
		return next, cmd, true
	default:
		return m, nil, false
	}
	return m, cmd, true
}

// boardCopy is c: the chosen card's last message on the clipboard.
func (m *Model) boardCopy(s sample.Session, ok bool) tea.Cmd {
	words, said := "", "nothing to copy: no card is chosen"
	if ok {
		words, said = m.lastWords(s), "copied: the last message of "+s.Name
	}
	if ok && words == "" {
		said = "nothing to copy: " + s.Name + " said nothing yet"
	}
	note := m.once("copy", m.keys.board.Copy.Help().Key+" copies now. The stoker is on its card.", said)
	if words == "" {
		return note
	}
	return tea.Batch(m.clip(words), note)
}

// boardMerge is p: the merge of the chosen card's leading ship, behind the
// consent box, when that ship waits for it.
func (m *Model) boardMerge(s sample.Session) tea.Cmd {
	sh, ok := m.mergeable(s)
	said := "no ship of " + s.Name + " waits for a merge"
	if ok {
		said = ""
	}
	// once keeps the views, which clears the flash: it goes before refuse,
	// which says in the flash why a host takes no merge.
	note := m.once("merge", m.keys.board.Merge.Help().Key+" merges now. The ships are on their cards.", said)
	if ok {
		m.pending, m.previewTop = m.onRowHost(m.mergeAction(sh)), 0
		m.refuse()
	}
	return note
}

// boardKill is x: the kill of the chosen match, behind the consent box. On
// a card whose ship waits, the first x says once that p merges now.
func (m *Model) boardKill(s sample.Session) tea.Cmd {
	if u, head := m.headRow(s); head {
		return m.notice("the stoker of chain " + u.Name + " is no session: quit it on the STOKER tab")
	}
	a := m.killAction(s)
	switch {
	case a == nil && s.Pane == "":
		return m.notice(s.Name + " runs outside tmux: no pane to kill")
	case a == nil: // the stoker's own session
		return m.notice("the stoker is no session: quit it on the STOKER tab")
	}
	var note tea.Cmd
	if sh, ok := m.mergeable(s); ok {
		note = m.once("kill", m.keys.main.Second.Help().Key+" kills. "+m.keys.board.Merge.Help().Key+" merges #"+strconv.Itoa(sh.Number)+".", "")
	}
	m.pending, m.previewTop = m.onRowHost(a), 0
	m.refuse()
	return note
}

// boardGo is y: the go that a stoker step waits for on the chosen match.
// Once the go is sent, the choice moves on.
func (m Model) boardGo(s sample.Session) (tea.Model, tea.Cmd) {
	if s.Host == m.opt.Host {
		for _, r := range m.stokerRows() {
			if (r.kind == rowGo || r.kind == rowPlan) && r.state() == "waits" && r.e.Needs != "" && r.e.Pane == s.Pane {
				next, cmd := m.grant(r)
				if nm, ok := next.(Model); ok && cmd != nil {
					next = nm.boardActed()
				}
				return next, cmd
			}
		}
	}
	return m, m.notice("no step waits for your go on " + s.Name)
}

// boardTapGo is a tap on a card's go: a tap is not consent, so the first
// tap arms it and a second tap on the same card's go, while the notice
// shows, gives it. A move, esc or the notice's end disarms it.
func (m Model) boardTapGo(s sample.Session) (tea.Model, tea.Cmd) {
	if m.goArmed == paneOf(s) && m.now().Before(m.noticeUntil) {
		m.goArmed = paneKey{}
		return m.boardGo(s)
	}
	g := m.keys.main.Grant
	cmd := m.notice("tap " + g.Help().Key + " again to " + g.Help().Desc)
	m.goArmed = paneOf(s)
	return m, cmd
}

// boardHints are the board's keys on the key line: the chosen card's, then
// c v s u, then ←→ and tab. A phone shows c v s u only: the chosen card
// shows its own keys.
func (m Model) boardHints(w int) []hint {
	bk := m.keys.board
	density := hintFor(bk.Density, "")
	if m.oneLine() {
		density.label = "verbose"
	}
	tail := []hint{hintFor(bk.Copy, ""), density, hintFor(bk.Host, ""), hintFor(bk.Strip, "")}
	if m.layoutAt(w) == Narrow {
		return tail
	}
	var hs []hint
	switch b, _ := m.boardDrawOf(); {
	case b.picked:
		q, answers := m.oneChoice(b.chosen)
		hs, _ = m.cardHints(b.chosen, cardOpts{sel: true}, len(q.Options), answers)
	case m.have && m.filter == "" && b.empty(): // the ghost row leads
		hs = []hint{hintFor(m.keys.main.Open, "new session")}
	}
	return append(append(hs, tail...), hintFor(bk.Right, ""), hintFor(m.keys.main.NextTab, ""))
}

// boardKeyLine is the board's key line: its hints, three cells apart. A
// hint that does not fit leaves whole, with every one after it.
func (m Model) boardKeyLine(w int) (string, []span) {
	st := m.st
	line, x := " ", 1
	var spans []span
	for i, h := range m.boardHints(w) {
		gap := ""
		if i > 0 {
			gap = "   "
		}
		part := st.text.Render(h.key) + st.muted.Render(" "+h.label)
		if x+len(gap)+lipgloss.Width(part) > w {
			break
		}
		line, x = line+gap+part, x+len(gap)
		if h.press != "" {
			spans = append(spans, span{x, x + lipgloss.Width(part), h.press})
		}
		x += lipgloss.Width(part)
	}
	return fit(line, w), spans
}

// boardTap is a tap at x on line y of the board's body: it chooses the card
// there, and a tap on one of the card's keys presses that key. A
// destructive key only opens the consent box.
func (m Model) boardTap(x, y int) (tea.Model, tea.Cmd) {
	w := max(m.width, minWidth)
	d := m.boardPanel(w, m.bodyRows())
	b, _ := m.boardDrawOf()
	inColumn := func(c boardCol) bool {
		if m.layoutAt(w) == Narrow {
			return true // the page is the one column
		}
		i := slices.IndexFunc(d.heads, func(h span) bool { return h.press == c.short() })
		return i < 0 || x >= d.heads[i].x0 && x < d.heads[i].x1 // no head: the QUIET drawer, the full width
	}
	for _, c := range boardCols {
		k, ok := d.cards[boardSpot{y, c}]
		if !ok || !inColumn(c) {
			continue
		}
		at := slices.IndexFunc(b.idx[c], func(s sample.Session) bool { return paneOf(s) == k })
		if at < 0 {
			return m, nil
		}
		m.boardChooseCard(c, b.idx[c][at])
		sp, ok := spanAt(d.taps[y], x)
		switch {
		case !ok || sp.press == "":
			return m, nil
		case sp.press == m.keys.main.Grant.Keys()[0]:
			return m.boardTapGo(b.idx[c][at])
		}
		return m.keyed(keyPress(sp.press))
	}
	return m, nil
}
