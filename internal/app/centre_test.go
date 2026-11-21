package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/remote"
)

// feedEach runs each command of a batch, one after the other, and gives
// each message, and what follows, to the console. A frame tick ends its line.
func feedEach(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			m = feedEach(t, m, c)
		}
		return m
	case splashMsg, pairFrameMsg:
		return m
	default:
		next, c := m.Update(msg)
		return feedEach(t, next.(Model), c)
	}
}

// TestTheStartScreenPlaysInTheMiddleOfTheWindow: beside the agent, the
// match struck in the list's sidebar, off the middle of the window. The
// list's pane zooms while it plays and gives the window back after it
// (owner, 2026-11-20).
func TestTheStartScreenPlaysInTheMiddleOfTheWindow(t *testing.T) {
	tm := &besideTmux{winW: "160", winH: "50"}
	opt := Options{Conn: newFake(), Now: func() time.Time { return demoAt }, Beside: true, Pane: "%1", Tmux: tm.run}
	var next tea.Model = New(opt)
	next, _ = next.Update(tea.WindowSizeMsg{Width: 32, Height: 50})
	m := next.(Model)
	if !m.splashing() {
		t.Fatal("no start screen")
	}
	m = feedEach(t, m, m.besideCmd())
	if !tm.ran(remote.ZoomArgv("%1", true)) {
		t.Fatalf("the start screen plays in the sidebar: no zoom in %q", tm.lines())
	}
	if tm.ran(remote.ZoomArgv("%1", false)) {
		t.Fatalf("the split's size ended the start screen's zoom: %q", tm.lines())
	}
	tm.calls = nil
	m.splashDone = true
	next, cmd := m.Update(splashMsg{})
	feedEach(t, next.(Model), cmd)
	if !tm.ran(remote.ZoomArgv("%1", false)) || !tm.ran(remote.ListWidthArgv("%1", listWide)) {
		t.Fatalf("the window is not given back after the start screen: %q", tm.lines())
	}
}

// TestThePairingPagePlaysInTheMiddleOfTheWindow: the knock and the
// opening take the whole window, as the start screen does.
func TestThePairingPagePlaysInTheMiddleOfTheWindow(t *testing.T) {
	m, tm := besideConsoleWith(t, boardFixture(t), 32, 50, &besideTmux{winW: "160", winH: "50"})
	tm.calls = nil
	m.pair = &pairPage{}
	next, cmd := m.Update(besideSizeMsg{160, 50})
	feedEach(t, next.(Model), cmd)
	if !tm.ran(remote.ZoomArgv("%1", true)) {
		t.Fatalf("the pairing page plays in the sidebar: no zoom in %q", tm.lines())
	}
}
