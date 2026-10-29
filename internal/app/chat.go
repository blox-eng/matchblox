package app

import (
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
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

// chatLines is the watched screen fitted to w by h, or what stands in its
// place: that the session ended, or blank lines until a screen comes.
func (m Model) chatLines(w, h int) []string {
	lines := make([]string, max(h, 0))
	s, ok := m.selected()
	if !ok || s.Pane == "" || m.screen.Pane != s.Pane {
		return lines
	}
	if m.screen.Gone {
		const text = "That session ended."
		if h > 0 {
			lines[h/2] = strings.Repeat(" ", max((w-ansi.StringWidth(text))/2, 0)) + text
		}
		return lines
	}
	fit := fitScreen(m.screen.Text, w, h)
	return append(lines[:len(lines)-len(fit)], fit...)
}
