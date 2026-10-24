package app

import (
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

func sessionOn(t *testing.T, st proto.State, pane string) sample.Session {
	t.Helper()
	for _, s := range st.Sessions {
		if s.Pane == pane {
			return s
		}
	}
	t.Fatalf("no session on %s", pane)
	return sample.Session{}
}

func twoChildren(p sample.Session) []sample.Session {
	ref := &sample.ProcRef{PID: p.PID, Start: p.Start}
	return []sample.Session{
		{PID: 901, Start: 5, Agent: "codex", Name: "codex exec", Busy: true, Status: "busy", Parent: ref, Via: "process"},
		{PID: 902, Start: 6, Agent: "claude", Name: "claude -p", Busy: true, Status: "busy", Parent: ref, Via: "env"},
	}
}

// A child shows right under its parent with a branch, wide and on a phone.
func TestAChildShowsUnderItsParent(t *testing.T) {
	for _, w := range []int{120, 46} {
		st := queueState()
		p := sessionOn(t, st, "%1")
		st.Children = twoChildren(p)
		m, _ := loadedWith(t, w, st)
		out := screen(m)
		lines := strings.Split(out, "\n")
		// On a phone the selected row opens, so its children follow its block.
		row, first, next := lineOf(t, m, p.Name), lineOf(t, m, "├─ codex exec"), lineOf(t, m, "app-review")
		if row >= first || first+1 >= next || !strings.Contains(lines[first+1], "└─ claude -p") {
			t.Fatalf("w=%d: want %s, then ├─ codex exec, then └─ claude -p, then app-review in\n%s", w, p.Name, out)
		}
	}
}

// Under a last child the indent is blank; under one with siblings below, a
// bar; a child with no status shows its name alone.
func TestGrandchildrenIndentLikeATree(t *testing.T) {
	st := queueState()
	p := sessionOn(t, st, "%1")
	top := &sample.ProcRef{PID: p.PID, Start: p.Start}
	a, b := &sample.ProcRef{PID: 901, Start: 1}, &sample.ProcRef{PID: 902, Start: 2}
	st.Children = []sample.Session{
		{PID: 901, Start: 1, Name: "codex exec", Status: "busy", Parent: top},
		{PID: 902, Start: 2, Name: "claude -p", Status: "busy", Parent: top},
		{PID: 903, Start: 3, Name: "a1", Status: "busy", Parent: a},
		{PID: 904, Start: 4, Name: "a2", Status: "busy", Parent: a},
		{PID: 905, Start: 5, Name: "b1", Parent: b},
	}
	m, _ := loadedWith(t, 120, st)
	out := screen(m)
	lines := strings.Split(out, "\n")
	at := lineOf(t, m, "├─ codex exec")
	col := strings.Index(lines[at], "├─ codex exec") - 2
	// Each line is followed by blank columns: nothing trails the name or status.
	want := []string{"  ├─ codex exec busy", "  │ ├─ a1 busy", "  │ └─ a2 busy", "  └─ claude -p busy", "    └─ b1"}
	for i, w := range want {
		if l := lines[at+i]; len(l) < col || !strings.HasPrefix(l[col:], w+"  ") {
			t.Fatalf("line %d = %q, want %q in\n%s", i, l, w, out)
		}
	}
}

// No key acts on a child: down from a parent lands on the next session.
func TestTheCursorSkipsChildren(t *testing.T) {
	st := queueState()
	st.Children = twoChildren(sessionOn(t, st, "%1"))
	m, _ := loadedWith(t, 120, st)
	at := selectPane(t, m, "%1")
	next, _ := key(at, "down")
	s, ok := next.(Model).selectedSession()
	if !ok || s.Pane == "" || s.Pane == "%1" {
		t.Fatalf("selected %+v", s)
	}
}

func TestTheOpenedRowNamesChildren(t *testing.T) {
	st := queueState()
	st.Children = twoChildren(sessionOn(t, st, "%1"))
	m, _ := loadedWith(t, 160, st)
	out := screen(selectPane(t, m, "%1"))
	for _, want := range []string{"children", "codex exec · claude -p"} {
		if !strings.Contains(out, want) {
			t.Fatalf("no %q in\n%s", want, out)
		}
	}
}

func TestDeepChildrenFold(t *testing.T) {
	st := queueState()
	p := sessionOn(t, st, "%1")
	parent := &sample.ProcRef{PID: p.PID, Start: p.Start}
	for i := 1; i <= 6; i++ {
		pid := 900 + i
		st.Children = append(st.Children, sample.Session{PID: pid, Start: uint64(i), Name: "link" + string(rune('0'+i)), Busy: true, Status: "busy", Parent: parent})
		parent = &sample.ProcRef{PID: pid, Start: uint64(i)}
	}
	m, _ := loadedWith(t, 120, st)
	out := screen(m)
	if !strings.Contains(out, "… 2 more") || strings.Contains(out, "link5") {
		t.Fatalf("no fold in\n%s", out)
	}
}
