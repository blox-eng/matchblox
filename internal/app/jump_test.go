package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
)

// TestARowShowsItsNumber: a numbered session draws its number in a faint
// column left of the match; one with no number draws a space there, so the
// names stay aligned. The chosen row keeps its ▌.
func TestARowShowsItsNumber(t *testing.T) {
	s := boardMatch("hearth-api", time.Hour)
	s.Number = 3
	m := boardModel(t, boardState(s))
	numbered := m.all.sessions[0]
	bare := numbered
	bare.Number = 0

	row := m.sideRow(numbered, 40, false, sideLink{})[0]
	if !strings.HasPrefix(row, " "+m.st.faint.Render("3")+" ") {
		t.Fatalf("row %q, want a faint 3 after the mark", row)
	}
	got, want := ansi.Strip(m.sideRow(bare, 40, false, sideLink{})[0]), ansi.Strip(row)
	if i, j := strings.Index(got, "hearth-api"), strings.Index(want, "hearth-api"); i != j || !strings.HasPrefix(got, "   ") {
		t.Fatalf("a row with no number %q, want its name aligned with %q", got, want)
	}
	chosen := plainLines(m.sideRow(numbered, 40, true, sideLink{}))
	if !strings.HasPrefix(chosen[0], "▌3 ") {
		t.Fatalf("chosen row %q, want ▌ then 3", chosen[0])
	}
}

// numbered is the fixture machine with five sessions numbered 1 to 5, on
// the Sessions tab.
func numbered(t *testing.T) Model {
	t.Helper()
	var ss []sample.Session
	for i, name := range []string{"hearth-api", "ember-billing", "spark-docs", "wren-cli", "moss-web"} {
		s := boardMatch(name, time.Duration(i+1)*time.Hour)
		s.Number = i + 1
		ss = append(ss, s)
	}
	m, _, _ := sideShotOf(t, boardState(ss...), 160, 38)
	return m
}

func numberedAs(t *testing.T, m Model, n int) sample.Session {
	t.Helper()
	for _, s := range m.sideSessions() {
		if s.Number == n {
			return s
		}
	}
	t.Fatalf("no session numbered %d", n)
	return sample.Session{}
}

// TestAPhonesEscDigitTypesInTheLine: the phone has no ⌥ key, and tmux joins
// its Esc and a quick 1 into alt+1. On Pocket that types 1 in the line, or
// answers as the digit does; it never chooses another session.
func TestAPhonesEscDigitTypesInTheLine(t *testing.T) {
	m := numbered(t)
	pocketOnly(&m)
	m.chooseRow(numberedAs(t, m, 1))
	m.toLine()
	was := m.chosenSide()
	next, cmd := m.Update(tea.KeyPressMsg{Code: '3', Mod: tea.ModAlt})
	nm := fed(next, cmd).(Model)
	if nm.chosenSide() != was {
		t.Fatalf("ESC 3 on Pocket chose %q, want the choice kept", chosenSideName(nm))
	}
	if got := nm.lines[was]; got != "3" {
		t.Fatalf("ESC 3 on Pocket: the line holds %q, want 3", got)
	}
}

// TestAltDigitChoosesThatSession: ⌥3 chooses the session numbered 3, from
// the list and from the line, and the preview follows as after ↓.
func TestAltDigitChoosesThatSession(t *testing.T) {
	restNow(t)
	m := numbered(t)
	m.chooseRow(numberedAs(t, m, 1))
	for _, focus := range []string{"list", "line"} {
		f := m
		if focus == "list" {
			f.toList()
		} else {
			f.toLine()
		}
		next, _ := f.Update(tea.KeyPressMsg{Code: '3', Mod: tea.ModAlt})
		nm := next.(Model)
		if s, _ := nm.sideChosen(); s.Number != 3 {
			t.Errorf("%s: ⌥3 chose %q (number %d), want number 3", focus, s.Name, s.Number)
		}
		if nm.lines[nm.chosenSide()] != "" || nm.lines[m.chosenSide()] != "" {
			t.Errorf("%s: ⌥3 typed in a line: %v", focus, nm.lines)
		}
	}
	beside := m
	beside.beside = "%9"
	beside.toList()
	_, cmd := beside.Update(tea.KeyPressMsg{Code: '3', Mod: tea.ModAlt})
	if !slices.ContainsFunc(cmdMsgs(cmd), func(msg tea.Msg) bool { _, ok := msg.(previewMsg); return ok }) {
		t.Error("⌥3 beside the agent asked for no preview")
	}
}

// TestAltDigitOnTheChosenHost: in all hosts, each host has its own 3. ⌥3
// chooses the 3 of the chosen session's host.
func TestAltDigitOnTheChosenHost(t *testing.T) {
	num := func(n int) func(*sample.Session) { return func(s *sample.Session) { s.Number = n } }
	m, _ := twoHosts(t,
		hostState(sess(1, "hearth-api", "%1", num(1)), sess(3, "ember-billing", "%3", num(3))),
		hostState(sess(2, "spark-docs", "%2", num(1)), sess(4, "wren-cli", "%4", num(3))))
	m = consoleOn(m)
	for from, want := range map[string]string{"hearth-api": "ember-billing", "spark-docs": "wren-cli"} {
		f := m
		i := slices.IndexFunc(f.sideSessions(), func(s sample.Session) bool { return s.Name == from })
		if i < 0 {
			t.Fatalf("no %s in the list", from)
		}
		f.chooseRow(f.sideSessions()[i])
		f.toList()
		next, _ := f.Update(tea.KeyPressMsg{Code: '3', Mod: tea.ModAlt})
		if got := chosenSideName(next.(Model)); got != want {
			t.Errorf("⌥3 from %s chose %q, want %s", from, got, want)
		}
	}
}

// TestAltDigitWithNoSuchSession: ⌥7 with five sessions names the numbers
// there are, and the choice stays.
func TestAltDigitWithNoSuchSession(t *testing.T) {
	m := numbered(t)
	m.chooseRow(numberedAs(t, m, 2))
	m.toList()
	next, cmd := m.Update(tea.KeyPressMsg{Code: '7', Mod: tea.ModAlt})
	nm := fed(next, cmd).(Model)
	if nm.chosenSide() != m.chosenSide() {
		t.Errorf("⌥7 moved the choice to %q", chosenSideName(nm))
	}
	if want := "no session 7: the list numbers 1 to 5"; !strings.Contains(nm.noticeText, want) {
		t.Errorf("⌥7 said %q, want %q", nm.noticeText, want)
	}
}
