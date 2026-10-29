package app

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/limits"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/sample"
)

// hostState is the fixture's state with only the sessions given, each
// made from the fixture's first in pane pane.
func hostState(sess ...func(sample.Session) sample.Session) proto.State {
	st := fixtureState()
	base := st.Sessions[0]
	base.SessionID, base.Busy, base.Status, base.Idle, base.Do, base.Shells, base.Agents, base.Progress =
		"", false, "idle", time.Minute, "", nil, 0, nil
	st.Sessions, st.Children, st.Queue, st.Holds, st.Reasons = nil, nil, nil, nil, nil
	for _, f := range sess {
		st.Sessions = append(st.Sessions, f(base))
	}
	return st
}

func sess(pid int, name, pane string, f func(*sample.Session)) func(sample.Session) sample.Session {
	return func(s sample.Session) sample.Session {
		s.PID, s.Name, s.Pane, s.Target = pid, name, pane, "w:"+name
		if f != nil {
			f(&s)
		}
		return s
	}
}

// twoHosts is a console whose home host sent home and whose peer ws-1 sent peer.
func twoHosts(t *testing.T, home, peer proto.State) (Model, *fakeConn) {
	t.Helper()
	m, _, ws := peered(nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 132, Height: 30})
	next, _ = next.Update(stateMsg(home))
	next, _ = next.Update(peerMsg{host: "ws-1", conn: ws, msg: stateMsg(peer)})
	return next.(Model), ws
}

func TestPeerSessionsJoinTheList(t *testing.T) {
	m, _ := twoHosts(t, hostState(sess(1, "api-auth", "%1", nil)), hostState(sess(2, "train-loop", "%2", nil)))
	hosts := map[string]string{}
	for _, s := range m.all.sessions {
		hosts[s.Name] = s.Host
	}
	if want := map[string]string{"api-auth": "", "train-loop": "ws-1"}; !reflect.DeepEqual(hosts, want) {
		t.Fatalf("the lists hold %v, want %v", hosts, want)
	}
	if got := order(m); !strings.Contains(got, "api-auth") || !strings.Contains(got, "train-loop") {
		t.Fatalf("the drawn list is %q", got)
	}
	if m.hostCount() != 2 {
		t.Fatalf("hostCount %d", m.hostCount())
	}
}

func TestAWaitOnAPeerSortsAboveHomeWork(t *testing.T) {
	busy := func(s *sample.Session) { s.Busy, s.Status = true, "busy" }
	home := hostState(sess(1, "api-auth", "%1", busy), sess(3, "web-ui", "%3", busy))
	peer := hostState(sess(2, "train-loop", "%2", nil))
	peer.Queue = []queue.Item{{Pane: "%2", Name: "train-loop", State: queue.StateQuestion, Since: peer.At.Add(-time.Minute)}}
	m, _ := twoHosts(t, home, peer)
	if got := order(m); got != "train-loop api-auth web-ui" {
		t.Fatalf("order %q", got)
	}
	if w, l := lineOf(t, m, "WAITS FOR YOU · 1"), lineOf(t, m, "WORKING · 2"); w > l {
		t.Fatalf("the peer's wait is under home's work:\n%s", screen(m))
	}
	if it, ok := m.waitOf(m.snap.Sessions[0]); !ok || it.Host != "ws-1" {
		t.Fatalf("the wait is %+v, %v", it, ok)
	}
}

// TestSamePaneOnTwoHostsAreTwoRows: pane ids and PIDs repeat across hosts;
// the selection is (host, PID) and stays on its host.
func TestSamePaneOnTwoHostsAreTwoRows(t *testing.T) {
	home := hostState(sess(4242, "home-12", "%12", nil))
	m, _ := twoHosts(t, home, hostState(sess(4242, "peer-12", "%12", nil)))
	if got := order(m); got != "home-12 peer-12" {
		t.Fatalf("order %q", got)
	}
	var next tea.Model = m
	for range 2 {
		if s, _ := next.(Model).selectedSession(); s.Name == "peer-12" {
			break
		}
		next, _ = key(next, "down")
	}
	if s, _ := next.(Model).selectedSession(); s.Name != "peer-12" || s.Host != "ws-1" {
		t.Fatalf("selected %s on %q", s.Name, s.Host)
	}
	next, _ = next.Update(stateMsg(home))
	if s, _ := next.(Model).selectedSession(); s.Name != "peer-12" {
		t.Fatalf("a new home state moved the selection to %s", s.Name)
	}
	kids := home
	kid := kids.Sessions[0]
	kid.PID, kid.Name, kid.Parent = 5000, "helper", &sample.ProcRef{PID: 4242, Start: kid.Start}
	kids.Children = []sample.Session{kid}
	next, _ = next.Update(stateMsg(kids))
	out := ansi.Strip(next.(Model).render())
	if n := strings.Count(out, "helper"); n != 1 {
		t.Fatalf("home's child shows %d times:\n%s", n, out)
	}
}

// TestHomeShipsAndFocusStayHome: the ships and the focus are the home
// host's; a peer's session in the same pane is not theirs.
func TestHomeShipsAndFocusStayHome(t *testing.T) {
	home := hostState(sess(1, "home-12", "%12", nil), sess(2, "home-13", "%13", nil))
	home.Ships = &proto.Ships{Ships: []proto.Ship{{Repo: "o/r", Number: 7, Session: proto.ShipSession{Pane: "%12"}}}}
	m, _ := twoHosts(t, home, hostState(sess(1, "a-peer-12", "%12", nil))) // A to Z: it sorts first
	for _, s := range m.snap.Sessions {
		if n := len(m.shipsOf(s)); (s.Name == "home-12") != (n == 1) {
			t.Errorf("%s on %q drives %d ships", s.Name, s.Host, n)
		}
	}
	if d, _ := m.driver(home.Ships.Ships[0]); d.Host != "" {
		t.Fatalf("the ship's driver is on %q", d.Host)
	}
	focus := home
	focus.Focus = &proto.Focus{Pane: "%12", At: home.At}
	next, _ := m.Update(stateMsg(focus))
	if s, _ := next.(Model).selectedSession(); s.Name != "home-12" {
		t.Fatalf("the focus opened %s", s.Name)
	}
}

func TestLookupsByPaneStayOnTheirHost(t *testing.T) {
	signing := func(s *sample.Session) { s.SigningIn = true }
	home := hostState(sess(1, "home-12", "%12", nil), sess(2, "home-13", "%13", nil), sess(3, "home-14", "%14", signing))
	home.Holds = []proto.Hold{{Pane: "%12", Name: "home-12", By: holdByYou}}
	home.Reasons = map[string]proto.IdleReason{"%13": {Word: "rate limit", Tone: "warn"}}
	home.SignIns = map[string]string{"%14": "by hand"}
	peer := hostState(sess(1, "peer-12", "%12", nil), sess(2, "peer-13", "%13", nil), sess(3, "peer-14", "%14", signing))
	m, _ := twoHosts(t, home, peer)
	byName := map[string]sample.Session{}
	for _, s := range m.snap.Sessions {
		byName[s.Name] = s
	}
	if _, held := m.holdOf(byName["home-12"]); !held {
		t.Fatal("home's hold is lost")
	}
	if _, held := m.holdOf(byName["peer-12"]); held {
		t.Fatal("a hold on home %12 holds the peer's %12")
	}
	for name, want := range map[string]string{"home-13": "rate limit", "peer-13": "idle", "home-14": "sign in by hand", "peer-14": "signing in"} {
		if w, _ := m.rowWord(byName[name]); ansi.Strip(w) != want {
			t.Errorf("%s: word %q, want %q", name, ansi.Strip(w), want)
		}
	}
}

// TestOneHostMergeIsToday: with no peers, merge leaves the lists as the
// home host sent them.
func TestOneHostMergeIsToday(t *testing.T) {
	st := fixtureState()
	want := lists{queue: slices.Clone(st.Queue), recs: slices.Clone(st.Recommendations),
		sessions: slices.Clone(st.Sessions), orphans: slices.Clone(st.Orphans), children: slices.Clone(st.Children)}
	m, _ := loadedWith(t, 132, st)
	if !reflect.DeepEqual(m.all, want) {
		t.Fatalf("one host's lists changed:\n got %+v\nwant %+v", m.all, want)
	}
	if !reflect.DeepEqual(m.snap.Children, st.Children) {
		t.Fatal("one host's children changed")
	}
	if m.hostCount() != 1 {
		t.Fatalf("hostCount %d", m.hostCount())
	}
}

func TestMachineAndLimitsAreTheHosts(t *testing.T) {
	home, peer := hostState(), hostState()
	home.Machine.MemTotal, peer.Machine.MemTotal = 1, 2
	peer.Limits = []limits.Account{{Account: "ws-1 max"}}
	home.Limits = nil
	m, _ := twoHosts(t, home, peer)
	if m.machineOf("").MemTotal != 1 || m.machineOf("ws-1").MemTotal != 2 {
		t.Fatalf("machines %d, %d", m.machineOf("").MemTotal, m.machineOf("ws-1").MemTotal)
	}
	if len(m.limitsOf("")) != 0 || len(m.limitsOf("ws-1")) != 1 || m.limitsOf("ws-1")[0].Account != "ws-1 max" {
		t.Fatalf("limits %v, %v", m.limitsOf(""), m.limitsOf("ws-1"))
	}
}
