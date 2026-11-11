package app

import (
	"hash/crc32"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// boardAt is the clock of the board's fixtures.
var boardAt = demoAt

// boardMatch is a match of the demo fleet on the fixture machine: at rest
// for ten minutes on its own pane and pid, with no shell, no question, no
// limit.
func boardMatch(name string, started time.Duration) sample.Session {
	s := fixtureState().Sessions[0]
	s.PID = 1000 + int(crc32.ChecksumIEEE([]byte(name))%100000)
	s.Name, s.Pane, s.Target, s.SessionID = name, "%"+name, "w:"+name, "sid-"+name
	s.Busy, s.Status, s.Idle, s.Out = false, "idle", 10*time.Minute, time.Time{}
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
	return m
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

// TestColumnOf: a match is in the leftmost column whose rule holds, one
// case for each line of the spec's Columns table and for each clash.
func TestColumnOf(t *testing.T) {
	asks := func(st *proto.State, s *sample.Session) {
		st.Queue = append(st.Queue, waitFor(*s, queue.StatePermission, 4*time.Minute))
	}
	ship := func(kind, note string, stage int) func(*proto.State, *sample.Session) {
		return func(st *proto.State, s *sample.Session) {
			st.Ships.Ships = append(st.Ships.Ships, shipFor(*s, 4556, kind, note, stage, time.Hour))
		}
	}
	hold := func(by string) func(*proto.State, *sample.Session) {
		return func(st *proto.State, s *sample.Session) {
			st.Holds = append(st.Holds, proto.Hold{SessionID: s.SessionID, Pane: s.Pane, Name: s.Name, By: by})
		}
	}
	busy := func(_ *proto.State, s *sample.Session) { s.Busy, s.Status, s.Idle = true, "busy", 0 }
	shell := func(_ *proto.State, s *sample.Session) {
		s.Shells = []sample.Shell{{Command: "npm run dev"}}
	}
	for _, c := range []struct {
		name   string
		set    []func(*proto.State, *sample.Session)
		want   boardCol
		counts bool
	}{
		{"asks a question", []func(*proto.State, *sample.Session){func(st *proto.State, s *sample.Session) {
			st.Queue = append(st.Queue, waitFor(*s, queue.StateQuestion, 4*time.Minute))
		}}, colNeed, true},
		{"asks a permission", []func(*proto.State, *sample.Session){asks}, colNeed, true},
		{"asks from its pane", []func(*proto.State, *sample.Session){func(_ *proto.State, s *sample.Session) { s.Asks = true }}, colNeed, true},
		{"waits for your merge", []func(*proto.State, *sample.Session){ship(shipWaits, "green · waits for your merge", 2)}, colNeed, true},
		{"a stoker step waits for a go on its pane", []func(*proto.State, *sample.Session){func(st *proto.State, s *sample.Session) {
			st.Stoker = &proto.StokerView{Mode: proto.StokerMode{On: true}, Run: []proto.StokerEntry{goFor(*s, 2*time.Minute)}}
		}}, colNeed, true},
		{"a chain stoker's step waits for a go on its pane", []func(*proto.State, *sample.Session){func(st *proto.State, s *sample.Session) {
			st.Stokers = []state.ChainStoker{{ID: "u1", Chain: "c1", Name: "hearth",
				View: proto.StokerView{Mode: proto.StokerMode{On: true}, Run: []proto.StokerEntry{goFor(*s, 2*time.Minute)}}}}
		}}, colNeed, true},
		{"a go that was given", []func(*proto.State, *sample.Session){func(st *proto.State, s *sample.Session) {
			given := goFor(*s, time.Minute)
			given.State = "done"
			st.Stoker = &proto.StokerView{Mode: proto.StokerMode{On: true}, Run: []proto.StokerEntry{goFor(*s, 2*time.Minute), given}}
		}}, colQuiet, false},
		{"a ship stuck", []func(*proto.State, *sample.Session){ship(shipStuck, "CI failed · lint", 1)}, colNeed, true},
		{"turn ended, waits for input", []func(*proto.State, *sample.Session){func(st *proto.State, s *sample.Session) {
			st.Queue = append(st.Queue, waitFor(*s, queue.StateFinished, 4*time.Minute))
		}}, colNeed, true},
		{"turn ended, the idle prompt", []func(*proto.State, *sample.Session){func(st *proto.State, s *sample.Session) {
			it := waitFor(*s, queue.StateQuestion, 4*time.Minute)
			it.Reminder = true
			st.Queue = append(st.Queue, it)
		}}, colNeed, true},
		{"turn ended, waits since yesterday", []func(*proto.State, *sample.Session){func(st *proto.State, s *sample.Session) {
			s.Idle = 26 * time.Hour
			st.Queue = append(st.Queue, waitFor(*s, queue.StateFinished, 26*time.Hour))
		}}, colQuiet, false},
		{"asks, and a background shell runs", []func(*proto.State, *sample.Session){asks, shell}, colNeed, true},
		{"asks, while the sample says busy", []func(*proto.State, *sample.Session){asks, busy}, colNeed, true},
		{"busy", []func(*proto.State, *sample.Session){busy}, colWork, false},
		{"background agents", []func(*proto.State, *sample.Session){func(_ *proto.State, s *sample.Session) {
			s.Status, s.Agents = sample.StatusAgents, 2
		}}, colWork, false},
		{"busy, and its ship is stuck", []func(*proto.State, *sample.Session){busy, ship(shipStuck, "CI failed · lint", 1)}, colWork, false},
		{"busy, and its ship waits for your merge", []func(*proto.State, *sample.Session){busy, ship(shipWaits, "green · waits for your merge", 2)}, colWork, false},
		{"at rest, ship in ci", []func(*proto.State, *sample.Session){ship(shipRunning, "CI · 2 of 5 jobs", 1)}, colShip, false},
		{"at rest, ship merged, deploying", []func(*proto.State, *sample.Session){ship(shipRunning, "merged · deploying", 3)}, colShip, false},
		{"at rest, ship live", []func(*proto.State, *sample.Session){ship(shipDone, "live", 4)}, colQuiet, false},
		{"idle, no ship", nil, colQuiet, false},
		{"parked by the owner, and it asks", []func(*proto.State, *sample.Session){hold(holdByYou), asks}, colQuiet, false},
		{"held by the stoker, and it asks", []func(*proto.State, *sample.Session){hold("stoker"), asks}, colNeed, true},
		{"held by the stoker, at rest", []func(*proto.State, *sample.Session){hold("stoker")}, colQuiet, false},
		{"parked, ship in ci", []func(*proto.State, *sample.Session){hold(holdByYou), ship(shipRunning, "CI · 2 of 5 jobs", 1)}, colQuiet, false},
		{"parked, and busy", []func(*proto.State, *sample.Session){hold(holdByYou), busy}, colQuiet, false},
		{"out (the plan's limit)", []func(*proto.State, *sample.Session){func(st *proto.State, s *sample.Session) {
			s.Out = boardAt.Add(2 * time.Hour)
			st.Queue = append(st.Queue, waitFor(*s, queue.StateFinished, 4*time.Minute))
		}}, colQuiet, false},
		{"cold", []func(*proto.State, *sample.Session){func(_ *proto.State, s *sample.Session) { s.Idle = 30 * time.Hour }}, colQuiet, false},
		{"cold, and it asks", []func(*proto.State, *sample.Session){func(st *proto.State, s *sample.Session) {
			s.Idle = 30 * time.Hour
			st.Queue = append(st.Queue, waitFor(*s, queue.StatePermission, 30*time.Hour))
		}}, colQuiet, false},
		{"stale, and its ship is stuck", []func(*proto.State, *sample.Session){func(_ *proto.State, s *sample.Session) {
			s.Idle = 8 * 24 * time.Hour
		}, ship(shipStuck, "conflicts", 1)}, colQuiet, false},
		{"at rest with a background shell", []func(*proto.State, *sample.Session){shell}, colWork, false},
		{"a background shell, and its ship waits for your merge", []func(*proto.State, *sample.Session){shell, ship(shipWaits, "green · waits for your merge", 2)}, colWork, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := boardMatch("kindle-web", time.Hour)
			st := boardState()
			for _, f := range c.set {
				f(&st, &s)
			}
			st.Sessions = []sample.Session{s}
			m := boardModel(t, st)
			if len(m.all.sessions) != 1 {
				t.Fatalf("the board has %d matches", len(m.all.sessions))
			}
			if got := m.columnOf(m.all.sessions[0]); got != c.want {
				t.Fatalf("column %s, want %s", got.name(), c.want.name())
			}
			if n, want := m.needCount(), map[bool]int{true: 1}[c.counts]; n != want {
				t.Fatalf("needCount %d, want %d", n, want)
			}
		})
	}
}

func TestBoardColNames(t *testing.T) {
	var names, shorts []string
	for _, c := range boardCols {
		names, shorts = append(names, c.name()), append(shorts, c.short())
	}
	if got := strings.Join(names, "|"); got != "NEEDS YOU|WORKING|SHIPPING|QUIET" {
		t.Fatalf("names %q", got)
	}
	if got := strings.Join(shorts, "|"); got != "need|work|ship|quiet" {
		t.Fatalf("shorts %q", got)
	}
}

// boardNames are the names of each column of m, left to right.
func boardNames(m Model) [4]string {
	var out [4]string
	for _, c := range boardCols {
		var names []string
		for _, s := range m.boardColumn(c) {
			names = append(names, s.Name)
		}
		out[c] = strings.Join(names, " ")
	}
	return out
}

// TestBoardColumnOrder: NEEDS YOU by the oldest wait first, WORKING by
// the oldest session first, SHIPPING by the oldest stage first, QUIET by
// the latest activity first; a match with no time sorts last; a chain's
// members in one column sit together.
func TestBoardColumnOrder(t *testing.T) {
	var ss []sample.Session
	add := func(name string, started time.Duration, f func(*sample.Session)) sample.Session {
		s := boardMatch(name, started)
		if f != nil {
			f(&s)
		}
		ss = append(ss, s)
		return s
	}
	busy := func(s *sample.Session) { s.Busy, s.Status, s.Idle = true, "busy", 0 }

	// NEEDS YOU: a wait of 4m, one of 30m, a go of 12m, a stuck ship of 2h,
	// and a question read from the pane only, 20m after its last activity.
	kindle := add("kindle-web", time.Hour, nil)
	hearth := add("hearth-api", 5*time.Hour, nil)
	ember := add("ember-billing", 2*time.Hour, nil)
	flint := add("flint-auth", 3*time.Hour, nil)
	add("pyre-app", time.Hour, func(s *sample.Session) { s.Asks, s.Idle = true, 20*time.Minute })
	// WORKING: started 1h, 6h and 3h ago, and one with no start.
	add("spark-docs", time.Hour, busy)
	add("fume-cli", 0, func(s *sample.Session) { busy(s); s.Started = time.Time{} })
	add("cinder-ui", 6*time.Hour, busy)
	add("ash-cli", 3*time.Hour, busy)
	// SHIPPING: stages reached 20m, 3h and 1h ago.
	tinder := add("tinder-db", time.Hour, nil)
	coal := add("coal-jobs", time.Hour, nil)
	blaze := add("blaze-www", time.Hour, nil)
	// QUIET: idle 40m, 50m and 5m; their starts would order them the
	// other way.
	add("smoke-docs", 2*time.Hour, func(s *sample.Session) { s.Idle = 40 * time.Minute })
	add("wick-sdk", 30*time.Minute, func(s *sample.Session) { s.Idle = 50 * time.Minute })
	add("char-ops", 5*time.Hour, func(s *sample.Session) { s.Idle = 5 * time.Minute })

	st := boardState(ss...)
	st.Queue = []queue.Item{waitFor(kindle, queue.StatePermission, 4*time.Minute), waitFor(hearth, queue.StateFinished, 30*time.Minute)}
	st.Stoker = &proto.StokerView{Mode: proto.StokerMode{On: true}, Run: []proto.StokerEntry{goFor(ember, 12*time.Minute)}}
	st.Ships.Ships = []proto.Ship{
		shipFor(flint, 4561, shipStuck, "CI failed · lint", 1, 2*time.Hour),
		shipFor(tinder, 4547, shipRunning, "CI · 2 of 5 jobs", 1, 20*time.Minute),
		shipFor(coal, 219, shipRunning, "merged · deploying", 3, 3*time.Hour),
		shipFor(blaze, 4549, shipRunning, "green · auto-merge", 2, time.Hour),
	}
	want := [4]string{
		"flint-auth hearth-api pyre-app ember-billing kindle-web",
		"cinder-ui ash-cli spark-docs fume-cli",
		"coal-jobs blaze-www tinder-db",
		"char-ops smoke-docs wick-sdk",
	}
	m := boardModel(t, st)
	if got := boardNames(m); got != want {
		t.Fatalf("columns\n got %q\nwant %q", got, want)
	}
	if got := boardNames(boardModel(t, st)); got != want {
		t.Fatalf("a second console orders\n%q", got)
	}
	if idx := m.boardIndex(); len(idx[colNeed]) != m.needCount() {
		t.Fatalf("needCount %d, NEEDS YOU has %d", m.needCount(), len(idx[colNeed]))
	}

	// A chain of char-ops and wick-sdk: wick-sdk moves up to char-ops, the
	// first of them; smoke-docs, not in the chain, follows.
	st.Chains = &state.ChainsView{Chains: []chains.Chain{{ID: "c1", Name: "hearth", Repo: "/home/user/src/hearth"}}}
	for _, s := range ss {
		if s.Name == "wick-sdk" || s.Name == "char-ops" || s.Name == "hearth-api" || s.Name == "spark-docs" {
			st.Chains.Chains[0].Members = append(st.Chains.Chains[0].Members, memberOf(s))
		}
	}
	m = boardModel(t, st)
	got := boardNames(m)
	if got[colQuiet] != "char-ops wick-sdk smoke-docs" {
		t.Fatalf("the chain is apart in QUIET: %q", got[colQuiet])
	}
	// One chain across two other columns keeps each column's order.
	if got[colNeed] != want[colNeed] || got[colWork] != want[colWork] {
		t.Fatalf("a chain moved a lone member:\n%q", got)
	}
}
