package app

import (
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

// waiting is a host's state with one session that waits for a yes.
func waiting(pid int, name, pane string, at time.Time) proto.State {
	st := hostState(sess(pid, name, pane, nil))
	st.Queue = []queue.Item{waitFor(st.Sessions[0], queue.StatePermission, time.Minute)}
	return stateAt(st, at)
}

// hostsConsole is the list and the chat, the keys in the list, on ws-1
// (home) with hearth-api and ws-2 with flint-auth. Each session waits for
// a yes. hidden are the hosts the owner hid.
func hostsConsole(t *testing.T, hidden ...string) (Model, *time.Time, map[string]*fakeConn) {
	t.Helper()
	now := boardAt
	m, _, conns := hostsAt(t, &now, "", 160, 40, "ws-2")
	m = upd(t, m, helloMsg(proto.Hello{Version: proto.Version, Host: "ws-1"}))
	m = upd(t, m, stateMsg(waiting(1, "hearth-api", "%1", now)))
	m = upd(t, m, peerMsg{host: "ws-2", conn: conns["ws-2"], msg: stateMsg(waiting(2, "flint-auth", "%2", now))})
	m = boardOn(m)
	m.tab, m.sideOn, m.keysIn, m.hiddenHosts = tabSessions, true, focusList, hidden
	return m, &now, conns
}

func headerOf(m Model) string {
	head, _, _ := m.header(m.width)
	return plainLines([]string{head})[0]
}

func TestAllHostsLeavesOutHidden(t *testing.T) {
	both, _, _ := hostsConsole(t)
	if got := headerOf(both); !strings.Contains(got, "● 2 need you") {
		t.Fatalf("nothing hidden: header %q, want both waits", got)
	}
	m, _, _ := hostsConsole(t, "ws-2")
	panel := panelText(m, 60, 20)
	if !strings.Contains(panel, "hearth-api") || strings.Contains(panel, "flint-auth") || !strings.Contains(panel, "NEEDS YOU · 1") {
		t.Fatalf("all hosts with ws-2 hidden:\n%s", panel)
	}
	got := headerOf(m)
	if !strings.Contains(got, "● 1 need you") || strings.Contains(got, "ws-2") || !strings.Contains(got, "1 hidden") {
		t.Fatalf("header %q: want ws-1's wait only, and ws-2 left out", got)
	}
	if order := m.sideOrder(); len(order) != 1 || order[0].pane != "%1" {
		t.Fatalf("↑↓ walk %+v, want hearth-api only", order)
	}
}

func TestSReachesAHiddenHost(t *testing.T) {
	m, _, _ := hostsConsole(t, "ws-2")
	m = pressOn(m, "s")
	if m.boardHost != "ws-2" {
		t.Fatalf("s turned to %q, want the hidden ws-2", m.boardHost)
	}
	if got := headerOf(m); !strings.Contains(got, "ws-2 · hidden from all hosts · h shows it") {
		t.Fatalf("header %q", got)
	}
	if panel := panelText(m, 60, 20); !strings.Contains(panel, "flint-auth") || strings.Contains(panel, "hearth-api") {
		t.Fatalf("the list on ws-2:\n%s", panel)
	}
}

func TestHUnhidesAndAllHostsShowsIt(t *testing.T) {
	r := hidingShell(t, []string{"ws-2"}, []string{"ws-2"}, "")
	if !slices.Equal(peerHosts(r.sh.cur), []string{"ws-2"}) {
		t.Fatalf("peers %q: a hidden host has a link too", peerHosts(r.sh.cur))
	}
	r.update(stateMsg(waiting(1, "hearth-api", "%1", time.Now())))
	r.update(peerMsg{host: "ws-2", conn: r.conns["ws-2"], msg: stateMsg(waiting(2, "flint-auth", "%2", time.Now()))})
	r.sh.cur = boardOn(r.sh.cur)
	r.sh.cur.tab, r.sh.cur.sideOn, r.sh.cur.keysIn = tabSessions, true, focusList
	if strings.Contains(panelText(r.sh.cur, 60, 20), "flint-auth") {
		t.Fatalf("all hosts shows the hidden ws-2:\n%s", panelText(r.sh.cur, 60, 20))
	}
	r.key(t, "h")
	if r.sh.cur.noticeText != "h shows a hidden host · s goes to it" || len(r.hidden) != 1 {
		t.Fatalf("h on all hosts: notice %q, hidden %q", r.sh.cur.noticeText, r.hidden)
	}
	r.key(t, "s") // ws-2: this machine alone is all hosts
	r.key(t, "h")
	if len(r.hidden) != 0 {
		t.Fatalf("h on ws-2 did not call Show: hidden %q", r.hidden)
	}
	if got := headerOf(r.sh.cur); strings.Contains(got, "hidden") || !strings.Contains(r.sh.cur.noticeText, "ws-2 shows in all hosts") {
		t.Fatalf("after h: header %q, notice %q", got, r.sh.cur.noticeText)
	}
	r.sh.cur.boardHost = ""
	if panel := panelText(r.sh.cur, 60, 20); !strings.Contains(panel, "flint-auth") || !strings.Contains(panel, "hearth-api") {
		t.Fatalf("all hosts after h:\n%s", panel)
	}
	r.toHosts(t)
	if row := r.rowOf("ws-2"); !strings.Contains(row, "● ws-2") {
		t.Fatalf("Machine › Hosts after h: %q, want ws-2 ticked", row)
	}
}

func TestOneUnhiddenHostSStepsHostToHost(t *testing.T) {
	steps := func(m Model) []string {
		var out []string
		for range 4 {
			m = pressOn(m, "s")
			out = append(out, m.boardHost)
		}
		return out
	}
	m, _, _ := hostsConsole(t, "ws-2")
	if got := headerOf(m); !strings.Contains(got, "ws-1 ▾") {
		t.Fatalf("header %q: all hosts is ws-1 alone, by its name", got)
	}
	if got := steps(m); !slices.Equal(got, []string{"ws-2", "", "ws-2", ""}) {
		t.Fatalf("s with ws-2 hidden steps %q, want no second stop on ws-1", got)
	}
	both, _, _ := hostsConsole(t)
	if got := steps(both); !slices.Equal(got, []string{"ws-1", "ws-2", "", "ws-1"}) {
		t.Fatalf("s with nothing hidden steps %q", got)
	}
}

func TestSIsNeverSilent(t *testing.T) {
	m, _ := sideConsole(t, hostState(sess(1, "hearth-api", "%1", nil)))
	m = pressOn(m, "esc", "s")
	if m.noticeText != "this machine is the only host · Machine › Hosts adds one" {
		t.Fatalf("s with one host: notice %q", m.noticeText)
	}
	if m = pressOn(m, "h"); m.noticeText != "no host is hidden" {
		t.Fatalf("h with nothing hidden: notice %q", m.noticeText)
	}
}

func TestUnreachedHostDimsItsRows(t *testing.T) {
	m, now, conns := hostsConsole(t)
	m = upd(t, m, peerMsg{host: "ws-2", conn: conns["ws-2"], msg: lostMsg{err: io.EOF, conn: conns["ws-2"]}})
	*now = now.Add(2 * time.Minute)
	m = pressOn(m, "s", "s") // ws-2
	if got := headerOf(m); !strings.Contains(got, "ws-2 not reached · since 2m") {
		t.Fatalf("header %q", got)
	}
	i := slices.IndexFunc(m.all.sessions, func(s sample.Session) bool { return s.Name == "flint-auth" })
	row := m.sideRow(m.all.sessions[i], 60, false, sideLink{})[0]
	if !strings.Contains(row, m.st.faint.Render("flint-auth")) {
		t.Fatalf("ws-2's row is not dimmed: %q", row)
	}
	if panel := panelText(m, 60, 20); !strings.Contains(panel, "flint-auth") {
		t.Fatalf("ws-2's rows went:\n%s", panel)
	}
}

// lostHidden is hostsConsole with ws-2 hidden, its stoker on, lost 2m ago.
func lostHidden(t *testing.T) Model {
	t.Helper()
	m, now, conns := hostsConsole(t, "ws-2")
	st := waiting(2, "flint-auth", "%2", *now)
	st.Stoker = &proto.StokerView{On: true}
	m = upd(t, m, peerMsg{host: "ws-2", conn: conns["ws-2"], msg: stateMsg(st)})
	m = upd(t, m, peerMsg{host: "ws-2", conn: conns["ws-2"], msg: lostMsg{err: io.EOF, conn: conns["ws-2"]}})
	*now = now.Add(2 * time.Minute)
	return m
}

func TestHiddenLostHostIsNamedNowhereInAllHosts(t *testing.T) {
	for _, side := range []bool{false, true} {
		m := lostHidden(t)
		m.sideOn = side
		if f := strings.Join(frameOf(m), "\n"); strings.Contains(f, "ws-2") || strings.Contains(f, "stoker") {
			t.Errorf("side %v: all hosts names the hidden ws-2 or its stoker:\n%s", side, f)
		}
		m.peers = slices.Clone(m.peers)
		m.peers[0].blocked = errors.New("permission denied")
		if f := strings.Join(frameOf(m), "\n"); strings.Contains(f, "ws-2") {
			t.Errorf("side %v: all hosts has a blocked row of the hidden ws-2:\n%s", side, f)
		}
		if bs := m.blockedHosts(); len(bs) != 0 {
			t.Errorf("side %v: blocked rows %+v", side, bs)
		}
	}
}

func TestHiddenHostShowsItsTroubleWhenShown(t *testing.T) {
	m := lostHidden(t)
	m.sideOn = false
	m = pressOn(m, "s")
	f := strings.Join(frameOf(m), "\n")
	if m.boardHost != "ws-2" || !strings.Contains(f, "ws-2: connection lost.") || !strings.Contains(f, "ws-2: not reached since") {
		t.Fatalf("the board on ws-2 hides its trouble:\n%s", f)
	}
	if !strings.Contains(m.stokerBadge(false), "stoker 1/2") {
		t.Errorf("the stoker badge on ws-2: %q", m.stokerBadge(false))
	}
	m.sideOn = true
	if got := headerOf(m); !strings.Contains(got, "ws-2 · hidden from all hosts · h shows it") {
		t.Errorf("side header on ws-2: %q", got)
	}
}

// TestAnUnlinkedHiddenHostSaysWhereToShowIt: this machine hidden while
// ws-1 is home has no link, so s and h say where it shows again.
func TestAnUnlinkedHiddenHostSaysWhereToShowIt(t *testing.T) {
	now := boardAt
	m, _, _ := hostsAt(t, &now, "ws-1", 160, 40)
	m = boardOn(upd(t, m, stateMsg(waiting(1, "hearth-api", "%1", now))))
	m.tab, m.sideOn, m.keysIn, m.hiddenHosts = tabSessions, true, focusList, []string{"this machine"}
	for _, k := range []string{"s", "h"} {
		if got := pressOn(m, k).noticeText; got != "this machine is hidden · Machine › Hosts shows it" {
			t.Errorf("%s: notice %q", k, got)
		}
	}
}

// TestOneHostShownStillNamesAnotherLostHost: s on ws-1 leaves out no
// host's trouble: only all hosts leaves a hidden host out.
func TestOneHostShownStillNamesAnotherLostHost(t *testing.T) {
	m, now, conns := hostsConsole(t)
	st := waiting(2, "flint-auth", "%2", *now)
	st.Stoker = &proto.StokerView{On: true}
	m = upd(t, m, peerMsg{host: "ws-2", conn: conns["ws-2"], msg: stateMsg(st)})
	m = upd(t, m, peerMsg{host: "ws-2", conn: conns["ws-2"], msg: lostMsg{err: io.EOF, conn: conns["ws-2"]}})
	*now = now.Add(2 * time.Minute)
	m.sideOn = false
	m = pressOn(m, "s") // ws-1
	if m.boardHost != "ws-1" {
		t.Fatalf("s turned to %q", m.boardHost)
	}
	f := strings.Join(frameOf(m), "\n")
	if !strings.Contains(f, "ws-2: connection lost.") || !strings.Contains(f, "ws-2: not reached since") || !strings.Contains(frameOf(m)[0], "ws-2 stale") {
		t.Errorf("s on ws-1 hides the lost ws-2:\n%s", f)
	}
	if got := m.stokerBadge(false); !strings.Contains(got, "stoker 1/2") {
		t.Errorf("the stoker badge on ws-1: %q", got)
	}
	m.peers = slices.Clone(m.peers)
	m.peers[0].blocked = errors.New("permission denied")
	if bs := m.blockedHosts(); len(bs) != 1 || bs[0].host != "ws-2" {
		t.Fatalf("s on ws-1: blocked rows %+v, want ws-2's", bs)
	}
	if got := m.unreached(); !slices.Equal(got, []string{"ws-2"}) {
		t.Errorf("s on ws-1: the lines ↑ reaches %q", got)
	}
}
