package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

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

// chainedFleet is n sessions on one host at 200 by 60 on Sessions, in
// chains of three when chained.
func chainedFleet(tb testing.TB, n int, chained bool) Model {
	tb.Helper()
	var ss []func(sample.Session) sample.Session
	for i := range n {
		ss = append(ss, sess(i+1, fmt.Sprintf("s%d", i), fmt.Sprintf("%%%d", i+1), nil))
	}
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
	now := lostAt
	var next tea.Model = New(Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return now }})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	next, _ = next.Update(stateMsg(st))
	m := next.(Model)
	m.tab = tabSessions
	return m
}

// TestFrameCostWithChains: a frame of 900 sessions in chains of three costs
// about what one with no chain does: the chains are worked out once in
// refresh, not at each row (that took seconds a frame).
func TestFrameCostWithChains(t *testing.T) {
	const n = 900
	frame := func(chained bool) time.Duration {
		m := chainedFleet(t, n, chained)
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

// TestAFrameBuildsTheGroupsOnce: the list, the chat, the header and the
// choice all read one build of the groups in a frame.
func TestAFrameBuildsTheGroupsOnce(t *testing.T) {
	m := chainedFleet(t, 30, true)
	for _, w := range []int{200, 40} {
		memo := &sideMemo{}
		_ = m.drawWith(w, memo)
		if n := memo.builds; n != 1 {
			t.Errorf("%d: a frame built the groups %d times", w, n)
		}
	}
}

func BenchmarkFrameOf900InChains(b *testing.B) {
	m := chainedFleet(b, 900, true)
	for b.Loop() {
		_ = m.draw(200)
	}
}

// TestAnOldViewFileStillLoads: a view.json with the hidden columns of an
// older console loads, and the console ignores them.
func TestAnOldViewFileStillLoads(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "view.json"), []byte(`{"list_only_phone":false,"hidden_phone":["work"],"hidden_desk":["need","quiet"],"told":["chain"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	p := loadViewPrefs(dir)
	if p.ListOnlyPhone || len(p.Told) != 1 {
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
