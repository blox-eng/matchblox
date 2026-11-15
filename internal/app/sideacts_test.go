package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/proto"
)

// listShot is the console at w by h on st with the keys in the list and
// the session named name chosen.
func listShot(t *testing.T, st proto.State, w, h int, name string) Model {
	t.Helper()
	m := boardShot(t, st, w, h, nil)
	m.sideOn = true
	m = pickSide(t, m, name)
	m.toList()
	return m
}

func TestListKillsTheChosenSessionWithItsConfirm(t *testing.T) {
	st := boardFixture(t)
	pane := paneNamed(st, "kindle-web")
	m := pressOn(listShot(t, st, 160, 38, "kindle-web"), "x")
	if box := consentText(m); !strings.Contains(box, "tmux kill-pane -t "+pane) {
		t.Fatalf("x shows %q", box)
	}
	if len(boardFake(m).acts()) != 0 {
		t.Fatalf("x alone acted: %+v", boardFake(m).acts())
	}
	m = pressOn(m, "y", "y")
	if acts := boardFake(m).acts(); len(acts) != 1 || acts[0].RecID != "kill:"+pane {
		t.Fatalf("x y y sent %+v", acts)
	}
}

func TestListMergesTheChosenSessionsShipWithItsConfirm(t *testing.T) {
	st := boardFixture(t)
	sh := mergeOf(t, st)
	m := pressOn(listShot(t, st, 160, 38, "kindle-web"), "p")
	if !m.consenting() || !strings.Contains(consentText(m), strings.Join(sh.Merge, " ")) {
		t.Fatalf("p does not show the merge: %q", consentText(m))
	}
	m = pressOn(m, "y")
	want := proto.Act{RecID: "ship:" + sh.Repo + "#4556", Which: "secondary", Confirm: "y"}
	if acts := boardFake(m).acts(); len(acts) != 1 || acts[0] != want {
		t.Fatalf("y sent %+v, want %+v", acts, want)
	}
}

func TestListOpensTheFormsOfTheChosenSession(t *testing.T) {
	st := boardFixture(t)
	for _, k := range []string{"r", "A"} {
		m := pressOn(listShot(t, st, 160, 38, "spark-docs"), k)
		if m.form == nil && m.flash == "" && m.noticeText == "" {
			t.Errorf("%s opened nothing and said nothing", k)
		}
		if m.lines[m.chosenSide()] != "" {
			t.Errorf("%s typed into the line", k)
		}
	}
}

func TestLineTypesTheCardLetters(t *testing.T) {
	m := boardShot(t, boardFixture(t), 160, 38, nil)
	m.sideOn = true
	m = pickSide(t, m, "spark-docs")
	m = typeLine(m, "xrApyfFod")
	if got := m.lines[m.chosenSide()]; got != "xrApyfFod" {
		t.Fatalf("the line holds %q", got)
	}
	if m.pending != nil || m.form != nil || len(boardFake(m).acts()) != 0 {
		t.Fatalf("a letter in the line acted: pending %v, form %v, acts %+v", m.pending, m.form != nil, boardFake(m).acts())
	}
}

func TestAFootPartTakesNoCardKey(t *testing.T) {
	m := listShot(t, boardFixture(t), 160, 38, "spark-docs")
	m.footAt = 1
	m = pressOn(m, "x")
	if m.pending != nil || !strings.Contains(m.noticeText, "act on a session") {
		t.Fatalf("x on the foot: pending %v, notice %q", m.pending, m.noticeText)
	}
}

// TestMenuHoldsNoActThatDestroys: the menu names the acts that can be
// undone (the owner, 2026-11-12: the menu destroys nothing). The kill, the
// merge, the go, the switch and the stoker's stop stay keys of the list.
func TestMenuHoldsNoActThatDestroys(t *testing.T) {
	st := boardFixture(t)
	for _, s := range st.Sessions {
		m := pressOn(listShot(t, st, 160, 38, s.Name), "m")
		if m.menu == nil {
			t.Fatalf("m opened no menu on %s", s.Name)
		}
		for i, it := range m.menu.items {
			if slices.Contains([]string{"x", "p", "y", "A", "f"}, it.key) {
				t.Errorf("%s's menu has %q (%s)", s.Name, it.label, it.key)
			}
			run := m
			for range i {
				run = pressOn(run, "down")
			}
			run = pressOn(run, "enter")
			if run.pending != nil && run.pending.destructive || run.menu != nil {
				t.Errorf("%s's %q opened a destructive step or kept the menu: %v", s.Name, it.label, run.pending)
			}
			for _, a := range boardFake(run).acts() {
				if strings.HasPrefix(a.RecID, "kill:") || strings.HasPrefix(a.RecID, "ship:") {
					t.Errorf("%s's %q sent %s", s.Name, it.label, a.RecID)
				}
			}
		}
	}
}

func TestMenuNamesTheListsActs(t *testing.T) {
	st := boardFixture(t)
	m := pressOn(listShot(t, st, 160, 38, "flint-auth"), "m")
	if m.menu == nil {
		t.Fatal("m opened no menu")
	}
	got := map[string]string{}
	for _, it := range m.menu.items {
		got[it.key] = it.label
	}
	if got["r"] != "rename" || got["F"] != "stoke the chain login" {
		t.Errorf("the menu names %v", got)
	}
	m = pressOn(m, "r")
	if m.menu != nil || m.form == nil {
		t.Fatalf("r in the menu: menu %v, form %v", m.menu != nil, m.form != nil)
	}
}
