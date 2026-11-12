package app

import (
	"errors"
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

// toTab opens tab t, as its digit did before the board was the home: a
// digit on the board answers its card, and Git and Panes open from
// Machine's rows at Task 12 step 6c.
func toTab(m tea.Model, t int) tea.Model {
	mm := m.(Model)
	mm.setTab(t)
	return mm
}

// actionLine is the key line, at the bottom.
func actionLine(m tea.Model) string {
	if mm, ok := m.(Model); ok && mm.consenting() {
		return consentText(mm)
	}
	lines := strings.Split(screen(m), "\n")
	return lines[len(lines)-1]
}

// consentText is the question box's words on one line: "title: lines ·
// footer", its edges dropped.
func consentText(m Model) string {
	box := m.consentBox(72)
	edge := func(l string) string { return strings.Trim(ansi.Strip(l), "╭╮╰╯─ ") }
	var out []string
	for _, l := range box[1 : len(box)-1] {
		if l = strings.TrimSpace(strings.Trim(ansi.Strip(l), "│")); l != "" {
			out = append(out, l)
		}
	}
	return edge(box[0]) + ": " + strings.Join(out, " ") + " · " + edge(box[len(box)-1])
}

func TestSearchEndsWithTheTab(t *testing.T) {
	m, _ := loadedWith(t, 100, fixtureState())
	var next tea.Model = m
	next, _ = key(next, "/")
	next = typeText(next, "review")
	next, _ = key(next, "enter")
	next, _ = key(next, "tab") // a digit on the board answers its card
	next, _ = key(next, "shift+tab")
	if out := screen(next); !strings.Contains(out, "app-feature") {
		t.Fatalf("the search stayed after a tab change:\n%s", out)
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

// TestSessionsBusyFirst: with nobody waiting, who works is on top, then
// the newest idle; the oldest are at the bottom (owner, 2026-10-08).
func TestSessionsBusyFirst(t *testing.T) {
	m, _ := loadedWith(t, 100, sortFixture())
	if got := order(m); got != "bravo alpha charlie" {
		t.Fatalf("order %q", got)
	}
}

func TestTapOnAHeaderSorts(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	var next tea.Model = m
	next = toTab(next, tabPanes)
	y := lineOf(t, next, "PATH")
	x := strings.Index(strings.Split(screen(next), "\n")[y], "RUNS")
	next, _ = tap(next, x+1, y)
	if s := next.(Model).paneSort; s.col != paneByRuns || s.rev {
		t.Fatalf("a tap on RUNS: %+v", s)
	}
	next, _ = tap(next, x+1, y)
	if s := next.(Model).paneSort; s.col != paneByRuns || !s.rev {
		t.Fatalf("a second tap on RUNS did not reverse: %+v", s)
	}
}

func TestPanesSort(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	var next tea.Model = m
	next = toTab(next, tabPanes)
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
	next = toTab(next, tabGit)
	next, _ = key(next, "X")
	if n := strings.Count(screen(next), "●"); n != 2 {
		t.Fatalf("%d marked, want the 2 safe ones:\n%s", n, screen(next))
	}
	next, _ = key(next, "x")
	if line := actionLine(next); !strings.Contains(line, "git -C /w/app worktree remove /w/wt/done-1 git -C /w/app worktree remove /w/wt/done-2") || !strings.Contains(line, "y run") {
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
	next = toTab(next, tabGit)
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
	next = toTab(next, tabGit)
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
	next = toTab(next, tabGit)
	next, _ = key(next, "down")
	next, _ = key(next, " ")
	next, _ = key(next, "/")
	next = typeText(next, "done-2")
	next, _ = key(next, "enter")
	next, _ = key(next, " ")
	if p := next.(Model).picked; !p[wtKey{"", "/w/wt/done-1"}] || !p[wtKey{"", "/w/wt/done-2"}] {
		t.Fatalf("marks %v", p)
	}
	next, _ = key(next, "x")
	if line := actionLine(next); !strings.Contains(line, "/w/wt/done-1") || !strings.Contains(line, "/w/wt/done-2") {
		t.Fatalf("the hidden mark does not count: %q", line)
	}
	got := next.(Model)
	got.git = nil
	got.refresh()
	if len(got.picked) != 2 {
		t.Fatal("a state without git dropped the marks")
	}
}

func TestRemovingOneWorktreeSaysWorktree(t *testing.T) {
	st := gitState()
	wts := st.Git.Repos[0].Worktrees
	st.Git.Repos[0].Worktrees = append(wts[:2:2], wts[3])
	m, _ := loadedWith(t, 100, st)
	next := toTab(m, tabGit)
	for _, k := range []string{"X", "x", "y"} {
		next, _ = key(next, k)
	}
	if line := actionLine(next); !strings.Contains(line, "removing 1 worktree…") {
		t.Fatalf("action line %q", line)
	}
}
