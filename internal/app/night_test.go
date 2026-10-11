package app

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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

// quietNight ended at 07:00 with nothing to do: no step, all ready.
func quietNight() proto.StokerView {
	end := time.Date(2026, 10, 11, 7, 0, 0, 0, time.Local)
	since := time.Date(2026, 10, 11, 1, 8, 0, 0, time.Local)
	return proto.StokerView{
		Mode: proto.StokerMode{Since: since, Ended: end, Nightly: true,
			Card: &proto.StokerCard{Night: true, Since: since, Ended: end, Watched: 7, Ready: 7, Of: 7}},
		Unseen: true,
	}
}

func TestAQuietNightShowsItsCard(t *testing.T) {
	for _, w := range []int{40, 120} {
		m, _ := loadedWith(t, w, stokerState(quietNight()))
		out := ansi.Strip(m.render())
		for _, want := range []string{"NIGHT 01:08 → 07:00 · ready 7/7", "7 watched", "0 compacted"} {
			if !strings.Contains(out, want) {
				t.Fatalf("w=%d: no %q in\n%s", w, want, out)
			}
		}
		if d, ok := m.selectedDoor(); !ok || d.ID != stokedRow {
			t.Fatalf("w=%d: the card is not selected first", w)
		}
	}
}

func TestTheCardNamesWhatIsNotReady(t *testing.T) {
	v := quietNight()
	c := *v.Card
	c.Ready, c.Full, c.Compacted, c.Waited, c.Given = 5, 2, 1, 1, 166600
	c.NotReady = []proto.StokerMiss{{Name: "api-gw", Why: "waits for you"}, {Name: "billing", Why: "full · 91%"}}
	v.Card = &c
	m, _ := loadedWith(t, 120, stokerState(v))
	out := ansi.Strip(m.render())
	for _, want := range []string{"ready 5/7", "2 were full", "1 compacted", "1 waited", "167k tokens given back", "api-gw", "waits for you", "billing", "full · 91%"} {
		if !strings.Contains(out, want) {
			t.Fatalf("no %q in\n%s", want, out)
		}
	}
}

func TestADayRunCardIsStoked(t *testing.T) {
	v := quietNight()
	v.Nightly, v.Card.Night = false, false
	m, _ := loadedWith(t, 120, stokerState(v))
	if out := ansi.Strip(m.render()); !strings.Contains(out, "STOKED 01:08 → 07:00 · ready 7/7") {
		t.Fatalf("%s", out)
	}
}

func TestEnterFoldsTheCard(t *testing.T) {
	m, f := loadedWith(t, 120, stokerState(quietNight()))
	sendKey(m, "enter")
	if a := f.acts(); len(a) != 1 || a[0].RecID != "stoker:ack" {
		t.Fatalf("acts %+v", a)
	}
	v := quietNight()
	v.Unseen = false
	m, _ = loadedWith(t, 120, stokerState(v))
	if out := ansi.Strip(m.render()); strings.Contains(out, "NIGHT 01:08") {
		t.Fatalf("a folded card shows:\n%s", out)
	}
}

func TestSummaryCarriesTheCard(t *testing.T) {
	v := quietNight()
	c := *v.Card
	c.Ready, c.Compacted, c.Given = 6, 1, 166600
	c.NotReady = []proto.StokerMiss{{Name: "api-gw", Why: "waits for you"}}
	v.Card = &c
	out := Summary(stokerState(v).Doc)
	for _, want := range []string{
		"stoker: night 01:08 → 07:00 · ready 6/7 · 7 watched · 0 were full · 1 compacted · 0 waited · 167k tokens given back\n",
		"  not ready: api-gw (waits for you)\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("no %q in\n%s", want, out)
		}
	}
	on := stokerState(proto.StokerView{Mode: proto.StokerMode{On: true, Until: time.Date(2026, 10, 12, 7, 0, 0, 0, time.Local)}})
	if out := Summary(on.Doc); !strings.Contains(out, "stoker: night → 07:00\n") {
		t.Fatalf("a running night:\n%s", out)
	}
}

func TestTheCardStandsApartFromSetUp(t *testing.T) {
	st := doorsAndQueue(hooksDoor)
	v := quietNight()
	st.Stoker = &v
	m, _ := loadedWith(t, 120, st)
	lines := strings.Split(ansi.Strip(m.render()), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, " SET UP") && strings.TrimSpace(lines[i-1]) != "" {
			t.Fatalf("SET UP sits on the card: %q", lines[i-1])
		}
	}
}

func TestTheNightCoversEveryScreen(t *testing.T) {
	m, _ := loadedWith(t, 120, nightState())
	m.mismatch = true
	if n := len(parsed(t, m.render())); n != 30 {
		t.Fatalf("%d lines painted, the screen has 30", n)
	}
}

func TestStatusShowsTheCardUntilItIsFolded(t *testing.T) {
	v := quietNight()
	v.Unseen = false
	if out := Summary(stokerState(v).Doc); strings.Contains(out, "stoker:") {
		t.Fatalf("a folded card:\n%s", out)
	}
}
