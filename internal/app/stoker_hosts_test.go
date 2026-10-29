package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
)

// stokedHosts is a console on this machine with the peers ws-1 and gpu-1;
// each host's state has one session and the stoker view given, nil for
// none.
func stokedHosts(t *testing.T, home, ws, gpu *proto.StokerView) (Model, *fakeConn, map[string]*fakeConn) {
	t.Helper()
	now := lostAt
	m, homeConn, conns := hostsAt(t, &now, "", 132, 30, "ws-1", "gpu-1")
	state := func(pid int, name, pane string, v *proto.StokerView) proto.State {
		st := stateAt(hostState(sess(pid, name, pane, nil)), now)
		st.Stoker = v
		return st
	}
	m = upd(t, m, stateMsg(state(1, "home-1", "%1", home)))
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(state(2, "ws-run", "%2", ws))})
	m = upd(t, m, peerMsg{host: "gpu-1", conn: conns["gpu-1"], msg: stateMsg(state(3, "gpu-run", "%3", gpu))})
	return m, homeConn, conns
}

func nightView() *proto.StokerView {
	return &proto.StokerView{Mode: proto.StokerMode{On: true, Until: lostAt.Add(8 * time.Hour)}}
}

// selectRow selects the row of the session name, from the top.
func selectRow(t *testing.T, m Model, name string) Model {
	t.Helper()
	m.pending = nil
	for range m.rowCount() {
		m = upd(t, m, keyPress("up"))
	}
	for range m.rowCount() {
		if s, ok := m.selectedSession(); ok && s.Name == name {
			return m
		}
		m = upd(t, m, keyPress("down"))
	}
	t.Fatalf("no row %s", name)
	return m
}

func stokerActs(c *fakeConn) []string {
	var out []string
	for _, a := range c.acts() {
		if strings.HasPrefix(a.RecID, "stoker:") {
			out = append(out, a.RecID)
		}
	}
	return out
}

// TestStokerRestIsThatHostsOnly: the stoker runs the night on ws-1 and
// gpu-1. A key on a ws-1 row asks to take over ws-1's stoker, and y rests
// that one only.
func TestStokerRestIsThatHostsOnly(t *testing.T) {
	m, home, conns := stokedHosts(t, nil, nightView(), nightView())
	// A key on this machine's row acts at once: its host runs no stoker.
	m = selectRow(t, m, "home-1")
	if f := m.focus(keyPress("enter")); f == layerTakeOver {
		t.Fatal("a row of a host with no stoker asks to take over")
	}
	m = selectRow(t, m, "ws-run")
	if f := m.focus(keyPress("enter")); f != layerTakeOver {
		t.Fatalf("a key on a ws-1 row: layer %v, want the take-over question", f)
	}
	next := pressAll(m, "enter", "y")
	nm := next.(Model)
	if got := stokerActs(conns["ws-1"]); !slices.Equal(got, []string{"stoker:rest"}) {
		t.Fatalf("ws-1 got %v", got)
	}
	if got, other := stokerActs(conns["gpu-1"]), stokerActs(home); len(got) != 0 || len(other) != 0 {
		t.Fatalf("gpu-1 got %v, home got %v", got, other)
	}
	if !nm.stokerOf("ws-1").Asked || nm.stokerOf("ws-1").Rests.IsZero() {
		t.Fatalf("ws-1's stoker does not rest: %+v", nm.stokerOf("ws-1").Mode)
	}
	if v := nm.stokerOf("gpu-1"); v.Asked || !v.Rests.IsZero() {
		t.Fatalf("gpu-1's stoker changed: %+v", v.Mode)
	}
	// gpu-1's stoker still asks for its own rows.
	nm = selectRow(t, nm, "gpu-run")
	if f := nm.focus(keyPress("enter")); f != layerTakeOver {
		t.Fatalf("a key on a gpu-1 row: layer %v", f)
	}
}

// TestStokerTakeOverRefusedByItsHost: the home host is lost; the question
// of a healthy peer's stoker still goes through.
func TestStokerTakeOverRefusedByItsHost(t *testing.T) {
	m, _, conns := stokedHosts(t, nil, nightView(), nil)
	m.lost = true
	m = selectRow(t, m, "ws-run")
	nm := pressAll(m, "enter", "y").(Model)
	if got := stokerActs(conns["ws-1"]); !slices.Equal(got, []string{"stoker:rest"}) {
		t.Fatalf("ws-1 got %v, flash %q", got, nm.flash)
	}
}

// TestStokerTouchGoesToTheHostActedOn: a key while ws-1's stoker rests
// touches ws-1's, not another host's.
func TestStokerTouchGoesToTheHostActedOn(t *testing.T) {
	rests := nightView()
	rests.Rests = lostAt
	m, home, conns := stokedHosts(t, nightView(), rests, nil)
	m = selectRow(t, m, "ws-run")
	pressAll(m, "down", "up")
	if got := stokerActs(conns["ws-1"]); !slices.Equal(got, []string{"stoker:touch"}) {
		t.Fatalf("ws-1 got %v", got)
	}
	if got := stokerActs(home); len(got) != 0 {
		t.Fatalf("home got %v", got)
	}
}

// TestStokerBadgeCounts: the header counts the hosts where the stoker
// runs.
func TestStokerBadgeCounts(t *testing.T) {
	m, _, _ := stokedHosts(t, nightView(), &proto.StokerView{}, nightView())
	if head := frameOf(m)[0]; !strings.Contains(head, "stoker 2/3") {
		t.Fatalf("header %q", head)
	}
}

// TestStokerTabSectionPerHost: the STOKER tab has one section for each
// host with a stoker view, headed by its name, in Hosts order.
func TestStokerTabSectionPerHost(t *testing.T) {
	m, _, _ := stokedHosts(t, nil, &proto.StokerView{}, nightView())
	m.setTab(tabStoker)
	var heads []string
	for _, l := range frameOf(m) {
		switch w := strings.TrimSpace(ansi.Strip(l)); w {
		case "this machine", "ws-1", "gpu-1":
			heads = append(heads, w)
		}
	}
	if !slices.Equal(heads, []string{"ws-1", "gpu-1"}) {
		t.Fatalf("sections %q\n%s", heads, strings.Join(frameOf(m), "\n"))
	}
}
