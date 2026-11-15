package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

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
