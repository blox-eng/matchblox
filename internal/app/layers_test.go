package app

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/setup"
	"github.com/blox-eng/matchblox/internal/transport"
)

// shellFor is a shell whose consoles talk to fakes; opened records the hosts
// it opened, ran what it gave the terminal to.
type shellRig struct {
	sh      Shell
	opened  []string
	conns   map[string]*fakeConn
	ran     [][]string
	added   []string
	hidden  []string       // the hidden file's lines
	redials map[string]int // the redials of each host's stream
}

func newShell(t *testing.T, added []string, ssh []string, start string, firstRun bool) *shellRig {
	t.Helper()
	return rigShell(t, added, nil, ssh, start, firstRun)
}

// hidingShell is a shell whose hidden file already lists hidden.
func hidingShell(t *testing.T, added, hidden []string, start string) *shellRig {
	t.Helper()
	return rigShell(t, added, hidden, nil, start, false)
}

func rigShell(t *testing.T, added, hidden, ssh []string, start string, firstRun bool) *shellRig {
	t.Helper()
	r := &shellRig{added: added, hidden: hidden, conns: map[string]*fakeConn{}, redials: map[string]int{}}
	r.sh = NewShell(ShellOptions{
		Base:     Options{NoMotion: true, Self: "/bin/matchblox"},
		Hosts:    func() []string { return slices.Clone(r.added) },
		SSHHosts: func() []string { return ssh },
		Hidden:   func() []string { return slices.Clone(r.hidden) },
		Hide:     func(h string) error { r.hidden = append(r.hidden, h); return nil },
		Show: func(h string) error {
			r.hidden = slices.DeleteFunc(r.hidden, func(l string) bool { return l == h })
			return nil
		},
		Open: func(host string) Options {
			r.opened = append(r.opened, host)
			f := newFake()
			r.conns[host] = f
			redial := func() (transport.Conn, error) { r.redials[host]++; return newFake(), nil }
			return Options{Conn: f, Host: host, Redial: redial}
		},
		Exec: func(argv []string, done func(error) tea.Msg) tea.Cmd {
			r.ran = append(r.ran, argv)
			if len(argv) == 3 && argv[1] == "connect" && !slices.Contains(r.added, argv[2]) {
				r.added = append(r.added, argv[2]) // what matchblox connect writes
			}
			return func() tea.Msg { return done(nil) }
		},
		Start:    start,
		FirstRun: firstRun,
	})
	next, _ := r.sh.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	r.sh = next.(Shell)
	r.dialPeers()
	return r
}

// dialPeers runs the first dial of each peer of the console, as its Init
// would.
func (r *shellRig) dialPeers() {
	for i, l := range r.sh.cur.peers {
		if l.conn == nil && l.redialing {
			r.update(r.sh.cur.dialPeer(i)())
		}
	}
}

func (r *shellRig) key(t *testing.T, k string) {
	t.Helper()
	next, cmd := key(r.sh, k)
	r.sh = next.(Shell)
	r.drain(cmd)
}

// drain runs what a key asked for and feeds back the shell's own messages.
// A console's reads block on the fake, so each command gets 100 ms.
func (r *shellRig) drain(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	got := make(chan tea.Msg, 1)
	go func() { got <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-got:
	case <-time.After(100 * time.Millisecond):
		return
	}
	switch msg := msg.(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			r.drain(c)
		}
	case layerMsg, connectedHostMsg, peerMsg:
		next, cmd := r.sh.Update(msg)
		r.sh = next.(Shell)
		r.drain(cmd)
	}
}

func (r *shellRig) view() string { return ansi.Strip(r.sh.render()) }

func TestNoHostsStartsInThisMachinesConsole(t *testing.T) {
	r := newShell(t, nil, nil, "", false)
	if r.sh.layer != layerConsole || !slices.Equal(r.opened, []string{""}) {
		t.Fatalf("layer %v, opened %q", r.sh.layer, r.opened)
	}
}

func TestHostsListsEveryHost(t *testing.T) {
	r := newShell(t, []string{"ws-1", "ws-2"}, nil, "", false)
	r.toHosts(t)
	out := r.view()
	for _, want := range []string{"matchblox · hosts", "this machine", "ws-1", "ws-2", "+ add a host"} {
		if !strings.Contains(out, want) {
			t.Fatalf("view lacks %q:\n%s", want, out)
		}
	}
}

func TestEnterOpensAHostAndEscComesBack(t *testing.T) {
	r := hidingShell(t, []string{"ws-1"}, []string{"this machine"}, "")
	r.toHosts(t) // ws-1 selected
	r.key(t, "enter")
	if r.sh.layer != layerConsole || !slices.Equal(r.opened, []string{"ws-1"}) {
		t.Fatalf("layer %v, opened %q", r.sh.layer, r.opened)
	}
	next, _ := r.sh.Update(stateMsg(fixtureState()))
	r.sh = next.(Shell)
	if first := strings.Split(r.view(), "\n")[0]; !strings.Contains(first, "matchblox · ws-1 · 1 hidden") {
		t.Fatalf("header %q", first)
	}
	r.key(t, "esc")
	if r.sh.layer != layerHosts || r.conns["ws-1"].closed {
		t.Fatalf("esc: layer %v, closed %v", r.sh.layer, r.conns["ws-1"].closed)
	}
	if r.sh.sel != 1 {
		t.Fatalf("the hosts layer must keep ws-1 selected, sel %d", r.sh.sel)
	}
}

func TestEscClearsTheSearchBeforeItLeaves(t *testing.T) {
	r := newShell(t, []string{"ws-1"}, nil, "ws-1", false)
	next, _ := r.sh.Update(stateMsg(fixtureState()))
	r.sh = next.(Shell)
	r.key(t, "/")
	r.key(t, "a")
	r.key(t, "enter")
	r.key(t, "esc")
	if r.sh.layer != layerConsole {
		t.Fatal("esc left the console while a search was on")
	}
	r.key(t, "esc")
	if r.sh.layer != layerHosts {
		t.Fatal("the second esc did not go back")
	}
}

func TestAHostArgumentOpensItDirectly(t *testing.T) {
	r := newShell(t, nil, nil, "ws-9", false)
	if r.sh.layer != layerConsole || !slices.Equal(r.opened, []string{"ws-9"}) {
		t.Fatalf("layer %v, opened %q", r.sh.layer, r.opened)
	}
}

func TestAddAHostConnectsThenOpensIt(t *testing.T) {
	r := newShell(t, []string{"ws-1"}, []string{"ws-1", "ws-2", "github.com"}, "", false)
	r.toHosts(t)
	r.key(t, "down")
	r.key(t, "down")
	r.key(t, "enter") // + add a host
	out := r.view()
	if !strings.Contains(out, "ws-2") || !strings.Contains(out, "type a host") || strings.Count(out, "ws-1") != 0 {
		t.Fatalf("the picker must list the ssh hosts not added yet:\n%s", out)
	}
	r.key(t, "enter") // ws-2
	if !strings.Contains(r.view(), "/bin/matchblox connect ws-2") || len(r.ran) != 0 {
		t.Fatalf("enter must show the connect and wait:\n%s", r.view())
	}
	r.key(t, "y")
	if len(r.ran) != 1 || strings.Join(r.ran[0], " ") != "/bin/matchblox connect ws-2" {
		t.Fatalf("ran %q", r.ran)
	}
	if r.sh.layer != layerConsole || r.opened[len(r.opened)-1] != "ws-2" {
		t.Fatalf("after the connect: layer %v, opened %q", r.sh.layer, r.opened)
	}
}

func TestTypeAHost(t *testing.T) {
	r := newShell(t, nil, nil, "", true)
	r.key(t, "down")
	r.key(t, "enter") // another machine
	r.key(t, "enter") // type a host (the only row)
	for _, k := range strings.Split("me@ws-3", "") {
		r.key(t, k)
	}
	r.key(t, "enter")
	r.key(t, "y")
	if len(r.ran) != 1 || strings.Join(r.ran[0], " ") != "/bin/matchblox connect me@ws-3" {
		t.Fatalf("ran %q\n%s", r.ran, r.view())
	}
}

func TestATypedHostMustBeAHostName(t *testing.T) {
	r := newShell(t, nil, nil, "", true)
	r.key(t, "down")
	r.key(t, "enter")
	r.key(t, "enter")
	for _, k := range strings.Split("-oProxyCommand=x", "") {
		r.key(t, k)
	}
	r.key(t, "enter")
	if !strings.Contains(r.view(), "not a host name") {
		t.Fatalf("no refusal:\n%s", r.view())
	}
	r.key(t, "y")
	if len(r.ran) != 0 {
		t.Fatalf("ran %q", r.ran)
	}
}

func TestFirstRunAsksWhereTheAgentsRun(t *testing.T) {
	r := newShell(t, nil, nil, "", true)
	out := r.view()
	for _, want := range []string{"WHERE DO YOUR AGENTS RUN?", "this machine", "another machine"} {
		if !strings.Contains(out, want) {
			t.Fatalf("view lacks %q:\n%s", want, out)
		}
	}
	answered := false
	r.sh.opt.Answered = func() { answered = true }
	r.key(t, "enter")
	if r.sh.layer != layerConsole || !slices.Equal(r.opened, []string{""}) || !answered {
		t.Fatalf("this machine: layer %v, opened %q, answered %v", r.sh.layer, r.opened, answered)
	}
	r.key(t, "esc")
	if out := r.view(); strings.Contains(out, "WHERE DO YOUR AGENTS RUN?") || !strings.Contains(out, "+ add a host") {
		t.Fatalf("after the answer, Hosts is the plain list:\n%s", out)
	}
}

func TestRemoveAHostNeedsAY(t *testing.T) {
	removed := ""
	r := newShell(t, []string{"ws-1"}, nil, "", false)
	r.sh.opt.Remove = func(h string) error { removed = h; return nil }
	r.toHosts(t)
	r.key(t, "down")
	r.key(t, "x")
	if removed != "" || !strings.Contains(r.view(), "╭─ Remove") || !strings.Contains(r.view(), "remove ws-1") {
		t.Fatalf("x must ask first, in a box:\n%s", r.view())
	}
	r.key(t, "y")
	if removed != "ws-1" {
		t.Fatalf("removed %q", removed)
	}
}

func TestOldConsoleMessagesDoNotReachTheNewOne(t *testing.T) {
	r := newShell(t, []string{"ws-1", "ws-2"}, nil, "ws-1", false)
	old := r.conns["ws-1"]
	r.key(t, "esc")
	r.key(t, "down")  // from ws-1 to ws-2
	r.key(t, "enter") // the ticked hosts: a new console, this machine its home
	if r.curHostIs(t) != "" || r.conns["ws-1"] == old {
		t.Fatalf("opened %q", r.opened)
	}
	next, _ := r.sh.Update(fromConn{old, stateMsg(fixtureState())})
	r.sh = next.(Shell)
	if r.sh.cur.have {
		t.Fatal("a state of ws-1 reached the console of ws-2")
	}
	redials := 0
	r.sh.cur.opt.Redial = func() (transport.Conn, error) { redials++; return newFake(), nil }
	next, cmd := r.sh.Update(redialMsg{})
	r.sh = next.(Shell)
	if cmd != nil {
		cmd()
	}
	if redials != 0 {
		t.Fatal("a redial tick of the closed console dialed again")
	}
}

func (r *shellRig) curHostIs(t *testing.T) string {
	t.Helper()
	if r.sh.layer != layerConsole {
		t.Fatal("no console open")
	}
	return r.sh.curHost
}

// A console that was redialing when the builder left it must not hand its
// late connection, or its failure, to the console opened next.
func TestALateRedialOfAClosedConsoleIsDropped(t *testing.T) {
	r := newShell(t, []string{"ws-1"}, nil, "ws-1", false)
	gen := r.sh.cur.opt.Gen
	r.key(t, "esc")
	r.key(t, "enter") // the ticked hosts: a new console, this machine its home
	if r.sh.layer != layerConsole || r.sh.cur.opt.Gen == gen {
		t.Fatalf("no new console: layer %v, gen %d", r.sh.layer, r.sh.cur.opt.Gen)
	}
	r.key(t, "esc")
	late := newFake()
	next, _ := r.sh.Update(connMsg{conn: late, gen: gen}) // on Hosts
	r.sh = next.(Shell)
	if !late.closed {
		t.Fatal("a late connection that reached Hosts was left open")
	}
	r.key(t, "enter")
	cur := r.sh.cur.conn
	late = newFake()
	next, _ = r.sh.Update(connMsg{conn: late, gen: gen})
	r.sh = next.(Shell)
	if r.sh.cur.conn != cur || !late.closed {
		t.Fatalf("the old console's connection replaced the new one's (closed %v)", late.closed)
	}
	next, _ = r.sh.Update(redialFailed{err: remote.ErrNotConnected, gen: gen})
	r.sh = next.(Shell)
	if r.sh.cur.blocked != nil || r.sh.cur.lost {
		t.Fatal("the old console's failure reached the new one")
	}
}

// A login check of a closed console must not land on the console of the
// same host opened next: no false flash, no early jump, and its own check
// stays cancellable.
func TestALateLoginCheckOfAClosedConsoleIsDropped(t *testing.T) {
	r := newShell(t, []string{"ws-1"}, nil, "ws-1", false)
	gen := r.sh.cur.opt.Gen
	r.key(t, "esc")
	r.key(t, "enter") // ws-1, still selected: a new console
	if r.sh.layer != layerConsole || r.sh.cur.opt.Gen == gen {
		t.Fatalf("no new console: layer %v, gen %d", r.sh.layer, r.sh.cur.opt.Gen)
	}
	stopped := false
	r.sh.cur.setLogin("ws-1", hostLogin{checking: true, checkStop: func() { stopped = true }, jumpLater: &action{}, login: remote.LoginPassword})
	r.sh.cur.flash = "checking"
	next, cmd := r.sh.Update(loginMsg{host: "ws-1", gen: gen, login: remote.LoginUnreachable, detail: "timed out"})
	r.sh = next.(Shell)
	c := r.sh.cur.loginOf("ws-1")
	if cmd != nil || !c.checking || c.checkStop == nil || c.jumpLater == nil || r.sh.cur.flash != "checking" || c.login != remote.LoginPassword {
		t.Fatalf("the old check reached the new console: checking %v, stop %v, waits %v, flash %q, login %v", c.checking, c.checkStop != nil, c.jumpLater != nil, r.sh.cur.flash, c.login)
	}
	c.checkStop()
	if !stopped {
		t.Fatal("the new console's check cannot be cancelled")
	}
}

// A long ~/.ssh/config fills more rows than the screen has: the list
// follows the selection, so the last row is reachable.
func TestALongHostListScrolls(t *testing.T) {
	var many []string
	for i := range 40 {
		many = append(many, fmt.Sprintf("ws-%02d", i))
	}
	r := newShell(t, nil, many, "", true)
	next, _ := r.sh.Update(tea.WindowSizeMsg{Width: 100, Height: 12})
	r.sh = next.(Shell)
	r.key(t, "down")
	r.key(t, "enter") // another machine
	for range 40 {
		r.key(t, "down")
	}
	out := r.view()
	if !strings.Contains(out, "▌ type a host…") || len(strings.Split(out, "\n")) > 12 {
		t.Fatalf("the last row is off the screen:\n%s", out)
	}
	r.key(t, "enter")
	if r.sh.input == nil {
		t.Fatal("enter on the last row did not start typing")
	}
}

// TestTailscaleHostRowWarns: a host whose check met Tailscale SSH keeps
// that on its Hosts row: the stream is not gated there (#185). The secure
// fix comes first; the selected row shows the whole message.
func TestTailscaleHostRowWarns(t *testing.T) {
	r := newShell(t, []string{"ws-1", "ws-2"}, nil, "", false)
	r.sh.opt.TailscaleGate = setup.TailscaleGateMessage
	r.sh, _ = r.sh.open("ws-1", false)
	next, _ := r.sh.Update(loginMsg{host: "ws-1", gen: r.sh.cur.opt.Gen, login: remote.LoginOK, seen: true, tailscale: true})
	r.sh = next.(Shell).back()
	rows := r.sh.rows()
	if !rows[1].tailscale || rows[2].tailscale {
		t.Fatalf("rows %+v", rows)
	}
	lines := strings.Split(r.view(), "\n")
	var row string
	for _, l := range lines {
		if strings.Contains(l, "ws-1") {
			row = l
			break
		}
	}
	if !strings.Contains(row, "tailscale set --ssh=false") {
		t.Fatalf("the ws-1 row does not warn:\n%s", r.view())
	}
	// ws-1 is selected (back from its console): the whole message shows.
	flat := strings.Join(strings.Fields(strings.ReplaceAll(r.view(), " ! ", " ")), " ")
	if !strings.Contains(flat, setup.TailscaleGateMessage) || !strings.Contains(flat, "docs/remote.md#tailscale-ssh") {
		t.Fatalf("no #185 message:\n%s", r.view())
	}
	if i, j := strings.Index(setup.TailscaleGateMessage, "tailscale set --ssh=false"), strings.Index(setup.TailscaleGateMessage, "or connect"); i < 0 || i > j {
		t.Fatalf("the secure fix is not first: %q", setup.TailscaleGateMessage)
	}
	r.key(t, "down")
	if flat := strings.Join(strings.Fields(strings.ReplaceAll(r.view(), " ! ", " ")), " "); strings.Contains(flat, setup.TailscaleGateMessage) {
		t.Fatal("the message shows on another host's row")
	}
	// The console of ws-1 opens knowing its kind.
	r.sh, _ = r.sh.open("ws-1", false)
	if !homeLogin(r.sh.cur).tailscale {
		t.Fatal("the console of ws-1 forgot its kind")
	}
}

// rowOf is the Hosts line that names host.
func (r *shellRig) rowOf(host string) string {
	for _, l := range strings.Split(r.view(), "\n") {
		if strings.Contains(l, " "+host+" ") || strings.HasSuffix(l, " "+host) {
			return l
		}
	}
	return ""
}

// toHosts goes from the console back to Hosts.
func (r *shellRig) toHosts(t *testing.T) {
	t.Helper()
	r.key(t, "esc")
	if r.sh.layer != layerHosts {
		t.Fatal("esc did not go back to Hosts")
	}
}

func (r *shellRig) update(msg tea.Msg) tea.Cmd {
	next, cmd := r.sh.Update(msg)
	r.sh = next.(Shell)
	return cmd
}

func peerHosts(m Model) []string {
	var out []string
	for _, l := range m.peers {
		out = append(out, l.host)
	}
	return out
}

func TestSpaceHidesAndShows(t *testing.T) {
	r := newShell(t, []string{"ws-1", "gpu-1"}, nil, "", false)
	r.toHosts(t)
	if row := r.rowOf("gpu-1"); !strings.Contains(row, "● gpu-1") {
		t.Fatalf("gpu-1 is not ticked: %q", row)
	}
	r.key(t, "down")
	r.key(t, "down")
	r.key(t, "space")
	if !slices.Equal(r.hidden, []string{"gpu-1"}) {
		t.Fatalf("space wrote %q", r.hidden)
	}
	if row := r.rowOf("gpu-1"); !strings.Contains(row, "○ gpu-1") || !strings.HasSuffix(strings.TrimSpace(row), "hidden") {
		t.Fatalf("the hidden row: %q", row)
	}
	if !strings.Contains(r.view(), "HOSTS · 3 · 2 SHOWN") || !strings.Contains(r.view(), "show/hide space  open ⏎  connect c  quit q") {
		t.Fatalf("label or key line:\n%s", r.view())
	}
	r.key(t, "space")
	if len(r.hidden) != 0 || !strings.Contains(r.rowOf("gpu-1"), "● gpu-1") {
		t.Fatalf("the second space: hidden %q, row %q", r.hidden, r.rowOf("gpu-1"))
	}
}

// The note of a ticked host is live: its stream runs while Hosts shows.
func TestHostsNotesAreLive(t *testing.T) {
	r := newShell(t, []string{"ws-1"}, nil, "", false)
	r.toHosts(t)
	ws := hostState(sess(2, "api-auth", "%1", nil), sess(3, "docs", "%2", nil))
	ws.Queue = []queue.Item{{Pane: "%1", Name: "api-auth", State: queue.StateQuestion, Since: time.Now().Add(-time.Minute)}}
	r.update(peerMsg{host: "ws-1", conn: r.conns["ws-1"], msg: stateMsg(ws)})
	if row := r.rowOf("ws-1"); !strings.HasSuffix(strings.TrimSpace(row), "2 sessions · 1 waits") {
		t.Fatalf("the ws-1 note: %q", row)
	}
}

func TestEnterOpensTheTickedHosts(t *testing.T) {
	r := hidingShell(t, []string{"ws-1", "gpu-1"}, []string{"gpu-1"}, "gpu-1")
	r.toHosts(t)
	r.key(t, "enter")
	if r.sh.layer != layerConsole || r.sh.cur.opt.Host != "" || !slices.Equal(peerHosts(r.sh.cur), []string{"ws-1"}) {
		t.Fatalf("layer %v, home %q, peers %q", r.sh.layer, r.sh.cur.opt.Host, peerHosts(r.sh.cur))
	}
	if !slices.Equal(r.opened, []string{"gpu-1", "", "ws-1"}) || !r.conns["gpu-1"].closed {
		t.Fatalf("opened %q, gpu-1 closed %v", r.opened, r.conns["gpu-1"].closed)
	}
}

func TestEscKeepsTheStreams(t *testing.T) {
	r := newShell(t, []string{"ws-1"}, nil, "", false)
	r.update(stateMsg(fixtureState()))
	r.key(t, "down")
	gen, sel := r.sh.cur.opt.Gen, r.sh.cur.selPID
	r.toHosts(t)
	r.key(t, "enter")
	if r.sh.layer != layerConsole || r.sh.cur.opt.Gen != gen {
		t.Fatalf("layer %v, gen %d, was %d", r.sh.layer, r.sh.cur.opt.Gen, gen)
	}
	if !slices.Equal(r.opened, []string{"", "ws-1"}) || r.conns[""].closed || r.conns["ws-1"].closed {
		t.Fatalf("opened %q, closed home %v ws-1 %v", r.opened, r.conns[""].closed, r.conns["ws-1"].closed)
	}
	if r.sh.cur.selPID != sel || sel == 0 {
		t.Fatalf("selection %d, was %d", r.sh.cur.selPID, sel)
	}
}

// Review Focus 3: a hidden peer's late redial tick reopens nothing, and its
// stale tick draws nothing. A host shown again is a new link: the old tick
// is not its own.
func TestHidingAPeerClosesItAndItsTick(t *testing.T) {
	r := newShell(t, []string{"ws-1"}, nil, "", false)
	r.update(stateMsg(fixtureState()))
	ws := r.conns["ws-1"]
	r.update(peerMsg{host: "ws-1", conn: ws, msg: stateMsg(hostState(sess(2, "api-auth", "%1", nil)))})
	r.update(peerMsg{host: "ws-1", conn: ws, msg: lostMsg{err: io.EOF, conn: ws}})
	late := peerRedialMsg{host: "ws-1", gen: r.sh.cur.opt.Gen, id: r.sh.cur.peers[0].id}
	r.toHosts(t)
	r.key(t, "down")
	r.key(t, "space")
	r.key(t, "enter")
	if len(r.sh.cur.peers) != 0 || !ws.closed {
		t.Fatalf("peers %q, ws-1 closed %v", peerHosts(r.sh.cur), ws.closed)
	}
	if strings.Contains(r.view(), "api-auth") {
		t.Fatalf("a hidden host's rows stay:\n%s", r.view())
	}
	if cmd := r.update(late); cmd != nil {
		cmd()
	}
	before := r.view()
	r.update(staleMsg{})
	if r.redials["ws-1"] != 0 || r.view() != before {
		t.Fatalf("the late ticks: %d redials, frame changed %v", r.redials["ws-1"], r.view() != before)
	}
	// Shown again: a new stream, lost too; only its own tick dials.
	r.toHosts(t)
	r.key(t, "down")
	r.key(t, "space")
	r.key(t, "enter")
	again := r.conns["ws-1"]
	if again == ws || len(r.sh.cur.peers) != 1 {
		t.Fatalf("ws-1 shown again: peers %q", peerHosts(r.sh.cur))
	}
	r.update(peerMsg{host: "ws-1", conn: again, msg: lostMsg{err: io.EOF, conn: again}})
	if cmd := r.update(late); cmd != nil {
		cmd()
	}
	if r.redials["ws-1"] != 0 {
		t.Fatal("the old link's tick dialed the new one")
	}
	own := peerRedialMsg{host: "ws-1", gen: r.sh.cur.opt.Gen, id: r.sh.cur.peers[0].id}
	if cmd := r.update(own); cmd == nil || cmd() == nil || r.redials["ws-1"] != 1 {
		t.Fatalf("its own tick: %d redials", r.redials["ws-1"])
	}
}

func TestHeaderCountsHidden(t *testing.T) {
	r := hidingShell(t, []string{"ws-1", "gpu-1"}, []string{"gpu-1"}, "")
	r.update(stateMsg(fixtureState()))
	r.sh.cur.splashDone = true
	if head := strings.Split(r.view(), "\n")[0]; !strings.Contains(head, "2 hosts · 1 hidden") {
		t.Fatalf("header %q", head)
	}
	one := newShell(t, nil, nil, "", false)
	one.update(stateMsg(fixtureState()))
	one.sh.cur.splashDone = true
	if head := strings.Split(one.view(), "\n")[0]; strings.Contains(head, "hidden") {
		t.Fatalf("one host: header %q", head)
	}
}

// R8: matchblox with added hosts opens the ticked hosts; matchblox ws-1
// opens ws-1 alone.
func TestStartOpensTheTickedHosts(t *testing.T) {
	r := hidingShell(t, []string{"ws-1", "gpu-1"}, []string{"gpu-1"}, "")
	if r.sh.layer != layerConsole || r.sh.cur.opt.Host != "" || !slices.Equal(r.opened, []string{"", "ws-1"}) || !slices.Equal(peerHosts(r.sh.cur), []string{"ws-1"}) {
		t.Fatalf("layer %v, opened %q, peers %q", r.sh.layer, r.opened, peerHosts(r.sh.cur))
	}
	if r.sh.cur.splashDone {
		t.Fatal("the first console of a run plays its start")
	}
	one := hidingShell(t, []string{"ws-1", "gpu-1"}, nil, "ws-1")
	if !slices.Equal(one.opened, []string{"ws-1"}) || len(one.sh.cur.peers) != 0 {
		t.Fatalf("matchblox ws-1: opened %q, peers %q", one.opened, peerHosts(one.sh.cur))
	}
	none := hidingShell(t, []string{"ws-1"}, []string{"this machine", "ws-1"}, "")
	if none.sh.layer != layerHosts || len(none.opened) != 0 {
		t.Fatalf("nothing ticked: layer %v, opened %q", none.sh.layer, none.opened)
	}
}

// Back to Hosts keeps the kind every host's check found, peers' too.
func TestBackKeepsEveryHostsKind(t *testing.T) {
	r := newShell(t, []string{"ws-1", "ws-2"}, nil, "", false)
	r.update(loginMsg{host: "ws-2", gen: r.sh.cur.opt.Gen, login: remote.LoginOK, seen: true, tailscale: true})
	r.toHosts(t)
	if rows := r.sh.rows(); rows[1].tailscale || !rows[2].tailscale {
		t.Fatalf("rows %+v", rows)
	}
}

// within fails the test when f takes a second or more.
func within(t *testing.T, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { f(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("%s waited for a slow host's dial", what)
	}
}

// I1: a peer's dial runs in the background. A host that takes long to
// answer holds neither the start, nor Enter, nor this machine's rows.
func TestASlowHostNeverStallsTheConsole(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	conns := map[string]*fakeConn{"": newFake(), "ws-1": newFake(), "gpu-1": newFake()}
	added := []string{"ws-1"}
	opt := ShellOptions{
		Base:  Options{NoMotion: true},
		Hosts: func() []string { return slices.Clone(added) },
		Open: func(host string) Options {
			if host != "" {
				<-release // the ssh of a sleeping host
			}
			return Options{Conn: conns[host], Host: host}
		},
	}
	var sh Shell
	within(t, "NewShell", func() { sh = NewShell(opt) })
	if sh.layer != layerConsole || len(sh.cur.peers) != 1 || !sh.cur.peers[0].redialing {
		t.Fatalf("layer %v, peers %+v", sh.layer, sh.cur.peers)
	}
	dial := sh.cur.dialPeer(0)
	var next tea.Model = sh
	within(t, "the first frame", func() {
		next, _ = next.Update(tea.WindowSizeMsg{Width: 132, Height: 30})
		next, _ = next.Update(stateMsg(hostState(sess(2, "home-run", "%2", nil))))
		_ = next.(Shell).render()
	})
	s := next.(Shell)
	if out := ansi.Strip(s.render()); !strings.Contains(out, "home-run") {
		t.Fatalf("this machine's rows do not show:\n%s", out)
	}
	if why := s.cur.refusal("ws-1"); why != "ws-1 is not connected." {
		t.Fatalf("an act to ws-1 while it dials: %q", why)
	}
	s.cur.pending = &action{label: "go", say: "go", rec: "go:%2", which: "primary"}
	next = pressAll(s, "enter")
	if got := conns[""].acts(); len(got) != 1 {
		t.Fatalf("this machine got %+v while ws-1 dials", got)
	}
	// Enter with a new host ticked: gpu-1 dials in the background too.
	added = append(added, "gpu-1")
	next, _ = next.Update(layerMsg{}) // esc
	within(t, "Enter", func() { next, _ = key(next, "enter") })
	s = next.(Shell)
	if s.layer != layerConsole || len(s.cur.peers) != 2 || !s.cur.peers[1].redialing {
		t.Fatalf("after Enter: layer %v, peers %+v", s.layer, s.cur.peers)
	}
	// The dial ends: ws-1's rows join.
	got := make(chan tea.Msg, 1)
	go func() { got <- dial() }()
	release <- struct{}{}
	next, _ = s.Update(<-got)
	next, _ = next.Update(peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(hostState(sess(3, "ws-run", "%3", nil)))})
	if out := ansi.Strip(next.(Shell).render()); !strings.Contains(out, "ws-run") || !strings.Contains(out, "home-run") {
		t.Fatalf("ws-1's rows did not join:\n%s", out)
	}
}

// I2: a connect keeps the ticks: the console that opens after it is the
// ticked hosts, the connected one among them.
func TestAConnectKeepsTheTicks(t *testing.T) {
	r := newShell(t, []string{"ws-1", "gpu-1"}, []string{"ws-1", "gpu-1", "ws-2"}, "", false)
	r.toHosts(t)
	r.key(t, "down") // ws-1
	r.key(t, "c")
	r.key(t, "y")
	if r.sh.layer != layerConsole || r.sh.cur.opt.Host != "" || !slices.Equal(peerHosts(r.sh.cur), []string{"ws-1", "gpu-1"}) {
		t.Fatalf("c on ws-1: layer %v, home %q, peers %q", r.sh.layer, r.sh.cur.opt.Host, peerHosts(r.sh.cur))
	}
	r.toHosts(t)
	r.key(t, "down")
	r.key(t, "down")
	r.key(t, "down")
	r.key(t, "enter") // + add a host
	r.key(t, "enter") // ws-2
	r.key(t, "y")
	if !slices.Equal(peerHosts(r.sh.cur), []string{"ws-1", "gpu-1", "ws-2"}) {
		t.Fatalf("ws-2 added: peers %q", peerHosts(r.sh.cur))
	}
}

// S1: with no host ticked, Enter opens the selected host alone, says so,
// and the header still counts the hidden.
func TestEnterWithNoneTickedSaysSo(t *testing.T) {
	r := hidingShell(t, []string{"ws-1"}, []string{"this machine", "ws-1"}, "")
	r.key(t, "down") // ws-1
	r.key(t, "enter")
	if r.sh.layer != layerConsole || r.sh.curHost != "ws-1" || len(r.sh.cur.peers) != 0 {
		t.Fatalf("layer %v, host %q", r.sh.layer, r.sh.curHost)
	}
	r.update(stateMsg(fixtureState()))
	r.sh.cur.splashDone = true
	lines := strings.Split(r.view(), "\n")
	if !strings.Contains(lines[0], "ws-1 · 1 hidden") {
		t.Fatalf("header %q", lines[0])
	}
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "No host is ticked: ws-1 opens alone." {
		t.Fatalf("key line %q", last)
	}
}
