package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/sample"
)

// demoIn is the demo machine at w by h, its views kept in dir ("" keeps
// them in memory).
func demoIn(t *testing.T, dir string, w, h int) Model {
	t.Helper()
	m := New(Options{NoMotion: true, Conn: newFake(), StateDir: dir, Now: func() time.Time { return demoAt }})
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	next, _ = next.Update(stateMsg(screensState(t)))
	nm := next.(Model)
	nm.splashDone = true
	return nm
}

// fullView is m with list only off on both shapes.
func fullView(m Model) Model {
	m.viewPrefs = viewPrefs{}
	return m
}

func keyText(sc drawn) string { return ansi.Strip(sc.lines[sc.keysAt]) }

// opensInPlace: the selected row has more than one line of the list.
func opensInPlace(m Model, sc drawn) bool {
	n := 0
	for _, r := range sc.rows {
		if r == m.rowIndex() {
			n++
		}
	}
	return n > 1
}

// TestVFoldsThePaneAndTheDock: on a desk, v gives the list the whole
// screen, folds the dock and opens the selected row in place; the key
// line says so, and a tap on its word flips it back.
func TestVFoldsThePaneAndTheDock(t *testing.T) {
	m := demoIn(t, "", 92, 40)
	sc := m.draw(92)
	if sc.listW >= 92 || sc.dockAt < 0 || opensInPlace(m, sc) {
		t.Fatalf("the full view: list %d, dock at %d, opened %v", sc.listW, sc.dockAt, opensInPlace(m, sc))
	}
	if k := keyText(sc); strings.HasSuffix(strings.TrimSpace(k), "list only") {
		t.Fatalf("the full view's key line: %q", k)
	}
	next, _ := key(m, "v")
	lo := next.(Model)
	sc = lo.draw(92)
	if !lo.listOnly() || sc.listW != 92 || sc.dockAt != -1 || !opensInPlace(lo, sc) {
		t.Fatalf("list only: on %v, list %d, dock at %d, opened %v\n%s", lo.listOnly(), sc.listW, sc.dockAt, opensInPlace(lo, sc), screen(lo))
	}
	k := keyText(sc)
	if !strings.Contains(k, "full view v") {
		t.Fatalf("list only's key line: %q", k)
	}
	panes := lo
	panes.setTab(tabPanes) // fewer keys: room for the mark
	if k := keyText(panes.draw(92)); !strings.HasSuffix(strings.TrimSpace(k), "list only") {
		t.Fatalf("list only's key line has no mark: %q", k)
	}
	var at span
	for _, s := range sc.keys {
		if s.press == "v" {
			at = s
		}
	}
	if at == (span{}) {
		t.Fatalf("full view v is no tap target: %q", k)
	}
	back, _ := tap(lo, at.x0, sc.keysAt)
	if back.(Model).listOnly() {
		t.Fatal("a tap on full view v did not bring the full view back")
	}
}

// TestVFlipsOnlyTheCurrentShape: v on a desk leaves the phone's flag.
func TestVFlipsOnlyTheCurrentShape(t *testing.T) {
	m := demoIn(t, "", 92, 40)
	next, _ := key(m, "v")
	next, _ = next.Update(tea.WindowSizeMsg{Width: 60, Height: 40})
	if nm := next.(Model); !nm.upright() || !nm.listOnly() || !nm.viewPrefs.ListOnlyDesk {
		t.Fatalf("upright %v, list only %v, %+v", nm.upright(), nm.listOnly(), nm.viewPrefs)
	}
	next, _ = key(next, "v")
	next, _ = next.Update(tea.WindowSizeMsg{Width: 92, Height: 40})
	if nm := next.(Model); !nm.listOnly() || nm.viewPrefs.ListOnlyPhone {
		t.Fatalf("v on the phone changed the desk: %+v", nm.viewPrefs)
	}
}

// TestListOnlyIsKeptAcrossARestart: the next console opens as the last
// one was left, from view.json.
func TestListOnlyIsKeptAcrossARestart(t *testing.T) {
	dir := t.TempDir()
	next, _ := key(demoIn(t, dir, 92, 40), "v")
	if !next.(Model).listOnly() {
		t.Fatal("v did not turn list only on")
	}
	if _, err := os.Stat(filepath.Join(dir, "view.json")); err != nil {
		t.Fatal(err)
	}
	again := demoIn(t, dir, 92, 40)
	if !again.listOnly() || !again.viewPrefs.ListOnlyPhone {
		t.Fatalf("the next console lost the views: %+v", again.viewPrefs)
	}
}

// TestPhoneStartsListOnly: an upright screen wide enough to split opens
// with no pane and no dock while no view.json says otherwise (R9).
func TestPhoneStartsListOnly(t *testing.T) {
	m := demoIn(t, t.TempDir(), 96, 60)
	if sc := m.draw(96); sc.listW != 96 || sc.dockAt != -1 {
		t.Fatalf("list %d, dock at %d:\n%s", sc.listW, sc.dockAt, screen(m))
	}
}

// TestWaitsStillShowInListOnly: the header keeps who waits.
func TestWaitsStillShowInListOnly(t *testing.T) {
	m := demoIn(t, "", 92, 40)
	full := ansi.Strip(m.draw(92).lines[0])
	next, _ := key(m, "v")
	if head := ansi.Strip(next.(Model).draw(92).lines[0]); !strings.Contains(head, "● 6 wait for you") || head != full {
		t.Fatalf("the header lost who waits: %q, was %q", head, full)
	}
}

// TestVIsFree: v is list only's alone.
func TestVIsFree(t *testing.T) {
	main, _ := layersOf(defaultKeys())
	if got := owners(t, main)["v"]; got != "ListOnly" {
		t.Fatalf("v is %q", got)
	}
	TestKeymapsHaveNoCollisions(t)
}

// TestTheSwitchGivesWayOnTheDesk: on the full view list only v is the
// first spare key to go, so a desk key line keeps every key it had, and
// shows the switch where room is left.
func TestTheSwitchGivesWayOnTheDesk(t *testing.T) {
	m := loaded(t, 100)
	if k := keyText(m.draw(100)); !strings.Contains(k, "/ find") || strings.Contains(k, "list only v") {
		t.Fatalf("the switch took a key's place: %q", k)
	}
	m.tab = tabPanes
	if k := keyText(m.draw(100)); !strings.Contains(k, "/ find") || !strings.Contains(k, "q quit") || !strings.Contains(k, "list only v") {
		t.Fatalf("a line with room has no switch, or lost a key: %q", k)
	}
	desk := demoIn(t, "", 132, 32)
	if k := keyText(desk.draw(132)); !strings.Contains(k, "A switch") || !strings.Contains(k, "/ find") {
		t.Fatalf("the switch took a key's place at 132: %q", k)
	}
}

// TestFullViewStaysWhenRoomIsShort: in list only the way back stays; the
// list only mark goes first, then the age, before a spare key.
func TestFullViewStaysWhenRoomIsShort(t *testing.T) {
	m, _ := loadedWith(t, 50, queueState())
	m.tab = tabPanes
	k := keyText(m.draw(50))
	if !m.listOnly() || !strings.Contains(k, "full view v") || !strings.Contains(k, "s sort") || strings.HasSuffix(strings.TrimSpace(k), "list only") {
		t.Fatalf("list only on a short line: %q", k)
	}
	if !strings.Contains(k, "sampled") {
		t.Fatalf("the age went before the mark: %q", k)
	}
}

// TestTheMarkKeepsItsGap: the age and the list only mark stand two
// columns clear of the keys, so they never read as one phrase.
func TestTheMarkKeepsItsGap(t *testing.T) {
	gap := regexp.MustCompile(`\S {2,}(sampled \S+ ago  )?list only $`)
	for w := 40; w <= 140; w++ {
		m := demoIn(t, "", w, 40)
		if !m.listOnly() {
			m = fullView(m)
			m.viewPrefs.ListOnlyDesk = true
		}
		for _, tab := range []int{tabSessions, tabPanes, tabMachine} {
			m.setTab(tab)
			k := keyText(m.draw(w))
			if strings.HasSuffix(k, "list only ") && !gap.MatchString(k) {
				t.Fatalf("w=%d %s: the mark has no gap: %q", w, tabNames[tab], k)
			}
		}
	}
}

// manyRows is one host with n sessions, list only, 200×60.
func manyRows(t *testing.T, n int) Model {
	t.Helper()
	var ss []func(sample.Session) sample.Session
	for i := range n {
		ss = append(ss, sess(i+1, fmt.Sprintf("s%d", i), fmt.Sprintf("%%%d", i+1), nil))
	}
	now := lostAt
	m, _, _ := hostsAt(t, &now, "", 200, 60)
	m = upd(t, m, stateMsg(hostState(ss...)))
	m.viewPrefs.ListOnlyDesk, m.splashDone = true, true
	return m
}

// TestListOnlyFrameCostGrowsWithRows: a list only frame's work grows with its
// rows, not with their square: 600 sessions cost about twice 300.
func TestListOnlyFrameCostGrowsWithRows(t *testing.T) {
	cost := func(n int) float64 {
		m := manyRows(t, n)
		return testing.AllocsPerRun(1, func() { _ = m.draw(200) })
	}
	if small, big := cost(300), cost(600); big > 3*small {
		t.Fatalf("600 rows cost %.0f allocations, 300 cost %.0f: more than twice", big, small)
	}
}
