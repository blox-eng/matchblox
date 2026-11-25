package app

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/keycmd"
	"github.com/blox-eng/matchblox/internal/remote"
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

// numberedInPanes is numbered with session n in pane %n, and the console
// in %99.
func numberedInPanes(t *testing.T) Model {
	t.Helper()
	var ss []sample.Session
	for i, name := range []string{"hearth-api", "ember-billing", "spark-docs", "wren-cli", "moss-web"} {
		s := boardMatch(name, time.Duration(i+1)*time.Hour)
		s.Number, s.Pane = i+1, "%"+strconv.Itoa(i+1)
		ss = append(ss, s)
	}
	m, _, _ := sideShotOf(t, boardState(ss...), 160, 38)
	m.opt.Pane = "%99"
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

// jumpFrom is the jump APC ⌥ and digit n send from pane, for the client
// on tty, with everything the console then runs.
func jumpFrom(t *testing.T, m Model, n int, tty, pane string) (Model, [][]string) {
	t.Helper()
	var ran [][]string
	m.opt.Run = func(argv []string) error { ran = append(ran, argv); return nil }
	apc := keycmd.FlipPrefix("jump") + strconv.Itoa(n) + " " + tty + " " + pane + "\x1b\\"
	next, cmd := m.Update(uv.UnknownApcEvent(apc))
	return settled(next, cmd).(Model), ran
}

func ranText(ran [][]string) string {
	var s []string
	for _, argv := range ran {
		s = append(s, strings.Join(argv, " "))
	}
	return strings.Join(s, "\n")
}

// TestJumpFromAPhoneTypesInThePane: the phone has no ⌥ key, and tmux joins
// its Esc and a quick 4 into M-4. From the phone's client the console types
// Esc and 4 back into the pane they came from; the client stays there.
func TestJumpFromAPhoneTypesInThePane(t *testing.T) {
	m := numbered(t)
	pocketOnly(&m)
	from := numberedAs(t, m, 1).Pane
	_, ran := jumpFrom(t, m, 4, "/dev/pts/7", from)
	want := []string{"tmux", "send-keys", "-t", from, "Escape", "4"}
	if !slices.ContainsFunc(ran, func(argv []string) bool { return slices.Equal(argv, want) }) {
		t.Errorf("ran %q, want %q", ranText(ran), want)
	}
	if got := ranText(ran); strings.Contains(got, "switch-client") {
		t.Errorf("the phone's Esc 4 moved its client: %q", got)
	}
}

// TestJumpFromAPhoneInAnAgentsPaneTypesInThePane: a phone that tapped into
// an agent's pane no longer views the console, so the console's own list of
// its viewers misses it. Its pairing still names its tty: Esc and 4 go back
// into the pane, and the phone stays there.
func TestJumpFromAPhoneInAnAgentsPaneTypesInThePane(t *testing.T) {
	m := numberedInPanes(t)
	pocketOnly(&m)
	m.viewers = ""
	m, _ = m.tellBelt(false)
	m.opt.ClientList = func() (string, error) { return "100 /dev/pts/7 %1", nil }
	in := time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC)
	m.opt.Tmux = clientsMadeAt("/dev/pts/7", in.Add(time.Second))
	m.opt.Pair = &PairHost{
		DeviceTTYs: func() map[string]string { return map[string]string{"/dev/pts/7": "phone-id"} },
		DeviceIn:   func(id string) (time.Time, string, bool) { return in, "/dev/pts/7", id == "phone-id" },
	}
	_, ran := jumpFrom(t, m, 4, "/dev/pts/7", "%1")
	want := []string{"tmux", "send-keys", "-t", "%1", "Escape", "4"}
	if !slices.ContainsFunc(ran, func(argv []string) bool { return slices.Equal(argv, want) }) {
		t.Errorf("ran %q, want %q", ranText(ran), want)
	}
	if got := ranText(ran); strings.Contains(got, "switch-client") {
		t.Errorf("the phone's Esc 4 in an agent's pane moved its client: %q", got)
	}
}

// clientsMadeAt answers tmux list-clients with tty's client made at created.
func clientsMadeAt(tty string, created time.Time) func(args ...string) (string, error) {
	return func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "list-clients" {
			return strconv.FormatInt(created.Unix(), 10) + " " + tty + "\n", nil
		}
		return "", nil
	}
}

// TestJumpFromADeskOnAPhonesOldTTYJumps: a phone's note outlives the phone,
// and the system gives its tty to a desk terminal made later. That desk's
// ⌥4 in an agent's pane jumps; a client made just after the note is the
// phone's, and its Esc and 4 still go back into the pane.
func TestJumpFromADeskOnAPhonesOldTTYJumps(t *testing.T) {
	created := time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name  string
		note  time.Time
		phone bool
	}{
		{"a note from yesterday", created.Add(-24 * time.Hour), false},
		{"a note made with the client", created.Add(-time.Second), true},
	} {
		m := numberedInPanes(t)
		m.opt.ClientList = func() (string, error) { return "100 " + deskTTY + " %1", nil }
		m.opt.Tmux = clientsMadeAt(deskTTY, created)
		note := c.note
		m.opt.Pair = &PairHost{
			DeviceTTYs: func() map[string]string { return map[string]string{deskTTY: "phone-id"} },
			DeviceIn:   func(id string) (time.Time, string, bool) { return note, deskTTY, id == "phone-id" },
		}
		_, ran := jumpFrom(t, m, 4, deskTTY, "%1")
		got := ranText(ran)
		typed := strings.Contains(got, "send-keys -t %1 Escape 4")
		moved := slices.ContainsFunc(ran, func(argv []string) bool {
			tty, pane, ok := remote.FocusOf(argv)
			return ok && tty == deskTTY && pane == "%4"
		})
		if c.phone && (!typed || moved) {
			t.Errorf("%s: ran %q, want Esc 4 typed back and no jump", c.name, got)
		}
		if !c.phone && (typed || !moved) {
			t.Errorf("%s: ran %q, want a switch-client to %%4 and nothing typed", c.name, got)
		}
	}
}

// TestJumpFromAPaneGoesToThatSession: ⌥4 from a desk's pane switches that
// client to session 4's pane. The client is in the agent's pane, not at
// the console, and it still moves.
func TestJumpFromAPaneGoesToThatSession(t *testing.T) {
	m := numberedInPanes(t)
	m.opt.ClientList = func() (string, error) { return "100 " + deskTTY + " %1", nil }
	_, ran := jumpFrom(t, m, 4, deskTTY, "%1")
	if !slices.ContainsFunc(ran, func(argv []string) bool {
		tty, pane, ok := remote.FocusOf(argv)
		return ok && tty == deskTTY && pane == "%4"
	}) {
		t.Errorf("ran %q, want switch-client -c %s -t %%4", ranText(ran), deskTTY)
	}
	if got := ranText(ran); strings.Contains(got, "send-keys") {
		t.Errorf("a desk's ⌥4 typed in the pane: %q", got)
	}
}

// TestJumpFromAPaneWithNoSuchSession: ⌥7 with five sessions says so on the
// client that pressed it, in the console's words, and types nothing.
func TestJumpFromAPaneWithNoSuchSession(t *testing.T) {
	m := numbered(t)
	_, ran := jumpFrom(t, m, 7, deskTTY, numberedAs(t, m, 1).Pane)
	want := []string{"tmux", "display-message", "-c", deskTTY, "no session 7: the list numbers 1 to 5"}
	if len(ran) != 1 || !slices.Equal(ran[0], want) {
		t.Errorf("ran %q, want only %q", ranText(ran), want)
	}
}
