package app

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

// TestSessionsGroupByNeed: who waits, the oldest wait first; who works (a
// turn, subagents, a shell: tests when it runs a test runner); then the
// quiet ones, the newest first. One that waits since yesterday is quiet.
// Another sort is one flat list.
func TestSessionsGroupByNeed(t *testing.T) {
	st := fixtureState()
	mk := func(pid int, name string, f func(*sample.Session)) sample.Session {
		s := st.Sessions[0]
		s.PID, s.Name, s.Pane, s.Target = pid, name, "%"+name, "w:"+name
		s.Busy, s.Status, s.Idle, s.Do, s.Shells, s.Agents, s.Progress = false, "idle", time.Minute, "", nil, 0, nil
		f(&s)
		return s
	}
	st.Sessions = []sample.Session{
		mk(1, "quiet-new", func(s *sample.Session) { s.Idle = 5 * time.Minute }),
		mk(2, "quiet-old", func(s *sample.Session) { s.Idle = 3 * time.Hour }),
		mk(3, "asks-late", func(*sample.Session) {}),
		mk(4, "asks-early", func(*sample.Session) {}),
		mk(5, "busy", func(s *sample.Session) { s.Busy, s.Status = true, "busy" }),
		mk(6, "agents", func(s *sample.Session) { s.Busy, s.Status, s.Agents = true, sample.StatusAgents, 2 }),
		mk(7, "tests", func(s *sample.Session) { s.Shells = []sample.Shell{{PID: 70, Command: "go test ./..."}} }),
		mk(8, "shell", func(s *sample.Session) { s.Shells = []sample.Shell{{PID: 80, Command: "sleep 100"}} }),
		mk(9, "cold-asks", func(s *sample.Session) { s.Idle = 30 * time.Hour }),
	}
	st.Queue = []queue.Item{
		{Pane: "%asks-late", Name: "asks-late", State: queue.StateQuestion, Since: st.At.Add(-2 * time.Minute)},
		{Pane: "%asks-early", Name: "asks-early", State: queue.StateFinished, Since: st.At.Add(-20 * time.Minute)},
		{Pane: "%cold-asks", Name: "cold-asks", State: queue.StateQuestion, Since: st.At.Add(-30 * time.Hour)},
	}
	m, _ := loadedWith(t, 132, st)
	if got := order(m); got != "asks-early asks-late agents busy shell tests quiet-new quiet-old cold-asks" {
		t.Fatalf("order %q", got)
	}
	labels := []int{lineOf(t, m, "WAITS FOR YOU · 2"), lineOf(t, m, "WORKING · 4"), lineOf(t, m, "QUIET · 3")}
	if labels[0] > labels[1] || labels[1] > labels[2] {
		t.Fatalf("the groups are out of order %v:\n%s", labels, screen(m))
	}
	out := m.render()
	for name, word := range map[string]string{"asks-early": "done", "asks-late": "waits", "agents": "agents", "busy": "busy",
		"tests": "tests", "shell": "shell", "quiet-old": "idle", "cold-asks": "cold"} {
		if l := listOf(rowOf(out, name), 132); !strings.Contains(l, " "+word+" ") {
			t.Errorf("%s: row %q lacks %q", name, l, word)
		}
	}
	if s, _ := m.selectedSession(); s.Name != "asks-early" {
		t.Fatalf("the console opens on %s, not the first that waits", s.Name)
	}
	if h := m.head(132); !strings.Contains(h, "● 2 wait for you") {
		t.Fatalf("header %q", h)
	}
	next, _ := key(m, "s")
	if out := screen(next); strings.Contains(out, "WORKING") || !strings.Contains(out, "9 SESSIONS · by idle ▾") {
		t.Fatalf("another sort keeps the groups:\n%s", out)
	}
}

// TestTabsAndTheirDigits: Sessions, Ships, Machine, Git, History, Panes,
// the digits in that order; p opens Ships, Tab and Shift+Tab cycle. Short
// of room the tabs far from the open one shorten to their digit.
func TestTabsAndTheirDigits(t *testing.T) {
	if want := []string{"sessions", "ships", "machine", "git", "history", "panes"}; !reflect.DeepEqual(tabNames, want) {
		t.Fatalf("tabs %q, want %q", tabNames, want)
	}
	m, _ := loadedWith(t, 100, queueState())
	for i := range tabNames {
		if next, _ := key(m, string(rune('1'+i))); next.(Model).tab != i {
			t.Fatalf("%d opens tab %d", i+1, next.(Model).tab)
		}
	}
	next, _ := key(m, "p")
	if next.(Model).tab != tabShips || !strings.Contains(screen(next), "No ships yet.") {
		t.Fatalf("p does not open Ships:\n%s", screen(next))
	}
	if next, _ := m.key("tab"); next.(Model).tab != tabShips {
		t.Fatal("Tab does not open the next tab")
	}
	if next, _ := m.key("shift+tab"); next.(Model).tab != tabPanes {
		t.Fatal("Shift+Tab does not open the previous tab")
	}
	phone, _ := loadedWith(t, 44, queueState())
	for tab, want := range map[int]string{
		tabSessions: " SESSIONS 3  ships  machine  4  5  6",
		tabHistory:  " 1  2  machine  git  HISTORY  panes",
	} {
		phone.tab = tab
		if line, _ := phone.tabsLine(44); !strings.HasPrefix(stripped(line), want) {
			t.Errorf("tab %d on a phone: %q, want %q", tab, stripped(line), want)
		}
	}
	// A detached busy loop marks the Machine tab.
	st := queueState()
	st.Orphans = []sample.Orphan{{PID: 4242, Comm: "bash"}}
	loops, _ := loadedWith(t, 100, st)
	if line, _ := loops.tabsLine(100); !strings.Contains(stripped(line), "machine !") {
		t.Fatalf("no alert on the Machine tab: %q", stripped(line))
	}
}
