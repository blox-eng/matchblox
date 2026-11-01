package app

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// menuChat is the chat of a (%1) at w by h with the keys, at the question
// tool, its screen text in, past the quiet.
func menuChat(t *testing.T, w, h int, text string) (Model, *fakeConn) {
	t.Helper()
	m, f := askModel(t, w, threeWays)
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	next, _ = next.Update(screenMsg{Pane: "%1", At: m.now(), Text: text})
	return pastQuiet(next.(Model)), f
}

// rowWith is the line of the frame that holds s.
func rowWith(t *testing.T, m Model, s string) int {
	t.Helper()
	y := slices.IndexFunc(m.draw(m.width).lines, func(l string) bool { return strings.Contains(ansi.Strip(l), s) })
	if y < 0 {
		t.Fatalf("no %q on the screen:\n%s", s, screen(m))
	}
	return y
}

func tapped(t *testing.T, m Model, x, y int) Model {
	t.Helper()
	next, cmd := tap(m, x, y)
	drain(cmd)
	return next.(Model)
}

// TestATapPicksTheOption: at each survey size, a tap anywhere across an
// option's row sends its digit, as a key with the screen it answers.
func TestATapPicksTheOption(t *testing.T) {
	for _, size := range slices.Concat(surveyGrids, [][2]int{{44, 31}, {132, 32}, {200, 50}}) {
		w, h := size[0], size[1]
		if h-3 < menuLeast {
			continue
		}
		m := pastQuiet(flintAt(t, w, h, "enter"))
		y := rowWith(t, m, "2. Rotate, keep the old one a minute")
		f := m.conn.(*fakeConn)
		n := len(keyActs(f))
		got := tapped(t, m, w-1, y) // the far end of the row
		acts := keyActs(f)[n:]
		if len(acts) != 1 || acts[0].Text != "lit:2" || acts[0].Seen.IsZero() || got.flash != "" {
			t.Errorf("%dx%d: keys %+v flash %q", w, h, acts, got.flash)
		}
		if len(pickActs(f)) != 0 {
			t.Errorf("%dx%d: the tap went through pick: %+v", w, h, pickActs(f))
		}
	}
}

// proseOverMenu is an agent that printed a numbered list, over its real menu.
const proseOverMenu = "I weighed these:\n1. Patch the check\n2. Move the store\n3. Leave it\n\n" +
	"────────────────────────\n ☐ Fix\n\nWhich way?\n\n" +
	"❯ 1. Patch\n     Quick and small.\n  2. Move\n  3. Type something.\n" +
	"────────────────────────\n  4. Chat about this\n\nEnter to select · ↑/↓ to navigate · Esc to cancel\n"

// TestATapOnProseDoesNothing: a list the agent merely prints is no target,
// over a real menu or with none under it; the rows of the menu are.
func TestATapOnProseDoesNothing(t *testing.T) {
	m, f := menuChat(t, 100, 30, proseOverMenu)
	for _, s := range []string{"1. Patch the check", "2. Move the store", "I weighed these", "Which way?", "Enter to select"} {
		tapped(t, m, 80, rowWith(t, m, s))
		if len(keyActs(f)) != 0 {
			t.Fatalf("a tap on %q sent %+v", s, keyActs(f))
		}
	}
	got := tapped(t, m, 80, rowWith(t, m, "4. Chat about this")) // under the menu's inner rule
	if acts := keyActs(f); len(acts) != 1 || acts[0].Text != "lit:4" {
		t.Fatalf("the row under the inner rule: keys %+v flash %q", acts, got.flash)
	}

	for _, text := range []string{
		"I weighed these:\n1. Patch the check\n2. Move the store\n",
		"> 1. Patch the check\n> 2. Move the store\n",
		"Done:\n1. Patch\n2. Move\n\n────────\n❯ \n────────\n",
	} {
		m, f := menuChat(t, 100, 30, text)
		tapped(t, m, 80, rowWith(t, m, "2. Move"))
		if len(keyActs(f)) != 0 {
			t.Fatalf("a list with no menu in %q: keys %+v", text, keyActs(f))
		}
	}
}

// TestATapAtNoWaitDoesNothing: with no menu wait, a menu on the screen sends
// nothing either.
func TestATapAtNoWaitDoesNothing(t *testing.T) {
	st := cardState("toolu_1", threeWays)
	st.Sessions[0].Ask = nil
	m, f := loadedWith(t, 100, st)
	m = pressKeys(t, onPane(t, m, "%1"), "enter")
	next, _ := m.Update(screenMsg{Pane: "%1", At: m.now(), Text: askScreen})
	m = pastQuiet(next.(Model))
	tapped(t, m, 80, rowWith(t, m, "2. Rotate, keep"))
	if len(keyActs(f)) != 0 {
		t.Fatalf("keys %+v", keyActs(f))
	}
}

// TestATapBeforeTheScreenIsSeen: with no screen there is no row; in the quiet
// the tap is refused and says so; the service's refusal shows, and the row
// goes back.
func TestATapBeforeTheScreenIsSeen(t *testing.T) {
	m, f := loadedWith(t, 100, cardState("toolu_1", threeWays))
	m = pastQuiet(pressKeys(t, onPane(t, m, "%1"), "enter"))
	sc := m.draw(100)
	for y := sc.bodyTop; y < len(sc.lines)-2; y++ {
		tapped(t, m, 80, y)
	}
	if len(keyActs(f)) != 0 {
		t.Fatalf("no screen yet: keys %+v", keyActs(f))
	}

	m, f = menuChat(t, 100, 30, askScreen)
	m.menuSeen.at = m.now()
	got := tapped(t, m, 80, rowWith(t, m, "2. Rotate, keep"))
	if len(keyActs(f)) != 0 || got.flash != askedAgain("a") {
		t.Fatalf("in the quiet: keys %+v flash %q", keyActs(f), got.flash)
	}

	m, _ = menuChat(t, 100, 30, askScreen)
	plain := m.screenLines(99, 20)
	got = tapped(t, m, 80, rowWith(t, m, "2. Rotate, keep"))
	got = answered(t, got, "not sent: a asks something new — look again")
	if got.flash != askedAgain("a") {
		t.Fatalf("flash %q", got.flash)
	}
	if lines := got.screenLines(99, 20); !slices.Equal(lines, plain) {
		t.Fatalf("a refused tap keeps its wash:\n%q", lines)
	}
}

// TestAnOptionsOtherLinesAreItsTarget: its description, and a line the wrap
// made of it, press its digit.
func TestAnOptionsOtherLinesAreItsTarget(t *testing.T) {
	long := "2. Rotate the token now and keep the old one alive for one more minute"
	text := "Which way?\n\n❯ 1. Patch\n     The old one stops at once.\n  " + long + "\n  3. Type something.\n\nEnter to select\n"
	for _, c := range []struct {
		find, digit string
	}{{"The old one stops", "lit:1"}, {"more minute", "lit:2"}, {"3. Type", "lit:3"}} {
		m, f := menuChat(t, 44, 31, text)
		got := tapped(t, m, 2, rowWith(t, m, c.find))
		if acts := keyActs(f); len(acts) != 1 || acts[0].Text != c.digit {
			t.Fatalf("%q: keys %+v flash %q\n%s", c.find, acts, got.flash, screen(m))
		}
	}
	m, f := menuChat(t, 44, 31, text)
	tapped(t, m, 2, rowWith(t, m, "Enter to select"))
	if len(keyActs(f)) != 0 {
		t.Fatalf("the hint: keys %+v", keyActs(f))
	}
}

// TestATapInABoxPicks: a permission prompt in its box, with the boxes' edges.
func TestATapInABoxPicks(t *testing.T) {
	m, f := menuChat(t, 100, 30, permitScreen)
	tapped(t, m, 80, rowWith(t, m, "2. Yes, and don't ask again"))
	if acts := keyActs(f); len(acts) != 1 || acts[0].Text != "lit:2" {
		t.Fatalf("keys %+v", acts)
	}
}

// TestAMultiSelectTapTogglesAndItsSubmitRowSubmits: a digit only toggles, so
// the row under the options submits: the cursor goes down to it from where the
// screen shows it, then Enter. No other tap sends Enter.
func TestAMultiSelectTapTogglesAndItsSubmitRowSubmits(t *testing.T) {
	menu := tallestMenu(t)
	m, f := menuChat(t, 132, 32, menu)
	tapped(t, m, 90, rowWith(t, m, "2. [ ] Olives"))
	if acts := keyActs(f); len(acts) != 1 || acts[0].Text != "lit:2" {
		t.Fatalf("toggle: keys %+v", acts)
	}

	m, f = menuChat(t, 132, 32, menu)
	y := slices.IndexFunc(m.draw(132).lines, func(l string) bool { return strings.HasSuffix(strings.TrimRight(ansi.Strip(l), " "), " Submit") })
	if y < 0 {
		t.Fatalf("no Submit row:\n%s", screen(m))
	}
	m = tapped(t, m, 90, y)
	var got []string
	for range 6 {
		got = append(got, keyActs(f)[len(keyActs(f))-1].Text)
		m = answered(t, m, "")
	}
	if want := []string{"Down", "Down", "Down", "Down", "Down", "Enter"}; !slices.Equal(got, want) {
		t.Fatalf("submit: keys %q, want %q", got, want)
	}

	moved := strings.Replace(strings.Replace(menu, "❯ 1. [ ]", "  1. [ ]", 1), "  3. [ ]", "❯ 3. [ ]", 1)
	m, f = menuChat(t, 132, 32, moved)
	y = slices.IndexFunc(m.draw(132).lines, func(l string) bool { return strings.HasSuffix(strings.TrimRight(ansi.Strip(l), " "), " Submit") })
	m = tapped(t, m, 90, y)
	got = nil
	for range 3 {
		got = append(got, keyActs(f)[len(keyActs(f))-1].Text)
		m = answered(t, m, "")
	}
	if want := []string{"Down", "Down", "Down"}; !slices.Equal(got, want) {
		t.Fatalf("from the third row: keys %q, want %q", got, want)
	}
}

// TestATapWashesItsRowUntilTheNextScreen: the feedback of a button, on the
// tapped option's rows only, gone with the next screen.
func TestATapWashesItsRowUntilTheNextScreen(t *testing.T) {
	m, _ := menuChat(t, 100, 30, askScreen)
	plain := m.screenLines(99, 20)
	got := tapped(t, m, 80, rowWith(t, m, "2. Rotate, keep"))
	lines := got.screenLines(99, 20)
	at := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "2. Rotate, keep") })
	want := got.st.selected.Render(fit("  2. Rotate, keep the old one a minute", 99)) + ansi.ResetStyle
	if lines[at] != want {
		t.Fatalf("row %q, want %q", lines[at], want)
	}
	for i := range lines {
		if i != at && lines[i] != plain[i] {
			t.Fatalf("row %d changed: %q", i, lines[i])
		}
	}
	next, _ := got.Update(screenMsg{Pane: "%1", At: got.now(), Text: "⏺ Rotating."})
	for _, l := range next.(Model).screenLines(99, 20) {
		if strings.Contains(l, "\x1b[48") {
			t.Fatalf("the wash stayed: %q", l)
		}
	}
}

// TestEveryChatHintIsTappable: each hint of the chat's key line does what it
// says, on all of its words, at phone widths and beside a list, live and up
// the history.
func TestEveryChatHintIsTappable(t *testing.T) {
	for _, w := range []int{44, 51, 100} {
		for _, hint := range []string{"ctrl+] back", "ctrl+o into tmux"} {
			m, _ := menuChat(t, w, 31, askScreen)
			sc := m.draw(w)
			line := ansi.Strip(sc.lines[sc.keysAt])
			at := strings.Index(line, hint)
			if at < 0 {
				t.Fatalf("%d: no %q in %q", w, hint, line)
			}
			x := ansi.StringWidth(line[:at])
			for _, dx := range []int{0, ansi.StringWidth(hint) - 1} {
				got := tapped(t, m, x+dx, sc.keysAt)
				if hint == "ctrl+] back" && got.chatKeys {
					t.Errorf("%d: a tap at %d on %q kept the keys", w, dx, hint)
				}
				if hint == "ctrl+o into tmux" && got.pending == nil {
					t.Errorf("%d: a tap at %d on %q asks nothing", w, dx, hint)
				}
			}
		}
		m, _ := menuChat(t, w, 31, askScreen)
		m = wheelChat(t, m, true, 1)
		next, _ := m.Update(screenMsg{Pane: "%1", At: m.now(), Text: historyScreen, Up: 3})
		m = next.(Model)
		sc := m.draw(w)
		line := ansi.Strip(sc.lines[sc.keysAt])
		at := strings.Index(line, "ctrl+] back")
		if at < 0 {
			t.Fatalf("%d up: no back in %q", w, line)
		}
		if got := tapped(t, m, ansi.StringWidth(line[:at])+1, sc.keysAt); got.chatKeys {
			t.Errorf("%d up: a tap on back kept the keys", w)
		}
	}
}
