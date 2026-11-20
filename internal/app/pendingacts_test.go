package app

import (
	"strconv"
	"testing"
)

// onlyPending is the one pending act of m.
func onlyPending(t *testing.T, m Model) (string, pendingAct) {
	t.Helper()
	if len(m.pendingActs) != 1 {
		t.Fatalf("pending acts %+v, want one", m.pendingActs)
	}
	for id, p := range m.pendingActs {
		return id, p
	}
	return "", pendingAct{}
}

func TestAConfirmedKillIsPending(t *testing.T) {
	st := boardFixture(t)
	m := pressOn(listShot(t, st, 160, 38, "kindle-web"), "x", "y", "y")
	s, _ := m.sessionAt(m.opt.Host, paneNamed(st, "kindle-web"))
	id, p := onlyPending(t, m)
	if acts := boardFake(m).acts(); len(acts) != 1 || id != "a"+strconv.Itoa(m.acts) {
		t.Fatalf("id %q for acts %+v", id, acts)
	}
	if p.kind != actKill || p.key != paneOf(s) || p.pid != s.PID || p.start != s.Start || p.name != "kindle-web" {
		t.Fatalf("pending %+v, want the kill of %+v", p, s)
	}
}

func TestAParkIsPending(t *testing.T) {
	m := pressOn(listShot(t, boardFixture(t), 160, 38, "kindle-web"), "z")
	if _, p := onlyPending(t, m); p.kind != actPark || !p.park {
		t.Fatalf("pending %+v, want a park", p)
	}
}

func TestARenameIsPending(t *testing.T) {
	m, _ := loadedWith(t, 100, lifeState())
	next, _ := key(m, "r")
	for range len(next.(Model).form.name) {
		next, _ = key(next, "backspace")
	}
	next = typeText(next, "ships")
	next, _ = key(next, "enter")
	if _, p := onlyPending(t, next.(Model)); p.kind != actRename || p.to != "ships" || p.key.pane != "%1" {
		t.Fatalf("pending %+v, want the rename of %%1 to ships", p)
	}
}

func TestAnActWithNoConnectionIsNotPending(t *testing.T) {
	m := listShot(t, boardFixture(t), 160, 38, "kindle-web")
	m.lost = true
	m = pressOn(m, "x", "y", "y")
	if len(m.pendingActs) != 0 {
		t.Fatalf("an act that did not go is pending: %+v", m.pendingActs)
	}
}
