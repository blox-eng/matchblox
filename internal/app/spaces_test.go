package app

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
)

func spaceWait(name, repo, host string) sample.Session {
	s := boardMatch(name, time.Hour)
	s.Cwd, s.Host, s.Permits = "/src/"+repo, host, true
	return s
}

func spaceIdle(name, repo string) sample.Session {
	s := boardMatch(name, time.Hour)
	s.Cwd = "/src/" + repo
	return s
}

func spaceDesk(t *testing.T, w int, ss ...sample.Session) Model {
	t.Helper()
	return boardShot(t, boardState(ss...), w, 34, func(m *Model) { m.deskFrame = true })
}

func spaceHead(m Model) string {
	return strings.Split(ansi.Strip(m.render()), "\n")[0]
}

// headerPane is the header frame after the same snapshot the list holds.
func headerPane(m Model) frameModel {
	w := m.width
	if w <= 0 {
		w = 160
	}
	h := newFrame("header", New(Options{NoMotion: true, Conn: newFake(), Now: m.opt.Now}))
	next, _ := h.Update(tea.WindowSizeMsg{Width: w, Height: 1})
	h = next.(frameModel)
	next, _ = h.Update(stateMsg(proto.State{Snapshot: m.snap}))
	return next.(frameModel)
}

func headerText(h frameModel) string {
	return ansi.Strip(h.View().Content)
}

func headerPress(sub string) string {
	switch {
	case strings.HasPrefix(sub, "+"):
		return spaceMore
	case sub == spaceAll || strings.HasPrefix(sub, spaceAll+" "):
		return spacePref
	}
	name, _, _ := strings.Cut(sub, " ")
	return spacePref + name
}

// tapHeader taps sub on the header pane and applies that press to the list,
// which owns the filter.
func tapHeader(t *testing.T, h frameModel, list Model, sub string) Model {
	t.Helper()
	text := headerText(h)
	x := cellOf(text, sub)
	if x < 0 {
		t.Fatalf("no %q on the header pane: %q", sub, text)
	}
	next, cmd := h.Update(tea.MouseClickMsg{X: x, Y: 0, Button: tea.MouseLeft})
	drain(cmd)
	_ = next
	next, cmd = list.pickSpace(headerPress(sub))
	drain(cmd)
	return next.(Model)
}

// TestSpaceChipsDrawInHeaderPane: the header pane, not the list, draws
// the space chips and the wait count.
func TestSpaceChipsDrawInHeaderPane(t *testing.T) {
	m := spaceDesk(t, 160, spaceWait("ledge", "ledge", ""), spaceWait("matchblox", "matchblox", ""), spaceWait("base", "base", ""))
	got := headerText(headerPane(m))
	if !strings.Contains(got, "ledge ●1") || !strings.Contains(got, "● 3 need you") {
		t.Fatalf("header pane %q, want ledge ●1 and ● 3 need you", got)
	}
	if head := spaceHead(m); strings.Contains(head, "ledge ●1") || strings.Contains(head, "all ·") {
		t.Fatalf("list stole the header line: %q", head)
	}
}

// TestSpaceFilterKeepsNeedYouCount: three repos, waits on ledge, matchblox
// and base; a tap on ledge ●1 names only ledge in the list, and the header
// still counts every wait.
func TestSpaceFilterKeepsNeedYouCount(t *testing.T) {
	m := spaceDesk(t, 160, spaceWait("ledge", "ledge", ""), spaceWait("matchblox", "matchblox", ""), spaceWait("base", "base", ""))
	h := headerPane(m)
	head := headerText(h)
	if !strings.Contains(head, "ledge ●1") || !strings.Contains(head, "● 3 need you") {
		t.Fatalf("header pane %q, want ledge ●1 and ● 3 need you", head)
	}
	m = tapHeader(t, h, m, "ledge ●1")
	if m.space != "ledge" {
		t.Fatalf("space %q, want ledge", m.space)
	}
	if got := drawnNames(m); !slices.Equal(got, []string{"ledge"}) {
		t.Fatalf("list names %v, want only ledge", got)
	}
	if head := headerText(headerPane(m)); !strings.Contains(head, "● 3 need you") {
		t.Fatalf("a space filter hid a wait: %q", head)
	}
}

// TestSpaceCountIgnoresDone: a session that does not need you (idle, no
// ask) in matchblox does not add to matchblox ●N. Task 3 has not added
// groupDone yet.
func TestSpaceCountIgnoresDone(t *testing.T) {
	m := spaceDesk(t, 160, spaceWait("asks", "matchblox", ""), spaceIdle("quiet", "matchblox"), spaceWait("ledge", "ledge", ""))
	head := headerText(headerPane(m))
	if !strings.Contains(head, "matchblox ●1") {
		t.Fatalf("header pane %q, want matchblox ●1 (the idle session is not a wait)", head)
	}
	if strings.Contains(head, "matchblox ●2") {
		t.Fatalf("an idle session counted on matchblox: %q", head)
	}
}

// TestSpacesOverflowOpensMenu: more spaces than fit; +3 ▾ then a tap
// opens the rest over the list, the header stays one row, and choosing
// one filters.
func TestSpacesOverflowOpensMenu(t *testing.T) {
	var ss []sample.Session
	for _, repo := range []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot"} {
		ss = append(ss, spaceWait(repo, repo, ""))
	}
	var m Model
	var head string
	for w := 40; w <= 120; w++ {
		m = spaceDesk(t, w, ss...)
		head = headerText(headerPane(m))
		if strings.Contains(head, "+3 ▾") {
			break
		}
	}
	if !strings.Contains(head, "+3 ▾") {
		t.Fatalf("no width hid three spaces; last header pane %q", head)
	}
	if strings.Count(head, "\n") != 0 {
		t.Fatalf("header is not one row: %q", head)
	}
	h := headerPane(m)
	m = tapHeader(t, h, m, "+3 ▾")
	if m.spaceMenu == nil {
		t.Fatal("the overflow menu did not open over the list")
	}
	after := strings.Split(ansi.Strip(m.render()), "\n")
	if head := headerText(headerPane(m)); !strings.Contains(head, "+3 ▾") {
		t.Fatalf("the header grew or lost the chip:\n%s", head)
	}
	rest := strings.Join(after[1:], "\n")
	hidden := m.spaceMenu.names[m.spaceMenu.at]
	if !strings.Contains(rest, hidden) {
		t.Fatalf("the rest did not open over the list:\n%s", strings.Join(after, "\n"))
	}
	m = pressOn(m, "enter")
	if m.space != hidden || m.spaceMenu != nil {
		t.Fatalf("space %q menu %v, want %s and the menu closed", m.space, m.spaceMenu, hidden)
	}
	if got := drawnNames(m); !slices.Equal(got, []string{hidden}) {
		t.Fatalf("list names %v, want only %s", got, hidden)
	}
}

func TestSpaceEmptyShowsAllCopy(t *testing.T) {
	m := spaceDesk(t, 160, spaceWait("base", "base", ""), spaceWait("ledge", "ledge", "gpu-1"))
	m.space, m.boardHost = "base", "gpu-1"
	got := ansi.Strip(m.render())
	if want := "Nothing in base on gpu-1. esc shows all."; !strings.Contains(got, want) {
		t.Fatalf("empty copy:\n%s\nwant %q", got, want)
	}
}

func TestSpaceLivesOnModelAndWindowOption(t *testing.T) {
	var calls [][]string
	m := boardShot(t, boardState(spaceWait("ledge", "ledge", ""), spaceWait("base", "base", "")), 160, 34, func(m *Model) {
		m.deskFrame = true
		m.opt.Pane = "%1"
		m.opt.Tmux = func(args ...string) (string, error) {
			calls = append(calls, args)
			return "\n", nil
		}
	})
	m = tapHeader(t, headerPane(m), m, "ledge ●1")
	if m.space != "ledge" {
		t.Fatalf("space %q, want ledge on the list Model", m.space)
	}
	if hasField(m.snap, "Space") || hasField(proto.State{}, "Space") {
		t.Fatal("space must not be a snapshot field")
	}
	want := []string{"set-option", "-w", "-t", "%1", sample.SpaceOption, "ledge"}
	if !slices.ContainsFunc(calls, func(c []string) bool { return slices.Equal(c, want) }) {
		t.Fatalf("tmux calls %v, want %v", calls, want)
	}
}

func TestSpaceBracketsCycleFromList(t *testing.T) {
	m := spaceDesk(t, 160, spaceWait("ledge", "ledge", ""), spaceWait("matchblox", "matchblox", ""), spaceWait("base", "base", ""))
	m = pressOn(m, "]")
	if m.space != "base" {
		t.Fatalf("] from all goes to %q, want base", m.space)
	}
	m = pressOn(m, "]")
	if m.space != "ledge" {
		t.Fatalf("] goes to %q, want ledge", m.space)
	}
	m = pressOn(m, "[")
	if m.space != "base" {
		t.Fatalf("[ goes to %q, want base", m.space)
	}
}

func TestSpaceOneRepoHasNoChips(t *testing.T) {
	m := spaceDesk(t, 160, spaceWait("ledge", "ledge", ""))
	head := headerText(headerPane(m))
	if strings.Contains(head, "all ·") || strings.Contains(head, "ledge ●") {
		t.Fatalf("one repo still draws space chips: %q", head)
	}
}

func TestSpaceCombinesWithHostFilter(t *testing.T) {
	m := spaceDesk(t, 160, spaceWait("base", "base", ""), spaceWait("ledge", "ledge", "gpu-1"))
	m.space, m.boardHost = "ledge", "gpu-1"
	if got := drawnNames(m); !slices.Equal(got, []string{"ledge"}) {
		t.Fatalf("space and host together: %v, want only ledge", got)
	}
	m.space, m.boardHost = "base", "gpu-1"
	if got := drawnNames(m); len(got) != 0 {
		t.Fatalf("base on gpu-1 still lists %v", got)
	}
}

func hasField(v any, name string) bool {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	_, ok := t.FieldByName(name)
	return ok
}

func TestSpaceAllHasNoCount(t *testing.T) {
	m := spaceDesk(t, 160, spaceWait("ledge", "ledge", ""), spaceWait("base", "base", ""))
	head := headerText(headerPane(m))
	if !strings.Contains(head, "all ·") {
		t.Fatalf("header pane %q, want all with no count", head)
	}
	if strings.Contains(head, "all ●") {
		t.Fatalf("all has a count: %q", head)
	}
}
