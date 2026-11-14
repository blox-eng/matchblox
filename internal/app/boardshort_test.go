package app

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// shortBoard is the chain fixture, kindle-web asking, on a w by h screen.
func shortBoard(t *testing.T, w, h int) (Model, *fakeConn) {
	t.Helper()
	m, f := chainBoard(t, w, headState(t, runningPlan(), true), "")
	return upd(t, m, tea.WindowSizeMsg{Width: w, Height: h}), f
}

// TestAShortScreenKeepsTheChosenCard: on a short screen the ghost row, the
// recommendations and the strip fold before the chosen card's head goes;
// where no card can be drawn at all, no key acts on one.
func TestAShortScreenKeepsTheChosenCard(t *testing.T) {
	for _, size := range [][2]int{{44, 10}, {44, 8}, {80, 8}, {96, 8}, {100, 8}, {160, 8}} {
		m, _ := shortBoard(t, size[0], size[1])
		if f := plainFrame(m); !strings.Contains(f, "kindle-web") {
			t.Errorf("%dx%d: the chosen card is not drawn:\n%s", size[0], size[1], f)
		}
	}
	for _, w := range []int{44, 100} {
		noCardDrawn(t, w, map[int]int{44: 7, 100: 6}[w])
	}
}

func noCardDrawn(t *testing.T, w, h int) {
	t.Helper()
	m, f := shortBoard(t, w, h)
	if strings.Contains(plainFrame(m), "kindle-web") {
		t.Fatalf("%dx%d has room for a card:\n%s", w, h, plainFrame(m))
	}
	for _, hn := range m.boardHints(w) {
		if hn.label == "go in" || hn.label == "mark an answer" {
			t.Errorf("%dx%d: the key line offers %q for a card it does not show", w, h, hn.label)
		}
	}
	for _, k := range []string{"z", "x"} {
		next := pressOn(m, k)
		if len(f.acts()) != 0 || next.pending != nil || next.noticeText != "no card is shown" {
			t.Errorf("%dx%d: %s with no card drawn: acts %+v, pending %v, notice %q", w, h, k, f.acts(), next.pending, next.noticeText)
		}
	}
}

// TestOnMediumTheTalkLineIsAboveTheQuietShelf: the room's talk line closes
// the columns; the QUIET shelf comes under it (the sketch, 0019).
func TestOnMediumTheTalkLineIsAboveTheQuietShelf(t *testing.T) {
	m, _ := hearthOpen(t, 80, headState(t, runningPlan(), true))
	if m.layoutAt(80) != Medium {
		t.Fatalf("80 columns is %v, want Medium", m.layoutAt(80))
	}
	lines := strings.Split(plainFrame(m), "\n")
	at := func(has string) int {
		return slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, has) })
	}
	if talk, quiet := at("› tell hearth's stoker…"), at(" QUIET"); talk < 0 || quiet < 0 || talk > quiet {
		t.Errorf("the talk line (%d) is not above the QUIET shelf (%d):\n%s", talk, quiet, strings.Join(lines, "\n"))
	}
}

// TestAShortRoomKeepsTheWaitingCard: on a short desk the stoker's card folds
// to its active step, then to its first line, before the columns lose the
// chosen card; the talk line stays, and no two hairlines meet.
func TestAShortRoomKeepsTheWaitingCard(t *testing.T) {
	for _, size := range [][2]int{{100, 10}, {100, 12}, {80, 8}} {
		m, _ := hearthOpen(t, size[0], headState(t, runningPlan(), true))
		m = upd(t, m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		frame := plainFrame(m)
		card := "kindle-web"
		if m.layoutAt(size[0]) == Medium && size[1] == 8 {
			card = "NEEDS YOU · 1  ↓ 1" // the stack keeps each section's name first: the head counts the wait
		}
		if !strings.Contains(frame, card) || !strings.Contains(frame, "› tell hearth's stoker…") {
			t.Errorf("%dx%d: the waiting card or the talk line is not drawn:\n%s", size[0], size[1], frame)
		}
		lines := strings.Split(frame, "\n")
		for i := 1; i < len(lines); i++ {
			if strings.HasPrefix(lines[i], "───") && strings.HasPrefix(lines[i-1], "───") {
				t.Errorf("%dx%d: two hairlines meet at row %d:\n%s", size[0], size[1], i, frame)
			}
		}
	}
}
