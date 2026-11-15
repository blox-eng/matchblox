package app

import (
	"hash/crc32"
	"sync"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

// boardAt is the clock of the board's fixtures.
var boardAt = demoAt

// fixtureSession is the first session of the fixture machine, sampled
// once: a board fixture makes fifteen matches of it, and a sample of the
// machine reads its files (minutes a test on macOS under -race).
var fixtureSession = sync.OnceValue(func() sample.Session { return fixtureState().Sessions[0] })

// boardMatch is a match of the demo fleet on the fixture machine: at rest
// for ten minutes on its own pane and pid, with no shell, no question, no
// limit, and no report of its own: its lane is read as for an agent that
// reports nothing (boardreport_test.go has the reports).
func boardMatch(name string, started time.Duration) sample.Session {
	s := fixtureSession()
	s.PID = 1000 + int(crc32.ChecksumIEEE([]byte(name))%100000)
	s.Name, s.Pane, s.Target, s.SessionID = name, "%"+name, "w:"+name, "sid-"+name
	s.Busy, s.Status, s.Idle, s.Out = false, "idle", 10*time.Minute, time.Time{}
	s.Reported, s.WaitingFor = false, ""
	s.Shells, s.Agents, s.Asks, s.Permits, s.Ask, s.Progress, s.Do = nil, 0, false, false, nil, nil, ""
	s.Started = boardAt.Add(-started)
	return s
}

// boardState is the fixture machine with only the matches ss: no queue,
// no ship, no hold and no stoker.
func boardState(ss ...sample.Session) proto.State {
	st := fixtureState()
	st.Sessions, st.Queue, st.Recommendations, st.Children = ss, nil, nil, nil
	st.Ships = &proto.Ships{}
	return st
}

func boardModel(t *testing.T, st proto.State) Model {
	t.Helper()
	m, _ := loadedWith(t, 160, st)
	m.opt.Now = func() time.Time { return boardAt }
	return (m)
}

func waitFor(s sample.Session, state string, ago time.Duration) queue.Item {
	return queue.Item{SessionID: s.SessionID, Pane: s.Pane, Name: s.Name, State: state, Since: boardAt.Add(-ago)}
}

func shipFor(s sample.Session, n int, kind, note string, stage int, ago time.Duration) proto.Ship {
	return proto.Ship{Repo: "hearth/" + s.Name, Number: n, Kind: kind, Note: note, Stage: stage,
		Since: boardAt.Add(-ago), Session: proto.ShipSession{Pane: s.Pane, Name: s.Name}}
}

func goFor(s sample.Session, ago time.Duration) proto.StokerEntry {
	return proto.StokerEntry{ID: "go-" + s.Name, At: boardAt.Add(-ago), Session: s.SessionID, Name: s.Name, Pane: s.Pane,
		Rule: "go", State: "waits", Needs: "merge:#4556", Want: "gh pr merge 4556"}
}
