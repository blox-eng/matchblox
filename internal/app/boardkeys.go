package app

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

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

// chooseRow makes s the list's chosen session. A move to another
// session starts its mark guard.
func (m *Model) chooseRow(s sample.Session) {
	if paneOf(s) != m.chosenSide() {
		m.startChatMark(s)
		m.goArmed = paneKey{}
	}
	m.sideAt = paneOf(s)
}

// startChatMark starts the mark guard on s: for peekDead its digits arm
// nothing, and a mark from before holds no more.
func (m *Model) startChatMark(s sample.Session) {
	m.peekAt, m.peekAsk, m.peekPane, m.peekNew = m.now(), askOf(s), paneOf(s), false
}

// askOf is the question s asks with its options (Ask.ID), "" for none.
func askOf(s sample.Session) string {
	if s.Ask == nil {
		return ""
	}
	return s.Ask.ID
}

// chatMarkOn keeps the mark guard on the chosen session. The session
// coming into reach of the digits (the first frame, a layer over the list
// that closes, the menu that closes), a choice that moved with no key (a
// session that ended, a filter) and a new question on the chosen session
// each start the guard again. A menu off the Sessions tab or on a session
// that is gone closes.
func (m Model) chatMarkOn() Model {
	m = m.copyCheckOn()
	if _, ok := m.menuOn(); m.menu != nil && !ok {
		m.menu = nil
	}
	s, ok := m.sideChosen()
	reach := ok && m.chatMarkReach()
	was := m.peekReached
	m.peekReached = reach
	switch {
	case !reach:
	case !was || paneOf(s) != m.peekPane:
		m.startChatMark(s)
	case askOf(s) != m.peekAsk:
		m.startChatMark(s)
		m.peekNew = askOf(s) != ""
	}
	return m
}

// chatMarkReach: a digit reaches the chat, with no layer or menu over it.
func (m Model) chatMarkReach() bool {
	return m.boardShown() && m.menu == nil && !m.standIn && !m.viewFull() && m.focus(keyPress("1")) == layerMain
}

// chatAnswers: the digits answer s's question, one question with one
// choice, given with its options.
func (m Model) chatAnswers(s sample.Session) bool {
	_, ok := m.oneChoice(s)
	return ok
}

// chatMarks are the option a first digit marked on s's card, and the
// option sent to its question; 0 for none.
func (m Model) chatMarks(s sample.Session) (armed, sent int) {
	switch id := askOf(s); {
	case id == "":
	case id == m.peekSentAsk:
		sent = m.peekSent
	case paneOf(s) == m.peekPane && id == m.peekAsk && m.peekArmedAt.After(m.peekAt):
		armed = m.peekArmed
	}
	return armed, sent
}

// answerFromChat is a digit on the chosen match (the spec's "Answering
// from the peek"): the first marks that option of its question, the
// same digit again sends it. It sends nothing for peekDead after the peek
// starts, to a question the card did not draw, or while the service is
// lost.
func (m Model) answerFromChat(digit string) (tea.Model, tea.Cmd) {
	s, ok := m.sideChosen()
	q, _ := m.oneChoice(s)
	n, _ := strconv.Atoi(digit)
	armed, sent := m.chatMarks(s)
	said, answer := "", tea.Cmd(nil)
	switch {
	case !ok || sent > 0:
		// no card, or the card says its answer is sent
	case !m.chatAnswers(s) && m.asksNow(s):
		said = m.cardName(s) + " asks in its pane · " + m.keys.main.Jump.Help().Key + " goes in"
	case !m.chatAnswers(s):
		said = m.cardName(s) + " asks no question"
	case m.cannotReach(s.Host) != "":
		m.peekArmed, said = 0, m.cannotReach(s.Host)
	case paneOf(s) != m.peekPane || askOf(s) != m.peekAsk:
		m.startChatMark(s) // a question the card has not drawn yet
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
		m.peekArmed, m.peekSent, m.peekSentAsk, m.peekSentPane = 0, n, askOf(s), s.Pane
		said = "sent " + digit + " to " + m.cardName(s) + " · now in " + sideWork.head()
	default:
		m.peekArmed, m.peekArmedAt = n, m.now()
	}
	note := m.once("digit", "A digit marks an answer now. Press it again to send.", said)
	return m, tea.Batch(answer, note)
}

// sendAnswer sends digit to the pane of s as sendKey does, seen at the
// time of the sample that carried the question the card drew: the card's
// options come from the sample, not from the pane's screen. A peer's
// session gets it on the peer's own conn. why says why the key did not go
// and does not wait in line; "" when it did.
func (m *Model) sendAnswer(s sample.Session, digit string) (tea.Cmd, string) {
	if s.Host == m.opt.Host {
		m.peekSentAct = "" // the home keys' refusal comes by the pane (model.go's keyActID)
		return m.sendKeySeen(s, proto.LitPrefix+digit, m.stateAt(s.Host))
	}
	if why := m.cannotReach(s.Host); why != "" {
		return nil, why
	}
	id, cmd := m.sendToID(s.Host, proto.Act{RecID: proto.KeysPrefix + s.Pane, Text: proto.LitPrefix + digit, PID: s.PID, Start: s.Start, Seen: m.stateAt(s.Host)})
	m.peekSentAct = id
	return cmd, ""
}

// isDigit: k is a digit with no modifier.
func isDigit(k tea.KeyPressMsg) bool {
	return k.Mod == 0 && len(k.Text) == 1 && k.Text[0] >= '0' && k.Text[0] <= '9'
}

// belowKey is ⏎, x or d on the tip with the id line: it takes them as
// its row in Sessions did.
func (m Model) belowKey(line string, k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.pickBelow(line)
	return m.key(k)
}

// pickBelow selects the tip with the id line while it shows.
func (m *Model) pickBelow(line string) {
	m.doorPick, m.recPick, m.blockedPick = "", "", 0
	if slices.ContainsFunc(m.recs, func(r advice.Rec) bool { return r.ID == line }) {
		m.recPick = line
	}
}

// listOnRow: the list's choice is a row of Sessions (onChoice). A search
// that matches nothing, and an open menu, have no row.
func (m Model) listOnRow() bool {
	if m.menu != nil {
		return false // the menu takes every key; its go in asks for itself
	}
	_, ok := m.sideChosen()
	return ok
}

// onChoice is m with the selection of Sessions on the list's choice: the
// layers over the list (the take-over question) read that selection, so
// they act on the session the key was pressed on. With the setup steps in
// the chat's place, the selection is the step they name.
func (m Model) onChoice() Model {
	if !m.sideActive() {
		return m
	}
	if m.sidePlace == placeSetup {
		if i := m.doorIndex(m.boardDoors()) - len(m.copyDoors()); i >= 0 && i < len(m.doors) {
			m.selectRow(i)
		}
		return m
	}
	if s, ok := m.sideChosen(); ok {
		return m.onCard(s)
	}
	return m
}

// clearBoardFilters is a tap on the count: NEEDS YOU shows, every chain,
// host and search goes, and the first wait is chosen.
func (m *Model) clearBoardFilters() {
	if m.room != nil { // the room is the narrowing: both go
		*m = m.closeRoom()
	}
	m.boardChain, m.boardHost, m.filter, m.searching = "", "", "", false
	m.refresh()
	m.peekAt = m.now()
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

// boardInto is ⏎: into the pane of s, then the first other wait is chosen.
func (m Model) boardInto(s sample.Session) (tea.Model, tea.Cmd) {
	return m.onCard(s).intoSelected()
}

// boardCopy is c: the chosen card's last message, whole, on the clipboard.
func (m *Model) boardCopy(s sample.Session, ok bool) tea.Cmd {
	first := m.keys.board.Copy.Help().Key + " copies now: the last message, whole."
	switch {
	case !ok:
		return m.once("copy", first, "nothing to copy: no session is chosen")
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
	note := m.once("merge", m.keys.board.Merge.Help().Key+" merges now: the pull request that waits for you.", said)
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

// boardGo is y: the go that a stoker step waits for on the chosen session.
func (m Model) boardGo(s sample.Session) (tea.Model, tea.Cmd) {
	r, ok := m.goStep(s)
	if !ok {
		return m, m.notice("no step waits for your go on " + s.Name)
	}
	return m.grant(r)
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
	s, card := m.sideChosen()
	card = board && card
	ks := []beltKey{home}
	if card {
		ks = append(ks, m.cardBelt(s)...)
	}
	_, head := m.headRow(s)
	return append(ks, up, down, beltOf(mk.Open, "", "", true),
		beltOf(bk.Copy, "", "copy", card),
		beltOf(bk.Menu, "", "menu", card),
		beltOf(bk.Chain, "", "chain", card && m.chainLabelOf(s) != ""),
		beltOf(bk.Host, "", "hosts", board && m.hostCount() > 1),
		beltOf(mk.NextTab, "⇥", "machine", !m.consenting()),
		beltOf(mk.Second, "", "kill", card && !head && m.killAction(s) != nil),
		beltOf(mk.Search, "", "", board),
		beltOf(mk.Park, "", "park", card),
	)
}

// cardBelt are the chosen card's keys on the belt: the digits of the
// options it asks (at most three; off on another host, which takes no
// digit), or p for the ship that waits for its merge and y for the go
// that waits.
func (m Model) cardBelt(s sample.Session) []beltKey {
	var ks []beltKey
	if q, answers := m.oneChoice(s); answers {
		if _, sent := m.chatMarks(s); sent == 0 {
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
	if s, ok := m.sideChosen(); ok && m.boardShown() {
		if a, _ := m.chatMarks(s); a > 0 {
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
// A tok is a beltKey's Key: 1 2 3 p y up down enter c m g s tab x / z. A card's key that is off (a digit on another host)
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
