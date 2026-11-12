package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
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
	m.viewPrefs = viewPrefs{CopyCheckedPhone: true, CopyCheckedDesk: true}
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
// screen, folds the chat and the stats under the list, and opens the
// selected row in place; the key line says so, and a tap on its word
// flips it back.
func TestVFoldsThePaneAndTheDock(t *testing.T) {
	t.Skip("dropped (Task 12 step 1): list only goes (step 3, intended change: v is the board's density)")
}

// TestVFlipsOnlyTheCurrentShape: v on a desk leaves the phone's flag.
func TestVFlipsOnlyTheCurrentShape(t *testing.T) {
	t.Skip("replaced by TestBoardPrefsAreKeptForEachShape (Task 12 step 1)")
}

// TestListOnlyIsKeptAcrossARestart: the next console opens as the last
// one was left, from view.json.
func TestListOnlyIsKeptAcrossARestart(t *testing.T) {
	t.Skip("replaced by TestBoardPrefsAreKeptForEachShape (Task 12 step 1)")
}

// TestPhoneStartsListOnly: an upright screen wide enough to split opens
// with no pane and no dock's switch while no view.json says otherwise (R9).
func TestPhoneStartsListOnly(t *testing.T) {
	t.Skip("replaced by TestAPhoneStartsVerboseInNeedsYouOnly (Task 12 step 1)")
	m := demoIn(t, t.TempDir(), 96, 60)
	if sc := m.draw(96); sc.listW != 96 || strings.Contains(screen(m), "machine ›") {
		t.Fatalf("list %d:\n%s", sc.listW, screen(m))
	}
}

// TestWaitsStillShowInListOnly: the header keeps who waits.
func TestWaitsStillShowInListOnly(t *testing.T) {
	t.Skip("replaced by TestTheCountSeesThroughEveryFilter (Task 12 step 1)")
	m := demoIn(t, "", 92, 40)
	full := ansi.Strip(m.draw(92).lines[0])
	next, _ := key(m, "v")
	if head := ansi.Strip(next.(Model).draw(92).lines[0]); !strings.Contains(head, "● 6 wait for you") || head != full {
		t.Fatalf("the header lost who waits: %q, was %q", head, full)
	}
}

// TestVIsFree: v is the board's density alone; the tabs bind no v.
func TestVIsFree(t *testing.T) {
	main, _ := layersOf(defaultKeys())
	if got := owners(t, main)["v"]; got != "" {
		t.Fatalf("v is %q on the tabs", got)
	}
	if !slices.Contains(defaultKeys().board.Density.Keys(), "v") {
		t.Fatalf("v is not the board's density: %q", defaultKeys().board.Density.Keys())
	}
	TestKeymapsHaveNoCollisions(t)
}

// TestTheSwitchGivesWayOnTheDesk: on the full view list only v is the
// first spare key to go, so a desk key line keeps every key it had, and
// shows the switch where room is left.
func TestTheSwitchGivesWayOnTheDesk(t *testing.T) {
	t.Skip("replaced by TestTheKeyLineDropsWholeKeysFromTheRight (Task 12 step 1)")
	m := loaded(t, 100)
	if k := keyText(m.draw(100)); strings.Contains(k, "list only v") {
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
	t.Skip("replaced by TestTheKeyLineDropsWholeKeysFromTheRight (Task 12 step 3): list only goes; short of room the spare keys go first")
}

// TestTheMarkKeepsItsGap: the age and the list only mark stand two
// columns clear of the keys, so they never read as one phrase.
func TestTheMarkKeepsItsGap(t *testing.T) {
	t.Skip("dropped (Task 12 step 3): list only goes, and its mark (intended change: v is the board's density)")
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

// TestFrameCostWithChains: a frame of 900 sessions in chains of three costs
// about what one with no chain does: the chains are worked out once in
// refresh, not at each row (that took seconds a frame).
func TestFrameCostWithChains(t *testing.T) {
	const n = 900
	var ss []func(sample.Session) sample.Session
	for i := range n {
		ss = append(ss, sess(i+1, fmt.Sprintf("s%d", i), fmt.Sprintf("%%%d", i+1), nil))
	}
	frame := func(chained bool) time.Duration {
		m := manyRows(t, n)
		st := hostState(ss...)
		if chained {
			st.Chains = &state.ChainsView{}
			for i := 0; i+2 < n; i += 3 {
				c := chains.Chain{ID: fmt.Sprintf("c%d", i)}
				for _, s := range st.Sessions[i : i+3] {
					c.Members = append(c.Members, memberOf(s))
				}
				st.Chains.Chains = append(st.Chains.Chains, c)
			}
		}
		m = upd(t, m, stateMsg(st))
		best := time.Hour
		for range 5 {
			start := time.Now()
			_ = m.draw(200)
			best = min(best, time.Since(start))
		}
		return best
	}
	plain, chained := frame(false), frame(true)
	if chained > 20*plain+100*time.Millisecond {
		t.Fatalf("a frame with chains took %v, one without %v", chained, plain)
	}
}

// TestOneColumnOpensTheRowInPlace: wherever Sessions is one column with no
// right side (a landscape screen of 60 to 82 columns, 96x8, 106x10), the
// selected row opens in place, as on a phone: its words show before Enter.
func TestOneColumnOpensTheRowInPlace(t *testing.T) {
	t.Skip("replaced by TestBoardKeepsTheChosenCardWhole (Task 12 step 1)")
	for _, size := range [][2]int{{80, 30}, {66, 24}, {96, 8}, {106, 10}} {
		m := fullView(flintAt(t, size[0], size[1]))
		if _, right := m.panes(size[0]); right != 0 {
			t.Fatalf("%v splits", size)
		}
		if sc := m.draw(size[0]); !opensInPlace(m, sc) {
			t.Errorf("%v: the selected row does not open:\n%s", size, screen(m))
		}
	}
}

// TestBoardPrefsAreKeptForEachShape: a phone and a desk each keep their
// hidden columns, density and strip; an old view.json still loads.
func TestBoardPrefsAreKeptForEachShape(t *testing.T) {
	dir := t.TempDir()
	m := demoIn(t, dir, 44, 31)
	if !m.upright() || m.colHidden(colWork) || m.oneLine() || m.stripFace() != "usage" {
		t.Fatalf("a fresh phone is not plain: %+v", m.viewPrefs)
	}
	m.flipHidden(colWork)
	m.flipOneLine()
	m.turnStrip()

	again := demoIn(t, dir, 44, 31)
	if !again.colHidden(colWork) || again.colHidden(colNeed) || !again.oneLine() || again.stripFace() != "machine" {
		t.Fatalf("the phone lost its board views: %+v", again.viewPrefs)
	}
	desk := demoIn(t, dir, 132, 32)
	if desk.upright() || desk.colHidden(colWork) || desk.oneLine() || desk.stripFace() != "usage" {
		t.Fatalf("the desk took the phone's board views: %+v", desk.viewPrefs)
	}

	again.flipHidden(colWork)
	if again.colHidden(colWork) {
		t.Fatal("a second flip did not show the column again")
	}
	for _, want := range []string{"off", "usage"} {
		again.turnStrip()
		if got := again.stripFace(); got != want {
			t.Fatalf("the strip turned to %q, want %q", got, want)
		}
	}
}

// TestAnOldViewFileStillLoads: a view.json with only list_only_phone keeps it
// and gives the board its defaults.
func TestAnOldViewFileStillLoads(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "view.json"), []byte(`{"list_only_phone":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	p := loadViewPrefs(dir)
	if p.ListOnlyPhone || p.OneLinePhone != nil || p.StripPhone != "" || len(p.HiddenPhone) != 0 || len(p.Told) != 0 {
		t.Fatalf("old file: %+v", p)
	}
}

// TestTellKeepsANoticeOnce: a told key stays told for the next console.
func TestTellKeepsANoticeOnce(t *testing.T) {
	dir := t.TempDir()
	m := demoIn(t, dir, 44, 31)
	if m.told("x") {
		t.Fatal("told before tell")
	}
	m.tell("x")
	m.tell("x")
	if len(m.viewPrefs.Told) != 1 {
		t.Fatalf("tell added twice: %v", m.viewPrefs.Told)
	}
	if again := demoIn(t, dir, 132, 32); !again.told("x") || again.told("y") {
		t.Fatalf("the next console lost what it was told: %v", again.viewPrefs.Told)
	}
}
