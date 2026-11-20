package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/remote"
)

// boxedConsole is a beside console in a window of 160 columns whose list
// pane is 32 wide, with the launch sheet open and the zoom it asks for done.
func boxedConsole(t *testing.T) (Model, *besideTmux) {
	t.Helper()
	m, tm := besideConsoleWith(t, boardFixture(t), 32, 50, &besideTmux{winW: "160", winH: "50", agent: "top\n\n\nagent says hello\nsecond line\n"})
	tm.calls = nil
	m.sheet = &sheet{chain: "c1", chainName: "flint"} // a chain's start sheet: no stoker needed
	next, cmd := m.Update(besideSizeMsg{160, 50})
	m = feed(t, next.(Model), cmd)
	next, cmd = m.Update(tea.WindowSizeMsg{Width: 160, Height: 50}) // tmux gave the console the window
	return feed(t, next.(Model), cmd), tm
}

// TestABoxTakesTheWholeWindow: a box opened beside the agent drew in the
// 32 columns of the list pane, its words cut. It zooms the list's pane to
// the window and centres over all of it (2026-11-18).
func TestABoxTakesTheWholeWindow(t *testing.T) {
	m, tm := boxedConsole(t)
	if !tm.ran(remote.ZoomArgv("%1", true)) {
		t.Fatalf("no zoom of the console for the box: %q", tm.lines())
	}
	if tm.ran(remote.ZoomArgv("%1", false)) {
		t.Fatalf("the window's new size ended the box's zoom: %q", tm.lines())
	}
	sc := m.draw(160)
	if want := (160 - sc.boxW) / 2; sc.boxX != want || sc.boxW < 60 {
		t.Fatalf("box at x %d, %d wide; want x %d, 60 or more wide", sc.boxX, sc.boxW, want)
	}
}

// TestTheAgentShowsDimBehindTheBox: the agent's pane stays in view, dim,
// beside the list, behind the box.
func TestTheAgentShowsDimBehindTheBox(t *testing.T) {
	m, _ := boxedConsole(t)
	frame := plainFrame(m)
	if !strings.Contains(frame, "agent says hello") {
		t.Fatalf("no agent screen behind the box:\n%s", frame)
	}
	if !strings.Contains(frame, "kindle-web") {
		t.Fatalf("no list behind the box:\n%s", frame)
	}
}

// TestAClosedBoxGivesTheWindowBack: the zoom ends, and the list goes back
// to its width beside the agent.
func TestAClosedBoxGivesTheWindowBack(t *testing.T) {
	m, tm := boxedConsole(t)
	tm.calls = nil
	m.sheet = nil
	next, cmd := m.Update(besideSizeMsg{160, 50})
	feed(t, next.(Model), cmd)
	if !tm.ran(remote.ZoomArgv("%1", false)) || !tm.ran(remote.ListWidthArgv("%1", listWide)) {
		t.Fatalf("the window is not given back: %q", tm.lines())
	}
}

// TestADragBehindABoxSelectsNothing: the dim agent behind a box is a
// picture in the list's pane; a drag over it would copy the sidebar's
// text again (mbcopy, 2026-11-18).
func TestADragBehindABoxSelectsNothing(t *testing.T) {
	m, _ := boxedConsole(t)
	if m = m.dragStart(100, 10); m.dragFrom != nil {
		t.Fatal("a press behind the box starts a drag")
	}
}

// TestABoxClosedBeforeItsZoomLeavesNoZoom: esc before tmux zoomed for the
// box. The two tmux runs race; a zoom that lands last must not leave the
// list zoomed with no box, the agent hidden.
func TestABoxClosedBeforeItsZoomLeavesNoZoom(t *testing.T) {
	m, tm := besideConsoleWith(t, boardFixture(t), 32, 50, &besideTmux{winW: "160", winH: "50"})
	m.sheet = &sheet{chain: "c1", chainName: "flint"}
	next, zoom := m.Update(besideSizeMsg{160, 50})
	m = next.(Model)
	m.sheet = nil
	next, unzoom := m.Update(besideSizeMsg{160, 50})
	m = next.(Model)
	m = feed(t, m, unzoom) // the give-back runs first
	tm.calls = nil
	next, cmd := m.Update(zoom()) // then the zoom lands
	feed(t, next.(Model), cmd)
	if !tm.ran(remote.ZoomArgv("%1", false)) {
		t.Fatalf("the late zoom keeps the window: %q", tm.lines())
	}
}

// TestABoxInAOnePaneWindowLeavesTheListWhole: a phone or a narrow window
// shows one pane, which already has the whole window. A box there lends
// nothing: the list stays whole behind it, with no agent picture.
func TestABoxInAOnePaneWindowLeavesTheListWhole(t *testing.T) {
	m, tm := besideConsoleWith(t, boardFixture(t), 70, 50, &besideTmux{winW: "70", winH: "50", agent: "top\n\n\nagent says hello\n"})
	tm.calls = nil
	m.sheet = &sheet{chain: "c1", chainName: "flint"}
	next, cmd := m.Update(besideSizeMsg{70, 50})
	m = feed(t, next.(Model), cmd)
	if m.boxZoom || tm.count("capture-pane") != 0 {
		t.Fatalf("a one-pane window zoomed for the box: %q", tm.lines())
	}
	if strings.Contains(plainFrame(m), "agent says hello") {
		t.Fatal("the agent picture cuts the list in a one-pane window")
	}
}
