package app

import (
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

func TestFitKeepsTheBottomAndCutsRules(t *testing.T) {
	prose := ""
	for i := 0; i < 180; i++ {
		prose += string(rune('a' + i%26))
	}
	var lines []string
	for i := 0; i < 39; i++ {
		if i%2 == 0 {
			lines = append(lines, strings.Repeat("─", 200))
		} else {
			lines = append(lines, prose)
		}
	}
	lines = append(lines, "> ")
	got := fitScreen(strings.Join(lines, "\n"), 44, 20)
	if len(got) != 20 {
		t.Fatalf("got %d lines, want 20", len(got))
	}
	full := false
	for i, l := range got {
		w := ansi.StringWidth(l)
		if w > 44 || strings.Contains(l, "─") && w != 44 {
			t.Fatalf("line %d is %d wide", i, w)
		}
		full = full || w == 44
	}
	if !full {
		t.Fatal("no line is 44 wide")
	}
	if strings.TrimRight(got[19], " ") != ">" {
		t.Fatalf("last line = %q", got[19])
	}
	// The last prose line wraps whole: 44+44+44+44+4.
	tail := strings.Join(got[13:18], "")
	if tail != prose {
		t.Fatalf("prose %q, want %q", tail, prose)
	}
}

func TestFitTrimsTrailingBlanks(t *testing.T) {
	got := fitScreen("hi"+strings.Repeat(" ", 100), 40, 5)
	if len(got) != 1 || got[0] != "hi" {
		t.Fatalf("got %q", got)
	}
}

func TestFitKeepsStyles(t *testing.T) {
	got := fitScreen("\x1b[31mred\x1b[0m   ", 40, 5)
	if len(got) != 1 || !strings.Contains(got[0], "\x1b[31m") || ansi.Strip(got[0]) != "red" {
		t.Fatalf("got %q", got)
	}
}

func chatModel(t *testing.T) (Model, *fakeConn) {
	t.Helper()
	st := fixtureState()
	st.Sessions = []sample.Session{
		{PID: 1, Pane: "%1", Name: "a"},
		{PID: 2, Pane: "%2", Name: "b"},
	}
	return loadedWith(t, 100, st)
}

func TestScreenOfAnotherPaneIsIgnored(t *testing.T) {
	m, _ := chatModel(t)
	next, _ := m.Update(screenMsg{Pane: m.watching, Text: "mine"})
	m = next.(Model)
	next, _ = m.Update(screenMsg{Pane: "%9", Text: "other"})
	if got := next.(Model).screen; got.Text != "mine" {
		t.Fatalf("screen = %+v", got)
	}
}

func TestSelectionChangeWatches(t *testing.T) {
	m, f := chatModel(t)
	first, _ := m.selected()
	next, cmd := key(m, "down")
	drain(cmd)
	second, _ := next.(Model).selected()
	if first.Pane == second.Pane {
		t.Fatal("down did not move")
	}
	acts := f.allActs()
	if last := acts[len(acts)-1]; last.RecID != proto.WatchPrefix+second.Pane {
		t.Fatalf("last act = %q, want watch:%s", last.RecID, second.Pane)
	}
}

func TestSelectionChangeDropsTheOldScreen(t *testing.T) {
	m, _ := chatModel(t)
	sel, _ := m.selected()
	next, _ := m.Update(screenMsg{Pane: sel.Pane, Text: "old"})
	next, _ = key(next, "down")
	if got := next.(Model).screen; got.Text != "" {
		t.Fatalf("screen of the old pane stayed: %+v", got)
	}
}

func TestLostConnectionForgetsTheWatch(t *testing.T) {
	m, _ := chatModel(t)
	next, _ := m.Update(lostMsg{err: io.EOF, conn: m.conn})
	if next.(Model).watching != "" {
		t.Fatal("still watching after the connection went")
	}
}

func TestChatLinesWhenTheSessionEnded(t *testing.T) {
	m, _ := chatModel(t)
	sel, _ := m.selected()
	next, _ := m.Update(screenMsg{Pane: sel.Pane, Gone: true})
	lines := next.(Model).chatLines(40, 5)
	if len(lines) != 5 || strings.TrimSpace(lines[2]) != "That session ended." {
		t.Fatalf("lines = %q", lines)
	}
}

func TestChatLinesFillTheViewFromTheBottom(t *testing.T) {
	m, _ := chatModel(t)
	sel, _ := m.selected()
	next, _ := m.Update(screenMsg{Pane: sel.Pane, Text: "one\ntwo"})
	lines := next.(Model).chatLines(40, 5)
	if len(lines) != 5 || lines[3] != "one" || lines[4] != "two" || lines[0] != "" {
		t.Fatalf("lines = %q", lines)
	}
	next, _ = m.Update(screenMsg{Pane: sel.Pane, Text: "   \n\n"})
	if lines := next.(Model).chatLines(40, 5); len(lines) != 5 {
		t.Fatalf("a blank screen gave %d lines", len(lines))
	}
}

func TestRefusedWatchRetriesOnTheNextSnapshotAndSaysNothing(t *testing.T) {
	m, f := chatModel(t)
	sel, _ := m.selected()
	next, cmd := m.Update(resultMsg(proto.Result{ActID: "watch:" + sel.Pane, Err: "no agent session in " + sel.Pane + " any more"}))
	drain(cmd)
	mm := next.(Model)
	if mm.flash != "" || mm.watching != "" {
		t.Fatalf("flash %q watching %q", mm.flash, mm.watching)
	}
	next, cmd = mm.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	drain(cmd)
	n := len(f.allActs())
	next, cmd = next.Update(stateMsg(fixtureStateWith(sel)))
	drain(cmd)
	acts := f.allActs()
	if len(acts) != n+1 || acts[n].RecID != proto.WatchPrefix+sel.Pane || next.(Model).flash != "" {
		t.Fatalf("acts %+v", acts[n:])
	}
}

func fixtureStateWith(ss ...sample.Session) proto.State {
	st := fixtureState()
	st.Sessions = ss
	return st
}

func TestChatLinesTakeNoNegativeHeight(t *testing.T) {
	m, _ := chatModel(t)
	sel, _ := m.selected()
	next, _ := m.Update(screenMsg{Pane: sel.Pane, Text: "one"})
	if lines := next.(Model).chatLines(40, -1); len(lines) != 0 {
		t.Fatalf("lines = %q", lines)
	}
}

func TestRefusalOfAnOldWatchHoldsNothing(t *testing.T) {
	m, _ := chatModel(t)
	a, _ := m.selected()
	next, cmd := key(m, "down")
	drain(cmd)
	b, _ := next.(Model).selected()
	next, _ = next.Update(screenMsg{Pane: b.Pane, Text: "kept"})
	next, _ = next.Update(resultMsg(proto.Result{ActID: "watch:" + a.Pane, Err: "no agent session in " + a.Pane + " any more"}))
	mm := next.(Model)
	if mm.watchHeld != "" || mm.watching != b.Pane || mm.screen.Text != "kept" {
		t.Fatalf("held %q watching %q screen %q", mm.watchHeld, mm.watching, mm.screen.Text)
	}
}
