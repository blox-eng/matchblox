package app

import (
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

var laneAt = time.Date(2026, 10, 29, 14, 2, 0, 0, time.UTC)

func busy(s *sample.Session) { s.Busy, s.Status = true, "busy" }

// waiting is hostState with a queue row for each of its sessions.
func waiting(st proto.State) proto.State {
	for _, s := range st.Sessions {
		st.Queue = append(st.Queue, queue.Item{Pane: s.Pane, Name: s.Name, State: queue.StateQuestion, Since: st.At})
	}
	return st
}

func laneModel(t *testing.T, now *time.Time, peers ...string) (Model, map[string]*fakeConn) {
	t.Helper()
	m, _, conns := hostsAt(t, now, "gpu-0", 132, 30, peers...)
	return m, conns
}

func laneOfPID(m Model, host string, pid int) [lanePast]laneState {
	for _, s := range m.all.sessions {
		if s.Host == host && s.PID == pid {
			return m.laneOf(s, m.now())
		}
	}
	return [lanePast]laneState{}
}

func TestLaneCellTakesTheMostUrgent(t *testing.T) {
	now := laneAt
	m, _ := laneModel(t, &now)
	m = upd(t, m, stateMsg(hostState(sess(1, "api", "%1", busy))))
	now = now.Add(time.Minute)
	m = upd(t, m, stateMsg(waiting(hostState(sess(1, "api", "%1", nil)))))
	now = now.Add(time.Minute)
	m = upd(t, m, stateMsg(hostState(sess(1, "api", "%1", busy))))
	if got := laneOfPID(m, "gpu-0", 1)[lanePast-1]; got != laneWait {
		t.Fatalf("the cell is %d, want wait", got)
	}
}

func TestLaneBeforeOpenIsBlank(t *testing.T) {
	now := laneAt
	m, _ := laneModel(t, &now)
	m = upd(t, m, stateMsg(hostState(sess(1, "api", "%1", busy))))
	lane := laneOfPID(m, "gpu-0", 1)
	for i, c := range lane[:lanePast-1] {
		if c != laneNone {
			t.Fatalf("cell %d is %d before the first state", i, c)
		}
	}
	if lane[lanePast-1] != laneWork {
		t.Fatalf("the open cell is %d", lane[lanePast-1])
	}
}

func TestLaneShifts(t *testing.T) {
	now := laneAt
	m, _ := laneModel(t, &now)
	m = upd(t, m, stateMsg(hostState(sess(1, "api", "%1", busy))))
	now = now.Add(7 * time.Minute)
	m = upd(t, m, stateMsg(hostState(sess(1, "api", "%1", nil))))
	lane := laneOfPID(m, "gpu-0", 1)
	if lane[lanePast-2] != laneWork || lane[lanePast-1] != laneIdle {
		t.Fatalf("the lane is %v", lane[lanePast-4:])
	}
}

func TestLaneReadsShiftWithoutAState(t *testing.T) {
	now := laneAt
	m, _ := laneModel(t, &now)
	m = upd(t, m, stateMsg(hostState(sess(1, "api", "%1", busy))))
	now = now.Add(10 * time.Minute)
	if lane := laneOfPID(m, "gpu-0", 1); lane[lanePast-3] != laneWork || lane[lanePast-1] != laneNone {
		t.Fatalf("the lane is %v", lane[lanePast-4:])
	}
}

func TestLaneStopsAtAStaleHost(t *testing.T) {
	now := laneAt
	m, conns := laneModel(t, &now, "ws-1")
	peer := peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(hostState(sess(2, "train", "%2", busy)))}
	m = upd(t, m, stateMsg(hostState(sess(1, "api", "%1", busy))))
	m = upd(t, m, peer)
	for range 12 {
		now = now.Add(time.Minute)
		m = upd(t, m, stateMsg(hostState(sess(1, "api", "%1", busy))))
	}
	peerLane := laneOfPID(m, "ws-1", 2)
	if peerLane[lanePast-1] != laneNone || peerLane[lanePast-2] != laneNone || peerLane[lanePast-3] != laneWork {
		t.Fatalf("the stale host's lane is %v", peerLane[lanePast-4:])
	}
	if homeLane := laneOfPID(m, "gpu-0", 1); homeLane[lanePast-1] != laneWork {
		t.Fatalf("home's lane is %v", homeLane[lanePast-4:])
	}
}

func TestLaneIsPerHost(t *testing.T) {
	now := laneAt
	m, conns := laneModel(t, &now, "ws-1")
	m = upd(t, m, stateMsg(hostState(sess(4242, "home-12", "%12", busy))))
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(hostState(sess(4242, "peer-12", "%12", nil)))})
	if got := laneOfPID(m, "gpu-0", 4242)[lanePast-1]; got != laneWork {
		t.Fatalf("home's cell is %d", got)
	}
	if got := laneOfPID(m, "ws-1", 4242)[lanePast-1]; got != laneIdle {
		t.Fatalf("the peer's cell is %d", got)
	}
}

func TestLaneDropsASessionGoneForAWindow(t *testing.T) {
	now := laneAt
	m, _ := laneModel(t, &now)
	m = upd(t, m, stateMsg(hostState(sess(1, "api", "%1", busy))))
	now = now.Add(lanePast * laneCell)
	m = upd(t, m, stateMsg(hostState(sess(2, "web", "%2", busy))))
	if _, ok := m.lanes[laneKey{"gpu-0", 1}]; ok {
		t.Fatal("the lane of a session gone for a window stays")
	}
}

func TestLaneCopiesOnWrite(t *testing.T) {
	now := laneAt
	m, _ := laneModel(t, &now)
	m = upd(t, m, stateMsg(hostState(sess(1, "api", "%1", busy))))
	before := m
	m = upd(t, m, stateMsg(waiting(hostState(sess(1, "api", "%1", nil)))))
	if before.laneOf(m.all.sessions[0], now)[lanePast-1] != laneWork {
		t.Fatal("a later state wrote into an earlier copy")
	}
}
