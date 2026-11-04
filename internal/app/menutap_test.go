package app

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

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
		for _, hint := range []string{"ctrl+a m back", "ctrl+o into tmux"} {
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
				if hint == "ctrl+a m back" && got.chatKeys {
					t.Errorf("%d: a tap at %d on %q kept the keys", w, dx, hint)
				}
				if hint == "ctrl+o into tmux" && got.pending == nil {
					t.Errorf("%d: a tap at %d on %q asks nothing", w, dx, hint)
				}
			}
		}
	}
}

// Two pages of one question, as Claude Code draws them: the next page comes
// within 150 ms, with its options on the same rows.
const (
	fruitPage = "Pick them.\n────────\n←  ☐ Fruit  ☐ Colour  ✔ Submit  →\n\nPick a fruit?\n\n" +
		"❯ 1. Apple\n     Crisp and sweet.\n  2. Banana\n     Soft and easy to peel.\n  3. Type something.\n" +
		"────────\n  4. Chat about this\n\nEnter to select · Tab/Arrow keys to navigate · Esc to cancel\n"
	colourPage = "Pick them.\n────────\n←  ☒ Fruit  ☐ Colour  ✔ Submit  →\n\nPick a colour?\n\n" +
		"❯ 1. Red\n     A warm, bold shade.\n  2. Blue\n     A cool, calm shade.\n  3. Type something.\n" +
		"────────\n  4. Chat about this\n\nEnter to select · Tab/Arrow keys to navigate · Esc to cancel\n"
)

// shownAt is m a moment later with text on the screen.
func shownAt(m Model, d time.Duration, text string) Model {
	at := m.now().Add(d)
	m.opt.Now = func() time.Time { return at }
	next, _ := m.Update(screenMsg{Pane: "%1", At: at, Text: text})
	return next.(Model)
}

// TestATapOnANewPageWaits: the second tap of a double tap, on the row the next
// page drew under it, must not answer a question nobody has read.
func TestATapOnANewPageWaits(t *testing.T) {
	m, f := menuChat(t, 100, 30, fruitPage)
	y := rowWith(t, m, "1. Apple")
	m = answered(t, tapped(t, m, 90, y), "")
	m = shownAt(m, 150*time.Millisecond, colourPage)
	if got := rowWith(t, m, "1. Red"); got != y {
		t.Fatalf("Red on row %d, Apple was on %d", got, y)
	}
	n := len(keyActs(f))
	got := tapped(t, m, 90, y)
	if acts := keyActs(f)[n:]; len(acts) != 0 || got.flash != askedAgain("a") {
		t.Fatalf("second tap: keys %+v flash %q", acts, got.flash)
	}
	got = tapped(t, pastQuiet(got), 90, y)
	if acts := keyActs(f)[n:]; len(acts) != 1 || acts[0].Text != "lit:1" {
		t.Fatalf("past the quiet: keys %+v flash %q", acts, got.flash)
	}
}

// TestTheSamePageKeepsItsQuietOver: a tick, the cursor or typed text change the
// screen, not the page, so the next key goes at once.
func TestTheSamePageKeepsItsQuietOver(t *testing.T) {
	menu := tallestMenu(t)
	ticked := strings.Replace(menu, "2. [ ] Olives", "2. [✔] Olives", 1)
	moved := strings.Replace(strings.Replace(menu, "❯ 1. [ ]", "  1. [ ]", 1), "  2. [ ] Olives", "❯ 2. [ ] Olives", 1)
	typed := strings.Replace(strings.Replace(menu, "❯ 1. [ ]", "  1. [ ]", 1), "  5. [ ] Type something", "❯ 5. [ ] hi", 1)
	for _, text := range []string{ticked, moved, typed, menu} {
		m, f := menuChat(t, 132, 32, menu)
		m = shownAt(m, 50*time.Millisecond, text)
		got := pressKeys(t, m, "down")
		if acts := keyActs(f); len(acts) != 1 || got.flash != "" {
			t.Fatalf("keys %+v flash %q on:\n%s", acts, got.flash, text)
		}
	}
}

const typingMenu = "Which door?\n\n  1. Red\n     The red door.\n  2. Blue\n     The blue door.\n❯ 3. %s\n" +
	"────────\n  4. Chat about this\n\nEnter to select · ↑/↓ to navigate · Esc to cancel\n"

// TestATapFromTheTypingRowPicks: with the cursor on the row that takes text, a
// digit would land in its field, so the cursor leaves it first. A tap on that
// row itself sends nothing: a digit there is text too.
func TestATapFromTheTypingRowPicks(t *testing.T) {
	for _, field := range []string{"Type something.", "hi"} {
		text := fmt.Sprintf(typingMenu, field)
		m, f := menuChat(t, 100, 30, text)
		m = tapped(t, m, 90, rowWith(t, m, "1. Red"))
		if acts := keyActs(f); len(acts) != 1 || acts[0].Text != "Up" {
			t.Fatalf("%q: first key %+v flash %q", field, acts, m.flash)
		}
		m = answered(t, m, "")
		if acts := keyActs(f); len(acts) != 2 || acts[1].Text != "lit:1" {
			t.Fatalf("%q: keys %+v flash %q", field, acts, m.flash)
		}

		m, f = menuChat(t, 100, 30, text)
		tapped(t, m, 90, rowWith(t, m, "3. "+field))
		if acts := keyActs(f); len(acts) != 0 {
			t.Fatalf("%q: a tap on the typing row sent %+v", field, acts)
		}
	}
}

// TestAPermissionsLastRowIsNoTypingRow: a menu with no inner rule has no field,
// so a digit there still picks at once.
func TestAPermissionsLastRowIsNoTypingRow(t *testing.T) {
	text := "Do you want to proceed?\n  1. Yes\n  2. Yes, and don't ask again\n❯ 3. No\n\nEsc to cancel\n"
	m, f := menuChat(t, 100, 30, text)
	tapped(t, m, 90, rowWith(t, m, "1. Yes"))
	if acts := keyActs(f); len(acts) != 1 || acts[0].Text != "lit:1" {
		t.Fatalf("keys %+v", acts)
	}
}

// TestTheFirstClickOnASideChatOnlyFocuses: a click that gives the chat the keys
// answers nothing, whatever row it lands on, and a quick second one waits too.
func TestTheFirstClickOnASideChatOnlyFocuses(t *testing.T) {
	m, f := loadedWith(t, 140, cardState("toolu_1", threeWays))
	m = onPane(t, m, "%1")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	next, _ = next.Update(screenMsg{Pane: "%1", At: m.now(), Text: permitScreen})
	m = pastQuiet(next.(Model))
	if m.chatKeys {
		t.Fatal("the chat has the keys already")
	}
	y := rowWith(t, m, "2. Yes, and don't ask again")
	got := tapped(t, m, 110, y)
	if !got.chatKeys || len(keyActs(f)) != 0 {
		t.Fatalf("chatKeys %v keys %+v", got.chatKeys, keyActs(f))
	}
	got = tapped(t, got, 110, y)
	if len(keyActs(f)) != 0 || got.flash != askedAgain("a") {
		t.Fatalf("a double click: keys %+v flash %q", keyActs(f), got.flash)
	}
	tapped(t, pastQuiet(got), 110, y)
	if acts := keyActs(f); len(acts) != 1 || acts[0].Text != "lit:2" {
		t.Fatalf("the next click: keys %+v", acts)
	}
}
