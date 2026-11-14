package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The wheel finds what it is over in the frame View drew last: a phone's
// swipe sends dozens of ticks a second, and a second draw for each made
// them queue (2026-11-10). This test proves that answer equals a new draw,
// over the live view beside the board.
func TestTheWheelReadsTheFrameOnScreen(t *testing.T) {
	m, _ := deskView(t)
	_ = m.View()
	if !m.shown.ok {
		t.Fatal("View keeps no frame: each wheel tick draws the screen again")
	}
	sc := m.draw(m.width)
	over := 0
	for y := range m.height {
		for x := 0; x < m.width; x += 7 {
			ms := tea.Mouse{X: x, Y: y}
			got, want := m.overBoardView(ms), overBoardViewOf(ms, sc)
			if got != want {
				t.Fatalf("at %d,%d the kept frame says over the view %v, a new draw %v", x, y, got, want)
			}
			if got {
				over++
			}
		}
	}
	if over == 0 {
		t.Fatal("no cell is over the view: the test proves nothing")
	}
	m.shown.sc = drawn{} // a kept frame with no live view: the wheel trusts it
	if m.overBoardView(tea.Mouse{X: m.width - 5, Y: sc.boardView.y0 + 1}) {
		t.Error("the wheel drew the screen again over a kept frame")
	}
}

func TestANewWidthDrawsAgain(t *testing.T) {
	m, _ := deskView(t)
	_ = m.View()
	m.width -= 20
	if _, kept := m.onScreen(m.width); kept {
		t.Fatal("a frame of another width answers for this one")
	}
}

func BenchmarkWheelTick(b *testing.B) {
	m := demoSized(b, 132, 32)
	sc := m.draw(m.width)
	var next tea.Model = m
	for i := 0; i < b.N; i++ {
		btn := tea.MouseWheelDown
		if (i/8)%2 == 1 {
			btn = tea.MouseWheelUp
		}
		next, _ = next.Update(tea.MouseWheelMsg{X: 5, Y: sc.bodyTop + 5, Button: btn})
		_ = next.View()
	}
}
