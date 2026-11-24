package app

import (
	"cmp"
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

// sideFocus is where the keys go while the list and the chat show.
type sideFocus int

const (
	focusLine sideFocus = iota // the chat's line: the default
	focusList
)

// agentKeys routes the console's input while the agent's screen shows on
// the desk. With the agent's keys, every key, a paste and a click in its
// region go to its screen; ctrl+q and the home key (ctrl+a m) give them
// back to the list. From the list, ctrl+q and a click in the region give
// the agent the keys, and its session counts as seen (design review D5).
// A drag in the region selects its text, which c copies; any other key
// ends the selection and goes on. From the list, c copies what the agent
// asked to copy. The wheel in the region goes to the agent's tmux (design
// review D17). A question or a sheet open over the desk takes the keys
// first. A tap on Pocket's belt is the list's, whoever has the keyboard's.
// ok is false for what the list handles.
func (m Model) agentKeys(msg tea.Msg) (Model, tea.Cmd, bool) {
	r := &m.agent
	k, _ := msg.(tea.KeyPressMsg)
	if f := m.focus(k); !r.shown() || m.tapping || f < layerTakeOver || f == layerScan || f == layerBlocked || f == layerPending {
		return m, nil, false
	}
	agent := r.mode() == "AGENT"
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		copyKey := bkey.Matches(msg, m.keys.board.Copy)
		if sel := r.sel; sel != nil {
			r.sel = nil
			if copyKey {
				cmd := m.copyAgentText(r.term.text(*sel, r.w))
				return m, cmd, true
			}
		}
		switch {
		case copyKey && !agent && r.copy != "":
			text := copyable(r.copy)
			r.copy = ""
			cmd := m.copyAgentText(text)
			return m, cmd, true
		case msg.String() == flipKey && agent:
			r.keys = false
		case msg.String() == flipKey:
			r.keys = true
			return m, m.grantKeys(), true
		case agent:
			r.term.scr.sendKey(uv.KeyPressEvent(msg))
		default:
			return m, nil, false
		}
		return m, nil, true
	case uv.UnknownApcEvent:
		if !agent || string(msg) != homeKey {
			return m, nil, false
		}
		r.keys = false
		return m, nil, true
	case tea.PasteMsg:
		if !agent {
			return m, nil, false
		}
		r.term.scr.paste(msg.Content)
		return m, nil, true
	case tea.MouseClickMsg:
		at, in := r.at(msg.Mouse())
		if !in {
			if agent && msg.X < r.x { // a click on the list gives it the keys
				r.keys = false
			}
			return m, nil, false
		}
		r.sel, r.press = nil, &point{at.X, at.Y}
		r.term.scr.sendMouse(uv.MouseClickEvent(at))
		if agent {
			return m, nil, true
		}
		r.keys = true
		return m, m.grantKeys(), true
	case tea.MouseMotionMsg:
		ms := msg.Mouse()
		if r.press == nil || ms.Button != tea.MouseLeft || ms.Mod.Contains(tea.ModShift) {
			return m, nil, false
		}
		if to := r.clamped(ms); r.sel != nil || to != *r.press {
			r.sel = &[2]point{*r.press, to}
			r.selText = r.term.text(*r.sel, r.w)
		}
		return m, nil, true
	case tea.MouseReleaseMsg:
		dragged := r.press != nil && r.sel != nil
		r.press = nil
		at, in := r.at(msg.Mouse())
		switch {
		case dragged: // the drag is matchblox's: the agent's tmux gets no release
			return m, nil, true
		case !agent || !in:
			return m, nil, false
		}
		r.term.scr.sendMouse(uv.MouseReleaseEvent(at))
		return m, nil, true
	case tea.MouseWheelMsg:
		at, in := r.at(msg.Mouse())
		if !in {
			return m, nil, false
		}
		r.term.scr.sendMouse(uv.MouseWheelEvent(at))
		return m, nil, true
	}
	return m, nil, false
}

// copyAgentText puts text from the agent's screen on the clipboard by the
// console's clip.
func (m *Model) copyAgentText(text string) tea.Cmd {
	if text == "" {
		return nil
	}
	return m.copyText(text, characters(utf8.RuneCountInString(text)))
}

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

// sideActive: the Sessions tab shows the list and the chat, and its keys
// go to the line or the list.
func (m Model) sideActive() bool {
	return m.boardShown() && !m.standIn
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
	gone := !slices.ContainsFunc(m.sideSessions(), func(s sample.Session) bool { return paneOf(s) == at })
	if at.waits() { // a waiting row is no session: it stays while the list draws it
		gone = !slices.Contains(order, at)
	}
	if at != (paneKey{}) && gone {
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
	if m.newer != "" && bkey.Matches(k, m.keys.main.StartAgain) && !m.rTypes() {
		if !m.startsAgain {
			return m, m.notice(m.newer), true
		}
		next, cmd := m.startAgain()
		return next, cmd, true // in the line R types
	}
	m.flash, m.sideIn = "", paneKey{}
	s, ok := m.sideChosen()
	switch {
	case bkey.Matches(k, m.keys.side.HideList):
		return m.flipList(), nil, true
	case bkey.Matches(k, m.keys.main.Jump):
		next, cmd := m.expand(s, ok)
		return next, cmd, true
	case m.sidePlace == placeRoom:
		next, cmd := m.roomPlaceKey(k)
		return next, cmd, true
	case m.tapping && m.sidePlace == placeChat: // a tap on the belt acts as in the list; the line keeps the keys
		next, cmd := m.listKey(k, s, ok)
		return next, cmd, true
	case m.sidePlace == placeChat && (m.onPocket() || m.pocketClient(m.askedBy)) && joinedEsc(k):
		// tmux joins an Esc and a letter typed within its escape-time into
		// alt+letter, and the phone has no alt key: the letter types.
		m.toLine()
		next, cmd := m.lineKey(letterOf(k), s, ok)
		return next, cmd, true
	case bkey.Matches(k, m.keys.room.Flip):
		next, cmd := m.flipHere()
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
// No letter acts on a session here: a line that takes no text says why,
// and the list keys act.
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
		cmd = m.lineTakes()
	}
	return m, cmd
}

// lineTakes is the notice of a key the line has no job for.
func (m *Model) lineTakes() tea.Cmd {
	return m.notice("the line takes text · " + m.keys.side.Leave.Help().Key + " gives the keys to the list")
}

// listKey is k in the list: ↑↓ choose, ⏎ goes back to the line, the
// board's keys act on the chosen session, and any other printable key
// goes back to the line and is typed there.
func (m Model) listKey(k tea.KeyPressMsg, s sample.Session, ok bool) (tea.Model, tea.Cmd) {
	sk, mk, bk := m.keys.side, m.keys.main, m.keys.board
	var cmd tea.Cmd
	switch {
	case m.footAt > 0 && bkey.Matches(k, append([]bkey.Binding{mk.Park, bk.Copy, bk.Chain, bk.Menu}, m.rowActKeys()...)...):
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
		cmd = m.previewSoon()
	case bkey.Matches(k, sk.Down):
		m.sideMove(1)
		cmd = m.previewSoon()
	case bkey.Matches(k, sk.Enter) && m.footAt > 0:
		parts := m.footParts()
		return m.openFoot(parts[min(m.footAt, len(parts))-1].press)
	case bkey.Matches(k, sk.Enter) && m.chosenSide().waits():
		cmd = m.reopenChosen()
	case bkey.Matches(k, sk.Enter) && m.besideMode():
		return m.showChosen()
	case bkey.Matches(k, sk.Enter):
		m.toLine()
		cmd = m.grantKeys()
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
		case m.space != "":
			m.space, m.peekAt = "", m.now()
		case m.boardHost != "":
			m.boardHost, m.peekAt = "", m.now()
		default:
			cmd = m.listTakes()
		}
	case bkey.Matches(k, bk.Menu) && (ok || m.chosenSide().waits()):
		m = m.openMenuOnChosen()
	case !ok && bkey.Matches(k, append(m.rowActKeys(), bk.Menu, bk.Copy, mk.Park)...):
		cmd = m.notice(m.noRowWhy())
	case bkey.Matches(k, bk.Host):
		if said := m.turnHost(); said != "" {
			cmd = m.notice(said)
		}
	case bkey.Matches(k, bk.SpaceBefore, bk.SpaceAfter):
		by := 1
		if bkey.Matches(k, bk.SpaceBefore) {
			by = -1
		}
		if said := m.turnSpace(by); said != "" {
			cmd = m.notice(said)
		}
	case bkey.Matches(k, sk.Unhide):
		cmd = m.unhide()
	case bkey.Matches(k, bk.Keys) && m.desk():
		m.over = "keys"
	case bkey.Matches(k, bk.Machine) && m.desk():
		m = m.openCog("cog:usage")
	case bkey.Matches(k, bk.Chain):
		cmd = m.boardChainOf(s, ok)
	case bkey.Matches(k, bk.Copy):
		cmd = m.boardCopy(s, ok)
	case bkey.Matches(k, mk.Park) && ok:
		return m.onCard(s).key(k)
	case bkey.Matches(k, m.rowActKeys()...) && ok:
		return m.rowAct(k, s)
	case bkey.Matches(k, mk.New, mk.Search, mk.NextTab, mk.PrevTab):
		return m.key(k)
	case printsText(k) && m.besideMode():
		said := "the agent takes text"
		if ok {
			said += " · " + sk.Enter.Help().Key + " goes into " + m.cardName(s)
		}
		cmd = m.notice(said)
	case printsText(k):
		m.toLine()
		return m.lineKey(k, s, ok)
	default:
		cmd = m.listTakes()
	}
	return m, cmd
}

// reopenChosen sends recover for the chosen member that waits for it. The
// owner chose this one match, so it sends with no confirm. It names the
// conversation, as JSON: the record knows no window name, and a tmux
// session's name would start each agent of that session.
func (m *Model) reopenChosen() tea.Cmd {
	v, c, ok := m.sideWaiting()
	if !ok {
		return nil
	}
	host := cmp.Or(c.host, m.opt.Host)
	if why := m.refusal(host); why != "" {
		return m.notice(why)
	}
	text, _ := json.Marshal(proto.Recover{Conversations: []string{v.Wait.Conversation}})
	m.reopening = map[string]string{v.Wait.Conversation: v.Wait.Name}
	cmd := m.sendTo(host, proto.Act{RecID: recoverID, Confirm: "y", Text: string(text)})
	return tea.Batch(cmd, m.notice("reopening "+v.Wait.Name))
}

// noRowWhy says why the list has no chosen session for a key to act on.
func (m Model) noRowWhy() string {
	v, _, waits := m.sideWaiting()
	switch {
	case waits:
		return v.Wait.Name + " waits for recover · matchblox recover reopens it"
	case m.filter != "":
		return `nothing matches "` + m.filter + `" · ` + m.keys.main.Back.Help().Key + " clears"
	case m.space != "":
		return m.spaceEmpty()
	case m.boardHost != "":
		return "no match on " + m.boardHost + " · " + m.keys.board.Host.Help().Key + " turns the host"
	}
	return "no session to act on · " + m.keys.main.New.Help().Key + " starts one"
}

// beltActs are the keys a tap on Pocket's belt acts with (keycmd.TapKeys).
func (m Model) beltActs() []bkey.Binding {
	bk, mk := m.keys.board, m.keys.main
	return []bkey.Binding{bk.Copy, bk.Menu, bk.Chain, bk.Host, bk.Merge, mk.Second, mk.Search, mk.Park, mk.Grant, mk.New}
}

// beltTap is a tap on an act key of Pocket's belt, from the client on tty
// (keycmd.ParseTapped). The tap names its client, so it acts for the app
// on its Board layout even with a desk attached. It acts as the list's key
// while the line has the keys; a typed key never acts so.
func (m Model) beltTap(key, tty string) (tea.Model, tea.Cmd) {
	r := []rune(key)
	if len(r) != 1 || !bkey.Matches(tea.KeyPressMsg{Code: r[0], Text: key}, m.beltActs()...) {
		return m, m.notice("the belt sent a key the console does not know: " + key)
	}
	if !m.pocketClient(tty) {
		return m, m.notice("a belt tap from a terminal that is not on its Board layout")
	}
	m.marked, m.tapping = clientMark{tty, m.now()}, true
	next, cmd := m.Update(tea.KeyPressMsg{Code: r[0], Text: key})
	if nm, ok := next.(Model); ok {
		nm.tapping = false
		next = nm
	}
	return next, cmd
}

// joinedEsc: k is alt with a letter, as tmux passes an Esc and a quick
// letter after it.
func joinedEsc(k tea.KeyPressMsg) bool {
	return k.Mod&^tea.ModShift == tea.ModAlt && unicode.IsPrint(letterOf(k).Code)
}

// letterOf is the letter of k with no alt: the shifted one with shift.
func letterOf(k tea.KeyPressMsg) tea.KeyPressMsg {
	code := k.Code
	if k.Mod&tea.ModShift != 0 && k.ShiftedCode != 0 {
		code = k.ShiftedCode
	}
	return tea.KeyPressMsg{Code: code, Text: string(code)}
}

// rowActKeys are the keys that act on the chosen row in the list, as they
// did on the board's chosen card.
func (m Model) rowActKeys() []bkey.Binding {
	mk := m.keys.main
	return []bkey.Binding{mk.Second, mk.Rename, mk.Switch, m.keys.board.Merge, mk.Grant, mk.Stoke, mk.StokeChain, mk.Order, mk.Night}
}

// rowAct is k, one of rowActKeys, on the chosen row s: the kill and the
// merge wait in the consent box, a go that a tap armed waits for the
// second, and the forms open on s.
func (m Model) rowAct(k tea.KeyPressMsg, s sample.Session) (tea.Model, tea.Cmd) {
	mk := m.keys.main
	switch {
	case bkey.Matches(k, m.keys.board.Merge):
		return m, m.boardMerge(s)
	case bkey.Matches(k, mk.Second):
		return m, m.boardKill(s)
	case bkey.Matches(k, mk.Grant) && (m.onPocket() || m.tapping) && m.goWaitsHere(s): // the belt's y is a tap: arm, then give
		return m.armGo(s, mk.Grant.Help().Key+" again gives the go: "+m.goWhat(s))
	case bkey.Matches(k, mk.Grant):
		return m.boardGo(s)
	}
	return m.onCard(s).key(k)
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
	was := m.side
	m.side = &sideMemo{} // a move changes only the choice: one build of the list serves it
	defer func() { m.side = was }()
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
	m.sideAt, m.doneKept = walk[clampMove(at, d, len(walk))], paneKey{}
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

// sidePaste is a paste on the Sessions tab: it lands in the line that
// shows. The chat's place gives it to the chosen session's line, and the
// room's talk line takes it as one line while it types. Any other place
// takes no text: it says so and keeps nothing.
func (m Model) sidePaste(text string) (tea.Model, tea.Cmd) {
	switch {
	case m.sidePlace == placeChat:
		return m.linePaste(text)
	case m.sidePlace == placeRoom && m.room != nil && m.room.typing:
		r := *m.room
		r.draft += oneLine(text)
		m.room = &r
		return m, nil
	}
	verb := "take"
	if m.sidePlace == placeRoom || m.sidePlace == placeHost {
		verb = "takes"
	}
	return m, m.notice("the " + m.placeName() + " " + verb + " no text · the paste is not kept")
}

// oneLine is text as one line: a newline or a tab is a space, and no other
// control character stays.
func oneLine(text string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, text)
}

// linePaste adds text to the chosen session's line, as one piece. A
// parked session's line is off: it says so, and takes nothing.
func (m Model) linePaste(text string) (tea.Model, tea.Cmd) {
	m.toLine()
	s, ok := m.sideChosen()
	switch {
	case !ok && m.chosenSide().waits():
		return m, m.notice(m.noRowWhy())
	case !ok:
		return m, m.notice("no session to write to · " + m.keys.main.New.Help().Key + " starts one")
	case m.lineOff(s) != "":
		return m, nil // the line says it is off, and why
	}
	m.setLine(paneOf(s), m.lines[paneOf(s)]+text)
	return m, nil
}

// lineOff is why the line of s takes no text, "" when it takes it: s is
// parked, or outside tmux.
func (m Model) lineOff(s sample.Session) string {
	if m.parked(s) {
		return m.parkedNote()
	}
	if s.Pane == "" {
		return s.Name + " runs outside tmux: no pane to write to"
	}
	return ""
}

// parkedNote is what the line of a parked session says: the list's z
// lets it go.
func (m Model) parkedNote() string {
	return "parked: " + m.keys.side.Leave.Help().Key + ", then " + m.keys.main.Park.Help().Key + " lets it go"
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
	if _, _, asks := m.digitMenu(s); !asks && !held {
		return false
	}
	return paneOf(s) == m.peekPane && m.menuID(s) == m.peekAsk && m.now().Sub(m.peekAt) >= peekDead
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
	queuedNote      = "queued for its next turn"
	oneHeldNote     = "one message already waits for its next turn"
	menuQueuedNote  = "queued until its question is answered"
	menuOneHeldNote = "one message already waits for the answer"
)

// menuUp: the pane of s draws a menu, a question to pick from or a
// permission prompt. A typed line would pick or approve there, and the
// service refuses it, so the line waits for the menu to go.
func (m Model) menuUp(s sample.Session) bool {
	if s.Ask != nil || s.Permits {
		return true
	}
	it, ok := m.waitOf(s)
	return ok && it.State == queue.StatePermission
}

// lineWaits: a line sent now cannot reach s. Its menu is up, or it works
// and its agent takes no typed input while it works.
func (m Model) lineWaits(s sample.Session) bool {
	return m.menuUp(s) || m.sideStateOf(s) == sideWork && !typesWhileBusy[s.Agent]
}

// heldNote is what the line of s says while a message waits for it.
func (m Model) heldNote(s sample.Session, another bool) string {
	switch {
	case m.menuUp(s) && another:
		return menuOneHeldNote
	case m.menuUp(s):
		return menuQueuedNote
	case another:
		return oneHeldNote
	}
	return queuedNote
}

// sendLine sends the chosen session's line as a message, on its host's
// conn. A line that cannot go keeps its text and says why; nothing
// retries by itself. To a session that works and does not take typed
// input, the message waits for the end of its turn.
func (m *Model) sendLine() tea.Cmd {
	s, ok := m.sideChosen()
	switch {
	case !ok && m.chosenSide().waits(): // ⏎ in the list reopens it, never a typed line
		return m.notice(m.noRowWhy())
	case !ok:
		return m.notice("no session to write to")
	}
	k := paneOf(s)
	text := m.lines[k]
	switch {
	case m.lineOff(s) != "":
		return nil // the line says it is off, and why
	case strings.TrimSpace(text) == "":
		return m.notice("the line is empty")
	}
	busy := m.sideStateOf(s) == sideWork
	if m.lineWaits(s) {
		if _, waits := m.lineHeld[k]; waits {
			m.lineNote = lineNote{k, m.heldNote(s, true)}
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
// or a tab, which a message refuses and a paste keeps, or for one longer
// than a message; else a message.
func lineAct(text string) string {
	if strings.ContainsAny(text, "\n\t") || utf8.RuneCountInString(text) > maxMessage {
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
			if by, held := m.holdOf(s); (held && by.By != holdByYou) || m.lineWaits(s) {
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
			note = m.parkedNote()
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
// went, and the answer with it. The texts come back in the order they
// went: giveBack puts each before the line, so the last sent goes first.
func (m Model) lineLost(host string) Model {
	var ids []string
	for id, sent := range m.lineActs {
		if sent.host == host {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(a, b string) int { return cmp.Compare(actNumber(b), actNumber(a)) })
	for _, id := range ids {
		sent := m.lineActs[id]
		m.lineActs = maps.Clone(m.lineActs)
		delete(m.lineActs, id)
		m.giveBack(sent, cmp.Or(m.cannotReach(host), "not sent: "+m.nameOf(host)+" is not reached"))
	}
	return m
}

// actNumber is the count in an act id sendOn gave ("a12" is 12).
func actNumber(id string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(id, "a"))
	return n
}

// answerResult is the answer to the act that carried a digit: a
// refusal gives the digits back, and the notice says why.
func (m Model) answerResult(r proto.Result) (Model, tea.Cmd, bool) {
	if r.ActID == "" || r.ActID != m.peekSentAct {
		return m, nil, false
	}
	m.peekSentAct = ""
	why := r.Err
	if why == "" && len(r.Skipped) > 0 { // a guard skipped it: it did not go
		why = r.Skipped[len(r.Skipped)-1]
	}
	if why == "" {
		return m, nil, true
	}
	m.peekSent, m.peekSentAsk, m.peekSentPane = 0, "", ""
	if !strings.HasPrefix(why, "not sent: ") {
		why = "not sent: " + why
	}
	return m, m.notice(why), true
}

// answerLost forgets the answer in flight to host: its conn went, and the
// act's answer with it. The digits take an answer again.
func (m Model) answerLost(host string, acts []string) (Model, tea.Cmd) {
	if m.peekSentAct == "" || !slices.Contains(acts, m.peekSentAct) {
		return m, nil
	}
	m.peekSentAct, m.peekSent, m.peekSentAsk, m.peekSentPane = "", 0, "", ""
	return m, m.notice(m.nameOf(host) + " is lost · the console does not know if the answer went")
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
