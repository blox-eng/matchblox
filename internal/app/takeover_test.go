package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
)

func footerOf(m tea.Model) string {
	mm := m.(Model)
	return ansi.Strip(mm.footer(max(mm.width, minWidth)))
}

func recIDs(acts []proto.Act) []string {
	var out []string
	for _, a := range acts {
		out = append(out, a.RecID)
	}
	return out
}

func TestTheFirstActingKeyAsksToTakeOver(t *testing.T) {
	for _, w := range []int{40, 120} {
		m, f := loadedWith(t, w, nightState())
		next := sendKey(m, "enter")
		if a := f.acts(); len(a) != 0 {
			t.Fatalf("w=%d: sent %+v before the answer", w, a)
		}
		if got := footerOf(next); !strings.Contains(got, "Take over?") || !strings.Contains(got, "y") || !strings.Contains(got, "n") {
			t.Fatalf("w=%d: footer %q", w, got)
		}
		if next.(Model).pending != nil {
			t.Fatalf("w=%d: the held key ran", w)
		}
	}
}

func TestYesRestsTheStokerAndRunsTheKey(t *testing.T) {
	m, f := loadedWith(t, 120, nightState())
	next := sendKey(sendKey(m, "enter"), "y")
	if got := recIDs(f.acts()); len(got) != 1 || got[0] != "stoker:rest" {
		t.Fatalf("acts %v", got)
	}
	mm := next.(Model)
	if mm.pending == nil {
		t.Fatal("the key held for the answer did not run")
	}
	if mm.night() || !strings.Contains(ansi.Strip(mm.header(120)), "stoker rests") {
		t.Fatalf("after y: night %v, header %q", mm.night(), ansi.Strip(mm.header(120)))
	}
	if !strings.Contains(mm.flash, "stoker rests · f turns it back on") {
		t.Fatalf("flash %q", mm.flash)
	}
}

func TestNoKeepsTheStokerOnAndRunsTheKey(t *testing.T) {
	m, f := loadedWith(t, 120, nightState())
	next := sendKey(sendKey(m, "enter"), "n")
	if got := recIDs(f.acts()); len(got) != 1 || got[0] != "stoker:keep" {
		t.Fatalf("acts %v", got)
	}
	mm := next.(Model)
	if mm.pending == nil || !mm.night() {
		t.Fatalf("after n: pending %v night %v", mm.pending, mm.night())
	}
	if again := sendKey(sendKey(next, "esc"), "enter"); strings.Contains(footerOf(again), "Take over?") {
		t.Fatal("asked twice in one night")
	}
}

func TestLookingAroundNeverAsks(t *testing.T) {
	m, f := loadedWith(t, 120, nightState())
	var next tea.Model = m
	for _, k := range []string{"j", "k", "down", "up", "tab", "2", "1", "/", "esc", "s"} {
		next = sendKey(next, k)
		if strings.Contains(footerOf(next), "Take over?") {
			t.Fatalf("%q asked", k)
		}
	}
	if a := f.acts(); len(a) != 0 {
		t.Fatalf("sent %+v", a)
	}
}

func TestAnswerAsksToo(t *testing.T) {
	m, _ := loadedWith(t, 120, nightState())
	// The first row waits on a permission prompt; the second can be answered.
	if got := footerOf(sendKey(sendKey(m, "j"), "a")); !strings.Contains(got, "Take over?") {
		t.Fatalf("footer %q", got)
	}
}

func TestAnyOtherKeyDropsTheQuestion(t *testing.T) {
	m, f := loadedWith(t, 120, nightState())
	next := sendKey(sendKey(m, "enter"), "esc")
	if strings.Contains(footerOf(next), "Take over?") || next.(Model).pending != nil || len(f.acts()) != 0 {
		t.Fatalf("esc: footer %q", footerOf(next))
	}
	if !strings.Contains(footerOf(sendKey(next, "enter")), "Take over?") {
		t.Fatal("unanswered, yet not asked again")
	}
}

func TestTakeOverWithoutAConnectionSaysSo(t *testing.T) {
	m, f := loadedWith(t, 120, nightState())
	next := sendKey(m, "enter")
	mm := next.(Model)
	mm.lost = true
	next = sendKey(mm, "y")
	if got := footerOf(next); !strings.Contains(got, "not sent, no connection · y tries again") || !strings.Contains(got, "Take over?") {
		t.Fatalf("footer %q", got)
	}
	if len(f.acts()) != 0 || next.(Model).pending != nil {
		t.Fatal("sent or ran without a connection")
	}
}

func restingState() proto.State {
	st := nightState()
	st.Stoker.Rests, st.Stoker.Asked = time.Now(), true
	return st
}

func TestAStokerThatRestsLooksLikeTheDay(t *testing.T) {
	m, _ := loadedWith(t, 120, restingState())
	if m.night() {
		t.Fatal("a stoker that rests wears the night")
	}
	if h := ansi.Strip(m.header(120)); !strings.Contains(h, "stoker rests") {
		t.Fatalf("header %q", h)
	}
	if got := footerOf(sendKey(m, "enter")); strings.Contains(got, "Take over?") {
		t.Fatal("asked while it rests")
	}
}

func TestFWakesAStokerThatRests(t *testing.T) {
	m, f := loadedWith(t, 120, restingState())
	sendKey(m, "f")
	if got := recIDs(f.acts()); len(got) != 2 || got[0] != "stoker:touch" || got[1] != "stoker:wake" {
		t.Fatalf("acts %v", got)
	}
}

func TestKeysKeepTheRestOnceAMinute(t *testing.T) {
	m, f := loadedWith(t, 120, restingState())
	var next tea.Model = m
	for _, k := range []string{"j", "k", "j"} {
		next = sendKey(next, k)
	}
	if got := recIDs(f.acts()); len(got) != 1 || got[0] != "stoker:touch" {
		t.Fatalf("acts %v", got)
	}
}

func TestASecondTapAsksAndYRunsIt(t *testing.T) {
	m, f := loadedWith(t, 100, nightState())
	var ran []string
	m.opt.Run = func(argv []string) error { ran = argv; return nil }
	y := lineOf(t, m, "app-review")
	next, _ := tap(m, 5, y)
	next, cmd := tap(next, 5, y)
	if cmd != nil || ran != nil || !strings.Contains(footerOf(next), "Take over?") {
		t.Fatalf("the second tap ran before the answer: %q", footerOf(next))
	}
	next, cmd = key(next, "y")
	for _, c := range cmd().(tea.BatchMsg) {
		if c != nil {
			next.Update(c())
		}
	}
	if got := recIDs(f.acts()); len(got) != 1 || got[0] != "stoker:rest" || ran == nil {
		t.Fatalf("acts %v, ran %q", got, ran)
	}
}
