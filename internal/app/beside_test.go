package app

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
)

// besideTmux is a fake tmux that records each command and answers the
// window's width and the new pane's id.
type besideTmux struct {
	winW  string
	calls [][]string
}

func (b *besideTmux) run(args ...string) (string, error) {
	b.calls = append(b.calls, args)
	switch args[0] {
	case "display":
		return b.winW + "\n", nil
	case "split-window":
		return "%9\n", nil
	}
	return "", nil
}

func (b *besideTmux) lines() []string {
	var out []string
	for _, c := range b.calls {
		out = append(out, strings.Join(c, " "))
	}
	return out
}

func (b *besideTmux) ran(argv []string) bool {
	return slices.Contains(b.lines(), strings.Join(argv[1:], " "))
}

func (b *besideTmux) count(prefix string) (n int) {
	for _, l := range b.lines() {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}
	return n
}

// besideConsole is the console at w by h in a window of winW columns, its
// split done.
func besideConsole(t *testing.T, st proto.State, w, h int, winW string) (Model, *besideTmux) {
	t.Helper()
	tm := &besideTmux{winW: winW}
	opt := Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }, Beside: true, Pane: "%1", Tmux: tm.run}
	var next tea.Model = New(opt)
	next, _ = next.Update(tea.WindowSizeMsg{Width: w, Height: h})
	next, _ = next.Update(stateMsg(st))
	m := next.(Model)
	m.splashDone = true
	m.viewPrefs.CopyCheckedPhone, m.viewPrefs.CopyCheckedDesk = true, true
	m = feed(t, m, m.besideCmd())
	return m, tm
}

// feed runs cmd and gives its message, and what follows, to the console.
func feed(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if _, batch := msg.(tea.BatchMsg); batch {
		return m // timers and the like: nothing for tmux
	}
	next, cmd := m.Update(msg)
	return feed(t, next.(Model), cmd)
}

func TestTheConsoleSplitsItsWindowOnceAtStart(t *testing.T) {
	m, tm := besideConsole(t, boardFixture(t), 40, 50, "160")
	if n := tm.count("split-window -h -d -t %1"); n != 1 {
		t.Fatalf("%d splits: %q", n, tm.lines())
	}
	joined := strings.Join(tm.lines(), "\n")
	for _, want := range []string{"set-option -w -t %1 @matchblox_console %1", "set-option -w -t %1 @matchblox_beside %9", "pane-active-border-style fg=#c3a56a"} {
		if !strings.Contains(joined, want) {
			t.Errorf("no %q in %q", want, joined)
		}
	}
	if m.beside != "%9" {
		t.Fatalf("beside = %q", m.beside)
	}
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 40, Height: 50})
	feed(t, next.(Model), cmd)
	if n := tm.count("split-window"); n != 1 {
		t.Fatalf("a second size made %d splits", n)
	}
}

func TestABesideConsoleDrawsOnlyTheList(t *testing.T) {
	m, _ := besideConsole(t, boardFixture(t), 40, 50, "160")
	frame := plainFrame(m)
	if !strings.Contains(frame, "kindle-web") {
		t.Fatalf("no list rows in\n%s", frame)
	}
	for _, not := range []string{"send to", "ctrl+o expand"} {
		if strings.Contains(frame, not) {
			t.Fatalf("%q drawn in\n%s", not, frame)
		}
	}
}

func TestTheZoomFollowsTheWindowWidth(t *testing.T) {
	for _, c := range []struct {
		winW string
		zoom bool
	}{{"160", false}, {"70", true}} {
		m, tm := besideConsole(t, boardFixture(t), 40, 50, c.winW)
		if !tm.ran(remote.ZoomArgv("%1", c.zoom)) {
			t.Errorf("window %s: no zoom=%v in %q", c.winW, c.zoom, tm.lines())
		}
		if tm.ran(remote.ZoomArgv("%1", !c.zoom)) {
			t.Errorf("window %s: zoom=%v too", c.winW, !c.zoom)
		}
		if c.zoom && tm.count("resize-pane -t") != 0 {
			t.Errorf("window %s: a resize would end the zoom: %q", c.winW, tm.lines())
		}
		if got := m.onePane(40); got != c.zoom {
			t.Errorf("window %s: onePane = %v", c.winW, got)
		}
	}
}

func TestTheListKeepsItsWidth(t *testing.T) {
	for _, c := range []struct {
		winW, height string
		w            int
	}{{"200", "50", 40}, {"110", "60", 32}} {
		h := 50
		if c.height == "60" {
			h = 60
		}
		_, tm := besideConsole(t, boardFixture(t), 40, h, c.winW)
		if want := remote.ListWidthArgv("%1", c.w); !tm.ran(want) {
			t.Errorf("window %s: no %v in %q", c.winW, want, tm.lines())
		}
	}
}

func TestTheDemoKeepsTheDrawnChat(t *testing.T) {
	tm := &besideTmux{}
	opt := Options{NoMotion: true, Conn: newFake(), Now: func() time.Time { return demoAt }, Pane: "%1", Tmux: tm.run}
	var next tea.Model = New(opt)
	next, _ = next.Update(tea.WindowSizeMsg{Width: 160, Height: 38})
	next, _ = next.Update(stateMsg(boardFixture(t)))
	m := next.(Model)
	m.splashDone = true
	if m.besideCmd() != nil {
		t.Fatal("a console without Beside runs tmux")
	}
	if !strings.Contains(plainFrame(m), "│") {
		t.Fatalf("no chat place in\n%s", plainFrame(m))
	}
	if len(tm.calls) != 0 {
		t.Fatalf("tmux calls: %q", tm.lines())
	}
}

func TestAFailedSplitKeepsTheDrawnChat(t *testing.T) {
	tm := &besideTmux{winW: "160"}
	opt := Options{NoMotion: true, Conn: newFake(), Beside: true, Pane: "%1", Tmux: func(args ...string) (string, error) {
		if args[0] == "split-window" {
			return "", errors.New("no server")
		}
		return tm.run(args...)
	}}
	m := New(opt)
	m = feed(t, m, m.besideCmd())
	if m.beside != "" || !strings.Contains(m.flash, "the agent pane did not open") {
		t.Fatalf("beside %q, flash %q", m.beside, m.flash)
	}
}
