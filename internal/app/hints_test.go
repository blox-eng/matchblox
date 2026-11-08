package app

import (
	"strings"
	"testing"
)

func TestHintForUsesBinding(t *testing.T) {
	k := defaultKeys()
	for _, c := range []struct {
		name string
		got  hint
		want hint
	}{
		{"open", hintFor(k.main.Open, "session"), hint{"⏎", "session", "enter"}},
		{"default label", hintFor(k.main.Sort, ""), hint{"s", "sort", "s"}},
		{"mark", hintFor(k.main.Mark, ""), hint{"␣", "mark", "space"}},
		{"upper", hintFor(k.main.New, ""), hint{"N", "new", "N"}},
		{"esc without a word", hintFor(k.form.Close, ""), hint{"esc", "", "esc"}},
		{"tab digit", tabHint(k.main.Tabs, 0, "sessions"), hint{"1", "sessions", "1"}},
	} {
		if c.got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, c.got, c.want)
		}
	}
}

func TestFootForSkipsDisabled(t *testing.T) {
	m := Model{}
	m.st = newStyles(true)
	k := defaultKeys()
	if f := m.st.footFor(k.form.Enter, "rename", " ─ "); f.press != "enter" || f.sep != " ─ " || f.text == "" {
		t.Errorf("footFor = %+v", f)
	}
	off := k.main.Open
	off.SetEnabled(false)
	if h := hintFor(off, "open"); h != (hint{}) {
		t.Errorf("a disabled binding gave the hint %+v", h)
	}
	if f := m.st.footFor(off, "open", " ─ "); f != (footPart{}) {
		t.Errorf("a disabled binding gave the part %+v", f)
	}
}

func TestDisabledTabHintAndZeroSkips(t *testing.T) {
	k := defaultKeys()
	off := k.main.Tabs
	off.SetEnabled(false)
	if h := tabHint(off, 0, "sessions"); h != (hint{}) {
		t.Errorf("a disabled tabs binding gave %+v", h)
	}
	if h := tabHint(k.main.Tabs, len(k.main.Tabs.Keys()), "x"); h != (hint{}) {
		t.Errorf("a tab past the last gave %+v", h)
	}
	m := Model{keys: k}
	m.st = newStyles(true)
	with, _ := m.hintRow([]hint{hintFor(k.main.Open, "")}, 2)
	skipped, _ := m.hintRow([]hint{{}, hintFor(k.main.Open, ""), {}}, 2)
	if with != skipped {
		t.Errorf("hintRow drew a zero hint: %q, want %q", skipped, with)
	}
	part := m.st.footFor(k.form.Enter, "start", "")
	a, _ := boxFrame(m.st.hair, "t", []string{"x"}, []footPart{part}, 30)
	b, _ := boxFrame(m.st.hair, "t", []string{"x"}, []footPart{{}, part, {}}, 30)
	if strings.Join(a, "\n") != strings.Join(b, "\n") {
		t.Errorf("boxFrame drew a zero part:\n%s\nwant\n%s", strings.Join(b, "\n"), strings.Join(a, "\n"))
	}
}

// TestOneChatHintAndTheWayOutOfListOnly: the key line names the chat once,
// as ⏎, and only where Enter opens it; a phone in list only keeps the way
// back to the full view.
func TestOneChatHintAndTheWayOutOfListOnly(t *testing.T) {
	m := flintAt(t, 51, 31)
	if !m.listOnly() {
		t.Fatal("51x31 does not start list only")
	}
	line := keyText(m.draw(51))
	if !strings.Contains(line, "full view v") || strings.Contains(line, "a chat") || !strings.Contains(line, "⏎ go in") {
		t.Errorf("51x31 key line %q", line)
	}
	if out := screen(m); strings.Contains(out, "a chat") {
		t.Errorf("the opened row names the chat twice:\n%s", out)
	}
	for _, size := range [][2]int{{96, 8}, {106, 10}} { // too short for flint-auth's menu: Enter only refuses
		m := fullView(flintAt(t, size[0], size[1]))
		if line := keyText(m.draw(size[0])); strings.Contains(line, "chat") {
			t.Errorf("%v key line %q offers a chat Enter refuses", size, line)
		}
	}
	st := screensState(t)
	for i := range st.Sessions {
		if st.Sessions[i].Name == "flint-auth" {
			st.Sessions[i].Pane = "" // outside tmux: nothing to type into
		}
	}
	m = fullView(sized(t, 200, 50, st))
	for _, s := range m.snap.Sessions {
		if s.Name == "flint-auth" {
			m.chose, m.selPID = true, s.PID
		}
	}
	if line := keyText(m.draw(200)); strings.Contains(line, "chat") {
		t.Errorf("a session outside tmux: key line %q", line)
	}
}
