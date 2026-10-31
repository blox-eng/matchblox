package app

import (
	"time"

	"github.com/blox-eng/matchblox/internal/sample"
)

// The lane of a session is its last two hours in cells of five minutes,
// kept by the console as it sees them: no service keeps a state history.
const (
	laneCell            = 5 * time.Minute
	lanePast, laneAhead = 24, 12
)

// laneState is what a session did in a cell; the most urgent state the
// console saw in the cell wins. A cell the console did not see is laneNone.
type laneState uint8

const (
	laneNone laneState = iota
	laneIdle
	laneWork
	laneWait
)

// laneKey is a session of a host: PIDs repeat across hosts, and a host
// reuses a PID, so the start tells a new process from the dead one.
type laneKey struct {
	host  string
	pid   int
	start uint64
}

// noteLanes writes the state of every session into the cell of now, after
// merge. A host whose last state is older than the cell adds nothing: a
// stale host leaves blank cells. A session gone for a window is dropped.
func (m *Model) noteLanes(now time.Time) {
	start := now.Truncate(laneCell)
	lanes := make(map[laneKey][lanePast]laneState, len(m.lanes))
	k := m.laneShift(start)
	for key, ring := range m.lanes {
		if ring = shiftLane(ring, k); ring != ([lanePast]laneState{}) {
			lanes[key] = ring
		}
	}
	for _, s := range m.all.sessions {
		if m.hostAt(s.Host).Before(start) {
			continue
		}
		key := laneKey{s.Host, s.PID, s.Start}
		ring := lanes[key]
		ring[lanePast-1] = max(ring[lanePast-1], m.laneStateOf(s))
		lanes[key] = ring
	}
	m.lanes = lanes
	if start.After(m.laneStart) {
		m.laneStart = start
	}
}

// laneOf is the lane of s as of now: the last cell is the one of now.
func (m Model) laneOf(s sample.Session, now time.Time) [lanePast]laneState {
	return shiftLane(m.lanes[laneKey{s.Host, s.PID, s.Start}], m.laneShift(now.Truncate(laneCell)))
}

// laneShift is the cells from the ring's end to the cell that starts at start.
func (m Model) laneShift(start time.Time) int {
	if m.laneStart.IsZero() || !start.After(m.laneStart) {
		return 0
	}
	return int(start.Sub(m.laneStart) / laneCell)
}

func shiftLane(ring [lanePast]laneState, k int) (out [lanePast]laneState) {
	if k < lanePast {
		copy(out[:], ring[k:])
	}
	return out
}

func (m Model) laneStateOf(s sample.Session) laneState {
	switch m.groupOf(s) {
	case groupWaits:
		return laneWait
	case groupWorking:
		return laneWork
	}
	return laneIdle
}

// hostAt is the time of the last state of a host. A peer that speaks less
// often than every cell leaves blank cells: the lane never invents a past.
func (m Model) hostAt(host string) time.Time {
	if i := m.peerAt(host); i >= 0 && host != m.opt.Host {
		return m.peers[i].at
	}
	return m.at
}
