package app

import (
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

func sessionOn(st proto.State, pane string) sample.Session {
	for _, s := range st.Sessions {
		if s.Pane == pane {
			return s
		}
	}
	return sample.Session{}
}

func twoChildren(p sample.Session) []sample.Session {
	ref := &sample.ProcRef{PID: p.PID, Start: p.Start}
	return []sample.Session{
		{PID: 901, Start: 5, Agent: "codex", Name: "codex exec", Busy: true, Status: "busy", Parent: ref, Via: "process"},
		{PID: 902, Start: 6, Agent: "claude", Name: "claude -p", Busy: true, Status: "busy", Parent: ref, Via: "env"},
	}
}

// A child shows under its parent with a branch, wide and on a phone.
func TestAChildShowsUnderItsParent(t *testing.T) {
	for _, w := range []int{120, 46} {
		st := queueState()
		st.Children = twoChildren(sessionOn(st, "%1"))
		m, _ := loadedWith(t, w, st)
		out := screen(m)
		for _, want := range []string{"├─ codex exec", "└─ claude -p"} {
			if !strings.Contains(out, want) {
				t.Fatalf("w=%d: no %q in\n%s", w, want, out)
			}
		}
	}
}

// No key acts on a child: down from a parent lands on the next session.
func TestTheCursorSkipsChildren(t *testing.T) {
	st := queueState()
	st.Children = twoChildren(sessionOn(st, "%1"))
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
	st.Children = twoChildren(sessionOn(st, "%1"))
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
	p := sessionOn(st, "%1")
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
