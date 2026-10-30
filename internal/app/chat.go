package app

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	bkey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

// watchActID starts the id of a watch act; the pane follows, so a refusal says which watch it was for.
const watchActID = "watch:"

// screenMsg is one capture of the watched pane.
type screenMsg proto.Screen

// trailingBlank is the spaces at the end of a line, and the style codes
// among them.
var trailingBlank = regexp.MustCompile(`(?:\x1b\[[0-9;:]*m|[ \t])+$`)

var sgr = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

// fitScreen fits a pane's text into w columns and the last h lines: a rule
// is cut, other text wraps, and the blank lines at the end go.
func fitScreen(text string, w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		line = trailingBlank.ReplaceAllStringFunc(line, func(tail string) string {
			return strings.Join(sgr.FindAllString(tail, -1), "")
		})
		if isRule(line) {
			out = append(out, ansi.Truncate(line, w, ""))
			continue
		}
		out = append(out, strings.Split(ansi.Hardwrap(line, w, true), "\n")...)
	}
	for len(out) > 0 && ansi.Strip(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	if len(out) > h {
		out = out[len(out)-h:]
	}
	return out
}

// isRule is a line of box-drawing runes and spaces, and not empty.
func isRule(line string) bool {
	plain := ansi.Strip(line)
	if strings.TrimSpace(plain) == "" {
		return false
	}
	for _, r := range plain {
		if r != ' ' && (r < 0x2500 || r > 0x257F) {
			return false
		}
	}
	return true
}

// watchCmd asks the service for the selected session's screen when it is
// not the one already watched, and for none when the selection has no pane.
// A screen of another pane never stays.
func (m *Model) watchCmd() tea.Cmd {
	want := ""
	if m.conn != nil && !m.lost && !m.mismatch {
		if s, ok := m.selected(); ok {
			want = s.Pane
		}
	}
	if m.watchHeld != "" && want == m.watchHeld {
		want = m.watching
	}
	if want == m.watching {
		return nil
	}
	m.watching, m.screen = want, proto.Screen{}
	if m.conn == nil || m.lost || m.mismatch {
		return nil // the service forgot the watch with the connection
	}
	c := m.conn
	return func() tea.Msg { // not m.send: the watch takes no number from the acts the builder sees
		if err := c.Send(proto.KindAct, watchActID+want, proto.Act{RecID: proto.WatchPrefix + want}); err != nil {
			return lostMsg{err, c}
		}
		return nil
	}
}

// chatLines is the watched screen fitted to w by h, with the question card
// at its foot, or what stands in its place: that the session ended, or
// blank lines until a screen comes.
func (m Model) chatLines(w, h int) []string {
	h = max(h, 0)
	lines := make([]string, h)
	s, ok := m.selected()
	if !ok || s.Pane == "" {
		return lines
	}
	shown := m.screen.Pane == s.Pane
	if shown && m.screen.Gone {
		const text = "That session ended."
		if h > 0 {
			lines[h/2] = strings.Repeat(" ", max((w-ansi.StringWidth(text))/2, 0)) + text
		}
		return lines
	}
	card := m.cardLines(w)
	card = card[max(len(card)-h, 0):]
	var fit []string
	if shown {
		fit = fitScreen(m.screen.Text, w, h-len(card))
	}
	fit = append(fit, card...)
	return append(lines[:len(lines)-len(fit)], fit...)
}

// chatShown is the session whose chat is the right side of Sessions: the
// selected one, when it has a pane and the screen has a right side.
func (m Model) chatShown(rightW int) (sample.Session, bool) {
	s, ok := m.selectedSession()
	return s, ok && s.Pane != "" && m.tab == tabSessions && rightW > 0
}

// chatTitle names the chat's session and its state.
func (m Model) chatTitle(s sample.Session) string { return s.Name + " · " + m.wordOf(s) }

// chatPane is the chat beside the list, w by h: its title, in the accent
// while the chat has the keys, over the screen.
func (m Model) chatPane(s sample.Session, w, h int) []string {
	ink := m.st.label
	if m.chatKeys {
		ink = m.st.accent
	}
	lines := []string{fit(" "+ink.Render(m.chatTitle(s)), w)}
	for _, l := range m.screenLines(w-1, h-1) {
		lines = append(lines, " "+l)
	}
	return lines
}

// screenLines are chatLines, each closed so the pane's styles stop at its
// end.
func (m Model) screenLines(w, h int) []string {
	lines := m.chatLines(w, h)
	for i, l := range lines {
		lines[i] = l + ansi.ResetStyle
	}
	return lines
}

// chatScreen is the chat on an upright phone: the whole screen under a
// title row that goes back to Sessions and says a toast, over the key
// line, with a step that asks to confirm on top. Its title, hairline and
// key line always show: a screen under 4 rows has no room for the agent's
// lines.
func (m Model) chatScreen(w int) drawn {
	st := m.st
	s, _ := m.selectedSession()
	title := " " + st.accent.Render("‹ Sessions") + st.muted.Render(" · ") + st.accent.Render(m.chatTitle(s))
	if m.toast != "" { // as the header says it, where there is one
		title = spread(title, st.accent.Render(ansi.Truncate(m.toast, max(w/2, 1), "…"))+" ", w)
	}
	title = fit(title, w)
	sc := drawn{lines: []string{title}, bodyTop: 1, listW: w, dockAt: -1, actKeysAt: -1, chatFull: true}
	sc.lines = append(sc.lines, m.screenLines(w, max(m.height-3, 0))...)
	m.markCard(&sc, len(sc.lines), 0, w, max(m.height-3, 0))
	keys, spans := m.keyLine(w)
	sc.keysAt, sc.keys = len(sc.lines)+1, spans
	sc.lines = append(sc.lines, st.hair.Render(strings.Repeat("─", w)), keys)
	if m.consenting() {
		m.overlayConsent(&sc, w)
	}
	return sc
}

// markCard finds the question card at the foot of a chat w columns wide
// and h lines tall, whose last line is above line bottom and whose first
// column is x.
func (m Model) markCard(sc *drawn, bottom, x, w, h int) {
	if s, ok := m.selected(); ok && m.screen.Pane == s.Pane && m.screen.Gone {
		return // the chat says the session ended, and shows no card
	}
	_, press, spans := m.cardRows(w)
	press = press[max(len(press)-h, 0):]
	sc.cardAt, sc.cardX, sc.cardW, sc.cardRows, sc.cardKeys = bottom-len(press), x, min(w, cardMaxW), press, spans
}

// homeKey is the app's home gesture (panes.HomeKey, which the console may
// not import): it gives the keys back to Sessions.
const homeKey = "\x1b_matchblox\x1b\\"

// chatKey is one key the chat sends to an agent: the pane and the agent it
// was pressed for, and when the screen it answers was taken.
type chatKey struct {
	pane string
	pid  int
	text string
	seen time.Time
}

// maxLit is the most text one "lit:" key carries; the service takes no more.
const maxLit = 200

// openChat gives the keys to the chat of the selected session, or of the
// session of a failed step, or says why there is none.
func (m Model) openChat() Model {
	pane, why := m.chatTarget()
	if pane == "" {
		m.flash = why
		return m
	}
	return m.chatOn(pane)
}

// chatTarget is the pane whose chat a opens: the selected session's, when
// it has a pane to type into, or a failed step's. Why says why there is
// none.
func (m Model) chatTarget() (pane, why string) {
	if m.tab == tabStoker {
		return m.stokerChatTarget()
	}
	s, ok := m.selectedSession()
	switch {
	case !ok:
		return "", ""
	case s.Pane == "":
		return "", s.Name + " runs outside tmux: there is no pane to type into"
	}
	return s.Pane, ""
}

// chatOn is Sessions with the session in pane selected and its chat
// holding the keys. The selection stays on the session while the chat has
// its keys.
func (m Model) chatOn(pane string) Model {
	s, ok := m.sessionIn(pane)
	if !ok {
		return m
	}
	if m.tab != tabSessions {
		m.setTab(tabSessions)
	}
	m.doorPick, m.recPick, m.ghostPick, m.chose, m.selPID = "", "", 0, true, s.PID
	m.chatKeys, m.flash = true, ""
	if s.Ask != nil { // what was on screen before the chat opened was not the card
		m.cardSeen = cardSeen{askID: s.Ask.ID, at: m.now()}
	}
	return m
}

// intoPane is ctrl+o: the selected session's pane, behind a confirm, as
// Enter on a session was before the chat.
func (m Model) intoPane() Model {
	if s, ok := m.selectedSession(); ok && s.Pane != "" {
		m.pending, m.previewTop = m.primary(), 0
	}
	return m
}

// ctrlCMark names an agent the chat sent ^C to, and when.
type ctrlCMark struct {
	pane string
	pid  int
	at   time.Time
}

func (c ctrlCMark) is(k chatKey) bool { return c.pane != "" && c.pane == k.pane && c.pid == k.pid }

// chatKey is k while the chat has the keys: ctrl+] gives them back, ctrl+o
// goes into the pane, the question card takes its own keys, and every
// other key goes to the agent.
func (m Model) chatKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.flash = ""
	switch {
	case bkey.Matches(k, m.keys.chat.Back):
		m.chatKeys = false
		return m, nil
	case bkey.Matches(k, m.keys.main.Jump):
		return m.intoPane(), nil
	case m.cardTakes(k):
		return m.cardKey(k)
	}
	text, ok := keyName(k)
	if !ok {
		return m, nil
	}
	name, keys := m.pressed(text)
	return m.toAgent(name, keys...)
}

// pressed is what goes to the chat's agent for the texts pressed now: its
// pane and agent, and the screen they answer, fixed at this moment. Name
// is the session's, for a line that says it ended.
func (m Model) pressed(texts ...string) (name string, keys []chatKey) {
	s, ok := m.selectedSession()
	if !ok || s.Pane == "" {
		return "", nil
	}
	var seen time.Time
	if m.screen.Pane == s.Pane {
		seen = m.screen.At
	}
	for _, t := range texts {
		keys = append(keys, chatKey{pane: s.Pane, pid: s.PID, text: t, seen: seen})
	}
	return s.Name, keys
}

// heldKeys is the take-over answer's replay of keys pressed in the chat:
// they go to the agent they were pressed for, wherever the selection is.
func (m Model) heldKeys(name string, keys []chatKey) func(Model) (tea.Model, tea.Cmd) {
	return func(m Model) (tea.Model, tea.Cmd) { return m.toAgent(name, keys...) }
}

// chatPaste is a paste in the chat: text to the agent. A newline goes as
// Enter; other control characters drop.
func (m Model) chatPaste(text string) (tea.Model, tea.Cmd) {
	m.flash = ""
	name, keys := m.pressed(pasteTexts(text)...)
	if len(keys) == 0 {
		return m, nil
	}
	if m.night() && !m.stoker.Asked {
		m.held = m.heldKeys(name, keys)
		return m, nil
	}
	return m.toAgent(name, keys...)
}

// pasteTexts are the keys of a paste: its text in "lit:" chunks of at most
// maxLit bytes, with Enter for each newline: \n, \r or \r\n, as terminals
// paste a newline as \r.
func pasteTexts(text string) []string {
	var out []string
	var chunk strings.Builder
	flush := func() {
		if chunk.Len() > 0 {
			out = append(out, "lit:"+chunk.String())
			chunk.Reset()
		}
	}
	var prev rune
	for _, r := range text {
		switch {
		case r == '\n' && prev == '\r': // \r\n is one newline
		case r == '\n' || r == '\r':
			flush()
			out = append(out, enterName)
		case unicode.IsControl(r):
		default:
			if chunk.Len()+utf8.RuneLen(r) > maxLit {
				flush()
			}
			chunk.WriteRune(r)
		}
		prev = r
	}
	flush()
	return out
}

// toAgent queues keys for the agent they were pressed for, unless there is
// no connection or that agent left. A ^C within a second of the last one
// sent to the same agent waits for a third: two ^C stop Claude Code.
func (m Model) toAgent(name string, keys ...chatKey) (tea.Model, tea.Cmd) {
	if len(keys) == 0 {
		return m, nil
	}
	if m.conn == nil || m.lost {
		m.flash = "not sent, no connection"
		return m, nil
	}
	if !m.agentIn(keys[0]) {
		m.flash = "not sent: " + name + " ended"
		return m, nil
	}
	if k := keys[0]; k.text == ctrlCName {
		if m.ctrlCHeld.is(k) {
			m.ctrlCHeld = ctrlCMark{}
		} else if last := m.lastCtrlC(k); last != "" {
			m.ctrlCHeld = ctrlCMark{pane: k.pane, pid: k.pid, at: m.now()}
			stop := m.keys.chat.Stop.Help().Key
			m.flash = fmt.Sprintf("%s held: %s, %s again stops the agent", stop, last, stop)
			return m, nil
		}
	}
	for _, k := range keys {
		m.queueKey(k)
	}
	cmd := m.sendKey()
	return m, cmd
}

// lastCtrlC says the ^C to k's agent that a new one comes too close to:
// one that waits in the queue, or one sent less than a second ago. Empty
// when there is none.
func (m Model) lastCtrlC(k chatKey) string {
	if slices.ContainsFunc(m.keyQ, func(q chatKey) bool { return q.text == k.text && q.pane == k.pane && q.pid == k.pid }) {
		return "one waits to go"
	}
	if since := m.now().Sub(m.ctrlC.at); m.ctrlC.is(k) && since < time.Second {
		return fmt.Sprintf("one went %.1f s ago", since.Seconds())
	}
	return ""
}

// agentIn tells if k's agent is still in its pane, in all the service
// sent: a search or the stoker's own row does not hide it.
func (m Model) agentIn(k chatKey) bool {
	return slices.ContainsFunc(m.all.sessions, func(s sample.Session) bool { return s.Pane == k.pane && s.PID == k.pid })
}

// queueKey puts k behind the keys that wait. Text typed after text for the
// same agent joins it, up to maxLit.
func (m *Model) queueKey(k chatKey) {
	q := slices.Clone(m.keyQ)
	if n := len(q); n > 0 {
		last := &q[n-1]
		typed, isText := strings.CutPrefix(k.text, "lit:")
		if isText && strings.HasPrefix(last.text, "lit:") && last.pane == k.pane && last.pid == k.pid && len(last.text)+len(typed) <= len("lit:")+maxLit {
			last.text += typed
			m.keyQ = q
			return
		}
	}
	m.keyQ = append(q, k)
}

// sendKey sends the first key that waits when none is in flight: one at a
// time, so they reach the agent in the order they were pressed.
func (m *Model) sendKey() tea.Cmd {
	if m.keyInFlight != "" || len(m.keyQ) == 0 || m.conn == nil || m.lost {
		return nil
	}
	k := m.keyQ[0]
	m.keyQ = m.keyQ[1:]
	cmd := m.send(proto.Act{RecID: proto.KeysPrefix + k.pane, Text: k.text, PID: k.pid, Seen: k.seen})
	m.keyInFlight = "a" + strconv.Itoa(m.acts)
	if k.text == ctrlCName {
		m.ctrlC = ctrlCMark{pane: k.pane, pid: k.pid, at: m.now()}
	}
	return cmd
}

// keyAnswered is the service's answer to the key in flight: the next key
// goes, or a refusal shows and the keys behind it drop, as they were
// pressed for a screen that is no longer there.
func (m Model) keyAnswered(r proto.Result) (tea.Model, tea.Cmd) {
	m.keyInFlight = ""
	if r.Err != "" {
		m.keyQ, m.flash = nil, r.Err
		return m, m.recv()
	}
	cmd := m.sendKey()
	return m, tea.Batch(m.recv(), cmd)
}

// keepChat gives the keys back to Sessions when the chat no longer shows
// the session it had (a chat opened just now on another session keeps
// them), and drops the keys of an agent that left.
func (m Model) keepChat(was Model) Model {
	if m.chatKeys {
		s, ok := m.selectedSession()
		before, _ := was.selectedSession()
		switch {
		case ok && s.Pane != "" && s.Pane == before.Pane && m.screen.Pane == s.Pane && m.screen.Gone:
			m.chatKeys, m.flash = false, "That session ended." // a phone shows the list again: the key line says why
		case !ok || s.Pane == "" || was.chatKeys && s.Pane != before.Pane:
			m.chatKeys = false
		}
	}
	if m.conn == nil || m.lost {
		m.keyQ, m.keyInFlight = nil, ""
		return m
	}
	var gone []chatKey
	q := slices.DeleteFunc(slices.Clone(m.keyQ), func(k chatKey) bool {
		if m.agentIn(k) {
			return false
		}
		gone = append(gone, k)
		return true
	})
	if len(gone) == 0 {
		return m
	}
	name := gone[0].pane
	if i := slices.IndexFunc(was.all.sessions, func(s sample.Session) bool { return s.PID == gone[0].pid }); i >= 0 {
		name = was.all.sessions[i].Name
	}
	m.keyQ, m.flash = q, "not sent: "+name+" ended"
	return m
}

// chatKeyLine is the key line while the chat has the keys: whose agent
// they go to, and the way back and the way into tmux.
func (m Model) chatKeyLine(w int) (string, []span) {
	st := m.st
	s, _ := m.selectedSession()
	lead := "keys go to " + s.Name + " · "
	line, x := " "+st.text.Render(lead), 1+lipgloss.Width(lead)
	var spans []span
	for i, b := range []bkey.Binding{m.keys.chat.Back, m.keys.main.Jump} {
		if i > 0 {
			line += st.muted.Render(" · ")
			x += lipgloss.Width(" · ")
		}
		part := st.accent.Render(b.Help().Key) + st.muted.Render(" "+b.Help().Desc)
		spans = append(spans, span{x, x + lipgloss.Width(part), b.Keys()[0]})
		line += part
		x += lipgloss.Width(part)
	}
	return fit(line, w), spans
}
