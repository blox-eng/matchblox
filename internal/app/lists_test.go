package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
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

// TestSessionsBusyFirst: who works is on top, then who waits longest.
func TestSessionsBusyFirst(t *testing.T) {
	m, _ := loadedWith(t, 100, sortFixture())
	if got := order(m); got != "bravo charlie alpha" {
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
