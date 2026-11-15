package app

import (
	"maps"
	"strconv"
	"strings"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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

// onePane: the screen shows the list or the chat, each full width. A phone,
// a screen held upright, and one too narrow for the chat beside the list.
func (m Model) onePane(w int) bool {
	return m.layoutAt(w) == Narrow || m.upright() || w-listMedium-1 < chatMin
}

// sideBoxAt lays out the body of w columns. Side by side the list is
// listWide on a desk and listMedium on a mid screen; a hidden list leaves
// the chat the whole width.
func (m Model) sideBoxAt(w int) sideBox {
	switch {
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
func (m Model) listHidden() bool { return m.listFlip != m.listStartsHidden() }

// hideList hides the list, or shows it.
func (m *Model) hideList(hidden bool) { m.listFlip = hidden != m.listStartsHidden() }

// listShown: the list draws on the screen as it is now.
func (m Model) listShown() bool { return m.sideBoxAt(max(m.width, minWidth)).listW > 0 }

// inList: the keys go to the list. Where one pane shows, they go where the
// screen is.
func (m Model) inList() bool {
	if w := max(m.width, minWidth); m.onePane(w) {
		return !m.listHidden()
	}
	return m.keysIn == focusList
}

// toLine gives the keys to the line. Where one pane shows, the chat comes.
func (m *Model) toLine() {
	m.keysIn = focusLine
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
	if box.chatW > 0 {
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
)

// chatPlace is the chat's place in w by h: the chat, the tips, or the
// first open setup step.
func (m Model) chatPlace(w, h int) []string {
	var lines []string
	switch m.sidePlace {
	case placeTips:
		lines = m.tipsPanel(w)
	case placeSetup:
		lines = m.setupPanel(w)
	default:
		return m.chatPanel(w, h)
	}
	lines = lines[:min(len(lines), h)]
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return lines
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

// tipsPanel is the tips in the chat's place: each as the board drew it
// under its columns, the chosen one with what it is about.
func (m Model) tipsPanel(w int) []string {
	lines := []string{m.placeHead("TIPS · "+strconv.Itoa(len(m.recs)), w), strings.Repeat(" ", w)}
	at := m.tipAt()
	for i, r := range m.recs {
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
	return lines
}

// tipAt is the chosen tip, kept in range as tips go.
func (m Model) tipAt() int { return max(min(m.sideTip, len(m.recs)-1), 0) }

// setupPanel is the first open setup step in the chat's place: its name,
// and why it matters or what went wrong.
func (m Model) setupPanel(w int) []string {
	ds := m.boardDoors()
	head := "SET UP"
	if len(ds) > 1 {
		head += " · 1 of " + strconv.Itoa(len(ds))
	}
	lines := []string{m.placeHead(head, w), strings.Repeat(" ", w)}
	if len(ds) == 0 {
		return append(lines, fit(" "+m.st.muted.Render("Every setup step is done."), w))
	}
	d := ds[0]
	lines = append(lines, fit(" "+m.st.text.Bold(true).Render(ansi.Truncate(d.Title, max(w-2, 1), "…")), w))
	text, ink := d.Why, m.st.muted
	if d.Problem != "" {
		text, ink = d.Problem, m.st.warn
	}
	for _, l := range wrap(text, w-1, ink.Render) {
		lines = append(lines, fit(" "+l, w))
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
	case bkey.Matches(k, mk.NextTab, mk.PrevTab):
		m.sidePlace = placeChat
		return m.key(k)
	}
	return m, m.notice("the " + m.placeName() + " take " + strings.Join(m.placeKeys(), " ") + " · " + m.keys.side.Leave.Help().Key + " goes back to the chat")
}

// placeName is the name of what has the chat's place.
func (m Model) placeName() string {
	if m.sidePlace == placeSetup {
		return "setup steps"
	}
	return "tips"
}

// placeHints are the keys of what has the chat's place.
func (m Model) placeHints() []hint {
	sk, mk := m.keys.side, m.keys.main
	back := hintFor(sk.Leave, "chat")
	if m.sidePlace == placeSetup {
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
	m.sideIn = paneOf(s)
	return m.onCard(s).intoSelected()
}

// sideSays is what the Sessions tab says now: a notice while one shows,
// else the last flash.
func (m Model) sideSays() string {
	if m.noticeText != "" && m.now().Before(m.noticeUntil) {
		return m.noticeText
	}
	return m.flash
}

// sideNotices puts what the tab says on the hair over the key line, and
// over the body's last lines when it takes more than one: the keys stay.
func (m Model) sideNotices(sc *drawn, w int) {
	text := m.sideSays()
	if text == "" {
		return
	}
	rows := m.noticeRows(text, w)
	end := len(sc.lines)
	if sc.keysAt >= 0 {
		end = sc.keysAt // the hair is the line over the key line
	}
	for i, r := range rows {
		if y := end - len(rows) + i; y >= sc.bodyTop {
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
	if m.newer != "" { // first: its key stays when the line is cut
		line, x = line+st.warn.Render(m.newer)+"  ", x+lipgloss.Width(m.newer)+2
	}
	line, x = line+st.label.Render(where), x+lipgloss.Width(where)
	var spans []span
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

// sideHints is where the keys go and the keys of that place.
func (m Model) sideHints() (string, []hint) {
	sk, mk, bk := m.keys.side, m.keys.main, m.keys.board
	one := m.onePane(max(m.width, minWidth))
	if !m.have && m.opt.Layered { // a host that never answered: the way back
		return "reading the host", []hint{hintFor(mk.Back, "hosts"), hintFor(mk.Quit, "")}
	}
	if m.sidePlace != placeChat {
		return "in the " + m.placeName(), m.placeHints()
	}
	if m.inList() {
		return "in the list", []hint{hintFor(sk.Up, ""), hintFor(sk.Enter, "write"), hintFor(bk.Host, ""), hintFor(bk.Chain, ""), hintFor(bk.Menu, "")}
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
	if m.sidePlace == placeChat {
		m.toLine()
	}
	return m, nil
}

// chooseSide chooses the session k and gives the keys to its line.
func (m Model) chooseSide(k paneKey) Model {
	m.sideAt, m.sidePlace = k, placeChat
	if s, ok := m.sideChosen(); ok {
		m.startChatMark(s)
		m.sideName = m.cardName(s)
	}
	m.toLine()
	return m
}

// footTap opens the part of the list's foot at x.
func (m Model) footTap(foot []span, x int) (tea.Model, tea.Cmd) {
	sp, ok := spanAt(foot, x)
	switch {
	case !ok:
		return m, nil
	case sp.press == footNew:
		return m.key(keyPress(m.keys.main.New.Keys()[0]))
	case sp.press == footTips:
		m.openPlace(placeTips)
	case sp.press == footSetup:
		m.openPlace(placeSetup)
	}
	return m, nil
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

// headNameSpans are where the header's names are on head: a tap on one
// chooses its session.
func (m Model) headNameSpans(head string) ([]span, []paneKey) {
	names := m.headNames()
	if len(names) == 0 {
		return nil, nil
	}
	plain := ansi.Strip(head)
	k, i := len(names)+1, -1
	for i < 0 && k > 1 { // short of room the header names the first ones
		k--
		i = strings.Index(plain, strings.Join(names[:k], " · ")+" ")
	}
	if i < 0 {
		return nil, nil
	}
	x := ansi.StringWidth(plain[:i])
	var spans []span
	var keys []paneKey
	for j, s := range m.sideWaits()[:k] {
		nw := ansi.StringWidth(names[j])
		spans, keys = append(spans, span{x, x + nw, "name"}), append(keys, paneOf(s))
		x += nw + 3
	}
	return spans, keys
}

// sideWheel is a wheel step on the Sessions tab: it chooses the session
// above or below, as ↑ and ↓ do.
func (m Model) sideWheel(ms tea.Mouse) (tea.Model, tea.Cmd) {
	switch ms.Button {
	case tea.MouseWheelUp:
		m.sideMove(-1)
	case tea.MouseWheelDown:
		m.sideMove(1)
	}
	return m, nil
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
	if box.listW == 0 || x >= box.listW || y < 0 {
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
