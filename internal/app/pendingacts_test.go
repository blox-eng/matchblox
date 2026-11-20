package app

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
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

// drawnRow is the drawn list line of the session named name, and the line
// after it.
func drawnRow(t *testing.T, m Model, name string) string {
	t.Helper()
	lines, _, _ := m.sideList(80, 60)
	for i, l := range lines {
		if strings.Contains(ansi.Strip(l), name) {
			if i+1 < len(lines) {
				return ansi.Strip(l + "\n" + lines[i+1])
			}
			return ansi.Strip(l)
		}
	}
	t.Fatalf("no row of %s in:\n%s", name, ansi.Strip(strings.Join(lines, "\n")))
	return ""
}

func TestAKilledRowSaysKillingAtOnce(t *testing.T) {
	st := boardFixture(t)
	m := pressOn(listShot(t, st, 160, 38, "kindle-web"), "x", "y", "y")
	if row := drawnRow(t, m, "kindle-web"); !strings.Contains(row, "killing") {
		t.Fatalf("the killed row reads %q, want killing", row)
	}
	if got := m.chosenSide(); got == (paneKey{m.opt.Host, paneNamed(st, "kindle-web")}) {
		t.Fatal("the cursor stays on the killed row")
	}
	if slices.Contains(m.sideOrder(), paneKey{m.opt.Host, paneNamed(st, "kindle-web")}) {
		t.Fatal("the walk still holds the killed row")
	}
}

// homeSample is the next sample of boardShot's home host: st's sessions
// off gpu-1, each changed by edit; a session edit drops is gone.
func homeSample(st proto.State, edit func(*sample.Session) bool) proto.State {
	home := st
	home.Sessions, home.Queue = nil, nil
	home.Ships = &proto.Ships{At: demoAt}
	for _, s := range st.Sessions {
		if s.Host == "gpu-1" || edit != nil && !edit(&s) {
			continue
		}
		home.Sessions = append(home.Sessions, s)
	}
	for _, it := range st.Queue {
		if it.Host != "gpu-1" {
			home.Queue = append(home.Queue, it)
		}
	}
	return home
}

// sampled is m after n samples of home.
func sampled(m Model, home proto.State, n int) Model {
	for range n {
		next, _ := m.Update(stateMsg(home))
		m = next.(Model)
	}
	return m
}

func without(name string) func(*sample.Session) bool {
	return func(s *sample.Session) bool { return s.Name != name }
}

func TestAKillGoesWithTheSampleThatLacksIt(t *testing.T) {
	st := boardFixture(t)
	m := pressOn(listShot(t, st, 160, 38, "kindle-web"), "x", "y", "y")
	m = sampled(m, homeSample(st, without("kindle-web")), 1)
	if len(m.pendingActs) != 0 {
		t.Fatalf("the kill is still pending: %+v", m.pendingActs)
	}
	if lines, _, _ := m.sideList(80, 60); strings.Contains(ansi.Strip(strings.Join(lines, "\n")), "kindle-web") {
		t.Fatal("the killed row is still drawn")
	}
}

func TestAKillThatDidNotTakeComesBack(t *testing.T) {
	st := boardFixture(t)
	m := pressOn(listShot(t, st, 160, 38, "kindle-web"), "x", "y", "y")
	m = sampled(m, homeSample(st, nil), 2)
	if len(m.pendingActs) != 1 {
		t.Fatalf("two samples ended the kill: %+v", m.pendingActs)
	}
	m = sampled(m, homeSample(st, nil), 1)
	if row := drawnRow(t, m, "kindle-web"); strings.Contains(row, "killing") || len(m.pendingActs) != 0 {
		t.Fatalf("after three samples the row reads %q, pending %+v", row, m.pendingActs)
	}
	if want := "kindle-web is still running after the kill. Press x to kill it again."; !strings.Contains(m.flash, want) {
		t.Fatalf("said %q, want %q", m.flash, want)
	}
}

func TestAReusedPidIsNotKilling(t *testing.T) {
	st := boardFixture(t)
	m := pressOn(listShot(t, st, 160, 38, "kindle-web"), "x", "y", "y")
	m = sampled(m, homeSample(st, func(s *sample.Session) bool {
		if s.Name == "kindle-web" {
			s.Start++
		}
		return true
	}), 1)
	if row := drawnRow(t, m, "kindle-web"); strings.Contains(row, "killing") || len(m.pendingActs) != 0 {
		t.Fatalf("a new process in the pane reads %q, pending %+v", row, m.pendingActs)
	}
}

// resulted is m after the service's answer to the one pending act.
func resulted(t *testing.T, m Model, r proto.Result) Model {
	t.Helper()
	r.ActID, _ = onlyPending(t, m)
	next, _ := m.Update(resultMsg(r))
	return next.(Model)
}

func TestAKillTheServiceRefusesComesBack(t *testing.T) {
	m := pressOn(listShot(t, boardFixture(t), 160, 38, "kindle-web"), "x", "y", "y")
	m = resulted(t, m, proto.Result{Err: "no such pane"})
	if row := drawnRow(t, m, "kindle-web"); strings.Contains(row, "killing") || len(m.pendingActs) != 0 {
		t.Fatalf("a refused kill reads %q, pending %+v", row, m.pendingActs)
	}
	if want := "kill failed: no such pane"; !strings.Contains(m.flash, want) {
		t.Fatalf("said %q, want %q", m.flash, want)
	}
}

func TestAParkTheServiceRefusesComesBack(t *testing.T) {
	m := pressOn(listShot(t, boardFixture(t), 160, 38, "kindle-web"), "z")
	m = resulted(t, m, proto.Result{Err: "parking is off"})
	if len(m.pendingActs) != 0 || !strings.Contains(m.flash, "park failed: parking is off") {
		t.Fatalf("pending %+v, said %q", m.pendingActs, m.flash)
	}
}

func TestARenameTheServiceRefusesComesBack(t *testing.T) {
	m, _ := loadedWith(t, 100, lifeState())
	next, _ := key(m, "r")
	next = typeText(next, "x")
	next, _ = key(next, "enter")
	m = resulted(t, next.(Model), proto.Result{Err: "tmux: no such pane"})
	if len(m.pendingActs) != 0 || m.form == nil || m.form.err != "tmux: no such pane" {
		t.Fatalf("pending %+v, form %+v", m.pendingActs, m.form)
	}
}

func TestARenameThatWaitsForARestIsNotPending(t *testing.T) {
	m, _ := loadedWith(t, 100, lifeState())
	next, _ := key(m, "r")
	next = typeText(next, "x")
	next, _ = key(next, "enter")
	m = resulted(t, next.(Model), proto.Result{Skipped: []string{"alpha renames when it rests"}})
	if len(m.pendingActs) != 0 {
		t.Fatalf("a rename the service holds back is pending: %+v", m.pendingActs)
	}
}
