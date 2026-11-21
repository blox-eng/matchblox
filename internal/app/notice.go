package app

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// A notice is a toast, a refusal or the data's age: it shows for a moment
// over the key line (the spec's "Notices").

// noticeFor is how long a notice shows.
const noticeFor = 2500 * time.Millisecond

// noticeLines is the most lines a notice takes of the board.
const noticeLines = 3

// noticeMsg comes when a notice ends: the frame drops it.
type noticeMsg struct{}

// noticeEnds is the command that says a notice ended. The notice's life
// runs on the console's clock (m.now), so tests, which set that clock,
// make this one come at once rather than wait noticeFor of wall time.
var noticeEnds = func(d time.Duration) tea.Cmd { return tea.Tick(d, func(time.Time) tea.Msg { return noticeMsg{} }) }

// noticeLife is how long a notice shows: noticeFor, or Options.NoticeFor.
func (m Model) noticeLife() time.Duration {
	if m.opt.NoticeFor > 0 {
		return m.opt.NoticeFor
	}
	return noticeFor
}

// notice is the one way to show a toast, a refusal or the data's age on
// the board: text takes the notice row for noticeLife. The header never
// shows it.
func (m *Model) notice(text string) tea.Cmd {
	m.noticeText, m.noticeUntil, m.goArmed, m.noticeNext = text, m.now().Add(m.noticeLife()), paneKey{}, ""
	return tea.Batch(noticeEnds(m.noticeLife()), m.stack(text, false))
}

// noticeEnded shows the notice that waits for the one that ends.
func (m *Model) noticeEnded() tea.Cmd {
	if m.noticeNext == "" || m.now().Before(m.noticeUntil) {
		return nil
	}
	return m.notice(m.noticeNext)
}

// noticeRows are a notice in w columns: one line, more on every shape
// when it does not fit, so its fix is never cut. Only a text longer than
// any of the board's own runs past noticeLines and ends in "…".
func (m Model) noticeRows(text string, w int) []string {
	lines := []string{text}
	if ansi.StringWidth(text) > w-1 {
		lines = upTo(cutNotice(text, w-1), noticeLines, w-1)
	}
	for i, l := range lines {
		lines[i] = fit(" "+m.st.text.Render(l), w)
	}
	return lines
}

// cutNotice breaks text into lines of w cells: at its last " · ", else
// at the end of its last sentence, where both halves fit; else at its
// spaces.
func cutNotice(text string, w int) []string {
	for _, sep := range []string{" · ", ". "} {
		keep := len(strings.TrimRight(sep, " "))
		for i := strings.LastIndex(text, sep); i >= 0; i = strings.LastIndex(text[:i], sep) {
			first, rest := text[:i+keep], text[i+len(sep):]
			if ansi.StringWidth(first) <= w && ansi.StringWidth(rest) <= w {
				return []string{first, rest}
			}
		}
	}
	return wrapPlain(text, w)
}

// standing are the errors that last, each with its fix, while they hold:
// the service is lost, its data is old, or the pull requests are read
// late. A toast says each once; then the header's mark counts them
// (#450).
func (m Model) standing() []string {
	var out []string
	switch age := m.now().Sub(m.snap.At); {
	case !m.have:
	case m.lost:
		out = append(out, "no service · it redials")
	case age >= staleAfter && age < 2*time.Minute:
		out = append(out, fmt.Sprintf("data is %d s old · the service is slow", int(age.Seconds())))
	case age >= staleAfter:
		out = append(out, fmt.Sprintf("data is %d min old · the service is slow", int(age.Minutes())))
	}
	return append(out, m.shipsBehind()...)
}

// standingKey is an error's words without its numbers: data that ages
// by the second is one error, said once.
func standingKey(text string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return -1
		}
		return r
	}, text)
}

// standingNews says each standing error once, when it begins: under what
// the message itself said, which names what happened to this act.
func (m Model) standingNews() Model {
	var keys []string
	for _, e := range m.standing() {
		k := standingKey(e)
		if !slices.Contains(m.standingSaid, k) {
			m.sayUnder(e)
		}
		keys = append(keys, k)
	}
	m.standingSaid = keys
	return m
}

// sayStanding says every standing error again: a tap on the mark.
func (m *Model) sayStanding() tea.Cmd {
	var cmds []tea.Cmd
	for _, e := range m.standing() {
		cmds = append(cmds, m.sayError(e))
	}
	return tea.Batch(cmds...)
}
