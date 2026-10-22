package app

import "testing"

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
