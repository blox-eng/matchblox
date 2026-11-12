package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
)

// TestTabsAndTheirDigits: two tabs, board and machine, turned by Tab and
// Shift+Tab; no digit opens one (on the board a digit answers its card,
// TestTheTabRowIsBoardAndMachine), and p merges and opens none. A phone's
// Machine names both tabs whole.
func TestTabsAndTheirDigits(t *testing.T) {
	m, _ := loadedWith(t, 100, queueState())
	machine := m
	machine.setTab(tabMachine)
	for _, k := range []string{"1", "2", "3", "7", "p"} {
		if next := pressOn(m, k); next.tab != tabSessions {
			t.Errorf("%s on the board opens tab %d", k, next.tab)
		}
		if next := pressOn(machine, k); next.tab != tabMachine {
			t.Errorf("%s on Machine opens tab %d", k, next.tab)
		}
	}
	if pressOn(m, "tab").tab != tabMachine || pressOn(machine, "tab").tab != tabSessions || pressOn(machine, "shift+tab").tab != tabSessions {
		t.Fatal("Tab and Shift+Tab do not turn between the two tabs")
	}
	phone, _ := loadedWith(t, 44, queueState())
	phone.setTab(tabMachine)
	if line, _ := phone.tabsLine(44); !slices.Equal(strings.Fields(ansi.Strip(line)), []string{"board", "3", "MACHINE"}) {
		t.Errorf("Machine on a phone: %q", ansi.Strip(line))
	}
}

// reasonFixture is three sessions at rest, one of them in pane %3.
func reasonFixture() proto.State {
	st := sortFixture()
	for i := range st.Sessions {
		st.Sessions[i].SessionID = "sid-" + st.Sessions[i].Name
	}
	st.Sessions[2].Pane = "%3"
	st.Reasons = map[string]proto.IdleReason{"%alpha": {Word: "#4556 waits your merge", Tone: "accent"}}
	return st
}

// TestTheStokersSessionStaysOutOfSessions: #81, the stoker's own agent is
// not a lane. It leaves the list and its counts, and the header says when
// it waits for you, so a wait is never hidden.
func TestTheStokersSessionStaysOutOfSessions(t *testing.T) {
	for _, w := range []int{44, 132} {
		st := reasonFixture()
		st.StokerServer = "mbtest"
		st.Sessions[0].Pane, st.Sessions[0].Target = st.Sessions[0].Pane+"@mbtest", "stoker:1.0"
		m, _ := loadedWith(t, w, st)
		if rowOf(m.render(), "alpha") != "" {
			t.Fatalf("width %d: the stoker's session is a row of Sessions", w)
		}
		for _, s := range m.snap.Sessions {
			if s.Server() == "mbtest" {
				t.Fatalf("width %d: the stoker's session is in the list", w)
			}
		}
		st.Sessions[0].Asks = true
		st.Stoker = &proto.StokerView{Mode: proto.StokerMode{On: true}}
		m, _ = loadedWith(t, w, st)
		if h := ansi.Strip(m.head(w)); !strings.Contains(h, "stoker waits") {
			t.Fatalf("width %d: the stoker waits for you and the header does not say so: %q", w, h)
		}
	}
}

func TestAShipWithoutADriverIsNotOutsideTmux(t *testing.T) {
	m, _ := loadedWith(t, 100, fixtureState())
	sh := proto.Ship{Number: 7}
	for _, got := range []string{m.driverName(sh), m.driverGone(sh)} {
		if strings.Contains(got, "outside tmux") {
			t.Fatalf("%q", got)
		}
	}
	// #80: a ship whose session is gone names no session, never a blank.
	if got := m.driverName(sh); got != "no session" {
		t.Fatalf("by %q", got)
	}
	if got := m.driverGone(sh); got != "no session owns #7" {
		t.Fatalf("%q", got)
	}
}
