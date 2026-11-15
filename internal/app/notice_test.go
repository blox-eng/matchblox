package app

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// boardNoticeRow is the board's notice row on the frame, plain: the lines
// between the body and the key line's hairline that the notice draws ("":
// none).
func boardNoticeRow(m Model) string {
	lines, _ := m.boardNotice(max(m.width, minWidth))
	if len(lines) == 0 {
		return ""
	}
	frame := strings.Split(plainFrame(m), "\n")
	return strings.Join(frame[len(frame)-2-len(lines):len(frame)-2], "\n")
}

func TestANoticeNeverHidesTheCount(t *testing.T) {
	now := demoAt
	m := boardShot(t, boardFixture(t), 80, 31, func(m *Model) { m.opt.Now = func() time.Time { return now } })
	head := strings.Split(plainFrame(m), "\n")[0]
	if !strings.Contains(head, "● 4 need you") || !strings.Contains(head, "all hosts ▾") {
		t.Fatalf("the header before the notice: %q", head)
	}
	m.notice("copied hearth-api's last answer")
	if h := strings.Split(plainFrame(m), "\n")[0]; h != head {
		t.Errorf("the header changed for a notice:\n%q\n%q", head, h)
	}
	if row := boardNoticeRow(m); !strings.Contains(row, "copied hearth-api's last answer") {
		t.Errorf("the notice row has no notice: %q", row)
	}
	rows := m.boardRows(80, m.bodyRows(), false)
	now = now.Add(noticeFor)
	next, _ := m.Update(noticeMsg{}) // the notice's tick, delivered at once
	m = next.(Model)
	if row := boardNoticeRow(m); row != "" {
		t.Errorf("the notice row stays after %v: %q", noticeFor, row)
	}
	if r := m.boardRows(80, m.bodyRows(), false); r != rows+1 {
		t.Errorf("the body does not take the notice's row back: %d rows, then %d", rows, r)
	}
	m.say("ran: git fetch") // the old console's toast
	if h := strings.Split(plainFrame(m), "\n")[0]; h != head || !strings.Contains(boardNoticeRow(m), "ran: git fetch") {
		t.Errorf("a toast on the board is not a notice: header %q, row %q", h, boardNoticeRow(m))
	}
}

func TestALongNoticeIsWholeOnAPhone(t *testing.T) {
	const text = "not copied: tmux drops it · tmux set -s set-clipboard on"
	m := boardShot(t, boardFixture(t), 40, 34, nil)
	m.notice(text)
	lines, _ := m.boardNotice(40)
	if len(lines) != 2 {
		t.Fatalf("the notice is %d lines on a phone, want 2: %q", len(lines), plainLines(lines))
	}
	first, second := strings.TrimSpace(ansi.Strip(lines[0])), strings.TrimSpace(ansi.Strip(lines[1]))
	if first+" "+second != text {
		t.Errorf("a word is lost or cut: %q / %q", first, second)
	}
	if !strings.HasSuffix(first, "·") && !strings.HasSuffix(first, " ") && strings.Contains(text, first+" ") {
		t.Errorf("the cut is not at a · or a space: %q", first)
	}
	if first != "not copied: tmux drops it ·" {
		t.Errorf("the cut is not at the ·: %q / %q", first, second)
	}
	for _, l := range lines {
		if ansi.StringWidth(l) != 40 {
			t.Errorf("a notice line is %d wide: %q", ansi.StringWidth(l), ansi.Strip(l))
		}
	}
	if !strings.Contains(boardNoticeRow(m), second) {
		t.Errorf("the frame does not draw the notice's second line:\n%s", plainFrame(m))
	}
}

// fixedNotices are the notices the board says in words of its own: the
// copy's, the once-notices of the keys that changed, and the refusals.
var fixedNotices = []string{
	tmuxDrops, "Now paste here · esc cancels", pasteWrong, pasteChecked,
	"copied: last message of ember-reports" + notChecked,
	"copied: 1234 characters" + notChecked,
	"c copies now. The stoker is on its card.",
	"v changes the cards now: verbose, or one line.",
	"s changes the host now. The order of a column is fixed.",
	"g shows one chain now. esc shows all.",
	"p merges now. The ships are on their cards.",
	"x kills. p merges #4556.",
	"A digit marks an answer now. Press it again to send.",
	"not sent: flint-auth is on gpu-1 · ⏎ goes in",
	"not sent: no service · it redials",
	"not sent: ember-reports asks something new — look again",
	"look at ember-reports's question first",
	"no card is chosen · ↓ goes back to the cards",
	"no card is chosen · tap a + name to show a column",
	"nothing to copy: no card is chosen",
	"nothing to copy: kindle-legacy said nothing yet",
	"no ship of kindle-legacy waits for a merge",
	"the stoker of chain login is no session: Q quits it on Machine › stoker",
	"kindle-legacy runs outside tmux: no pane to kill",
	"no step waits for your go on kindle-legacy",
	"tap y again to give the go",
	"the service does not answer: stop `matchblox serve`, then start matchblox again",
	"refused: a tmux move on gpu-1 never runs on this machine",
	"ember-reports asks in its pane · ⏎ goes in",
	"ember-reports asks no question",
	"1-2 only",
}

func TestEveryNoticeKeepsItsFix(t *testing.T) {
	for _, size := range [][2]int{{40, 34}, {80, 31}, {104, 38}, {160, 38}} {
		m := boardShot(t, boardFixture(t), size[0], size[1], nil)
		for _, text := range fixedNotices {
			m.notice(text)
			lines, _ := m.boardNotice(size[0])
			var said []string
			for _, l := range lines {
				said = append(said, strings.TrimSpace(ansi.Strip(l)))
			}
			if got := strings.Join(said, " "); got != text {
				t.Errorf("%dx%d: the notice shows %q, want %q", size[0], size[1], said, text)
			}
			if row := boardNoticeRow(m); !strings.Contains(row, said[len(said)-1]) {
				t.Errorf("%dx%d: the frame does not draw the notice's last line %q:\n%s", size[0], size[1], said[len(said)-1], row)
			}
		}
	}
}

func TestOldDataSaysSo(t *testing.T) {
	m := boardShot(t, boardFixture(t), 80, 31, func(m *Model) {
		at := m.snap.At.Add(40 * time.Second)
		m.opt.Now = func() time.Time { return at }
	})
	if row := strings.TrimSpace(boardNoticeRow(m)); row != "data is 40 s old · the service is slow" {
		t.Errorf("the notice row on old data is %q", row)
	}
	fresh := boardShot(t, boardFixture(t), 80, 31, nil)
	if row := boardNoticeRow(fresh); strings.Contains(row, "old") {
		t.Errorf("fresh data says it is old: %q", row)
	}
}
