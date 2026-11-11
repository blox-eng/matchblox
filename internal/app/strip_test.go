package app

import (
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/limits"
)

// stripRow is the strip's row on the frame, plain: the lines between the
// body and the key line's hairline that the strip draws ("": none).
func stripRow(m Model) string {
	w := max(m.width, minWidth)
	lines, _, ok := m.strip(w)
	if !ok {
		return ""
	}
	frame := strings.Split(plainFrame(m), "\n")
	return strings.Join(frame[len(frame)-2-len(lines):len(frame)-2], "\n")
}

// toldStrip is a board whose u said its once-notice before.
func toldStrip(m *Model) { m.viewPrefs.Told = []string{"strip"} }

func TestTheStripTurnsOnUAndOnATap(t *testing.T) {
	m := boardShot(t, boardFixture(t), 80, 31, toldStrip)
	m.opt.StateDir = t.TempDir()
	if f := m.stripFace(); f != "usage" {
		t.Fatalf("the strip starts on %q, want usage", f)
	}
	want := []struct{ face, word string }{{"machine", "hide ›"}, {"off", ""}, {"usage", "machine ›"}}
	for _, w := range want {
		m = pressOn(m, "u")
		if f := m.stripFace(); f != w.face {
			t.Fatalf("u turns the strip to %q, want %q", f, w.face)
		}
		row := stripRow(m)
		if w.word == "" {
			if row != "" {
				t.Errorf("the hidden strip still draws %q", row)
			}
			continue
		}
		if !strings.HasSuffix(strings.TrimRight(row, " "), w.word) {
			t.Errorf("the %s strip %q does not end with %q", w.face, row, w.word)
		}
	}
	sc := m.draw(80)
	if sc.stripAt < 0 || sc.strip.x1 <= sc.strip.x0 {
		t.Fatalf("the strip has no tap: at %d, %+v", sc.stripAt, sc.strip)
	}
	next, cmd := tap(m, sc.strip.x0+3, sc.stripAt)
	drain(cmd)
	if f := next.(Model).stripFace(); f != "machine" {
		t.Errorf("a tap on the strip turns it to %q, want machine", f)
	}
}

func TestAHiddenStripGivesItsRowToTheBody(t *testing.T) {
	on := boardShot(t, boardFixture(t), 80, 31, nil)
	off := boardShot(t, boardFixture(t), 80, 31, func(m *Model) { m.viewPrefs.StripDesk = "off" })
	if a, b := len(on.boardPanel(80, on.bodyRows()).lines), len(off.boardPanel(80, off.bodyRows()).lines); a != b {
		t.Fatalf("the panel is %d lines with the strip and %d without", a, b)
	}
	rows := func(m Model) int { return m.boardRows(80, m.bodyRows(), false) }
	if a, b := rows(on), rows(off); b != a+1 {
		t.Errorf("the columns take %d rows with the strip and %d without, want one more", a, b)
	}
	if len(frameLines(off)) != 31 {
		t.Errorf("the frame without the strip is %d lines", len(frameLines(off)))
	}
}

func TestTheStripFollowsTheHostFilter(t *testing.T) {
	m := boardShot(t, boardFixture(t), 104, 38, func(m *Model) { m.viewPrefs.StripDesk = "machine" })
	home := m.nameOf(m.opt.Host)
	all := stripRow(m)
	if !strings.Contains(all, home) || !strings.Contains(all, "gpu-1") || strings.Contains(all, "mem") {
		t.Fatalf("with all hosts the machine strip is not each host's CPU: %q", all)
	}
	m.boardHost = "gpu-1"
	one := stripRow(m)
	if !strings.Contains(one, "gpu-1") || strings.Contains(one, home) || !strings.Contains(one, "mem") {
		t.Errorf("with gpu-1 chosen the strip is not gpu-1's CPU and memory: %q", one)
	}
}

// gpuAtPlanEdge puts the first window of gpu-1's first account at pct.
func gpuAtPlanEdge(pct float64) func(*Model) {
	return func(m *Model) {
		m.peers = slices.Clone(m.peers)
		i := m.peerAt("gpu-1")
		ls := slices.Clone(m.peers[i].st.Limits)
		j := slices.IndexFunc(ls, func(a limits.Account) bool { return len(a.Windows) > 0 })
		ls[j].Windows = slices.Clone(ls[j].Windows)
		ls[j].Windows[0].UsedPct = pct
		m.peers[i].st.Limits = ls
	}
}

func TestAPlanOnAnotherHostWarnsToo(t *testing.T) {
	m := boardShot(t, boardFixture(t), 104, 38, func(m *Model) {
		m.viewPrefs.StripDesk = "machine"
		gpuAtPlanEdge(95)(m)
	})
	lines, _, _ := m.strip(104)
	plain := ansi.Strip(lines[0])
	if !strings.Contains(lines[0], m.meter(95, 8, m.st.neg)) || !strings.HasPrefix(plain, " ✻ Claude Max @gpu-1  5h") {
		t.Errorf("a plan at 95%% on gpu-1 does not lead the usage strip with its host and the neg bar: %q", plain)
	}
	m.boardHost = m.nameOf(m.opt.Host)
	if row := stripRow(m); !strings.Contains(row, "cpu") || strings.Contains(row, "95%") {
		t.Errorf("with the filter on this machine, gpu-1's plan turns the strip: %q", row)
	}
}

func TestTheStateNoticeKeepsTheTap(t *testing.T) {
	m := boardShot(t, boardFixture(t), 80, 31, func(m *Model) {
		toldStrip(m)
		at := m.snap.At.Add(40 * time.Second)
		m.opt.Now = func() time.Time { return at }
	})
	m.opt.StateDir = t.TempDir()
	sc := m.draw(80)
	if sc.stripAt < 0 || sc.strip.press == "" {
		t.Fatalf("the row under old data takes no tap: at %d, %+v", sc.stripAt, sc.strip)
	}
	next, cmd := tap(m, 3, sc.stripAt)
	drain(cmd)
	m = next.(Model)
	if f := m.stripFace(); f != "machine" {
		t.Errorf("a tap on the state notice turns the strip to %q, want machine", f)
	}
	if row := stripRow(m); !strings.Contains(row, "data is 40 s old") {
		t.Errorf("the state notice does not stay on top: %q", row)
	}
}

// atPlanEdge puts the first window of the home host's first account at
// pct.
func atPlanEdge(pct float64) func(*Model) {
	return func(m *Model) {
		ls := slices.Clone(m.snap.Limits)
		i := slices.IndexFunc(ls, func(a limits.Account) bool { return len(a.Windows) > 0 })
		ls[i].Windows = slices.Clone(ls[i].Windows)
		ls[i].Windows[0].UsedPct = pct
		m.snap.Limits = ls
	}
}

func TestTheStripShowsAWarningByItself(t *testing.T) {
	m := boardShot(t, boardFixture(t), 104, 38, func(m *Model) {
		m.viewPrefs.StripDesk = "machine"
		toldStrip(m)
		atPlanEdge(91)(m)
	})
	m.opt.StateDir = t.TempDir()
	lines, _, _ := m.strip(104)
	if !strings.Contains(lines[0], m.meter(91, 8, m.st.neg)) || !strings.Contains(ansi.Strip(lines[0]), "91%") {
		t.Fatalf("a plan at 91%% does not turn the strip to usage with the neg bar: %q", ansi.Strip(lines[0]))
	}
	if !strings.HasSuffix(strings.TrimRight(stripRow(m), " "), "machine ›") {
		t.Errorf("the warned strip does not name the machine next: %q", stripRow(m))
	}
	m = pressOn(m, "u")
	if row := stripRow(m); !strings.Contains(row, "cpu") || strings.Contains(row, "91%") {
		t.Errorf("after u the owner's machine does not hold: %q", row)
	}
	st := boardFixture(t) // a new state, with the plan still at its edge
	st.Limits = m.snap.Limits
	if m = newBoardState(t, m, st); !strings.Contains(stripRow(m), "cpu") {
		t.Errorf("the owner's choice does not hold while the warning lasts: %q", stripRow(m))
	}
	calm := m // the warning ends; the next one turns the strip again
	atPlanEdge(50)(&calm)
	st.Limits = calm.snap.Limits
	m = newBoardState(t, m, st)
	m.viewPrefs.StripDesk = "machine"
	atPlanEdge(91)(&m)
	if row := stripRow(m); !strings.Contains(row, "91%") {
		t.Errorf("a new warning does not turn the strip: %q", row)
	}
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
	if row := stripRow(m); !strings.Contains(row, "copied hearth-api's last answer") {
		t.Errorf("the strip's row has no notice: %q", row)
	}
	now = now.Add(noticeFor)
	next, _ := m.Update(noticeMsg{}) // the notice's tick, delivered at once
	m = next.(Model)
	if row := stripRow(m); !strings.Contains(row, "machine ›") {
		t.Errorf("the strip is not back after %v: %q", noticeFor, row)
	}
	m.say("ran: git fetch") // the old console's toast
	if h := strings.Split(plainFrame(m), "\n")[0]; h != head || !strings.Contains(stripRow(m), "ran: git fetch") {
		t.Errorf("a toast on the board is not a notice: header %q, row %q", h, stripRow(m))
	}

	now = demoAt
	off := boardShot(t, boardFixture(t), 80, 31, func(m *Model) {
		m.opt.Now = func() time.Time { return now }
		m.viewPrefs.StripDesk = "off"
	})
	rows := off.boardRows(80, off.bodyRows(), false)
	off.notice("copied hearth-api's last answer")
	if row := stripRow(off); !strings.Contains(row, "copied") {
		t.Errorf("a hidden strip shows no row for the notice: %q", row)
	}
	if r := off.boardRows(80, off.bodyRows(), false); r != rows-1 {
		t.Errorf("the notice's row is not taken from the body: %d rows, then %d", rows, r)
	}
	now = now.Add(noticeFor)
	if row := stripRow(off); row != "" {
		t.Errorf("the notice's row stays on a hidden strip: %q", row)
	}
	if r := off.boardRows(80, off.bodyRows(), false); r != rows {
		t.Errorf("the body does not take the row back: %d rows, want %d", r, rows)
	}
}

func TestALongNoticeIsWholeOnAPhone(t *testing.T) {
	const text = "not copied: tmux drops it · tmux set -s set-clipboard on"
	m := boardShot(t, boardFixture(t), 40, 34, nil)
	m.notice(text)
	lines, _, _ := m.strip(40)
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
	if !strings.Contains(stripRow(m), second) {
		t.Errorf("the frame does not draw the notice's second line:\n%s", plainFrame(m))
	}
}

// fixedNotices are the notices the board says in words of its own: the
// copy's, the once-notices of the keys that changed, and the refusals.
var fixedNotices = []string{
	tmuxDrops, pasteHere, pasteWrong, pasteChecked,
	"copied: last message of ember-reports" + notChecked,
	"copied: 1234 characters" + notChecked,
	"c copies now. The stoker is on its card.",
	"v changes the cards now: verbose, or one line.",
	"s changes the host now. The order of a column is fixed.",
	"g shows one chain now. esc shows all.",
	"u turns the strip now.",
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
	"the stoker of chain login is no session: quit it on the STOKER tab",
	"kindle-legacy runs outside tmux: no pane to kill",
	"no step waits for your go on kindle-legacy",
	"tap y again to give the go",
	"the service does not answer: stop `matchblox serve`, then start matchblox again",
	"refused: a tmux move on gpu-1 never runs on this machine",
}

func TestEveryNoticeKeepsItsFix(t *testing.T) {
	for _, size := range [][2]int{{40, 34}, {80, 31}, {104, 38}, {160, 38}} {
		m := boardShot(t, boardFixture(t), size[0], size[1], nil)
		for _, text := range fixedNotices {
			m.notice(text)
			lines, _, _ := m.strip(size[0])
			var said []string
			for _, l := range lines {
				said = append(said, strings.TrimSpace(ansi.Strip(l)))
			}
			if got := strings.Join(said, " "); got != text {
				t.Errorf("%dx%d: the notice shows %q, want %q", size[0], size[1], said, text)
			}
			if row := stripRow(m); !strings.Contains(row, said[len(said)-1]) {
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
	if row := strings.TrimSpace(stripRow(m)); row != "data is 40 s old · the service is slow" {
		t.Errorf("the strip's row on old data is %q", row)
	}
	fresh := boardShot(t, boardFixture(t), 80, 31, nil)
	if row := stripRow(fresh); strings.Contains(row, "old") {
		t.Errorf("fresh data says it is old: %q", row)
	}
}

func TestTheStripIsItsWidth(t *testing.T) {
	cell := regexp.MustCompile(`\S+`)
	for _, face := range []string{"usage", "machine"} {
		wide := boardShot(t, boardFixture(t), 400, 38, func(m *Model) { m.viewPrefs.StripDesk = face })
		whole, _, _ := wide.strip(400)
		words := cell.FindAllString(ansi.Strip(whole[0]), -1)
		for _, w := range []int{40, 80, 104, 160} {
			m := boardShot(t, boardFixture(t), w, 38, func(m *Model) {
				m.viewPrefs.StripDesk, m.viewPrefs.StripPhone = face, face
			})
			lines, _, ok := m.strip(w)
			if !ok || len(lines) != 1 {
				t.Fatalf("%s at %d: %d lines, ok %v", face, w, len(lines), ok)
			}
			plain := ansi.Strip(lines[0])
			if got := ansi.StringWidth(lines[0]); got != w {
				t.Errorf("%s at %d is %d wide: %q", face, w, got, plain)
			}
			if strings.Contains(plain, "…") {
				t.Errorf("%s at %d is cut: %q", face, w, plain)
			}
			for _, word := range cell.FindAllString(plain, -1) {
				if word != "·" && !slices.Contains(words, word) {
					t.Errorf("%s at %d has %q, a cut value: %q", face, w, word, plain)
				}
			}
		}
	}
}
