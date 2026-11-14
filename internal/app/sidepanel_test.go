package app

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

func panelText(m Model, w, h int) string {
	return strings.Join(plainLines(m.sidePanel(w, h)), "\n")
}

func TestNothingNeedsYouTakesNoRow(t *testing.T) {
	m := boardModel(t, boardState(boardMatch("hearth-api", time.Hour)))
	if got := panelText(m, 40, 10); strings.Contains(got, "NEEDS YOU") || !strings.Contains(got, "IDLE · 1") {
		t.Fatalf("panel:\n%s", got)
	}
	head, _, _ := m.header(100)
	if got := plainLines([]string{head})[0]; !strings.Contains(got, "✓ nothing needs you") {
		t.Fatalf("header %q", got)
	}
}

func TestPanelDrawsHeadsAndALineBetweenGroups(t *testing.T) {
	a, b := boardMatch("flint-auth", time.Hour), boardMatch("hearth-api", time.Hour)
	st := boardState(a, b)
	st.Queue = append(st.Queue, waitFor(a, queue.StatePermission, time.Minute))
	got := panelText(boardModel(t, st), 40, 12)
	i, j := strings.Index(got, "NEEDS YOU · 1  −"), strings.Index(got, "IDLE · 1  −")
	if i < 0 || j < i || !strings.Contains(got[i:j], "────") {
		t.Fatalf("panel:\n%s", got)
	}
}

func TestFootHidesSetupWhenNoStepIsOpen(t *testing.T) {
	st := boardState(boardMatch("hearth-api", time.Hour))
	m := boardModel(t, st)
	m.recs, m.doors = nil, nil
	lines := plainLines(m.sidePanel(60, 6))
	if got := strings.TrimSpace(lines[len(lines)-1]); got != "+ new" {
		t.Fatalf("foot %q", got)
	}
	m.recs = fixtureState().Recommendations[:1]
	m.doors = []doors.Door{{ID: "tmux", Title: "tmux"}}
	lines = plainLines(m.sidePanel(60, 6))
	if got := strings.TrimSpace(lines[len(lines)-1]); got != "+ new · ✧ 1 tips · ! 1 setup" {
		t.Fatalf("foot %q", got)
	}
}

func TestPanelKeepsTheChosenRowInView(t *testing.T) {
	var ss []sample.Session
	for i := range 25 {
		ss = append(ss, boardMatch(fmt.Sprintf("m%02d", i), time.Hour))
	}
	m := boardModel(t, boardState(ss...))
	order := m.sideOrder()
	m.sideAt = order[len(order)-1]
	last := m.sideGroups()[0].units[len(order)-1].members[0].Name
	lines := m.sidePanel(60, 12)
	if len(lines) != 12 {
		t.Fatalf("%d lines, want 12", len(lines))
	}
	if got := panelText(m, 60, 12); !strings.Contains(got, last) {
		t.Fatalf("chosen %s is out of view:\n%s", last, got)
	}
	if !slices.ContainsFunc(plainLines(lines), func(l string) bool { return strings.Contains(l, "+ new") }) {
		t.Fatal("no foot")
	}
}

func TestStatesWordsShow(t *testing.T) {
	m := boardModel(t, boardState(boardMatch("hearth-api", time.Hour)))
	loading := m
	loading.have = false
	if got := panelText(loading, 50, 5); !strings.Contains(got, "reading your matches…") {
		t.Fatalf("loading:\n%s", got)
	}
	m.filter = "x"
	if got := panelText(m, 60, 5); !strings.Contains(got, `Nothing matches "x". esc clears the search.`) {
		t.Fatalf("search:\n%s", got)
	}
}
