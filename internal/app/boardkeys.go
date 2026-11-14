package app

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// peekDead is how long the digits arm nothing after the peek lands on a
// card, the board opens, a tab or a filter changes, or a new question
// comes.
const peekDead = 400 * time.Millisecond

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
		m.startPeek(s)
		m.goArmed = paneKey{}
	}
	pick := maps.Clone(m.boardPick)
	if pick == nil {
		pick = map[boardCol]paneKey{}
	}
	pick[c] = paneOf(s)
	m.boardAt, m.boardPick, m.boardAbove, m.boardBelow = c, pick, 0, 0
}

// startPeek starts the peek of s's card: for peekDead its digits arm
// nothing, and a mark from before holds no more.
func (m *Model) startPeek(s sample.Session) {
	m.peekAt, m.peekAsk, m.peekPane, m.peekNew = m.now(), askOf(s), paneOf(s), false
}

// askOf is the question s asks with its options (Ask.ID), "" for none.
func askOf(s sample.Session) string {
	if s.Ask == nil {
		return ""
	}
	return s.Ask.ID
}

// chooseByHand marks the chosen card as the person's choice, after a key
// that moved it.
func (m *Model) chooseByHand() {
	if s, ok := m.boardSelected(); ok {
		m.boardMine = paneOf(s)
	}
}

// peekOn keeps the peek on the chosen card. The card coming into reach of
// the digits (the first frame, a layer over the board that closes, the
// menu that closes, a line above the columns left), a choice that moved
// with no key (a card that left its column, a filter) and a new question
// on the chosen card each start a new peek. A chosen line above the
// columns that is gone is no choice, and a menu off the board or on a
// match that is gone closes.
func (m Model) peekOn() Model {
	if m.boardAbove > len(m.aboveLines()) {
		m.boardAbove = 0
	}
	m.boardBelow = min(m.boardBelow, len(m.belowLines())) // a folded or dismissed one gives the choice back up
	m = m.copyCheckOn()
	if _, ok := m.menuOn(); m.menu != nil && !ok {
		m.menu = nil
	}
	s, ok := m.boardSelected()
	if !ok || paneOf(s) != m.boardMine {
		m.boardMine = paneKey{} // the choice moved by itself
	}
	reach := ok && m.peekReach()
	was := m.peekReached
	m.peekReached = reach
	switch {
	case !reach:
	case !was || paneOf(s) != m.peekPane:
		m.startPeek(s)
	case askOf(s) != m.peekAsk:
		m.startPeek(s)
		m.peekNew = askOf(s) != ""
	}
	return m
}

// peekReach: a digit reaches the board, with no layer or menu over it.
func (m Model) peekReach() bool {
	return m.boardShown() && m.menu == nil && !m.standIn && !m.viewFull() && m.focus(keyPress("1")) == layerMain
}

// peekAnswers: the digits answer s's question, one question with one
// choice, given with its options.
func (m Model) peekAnswers(s sample.Session) bool {
	_, ok := m.oneChoice(s)
	return ok
}

// peekMarks are the option a first digit marked on s's card, and the
// option sent to its question; 0 for none.
func (m Model) peekMarks(s sample.Session) (armed, sent int) {
	switch id := askOf(s); {
	case id == "":
	case id == m.peekSentAsk:
		sent = m.peekSent
	case paneOf(s) == m.peekPane && id == m.peekAsk && m.peekArmedAt.After(m.peekAt):
		armed = m.peekArmed
	}
	return armed, sent
}

// answerFromPeek is a digit on the board (the spec's "Answering from the
// peek"): the first marks that option of the chosen card's question, the
// same digit again sends it. It sends nothing for peekDead after the peek
// starts, to a question the card did not draw, or while the service is
// lost.
func (m Model) answerFromPeek(digit string) (tea.Model, tea.Cmd) {
	s, ok := m.boardSelected()
	q, _ := m.oneChoice(s)
	n, _ := strconv.Atoi(digit)
	armed, sent := m.peekMarks(s)
	said, answer, queued := "", tea.Cmd(nil), false
	switch {
	case !ok || sent > 0:
		// no card, or the card says its answer is sent
	case !m.peekAnswers(s) && m.asksNow(s):
		said = m.cardName(s) + " asks in its pane · " + m.keys.main.Open.Help().Key + " goes in"
	case !m.peekAnswers(s):
		said = m.cardName(s) + " asks no question"
	case m.lost || m.conn == nil:
		m.peekArmed, said = 0, "not sent: no service · it redials"
	case s.Host != m.opt.Host:
		said = "not sent: " + m.cardName(s) + " is on " + m.nameOf(s.Host) + " · " + m.keys.main.Open.Help().Key + " goes in"
	case paneOf(s) != m.peekPane || askOf(s) != m.peekAsk:
		m.startPeek(s) // a question the card has not drawn yet
		said = "not sent: " + m.cardName(s) + " asks something new — look again"
	case m.now().Sub(m.peekAt) < peekDead && m.peekNew:
		said = "not sent: " + m.cardName(s) + " asks something new — look again"
	case m.now().Sub(m.peekAt) < peekDead:
		said = "look at " + m.cardName(s) + "'s question first"
	case n < 1 || n > len(q.Options):
		said = "1-" + strconv.Itoa(len(q.Options)) + " only"
	case n == armed:
		var why string
		if answer, why = m.sendAnswer(s, digit); why != "" {
			said = why // not in line: the mark and the digits stay
			break
		}
		m.peekArmed, m.peekSent, m.peekSentAsk, m.peekSentPane, queued = 0, n, askOf(s), s.Pane, true
		said = "sent " + digit + " to " + m.cardName(s) + " · now in " + colWork.name()
	default:
		m.peekArmed, m.peekArmedAt = n, m.now()
	}
	note := m.once("digit", "A digit marks an answer now. Press it again to send.", said)
	if queued {
		return m.boardActed(), tea.Batch(answer, note)
	}
	return m, note
}

// sendAnswer sends digit to the pane of s as sendKey does, seen at the
// time of the sample that carried the question the card drew: the card's
// options come from the sample, not from the pane's screen. why says why
// the key did not go and does not wait in line; "" when it did.
func (m *Model) sendAnswer(s sample.Session, digit string) (tea.Cmd, string) {
	return m.sendKeySeen(s, proto.LitPrefix+digit, m.stateAt(s.Host))
}

// isDigit: k is a digit with no modifier.
func isDigit(k tea.KeyPressMsg) bool {
	return k.Mod == 0 && len(k.Text) == 1 && k.Text[0] >= '0' && k.Text[0] <= '9'
}

// boardMove chooses the card d cards down the open column. At Medium a
// section's end goes on into the next section with cards, down to its
// first card and up to its last. Up from the first card chooses the last
// line above the columns, and down from the last card the first line
// under them.
func (m *Model) boardMove(d int) {
	b, shown := m.boardDrawOf()
	ss := b.idx[b.open]
	i := slices.IndexFunc(ss, func(s sample.Session) bool { return b.picked && paneOf(s) == paneOf(b.chosen) })
	if b.picked && (d < 0 && i == 0 || d > 0 && i == len(ss)-1) {
		if c, ok := m.nextSection(b, shown, d); ok {
			next := b.idx[c][0]
			if d < 0 {
				next = b.idx[c][len(b.idx[c])-1]
			}
			m.boardChooseCard(c, next)
			return
		}
	}
	switch {
	case d < 0 && i <= 0 && len(m.aboveLines()) > 0:
		m.boardAbove = 1
	case d > 0 && b.picked && i == len(ss)-1 && len(m.belowLines()) > 0:
		m.boardBelow = 1
	case b.picked:
		m.boardChooseCard(b.open, ss[clampMove(i, d, len(ss))])
	}
}

// nextSection is the section of the stack at Medium after the open one
// (d 1) or before it (d -1) that shows cards. false: none, or no stack.
func (m Model) nextSection(b boardDraw, shown []boardCol, d int) (boardCol, bool) {
	if m.layoutAt(max(m.width, minWidth)) != Medium || b.open == colQuiet {
		return 0, false
	}
	for c := b.open + boardCol(d); c >= colNeed && c <= colShip; c += boardCol(d) {
		if slices.Contains(shown, c) && len(b.idx[c]) > 0 {
			return c, true
		}
	}
	return 0, false
}

// aboveMove is ↑ (d -1) and ↓ (d 1) on a line above the columns: up
// through them, and down from the last one to the first card of the open
// column.
func (m *Model) aboveMove(d int) {
	if m.boardAbove = min(m.boardAbove-d, len(m.aboveLines())); m.boardAbove > 0 {
		return
	}
	m.boardAbove = 0
	if b, _ := m.boardDrawOf(); len(b.idx[b.open]) > 0 {
		m.boardChooseCard(b.open, b.idx[b.open][0])
	}
}

// belowMove is ↓ (d 1) and ↑ (d -1) on a line under the columns: down
// through them, and up from the first one back to the card it left.
func (m *Model) belowMove(d int) {
	if m.boardBelow = min(m.boardBelow+d, len(m.belowLines())); m.boardBelow <= 0 {
		m.boardBelow = 0
		m.recPick, m.ghostPick = "", 0
	}
}

// belowKey is ⏎, x, d or h on a line under the columns: the ghost row and
// a recommendation take them as their rows in Sessions did.
func (m Model) belowKey(line string, k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.pickBelow(line)
	return m.key(k)
}

// pickBelow selects a line under the columns: the recommendation with the
// id line while it shows, else the ghost row.
func (m *Model) pickBelow(line string) {
	m.doorPick, m.recPick, m.ghostPick = "", "", 1
	if line != "" && slices.ContainsFunc(m.recs[:m.recRows()], func(r advice.Rec) bool { return r.ID == line }) {
		m.recPick, m.ghostPick = line, 0
	}
}

// boardOnRow: Sessions' selection is what the board chose (onChoice): a
// card of a session, a line under the columns, or a service's door. A ship
// card, a host line, the console's own doors, a search that matches nothing,
// every column hidden and an open menu have no row.
func (m Model) boardOnRow() bool {
	if m.menu != nil {
		return false // the menu takes every key; its go in asks for itself
	}
	if s, ok := m.boardSelected(); ok {
		return !isShipCard(s)
	}
	if _, ok := m.belowChosen(); ok {
		return true
	}
	line, ok := m.aboveChosen()
	return ok && line == "" && m.doorIndex(m.boardDoors()) >= len(m.copyDoors())
}

// onChoice is m with the selection of Sessions on what the board chose:
// the layers over the board (the take-over question) read that selection,
// so they act on the card, or the line, the key was pressed on.
func (m Model) onChoice() Model {
	if !m.boardShown() {
		return m
	}
	if s, ok := m.boardSelected(); ok {
		return m.onCard(s)
	}
	if line, ok := m.belowChosen(); ok {
		m.pickBelow(line)
	}
	if line, ok := m.aboveChosen(); ok && line == "" {
		if i := m.doorIndex(m.boardDoors()) - len(m.copyDoors()); i >= 0 {
			m.selectRow(i)
		}
	}
	return m
}

// doorKey is x on the doors line: a service's door closes, behind its y,
// as from its row in Sessions. false: the door is the console's own.
func (m Model) doorKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	ds, own := m.boardDoors(), len(m.copyDoors())
	i := m.doorIndex(ds)
	if i < own {
		return m, nil, false
	}
	m.selectRow(i - own)
	next, cmd := m.key(k)
	return next, cmd, true
}

// aboveOpen is ⏎ on a line above the columns: a blocked host gets its fix,
// a host that is not reached is dialed again, and the setup doors open as
// from their row in Sessions.
func (m Model) aboveOpen(host string, k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if host == "" {
		return m.openBoardDoor()
	}
	if b, ok := m.blockedOf(host); ok {
		return m.unblockHost(b)
	}
	if i := m.peerAt(host); i >= 0 {
		return m.peerTryAgain(i)
	}
	return m, nil
}

// boardSide opens the next shown column that has a card, at the left (d -1)
// or the right (d 1), on the card it had chosen. Past the last one it shows
// a hidden column, so a keyboard alone brings one back: the first of them
// at the right (their + names end the tab row), the last at the left.
func (m *Model) boardSide(d int) {
	b, shown := m.boardDrawOf()
	at := slices.Index(shown, b.open)
	if len(shown) == 0 {
		at = -1
	}
	for i := at + d; i >= 0 && i < len(shown); i += d {
		if m.boardOpen(shown[i]) {
			return
		}
	}
	hidden := slices.DeleteFunc(slices.Clone(boardCols[:]), func(c boardCol) bool { return !m.colHidden(c) })
	switch {
	case len(hidden) == 0:
	case d > 0:
		m.boardShowCol(hidden[0])
	default:
		m.boardShowCol(hidden[len(hidden)-1])
	}
}

// boardOpen opens column c on the card it had chosen, else its first;
// false: c has no card.
func (m *Model) boardOpen(c boardCol) bool {
	b, _ := m.boardDrawOf()
	ss := b.idx[c]
	if len(ss) == 0 {
		return false
	}
	k, ok := m.boardPick[c]
	at := slices.IndexFunc(ss, func(s sample.Session) bool { return ok && paneOf(s) == k })
	m.boardChooseCard(c, ss[max(at, 0)])
	return true
}

// boardShowCol shows column c when it is hidden, and opens it.
func (m *Model) boardShowCol(c boardCol) {
	if m.colHidden(c) {
		m.flipHidden(c)
	}
	if !m.boardOpen(c) {
		m.boardAt = c
	}
}

// clearBoardFilters is a tap on the count: NEEDS YOU shows, every chain,
// host and search goes, and the first wait is chosen.
func (m *Model) clearBoardFilters() {
	if m.colHidden(colNeed) {
		m.flipHidden(colNeed)
	}
	if m.room != nil { // the room is the narrowing: both go
		*m = m.closeRoom()
	}
	m.boardChain, m.boardHost, m.filter, m.searching = "", "", "", false
	m.refresh()
	m.peekAt, m.boardAt = m.now(), colNeed
	if ss := m.boardColumn(colNeed); len(ss) > 0 {
		m.boardChooseCard(colNeed, ss[0])
	}
}

// boardChainOf is g: the board narrows to the chosen card's chain. A card
// in no chain says so.
func (m *Model) boardChainOf(s sample.Session, ok bool) tea.Cmd {
	name, said := "", ""
	if ok {
		if name = m.chainLabelOf(s); name == "" {
			said = s.Name + " is in no chain"
		}
	}
	cmd := m.once("chain", m.keys.board.Chain.Help().Key+" shows one chain now. "+m.keys.main.Back.Help().Key+" shows all.", said)
	if k, in := m.chainKeyOf(s); ok && in {
		*m = m.openRoom(k)
	}
	return cmd
}

// turnChain is ] (d 1) and [ (d -1): the next or the previous chain on
// the board, from the first or the last when none is chosen.
func (m *Model) turnChain(d int) {
	names := m.boardChains()
	if len(names) == 0 {
		return
	}
	i := slices.Index(names, m.boardChain)
	switch {
	case i < 0 && d > 0:
		i = 0
	case i < 0:
		i = len(names) - 1
	default:
		i = (i + d + len(names)) % len(names)
	}
	if k := slices.IndexFunc(m.roomKeys(), func(k chainKey) bool { return m.chainLabel(k) == names[i] }); k >= 0 {
		*m = m.openRoom(m.roomKeys()[k])
	}
}

// turnHost is s: the next host, then all hosts again.
func (m *Model) turnHost() {
	if m.hostCount() == 1 {
		return
	}
	var names []string
	for _, h := range m.hosts() {
		names = append(names, m.nameOf(h))
	}
	next := ""
	if i := slices.Index(names, m.boardHost); i+1 < len(names) {
		next = names[i+1]
	}
	m.boardHost, m.peekAt = next, m.now()
}

// boardActed chooses, after an act on the chosen card, the first other card
// of NEEDS YOU; with none, the choice stays.
func (m Model) boardActed() Model {
	if _, below := m.belowChosen(); !m.boardShown() || below {
		return m // an act on a line under the columns keeps it, with what came of it
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
	if i := slices.IndexFunc(m.snap.Sessions, func(x sample.Session) bool {
		return paneOf(x) == paneOf(s) && (s.Pane != "" || x.PID == s.PID) // outside tmux the pane names no one
	}); i >= 0 {
		m.selectRow(len(m.doors) + i)
	}
	return m
}

// boardLive: the board has the live view of its chosen card at its right,
// with the rows for a menu.
func (m Model) boardLive() bool {
	w := max(m.width, minWidth)
	return m.layoutAt(w) == Wide && w >= liveFrom && m.boardRows(w, m.bodyRows(), false) >= menuLeast
}

// boardKey is k on the board: its own keys, and the keys that act on a
// match, on the chosen card. false: the tabs take k.
func (m Model) boardKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if !m.boardShown() || m.standIn || m.viewFull() {
		return m, nil, false
	}
	if m.menu != nil { // the menu takes every key
		next, cmd := m.menuKey(k)
		return next, cmd, true
	}
	bk, mk := m.keys.board, m.keys.main
	if bkey.Matches(k, mk.Back) && m.waiting() { // the record wait takes esc before the board
		cmd := m.switchNow()
		return m, cmd, true
	}
	if !m.have && m.opt.Layered && bkey.Matches(k, mk.Back) {
		return m, nil, false // a host that never answered: esc goes back to Hosts
	}
	if m.room != nil && m.copyCode == "" {
		if next, cmd, took := m.roomBoardKey(k); took {
			return next, cmd, true
		}
	}
	s, ok := m.boardSelected()
	line, above := m.aboveChosen()
	under, below := m.belowChosen()
	m.flash = ""
	var cmd tea.Cmd
	switch {
	case ok && (isDigit(k) || bkey.Matches(k, cardKeys(bk, mk)...)) && !m.cardDrawn(s):
		// A screen too short for the chosen card: no key acts on a card it does not show.
		cmd = m.notice("no card is shown")
	case m.copyCode != "" && bkey.Matches(k, mk.Back):
		m.copyCode = "" // the check closes, unchecked
		if m.noticeText == m.pasteHere() {
			m.noticeText = ""
		}
	case above && line == "" && bkey.Matches(k, bk.Left):
		m.stepDoor(-1)
	case above && line == "" && bkey.Matches(k, bk.Right):
		m.stepDoor(1)
	case above && bkey.Matches(k, mk.Up):
		m.aboveMove(-1)
	case above && bkey.Matches(k, mk.Down):
		m.aboveMove(1)
	case above && bkey.Matches(k, mk.Back):
		m.boardAbove = 0 // back to the card
	case above && bkey.Matches(k, mk.Open):
		next, cmd := m.aboveOpen(line, k)
		return next, cmd, true
	case above && line == "" && bkey.Matches(k, mk.Second):
		if next, cmd, took := m.doorKey(k); took {
			return next, cmd, true
		}
		cmd = m.notice("no card is chosen · ↓ goes back to the cards")
	case (above || below) && isDigit(k):
		// a digit answers a card, and no card is chosen
	case below && bkey.Matches(k, mk.Up):
		m.belowMove(-1)
	case below && bkey.Matches(k, mk.Down):
		m.belowMove(1)
	case below && bkey.Matches(k, mk.Back):
		m.belowMove(-m.boardBelow) // back to the card
	case below && bkey.Matches(k, mk.Open, mk.Second, mk.Dismiss, mk.Fold):
		next, cmd := m.belowKey(under, k)
		return next, cmd, true
	case bkey.Matches(k, mk.Up):
		m.boardMove(-1)
		m.chooseByHand()
	case bkey.Matches(k, mk.Down):
		m.boardMove(1)
		m.chooseByHand()
	case bkey.Matches(k, bk.Left):
		m.boardSide(-1)
		m.chooseByHand()
	case bkey.Matches(k, bk.Right):
		m.boardSide(1)
		m.chooseByHand()
	case bkey.Matches(k, mk.Back):
		// The menu took esc first; then the mark, the search, the chain.
		m.goArmed = paneKey{}
		switch armed, _ := m.peekMarks(s); {
		case ok && armed > 0:
			m.peekArmed = 0
		case m.filter != "":
			m.filter, m.peekAt = "", m.now()
			m.refresh()
		case m.room != nil:
			m = m.closeRoom()
		case m.boardChain != "":
			m.boardChain, m.peekAt = "", m.now()
		}
	case bkey.Matches(k, bk.Copy):
		cmd = m.boardCopy(s, ok)
	case bkey.Matches(k, bk.Density):
		cmd = m.once("density", bk.Density.Help().Key+" changes the cards now: verbose, or one line.", "")
		m.flipOneLine()
	case bkey.Matches(k, bk.Host):
		cmd = m.once("host", bk.Host.Help().Key+" changes the host now. The order of a column is fixed.", "")
		m.turnHost()
	case bkey.Matches(k, bk.Chain):
		cmd = m.boardChainOf(s, ok)
	case bkey.Matches(k, bk.NextChain):
		m.turnChain(1)
	case bkey.Matches(k, bk.PrevChain):
		m.turnChain(-1)
	case bkey.Matches(k, bk.Strip):
		cmd = m.once("strip", bk.Strip.Help().Key+" turns the strip now.", "")
		m.turnStripShown()
	case isDigit(k): // never a tab on the board
		next, cmd := m.answerFromPeek(k.Text)
		return next, cmd, true
	case !ok && len(m.snap.Sessions) > 0 && bkey.Matches(k, bk.Merge, bk.Menu, mk.Second, mk.Grant, mk.Open, mk.Into, mk.Jump,
		mk.Park, mk.Rename, mk.Switch, mk.Dismiss, mk.Fold, mk.Stoke, mk.StokeChain, mk.Night, mk.Order):
		// A line above or under the columns is chosen, a search or a host leaves
		// no card, or every column is hidden: never a row of Sessions the person
		// cannot see. The notice names the fix that is true.
		switch {
		case above:
			cmd = m.notice("no card is chosen · ↓ goes back to the cards")
		case below:
			cmd = m.notice("no card is chosen · ↑ goes back to the cards")
		case m.filter != "":
			cmd = m.notice(`nothing matches "` + m.filter + `" · ` + mk.Back.Help().Key + " clears")
		case m.boardHost != "" && !slices.ContainsFunc(boardCols[:], m.colHidden):
			cmd = m.notice("no match on " + m.boardHost + " · " + bk.Host.Help().Key + " turns the host")
		default:
			cmd = m.notice("no card is chosen · tap a + name to show a column")
		}
	case !ok && bkey.Matches(k, bk.Merge, bk.Menu, mk.Second, mk.Grant):
		// no match at all: nothing to act on
	case !ok:
		return m, nil, false
	case isShipCard(s) && bkey.Matches(k, mk.Open, mk.Into, mk.Jump):
		next, cmd := m.boardInto(s)
		return next, cmd, true
	case isShipCard(s) && !bkey.Matches(k, bk.Menu, bk.Merge):
		// no session: no pane to act on, nothing to kill, park or rename
		cmd = m.notice(s.Name + " has no session · " + mk.Open.Help().Key + " starts one")
	case bkey.Matches(k, bk.Menu):
		m = m.openMenuOnChosen(s)
	case bkey.Matches(k, bk.Merge):
		cmd = m.boardMerge(s)
	case bkey.Matches(k, mk.Second):
		cmd = m.boardKill(s)
	case bkey.Matches(k, mk.Grant) && m.onPocket() && m.goWaitsHere(s): // the belt's y is a tap: arm, then give
		next, cmd := m.armGo(s, mk.Grant.Help().Key+" again gives the go: "+m.goWhat(s))
		return next, cmd, true
	case bkey.Matches(k, mk.Grant):
		next, cmd := m.boardGo(s)
		return next, cmd, true
	case bkey.Matches(k, mk.Open, mk.Into, mk.Jump):
		next, cmd := m.boardInto(s)
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

// cardKeys are the keys that act on the chosen card.
func cardKeys(bk boardKeys, mk mainKeys) []bkey.Binding {
	return []bkey.Binding{bk.Merge, bk.Menu, bk.Copy, bk.Chain, mk.Second, mk.Grant, mk.Open, mk.Into, mk.Jump,
		mk.Park, mk.Rename, mk.Switch, mk.Dismiss, mk.Fold, mk.Stoke, mk.StokeChain, mk.Night, mk.Order}
}

// cardDrawn: the board draws the card of s on the screen it has now.
func (m Model) cardDrawn(s sample.Session) bool {
	d := m.boardPanel(max(m.width, minWidth), m.bodyRows())
	for _, k := range d.cards {
		if k == paneOf(s) {
			return true
		}
	}
	return false
}

// boardInto is ⏎: into the pane of s, then the first other wait is chosen.
func (m Model) boardInto(s sample.Session) (tea.Model, tea.Cmd) {
	if sh, ok := m.shipOfCard(s); ok { // a ship no session owns: a session starts on it
		next := m.takeShip(sh)
		cmd := next.flashNotice("")
		return next, cmd
	}
	next, cmd := m.onCard(s).intoSelected()
	if nm, ok := next.(Model); ok && nm.pending == nil && !nm.standIn {
		next = nm.boardActed() // a jump that waits for its y chooses on the y; the stand-in keeps its match
	}
	return next, cmd
}

// boardCopy is c: the chosen card's last message, whole, on the clipboard.
func (m *Model) boardCopy(s sample.Session, ok bool) tea.Cmd {
	first := m.keys.board.Copy.Help().Key + " copies now. The stoker is on its card."
	switch {
	case !ok:
		return m.once("copy", first, "nothing to copy: no card is chosen")
	case m.lastWords(s) == "":
		return m.once("copy", first, "nothing to copy: "+s.Name+" said nothing yet")
	}
	copied := m.copyText(m.lastWords(s), "last message of "+s.Name)
	if m.clipKnown && !m.clipOK || m.told("copy") {
		return copied // the fix comes before the news of the key
	}
	said := m.noticeText
	news := m.once("copy", first, "") // the first c says what changed, then what it copied
	m.noticeNext = said
	return tea.Batch(copied, news)
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

// quitStokerWhere says where a stoker's agent quits: its screen opens from
// Machine's row stoker (ruling 28).
func (m Model) quitStokerWhere() string {
	return m.keys.main.QuitAgent.Help().Key + " quits it on Machine › stoker"
}

// boardKill is x: the kill of the chosen match, behind the consent box. On
// a card whose ship waits, the first x says once that p merges now.
func (m *Model) boardKill(s sample.Session) tea.Cmd {
	if u, head := m.headRow(s); head {
		return m.notice("the stoker of chain " + u.Name + " is no session: " + m.quitStokerWhere())
	}
	a := m.killAction(s)
	switch {
	case a == nil && s.Pane == "":
		return m.notice(s.Name + " runs outside tmux: no pane to kill")
	case a == nil: // the stoker's own session
		return m.notice("the stoker is no session: " + m.quitStokerWhere())
	}
	var note tea.Cmd
	if sh, ok := m.mergeable(s); ok {
		note = m.once("kill", m.keys.main.Second.Help().Key+" kills. "+m.keys.board.Merge.Help().Key+" merges #"+strconv.Itoa(sh.Number)+".", "")
	}
	m.pending, m.previewTop = m.onRowHost(a), 0
	m.refuse()
	return note
}

// goStep is the step of this host's stoker that waits for the owner's go
// on the pane of s.
func (m Model) goStep(s sample.Session) (stokerRow, bool) {
	if s.Host != m.opt.Host {
		return stokerRow{}, false
	}
	for _, r := range m.stokerRows() {
		if (r.kind == rowGo || r.kind == rowPlan) && r.state() == "waits" && r.e.Needs != "" && r.e.Pane == s.Pane {
			return r, true
		}
	}
	return stokerRow{}, false
}

// goWaitsHere: a step of this host's stoker waits for the owner's go on s.
func (m Model) goWaitsHere(s sample.Session) bool {
	_, ok := m.goStep(s)
	return ok
}

// goWhat is what the go that waits on s lets run, in words: "merge #4556"
// for the step that needs merge:#4556.
func (m Model) goWhat(s sample.Session) string {
	if r, ok := m.goStep(s); ok {
		return strings.ReplaceAll(r.e.Needs, ":", " ")
	}
	return m.keys.main.Grant.Help().Desc
}

// boardGo is y: the go that a stoker step waits for on the chosen match.
// Once the go is sent, the choice moves on.
func (m Model) boardGo(s sample.Session) (tea.Model, tea.Cmd) {
	r, ok := m.goStep(s)
	if !ok {
		return m, m.notice("no step waits for your go on " + s.Name)
	}
	next, cmd := m.grant(r)
	if nm, ok := next.(Model); ok && cmd != nil {
		next = nm.boardActed()
	}
	return next, cmd
}

// goArmedOn: a first tap, or a first y on Pocket, armed the go of s, and
// its notice still shows.
func (m Model) goArmedOn(s sample.Session) bool {
	return m.goArmed == paneOf(s) && m.goArmed != (paneKey{}) && m.now().Before(m.noticeUntil)
}

// armGo is a tap on a card's go, or y on Pocket, whose belt y is a tap: a
// tap is not consent, so the first arms the go and says first, and a
// second on the same card's go, while the notice shows, gives it. Any
// other key or tap, and the notice's end, disarms it.
func (m Model) armGo(s sample.Session, first string) (tea.Model, tea.Cmd) {
	if m.goArmedOn(s) {
		m.goArmed = paneKey{}
		return m.boardGo(s)
	}
	cmd := m.notice(first)
	m.goArmed = paneOf(s)
	return m, cmd
}

// boardTapGo is a tap on a card's go (armGo).
func (m Model) boardTapGo(s sample.Session) (tea.Model, tea.Cmd) {
	g := m.keys.main.Grant
	return m.armGo(s, "tap "+g.Help().Key+" again to "+g.Help().Desc)
}

// boardHints are the board's keys on the key line: the chosen card's, then
// c v s u, then ←→ and tab. A phone shows c v s u only: the chosen card
// shows its own keys.
func (m Model) boardHints(w int) []hint {
	bk, mk := m.keys.board, m.keys.main
	if m.menu != nil { // the menu takes every key
		return []hint{hintFor(mk.Up, ""), hintFor(mk.Open, "do"), hintFor(mk.Back, "close")}
	}
	density := hintFor(bk.Density, "")
	if m.oneLine() {
		density.label = "verbose"
	}
	if !m.have && m.opt.Layered { // a host that never answered: the way back
		return []hint{hintFor(mk.Back, "hosts"), hintFor(mk.Quit, "")}
	}
	tail := []hint{hintFor(bk.Copy, ""), density, hintFor(bk.Host, ""), hintFor(bk.Strip, "")}
	cancel := hintFor(mk.Back, "cancel") // the copy check waits for its paste
	if b, _ := m.boardDrawOf(); m.have && b.none && m.copyCode == "" && m.boardAbove == 0 {
		// No session at all: only the keys that act on an empty board.
		return []hint{hintFor(mk.Open, "new session"), hintFor(bk.Strip, ""), hintFor(mk.NextTab, "")}
	}
	if m.layoutAt(w) == Narrow {
		if m.copyCode != "" {
			return append([]hint{cancel}, tail...)
		}
		return tail
	}
	var hs []hint
	line, above := m.aboveChosen()
	under, below := m.belowChosen()
	row, own := m.roomChosen()
	switch b, _ := m.boardDrawOf(); {
	case m.copyCode != "":
		hs = []hint{cancel}
	case own:
		hs = m.roomHints(row)
	case above && line == "" && m.doorIndex(m.boardDoors()) >= len(m.copyDoors()):
		hs = []hint{hintFor(mk.Open, "set up"), hintFor(mk.Second, "close")}
	case above && line == "":
		hs = []hint{hintFor(mk.Open, "set up")}
	case above:
		hs = []hint{hintFor(mk.Open, "try again")}
	case below:
		hs = m.belowHints(under)
	case b.picked && m.cardDrawn(b.chosen):
		q, answers := m.offersDigits(b.chosen)
		armed, sent := m.peekMarks(b.chosen)
		hs, _ = m.cardHints(b.chosen, cardOpts{sel: true, armed: armed}, len(q.Options), answers && sent == 0)
	}
	right := hintFor(bk.Right, "")
	switch {
	case above && line == "" && len(m.boardDoors()) > 1:
		right.label = "doors"
	case m.layoutAt(max(m.width, minWidth)) == Medium:
		right.label = "sections" // the stack's sections are its columns
	}
	return append(append(hs, tail...), right, hintFor(mk.NextTab, ""))
}

// belowHints are the keys of a line under the columns: the ghost row's,
// or the recommendation id's.
func (m Model) belowHints(id string) []hint {
	mk := m.keys.main
	i := slices.IndexFunc(m.recs, func(r advice.Rec) bool { return r.ID == id })
	if id == "" || i < 0 {
		return []hint{hintFor(mk.Open, "new session")}
	}
	hs := []hint{hintFor(mk.Open, "do")}
	if m.recs[i].Second != nil {
		hs = append(hs, hintFor(mk.Second, "other"))
	}
	return append(hs, hintFor(mk.Dismiss, "dismiss"), hintFor(mk.Fold, "hide"))
}

// boardKeyLine is the board's key line: the notice of a newer service
// first, then its hints, three cells apart. A hint that does not fit
// leaves whole, with every one after it.
func (m Model) boardKeyLine(w int) (string, []span) {
	st := m.st
	line, x := " ", 1
	if m.newer != "" { // first: its key stays when the line is cut
		line += st.warn.Render(m.newer)
		x += lipgloss.Width(m.newer)
	}
	var spans []span
	for i, h := range m.boardHints(w) {
		gap := ""
		if i > 0 || m.newer != "" {
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

// boardTap is a tap at x on line y of the board's body: on a column's name,
// the QUIET drawer's too, it hides the column; on a phone's page mark it
// opens that page, hides the open one and shows a hidden one; on a host
// line it dials the host again. On a card it chooses the card, and a tap
// on one of the card's keys presses that key. A destructive key only opens
// the consent box.
func (m Model) boardTap(x, y int) (tea.Model, tea.Cmd) {
	w := max(m.width, minWidth)
	d := m.boardPanel(w, m.bodyRows())
	b, _ := m.boardDrawOf()
	if h, ok := spanAt(d.heads, x); ok && y == d.headAt && h.press == stokerMark {
		return m.openStokerPage(), nil
	}
	if at, ok := d.roomAt[y]; ok {
		return m.roomTap(at)
	}
	if h, ok := spanAt(d.heads, x); ok && y == d.headAt {
		c := boardCols[slices.IndexFunc(boardCols[:], func(c boardCol) bool { return c.short() == h.press })]
		if m.colHidden(c) || m.layoutAt(w) == Narrow && c != b.open {
			m.boardShowCol(c)
		} else {
			m.flipHidden(c)
		}
		return m, nil
	}
	if host, ok := d.hostAt[y]; ok {
		return m.aboveOpen(host, keyPress(m.keys.main.Open.Keys()[0]))
	}
	if y == d.doorsAt {
		lines := m.aboveLines()
		m.boardAbove = len(lines) - slices.Index(lines, "") // counted from the columns up
		return m.openBoardDoor()
	}
	if at, ok := d.underAt[y]; ok {
		return m.underTap(at, x, w)
	}
	if sp, ok := spanAt(d.taps[y], x); ok {
		if i := slices.IndexFunc(boardCols[:], func(c boardCol) bool { return c.short() == sp.press }); i >= 0 {
			m.flipHidden(boardCols[i]) // the drawer's name, or a section's in the stack
			return m, nil
		}
	}
	if d.live > 0 && x >= w-d.live && y >= d.headAt && y < d.colsTo && b.picked {
		return m.boardInto(b.chosen) // the live view is the chosen card's pane
	}
	c, s, ok := m.cardAt(d, b, x, y)
	if !ok {
		return m, nil
	}
	again := m.chosenByHand(s)
	m.boardChooseCard(c, s)
	m.boardMine = paneOf(s) // a tap chooses by hand
	sp, ok := spanAt(d.taps[y], x)
	goIn := ok && sp.press == m.keys.main.Open.Keys()[0]
	switch {
	case (!ok || sp.press == "" || goIn) && again: // the chosen card again: in
		return m.boardInto(s)
	case !ok || sp.press == "" || goIn: // a tap goes in only on a card the person chose
		return m, nil
	case sp.press == m.keys.main.Grant.Keys()[0]:
		m.goArmed = m.goArmedTap // the tap disarmed it; a tap on its own go gives it
		return m.boardTapGo(s)
	}
	return m.keyed(keyPress(sp.press))
}

// underTap is a tap at x on a line under the columns, as on its row in
// Sessions: the ghost row opens New session, the recommendations' label
// folds them, and the chosen one's dismiss mark dismisses it.
func (m Model) underTap(at string, x, w int) (tea.Model, tea.Cmd) {
	mk := m.keys.main
	switch line, below := m.belowChosen(); {
	case at == atGhost:
		m.boardBelow = 1
		return m.belowKey("", keyPress(mk.Open.Keys()[0]))
	case at == atRecs:
		return m.belowKey("", keyPress(mk.Fold.Keys()[0]))
	case below && at == line && x >= w-m.dismissWidth():
		return m.belowKey(line, keyPress(mk.Dismiss.Keys()[0]))
	}
	return m, nil
}

// cardAt is the card at x on line y of the board d draws, and its column.
func (m Model) cardAt(d boardDrawn, b boardDraw, x, y int) (boardCol, sample.Session, bool) {
	inColumn := func(c boardCol) bool {
		if m.layoutAt(max(m.width, minWidth)) == Narrow {
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
			return c, sample.Session{}, false
		}
		return c, b.idx[c][at], true
	}
	return 0, sample.Session{}, false
}

// boardTabTap is a tap on the crumb of the tab row: esc all shows every
// chain, and a hidden column's + name shows it and opens it. false: the
// tap is on a tab.
func (m Model) boardTabTap(press string) (Model, bool) {
	if !m.boardShown() {
		return m, false
	}
	if press == m.keys.main.Back.Help().Key { // the room is the narrowing: both go
		return m.closeRoom(), true
	}
	for _, c := range boardCols {
		if press == c.short() {
			m.boardShowCol(c)
			return m, true
		}
	}
	return m, false
}

// beltKey is a key of the board on Pocket's belt (design decision 10): the
// key the console reads, its face and word on the belt, On when it applies
// (a key that is off is dim and sends nothing), and Act for the chosen
// card's own keys, in the accent.
type beltKey struct {
	Key, Face, Word string
	On, Act         bool
}

// beltOf is binding b on the belt: its first key, face (its help key when
// ""), and word.
func beltOf(b bkey.Binding, face, word string, on bool) beltKey {
	return beltKey{Key: b.Keys()[0], Face: cmp.Or(face, b.Help().Key), Word: word, On: on}
}

// beltKeys are the board's keys on the belt, in its order: home is ◂, the
// chosen card's keys (at most three), ↑ ↓ ⏎; the tail has every other key
// of the board. The consent box leaves ◂, y and esc; the menu ◂ ↑ ↓ ⏎ esc.
// Another consent (the take-over question, the scan's, ⏎'s) is home with no
// card keys and every key of the tail off.
func (m Model) beltKeys() []beltKey {
	bk, mk := m.keys.board, m.keys.main
	home := beltKey{Face: "◂", On: true}
	up, down := beltOf(mk.Up, "↑", "", true), beltOf(mk.Down, "↓", "", true)
	switch {
	case m.yesBox():
		yes := beltOf(m.keys.confirm.Yes, "", "runs", true)
		yes.Act = true
		return []beltKey{home, yes, beltOf(mk.Back, "", "cancels", true)}
	case m.menu != nil:
		return []beltKey{home, up, down, beltOf(mk.Open, "", "do", true), beltOf(mk.Back, "", "close", true)}
	}
	board := m.boardShown() && !m.consenting()
	s, card := m.boardSelected()
	_, above := m.aboveChosen()
	card = board && card && !above
	ks := []beltKey{home}
	if card {
		ks = append(ks, m.cardBelt(s)...)
	}
	_, head := m.headRow(s)
	chains := board && len(m.boardChains()) > 0
	return append(ks, up, down, beltOf(mk.Open, "", "", true),
		beltOf(bk.Copy, "", "copy", card),
		beltOf(bk.Menu, "", "menu", card),
		beltOf(bk.Chain, "", "chain", card && m.chainLabelOf(s) != ""),
		beltOf(bk.Density, "", "one line", board),
		beltOf(bk.Host, "", "hosts", board && m.hostCount() > 1),
		beltOf(bk.Strip, "", "strip", board),
		beltOf(mk.NextTab, "⇥", "machine", !m.consenting()),
		beltOf(mk.Second, "", "kill", card && !head && !isShipCard(s) && m.killAction(s) != nil),
		beltOf(bk.Left, "←", "", board),
		beltOf(bk.Right, "→", "", board),
		beltOf(bk.NextChain, "", "", chains),
		beltOf(bk.PrevChain, "", "", chains),
		beltOf(mk.Search, "", "", board),
		beltOf(mk.Park, "", "park", card && !isShipCard(s)),
	)
}

// cardBelt are the chosen card's keys on the belt: the digits of the
// options it asks (at most three; off on another host, which takes no
// digit), or p for the ship that waits for its merge and y for the go
// that waits.
func (m Model) cardBelt(s sample.Session) []beltKey {
	var ks []beltKey
	if q, answers := m.oneChoice(s); answers {
		if _, sent := m.peekMarks(s); sent == 0 {
			for i := range min(len(q.Options), 3) {
				n := strconv.Itoa(i + 1)
				ks = append(ks, beltKey{Key: n, Face: n, Word: "answer", On: s.Host == m.opt.Host, Act: true})
			}
			return ks
		}
	}
	if _, ok := m.mergeable(s); ok {
		ks = append(ks, beltOf(m.keys.board.Merge, "", "merge", true))
	}
	if m.goWaitsOn(s) {
		ks = append(ks, beltOf(m.keys.main.Grant, "", "go", true))
	}
	for i := range ks {
		ks[i].Act = true
	}
	return ks
}

// beltMax is the most bytes a board= value has: the app gives up on a
// longer one.
const beltMax = 48

// beltValue is what the console tells the app's belt (beltWord): "" off
// the board, "box" under the consent box, "menu" under the menu.
func (m Model) beltValue() string {
	switch {
	case m.standIn || m.viewFull() || m.tab != tabSessions && m.tab != tabMachine:
		return ""
	case m.yesBox():
		return "box"
	case m.menu != nil:
		return "menu"
	}
	armed := ""
	if s, ok := m.boardSelected(); ok && m.boardShown() {
		if a, _ := m.peekMarks(s); a > 0 {
			armed = strconv.Itoa(a)
		} else if m.goArmedOn(s) {
			armed = m.keys.main.Grant.Keys()[0]
		}
	}
	return beltWord(m.beltKeys(), armed)
}

// beltWord is the board= value of the keys ks, armed the card key that a
// first press marked ("" for none). The grammar, in printable ASCII:
//
//	value = card ";" dim
//	card  = "" | key *( "," key )   the card's keys (Act), in order; "*" after the armed one
//	dim   = "" | tok *( "," tok )   the keys that are off, in the belt's order
//	key   = tok [ "*" ]
//
// A tok is a beltKey's Key: 1 2 3 p y up down enter c m g v s u tab x
// left right ] [ / z. A card's key that is off (a digit on another host)
// is a dim tok. ◂ has none and is never sent. At most beltMax bytes:
// dim toks leave whole from the right, then card keys from the right.
// The other values ("" off the board, "box", "menu") come from beltValue.
func beltWord(ks []beltKey, armed string) string {
	var card, dim []string
	for _, k := range ks {
		switch {
		case k.Key == "":
		case k.Act && k.On && k.Key == armed:
			card = append(card, k.Key+"*")
		case k.Act && k.On:
			card = append(card, k.Key)
		case !k.On:
			dim = append(dim, k.Key)
		}
	}
	word := func() string { return strings.Join(card, ",") + ";" + strings.Join(dim, ",") }
	for len(word()) > beltMax && len(dim) > 0 {
		dim = dim[:len(dim)-1]
	}
	for len(word()) > beltMax && len(card) > 0 {
		card = card[:len(card)-1]
	}
	return word()
}

// beltSaid is the board= value the console last sent and the viewers it
// went to; passes: tmux passed it (3.3 and on, or no tmux); n counts the
// non-empty values sent, at the time the last went out. pocket are the
// clients that answered a value (BeltKey): the app on the Board layout.
type beltSaid struct {
	value, to string
	passes    bool
	n         int
	at        time.Time
	pocket    []pocketClient
	wait      int // viewers replies to wait for before an answer counts
}

// pocketClient is a client ("tty:pid", pid "" until the viewers name it)
// and the value it answered last (beltSaid.n).
type pocketClient struct {
	client string
	n      int
}

// beltAnswerWait is how long the app's answer to the value before holds
// while the console waits for the answer to the last one: the key line
// does not flash at each new value.
const beltAnswerWait = time.Second

// tellBelt sends the board's keys to the app when the value changes, and
// again to new viewers; again: a terminal came to show the console. Off
// the board it sends "" once.
func (m Model) tellBelt(again bool) (Model, tea.Cmd) {
	m.belt.pocket = keepPocket(m.belt.pocket, m.viewers)
	v := m.beltValue()
	if v == m.belt.value && (v == "" || m.belt.to == m.viewers && !again) {
		return m, nil
	}
	say := sayAgent(keycmd.BoardSignal(v), m.opt)
	m.belt.value, m.belt.to, m.belt.passes = v, m.viewers, say != nil
	if v != "" {
		m.belt.n, m.belt.at = m.belt.n+1, m.now()
	}
	return m, say
}

// heardFrom is the belt once the client on tty answered the board's keys
// (BeltKey after its mark): it counts while a board= value is out and
// passed.
func (m Model) heardFrom(tty string) beltSaid {
	b := m.belt
	if tty == "" || b.value == "" || !b.passes || b.wait > 0 {
		return b
	}
	client := tty + ":"
	for _, v := range viewerClients(m.viewers) {
		if strings.HasPrefix(v, client) {
			client = v
		}
	}
	b.pocket = append(slices.DeleteFunc(slices.Clone(b.pocket), func(p pocketClient) bool { return p.client == client }), pocketClient{client, b.n})
	return b
}

// waitViewers is a terminal that came (ViewerKey): every answer is
// cleared, and none counts until the console knows its viewers again, so
// a desk that attached keeps the key line. It asks at once; a question
// already out may have its reply from before the terminal came.
func (m Model) waitViewers() (Model, tea.Cmd) {
	m.belt.pocket = nil
	if m.opt.Graphics == nil {
		return m, nil
	}
	m.belt.wait = 1
	if m.gfxBusy {
		m.belt.wait = 2
	}
	return m.askViewers()
}

// beltPoll says, for msg, if the value goes again as a new one: the
// viewers reply a terminal that came waited for, or a viewers poll while
// on Pocket (ruling 22), which an app still on Board answers and one that
// left it does not. Off Pocket an unchanged value never goes again.
func (m *Model) beltPoll(msg tea.Msg) bool {
	if g, ok := msg.(graphicsMsg); !ok || g.gen != m.opt.Gen {
		return false
	}
	if m.belt.wait > 0 {
		m.belt.wait--
		return m.belt.wait == 0
	}
	return m.onPocket()
}

// keepPocket keeps the clients that still view the console: one gone, or
// a new client on its tty, has not answered since it attached.
func keepPocket(pocket []pocketClient, viewers string) []pocketClient {
	if len(pocket) == 0 {
		return pocket
	}
	vs := viewerClients(viewers)
	var kept []pocketClient
	for _, p := range pocket {
		tty, pid, _ := strings.Cut(p.client, ":")
		for _, v := range vs {
			if vtty, vpid, _ := strings.Cut(v, ":"); vtty == tty && (pid == "" || pid == vpid) {
				kept = append(kept, pocketClient{v, p.n})
			}
		}
	}
	return kept
}

// viewerClients are the clients of a viewers key (logos.Viewers): "tty:pid"
// each, where they look left out.
func viewerClients(viewers string) []string {
	var out []string
	for v := range strings.SplitSeq(viewers, ",") {
		if c, _, _ := strings.Cut(v, "@"); c != "" {
			out = append(out, c)
		}
	}
	return out
}

// onPocket: every client on the console's session is the app on its
// Board layout: it answered the last board= value since it attached (or
// the one before, for beltAnswerWait after the last went out). One frame
// goes to every viewer, so a desk among them keeps the key line.
func (m Model) onPocket() bool {
	vs := viewerClients(m.viewers)
	if len(vs) == 0 {
		return false
	}
	b := m.belt
	for _, v := range vs {
		i := slices.IndexFunc(b.pocket, func(p pocketClient) bool { return p.client == v })
		if i < 0 {
			return false
		}
		if n := b.pocket[i].n; n != b.n && (n != b.n-1 || m.now().Sub(b.at) >= beltAnswerWait) {
			return false
		}
	}
	return true
}

// yesBox: the consent box that takes a typed y, and esc, shows: a
// destructive step's.
func (m Model) yesBox() bool {
	return m.consenting() && m.held == nil && !m.scanShown() && m.pending != nil && m.pending.destructive
}

// beltOnly: the belt has the board's keys, and the frame has no key line.
// The menu, the consent box, a search and a lost host keep it.
func (m Model) beltOnly() bool {
	return m.boardShown() && m.onPocket() && !m.consenting() && m.menu == nil && !m.searching && !m.viewFull() && m.lostLine() == ""
}
