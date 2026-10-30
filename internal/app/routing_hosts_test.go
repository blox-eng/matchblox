package app

import (
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
)

// samePane is a console of two hosts that both run a session in %12 with
// the same PID, the peer's row selected. The peer alone can start agents.
func samePane(t *testing.T, peer func(*proto.State)) (tea.Model, *fakeConn, *fakeConn) {
	t.Helper()
	on := func(s *sample.Session) { s.Agent, s.Cwd = "claude", "/src/api" }
	home := hostState(sess(4242, "home-12", "%12", on))
	ws := hostState(sess(4242, "peer-12", "%12", on))
	ws.Agents = []string{"claude", "codex"}
	if peer != nil {
		peer(&ws)
	}
	m, homeConn, wsConn := peered(nil)
	var next tea.Model = m
	next, _ = next.Update(tea.WindowSizeMsg{Width: 132, Height: 30})
	next, _ = next.Update(stateMsg(home))
	next, _ = next.Update(peerMsg{host: "ws-1", conn: wsConn, msg: stateMsg(ws)})
	for range next.(Model).rowCount() {
		if s, ok := next.(Model).selectedSession(); ok && s.Host == "ws-1" {
			return next, homeConn, wsConn
		}
		next, _ = key(next, "down")
	}
	t.Fatal("no peer row to select")
	return nil, nil, nil
}

// pressAll presses each key and runs what it sends.
func pressAll(m tea.Model, keys ...string) tea.Model {
	for _, k := range keys {
		var cmd tea.Cmd
		m, cmd = key(m, k)
		drain(cmd)
	}
	return m
}

// TestActsGoToTheRowsHost: an act on the peer's %12 goes on the peer's
// conn with today's RecID, and never on home's, whose %12 is another pane.
func TestActsGoToTheRowsHost(t *testing.T) {
	waits := func(st *proto.State) {
		st.Queue = []queue.Item{{Pane: "%12", Name: "peer-12", State: queue.StateQuestion, Since: st.At.Add(-time.Minute)}}
	}
	asks := func(st *proto.State) {
		waits(st)
		st.Sessions[0].Ask = &said.Ask{ID: "t1", Questions: []said.Question{{Question: "which?", Options: []said.Option{{Label: "one"}, {Label: "two"}}}}}
	}
	previewed := func(t *testing.T, m tea.Model, ws *fakeConn) tea.Model {
		id := m.(Model).pending.previewAct
		next, _ := m.Update(peerMsg{host: "ws-1", conn: ws, msg: resultMsg{ActID: id, Preview: "# Handover"}})
		return next
	}
	for _, c := range []struct {
		name  string
		peer  func(*proto.State)
		keys  []string
		then  func(*testing.T, tea.Model, *fakeConn) tea.Model
		after []string
		want  string
	}{
		{name: "kill", keys: []string{"x", "y"}, want: "kill:%12"},
		{name: "park", keys: []string{"z"}, want: "park:%12"},
		{name: "rename", keys: []string{"r", "enter"}, want: "rename:%12"},
		{name: "switch-preview", keys: []string{"A", "enter"}, want: "switch-preview:%12"},
		{name: "switch", keys: []string{"A", "enter"}, then: previewed, after: []string{"y"}, want: "switch:%12"},
		{name: "new:session", keys: []string{"N", "tab", "enter"}, want: "new:session"},
		{name: "answer", peer: waits, keys: []string{"a", "o", "k", "enter"}, want: "answer:%12"},
		{name: "message", keys: []string{"a", "h", "i", "enter"}, want: "message:%12"},
		{name: "pick", peer: asks, keys: []string{"a", "1"}, want: "pick:%12"},
	} {
		t.Run(c.name, func(t *testing.T) {
			m, home, ws := samePane(t, c.peer)
			m = pressAll(m, c.keys...)
			if c.then != nil {
				m = c.then(t, m, ws)
			}
			m = pressAll(m, c.after...)
			if got := recIDs(ws.acts()); !slices.Contains(got, c.want) {
				t.Fatalf("the peer got %v, want %s; flash %q", got, c.want, m.(Model).flash)
			}
			if got := home.acts(); len(got) != 0 {
				t.Fatalf("home got %v", recIDs(got))
			}
		})
	}
}

// TestTheGenericActGoesToItsHost: an action that names a peer goes there
// on confirm.
func TestTheGenericActGoesToItsHost(t *testing.T) {
	m, home, ws := samePane(t, nil)
	mm := m.(Model)
	mm.pending = &action{label: "go", say: "go", rec: "go:%12", which: "primary", host: "ws-1"}
	pressAll(mm, "enter")
	if got := ws.acts(); len(got) != 1 || got[0] != (proto.Act{RecID: "go:%12", Which: "primary"}) {
		t.Fatalf("the peer got %+v", got)
	}
	if got := home.acts(); len(got) != 0 {
		t.Fatalf("home got %v", recIDs(got))
	}
}

func TestJumpToAPeerRowUsesThatHost(t *testing.T) {
	m, _, _ := samePane(t, nil)
	mm := m.(Model)
	mm.opt.JumpID = func() string { return "0a1b2c3d" }
	j := remote.Jump{Host: "ws-1", Pane: "%12", Prefix: "C-b", ID: "0a1b2c3d", Outer: true}
	want := j.Local(remote.JumpSSH("ws-1", j, nil))
	next, _ := key(mm, "enter")
	if a := next.(Model).pending; a == nil || len(a.steps) != 1 || !slices.Equal(a.steps[0], want) {
		t.Fatalf("the jump is %+v, want %q", a, want)
	}
}

func TestActOnAStaleRowIsRefused(t *testing.T) {
	m, home, ws := samePane(t, nil)
	lose := func(m tea.Model) Model {
		mm := m.(Model)
		mm.peers = slices.Clone(mm.peers)
		p := &mm.peers[0]
		p.lost, p.redialing, p.backoff, p.at = true, true, time.Second, mm.now().Add(-31*time.Second)
		return mm
	}
	const want = "ws-1 is not connected. It tries again in 1s."
	next := pressAll(lose(m), "x")
	if nm := next.(Model); nm.flash != want || nm.pending != nil {
		t.Fatalf("flash %q, pending %+v", nm.flash, nm.pending)
	}
	// The link goes while the confirm waits: the y sends nothing either.
	next = pressAll(lose(pressAll(m, "x")), "y")
	if got := next.(Model).flash; got != want {
		t.Fatalf("flash %q", got)
	}
	if len(ws.acts()) != 0 || len(home.acts()) != 0 {
		t.Fatalf("sent %v and %v", recIDs(ws.acts()), recIDs(home.acts()))
	}
}

// TestNewOnAPeerRowListsThatHostsRepos: N on the peer's row starts on the
// peer, from its own repos, and the list names the host.
func TestNewOnAPeerRowListsThatHostsRepos(t *testing.T) {
	m, _, _ := samePane(t, func(st *proto.State) {
		st.Found = &proto.Found{Consent: "yes", Repos: []proto.Checkout{{Path: "/w/peer-repo"}}}
	})
	mm := m.(Model)
	mm.found = &proto.Found{Consent: "yes", Repos: []proto.Checkout{{Path: "/w/home-repo"}}}
	mm.agents, mm.wrepos = []string{"claude"}, mm.whereRepos(mm.opt.Host)
	next, _ := key(mm, "N")
	nm := next.(Model)
	if nm.form == nil || nm.formHost() != "ws-1" {
		t.Fatalf("form %+v, flash %q", nm.form, nm.flash)
	}
	var paths []string
	for _, r := range nm.form.rows {
		if r.picks() {
			paths = append(paths, r.path())
		}
	}
	if slices.Contains(paths, "/w/home-repo") || !slices.Contains(paths, "/w/peer-repo") {
		t.Fatalf("the list holds %v", paths)
	}
	if r := nm.form.rows[0]; r.kind != rowLabel || r.text != "SUGGESTED · ws-1" {
		t.Fatalf("the label is %+v", r)
	}
	// The ghost row is the home host's.
	next, _ = key(mm, "down")
	if _, ok := next.(Model).selectedGhost(); !ok {
		t.Fatal("no ghost row under the peer's")
	}
	next, _ = key(next, "enter")
	if nm := next.(Model); nm.form == nil || nm.formHost() != nm.opt.Host {
		t.Fatalf("the ghost row starts on %q: %+v", nm.formHost(), nm.form)
	}
}

// TestPeekAndKillStayOnTheirHost: a peek on the peer's %12 selects the
// peer's row, and a wait on home's %12 asks no second y for the peer's.
func TestPeekAndKillStayOnTheirHost(t *testing.T) {
	m, _, _ := samePane(t, nil)
	mm := m.(Model)
	mm.queue = []queue.Item{{Pane: "%12", Name: "home-12", State: queue.StateQuestion, Host: mm.opt.Host}}
	mm.selPID, mm.selHost = 0, "" // nothing selected: the peek selects
	mm.openPeek(paneKey{"ws-1", "%12"})
	if s, ok := mm.selectedSession(); !ok || s.Host != "ws-1" {
		t.Fatalf("the peek selected %+v", s)
	}
	if v, _, ok := mm.peekNow(); !ok || v.s.Host != "ws-1" {
		t.Fatalf("the peek shows %+v", v.s)
	}
	s, _ := mm.selectedSession()
	if a := mm.killAction(s); a == nil || a.again != "" || a.host != "ws-1" {
		t.Fatalf("the kill is %+v", a)
	}
}

// TestHomeRefusalAndLabelUseItsName: after its hello, the home host is
// called by its own name in a refusal and in the where list's label.
func TestHomeRefusalAndLabelUseItsName(t *testing.T) {
	m, _, _ := samePane(t, nil)
	mm := upd(t, m.(Model), helloMsg(proto.Hello{Version: proto.Version, Host: "hearth"}))
	mm.lost, mm.redialing, mm.backoff = true, true, time.Second
	if got, want := mm.refusal(mm.opt.Host), "hearth is not connected. It tries again in 1s."; got != want {
		t.Fatalf("refusal %q, want %q", got, want)
	}
	mm.lost = false
	mm.found = &proto.Found{Consent: "yes", Repos: []proto.Checkout{{Path: "/w/home-repo"}}}
	mm.agents, mm.wrepos = []string{"claude"}, mm.whereRepos(mm.opt.Host)
	nm := mm.openNewAt(mm.opt.Host, "", "")
	if nm.form == nil || len(nm.form.rows) == 0 {
		t.Fatalf("no where list: %+v", nm.form)
	}
	if r := nm.form.rows[0]; r.kind != rowLabel || r.text != "SUGGESTED · hearth" {
		t.Fatalf("the label is %+v", r)
	}
}

// TestSwitchJumpsOnItsHost: a switch on the peer that ran jumps to the
// peer's pane over ssh, never to this machine's tmux.
func TestSwitchJumpsOnItsHost(t *testing.T) {
	m, _, _ := samePane(t, nil)
	mm := m.(Model)
	mm.opt.Run = func(argv []string) error { t.Fatalf("a tmux move of ws-1 ran on this machine: %v", argv); return nil }
	mm.jumpAfter.act, mm.jumpAfter.host, mm.jumpAfter.pane, mm.jumpAfter.done = "a9", "ws-1", "%12", "switched peer-12 to codex"
	next, jump := mm.switched(proto.Result{ActID: "a9", Ran: [][]string{{"tmux"}}})
	if jump == nil || next.flash != "switched peer-12 to codex" {
		t.Fatalf("jump %v, flash %q; want the switch, and the jump on ws-1", jump != nil, next.flash)
	}
}
