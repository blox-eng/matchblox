package app

import (
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/transport"
)

// lostAt is when a peer is lost in these tests: 14:02.
var lostAt = time.Date(2026, 10, 29, 14, 2, 0, 0, time.UTC)

// hostsAt is a console on home with the peers given, w×h, its clock at
// *now. Every peer redials.
func hostsAt(t *testing.T, now *time.Time, home string, w, h int, peers ...string) (Model, *fakeConn, map[string]*fakeConn) {
	t.Helper()
	homeConn, conns := newFake(), map[string]*fakeConn{}
	var ps []Peer
	for _, p := range peers {
		conns[p] = newFake()
		ps = append(ps, Peer{Host: p, Conn: conns[p], Redial: func() (transport.Conn, error) { return newFake(), nil }})
	}
	m := New(Options{NoMotion: true, Host: home, Conn: homeConn, Peers: ps, Now: func() time.Time { return *now }})
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(Model), homeConn, conns
}

// stateAt is st sampled at at.
func stateAt(st proto.State, at time.Time) proto.State {
	st.At = at
	return st
}

func upd(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

func frameOf(m Model) []string {
	return strings.Split(ansi.Strip(m.render()), "\n")
}

// lineWith is the first line of the frame with s, or "".
func lineWith(lines []string, s string) string {
	for _, l := range lines {
		if strings.Contains(ansi.Strip(l), s) {
			return l
		}
	}
	return ""
}

const hosts1 = "this machine"

func TestHeaderSaysThreeHosts(t *testing.T) {
	t.Skip("board rule: TestANoticeNeverHidesTheCount")
	now := lostAt
	m, _, conns := hostsAt(t, &now, "", 196, 30, "gpu-1", "ws-1")
	m = upd(t, m, stateMsg(hostState(sess(1, "api-auth", "%1", nil))))
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(hostState(sess(2, "docs-sweep", "%2", nil)))})
	if h := frameOf(m)[0]; !strings.HasPrefix(h, " ▰ matchblox · 3 hosts ") {
		t.Fatalf("the header is %q, want it to say 3 hosts", h)
	}
}

// lostPeer is a console of this machine and ws-1 whose peer sent its
// state at lostAt and was lost then; the clock is at lostAt+after, and
// home sampled then.
func lostPeer(t *testing.T, after time.Duration) Model {
	t.Helper()
	now := lostAt
	m, _, conns := hostsAt(t, &now, "", 196, 30, "ws-1")
	home := hostState(sess(1, "api-auth", "%1", nil))
	m = upd(t, m, stateMsg(stateAt(home, now)))
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(hostState(sess(2, "train-loop", "%2", nil)))})
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: lostMsg{err: io.EOF, conn: conns["ws-1"]}})
	now = lostAt.Add(after)
	return upd(t, m, stateMsg(stateAt(home, now)))
}

func TestTwoLostHostsLine(t *testing.T) {
	now := lostAt
	m, _, conns := hostsAt(t, &now, "", 196, 30, "gpu-1", "ws-1")
	home := hostState(sess(1, "api-auth", "%1", nil))
	m = upd(t, m, stateMsg(stateAt(home, now)))
	for i, p := range []string{"gpu-1", "ws-1"} {
		m = upd(t, m, peerMsg{host: p, conn: conns[p], msg: stateMsg(hostState(sess(2+i, p+"-job", "%2", nil)))})
	}
	m = upd(t, m, peerMsg{host: "gpu-1", conn: conns["gpu-1"], msg: lostMsg{err: io.EOF, conn: conns["gpu-1"]}})
	now = lostAt.Add(40 * time.Second)
	m.peers[0].backoff = 8 * time.Second
	m = upd(t, m, stateMsg(stateAt(home, now)))
	if got, want := m.lostLine(), "gpu-1: connection lost. Tries again in 8s."; got != want {
		t.Fatalf("one lost host: %q, want %q", got, want)
	}
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: lostMsg{err: io.EOF, conn: conns["ws-1"]}})
	now = lostAt.Add(80 * time.Second)
	m.peers[1].backoff = 10 * time.Second
	m = upd(t, m, stateMsg(stateAt(home, now)))
	want := "2 hosts lost: gpu-1, ws-1. Tries again in 8s."
	if got := m.lostLine(); got != want {
		t.Fatalf("two lost hosts: %q, want %q", got, want)
	}
	f := frameOf(m)
	if got := strings.TrimSpace(f[len(f)-1]); got != want {
		t.Fatalf("the key line is %q, want %q", got, want)
	}
}

func TestOneHostStaleIsToday(t *testing.T) {
	now := lostAt
	m, home, _ := hostsAt(t, &now, "", 196, 30)
	m = upd(t, m, stateMsg(hostState(sess(1, "api-auth", "%1", nil))))
	m = upd(t, m, lostMsg{err: io.EOF, conn: home})
	if h := frameOf(m)[0]; !strings.Contains(h, "matchblox · stale") {
		t.Fatalf("the header is %q, want stale at once", h)
	}
}

// TestThreeHostFrames: the golden frames of three hosts show what they
// are for.
func TestThreeHostFrames(t *testing.T) {
	st := screensState(t)
	for _, c := range []struct {
		gpu  string
		w, h int
		want []string
	}{
		// On the board (Task 12 step 1): the header names the host filter, a
		// card another host's, a host that is not reached is a line over the
		// columns in the service's own words (ruling 30).
		{"fine", 86, 30, []string{"matchblox · all hosts ▾", "docs-sweep…"}},
		{"fine", 132, 32, []string{"docs-sweep @ws-1"}},
		{"stale", 86, 30, []string{"· gpu-1 stale", "gpu-1: not reached since 22:49 · ⏎ try again", "gpu-1: connection lost. Tries again in 8s."}},
		{"stale", 132, 50, []string{"gpu-1: not reached since 22:49"}},
		{"blocked", 86, 30, []string{"gpu-1: is not connected · ⏎ connect"}},
	} {
		all := strings.Join(frameOf(threeHosts(t, st, c.w, c.h, c.gpu)), "\n")
		for _, w := range c.want {
			if !strings.Contains(all, w) {
				t.Errorf("%s at %d: no %q in the frame", c.gpu, c.w, w)
			}
		}
	}
}

// TestShortHomeLossStaysQuiet: with a peer, a home loss and reconnect
// under 30 s leaves the frame as it was (anti-goal 5).
func TestShortHomeLossStaysQuiet(t *testing.T) {
	t.Skip("board rule: TestTheBoardsStates")
	now := lostAt
	m, home, conns := hostsAt(t, &now, "", 196, 12, "ws-1")
	m = upd(t, m, stateMsg(stateAt(hostState(sess(1, "api", "%1", nil)), now)))
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(hostState(sess(2, "tl", "%2", nil)))})
	before := strings.Join(frameOf(m), "\n")
	m = upd(t, m, lostMsg{err: io.EOF, conn: home})
	m = upd(t, m, connMsg{conn: newFake()})
	if got := strings.Join(frameOf(m), "\n"); got != before {
		t.Fatalf("a short home loss changed the frame:\n%s\nwant:\n%s", got, before)
	}
}

// TestPeerStartWatchesForSilence: a peer's start ticks once silentAfter
// passes, with its conn.
func TestPeerStartWatchesForSilence(t *testing.T) {
	now := lostAt
	m, _, conns := hostsAt(t, &now, "", 196, 30, "ws-1")
	defer conns["ws-1"].Close() // ends the read the start began
	batch, ok := m.peerStart(0)().(tea.BatchMsg)
	if !ok {
		t.Fatal("the peer's start is not a batch")
	}
	got := make(chan tea.Msg, len(batch))
	for _, c := range batch {
		go func() { got <- c() }()
	}
	deadline := time.After(silentAfter + 2*time.Second)
	for {
		select {
		case msg := <-got:
			if pm, ok := msg.(peerMsg); ok {
				if s, ok := pm.msg.(silentMsg); ok {
					if pm.conn != conns["ws-1"] || s.conn != conns["ws-1"] {
						t.Fatalf("the tick names conn %v", pm.conn)
					}
					return
				}
			}
		case <-deadline:
			t.Fatal("no silence tick for the peer")
		}
	}
}

// The lost host's line outranks the stand-in's key line: the keys may not go.
func TestTheLostLineOutranksTheStandInsKeys(t *testing.T) {
	m := lostPeer(t, 40*time.Second)
	m.standIn, m.flash = true, ""
	want := m.lostLine()
	if line, _ := m.keyLine(m.width); want == "" || !strings.Contains(ansi.Strip(line), want[:20]) {
		t.Fatalf("key line %q, want the lost line %q", ansi.Strip(line), want)
	}
}

// releaseRig is Hosts over this machine and ws-1, ws-1 selected: the
// console runs console, ws-1 hostRel with its update view (nil: a host
// older than the update act).
func releaseRig(t *testing.T, console, hostRel string, view *state.UpdateView) *shellRig {
	t.Helper()
	r := newShell(t, []string{"ws-1"}, nil, "", false)
	r.sh.opt.Base.Binary, r.sh.cur.opt.Binary, r.sh.cur.opt.Local = console, console, true
	ws := r.conns["ws-1"]
	r.update(peerMsg{host: "ws-1", conn: ws, msg: helloMsg(proto.Hello{Version: proto.Version, Binary: hostRel})})
	st := hostState(sess(2, "api-auth", "%1", nil))
	st.Update = view
	r.update(peerMsg{host: "ws-1", conn: ws, msg: stateMsg(st)})
	r.toHosts(t)
	r.key(t, "down")
	return r
}

func viewOn(current string) *state.UpdateView {
	return &state.UpdateView{Mode: "offer", Current: current}
}

func TestHostRowNamesBothReleases(t *testing.T) {
	r := releaseRig(t, "v0.42.0", "v0.38.1", viewOn("v0.38.1"))
	if row := r.rowOf("ws-1"); !strings.HasSuffix(strings.TrimSpace(row), "1 sessions · v0.38.1, behind this console (v0.42.0)") {
		t.Fatalf("the ws-1 row: %q", row)
	}
	if row := r.rowOf("this machine"); strings.Contains(row, "this console") {
		t.Fatalf("this machine's row: %q", row)
	}
}

func TestHostRowSilentWhenEqual(t *testing.T) {
	r := releaseRig(t, "v0.42.0", "v0.42.0", viewOn("v0.42.0"))
	if row := r.rowOf("ws-1"); !strings.HasSuffix(strings.TrimSpace(row), "1 sessions") {
		t.Fatalf("the ws-1 row: %q", row)
	}
	if out := r.view(); strings.Contains(out, "matchblox update") || strings.Contains(out, "update ws-1") {
		t.Fatalf("an offer for a host on the same release:\n%s", out)
	}
}

func TestHostRowSilentForADevBuild(t *testing.T) {
	for _, c := range [][2]string{{"dev", "v0.38.1"}, {"v0.42.0", "dev"}, {"v0.42.0", "v0.42.1-0.20261112093000-f15b80c1abcd"}} {
		r := releaseRig(t, c[0], c[1], viewOn(c[1]))
		if row := r.rowOf("ws-1"); !strings.HasSuffix(strings.TrimSpace(row), "1 sessions") {
			t.Errorf("console %s, ws-1 %s: the row %q", c[0], c[1], row)
		}
	}
}

func TestHostUpdateKeySendsTheActToThatHost(t *testing.T) {
	r := releaseRig(t, "v0.42.0", "v0.38.1", viewOn("v0.38.1"))
	if out := r.view(); !strings.Contains(out, "u update ws-1") {
		t.Fatalf("no key for the update:\n%s", out)
	}
	ws, home := r.conns["ws-1"], r.conns[""]
	r.key(t, "u")
	r.key(t, "enter") // not a typed y
	if acts := ws.acts(); len(acts) != 0 {
		t.Fatalf("an act with no typed y: %+v", acts)
	}
	r.key(t, "u")
	if out := r.view(); !strings.Contains(out, "Update ws-1") {
		t.Fatalf("no confirm:\n%s", out)
	}
	r.key(t, "y")
	want := proto.Act{RecID: proto.HostUpdateAct, Confirm: "y"}
	if acts := ws.acts(); len(acts) != 1 || acts[0] != want {
		t.Fatalf("ws-1 got %+v", acts)
	}
	for _, a := range home.acts() {
		if a.RecID == proto.HostUpdateAct {
			t.Fatalf("home got the update: %+v", a)
		}
	}
	ws.mu.Lock()
	id := ws.sent[len(ws.sent)-1].ID
	ws.mu.Unlock()
	r.update(peerMsg{host: "ws-1", conn: ws, msg: resultMsg(proto.Result{ActID: id, Skipped: []string{"v0.38.1 is the latest release"}})})
	if out := r.view(); !strings.Contains(out, "v0.38.1 is the latest release") {
		t.Fatalf("Hosts does not say how it went:\n%s", out)
	}
}

func TestOldHostGetsTheCommandNotTheKey(t *testing.T) {
	r := releaseRig(t, "v0.42.0", "v0.38.1", nil)
	out := r.view()
	if !strings.Contains(out, "on ws-1: matchblox update") || strings.Contains(out, "u update") {
		t.Fatalf("an old host's fix:\n%s", out)
	}
	r.key(t, "u")
	if r.sh.pending != nil || len(r.conns["ws-1"].acts()) != 0 {
		t.Fatalf("u on an old host: pending %+v, acts %+v", r.sh.pending, r.conns["ws-1"].acts())
	}
}

func TestNewerHostPointsAtThisMachine(t *testing.T) {
	r := releaseRig(t, "v0.42.0", "v0.43.0", viewOn("v0.43.0"))
	out := r.view()
	if !strings.Contains(r.rowOf("ws-1"), "v0.43.0, ahead of this console (v0.42.0)") || !strings.Contains(out, "this console is older: run matchblox update") {
		t.Fatalf("a newer host:\n%s", out)
	}
	r.key(t, "u")
	if r.sh.pending != nil {
		t.Fatalf("u on a newer host: %+v", r.sh.pending)
	}
}

// A console whose home is a remote host has no service of its own to ask:
// the home host's latest release tells it, and it gets the command.
func TestRemoteHomeConsoleGetsTheCommand(t *testing.T) {
	behind := &state.UpdateView{Mode: "offer", Current: "v0.43.0", Latest: "v0.43.0"}
	for _, c := range []struct {
		local   bool
		console string
		say     bool
	}{{false, "v0.41.0", true}, {true, "v0.41.0", false}, {false, "v0.43.0", false}, {false, "dev", false}} {
		conn := newFake()
		m := New(Options{NoMotion: true, Host: "ws-1", Conn: conn, Binary: c.console, Local: c.local})
		m = upd(t, m, tea.WindowSizeMsg{Width: 104, Height: 38})
		st := fixtureState()
		st.Update = behind
		m = upd(t, m, stateMsg(st))
		notice := "run matchblox update · console v0.41.0, latest v0.43.0"
		if got := strings.Contains(strings.Join(frameOf(m), "\n"), notice); got != c.say {
			t.Errorf("local %v, console %s: notice %v, want %v\n%s", c.local, c.console, got, c.say, strings.Join(frameOf(m), "\n"))
		}
		for _, a := range conn.acts() {
			if a.RecID == proto.HostUpdateAct || strings.HasPrefix(a.RecID, "door:") {
				t.Errorf("local %v, console %s: sent %+v", c.local, c.console, a)
			}
		}
	}
}

// What mode auto did shows once for each record, on this console and on
// the next one that starts.
func TestAutoUpdateNewsShowsOnce(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 11, 12, 3, 12, 0, 0, time.Local)
	auto := &state.UpdateView{Mode: "auto", Current: "v0.43.0", Latest: "v0.43.0",
		Last: &state.UpdateRecord{From: "v0.42.0", To: "v0.43.0", At: at, Checksum: true}}
	console := func() Model {
		m := New(Options{NoMotion: true, Host: "ws-1", Conn: newFake(), Binary: "v0.43.0", Local: true, StateDir: dir,
			Now: func() time.Time { return at.Add(time.Hour) }})
		return upd(t, m, tea.WindowSizeMsg{Width: 104, Height: 38})
	}
	stateWith := func(v *state.UpdateView) stateMsg {
		st := fixtureState()
		st.Update = v
		return stateMsg(st)
	}
	m := upd(t, console(), stateWith(auto))
	if out := strings.Join(frameOf(m), "\n"); !strings.Contains(out, "updated to v0.43.0 at 03:12") {
		t.Fatalf("no news of the update:\n%s", out)
	}
	m.flash = ""
	if m = upd(t, m, stateWith(auto)); m.flash != "" {
		t.Fatalf("the news shows again: %q", m.flash)
	}
	if m = upd(t, console(), stateWith(auto)); m.flash != "" {
		t.Fatalf("the next console shows it again: %q", m.flash)
	}

	failed := *auto
	failed.Current, failed.Last = "v0.42.0", &state.UpdateRecord{From: "v0.42.0", To: "v0.43.0", At: at.Add(24 * time.Hour), Err: "no route to host"}
	if m = upd(t, m, stateWith(&failed)); m.flash != "the update to v0.43.0 failed: no route to host · run: matchblox update" {
		t.Fatalf("a failed update: %q", m.flash)
	}

	yesterday := *auto
	yesterday.Last = &state.UpdateRecord{From: "v0.42.0", To: "v0.43.0", At: at.Add(-24 * time.Hour), Checksum: true, Provenance: true}
	if m = upd(t, m, stateWith(&yesterday)); m.flash != "updated to v0.43.0 on Nov 11 at 03:12" {
		t.Fatalf("a record from another day: %q", m.flash)
	}

	offer := *auto
	offer.Mode, offer.Last = "offer", &state.UpdateRecord{From: "v0.42.0", To: "v0.43.0", At: at.Add(48 * time.Hour)}
	if m = upd(t, console(), stateWith(&offer)); m.flash != "" {
		t.Fatalf("a person's own install is news: %q", m.flash)
	}
}

// Auto installs with no person there: when no gh checked the build's
// provenance, the news says so.
func TestUpdateNewsSaysProvenanceWasNotChecked(t *testing.T) {
	at := time.Date(2026, 11, 12, 3, 12, 0, 0, time.Local)
	for _, c := range []struct {
		proven bool
		at     time.Time
		want   string
	}{
		{false, at, "updated to v0.43.0 at 03:12 · provenance not checked"},
		{true, at, "updated to v0.43.0 at 03:12"},
		{false, at.Add(-24 * time.Hour), "updated to v0.43.0 on Nov 11 at 03:12 · provenance not checked"},
	} {
		m := New(Options{NoMotion: true, Host: "ws-1", Conn: newFake(), Binary: "v0.43.0", Local: true, StateDir: t.TempDir(),
			Now: func() time.Time { return at.Add(time.Hour) }})
		m = upd(t, m, tea.WindowSizeMsg{Width: 104, Height: 38})
		st := fixtureState()
		st.Update = &state.UpdateView{Mode: config.UpdateAuto, Current: "v0.43.0", Latest: "v0.43.0",
			Last: &state.UpdateRecord{From: "v0.42.0", To: "v0.43.0", At: c.at, Checksum: true, Provenance: c.proven}}
		if m = upd(t, m, stateMsg(st)); m.flash != c.want {
			t.Errorf("provenance checked %v: the news %q, want %q", c.proven, m.flash, c.want)
		}
	}
}

// The update of a host says when its build's provenance was not checked.
func TestAHostUpdateSaysProvenanceWasNotChecked(t *testing.T) {
	for _, proven := range []bool{false, true} {
		r := releaseRig(t, "v0.42.0", "v0.38.1", viewOn("v0.38.1"))
		ws := r.conns["ws-1"]
		r.key(t, "u")
		r.key(t, "y")
		ws.mu.Lock()
		id := ws.sent[len(ws.sent)-1].ID
		ws.mu.Unlock()
		st := hostState(sess(2, "api-auth", "%1", nil))
		st.Update = viewOn("v0.38.1")
		st.Update.Last = &state.UpdateRecord{From: "v0.38.1", To: "v0.42.0", Checksum: true, Provenance: proven}
		r.update(peerMsg{host: "ws-1", conn: ws, msg: stateMsg(st)})
		r.update(peerMsg{host: "ws-1", conn: ws, msg: resultMsg(proto.Result{ActID: id, Ran: [][]string{{"install", "v0.42.0"}}})})
		want := "ws-1 installed v0.42.0: its service starts again"
		if !proven {
			want += " · provenance not checked"
		}
		if got := r.sh.cur.flash; got != want {
			t.Errorf("provenance checked %v: %q, want %q", proven, got, want)
		}
	}
}

func TestClosedReleaseNoteStaysClosedForThatPair(t *testing.T) {
	r := releaseRig(t, "v0.42.0", "v0.38.1", viewOn("v0.38.1"))
	dir := t.TempDir()
	r.sh.cur.opt.StateDir = dir
	r.key(t, "d")
	if row := r.rowOf("ws-1"); strings.Contains(row, "this console") {
		t.Fatalf("the closed note shows: %q", row)
	}
	if out := r.view(); strings.Contains(out, "u update ws-1") {
		t.Fatalf("the closed note keeps its key:\n%s", out)
	}
	if p := loadRecPrefs(dir); len(p.Releases) != 1 || p.Releases[0] != "ws-1\x00v0.38.1\x00v0.42.0" {
		t.Fatalf("kept %+v", p)
	}
	ws := r.conns["ws-1"]
	r.update(peerMsg{host: "ws-1", conn: ws, msg: helloMsg(proto.Hello{Version: proto.Version, Binary: "v0.39.0"})})
	if row := r.rowOf("ws-1"); !strings.Contains(row, "v0.39.0, behind this console (v0.42.0)") {
		t.Fatalf("a new release of the host: %q", row)
	}
	r.update(peerMsg{host: "ws-1", conn: ws, msg: helloMsg(proto.Hello{Version: proto.Version, Binary: "v0.38.1"})})
	r.sh.cur.opt.Binary = "v0.43.0"
	if row := r.rowOf("ws-1"); !strings.Contains(row, "v0.38.1, behind this console (v0.43.0)") {
		t.Fatalf("a new release of the console: %q", row)
	}
}

// After its update the host's service starts again: the peer redials, and
// the new hello takes the note away, with no stale row.
func TestAnUpdatedHostShowsItsNewRelease(t *testing.T) {
	r := releaseRig(t, "v0.42.0", "v0.38.1", viewOn("v0.38.1"))
	ws := r.conns["ws-1"]
	r.update(peerMsg{host: "ws-1", conn: ws, msg: lostMsg{err: io.EOF, conn: ws}})
	r.drain(r.update(peerRedialMsg{host: "ws-1", gen: r.sh.cur.opt.Gen, id: r.sh.cur.peers[0].id}))
	again := r.sh.cur.peers[0].conn
	if again == nil || again == transport.Conn(ws) {
		t.Fatalf("no redial: %v", again)
	}
	r.update(peerMsg{host: "ws-1", conn: again, msg: helloMsg(proto.Hello{Version: proto.Version, Binary: "v0.42.0"})})
	st := hostState(sess(2, "api-auth", "%1", nil))
	st.Update = viewOn("v0.42.0")
	r.update(peerMsg{host: "ws-1", conn: again, msg: stateMsg(st)})
	if row := r.rowOf("ws-1"); !strings.HasSuffix(strings.TrimSpace(row), "1 sessions") {
		t.Fatalf("the ws-1 row after its update: %q", row)
	}
	if out := r.view(); strings.Contains(out, "lost") || strings.Contains(out, "stale") || strings.Contains(out, "update ws-1") {
		t.Fatalf("Hosts after the update:\n%s", out)
	}
}

// A Homebrew host is updated by Homebrew: its row gives the command, and
// u does nothing.
func TestHomebrewHostGetsItsOwnCommand(t *testing.T) {
	view := viewOn("v0.38.1")
	view.Managed = "brew upgrade matchblox"
	r := releaseRig(t, "v0.42.0", "v0.38.1", view)
	if out := r.view(); !strings.Contains(out, "on ws-1: brew upgrade matchblox") || strings.Contains(out, "u update") {
		t.Fatalf("a Homebrew host's fix:\n%s", out)
	}
	r.key(t, "u")
	if r.sh.pending != nil || len(r.conns["ws-1"].acts()) != 0 {
		t.Fatalf("u on a Homebrew host: pending %+v, acts %+v", r.sh.pending, r.conns["ws-1"].acts())
	}
}

// On a phone the row has no room for its note: one word marks it.
func TestNarrowHostRowSaysWhichSide(t *testing.T) {
	for rel, want := range map[string]string{"v0.38.1": "ws-1 behind", "v0.43.0": "ws-1 ahead", "v0.42.0": "ws-1"} {
		r := releaseRig(t, "v0.42.0", rel, viewOn(rel))
		r.update(tea.WindowSizeMsg{Width: 44, Height: 31})
		r.key(t, "up") // the mark is on the row, selected or not
		if row := strings.TrimSpace(r.rowOf("ws-1")); !strings.HasSuffix(row, want) || rel == "v0.42.0" && strings.HasSuffix(row, "d") {
			t.Errorf("ws-1 on %s: the row %q, want it to end %q", rel, row, want)
		}
	}
}

// How the update of a host went shows on Hosts in the host's name: the
// reason of one that failed, and the release of one that worked.
func TestHostsSaysHowAHostUpdateWent(t *testing.T) {
	for _, c := range []struct {
		result proto.Result
		want   string
	}{
		{proto.Result{Err: "checksum mismatch for matchblox-linux-amd64. Nothing was changed"},
			"ws-1 is not updated: checksum mismatch for matchblox-linux-amd64. Nothing was changed"},
		{proto.Result{Ran: [][]string{{"install", "v0.42.0"}}}, "ws-1 installed v0.42.0: its service starts again"},
	} {
		r := releaseRig(t, "v0.42.0", "v0.38.1", viewOn("v0.38.1"))
		ws := r.conns["ws-1"]
		r.key(t, "u")
		r.key(t, "y")
		ws.mu.Lock()
		c.result.ActID = ws.sent[len(ws.sent)-1].ID
		ws.mu.Unlock()
		r.update(peerMsg{host: "ws-1", conn: ws, msg: resultMsg(c.result)})
		if got := r.sh.cur.flash; got != c.want {
			t.Errorf("%+v: %q, want %q", c.result, got, c.want)
		}
		if out := r.view(); !strings.Contains(out, c.want[:40]) {
			t.Errorf("%+v: Hosts does not say it:\n%s", c.result, out)
		}
	}
}

// d dismisses the note of any host that has one, with or without the
// update act: a host too old for it, a Homebrew host, and a host that is
// ahead.
func TestDismissWorksOnAHostWithNoUpdateAct(t *testing.T) {
	brew := viewOn("v0.38.1")
	brew.Managed = "brew upgrade matchblox"
	for name, c := range map[string]struct {
		host string
		view *state.UpdateView
		fix  string
	}{
		"no update view": {"v0.38.1", nil, "on ws-1: matchblox update"},
		"Homebrew":       {"v0.38.1", brew, "on ws-1: brew upgrade matchblox"},
		"ahead":          {"v0.43.0", viewOn("v0.43.0"), "this console is older: run matchblox update"},
	} {
		t.Run(name, func(t *testing.T) {
			r := releaseRig(t, "v0.42.0", c.host, c.view)
			dir := t.TempDir()
			r.sh.cur.opt.StateDir = dir
			if out := r.view(); !strings.Contains(r.rowOf("ws-1"), "this console (v0.42.0)") || !strings.Contains(out, c.fix) || strings.Contains(out, "u update") {
				t.Fatalf("before d: the row %q, want the note and the fix %q with no u:\n%s", r.rowOf("ws-1"), c.fix, out)
			}
			r.key(t, "d")
			if row := r.rowOf("ws-1"); strings.Contains(row, "this console") {
				t.Fatalf("the dismissed note shows: %q", row)
			}
			if out := r.view(); strings.Contains(out, c.fix) || !strings.Contains(out, "dismissed: ws-1 on "+c.host) {
				t.Fatalf("after d: the fix stays, or no word of the dismissal:\n%s", out)
			}
			if p := loadRecPrefs(dir); len(p.Releases) != 1 || p.Releases[0] != "ws-1\x00"+c.host+"\x00v0.42.0" {
				t.Fatalf("kept %+v", p)
			}
		})
	}
}
