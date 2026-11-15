package app

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// sideFocus is where the keys go while the list and the chat show.
type sideFocus int

const (
	focusLine sideFocus = iota // the chat's line: the default
	focusList
)

// lineNote is what the line of the session k says in place of its ⏎:
// why its text did not go, or that the session it was typed for ended.
type lineNote struct {
	k    paneKey
	text string
}

// lineSent is a message on its way: the session it went to, and its text,
// which comes back to the line when the service refuses it.
type lineSent struct {
	k          paneKey
	name, text string
}

// sideActive: the Sessions tab is the list and the chat, and its keys go
// to the line or the list before the board's.
func (m Model) sideActive() bool {
	return m.sideOn && m.boardShown() && !m.standIn && !m.viewFull()
}

// sideChosen is the session the list chooses.
func (m Model) sideChosen() (sample.Session, bool) {
	k := m.chosenSide()
	listed := m.sideSessions()
	i := slices.IndexFunc(listed, func(s sample.Session) bool { return paneOf(s) == k })
	if k.pane == "" || i < 0 {
		return sample.Session{}, false
	}
	return listed[i], true
}

// sideKeep keeps the choice on its session while the list moves. The
// first choice is the first session drawn. When the chosen session ends,
// the choice moves to the next one as drawn, and a draft of the one that
// ended stays under its own name, unsent.
func (m Model) sideKeep() Model {
	if !m.sideActive() {
		return m
	}
	order := m.sideOrder()
	at := m.sideAt
	if at != (paneKey{}) && !slices.ContainsFunc(m.sideSessions(), func(s sample.Session) bool { return paneOf(s) == at }) {
		m.sideAt = m.sideAfter(at, order)
		if strings.TrimSpace(m.lines[at]) != "" && m.sideAt != (paneKey{}) {
			m.lineNote = lineNote{m.sideAt, m.sideName + " ended · the text is not sent"}
		}
	}
	if m.sideAt == (paneKey{}) {
		m.sideAt = m.chosenSide()
	}
	m.sideWas = order
	if s, ok := m.sideChosen(); ok {
		m.sideName = m.cardName(s)
	}
	return m
}

// sideAfter is the session drawn after at, as the list last drew it, that
// order still draws; else the one before it; else none.
func (m Model) sideAfter(at paneKey, order []paneKey) paneKey {
	i := slices.Index(m.sideWas, at)
	if i < 0 {
		return paneKey{}
	}
	for _, k := range m.sideWas[i+1:] {
		if slices.Contains(order, k) {
			return k
		}
	}
	for j := i - 1; j >= 0; j-- {
		if slices.Contains(order, m.sideWas[j]) {
			return m.sideWas[j]
		}
	}
	return paneKey{}
}

// sideKey is k while the list and the chat show: the line or the list
// takes it, and the menu over them takes every key. false: the board and
// the tabs take k.
func (m Model) sideKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if !m.sideActive() {
		return m, nil, false
	}
	if m.menu != nil {
		next, cmd := m.menuKey(k)
		return next, cmd, true
	}
	if bkey.Matches(k, m.keys.main.Back) && m.waiting() { // the record wait takes esc first
		return m, m.switchNow(), true
	}
	m.flash = ""
	s, ok := m.sideChosen()
	if m.keysIn == focusList {
		next, cmd := m.listKey(k, s, ok)
		return next, cmd, true
	}
	next, cmd := m.lineKey(k, s, ok)
	return next, cmd, true
}

// lineKey is k in the line: printable keys type, ⏎ sends, and with the
// line empty a digit answers the question on screen and the arrows choose.
func (m Model) lineKey(k tea.KeyPressMsg, s sample.Session, ok bool) (tea.Model, tea.Cmd) {
	sk, mk := m.keys.side, m.keys.main
	text := m.lines[paneOf(s)]
	var cmd tea.Cmd
	switch {
	case bkey.Matches(k, sk.Leave):
		if ok && m.chatMark(s) > 0 {
			m.peekArmed = 0
		} else {
			m.keysIn = focusList
		}
	case bkey.Matches(k, sk.Enter):
		cmd = m.sendLine()
	case bkey.Matches(k, sk.Erase):
		if r := []rune(text); len(r) > 0 {
			m.setLine(paneOf(s), string(r[:len(r)-1]))
		}
	case bkey.Matches(k, sk.Up, sk.Down) && text != "":
		cmd = m.notice("the line holds text · " + sk.Leave.Help().Key + " goes to the list")
	case bkey.Matches(k, sk.Up):
		m.sideMove(-1)
	case bkey.Matches(k, sk.Down):
		m.sideMove(1)
	case ok && m.parked(s) && bkey.Matches(k, mk.Park):
		return m.onCard(s).key(k)
	case ok && isDigit(k) && text == "" && m.digitAnswers(s):
		if _, held := m.stokerHolds(s); held {
			cmd = m.releaseDigit(s, k.Text)
			break
		}
		return m.answerFromChat(k.Text)
	case printsText(k):
		m = m.linePaste(k.Text)
	case bkey.Matches(k, mk.NextTab, mk.PrevTab, mk.Jump):
		return m.key(k)
	}
	return m, cmd
}

// listKey is k in the list: ↑↓ choose, ⏎ goes back to the line, the
// board's keys act on the chosen session, and any other printable key
// goes back to the line and is typed there.
func (m Model) listKey(k tea.KeyPressMsg, s sample.Session, ok bool) (tea.Model, tea.Cmd) {
	sk, mk, bk := m.keys.side, m.keys.main, m.keys.board
	var cmd tea.Cmd
	switch {
	case bkey.Matches(k, sk.Up):
		m.sideMove(-1)
	case bkey.Matches(k, sk.Down):
		m.sideMove(1)
	case bkey.Matches(k, sk.Enter):
		m.keysIn = focusLine
	case bkey.Matches(k, sk.Leave):
		switch {
		case m.filter != "":
			m.filter, m.peekAt = "", m.now()
			m.refresh()
		case m.room != nil:
			m = m.closeRoom()
		case m.boardChain != "":
			m.boardChain, m.peekAt = "", m.now()
		}
	case bkey.Matches(k, bk.Host):
		m.turnHost()
	case bkey.Matches(k, bk.Chain):
		cmd = m.boardChainOf(s, ok)
	case bkey.Matches(k, bk.Menu) && ok:
		m = m.openMenuOnChosen(s)
	case bkey.Matches(k, bk.Copy):
		cmd = m.boardCopy(s, ok)
	case bkey.Matches(k, mk.Park) && ok:
		return m.onCard(s).key(k)
	case bkey.Matches(k, mk.New, mk.Search, mk.NextTab, mk.PrevTab, mk.Jump):
		return m.key(k)
	case printsText(k):
		m.keysIn = focusLine
		return m.lineKey(k, s, ok)
	}
	return m, cmd
}

// printsText: k prints text, with no ctrl or alt.
func printsText(k tea.KeyPressMsg) bool {
	return k.Text != "" && k.Mod&(tea.ModCtrl|tea.ModAlt) == 0
}

// sideMove chooses the session d places down the list as drawn.
func (m *Model) sideMove(d int) {
	order := m.sideOrder()
	if len(order) == 0 {
		return
	}
	m.sideAt = order[clampMove(slices.Index(order, m.chosenSide()), d, len(order))]
	if s, ok := m.sideChosen(); ok {
		m.startChatMark(s)
		m.sideName = m.cardName(s)
	}
}

// setLine puts text in the line of k.
func (m *Model) setLine(k paneKey, text string) {
	m.lines = maps.Clone(m.lines)
	if m.lines == nil {
		m.lines = map[paneKey]string{}
	}
	if text == "" {
		delete(m.lines, k)
	} else {
		m.lines[k] = text
	}
	if m.lineNote.k == k {
		m.lineNote = lineNote{}
	}
}

// linePaste adds text to the chosen session's line, as one piece. A
// parked session's line is off: it says so, and takes nothing.
func (m Model) linePaste(text string) Model {
	m.keysIn = focusLine
	s, ok := m.sideChosen()
	if !ok || m.parked(s) {
		return m
	}
	m.setLine(paneOf(s), m.lines[paneOf(s)]+text)
	return m
}

// parked: the owner parked s.
func (m Model) parked(s sample.Session) bool {
	h, held := m.holdOf(s)
	return held && h.By == holdByYou
}

// stokerHolds is the stoker's hold on s while s asks nothing of its own:
// the chat asks whether to let it go.
func (m Model) stokerHolds(s sample.Session) (proto.Hold, bool) {
	h, held := m.holdOf(s)
	return h, held && h.By != holdByYou && !m.asksNow(s)
}

// digitAnswers: a digit in the empty line answers s. Its question is on
// screen, and it has been for peekDead: a digit typed as a question
// opens is text.
func (m Model) digitAnswers(s sample.Session) bool {
	_, held := m.stokerHolds(s)
	if _, asks := m.oneChoice(s); !asks && !held {
		return false
	}
	return paneOf(s) == m.peekPane && askOf(s) == m.peekAsk && m.now().Sub(m.peekAt) >= peekDead
}

// chatMark is the option a first digit marked on s, 0 for none.
func (m Model) chatMark(s sample.Session) int {
	if _, held := m.stokerHolds(s); held {
		if paneOf(s) == m.peekPane && m.peekAsk == "" && m.peekArmedAt.After(m.peekAt) {
			return m.peekArmed
		}
		return 0
	}
	armed, _ := m.chatMarks(s)
	return armed
}

// releaseDigit is a digit on a session the stoker holds: 1 marks "let it
// go", and 1 again sends the release.
func (m *Model) releaseDigit(s sample.Session, digit string) tea.Cmd {
	switch {
	case digit != "1":
		return m.notice("1 only")
	case m.chatMark(s) != 1:
		m.peekArmed, m.peekArmedAt = 1, m.now()
		return nil
	}
	if why := m.cannotReach(s.Host); why != "" {
		return m.notice(why)
	}
	m.peekArmed = 0
	return tea.Batch(m.sendTo(s.Host, proto.Act{RecID: "release:" + s.Pane, Confirm: "y"}), m.say("let go: "+m.cardName(s)))
}

// cannotReach says why nothing can go to host now: the service is lost, or
// the host is not reached. "" when an act can go.
func (m Model) cannotReach(host string) string {
	switch {
	case m.watchConn(host) != nil:
		return ""
	case host == m.opt.Host:
		return "not sent: no service · it redials"
	}
	return "not sent: " + m.nameOf(host) + " is not reached"
}

// sendLine sends the chosen session's line as a message, on its host's
// conn. A line that cannot go keeps its text and says why; nothing
// retries by itself.
func (m *Model) sendLine() tea.Cmd {
	s, ok := m.sideChosen()
	if !ok {
		return m.notice("no session to write to")
	}
	k := paneOf(s)
	text := m.lines[k]
	switch {
	case m.parked(s):
		return nil // the line says it is off, and why
	case strings.TrimSpace(text) == "":
		return m.notice("the line is empty")
	}
	if why := m.cannotReach(s.Host); why != "" {
		m.lineNote = lineNote{k, why}
		return nil
	}
	cmd := m.sendTo(s.Host, proto.Act{RecID: "message:" + s.Pane, Text: text, Confirm: "y"})
	m.lineActs = maps.Clone(m.lineActs)
	if m.lineActs == nil {
		m.lineActs = map[string]lineSent{}
	}
	m.lineActs["a"+strconv.Itoa(m.acts)] = lineSent{k, m.cardName(s), text}
	m.setLine(k, "")
	return cmd
}

// lineResult is the service's answer to a message of the line: a refusal
// puts the text back, when the line is still empty, and the line says
// the service's words.
func (m Model) lineResult(r proto.Result) (Model, tea.Cmd, bool) {
	sent, ok := m.lineActs[r.ActID]
	if !ok {
		return m, nil, false
	}
	m.lineActs = maps.Clone(m.lineActs)
	delete(m.lineActs, r.ActID)
	why := r.Err
	if why == "" && len(r.Skipped) > 0 {
		why = r.Skipped[len(r.Skipped)-1]
	}
	if why == "" {
		return m, m.say("sent to " + sent.name), true
	}
	if m.lines[sent.k] == "" {
		m.setLine(sent.k, sent.text)
	}
	m.lineNote = lineNote{sent.k, "not sent: " + why}
	return m, nil, true
}

// lineShown is a draft as the line draws it: a newline as ↵, and no
// control character reaches the terminal.
func lineShown(text string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			return '↵'
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, text)
}

// sideKeyWords is the key line of the list and the chat: where the keys
// go, then at most five keys for that place.
func (m Model) sideKeyWords() string {
	sk, mk, bk := m.keys.side, m.keys.main, m.keys.board
	hint := func(b bkey.Binding, desc string) string {
		return b.Help().Key + " " + cmp.Or(desc, b.Help().Desc)
	}
	if m.keysIn == focusList {
		return strings.Join([]string{"in the list", hint(sk.Up, ""), hint(sk.Enter, "write"), hint(bk.Host, ""), hint(bk.Chain, ""), hint(bk.Menu, "")}, " · ")
	}
	name := "no session"
	if s, ok := m.sideChosen(); ok {
		name = m.cardName(s)
	}
	return strings.Join([]string{"writing to " + name, hint(sk.Enter, ""), hint(sk.Up, ""), hint(sk.Leave, ""), hint(mk.Jump, ""), hint(mk.NextTab, "Machine")}, " · ")
}
