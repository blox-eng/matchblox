package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

func screen(m tea.Model) string { return ansi.Strip(m.(Model).render()) }

func actionLine(m tea.Model) string { return strings.Split(screen(m), "\n")[2] }

// TestSlashSearchesTheTab: / starts a search of the open tab; while it is
// typed every letter is text, q too; Enter keeps it, Esc clears it.
func TestSlashSearchesTheTab(t *testing.T) {
	m, _ := loadedWith(t, 100, fixtureState())
	var next tea.Model = m
	next, _ = key(next, "2")
	next, _ = key(next, "/")
	next = typeText(next, "review q")
	if next.(Model).quitting {
		t.Fatal("q quit while typing a search")
	}
	next, _ = key(next, "backspace")
	next, _ = key(next, "backspace")
	if out := screen(next); !strings.Contains(out, "app-review") || strings.Contains(out, "app-feature") {
		t.Fatalf("the search did not narrow the sessions:\n%s", out)
	}
	if !strings.Contains(actionLine(next), "/ review") {
		t.Fatalf("the search is not on the action line: %q", actionLine(next))
	}
	next, _ = key(next, "enter")
	if next.(Model).searching {
		t.Fatal("enter did not keep the search")
	}
	if !strings.Contains(actionLine(next), "/ review") {
		t.Fatalf("a kept search is not shown: %q", actionLine(next))
	}
	if s, ok := next.(Model).selected(); !ok || s.Name != "app-review" {
		t.Fatalf("the selection is not in the found rows: %+v", s)
	}
	next, _ = key(next, "esc")
	if out := screen(next); !strings.Contains(out, "app-feature") {
		t.Fatalf("esc did not clear the search:\n%s", out)
	}
}

func TestSearchEndsWithTheTab(t *testing.T) {
	m, _ := loadedWith(t, 100, fixtureState())
	var next tea.Model = m
	next, _ = key(next, "2")
	next, _ = key(next, "/")
	next = typeText(next, "review")
	next, _ = key(next, "enter")
	next, _ = key(next, "3")
	next, _ = key(next, "2")
	if out := screen(next); !strings.Contains(out, "app-feature") {
		t.Fatalf("the search stayed after a tab change:\n%s", out)
	}
}

func TestSearchFindsNothingSaysSo(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	next, _ := key(m, "/")
	next = typeText(next, "zzz")
	if out := screen(next); !strings.Contains(out, "nothing matches zzz") {
		t.Fatalf("no empty state for the search:\n%s", out)
	}
}

func sortFixture() proto.State {
	st := fixtureState()
	base := st.Sessions[0]
	mk := func(pid int, name string, busy bool, idle time.Duration, ctx, cpu float64) sample.Session {
		s := base
		s.PID, s.Name, s.Busy, s.Idle, s.Context, s.ContextPct, s.CPU = pid, name, busy, idle, "known", ctx, cpu
		s.Pane, s.Target, s.Do = "%"+name, "w:"+name, ""
		return s
	}
	st.Sessions = []sample.Session{
		mk(1, "alpha", false, 10*time.Minute, 20, 1),
		mk(2, "bravo", true, 0, 50, 30),
		mk(3, "charlie", false, time.Hour, 90, 2),
	}
	st.Queue = nil
	return st
}

func order(m tea.Model) string {
	var names []string
	for _, s := range m.(Model).snap.Sessions {
		names = append(names, s.Name)
	}
	return strings.Join(names, " ")
}

// TestSessionsBusyFirst: who works is on top, then the newest idle; the
// oldest are at the bottom (owner, 2026-10-08).
func TestSessionsBusyFirst(t *testing.T) {
	m, _ := loadedWith(t, 100, sortFixture())
	if got := order(m); got != "bravo alpha charlie" {
		t.Fatalf("order %q", got)
	}
}

func TestSortCyclesAndReverses(t *testing.T) {
	m, _ := loadedWith(t, 100, sortFixture())
	var next tea.Model = m
	next, _ = key(next, "2")
	next, _ = key(next, "s") // idle
	next, _ = key(next, "s") // context
	if got := order(next); got != "charlie bravo alpha" {
		t.Fatalf("by context: %q", got)
	}
	if !strings.Contains(screen(next), "CONTEXT▾") {
		t.Fatalf("the header does not show the sort:\n%s", screen(next))
	}
	next, _ = key(next, "S")
	if got := order(next); got != "alpha bravo charlie" {
		t.Fatalf("by context, reversed: %q", got)
	}
	// The order holds when the next state comes.
	next, _ = next.Update(stateMsg(sortFixture()))
	if got := order(next); got != "alpha bravo charlie" {
		t.Fatalf("a new state undid the sort: %q", got)
	}
}

func TestNarrowShowsTheSort(t *testing.T) {
	m, _ := loadedWith(t, 50, sortFixture())
	var next tea.Model = m
	next, _ = key(next, "2")
	next, _ = key(next, "s")
	if !strings.Contains(screen(next), "by idle ▾") {
		t.Fatalf("the phone does not show the sort:\n%s", screen(next))
	}
}

func TestTapOnAHeaderSorts(t *testing.T) {
	m, _ := loadedWith(t, 100, sortFixture())
	var next tea.Model = m
	next, _ = key(next, "2")
	y := lineOf(t, next, "WORKTREE")
	x := strings.Index(strings.Split(screen(next), "\n")[y], "CPU")
	next, _ = tap(next, x+1, y)
	if got := order(next); got != "bravo charlie alpha" || next.(Model).sessSort.col != sortCPU {
		t.Fatalf("a tap on CPU: %q", got)
	}
	next, _ = tap(next, x+1, y)
	if got := order(next); got != "alpha charlie bravo" {
		t.Fatalf("a second tap on CPU did not reverse: %q", got)
	}
}

func TestPanesSort(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	var next tea.Model = m
	next, _ = key(next, "7")
	next, _ = key(next, "s")
	rows := next.(Model).paneRows()
	for i := 1; i < len(rows); i++ {
		if rows[i-1].what > rows[i].what {
			t.Fatalf("panes not by what runs: %+v", rows)
		}
	}
}

func gitState() proto.State {
	st := fixtureState()
	st.Git = &proto.GitReport{At: st.At, Repos: []gitscan.Repo{{Path: "/w/app", Main: "main", Worktrees: []gitscan.Worktree{
		{Path: "/w/app", Branch: "main"},
		{Path: "/w/wt/done-1", Branch: "done-1", Merged: true, Safe: true, Remove: []string{"git", "-C", "/w/app", "worktree", "remove", "/w/wt/done-1"}},
		{Path: "/w/wt/done-2", Branch: "done-2", Merged: true, Safe: true, Remove: []string{"git", "-C", "/w/app", "worktree", "remove", "/w/wt/done-2"}},
		{Path: "/w/wt/busy", Branch: "busy", Merged: true, Dirty: 3},
	}}}}
	return st
}

// TestRemoveEverySafeWorktree: X marks every safe worktree, x asks once
// for a typed y, and the service gets one guarded act for each.
func TestRemoveEverySafeWorktree(t *testing.T) {
	m, f := loadedWith(t, 100, gitState())
	var next tea.Model = m
	next, _ = key(next, "5")
	next, _ = key(next, "X")
	if n := strings.Count(screen(next), "●"); n != 2 {
		t.Fatalf("%d marked, want the 2 safe ones:\n%s", n, screen(next))
	}
	next, _ = key(next, "x")
	if line := actionLine(next); !strings.Contains(line, "RUN git -C /w/app worktree remove /w/wt/done-1  (+1 more)") || !strings.Contains(line, "y run") {
		t.Fatalf("the confirm does not show the steps: %q", line)
	}
	next, cmd := key(next, "y")
	if cmd == nil {
		t.Fatal("y sent nothing")
	}
	for _, msg := range collect(cmd) {
		next, _ = next.Update(msg)
	}
	acts := f.acts()
	if len(acts) != 2 || acts[0] != (proto.Act{RecID: "worktree:/w/wt/done-1", Which: "secondary", Confirm: "y"}) ||
		acts[1].RecID != "worktree:/w/wt/done-2" {
		t.Fatalf("sent %+v", acts)
	}
	next, _ = next.Update(resultMsg(proto.Result{ActID: "a1", Ran: [][]string{{"git"}}}))
	next, _ = next.Update(resultMsg(proto.Result{ActID: "a2", Skipped: []string{"has changes"}}))
	if line := actionLine(next); !strings.Contains(line, "removed 1, skipped 1: has changes") {
		t.Fatalf("no summary: %q", line)
	}
	if strings.Contains(screen(next), "●") {
		t.Fatal("the marks stayed after the removal")
	}
}

func TestSpaceMarksOnlyASafeWorktree(t *testing.T) {
	m, _ := loadedWith(t, 100, gitState())
	var next tea.Model = m
	next, _ = key(next, "5")
	for range 3 {
		next, _ = key(next, "down")
	}
	next, _ = key(next, " ")
	if strings.Contains(screen(next), "●") || !strings.Contains(actionLine(next), "not safe to remove") {
		t.Fatalf("an unsafe worktree was marked, or no reason: %q", actionLine(next))
	}
	next, _ = key(next, "up")
	next, _ = key(next, " ")
	if strings.Count(screen(next), "●") != 1 {
		t.Fatal("space did not mark the safe worktree")
	}
	next, _ = key(next, " ")
	if strings.Contains(screen(next), "●") {
		t.Fatal("a second space did not unmark")
	}
}

// collect runs a command, and the commands of a batch, for their messages.
func collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, collect(c)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

// Review 2, #1: without a connection a bulk removal is not sent, the marks
// stay, and a batch cut by a lost connection says what was pending.
func TestBatchWithoutAConnectionIsNotSent(t *testing.T) {
	m, f := loadedWith(t, 100, gitState())
	var next tea.Model = m
	next, _ = key(next, "5")
	next, _ = key(next, "X")
	got := next.(Model)
	got.lost = true
	next, _ = key(got, "x")
	next, cmd := key(next, "y")
	if cmd != nil {
		for _, msg := range collect(cmd) {
			next, _ = next.Update(msg)
		}
	}
	if len(f.acts()) != 0 || !strings.Contains(actionLine(next), "not sent, no connection") {
		t.Fatalf("sent %v, line %q", f.acts(), actionLine(next))
	}
	if n := strings.Count(screen(next), "●"); n != 2 {
		t.Fatalf("the marks went: %d", n)
	}

	got = next.(Model)
	got.lost = false
	got.batch = &batch{ids: map[string]bool{"a1": true, "a2": true}, removed: 1}
	next, _ = got.Update(lostMsg{err: errors.New("broken pipe"), conn: got.conn})
	if next.(Model).batch != nil || !strings.Contains(next.(Model).flash, "2 removals have no answer") {
		t.Fatalf("a lost batch: %+v %q", next.(Model).batch, next.(Model).flash)
	}
}

// Review 2, #2: a search on the Git tab hides rows, not marks.
func TestSearchKeepsTheMarks(t *testing.T) {
	m, _ := loadedWith(t, 100, gitState())
	var next tea.Model = m
	next, _ = key(next, "5")
	next, _ = key(next, "down")
	next, _ = key(next, " ")
	next, _ = key(next, "/")
	next = typeText(next, "done-2")
	next, _ = key(next, "enter")
	next, _ = key(next, " ")
	if p := next.(Model).picked; !p["/w/wt/done-1"] || !p["/w/wt/done-2"] {
		t.Fatalf("marks %v", p)
	}
	next, _ = key(next, "x")
	if line := actionLine(next); !strings.Contains(line, "(+1 more)") {
		t.Fatalf("the hidden mark does not count: %q", line)
	}
	got := next.(Model)
	got.git = nil
	got.refresh()
	if len(got.picked) != 2 {
		t.Fatal("a state without git dropped the marks")
	}
}

// Review 2, #5: the progress cell holds its column at any percent.
func TestProgressHoldsItsColumn(t *testing.T) {
	st := sortFixture()
	st.Sessions[0].Progress = &sample.Progress{Pct: 5}
	st.Sessions[1].Progress = &sample.Progress{Pct: 100}
	m, _ := loadedWith(t, 120, st)
	next, _ := key(m, "2")
	lines := strings.Split(screen(next), "\n")
	a, b := lines[lineOf(t, next, "alpha")], lines[lineOf(t, next, "bravo")]
	wt := worktree(st.Sessions[0].Cwd)
	at := func(line string) int { return lipgloss.Width(line[:max(strings.LastIndex(line, wt), 0)]) }
	if ia, ib := at(a), at(b); !strings.Contains(a, wt) || ia != ib {
		t.Fatalf("the worktree column moved (%d, %d):\n%s\n%s", ia, ib, a, b)
	}
}

// Review 2, #9: a tap ends the typing of a search, so a confirm it asks
// for is on the screen.
func TestTapEndsTheTypingOfASearch(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	next, _ := key(m, "/")
	next = typeText(next, "app")
	y := lineOf(t, next, "app-review")
	next, _ = tap(next, 5, y)
	if next.(Model).searching || next.(Model).filter != "app" {
		t.Fatalf("searching %v filter %q", next.(Model).searching, next.(Model).filter)
	}
	if !strings.Contains(actionLine(next), "tap again") {
		t.Fatalf("line %q", actionLine(next))
	}
}

// TestAnOpenPullRequestShowsWhereItsAgentShows: the PR of the worktree an
// agent works in is on its Sessions row, its queue row and in the detail.
func TestAnOpenPullRequestShowsWhereItsAgentShows(t *testing.T) {
	st := fixtureState()
	st.Sessions = append(st.Sessions, sample.Session{PID: 950, Pane: "%9", Target: "api:1.1", Name: "cache-work",
		Status: "idle", Idle: time.Minute, Cwd: "/w/app-cache/internal", Context: "fresh"})
	st.Queue = []queue.Item{{Pane: "%9", Target: "api:1.1", Name: "cache-work", State: queue.StateQuestion,
		Since: st.At.Add(-time.Minute), LastLine: "Shall I merge it?"}}
	st.Git = &proto.GitReport{At: st.At, Repos: []gitscan.Repo{{Path: "/w/app", Main: "main", Worktrees: []gitscan.Worktree{
		{Path: "/w/app", Branch: "main"},
		{Path: "/w/app-cache", Branch: "cache", Sessions: 1, PR: &gitscan.PR{Number: 4312, Title: "Add the cache", URL: "https://example.com/pr/4312"}},
	}}}}
	m, _ := loadedWith(t, 120, st)
	if out := ansi.Strip(m.render()); !strings.Contains(out, "#4312") {
		t.Fatalf("the queue row lacks the PR:\n%s", out)
	}
	m.tab, m.selPID = tabSessions, 950
	out := ansi.Strip(m.render())
	for _, want := range []string{"#4312", "PR #4312 Add the cache", "https://example.com/pr/4312"} {
		if !strings.Contains(out, want) {
			t.Fatalf("Sessions lacks %q:\n%s", want, out)
		}
	}
	for _, w := range []int{80, 100} {
		m.width = w
		for _, l := range strings.Split(ansi.Strip(m.render()), "\n") {
			if strings.Contains(l, "cache-work") && !strings.Contains(l, "PR #") && !strings.Contains(l, "#4312") {
				t.Fatalf("the row at %d columns lacks the PR: %q", w, l)
			}
		}
	}
	m.width = 50 // a phone
	if out := ansi.Strip(m.render()); !strings.Contains(out, "#4312") {
		t.Fatalf("the phone row lacks the PR:\n%s", out)
	}
}
