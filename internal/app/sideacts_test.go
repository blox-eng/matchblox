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

func TestMenuNamesTheListsActs(t *testing.T) {
	st := boardFixture(t)
	labels := func(m Model) map[string]string {
		out := map[string]string{}
		for _, it := range m.menu.items {
			out[it.key] = it.label
		}
		return out
	}
	m := pressOn(listShot(t, st, 160, 38, "kindle-web"), "m")
	if m.menu == nil {
		t.Fatal("m opened no menu")
	}
	got := labels(m)
	for _, k := range []string{"x", "r", "A", "p"} {
		if got[k] == "" {
			t.Errorf("the menu names no act on %s: %v", k, got)
		}
	}
	m = pressOn(m, "x")
	if m.menu != nil || !strings.Contains(consentText(m), "kill-pane") {
		t.Fatalf("x in the menu: menu %v, box %q", m.menu != nil, consentText(m))
	}

	m = pressOn(listShot(t, st, 160, 38, "spark-docs"), "m")
	if got := labels(m); got["p"] != "" {
		t.Errorf("a session with no ship that waits offers a merge: %v", got)
	}
	if !slices.ContainsFunc(m.menu.items, func(it menuItem) bool { return it.key == "x" }) {
		t.Error("spark-docs's menu has no kill")
	}
}
