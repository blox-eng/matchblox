package app

import (
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// The Sessions tab at every size: the list beside one chat, or one of the
// two at a time where the chat would be too narrow beside the list.

const (
	listWide   = 40 // the list's width on a desk (Wide)
	listMedium = 32 // the list's width on a mid screen (Medium)
	// chatMin is the narrowest chat beside the list: the head line and the
	// line, with "⏎ send to <name>", still fit. A screen that leaves the chat
	// less shows one pane at a time.
	chatMin = 40
)

// sideBox is where the list and the chat draw in the body: the list from
// column 0, listW wide, then a rule, then the chat from chatX. A width of
// 0 is a pane that does not show.
type sideBox struct {
	listW, chatX, chatW int
}

// onePane: the screen shows the list or the chat, each full width. Pocket
// on a phone, held upright or narrow, and any screen too narrow for the
// chat beside the list. A desk pane of the same shape, such as half a
// laptop screen at 107x60, shows both: only Pocket tells a phone.
func (m Model) onePane(w int) bool {
	if m.beside != "" {
		return m.onePaneAt(max(m.winW, w), m.winH)
	}
	return w-listMedium-1 < chatMin || m.onPocket() && (m.upright() || m.layoutAt(w) == Narrow)
}

// sideBoxAt lays out the body of w columns. Side by side the list is
// listWide on a desk and listMedium on a mid screen; a hidden list leaves
// the chat the whole width.
func (m Model) sideBoxAt(w int) sideBox {
	switch {
	case m.beside != "" && m.boxZoom: // the list at its width, the agent's screen beside it
		lw := m.listWidth(w, m.height)
		return sideBox{listW: lw, chatX: lw + 1, chatW: max(w-lw-1, 0)}
	case m.beside != "":
		return sideBox{listW: w} // the agent has its own pane
	case m.listHidden():
		return sideBox{chatW: w}
	case m.onePane(w):
		return sideBox{listW: w}
	}
	lw := listMedium
	if m.layoutAt(w) == Wide {
		lw = listWide
	}
	return sideBox{listW: lw, chatX: lw + 1, chatW: w - lw - 1}
}

// listStartsHidden: the screen starts on the chat, and the header names
// the waits. A landscape screen too narrow for the chat beside the list
// does: its chat line stays on the screen. A phone starts on the list.
func (m Model) listStartsHidden() bool {
	w := max(m.width, minWidth)
	return m.onePane(w) && m.layoutAt(w) != Narrow && !m.upright()
}

// listHidden: the chat takes the body, and the list does not draw.
func (m Model) listHidden() bool {
	if m.beside != "" {
		return false
	}
	if m.listSet && m.listOne == m.onePane(max(m.width, minWidth)) {
		return m.listOff
	}
	return m.listStartsHidden()
}

// hideList hides the list, or shows it: the owner's choice, until the
// screen changes between one pane and two.
func (m *Model) hideList(hidden bool) {
	m.listOff, m.listSet, m.listOne = hidden, true, m.onePane(max(m.width, minWidth))
}

// listShown: the list draws on the screen as it is now.
func (m Model) listShown() bool { return m.sideBoxAt(max(m.width, minWidth)).listW > 0 }

// inList: the keys go to the list. Where one pane shows, they go where the
// screen is.
func (m Model) inList() bool {
	if m.besideMode() { // the agent has its own pane: no line takes keys
		return true
	}
	if w := max(m.width, minWidth); m.onePane(w) {
		return !m.listHidden()
	}
	return m.keysIn == focusList
}

// toLine gives the keys to the line. Where one pane shows, the chat comes.
func (m *Model) toLine() {
	m.keysIn, m.footAt = focusLine, 0
	if m.onePane(max(m.width, minWidth)) {
		m.hideList(true)
	}
}

// toList gives the keys to the list, and shows it.
func (m *Model) toList() {
	m.keysIn = focusList
	m.hideList(false)
}

// sideBody is the body of the Sessions tab in w by h: the list, a rule and
// the chat side by side, or the one pane that shows.
func (m Model) sideBody(w, h int) body {
	box := m.sideBoxAt(w)
	var list, chat []string
	if box.listW > 0 {
		list = m.sidePanel(box.listW, h)
	}
	switch {
	case box.chatW > 0 && m.beside != "":
		chat = m.agentBack(box.chatW, h)
	case box.chatW > 0:
		chat = m.chatPlace(box.chatW, h)
	}
	var b body
	rule := m.st.hair.Render("│")
	for i := range h {
		switch {
		case list != nil && chat != nil:
			b.add(-1, list[i]+rule+chat[i])
		case list != nil:
			b.add(-1, list[i])
		default:
			b.add(-1, chat[i])
		}
	}
	return b
}

// sidePlace is what the chat's place shows: the chat, or what a part of
// the list's foot opened there.
type sidePlace int

const (
	placeChat sidePlace = iota
	placeTips
	placeSetup
	placeRoom // the chain's room: the stoker's card, its steps and the talk line
	placeHost // a host's trouble and its fix
)

// chatPlace is the chat's place in w by h: the chat, the tips, the first
// open setup step, the room or a host's trouble.
func (m Model) chatPlace(w, h int) []string {
	if m.sidePlace == placeChat {
		return m.chatPanel(w, h)
	}
	lines, _ := m.placeLines(w, h)
	return lines
}

// placeLines is what has the chat's place in w by h, and the line of the
// whole place each shown line is (-1 for a cut's mark). A place taller
// than h keeps its head, and the chosen row or the talk line in view, and
// says how many lines are cut above and below.
func (m Model) placeLines(w, h int) (lines []string, from []int) {
	var all []string
	focus := 0
	switch m.sidePlace {
	case placeTips:
		all, focus = m.tipsLines(w)
	case placeSetup:
		all = m.setupPanel(w)
	case placeRoom:
		var rows []int
		all, rows = m.roomPanelRows(w)
		switch {
		case m.room != nil && m.room.typing:
			focus = len(all) - 1
		case m.room != nil:
			focus = max(slices.Index(rows, m.room.at), 0)
		}
	case placeHost:
		all = m.hostPanel(w)
	}
	blank := strings.Repeat(" ", w)
	if len(all) <= h || h < 4 {
		for i := range min(len(all), h) {
			lines, from = append(lines, all[i]), append(from, i)
		}
		for len(lines) < h {
			lines, from = append(lines, blank), append(from, -1)
		}
		return lines, from
	}
	// The head stays. Under it, the first window that shows focus, less a
	// line for each mark it needs.
	start, end := 1, 1
	for ; ; start++ {
		n := h - 1
		if start > 1 {
			n--
		}
		if end = start + n; end < len(all) {
			end--
		}
		if focus < end || end >= len(all) {
			break
		}
	}
	mark := func(arrow string, n int) string {
		return fit(" "+m.st.muted.Render(arrow+" "+strconv.Itoa(n)+" more"), w)
	}
	lines, from = []string{all[0]}, []int{0}
	if start > 1 {
		lines, from = append(lines, mark("↑", start-1)), append(from, -1)
	}
	for i := start; i < end; i++ {
		lines, from = append(lines, all[i]), append(from, i)
	}
	if end < len(all) {
		lines, from = append(lines, mark("↓", len(all)-end)), append(from, -1)
	}
	return lines, from
}

// placeHead is the first line of a place in the chat's: its name, with the
// way back to the list where one pane shows.
func (m Model) placeHead(name string, w int) string {
	return fit(m.backMark(w)+" "+m.st.label.Render(name), w)
}

// backMark is the lead of the chat's head line: the way back to the list
// where the chat takes the screen alone, else a blank.
func (m Model) backMark(w int) string {
	if m.onePane(max(m.width, minWidth)) && w >= max(m.width, minWidth) {
		return " " + m.st.accent.Render(m.keys.side.List.Help().Key)
	}
	return ""
}

// tipsLines is the tips in the chat's place: each as the board drew it
// under its columns, the chosen one with what it is about. chosen is the
// chosen tip's line.
func (m Model) tipsLines(w int) (lines []string, chosen int) {
	lines = []string{m.placeHead("TIPS · "+strconv.Itoa(len(m.recs)), w), strings.Repeat(" ", w)}
	at := m.tipAt()
	for i, r := range m.recs {
		if i == at {
			chosen = len(lines)
		}
		lines = append(lines, fit(m.recLine(r, i == at, w), w))
		if i == at && r.Evidence != "" {
			for _, l := range upTo(wrapPlain(r.Evidence, max(w-4, 1)), 3, max(w-4, 1)) {
				lines = append(lines, fit("   "+m.st.muted.Render(l), w))
			}
		}
	}
	if len(m.recs) == 0 {
		lines = append(lines, fit(" "+m.st.muted.Render("No tips now."), w))
	}
	return lines, chosen
}

// tipAt is the chosen tip, kept in range as tips go.
func (m Model) tipAt() int { return max(min(m.sideTip, len(m.recs)-1), 0) }

// setupPanel is the first open setup step in the chat's place: its name,
// and why it matters or what went wrong.
func (m Model) setupPanel(w int) []string {
	ds := m.boardDoors()
	head := "SET UP"
	if len(ds) > 1 {
		head += " · " + strconv.Itoa(m.doorIndex(ds)+1) + " of " + strconv.Itoa(len(ds))
	}
	lines := []string{m.placeHead(head, w), strings.Repeat(" ", w)}
	if len(ds) == 0 {
		return append(lines, fit(" "+m.st.muted.Render("Every setup step is done."), w))
	}
	at := m.doorIndex(ds)
	d := ds[at]
	lines = append(lines, fit(" "+m.st.text.Bold(true).Render(ansi.Truncate(d.Title, max(w-2, 1), "…")), w))
	text, ink := d.Why, m.st.muted
	if d.Problem != "" {
		text, ink = d.Problem, m.st.warn
	}
	for _, l := range wrap(text, w-1, ink.Render) {
		lines = append(lines, fit(" "+l, w))
	}
	if p, ok := m.previewDoor(); ok && p.ID == d.ID { // the step's change, read before its y
		var b body
		m.preview(&b, d, w)
		lines = append(append(lines, strings.Repeat(" ", w)), b.lines...)
	}
	return lines
}

// openPlace shows p in the chat's place, and gives it the keys. The setup
// place opens on the first open step.
func (m *Model) openPlace(p sidePlace) {
	m.sidePlace, m.sideTip = p, 0
	if p == placeSetup {
		ds := m.boardDoors()
		m.boardDoor, m.boardDoorsOf = 0, doorIDs(ds)
	}
	m.toLine()
}

// placeKey is k while the tips or a setup step have the chat's place:
// ↑↓ choose a tip, ⏎ does it, d dismisses it, esc gives the place back to
// the chat.
func (m Model) placeKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	sk, mk := m.keys.side, m.keys.main
	enter := keyPress(mk.Open.Keys()[0])
	switch {
	case bkey.Matches(k, sk.Leave):
		m.sidePlace = placeChat
		return m, nil
	case bkey.Matches(k, sk.List):
		m.sidePlace = placeChat
		m.toList()
		return m, nil
	case m.sidePlace == placeTips && bkey.Matches(k, sk.Up, sk.Down) && len(m.recs) > 0:
		d := 1
		if bkey.Matches(k, sk.Up) {
			d = -1
		}
		m.sideTip = clampMove(m.tipAt(), d, len(m.recs))
		return m, nil
	case m.sidePlace == placeTips && bkey.Matches(k, sk.Enter) && len(m.recs) > 0:
		return m.belowKey(m.recs[m.tipAt()].ID, enter)
	case m.sidePlace == placeTips && bkey.Matches(k, mk.Dismiss) && len(m.recs) > 0:
		return m.belowKey(m.recs[m.tipAt()].ID, k)
	case m.sidePlace == placeSetup && bkey.Matches(k, sk.Enter) && len(m.boardDoors()) > 0:
		return m.openBoardDoor()
	case m.sidePlace == placeSetup && bkey.Matches(k, mk.Second) && len(m.boardDoors()) > 0:
		if next, cmd, took := m.doorKey(k); took {
			return next, cmd
		}
	case m.sidePlace == placeHost && bkey.Matches(k, sk.Enter) && m.hostFix(m.sideHost) != "":
		m.sidePlace = placeChat // the fix's words, or its box, come back over the chat
		if b, ok := m.blockedOf(m.sideHost); ok {
			return m.unblockHost(b)
		}
		return m.peerTryAgain(m.peerAt(m.sideHost))
	case bkey.Matches(k, mk.NextTab, mk.PrevTab):
		m.sidePlace = placeChat
		return m.key(k)
	}
	return m, m.notice("the " + m.placeName() + " take " + strings.Join(m.placeKeys(), " ") + " · " + m.keys.side.Leave.Help().Key + " goes back to the chat")
}

// placeName is the name of what has the chat's place.
func (m Model) placeName() string {
	switch m.sidePlace {
	case placeSetup:
		return "setup steps"
	case placeRoom:
		return "room"
	case placeHost:
		return "host's trouble"
	}
	return "tips"
}

// placeHints are the keys of what has the chat's place.
func (m Model) placeHints() []hint {
	sk, mk := m.keys.side, m.keys.main
	back := hintFor(sk.Leave, "chat")
	if m.sidePlace == placeRoom {
		row, _ := m.roomChosen()
		return append(m.roomHints(row), hintFor(sk.Up, ""))
	}
	if m.sidePlace == placeHost {
		if fix := m.hostFix(m.sideHost); fix != "" {
			return []hint{hintFor(sk.Enter, fix), back}
		}
		return []hint{back}
	}
	if m.sidePlace == placeSetup && m.copyCode != "" { // the check waits for its paste
		return []hint{hintFor(mk.Back, "cancel")}
	}
	if m.sidePlace == placeSetup {
		if ds := m.boardDoors(); len(ds) > len(m.copyDoors()) && m.doorIndex(ds) >= len(m.copyDoors()) {
			return []hint{hintFor(sk.Enter, "set up"), hintFor(mk.Second, "close"), back}
		}
		return []hint{hintFor(sk.Enter, "set up"), back}
	}
	if len(m.recs) == 0 {
		return []hint{back}
	}
	return []hint{hintFor(sk.Up, ""), hintFor(sk.Enter, "do"), hintFor(mk.Dismiss, ""), back}
}

func (m Model) placeKeys() []string {
	var keys []string
	for _, h := range m.placeHints() {
		keys = append(keys, h.key)
	}
	return keys
}

// flipList is ctrl+l: the list hides, or shows again. Where one pane shows,
// the list and the chat take turns.
func (m Model) flipList() Model {
	if m.listHidden() {
		m.toList()
		return m
	}
	m.keysIn = focusLine
	m.hideList(true)
	return m
}

// expand is ctrl+o: into the chosen session's own pane, on its host. The
// key line then says the way back.
func (m Model) expand(s sample.Session, ok bool) (tea.Model, tea.Cmd) {
	if !ok {
		return m, m.notice("no session to expand · " + m.keys.main.New.Help().Key + " starts one")
	}
	next, cmd := m.onCard(s).intoSelected()
	if nm, ok := next.(Model); ok && cmd != nil && nm.flash == "" && nm.pending == nil && nm.held == nil {
		nm.sideIn = paneOf(s) // the jump runs: the key line says the way back
		return nm, cmd
	}
	return next, cmd
}

// sideSays is what the Sessions tab says now: a notice while one shows,
// the copy check while it waits for its paste, a newer release on Pocket,
// which has no key line to say it, the last flash, else the service's
// state.
func (m Model) sideSays() string {
	switch {
	case m.noticeText != "" && m.now().Before(m.noticeUntil):
		return m.noticeText
	case m.copyCode != "":
		return m.pasteHere()
	case m.newer != "" && !m.lost && m.beltOnly():
		return m.newerSaid()
	case m.flash != "":
		return m.flash
	}
	return m.stateNotice()
}

// noticeAt is where what the tab says draws, in lines of the body: the
// lines just over its last one, which holds the chat's line and the
// list's foot. y0 == y1: nothing is said.
func (m Model) noticeAt(w int) (rows []string, y0, y1 int) {
	text := m.sideSays()
	if text == "" {
		return nil, 0, 0
	}
	rows = m.noticeRows(text, w)
	y1 = max(m.bodyRows()-1, 0)
	y0 = max(y1-len(rows), 0)
	return rows[len(rows)-(y1-y0):], y0, y1
}

// sideNotices draws what the tab says over the body, above the chat's line
// and the list's foot: the keys, the line and the foot stay.
func (m Model) sideNotices(sc *drawn, w int) {
	rows, y0, _ := m.noticeAt(w)
	for i, r := range rows {
		if y := sc.bodyTop + y0 + i; y < len(sc.lines) {
			sc.lines[y] = r
		}
	}
}

// sideKeyLine is the key line of the Sessions tab: where the keys go, then
// at most five keys for that place, each a tap target.
func (m Model) sideKeyLine(w int) (string, []span) {
	st := m.st
	where, hints := m.sideHints()
	line, x := " ", 1
	var spans []span
	if newer := m.newerSaid(); newer != "" { // first: its key stays when the line is cut
		if m.startsAgain {
			spans = append(spans, span{x, x + lipgloss.Width(newer), tapAgain})
		}
		line, x = line+st.warn.Render(newer)+"  ", x+lipgloss.Width(newer)+2
	}
	line, x = line+st.label.Render(where), x+lipgloss.Width(where)
	for _, h := range hints {
		part := st.text.Render(h.key) + st.muted.Render(" "+h.label)
		if x+3+lipgloss.Width(part) > w {
			break
		}
		line, x = line+st.faint.Render(" · ")+part, x+3
		if h.press != "" {
			spans = append(spans, span{x, x + lipgloss.Width(part), h.press})
		}
		x += lipgloss.Width(part)
	}
	return fit(line, w), spans
}

// tapAgain is what a tap on the notice of a newer release presses: no
// key, as R types where the line or the room has the keys.
const tapAgain = "again"

// rTypes: R goes as text where the keys are now, to the chat's line or to
// the room. The tips, a setup step and a host's trouble take no text.
func (m Model) rTypes() bool {
	return m.sideActive() && m.menu == nil && (m.sidePlace == placeRoom || m.sidePlace == placeChat && !m.inList())
}

// newerSaid is the notice of a newer release as the key line says it:
// where R types, it names first the key that gives the keys to the list.
func (m Model) newerSaid() string {
	if m.newer == "" || !m.startsAgain || !m.rTypes() {
		return m.newer
	}
	first := m.keys.side.Leave
	if m.sidePlace == placeRoom {
		first = m.keys.side.List
	}
	return first.Help().Key + " " + m.newer
}

// sideHints is where the keys go and the keys of that place.
func (m Model) sideHints() (string, []hint) {
	sk, mk, bk := m.keys.side, m.keys.main, m.keys.board
	one := m.onePane(max(m.width, minWidth))
	if !m.have && m.opt.Layered { // a host that never answered: the way back
		return "reading the host", []hint{hintFor(mk.Back, "hosts"), hintFor(mk.Quit, "")}
	}
	if m.menu != nil {
		return "in the menu", []hint{hintFor(mk.Up, "move"), hintFor(mk.Open, "do"), hintFor(mk.Back, "close")}
	}
	if m.sidePlace != placeChat {
		return "in the " + m.placeName(), m.placeHints()
	}
	if parts := m.footParts(); m.inList() && m.footAt > 0 {
		return "in the list: " + parts[min(m.footAt, len(parts))-1].text, []hint{hintFor(sk.Enter, "open"), hintFor(sk.Up, "")}
	}
	if m.inList() {
		chain := hintFor(bk.Chain, "")
		if m.room != nil { // the room stays open behind the chat: g brings it back
			chain.label = "room"
		}
		enter := "write"
		if m.besideMode() { // the agent's own pane takes the text: ⏎ goes into it
			enter = "go in"
		}
		return "in the list", []hint{hintFor(sk.Up, ""), hintFor(sk.Enter, enter), hintFor(bk.Host, ""), chain, hintFor(bk.Menu, "")}
	}
	name := "no session"
	s, ok := m.sideChosen()
	if ok {
		name = m.cardName(s)
	}
	if ok && m.sideIn == paneOf(s) {
		return "went into " + name, []hint{{key: m.backKeys(), label: "back"}, hintFor(mk.Jump, "expand"), hintFor(sk.Enter, "")}
	}
	if one { // the way back to the list first: a phone's line is short
		return "writing to " + name, []hint{hintFor(sk.List, ""), hintFor(sk.Enter, ""), hintFor(sk.Up, ""), hintFor(mk.Jump, "expand")}
	}
	return "writing to " + name, []hint{hintFor(sk.Enter, ""), hintFor(sk.Up, ""), hintFor(sk.Leave, ""), hintFor(mk.Jump, "expand"), hintFor(mk.NextTab, "Machine")}
}

// sideKeyWords is the key line's words: where the keys go, then the keys.
func (m Model) sideKeyWords() string {
	where, hints := m.sideHints()
	parts := []string{where}
	for _, h := range hints {
		parts = append(parts, h.key+" "+h.label)
	}
	return strings.Join(parts, " · ")
}

// sideTap is a tap at x on line y of the Sessions body. On a row it
// chooses the session and opens its chat; on a group's head it folds or
// unfolds the group; on the foot it opens that part in the chat's place.
// In the chat, a tap on ◂ goes back to the list, and any other tap gives
// the keys to the line.
func (m Model) sideTap(x, y int) (tea.Model, tea.Cmd) {
	w := max(m.width, minWidth)
	box := m.sideBoxAt(w)
	if _, y0, y1 := m.noticeAt(w); y >= y0 && y < y1 {
		if m.newer != "" && m.sideSays() == m.newerSaid() && m.startsAgain { // Pocket's notice of a newer release: a tap starts again
			return m.startAgain()
		}
		return m, nil // a notice covers the line: a tap on it does nothing to what is under it
	}
	if box.listW > 0 && x < box.listW {
		lines, hits, foot := m.sideList(box.listW, m.bodyRows())
		switch hit := hits[min(max(y, 0), len(hits)-1)]; {
		case y == len(lines)-1:
			return m.footTap(foot, x)
		case hit.head:
			fold := maps.Clone(m.sideFold)
			if fold == nil {
				fold = map[sideState]bool{}
			}
			fold[hit.state] = !fold[hit.state]
			m.sideFold = fold
			return m, nil
		case hit.k != (paneKey{}) && m.besideMode():
			return m.chooseSide(hit.k).showChosen()
		case hit.k != (paneKey{}):
			return m.chooseSide(hit.k), nil
		}
		return m, nil
	}
	if box.chatW == 0 || x < box.chatX {
		return m, nil // the rule between the two
	}
	if m.backMark(box.chatW) != "" && y == 0 && x-box.chatX < 3 {
		m.sidePlace = placeChat
		m.toList()
		return m, nil
	}
	if m.sidePlace == placeRoom {
		_, rows := m.roomPanelRows(box.chatW)
		if _, from := m.placeLines(box.chatW, m.bodyRows()); y >= 0 && y < len(from) && from[y] >= 0 && rows[from[y]] >= 0 {
			return m.roomTap(rows[from[y]])
		}
		return m, nil
	}
	if m.sidePlace == placeChat {
		m.toLine()
	}
	return m, nil
}

// chooseSide chooses the session k and gives the keys to its line.
func (m Model) chooseSide(k paneKey) Model {
	m.sideAt, m.sidePlace, m.footAt = k, placeChat, 0
	if s, ok := m.sideChosen(); ok {
		m.startChatMark(s)
		m.sideName = m.cardName(s)
	}
	if !m.besideMode() { // beside, the agent has its own pane: no line
		m.toLine()
	}
	return m
}

// footTap opens the part of the list's foot at x.
func (m Model) footTap(foot []span, x int) (tea.Model, tea.Cmd) {
	if sp, ok := spanAt(foot, x); ok {
		return m.openFoot(sp.press)
	}
	return m, nil
}

// openFoot opens a part of the list's foot in the chat's place: New
// session, the tips, the first open setup step; or shows the exited agents;
// or asks to reopen the matches that wait for recover on a host.
func (m Model) openFoot(press string) (tea.Model, tea.Cmd) {
	m.footAt = 0
	switch press {
	case footNew:
		m.sidePlace = placeChat
		return m.key(keyPress(m.keys.main.New.Keys()[0]))
	case footTips:
		m.openPlace(placeTips)
	case footSetup:
		m.openPlace(placeSetup)
	case footExited:
		return m, m.flipExited()
	}
	if host, ok := strings.CutPrefix(press, footHost); ok {
		m.openPlace(placeHost)
		m.sideHost = host
	}
	if host, ok := strings.CutPrefix(press, footReopen); ok {
		return m.askReopen(host)
	}
	return m, nil
}

// askReopen asks for a typed y to reopen all the matches that wait for
// recover on host. Reopening starts agents, so the confirm names each one.
// The names go into the act as JSON, never as shell words.
func (m Model) askReopen(host string) (tea.Model, tea.Cmd) {
	_, all := m.waitingByHost()
	names := all[host]
	if len(names) == 0 {
		return m, nil
	}
	text, err := json.Marshal(proto.Recover{Names: names})
	if err != nil {
		return m, m.notice("not reopened: " + err.Error())
	}
	say := "reopen " + names[0] + "?"
	if len(names) > 1 {
		say = "reopen " + strconv.Itoa(len(names)) + " matches: " + strings.Join(names, ", ") + "?"
	}
	m.pending = &action{label: say, say: say, rec: recoverID, text: string(text), destructive: true, host: host}
	return m, nil
}

// onLastRow: the chosen session is the last row the list draws, or it
// draws none; ↓ goes on to the foot.
func (m Model) onLastRow() bool {
	walk := m.sideWalk()
	return len(walk) == 0 || walk[len(walk)-1] == m.chosenSide()
}

// flipExited is e in the list: the exited agents show at the end of IDLE,
// or hide again.
func (m *Model) flipExited() tea.Cmd {
	if len(m.snap.Exited) == 0 {
		return m.notice("no agent exited")
	}
	m.showExited = !m.showExited
	return nil
}

// sideWaits are the sessions that need you, as the list draws them: what
// the header names while the list is hidden.
func (m Model) sideWaits() []sample.Session {
	var out []sample.Session
	for _, g := range m.sideGroups() {
		if g.state != sideNeed {
			continue
		}
		for _, u := range m.sideShows(g) {
			for _, s := range u.members {
				if m.inBoard(s) && m.sideStateOf(s) == sideNeed {
					out = append(out, s)
				}
			}
		}
	}
	return out
}

// sideTitle is a session's name in the list and the header: with @host
// for another host while all hosts show.
func (m Model) sideTitle(s sample.Session) string {
	title := m.cardName(s)
	if m.hostCount() > 1 && m.boardHost == "" && s.Host != m.opt.Host {
		title += "@" + m.nameOf(s.Host)
	}
	return title
}

// headNames is what the header says while the list is hidden: the name of
// each session that needs you. "" while the list shows.
func (m Model) headNames() []string {
	if !m.sideActive() || m.listShown() || !m.have {
		return nil
	}
	var names []string
	for _, s := range m.sideWaits() {
		names = append(names, m.sideTitle(s))
	}
	return names
}

// sideWheel is a wheel step on the Sessions tab. Over the list's columns
// it chooses the session above or below, as ↑ and ↓ do. Over the chat it
// scrolls the chat, and the choice and its mark stay.
func (m Model) sideWheel(ms tea.Mouse) (tea.Model, tea.Cmd) {
	up := ms.Button == tea.MouseWheelUp
	if !up && ms.Button != tea.MouseWheelDown {
		return m, nil
	}
	if box := m.sideBoxAt(max(m.width, minWidth)); box.listW > 0 && ms.X < box.listW {
		m.sideMove(map[bool]int{true: -1, false: 1}[up])
		return m, m.previewSoon()
	}
	if m.sidePlace != placeChat {
		return m.placeWheel(up)
	}
	return m.chatWheel(up)
}

// placeWheel is a wheel step over what has the chat's place: it moves the
// choice of the tips or the room, as ↑ and ↓ do. The chosen session's
// pane is under the place, so the wheel never goes to it.
func (m Model) placeWheel(up bool) (tea.Model, tea.Cmd) {
	d := map[bool]int{true: -1, false: 1}[up]
	switch {
	case m.sidePlace == placeTips && len(m.recs) > 0:
		m.sideTip = clampMove(m.tipAt(), d, len(m.recs))
		return m, nil
	case m.sidePlace == placeRoom && m.room != nil && !m.room.typing:
		if rows := m.roomRows(m.room.k); len(rows) > 0 {
			m.roomOn(clampMove(max(m.room.at, 0), d, len(rows)))
			return m, nil
		}
	}
	return m, m.notice("nothing in the " + m.placeName() + " scrolls")
}

// chatWheel scrolls the chat of the chosen session through its pane's
// history, as a wheel in its pane does: the service moves the screen it
// watches, or gives the wheel to an agent that reads the mouse. A chat
// with no screen to scroll says so.
func (m Model) chatWheel(up bool) (tea.Model, tea.Cmd) {
	s, ok := m.sideChosen()
	if !ok || s.Pane == "" || m.watchHost != s.Host || m.watching != s.Pane || m.screen.Pane != s.Pane || m.screen.Gone {
		return m, m.notice("the chat has no screen to scroll")
	}
	if why := m.cannotReach(s.Host); why != "" {
		return m, m.notice(why)
	}
	text := proto.WheelDown
	if up {
		text = proto.WheelUp
	}
	if s.Host == m.opt.Host {
		cmd, why := m.sendKeySeen(s, text, m.screen.At)
		if why != "" {
			return m, m.notice(why)
		}
		return m, cmd
	}
	return m, m.sendTo(s.Host, proto.Act{RecID: proto.KeysPrefix + s.Pane, Text: text, PID: s.PID, Start: s.Start, Seen: m.screen.At})
}

// sideChatCols are the columns of the chat on a screen w wide, [0, 0]
// when it does not show: a drag in it selects the chat's text only.
func (m Model) sideChatCols(w int) [2]int {
	box := m.sideBoxAt(w)
	if box.chatW == 0 {
		return [2]int{}
	}
	return [2]int{box.chatX, box.chatX + box.chatW}
}

// sideDragCols are the cells of each row a drag from x may cover: the
// chat's, or the list's.
func (m Model) sideDragCols(x, w int) [2]int {
	if c := m.sideChatCols(w); c[1] > 0 && x >= c[0] {
		return c
	}
	return [2]int{0, m.sideBoxAt(w).listW}
}

// sideRowAt is the session whose row is at the frame's cell x, y.
func (m Model) sideRowAt(x, y int) (paneKey, bool) {
	w := max(m.width, minWidth)
	box := m.sideBoxAt(w)
	y -= m.drawScreen(w).bodyTop
	if _, y0, y1 := m.noticeAt(w); box.listW == 0 || x >= box.listW || y < 0 || y >= y0 && y < y1 {
		return paneKey{}, false
	}
	_, hits, _ := m.sideList(box.listW, m.bodyRows())
	if y >= len(hits) || hits[y].k == (paneKey{}) {
		return paneKey{}, false
	}
	return hits[y].k, true
}

// sideMenuOn chooses the session k and opens its menu at x, y: a
// right-click or a long press on its row.
func (m Model) sideMenuOn(k paneKey, x, y int) (tea.Model, tea.Cmd) {
	m = m.chooseSide(k)
	if v, c, ok := m.sideWaiting(); ok {
		return m.openWaitMenu(v, c, x, y), nil
	}
	s, ok := m.sideChosen()
	if !ok {
		return m, nil
	}
	return m.openMenu(s, x, y), nil
}

// sideRightClick is a right-click on a row of the list: it opens the
// session's menu there.
func (m Model) sideRightClick(ms tea.Mouse) (tea.Model, tea.Cmd) {
	k, ok := m.sideRowAt(ms.X, ms.Y)
	if !ok {
		return m, nil
	}
	return m.sideMenuOn(k, ms.X, ms.Y)
}

// formInChat draws the open form in the chat's place, beside the list:
// the form's box from the body's first line, blank under it. Where one
// pane shows, the form takes the width.
func (m Model) formInChat(sc *drawn, w int) {
	box := m.sideBoxAt(w)
	x, bw := box.chatX, box.chatW
	if bw == 0 {
		x, bw = 0, w
	}
	lines, press, spans, agentsAt, agents := m.formBox(bw)
	if rows := m.bodyRows(); len(lines) > rows && rows > 1 {
		// Short of room the form's last fields go: its key row stays.
		end := len(lines) - 1
		lines = append(lines[:rows-1:rows-1], lines[end])
		press = append(press[:rows-1:rows-1], press[end])
		if agentsAt >= rows-1 {
			agentsAt, agents = -sc.bodyTop-1, nil // the agent chips are cut: no tap finds them
		}
	}
	for i := range m.bodyRows() {
		y := sc.bodyTop + i
		if y >= len(sc.lines) {
			break
		}
		l := strings.Repeat(" ", bw)
		if i < len(lines) {
			l = fit(lines[i], bw)
		}
		left := ansi.Truncate(sc.lines[y], x, "")
		sc.lines[y] = left + strings.Repeat(" ", max(x-ansi.StringWidth(left), 0)) + l
	}
	sc.boxAt, sc.boxX, sc.boxW, sc.boxRows, sc.boxKeys = sc.bodyTop, x, bw, press, spans
	sc.formAgentsAt, sc.formAgents = sc.bodyTop+agentsAt, agents
}

// roomPanel is the chain's room in the chat's place: its name, then the
// stoker's card and its steps, then the talk line.
func (m Model) roomPanel(w int) []string {
	lines, _ := m.roomPanelRows(w)
	return lines
}

// roomPanelRows are roomPanel's lines, and the room's row each line is
// (-1 for none).
func (m Model) roomPanelRows(w int) (lines []string, rows []int) {
	if m.room == nil {
		return nil, nil
	}
	blank := strings.Repeat(" ", w)
	lines, rows = []string{m.placeHead("CHAIN "+m.roomName(m.room.k), w), blank}, []int{-1, -1}
	card, talk := m.roomDraws(w)
	for _, d := range card {
		lines, rows = append(lines, d.text), append(rows, d.at)
	}
	if len(card) > 0 {
		lines, rows = append(lines, blank), append(rows, -1)
	}
	for _, d := range talk {
		lines, rows = append(lines, d.text), append(rows, d.at)
	}
	return lines, rows
}

// roomPlaceKey is k while the room has the chat's place: ↑↓ walk its rows,
// ◂ gives the keys to the list with the room still open, and the room's
// own keys do the rest (y gives a go, ⏎ unfolds or talks, esc closes the
// room). A room that closes gives the place back to the chat.
func (m Model) roomPlaceKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	sk, mk := m.keys.side, m.keys.main
	if m.room == nil {
		m.sidePlace = placeChat
		return m, nil
	}
	rows := m.roomRows(m.room.k)
	switch {
	case m.room.typing:
	case bkey.Matches(k, sk.Up, sk.Down) && len(rows) > 0:
		d := 1
		if bkey.Matches(k, sk.Up) {
			d = -1
		}
		m.roomOn(clampMove(max(m.room.at, 0), d, len(rows)))
		return m, nil
	case bkey.Matches(k, sk.List):
		m.sidePlace = placeChat
		m.toList()
		return m, nil
	case bkey.Matches(k, mk.NextTab, mk.PrevTab):
		return m.key(k) // the room waits for the tab to come back
	case !bkey.Matches(k, m.keys.room.bindings()...):
		return m, m.notice("the room takes " + strings.Join(m.placeKeys(), " ") + " · " + sk.List.Help().Key + " gives the keys to the list")
	}
	next, cmd := m.roomKey(k)
	if nm, ok := next.(Model); ok && nm.room == nil && nm.sidePlace == placeRoom {
		nm.sidePlace = placeChat
		next = nm
	}
	return next, cmd
}

// hostPanel is a host's trouble in the chat's place: why the console
// does not read it, and its fix; or that it answers again.
func (m Model) hostPanel(w int) []string {
	host := m.sideHost
	lines := []string{m.placeHead("HOST "+m.nameOf(host), w), strings.Repeat(" ", w)}
	says, how := m.nameOf(host)+" answers again.", ""
	switch b, blocked := m.blockedOf(host); {
	case blocked:
		why, fix := m.blockedWords(b)
		says, how = m.nameOf(host)+": "+why+".", fix
	case slices.Contains(m.unreached(), host):
		says, how = m.nameOf(host)+": not reached since "+m.stateAt(host).Format("15:04")+".", "it dials again"
	}
	for _, l := range wrap(says, w-1, m.st.text.Render) {
		lines = append(lines, fit(" "+l, w))
	}
	if how != "" {
		if fix := m.hostFix(host); fix != "" {
			how = m.keys.side.Enter.Help().Key + " " + fix
		}
		lines = append(lines, fit(" "+m.st.muted.Render(how), w))
	}
	return lines
}

// hostFix is the words of what ⏎ does for host's trouble: the blocked
// host's fix when the console can run it, else try again for a host that
// is not reached; "" when ⏎ does nothing there.
func (m Model) hostFix(host string) string {
	if b, ok := m.blockedOf(host); ok {
		if !b.fixable() {
			return ""
		}
		_, fix := m.blockedWords(b)
		return fix
	}
	if slices.Contains(m.unreached(), host) && m.peerAt(host) >= 0 {
		return "try again"
	}
	return ""
}

// boardShown: the Sessions tab is open, the console's home.
func (m Model) boardShown() bool { return m.tab == tabSessions }
