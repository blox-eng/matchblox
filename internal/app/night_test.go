package app

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/replay"
)

var nightTokens = []string{"night-bg", "night-fg", "night-muted", "night-faint", "night-hair", "night-wash", "night"}

func TestNightTokensAreTheSameInBothThemes(t *testing.T) {
	dark, light := Tokens(true), Tokens(false)
	for _, n := range nightTokens {
		if dark[n] == nil || light[n] == nil {
			t.Fatalf("token %s missing", n)
		}
		if hex(dark[n]) != hex(light[n]) {
			t.Fatalf("%s: dark %s, light %s: the night is one palette", n, hex(dark[n]), hex(light[n]))
		}
	}
	seen := map[string]string{}
	for n, c := range dark {
		if o, ok := seen[hex(c)]; ok {
			t.Fatalf("%s and %s are both %s: the replay cannot tell them apart", n, o, hex(c))
		}
		seen[hex(c)] = n
	}
}

func hex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
}

// dayInk are the text colours the night replaces; "" is the terminal's own.
var dayInk = map[string]bool{"": true, "fg": true, "muted": true, "faint": true, "hair": true}

func nightState() proto.State {
	until := time.Date(2026, 10, 10, 7, 0, 0, 0, time.Local)
	return stokerState(proto.StokerView{Mode: proto.StokerMode{On: true, Since: until.Add(-6 * time.Hour), Until: until}})
}

// parsed reads a frame the way the replay does, in the dark tokens.
func parsed(t *testing.T, view string) [][]replay.Span {
	t.Helper()
	lines, err := replay.Parse(view, Tokens(true))
	if err != nil {
		t.Fatal(err)
	}
	return lines
}

func TestNightPaintsEveryCell(t *testing.T) {
	for _, w := range []int{40, 120} {
		m, _ := loadedWith(t, w, nightState())
		lines := parsed(t, m.render())
		if len(lines) != 30 {
			t.Fatalf("w=%d: %d lines", w, len(lines))
		}
		for i, line := range lines {
			width := 0
			for _, s := range line {
				width += lipgloss.Width(s.Text)
				if s.BG != "night-bg" && s.BG != "night-wash" {
					t.Fatalf("w=%d line %d: span %q on %q, not the night ground", w, i, s.Text, s.BG)
				}
				if dayInk[s.FG] && strings.TrimSpace(s.Text) != "" {
					t.Fatalf("w=%d line %d: %q drawn in the day's %q", w, i, s.Text, s.FG)
				}
			}
			if width != w {
				t.Fatalf("w=%d line %d is %d wide", w, i, width)
			}
		}
	}
}

func TestNightWordAndRuleTakeTheNightToken(t *testing.T) {
	m, _ := loadedWith(t, 120, nightState())
	lines := parsed(t, m.render())
	word := false
	for _, s := range lines[0] {
		if strings.Contains(s.Text, "night → 07:00") && s.FG == "night" {
			word = true
		}
	}
	if !word {
		t.Fatalf("header %+v: the word is not in the night token", lines[0])
	}
	rule := lines[len(m.limitLines(120))+3] // header, limits, tabs, the action line, then the rule
	ruled := false
	for _, s := range rule {
		if strings.Contains(s.Text, "─") && s.FG == "night" {
			ruled = true
		}
	}
	if !ruled {
		t.Fatalf("rule %+v: not in the night token", rule)
	}
}

func TestTheFlamesStayWarmAtNight(t *testing.T) {
	m, _ := loadedWith(t, 120, nightState())
	m.tab = tabSessions
	warm := false
	for _, line := range parsed(t, m.render()) {
		for _, s := range line {
			if strings.HasPrefix(s.FG, "match-") || strings.HasPrefix(s.FG, "flame") {
				warm = true
			}
		}
	}
	if !warm {
		t.Fatal("no match drawn in its own colours at night")
	}
}

func TestDayHasNoNightGround(t *testing.T) {
	m, _ := loadedWith(t, 120, stokerState(proto.StokerView{}))
	for _, line := range parsed(t, m.render()) {
		for _, s := range line {
			if strings.HasPrefix(s.BG, "night") || strings.HasPrefix(s.FG, "night") {
				t.Fatalf("day frame has %+v", s)
			}
		}
	}
}

func TestClearsBGReadsColoursWhole(t *testing.T) {
	for params, want := range map[string]bool{
		"": true, "0": true, "49": true, "1;0": true,
		"38;2;0;10;20": false, "48;2;12;0;0": false, "38;5;0": false, "1": false, "38;2;0;0;0;49": true,
	} {
		if got := clearsBG(params); got != want {
			t.Errorf("clearsBG(%q) = %v", params, got)
		}
	}
}
