package app

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

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
	k                paneKey
	host, name, text string
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
	if !m.have && m.opt.Layered && bkey.Matches(k, m.keys.main.Back) {
		return m, nil, false // a host that never answered: esc goes back to Hosts
	}
	if m.newer != "" && bkey.Matches(k, m.keys.main.StartAgain) && m.lines[m.chosenSide()] == "" {
		return m, nil, false // the key line offers it first: R starts the console again
	}
	m.flash, m.sideIn = "", paneKey{}
	s, ok := m.sideChosen()
	switch {
	case bkey.Matches(k, m.keys.side.HideList):
		return m.flipList(), nil, true
	case bkey.Matches(k, m.keys.main.Jump):
		next, cmd := m.expand(s, ok)
		return next, cmd, true
	case m.sidePlace != placeChat:
		next, cmd := m.placeKey(k)
		return next, cmd, true
	case m.inList():
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
			m.toList()
		}
	case bkey.Matches(k, sk.List):
		m.toList()
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
		return m.linePaste(k.Text)
	case bkey.Matches(k, mk.NextTab, mk.PrevTab):
		return m.key(k)
	default:
		cmd = m.notice("the line takes text · " + sk.Leave.Help().Key + " gives the keys to the list")
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
	case m.footAt > 0 && bkey.Matches(k, mk.Park, bk.Copy, bk.Chain, bk.Menu):
		cmd = m.notice(strings.Join([]string{mk.Park.Help().Key, bk.Copy.Help().Key, bk.Chain.Help().Key, bk.Menu.Help().Key}, " ") +
			" act on a session · " + sk.Up.Help().Key + " chooses one")
	case bkey.Matches(k, sk.Up) && m.footAt > 0:
		m.footAt--
	case bkey.Matches(k, sk.Down) && m.footAt > 0:
		m.footAt = min(m.footAt+1, len(m.footParts()))
	case bkey.Matches(k, sk.Down) && m.onLastRow():
		m.footAt = 1 // past the last row: the foot's first part
	case bkey.Matches(k, sk.Up) && len(m.sideOrder()) == 0:
		cmd = m.notice("no session to choose")
	case bkey.Matches(k, sk.Up):
		m.sideMove(-1)
	case bkey.Matches(k, sk.Down):
		m.sideMove(1)
	case bkey.Matches(k, sk.Enter) && m.footAt > 0:
		parts := m.footParts()
		return m.openFoot(parts[min(m.footAt, len(parts))-1].press)
	case bkey.Matches(k, sk.Enter):
		m.toLine()
	case bkey.Matches(k, mk.Exited):
		cmd = m.flipExited()
	case bkey.Matches(k, sk.Leave):
		switch {
		case m.filter != "":
			m.filter, m.peekAt = "", m.now()
			m.refresh()
		case m.room != nil:
			m = m.closeRoom()
		case m.boardChain != "":
			m.boardChain, m.peekAt = "", m.now()
		default:
			cmd = m.listTakes()
		}
	case bkey.Matches(k, bk.Host):
		if said := m.turnHost(); said != "" {
			cmd = m.notice(said)
		}
	case bkey.Matches(k, sk.Unhide):
		cmd = m.unhide()
	case bkey.Matches(k, bk.Chain):
		cmd = m.boardChainOf(s, ok)
	case bkey.Matches(k, bk.Menu) && ok:
		m = m.openMenuOnChosen(s)
	case bkey.Matches(k, bk.Copy):
		cmd = m.boardCopy(s, ok)
	case bkey.Matches(k, mk.Park) && ok:
		return m.onCard(s).key(k)
	case bkey.Matches(k, mk.New, mk.Search, mk.NextTab, mk.PrevTab):
		return m.key(k)
	case printsText(k):
		m.toLine()
		return m.lineKey(k, s, ok)
	default:
		cmd = m.listTakes()
	}
	return m, cmd
}

// listTakes is the notice of a key the list has no job for: the keys it
// takes.
func (m *Model) listTakes() tea.Cmd {
	sk, mk, bk := m.keys.side, m.keys.main, m.keys.board
	var keys []string
	for _, b := range []bkey.Binding{sk.Up, sk.Enter, bk.Host, bk.Chain, mk.Search, bk.Menu, bk.Copy, mk.Park, mk.New, mk.Exited, mk.NextTab} {
		keys = append(keys, b.Help().Key)
	}
	return m.notice("the list takes " + strings.Join(keys, " ") + " · a letter types in the line")
}

// printsText: k prints text, with no ctrl or alt.
func printsText(k tea.KeyPressMsg) bool {
	return k.Text != "" && k.Mod&(tea.ModCtrl|tea.ModAlt) == 0
}

// sideMove chooses the session d places down the list as drawn: a folded
// group's rows are not drawn, so the walk passes them.
func (m *Model) sideMove(d int) {
	walk := m.sideWalk()
	if len(walk) == 0 {
		return
	}
	at := slices.Index(walk, m.chosenSide())
	if at < 0 { // the chosen row is folded away: the walk goes on from its place
		order := m.sideOrder()
		i := slices.Index(order, m.chosenSide())
		at = len(walk) - 1
		if d > 0 {
			at = 0
		}
		for j, k := range walk {
			if p := slices.Index(order, k); d > 0 && p > i || d < 0 && p < i {
				at = j
				if d > 0 {
					break
				}
			}
		}
		d = 0
	}
	m.sideAt = walk[clampMove(at, d, len(walk))]
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
func (m Model) linePaste(text string) (tea.Model, tea.Cmd) {
	m.toLine()
	s, ok := m.sideChosen()
	switch {
	case !ok:
		return m, m.notice("no session to write to · " + m.keys.main.New.Help().Key + " starts one")
	case m.parked(s):
		return m, nil // the line says it is off, and why
	}
	m.setLine(paneOf(s), m.lines[paneOf(s)]+text)
	return m, nil
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

// typesWhileBusy: the agent takes a message typed while it works and
// queues it for its next turn. The values come from each agent's
// documented behaviour: Claude Code and Codex queue a message typed while
// they work, and OpenCode is unknown, so the console holds its messages.
// They are NOT checked by hand yet. An agent not listed counts as false.
var typesWhileBusy = map[string]bool{"claude": true, "codex": true, "opencode": false}

const (
	queuedNote  = "queued for its next turn"
	oneHeldNote = "one message already waits for its next turn"
)

// sendLine sends the chosen session's line as a message, on its host's
// conn. A line that cannot go keeps its text and says why; nothing
// retries by itself. To a session that works and does not take typed
// input, the message waits for the end of its turn.
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
	busy := m.sideStateOf(s) == sideWork
	if busy && !typesWhileBusy[s.Agent] {
		if _, waits := m.lineHeld[k]; waits {
			m.lineNote = lineNote{k, oneHeldNote}
			return nil
		}
		m.lineHeld = maps.Clone(m.lineHeld)
		if m.lineHeld == nil {
			m.lineHeld = map[paneKey]heldMsg{}
		}
		m.lineHeld[k] = heldMsg{text, m.cardName(s)}
		m.setLine(k, "")
		return nil
	}
	cmd, sent := m.sendText(s, text)
	if busy && sent {
		m.lineNote = lineNote{k, queuedNote}
	}
	return cmd
}

// heldMsg is the message a busy session waits to get, and the session's
// name for when it ends first.
type heldMsg struct{ text, name string }

// maxMessage is the longest text a message: act takes, in characters.
const maxMessage = 500

// lineAct is the act that carries text: a paste for a text with a newline
// or one longer than a message, else a message.
func lineAct(text string) string {
	if strings.Contains(text, "\n") || utf8.RuneCountInString(text) > maxMessage {
		return proto.PastePrefix
	}
	return "message:"
}

// sendText sends text to s as a message and empties the line of s, or
// says why it cannot go and keeps the text there. sent: the act went.
func (m *Model) sendText(s sample.Session, text string) (cmd tea.Cmd, sent bool) {
	k := paneOf(s)
	if why := m.cannotReach(s.Host); why != "" {
		if m.lines[k] == text { // still in the line: it stays there
			m.lineNote = lineNote{k, why}
			return nil, false
		}
		m.giveBack(lineSent{k, s.Host, m.cardName(s), text}, why)
		return nil, false
	}
	id, cmd := m.sendToID(s.Host, proto.Act{RecID: lineAct(text) + s.Pane, Text: text, Confirm: "y"})
	m.lineActs = maps.Clone(m.lineActs)
	if m.lineActs == nil {
		m.lineActs = map[string]lineSent{}
	}
	m.lineActs[id] = lineSent{k, s.Host, m.cardName(s), text}
	if m.lines[k] == text {
		m.setLine(k, "")
	}
	return cmd, true
}

// heldRelease sends each held message whose session has left "works", as
// sendLine does. A session the stoker holds keeps the message waiting. A
// session the owner parked, or one that is gone, gives the text back to
// its line, unsent. A "queued" note ends with the turn it was said for.
func (m Model) heldRelease() (Model, tea.Cmd) {
	var cmds []tea.Cmd
	for k, h := range m.lineHeld {
		i := slices.IndexFunc(m.all.sessions, func(s sample.Session) bool { return paneOf(s) == k })
		if i >= 0 {
			s := m.all.sessions[i]
			if by, held := m.holdOf(s); (held && by.By != holdByYou) || m.sideStateOf(s) == sideWork {
				continue
			}
		}
		m.lineHeld = maps.Clone(m.lineHeld)
		delete(m.lineHeld, k)
		if i >= 0 && !m.parked(m.all.sessions[i]) {
			c, _ := m.sendText(m.all.sessions[i], h.text)
			cmds = append(cmds, c)
			continue
		}
		note := h.name + " ended · the text is not sent"
		if i >= 0 {
			note = "parked: " + m.keys.main.Park.Help().Key + " lets it go"
		}
		m.setLine(k, strings.TrimSpace(h.text+" "+m.lines[k]))
		if m.lineNote.k == (paneKey{}) || m.lineNote.k == k {
			m.lineNote = lineNote{k, note}
		}
	}
	if m.lineNote.text == queuedNote {
		if i := slices.IndexFunc(m.all.sessions, func(s sample.Session) bool { return paneOf(s) == m.lineNote.k }); i < 0 || m.sideStateOf(m.all.sessions[i]) != sideWork {
			m.lineNote = lineNote{}
		}
	}
	return m, tea.Batch(cmds...)
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
	m.giveBack(sent, "not sent: "+why)
	return m, nil, true
}

// giveBack puts the text of a message that did not go back in its line,
// before what the owner typed since, and the line says why.
func (m *Model) giveBack(sent lineSent, why string) {
	m.setLine(sent.k, strings.TrimSpace(sent.text+" "+m.lines[sent.k]))
	m.lineNote = lineNote{sent.k, why}
}

// lineLost gives back each message to host that has no answer: its conn
// went, and the answer with it.
func (m Model) lineLost(host string) Model {
	for id, sent := range m.lineActs {
		if sent.host != host {
			continue
		}
		m.lineActs = maps.Clone(m.lineActs)
		delete(m.lineActs, id)
		m.giveBack(sent, cmp.Or(m.cannotReach(host), "not sent: "+m.nameOf(host)+" is not reached"))
	}
	return m
}

// answerResult is a peer's answer to the act that carried a digit: a
// refusal gives the digits back, and the notice says why.
func (m Model) answerResult(r proto.Result) (Model, tea.Cmd, bool) {
	if r.ActID == "" || r.ActID != m.peekSentAct {
		return m, nil, false
	}
	m.peekSentAct = ""
	if r.Err == "" {
		return m, nil, true
	}
	m.peekSent, m.peekSentAsk, m.peekSentPane = 0, "", ""
	return m, m.notice("not sent: " + r.Err), true
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
