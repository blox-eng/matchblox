package app

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// boardNoticeRow is what the Sessions tab says over the list's foot and
// the chat's line, "" for nothing.
func boardNoticeRow(m Model) string {
	sep := "\n"
	if m.desk() { // the list's foot wraps a notice to the list's width
		sep = " "
	}
	return strings.Join(saidRows(m, max(m.width, minWidth)), sep)
}

func TestANoticeNeverHidesTheCount(t *testing.T) {
	now := demoAt
	m := boardShot(t, boardFixture(t), 80, 31, func(m *Model) { m.opt.Now = func() time.Time { return now } })
	head := strings.Split(plainFrame(m), "\n")[0]
	if !strings.Contains(head, "● 4 need you") || !strings.Contains(barLine(m), "2 hosts s▸") { // all hosts: the bar's chips
		t.Fatalf("the header before the notice: %q, the bar %q", head, barLine(m))
	}
	m.notice("copied hearth-api's last answer")
	if h := strings.Split(plainFrame(m), "\n")[0]; h != head {
		t.Errorf("the header changed for a notice:\n%q\n%q", head, h)
	}
	if row := boardNoticeRow(m); !strings.Contains(row, "copied hearth-api's last answer") {
		t.Errorf("the notice row has no notice: %q", row)
	}
	now = now.Add(noticeFor)
	next, _ := m.Update(noticeMsg{}) // the notice's tick, delivered at once
	m = next.(Model)
	if row := boardNoticeRow(m); row != "" {
		t.Errorf("the notice row stays after %v: %q", noticeFor, row)
	}
	m.say("ran: git fetch") // the old console's toast
	if h := strings.Split(plainFrame(m), "\n")[0]; h != head || !strings.Contains(boardNoticeRow(m), "ran: git fetch") {
		t.Errorf("a toast is not a notice: header %q, row %q", h, boardNoticeRow(m))
	}
}

func TestALongNoticeIsWholeOnAPhone(t *testing.T) {
	const text = "not sent: flint-auth is on gpu-1 · ctrl+o goes in to answer it"
	m := boardShot(t, boardFixture(t), 40, 34, nil)
	m.notice(text)
	lines := saidRows(m, 40)
	if len(lines) != 2 {
		t.Fatalf("the notice is %d lines on a phone, want 2: %q", len(lines), lines)
	}
	first, second := lines[0], lines[1]
	if first+" "+second != text {
		t.Errorf("a word is lost or cut: %q / %q", first, second)
	}
	if !strings.HasSuffix(first, "·") && !strings.HasSuffix(first, " ") && strings.Contains(text, first+" ") {
		t.Errorf("the cut is not at a · or a space: %q", first)
	}
	if first != "not sent: flint-auth is on gpu-1 ·" {
		t.Errorf("the cut is not at the ·: %q / %q", first, second)
	}
	for _, l := range strings.Split(m.render(), "\n") {
		if ansi.StringWidth(l) != 40 {
			t.Errorf("a line with the notice is %d wide: %q", ansi.StringWidth(l), ansi.Strip(l))
		}
	}
	if !strings.Contains(plainFrame(m), second) {
		t.Errorf("the frame does not draw the notice's second line:\n%s", plainFrame(m))
	}
}

// fixedNotices are the notices the board says in words of its own: the
// copy's, the once-notices of the keys that changed, and the refusals.
var fixedNotices = []string{
	"not copied: " + errNoScreen.Error(), "Now paste here · esc cancels", pasteWrong, pasteChecked,
	"copied: last message of ember-reports" + notChecked,
	"copied: 1234 characters" + notChecked,
	"c copies now: the last message, whole.",
	"v changes the cards now: verbose, or one line.",
	"g shows one chain now. esc shows all.",
	"p merges now: the pull request that waits for you.",
	"x kills. p merges #4556.",
	"A digit marks an answer now. Press it again to send.",
	"not sent: flint-auth is on gpu-1 · ctrl+o goes in",
	"not sent: no service · it redials",
	"not sent: ember-reports asks something new — look again",
	"look at ember-reports's question first",
	"no card is chosen · ↓ goes back to the cards",
	"no card is chosen",
	"nothing to copy: no card is chosen",
	"nothing to copy: kindle-legacy said nothing yet",
	"no ship of kindle-legacy waits for a merge",
	"the stoker of chain login is no session: Q quits it on Machine › stoker",
	"kindle-legacy runs outside tmux: no pane to kill",
	"no step waits for your go on kindle-legacy",
	"tap y again to give the go",
	"the service does not answer: stop `matchblox serve`, then start matchblox again",
	"refused: a tmux move on gpu-1 never runs on this machine",
	"ember-reports asks in its pane · ctrl+o goes in",
	"ember-reports asks no question",
	"1-2 only",
}

func TestEveryNoticeKeepsItsFix(t *testing.T) {
	for _, size := range [][2]int{{40, 34}, {80, 31}, {104, 38}, {160, 38}} {
		m := boardShot(t, boardFixture(t), size[0], size[1], nil)
		for _, text := range fixedNotices {
			m.saying = nil
			m.notice(text)
			said := saidRows(m, size[0])
			if got := strings.Join(said, " "); got != text {
				t.Errorf("%dx%d: the notice shows %q, want %q", size[0], size[1], said, text)
				continue
			}
			if frame := plainFrame(m); !strings.Contains(frame, said[len(said)-1]) {
				t.Errorf("%dx%d: the frame does not draw the notice's last line %q:\n%s", size[0], size[1], said[len(said)-1], frame)
			}
		}
	}
}

func TestOldDataSaysSo(t *testing.T) {
	m := boardShot(t, boardFixture(t), 80, 31, func(m *Model) {
		at := m.snap.At.Add(40 * time.Second)
		m.opt.Now = func() time.Time { return at }
	})
	m = upd(t, m, noticeMsg{}) // the next message: in use, the stale tick
	if said := toastNow(m); said != "data is 40 s old · the service is slow" {
		t.Errorf("the toast on old data is %q", said)
	}
	if head := strings.Split(plainFrame(m), "\n")[0]; !strings.Contains(head, "⚠ 1") {
		t.Errorf("the header does not mark old data: %q", head)
	}
	fresh := boardShot(t, boardFixture(t), 80, 31, nil)
	if row := boardNoticeRow(fresh); strings.Contains(row, "old") {
		t.Errorf("fresh data says it is old: %q", row)
	}
}
