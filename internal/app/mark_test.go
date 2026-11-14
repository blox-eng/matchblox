package app

import (
	"image/color"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func hasColour(f [][]cell, c color.Color) bool {
	for _, row := range f {
		for _, x := range row {
			if sameColour(x.top, c) || sameColour(x.bottom, c) {
				return true
			}
		}
	}
	return false
}

func sameColour(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	r1, g1, b1, a1 := a.RGBA()
	r2, g2, b2, a2 := b.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}

func TestMarkLastFrameDarkHasFlame(t *testing.T) {
	if !hasColour(markFrame(600*time.Millisecond, true), lipgloss.Color("#E8822E")) {
		t.Fatal("in a dark terminal the match burns at the end")
	}
}

func TestMarkLightIsUnlit(t *testing.T) {
	f := markFrame(600*time.Millisecond, false)
	if hasColour(f, lipgloss.Color("#E8822E")) {
		t.Fatal("in a light terminal the match stays unlit")
	}
	if !hasColour(f, lightTheme.Accent) {
		t.Fatal("the match head is the accent")
	}
}

func TestMarkStartsEmpty(t *testing.T) {
	f := markFrame(0, true)
	for y := 4; y < len(f); y++ {
		for _, x := range f[y] {
			if x.top != nil || x.bottom != nil {
				t.Fatalf("row %d has a pixel at t=0: the blocks start above the box", y)
			}
		}
	}
}

func TestSplashUnder600ms(t *testing.T) {
	for _, dark := range []bool{true, false} {
		a, b := markFrame(splashFor, dark), markFrame(10*time.Second, dark)
		if splashFor >= 600*time.Millisecond {
			t.Fatalf("the start screen takes %v, the design allows < 600 ms", splashFor)
		}
		for y := range a {
			for x := range a[y] {
				if !sameColour(a[y][x].top, b[y][x].top) || !sameColour(a[y][x].bottom, b[y][x].bottom) {
					t.Fatalf("dark=%v: frame at %v differs from the last frame at %d,%d", dark, splashFor, x, y)
				}
			}
		}
	}
}

// splashModel is a console whose start screen began at t0 and whose clock
// reads now().
func splashModel(t0 time.Time, now *time.Time) Model {
	m := New(Options{Conn: newFake(), Now: func() time.Time { return *now }})
	m.splashAt = t0
	return m
}

func TestSplashEndsOnKey(t *testing.T) {
	t0 := time.Now()
	now := t0.Add(100 * time.Millisecond)
	m := splashModel(t0, &now)
	next, _ := m.Update(stateMsg(fixtureState()))
	next, _ = key(next, "1")
	if got := next.(Model); got.tab != 0 || got.splashing() {
		t.Fatalf("a key during the start screen ends it and does nothing else (tab %d, splashing %v)", got.tab, got.splashing())
	}
	next, _ = key(next, "tab") // a digit on the board answers its card
	if next.(Model).tab != 1 {
		t.Fatal("after the start screen, keys work")
	}
}

func TestNoMotionNoTicks(t *testing.T) {
	m := New(Options{Conn: newFake(), NoMotion: true})
	if m.splashCmd() != nil {
		t.Fatal("with no motion there is no start screen tick")
	}
	out := m.render()
	if !strings.Contains(ansi.Strip(out), "reading your matches…") { // the board's first frame (TestTheBoardsStates)
		t.Fatalf("with no motion the last frame shows at once:\n%s", ansi.Strip(out))
	}
}

func TestNoTicksAfterSplash(t *testing.T) {
	t0 := time.Now()
	now := t0.Add(600 * time.Millisecond)
	m := splashModel(t0, &now)
	_, cmd := m.Update(splashMsg{})
	if slices.ContainsFunc(msgsWithin(cmd, time.Second), func(msg tea.Msg) bool { _, ok := msg.(splashMsg); return ok }) {
		t.Fatal("after the start screen no tick is asked for") // the board's belt may come
	}
	now = t0.Add(100 * time.Millisecond)
	if _, cmd := m.Update(splashMsg{}); cmd == nil {
		t.Fatal("during the start screen the next tick is asked for")
	}
}
