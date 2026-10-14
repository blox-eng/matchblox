package app

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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
	rule := lines[2] // the header, the tabs, then the rule
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

// TestAQuietNightShowsItsCard: the night's score takes the dock's stoker
// panel until it is seen, and c opens the whole card.
func TestAQuietNightShowsItsCard(t *testing.T) {
	for _, w := range []int{40, 120} {
		m, _ := loadedWith(t, w, stokerState(quietNight()))
		if out := ansi.Strip(m.render()); !strings.Contains(out, "STOKER") || !strings.Contains(out, "ready 7/7") || strings.Contains(out, "NIGHT · 01:08") {
			t.Fatalf("w=%d: the dock does not show the night's score:\n%s", w, out)
		}
		out := ansi.Strip(openCard(m).(Model).render())
		for _, want := range []string{"NIGHT · 01:08 → 07:00", "ready 7/7", "7 watched", "0 compacted"} {
			if !strings.Contains(out, want) {
				t.Fatalf("w=%d: no %q in\n%s", w, want, out)
			}
		}
	}
}

func openCard(m tea.Model) tea.Model {
	next, _ := key(m, "c")
	return next
}

func TestTheCardNamesWhatIsNotReady(t *testing.T) {
	v := quietNight()
	c := *v.Card
	c.Ready, c.Full, c.Compacted, c.Waited, c.Given = 5, 2, 1, 1, 166600
	c.NotReady = []proto.StokerMiss{{Name: "api-gw", Why: "waits for you"}, {Name: "billing", Why: "full · 91%"}}
	c.Left = []proto.StokerMiss{{Name: "Billed", Why: "69% · finishes ~79% · left alone"}}
	v.Card = &c
	m, _ := loadedWith(t, 160, stokerState(v))
	out := ansi.Strip(openCard(m).(Model).render())
	for _, want := range []string{"ready 5/7", "2 were full", "1 compacted", "1 waited", "167k tokens given back", "api-gw", "waits for you", "billing", "full · 91%", "Billed", "69% · finishes ~79% · left alone"} {
		if !strings.Contains(out, want) {
			t.Fatalf("no %q in\n%s", want, out)
		}
	}
}

func TestADayRunCardIsStoked(t *testing.T) {
	v := quietNight()
	v.Nightly, v.Card.Night = false, false
	m, _ := loadedWith(t, 120, stokerState(v))
	if out := ansi.Strip(openCard(m).(Model).render()); !strings.Contains(out, "STOKER · 01:08 → 07:00") {
		t.Fatalf("%s", out)
	}
}

func TestOpeningTheTabMarksTheStepsSeen(t *testing.T) {
	m, f := loadedWith(t, 120, stokerState(quietNight()))
	sendKey(m, "c")
	if a := f.acts(); len(a) != 1 || a[0].RecID != "stoker:ack" {
		t.Fatalf("acts %+v", a)
	}
	v := quietNight()
	v.Unseen = false
	m, _ = loadedWith(t, 120, stokerState(v))
	if out := ansi.Strip(m.render()); strings.Contains(out, "NIGHT · 01:08") {
		t.Fatalf("a folded card shows:\n%s", out)
	}
}

func TestSummaryCarriesTheCard(t *testing.T) {
	v := quietNight()
	c := *v.Card
	c.Ready, c.Compacted, c.Given = 6, 1, 166600
	c.NotReady = []proto.StokerMiss{{Name: "api-gw", Why: "waits for you"}}
	c.Left = []proto.StokerMiss{{Name: "Billed", Why: "69% · finishes ~79% · left alone"}}
	v.Card = &c
	out := Summary(stokerState(v).Doc)
	for _, want := range []string{
		"  left alone: Billed (69% · finishes ~79%)\n",
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

// TestTheCardLeavesSessions: the stoker's card is in the dock, not on top
// of Sessions, so the setup steps open the list.
func TestTheCardLeavesSessions(t *testing.T) {
	st := doorsAndQueue(hooksDoor)
	v := quietNight()
	st.Stoker = &v
	m, _ := loadedWith(t, 120, st)
	lines := strings.Split(ansi.Strip(m.render()), "\n")
	if !strings.HasPrefix(lines[3], " SET UP") || strings.Contains(strings.Join(lines[:10], "\n"), "NIGHT 01:08") {
		t.Fatalf("the card sits on top of Sessions:\n%s", strings.Join(lines, "\n"))
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

// A run the limit stopped says so first, with when it came back; each
// session it stopped is named with until when.
func TestTheCardSaysTheLimitRanOut(t *testing.T) {
	v := quietNight()
	c := *v.Card
	from := time.Date(2026, 10, 11, 1, 51, 0, 0, time.Local)
	back := time.Date(2026, 10, 11, 4, 50, 0, 0, time.Local)
	c.Ready = 0
	c.NotReady = []proto.StokerMiss{{Name: "billing", Why: "out until 04:50"}}
	c.Lapses = []proto.StokerLapse{{From: from, Until: back}}
	v.Card = &c
	for _, w := range []int{40, 120} {
		m0, _ := loadedWith(t, w, stokerState(v))
		m := openCard(m0)
		lines := strings.Split(ansi.Strip(m.(Model).render()), "\n")
		at := lineOf(t, m, "NIGHT · 01:08")
		if !strings.Contains(lines[at+1], "limit out 01:51 → 04:50") {
			t.Fatalf("w=%d: the lapse is not the card's first line:\n%s", w, strings.Join(lines[at:at+4], "\n"))
		}
	}
	out := Summary(stokerState(v).Doc)
	for _, want := range []string{"  limit out 01:51 → 04:50\n", "  not ready: billing (out until 04:50)\n"} {
		if !strings.Contains(out, want) {
			t.Fatalf("no %q in\n%s", want, out)
		}
	}
}

// A night across midnight writes the lapse with times alone.
func TestALapseAfterMidnightHasNoWeekday(t *testing.T) {
	since := time.Date(2026, 10, 10, 23, 10, 0, 0, time.Local)
	c := proto.StokerCard{Night: true, Since: since, Ended: since.Add(8 * time.Hour),
		Lapses: []proto.StokerLapse{{From: since.Add(5 * time.Hour), Until: since.Add(7*time.Hour + 30*time.Minute)}}}
	if got := lapseLines(c); len(got) != 1 || got[0] != "limit out 04:10 → 06:40" {
		t.Fatalf("%q", got)
	}
}
