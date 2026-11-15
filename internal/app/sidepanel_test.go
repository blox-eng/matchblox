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
	if got := strings.TrimSpace(lines[len(lines)-1]); got != "+ new · ✧ 1 tip · ! 1 setup" {
		t.Fatalf("foot %q", got)
	}
	m.recs = fixtureState().Recommendations[:2]
	lines = plainLines(m.sidePanel(60, 6))
	if got := strings.TrimSpace(lines[len(lines)-1]); got != "+ new · ✧ 2 tips · ! 1 setup" {
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

func TestHeadCountsWhatTheSearchShows(t *testing.T) {
	var ss []sample.Session
	for i := range 8 {
		ss = append(ss, boardMatch(fmt.Sprintf("m%02d", i), time.Hour))
	}
	ss[3].Name = "kindle-web"
	m := boardModel(t, boardState(ss...))
	m.filter = "kindle"
	if got := panelText(m, 50, 10); !strings.Contains(got, "IDLE · 1") {
		t.Fatalf("panel:\n%s", got)
	}
}

func TestPanelOfNoWidthDrawsNothing(t *testing.T) {
	m := boardModel(t, boardState(boardMatch("hearth-api", time.Hour)))
	for _, w := range []int{0, -3} {
		if got := m.sidePanel(w, 8); got != nil {
			t.Fatalf("w=%d drew %q", w, got)
		}
	}
}

func TestNoChoiceMeansTheFirstInOrder(t *testing.T) {
	a, b := boardMatch("flint-auth", time.Hour), boardMatch("hearth-api", time.Hour)
	st := boardState(a, b)
	st.Queue = append(st.Queue, waitFor(b, queue.StatePermission, time.Minute))
	m := boardModel(t, st)
	m.sideAt = paneKey{}
	first := m.sideOrder()[0]
	if first != paneOf(b) {
		t.Fatalf("fixture: sideAt %v, first %v", m.sideAt, first)
	}
	for _, l := range plainLines(m.sidePanel(50, 12)) {
		if strings.HasPrefix(l, "▌") {
			if !strings.Contains(l, "hearth-api") {
				t.Fatalf("chosen row %q, want hearth-api", l)
			}
			return
		}
	}
	t.Fatal("no row is chosen")
}

func TestChainLinksLightWhenAMemberIsChosen(t *testing.T) {
	m := loginChain(t, nil)
	other := boardMatch("spark-docs", time.Hour)
	m.all.sessions = append(m.all.sessions, other)
	lit := func(at paneKey) string {
		m.sideAt = at
		return strings.Join(m.sidePanel(60, 20), "\n")
	}
	var flint sample.Session
	for _, s := range m.all.sessions {
		if s.Name == "flint-auth" {
			flint = s
		}
	}
	on, off := lit(paneOf(flint)), lit(paneOf(other))
	head := func(out string) string {
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, "≋ login") {
				return l
			}
		}
		return ""
	}
	if head(on) == "" || head(on) == head(off) {
		t.Fatalf("head not lit by a member:\n%q\n%q", head(on), head(off))
	}
	if !strings.Contains(on, m.st.accent.Render("├")) || strings.Contains(off, m.st.accent.Render("├")) {
		t.Fatalf("links: on has accent %v, off has accent %v", strings.Contains(on, m.st.accent.Render("├")), strings.Contains(off, m.st.accent.Render("├")))
	}
}
