package app

import (
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

func TestFitKeepsTheBottomAndCutsRules(t *testing.T) {
	var lines []string
	for i := 0; i < 39; i++ {
		if i%2 == 0 {
			lines = append(lines, strings.Repeat("─", 200))
		} else {
			lines = append(lines, strings.Repeat("p", 180))
		}
	}
	lines = append(lines, "> ")
	got := fitScreen(strings.Join(lines, "\n"), 44, 20)
	if len(got) != 20 {
		t.Fatalf("got %d lines, want 20", len(got))
	}
	for i, l := range got {
		w := ansi.StringWidth(l)
		if w > 44 || strings.Contains(l, "─") && w != 44 {
			t.Fatalf("line %d is %d wide", i, w)
		}
	}
	if strings.TrimRight(got[19], " ") != ">" {
		t.Fatalf("last line = %q", got[19])
	}
	// The wrapped prose keeps all its text: 180 p's in 5 lines of 44 at most.
	if n := strings.Count(strings.Join(got, ""), "p"); n%180 != 0 || n == 0 {
		t.Fatalf("prose was cut: %d p's", n)
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
