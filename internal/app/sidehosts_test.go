package app

import (
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
