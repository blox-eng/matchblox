package app

import (
	"image/color"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

const ruleGlyphs = "│┬┴┿┼"

// nowState is a host with api at work, docs idle, and bench on a limit
// whose 5 hours reset in 35 minutes.
func nowState(now time.Time) proto.State {
	out := now.Add(35 * time.Minute)
	st := hostState(
		sess(1, "api", "%1", busy),
		sess(2, "docs", "%2", nil),
		sess(3, "bench", "%3", func(s *sample.Session) { s.Out = out }),
	)
	st.Limits = []limits.Account{{Provider: "claude", Account: "a@b.c", State: limits.Measured, At: now,
		Windows: []limits.Window{{UsedPct: 100, Minutes: 300, ResetsAt: out}, {UsedPct: 40, Minutes: limits.WeekMinutes, ResetsAt: now.Add(72 * time.Hour)}}}}
	return st
}

// nowModel is nowState in list only, w columns wide.
func nowModel(t *testing.T, w int) Model {
	t.Helper()
	now := laneAt
	m, _, _ := hostsAt(t, &now, "", w, 30)
	m = upd(t, m, stateMsg(nowState(now)))
	m.viewPrefs.ListOnlyDesk = true
	m.splashDone = true
	return m
}

// canvasOf is a frame as cells.
func canvasOf(frame string) *lipgloss.Canvas {
	lines := strings.Split(frame, "\n")
	w := 0
	for _, l := range lines {
		w = max(w, lipgloss.Width(l))
	}
	c := lipgloss.NewCanvas(w, len(lines))
	c.Compose(lipgloss.NewLayer(frame))
	return c
}

func cellGlyph(c *lipgloss.Canvas, x, y int) string {
	if cell := c.CellAt(x, y); cell != nil {
		return cell.Content
	}
	return ""
}

// ruleOf is the rule's column, from its meeting with the top hairline, and
// its last line, from its meeting with the bottom one.
func ruleOf(t *testing.T, c *lipgloss.Canvas) (x, bottom int) {
	t.Helper()
	x = -1
	for i := range c.Width() {
		if cellGlyph(c, i, 2) == "┬" {
			if x >= 0 {
				t.Fatalf("the top hairline meets the rule at %d and %d", x, i)
			}
			x = i
		}
	}
	if x < 0 {
		t.Fatal("the top hairline has no ┬")
	}
	for y := c.Height() - 1; y > 2; y-- {
		if cellGlyph(c, x, y) == "┴" {
			return x, y
		}
	}
	t.Fatal("the bottom hairline has no ┴")
	return 0, 0
}

func rowLine(t *testing.T, c *lipgloss.Canvas, word string) int {
	t.Helper()
	for y, l := range strings.Split(ansi.Strip(c.Render()), "\n") {
		if strings.Contains(l, word) {
			return y
		}
	}
	t.Fatalf("no line has %q", word)
	return -1
}

func TestNowRuleIsOneColumn(t *testing.T) {
	m := nowModel(t, 132)
	c := canvasOf(m.render())
	x, bottom := ruleOf(t, c)
	if bottom != c.Height()-2 {
		t.Fatalf("the rule ends on line %d, the bottom hairline is %d", bottom, c.Height()-2)
	}
	for y := range c.Height() {
		cell := c.CellAt(x, y)
		ruled := strings.Contains(ruleGlyphs, cell.Content) || strings.TrimSpace(cell.Content) != "" && sameColor(cell.Style.Fg, m.st.theme.Accent)
		if in := y >= 2 && y <= bottom; ruled != in || in && !sameColor(cell.Style.Fg, m.st.theme.Accent) {
			t.Errorf("line %d has %q in %v at the rule's column %d", y, cell.Content, cell.Style.Fg, x)
		}
	}
}

func TestNowRuleKeepsTheSelectedWash(t *testing.T) {
	m := nowModel(t, 132)
	c := canvasOf(m.render())
	x, _ := ruleOf(t, c)
	y := rowLine(t, c, "▌")
	cell := c.CellAt(x, y)
	if !strings.Contains(ruleGlyphs, cell.Content) {
		t.Fatalf("the selected row has %q at the rule", cell.Content)
	}
	if !sameColor(cell.Style.Bg, m.st.theme.AccentWash) {
		t.Fatalf("the rule's cell on the selected row is on %v, not the wash", cell.Style.Bg)
	}
}

func TestNowRuleCrossesBars(t *testing.T) {
	c := canvasOf(nowModel(t, 132).render())
	x, _ := ruleOf(t, c)
	if g := cellGlyph(c, x, rowLine(t, c, "api")); g != "┿" {
		t.Fatalf("the working row has %q at the rule", g)
	}
}

func TestNoLaneUnder107(t *testing.T) {
	f := ansi.Strip(nowModel(t, 106).render())
	if strings.Contains(f, "−2h") || strings.ContainsAny(f, "┬┴┿━") {
		t.Fatalf("a list of 106 columns has a lane:\n%s", f)
	}
}

func TestResizeMovesTheRule(t *testing.T) {
	m := nowModel(t, 132)
	old, _ := ruleOf(t, canvasOf(m.render()))
	m = upd(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	c := canvasOf(m.render())
	x, _ := ruleOf(t, c)
	if x == old {
		t.Fatalf("the rule stays at %d after the resize", x)
	}
	for y := range c.Height() {
		if g := cellGlyph(c, old, y); strings.Contains(ruleGlyphs, g) {
			t.Errorf("line %d keeps %q at the old column %d", y, g, old)
		}
	}
}

func TestLimitRunsToItsReset(t *testing.T) {
	f := ansi.Strip(nowModel(t, 132).render())
	row := lineWith(strings.Split(f, "\n"), "bench")
	if !strings.Contains(row, "│"+strings.Repeat("┄", 7)+" 5h resets") && !strings.Contains(row, "┼"+strings.Repeat("┄", 7)+" 5h resets") {
		t.Fatalf("the limited row runs %q", row)
	}
}

func TestLaneCellsDrawEachState(t *testing.T) {
	var l [lanePast]laneState
	l[0], l[lanePast-3], l[lanePast-2], l[lanePast-1] = laneIdle, laneWork, laneWait, laneIdle
	got := ansi.Strip(laneCells(l, time.Time{}, "", laneAt, newStyles(true)))
	want := "‹" + strings.Repeat(" ", lanePast-4) + "━━╌╌"
	if got != want {
		t.Fatalf("the lane is %q, want %q", got, want)
	}
}

func sameColor(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	r1, g1, b1, a1 := a.RGBA()
	r2, g2, b2, a2 := b.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}
