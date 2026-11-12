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
	if mm := m.(Model); mm.have && len(mm.snap.Sessions) > 0 {
		mm.boardBelow = 1 // the board's ghost row, under the columns
		m = mm.onChoice()
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

// ⏎ on the ghost row opens N on its folder, the cursor on agent: nothing
// starts without the form.
func TestEnterOnGhostOpensNOnItsFolder(t *testing.T) {
	m, f := loadedWith(t, 100, whereState(t))
	next, cmd := key(toGhost(t, m), "enter")
	if cmd != nil {
		drain(cmd)
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
		if nm := next.(Model); nm.pending != nil || nm.form != nil || nm.standIn || len(f.acts()) != 0 {
			t.Fatalf("%s acted: pending %+v form %+v stand-in %v sent %+v", k, nm.pending, nm.form, nm.standIn, f.acts())
		}
	}
	var words []string
	for _, h := range g.(Model).sessionKeys() {
		words = append(words, h.label)
	}
	if len(words) == 0 || slices.Contains(words, "kill") || slices.Contains(words, "rename") {
		t.Fatalf("keys %q", words)
	}
	if s := screen(g); !strings.Contains(s, "BOARD "+strconv.Itoa(n)) {
		t.Fatalf("the count is not %d:\n%s", n, s)
	}
}
