package app

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
)

// loadedHere is loadedWith for a console launched in the repo here.
func loadedHere(t *testing.T, width int, st proto.State, here string) (Model, *fakeConn) {
	t.Helper()
	f := newFake()
	m := New(Options{NoMotion: true, Conn: f, Here: here})
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	next, _ = next.Update(stateMsg(st))
	return next.(Model), f
}

// toGhost moves the selection down to the first ghost row.
func toGhost(t *testing.T, m tea.Model) tea.Model {
	t.Helper()
	for range m.(Model).rowCount() {
		if _, ok := m.(Model).selectedGhost(); ok {
			return m
		}
		m, _ = key(m, "down")
	}
	if _, ok := m.(Model).selectedGhost(); !ok {
		t.Fatalf("no ghost row:\n%s", screen(m))
	}
	return m
}

// emptyState is whereState with no session.
func emptyState(t *testing.T) proto.State {
	st := whereState(t)
	st.Sessions, st.Queue = nil, nil
	return st
}

// With sessions, the ghost row is the last of them, and the cursor still
// starts on the first session.
func TestGhostRowIsBelowTheSessions(t *testing.T) {
	m, _ := loadedWith(t, 100, whereState(t))
	if _, ok := m.selectedSession(); !ok {
		t.Fatalf("the cursor is not on a session:\n%s", screen(m))
	}
	s := screen(m)
	at := strings.Index(s, "+ start a new session here")
	if at < 0 || strings.Count(s, "+ start") != 1 {
		t.Fatalf("one ghost row:\n%s", s)
	}
	for _, ss := range m.snap.Sessions {
		if i := strings.Index(s, ss.Name); i < 0 || i > at {
			t.Fatalf("%s is not above the ghost row:\n%s", ss.Name, s)
		}
	}
	next := toGhost(t, m)
	if g, _ := next.(Model).selectedGhost(); g.repo.path != "/dev/matchblox" {
		t.Fatalf("here is %+v, not the most recent repo", g)
	}
	// One row has the cursor, and the detail is the ghost's.
	if s := screen(next); strings.Count(s, "▌") != 1 || !strings.Contains(s, "start a new session in ~/dev/matchblox") && !strings.Contains(s, "start a new session in /dev/matchblox") {
		t.Fatalf("not one selection, or no ghost detail:\n%s", s)
	}
}

// ⏎ on the ghost row opens N on its folder, the cursor on agent: nothing
// starts without the form.
func TestEnterOnGhostOpensNOnItsFolder(t *testing.T) {
	m, f := loadedWith(t, 100, whereState(t))
	next, cmd := key(toGhost(t, m), "enter")
	if cmd != nil {
		cmd()
	}
	fm := next.(Model).form
	if fm == nil || fm.where != "/dev/matchblox" || !fm.picked || fm.listOpen || fm.at != fieldAgent || next.(Model).pending != nil {
		t.Fatalf("form %+v pending %+v", fm, next.(Model).pending)
	}
	if len(f.acts()) != 0 {
		t.Fatalf("sent %+v", f.acts())
	}
	if fm.autoName() != "matchblox" {
		t.Fatalf("name %q", fm.autoName())
	}
}

// With no session the ghost row comes first and has the cursor, and two
// quick starts follow for the next most recent repos: ⏎ alone opens N.
func TestEmptySessionsLeadsWithTheGhostRows(t *testing.T) {
	m, _ := loadedWith(t, 100, emptyState(t))
	gs := m.ghosts()
	if len(gs) != 3 || gs[0].repo.path != "/dev/matchblox" || gs[1].repo.path != "/src/r10" || gs[2].repo.path != "/src/r48" {
		t.Fatalf("ghosts %+v", gs)
	}
	s := screen(m)
	for _, want := range []string{"+ start a new session here", "+ start in r10", "+ start in r48"} {
		if !strings.Contains(s, want) {
			t.Fatalf("no %q:\n%s", want, s)
		}
	}
	next, _ := key(m, "enter")
	if fm := next.(Model).form; fm == nil || fm.where != "/dev/matchblox" || fm.at != fieldAgent {
		t.Fatalf("form %+v", fm)
	}
	// ↓ goes to a quick start.
	next, _ = key(m, "down")
	next, _ = key(next, "enter")
	if fm := next.(Model).form; fm == nil || fm.where != "/src/r10" {
		t.Fatalf("form %+v", fm)
	}
}

// Launched in a repo, here is that repo, known or not.
func TestGhostHereIsTheLaunchRepo(t *testing.T) {
	m, _ := loadedHere(t, 100, emptyState(t), "/src/r05")
	if gs := m.ghosts(); gs[0].repo.path != "/src/r05" || gs[0].repo.branch != "main" || gs[1].repo.path != "/dev/matchblox" {
		t.Fatalf("ghosts %+v", gs)
	}
	m, _ = loadedHere(t, 100, emptyState(t), "/work/elsewhere")
	if gs := m.ghosts(); gs[0].repo.path != "/work/elsewhere" || gs[0].repo.name != "elsewhere" {
		t.Fatalf("ghosts %+v", gs)
	}
	next, _ := key(m, "enter")
	if fm := next.(Model).form; fm == nil || fm.where != "/work/elsewhere" {
		t.Fatalf("form %+v", fm)
	}
}

// No repo known and none here: the ghost row still opens N, as N does.
func TestGhostWithNoRepoOpensN(t *testing.T) {
	st := emptyState(t)
	st.Found, st.Used, st.Git = nil, nil, nil
	m, _ := loadedWith(t, 100, st)
	if !strings.Contains(screen(m), "+ start a new session") {
		t.Fatalf("no ghost row:\n%s", screen(m))
	}
	next, _ := key(m, "enter")
	if fm := next.(Model).form; fm == nil || fm.at != fieldWhere || !fm.listOpen {
		t.Fatalf("form %+v", fm)
	}
}

// The ghost row is no session: no session key acts on it, and the count
// leaves it out.
func TestNoSessionActionOnTheGhost(t *testing.T) {
	m, f := loadedWith(t, 100, whereState(t))
	n := len(m.snap.Sessions)
	g := toGhost(t, m)
	if _, ok := g.(Model).selectedSession(); ok {
		t.Fatal("the ghost row is a session")
	}
	for _, k := range []string{"x", "r", "A", "a", "z"} {
		next, cmd := key(g, k)
		if cmd != nil {
			runAll(cmd)
		}
		if nm := next.(Model); nm.pending != nil || nm.form != nil || nm.chatKeys || len(f.acts()) != 0 {
			t.Fatalf("%s acted: pending %+v form %+v chatKeys %v sent %+v", k, nm.pending, nm.form, nm.chatKeys, f.acts())
		}
	}
	var words []string
	for _, h := range g.(Model).sessionKeys() {
		words = append(words, h.label)
	}
	if len(words) == 0 || slices.Contains(words, "kill") || slices.Contains(words, "rename") {
		t.Fatalf("keys %q", words)
	}
	if s := screen(g); !strings.Contains(s, "SESSIONS "+strconv.Itoa(n)) {
		t.Fatalf("the count is not %d:\n%s", n, s)
	}
}
