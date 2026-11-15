package app

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/transport"
)

// watchActID starts the id of a watch act; the pane follows, so a refusal says which watch it was for.
const watchActID = "watch:"

// keyActID starts the id of a key the chat or the stand-in sends: its
// answer says nothing unless it is a refusal.
const keyActID = "key:"

// screenMsg is one capture of the watched pane.
type screenMsg proto.Screen

// trailingBlank is the spaces at the end of a line, and the style codes
// among them.
var trailingBlank = regexp.MustCompile(`(?:\x1b\[[0-9;:]*m|[ \t])+$`)

var sgr = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

// controlStrings is an OSC (a hyperlink, a title), or an APC, DCS, PM or SOS
// string, up to its terminator.
var controlStrings = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[_P^X][^\x1b]*\x1b\\`)

// escapes is any other escape sequence: a CSI, in its 7-bit or C1 form,
// or ESC and the bytes that end it; else a lone ESC, a C1 control or a C0
// control other than a tab or a new line.
var escapes = regexp.MustCompile("(?:\x1b\\[|\u009b)[0-?]*[ -/]*[@-~]|\x1b[ -/]*[0-~]|[\x00-\x08\x0b-\x1f\x7f\u0080-\u009f]")

// sgrOnly matches an SGR, a colour or a style: the one escape sequence an
// agent's screen keeps.
var sgrOnly = regexp.MustCompile(`^\x1b\[[0-9;:]*m$`)

// onlyColours is an agent's screen with only its colours and styles: no
// other escape sequence or control of the agent's reaches the console's
// terminal, so the screen cannot move the cursor, clear the console, change
// a mode, set a title or open a link there.
func onlyColours(text string) string {
	text = controlStrings.ReplaceAllString(strings.ReplaceAll(text, "\r", ""), "")
	return escapes.ReplaceAllStringFunc(text, func(seq string) string {
		if sgrOnly.MatchString(seq) {
			return seq
		}
		return ""
	})
}

// fitScreen fits a pane's text into w columns and the last h lines: a rule
// is cut, other text wraps between words, and the blank lines at the end go.
// Of its escape sequences only the styles stay: a link or a string control
// of the agent's never reaches the console's terminal.
func fitScreen(text string, w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	text = onlyColours(text)
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = trimBlank(line)
		if isRule(line) {
			out = append(out, ansi.Truncate(line, w, ""))
			continue
		}
		for _, piece := range strings.Split(ansi.Wrap(line, w, ""), "\n") {
			out = append(out, trimBlank(piece))
		}
	}
	for len(out) > 0 && ansi.Strip(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	if len(out) > h {
		out = out[len(out)-h:]
	}
	return out
}

// trimBlank drops the spaces at the end of a line and keeps the style codes
// among them.
func trimBlank(line string) string {
	return trailingBlank.ReplaceAllStringFunc(line, func(tail string) string {
		return strings.Join(sgr.FindAllString(tail, -1), "")
	})
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

// cropScreen fits a full-screen agent's text, laid out for its own pane,
// into w columns and the last h lines: the blank rows at its end go, and
// a line wider than w is cut, with a › at the cut. Nothing wraps: a box
// or a menu keeps its shape. Of its escape sequences only the styles
// stay, as in fitScreen.
func cropScreen(text string, w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	lines := strings.Split(onlyColours(text), "\n")
	for i, l := range lines {
		lines[i] = trimBlank(l)
	}
	for len(lines) > 0 && ansi.Strip(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	lines = lines[max(len(lines)-h, 0):]
	for i, l := range lines {
		if ansi.StringWidth(l) > w {
			lines[i] = ansi.Truncate(l, max(w-1, 0), "") + "\x1b[m›"
		}
	}
	return lines
}

// watchCmd asks the service of the watched session's host for its screen
// when it is not the one already watched, and for none when there is no
// such session, its host is not reached, or no screen shows the console
// (away). A new host's watch stops the old host's first. A screen of
// another pane never stays. The watch names no size: the agent's window
// keeps its own (#351).
func (m *Model) watchCmd() tea.Cmd {
	host, want := m.watchWant()
	if m.watchConn(host) == nil {
		host, want = m.watchHost, "" // the service forgot the watch with the connection
	}
	if host != m.watchHost {
		m.watchHeld = "" // a pane id the old host refused names another pane here
	}
	if m.watchHeld != "" && host == m.watchHost && want == m.watchHeld {
		want = m.watching
	}
	if host == m.watchHost && want == m.watching {
		return nil
	}
	var stop tea.Cmd
	if host != m.watchHost && m.watching != "" {
		stop = m.watchOn(m.watchHost, "")
	}
	m.screen = proto.Screen{} // another pane opens blank until its screen comes
	m.watchHost, m.watching = host, want
	return tea.Batch(stop, m.watchOn(host, want))
}

// watchWant is the host and the pane to watch: the stand-in's session,
// else the list's chosen session on any host; off the Sessions tab, the
// selected session of the home host. No pane: the host stays and the pane
// is "".
func (m Model) watchWant() (host, pane string) {
	if m.away {
		return m.watchHost, ""
	}
	if m.boardShown() {
		if s, ok := m.selectedSession(); ok && m.standIn && s.Host == m.opt.Host {
			return s.Host, s.Pane
		}
		if k := m.chosenSide(); k.pane != "" {
			return k.host, k.pane
		}
		return m.watchHost, ""
	}
	if s, ok := m.selected(); ok && s.Host == m.opt.Host { // another host's pane id names another pane here
		return s.Host, s.Pane
	}
	return m.watchHost, ""
}

// watchConn is the conn that reaches host's service: the home conn for the
// home host, else the peer's own. Nil when the host is not reached.
func (m Model) watchConn(host string) transport.Conn {
	if host == m.opt.Host {
		if m.conn == nil || m.lost || m.mismatch {
			return nil
		}
		return m.conn
	}
	i := m.peerAt(host)
	if i < 0 {
		return nil
	}
	if l := m.peers[i]; l.conn != nil && !l.lost && !l.mismatch && !l.redialing {
		return l.conn
	}
	return nil
}

// watchOn asks host's service to send the screen of pane, or of none for
// "". It is not m.send: the watch takes no number from the acts the
// builder sees.
func (m Model) watchOn(host, pane string) tea.Cmd {
	c := m.watchConn(host)
	if c == nil {
		return nil
	}
	act := proto.Act{RecID: proto.WatchPrefix + pane}
	send := func() tea.Msg {
		if err := c.Send(proto.KindAct, watchActID+pane, act); err != nil {
			return lostMsg{err, c}
		}
		return nil
	}
	if host == m.opt.Host {
		return send
	}
	return toPeer(host, send)
}

// screenFrom keeps a screen host sent when it is of the watched pane: a
// screen of another pane, or of the same pane id on another host, never
// stays. A read that failed flashes, and the last screen stays.
func (m Model) screenFrom(host string, msg screenMsg) Model {
	switch {
	case host != m.watchHost || msg.Pane != m.watching:
	case msg.Err != "":
		m.flash = msg.Err
	default:
		m.screen = proto.Screen(msg)
	}
	return m
}

// watchRefused holds the watch host refused: a pane that went before the
// watch came is a race, not the builder's error. false: r is no refused
// watch.
func (m Model) watchRefused(host string, r proto.Result) (Model, bool) {
	pane, ok := strings.CutPrefix(r.ActID, watchActID)
	if !ok || r.Err == "" {
		return m, false
	}
	if pane != "" && host == m.watchHost && pane == m.watching {
		m.watchHeld, m.watching, m.screen = pane, "", proto.Screen{}
	}
	return m, true
}

// keysOut are the keys the console sends, in order: one goes at a time,
// and the next after the service answers it. Each send runs on its own, so
// two fast keys sent at once could reach the agent swapped (#342).
type keysOut struct {
	conn  transport.Conn // the connection the key in flight went on
	busy  bool
	queue []proto.Act
}

// maxKeysAhead is how many keys wait behind the one in flight.
const maxKeysAhead = 64

// sendKey sends text to the pane of s as the keys act, for the screen of it
// the console shows. Not m.send: a key takes no number from the acts the
// builder sees, and its answer is no toast.
func (m *Model) sendKey(s sample.Session, text string) tea.Cmd {
	var seen time.Time
	if m.screen.Pane == s.Pane {
		seen = m.screen.At
	}
	if m.conn == nil || m.lost {
		return nil
	}
	cmd, why := m.sendKeySeen(s, text, seen)
	if why != "" {
		m.flash = why
	}
	return cmd
}

// sendKeySeen is sendKey for what the console showed of s at seen. why
// says why the key neither went nor waits in line; "" when it did.
func (m *Model) sendKeySeen(s sample.Session, text string, seen time.Time) (cmd tea.Cmd, why string) {
	if m.conn == nil || m.lost {
		return nil, "not sent: no service · it redials"
	}
	act := proto.Act{RecID: proto.KeysPrefix + s.Pane, Text: text, PID: s.PID, Start: s.Start, Seen: seen}
	if m.keysOut.conn != m.conn { // a new connection: no answer comes for the old one's keys
		m.keysOut = keysOut{conn: m.conn}
	}
	if m.keysOut.busy {
		if len(m.keysOut.queue) >= maxKeysAhead {
			return nil, "not sent: " + strconv.Itoa(maxKeysAhead) + " keys still wait for " + s.Name
		}
		m.keysOut.queue = append(m.keysOut.queue, act)
		return nil, ""
	}
	m.keysOut.busy = true
	return sendKeyAct(m.conn, act), ""
}

// nextKey sends the key that waits behind the one the service answered.
func (m *Model) nextKey() tea.Cmd {
	if len(m.keysOut.queue) == 0 {
		m.keysOut.busy = false
		return nil
	}
	act := m.keysOut.queue[0]
	m.keysOut.queue = m.keysOut.queue[1:]
	return sendKeyAct(m.keysOut.conn, act)
}

func sendKeyAct(c transport.Conn, act proto.Act) tea.Cmd {
	return func() tea.Msg {
		if err := c.Send(proto.KindAct, keyActID+strings.TrimPrefix(act.RecID, proto.KeysPrefix), act); err != nil {
			return lostMsg{err, c}
		}
		return nil
	}
}
