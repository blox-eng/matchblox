package app

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/sample"
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

func work(s *sample.Session) { s.AccountDir = "/home/user/.matchblox/accounts/claude/work" }

func TestHostWordOnlyWithTwoHosts(t *testing.T) {
	now := lostAt
	one, _, _ := hostsAt(t, &now, "", 132, 30)
	one = upd(t, one, stateMsg(hostState(sess(1, "api-auth", "%1", work))))
	if l := lineWith(frameOf(one), "api-auth"); strings.Contains(l, hosts1) || !strings.Contains(l, "api-auth work") {
		t.Fatalf("one host: the row is %q, want the account and no host word", l)
	}
	m, _, conns := hostsAt(t, &now, "", 132, 30, "ws-1")
	m = upd(t, m, stateMsg(hostState(sess(1, "api-auth", "%1", nil))))
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(hostState(sess(2, "tl", "%2", work)))})
	f := frameOf(m)
	if l := lineWith(f, "api-auth"); !strings.Contains(l, "api-auth this m…") {
		t.Fatalf("two hosts: the home row is %q, want its host word, cut", l)
	}
	if l := lineWith(f, " tl "); !strings.Contains(l, "tl ws-1 · work") {
		t.Fatalf("two hosts: the peer row is %q, want the host, then the account", l)
	}
	if got := ansi.Strip(m.taggedName("train-loop", m.rowTag(sample.Session{Name: "train-loop", Host: "ws-1", AccountDir: "/a/.matchblox/accounts/claude/work"}, 9), m.st.ink(false), 20)); got != "train-loop ws-1" {
		t.Fatalf("short of room the account goes first: %q", got)
	}
	long := sample.Session{Name: "train-loop", Host: "gpu-cluster-west-1", AccountDir: "/a/.matchblox/accounts/claude/work"}
	if got := ansi.Strip(m.taggedName(long.Name, m.rowTag(long, 9), m.st.ink(false), 20)); got != "train-loop gpu-clus…" {
		t.Fatalf("then the host is cut: %q", got)
	}
}

const hosts1 = "this machine"

func TestHeaderSaysThreeHosts(t *testing.T) {
	now := lostAt
	m, _, conns := hostsAt(t, &now, "", 132, 30, "gpu-1", "ws-1")
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
	m, _, conns := hostsAt(t, &now, "", 132, 30, "ws-1")
	home := hostState(sess(1, "api-auth", "%1", nil))
	m = upd(t, m, stateMsg(stateAt(home, now)))
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(hostState(sess(2, "train-loop", "%2", nil)))})
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: lostMsg{err: io.EOF, conn: conns["ws-1"]}})
	now = lostAt.Add(after)
	return upd(t, m, stateMsg(stateAt(home, now)))
}

func TestStaleRowSaysAsOf(t *testing.T) {
	m := lostPeer(t, 40*time.Second)
	l := lineWith(frameOf(m), "train-loop")
	for _, want := range []string{"stale", "40s", "as of 14:02"} {
		if !strings.Contains(l, want) {
			t.Fatalf("the stale row is %q, want %q in it", l, want)
		}
	}
}

func TestStaleStartsAt30s(t *testing.T) {
	now := lostAt.Add(-time.Second)
	m, _, conns := hostsAt(t, &now, "", 132, 12, "ws-1") // no dock: home's graph moves with each sample
	home, peer := hostState(sess(1, "api-auth", "%1", nil)), hostState(sess(2, "train-loop", "%2", nil))
	m = upd(t, m, stateMsg(stateAt(home, now)))
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(peer)})
	before := frameOf(m)
	now = lostAt
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(peer)})
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: lostMsg{err: io.EOF, conn: conns["ws-1"]}})
	for _, at := range []time.Duration{0, 29 * time.Second} {
		now = lostAt.Add(at)
		m = upd(t, m, stateMsg(stateAt(home, now)))
		if got := frameOf(m); strings.Join(got, "\n") != strings.Join(before, "\n") {
			t.Fatalf("%v after the loss the frame changed:\n%s\nwant:\n%s", at, strings.Join(got, "\n"), strings.Join(before, "\n"))
		}
	}
	now = lostAt.Add(30 * time.Second)
	m = upd(t, m, stateMsg(stateAt(home, now)))
	got := frameOf(m)
	if !strings.HasSuffix(strings.TrimRight(got[0], " "), "· ws-1 stale") && !strings.Contains(got[0], "· ws-1 stale ") {
		t.Fatalf("the header is %q, want · ws-1 stale", got[0])
	}
	at := func(lines []string, s string) int {
		for i, l := range lines {
			if strings.Contains(l, s) {
				return i
			}
		}
		return -1
	}
	if at(got, "train-loop") != at(before, "train-loop") || at(got, "train-loop") < 0 {
		t.Fatal("the stale row left its place in its group")
	}
	raw := strings.Split(m.render(), "\n")
	row := lineWith(raw, "train-loop")
	if !strings.Contains(ansi.Strip(row), "stale") || !strings.Contains(row, m.st.faint.Render("stale")) {
		t.Fatalf("the stale row is %q, want the faint word stale", ansi.Strip(row))
	}
	if strings.Contains(row, m.st.text.Render("train-loop")) {
		t.Fatal("the stale row's name is not faint")
	}
}

func TestStaleAgeTurnsAmberAtAMinute(t *testing.T) {
	m := lostPeer(t, 59*time.Second)
	if row := lineWith(strings.Split(m.render(), "\n"), "train-loop"); !strings.Contains(row, m.st.faint.Render("59s")) {
		t.Fatalf("under a minute the age is faint: %q", row)
	}
	m = lostPeer(t, time.Minute)
	if row := lineWith(strings.Split(m.render(), "\n"), "train-loop"); !strings.Contains(row, m.st.warn.Render("1m")) {
		t.Fatalf("from a minute the age is amber: %q", row)
	}
}

func TestTwoLostHostsLine(t *testing.T) {
	now := lostAt
	m, _, conns := hostsAt(t, &now, "", 132, 30, "gpu-1", "ws-1")
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

// TestHomeLostPeersWork (Review Focus 4): the home host blocked while a
// peer is fine is one row; the peer's rows still act.
func TestHomeLostPeersWork(t *testing.T) {
	now := lostAt
	m, home, conns := hostsAt(t, &now, "home-1", 132, 30, "ws-1")
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(hostState(sess(2, "train-loop", "%2", nil)))})
	m = upd(t, m, lostMsg{err: fmt.Errorf("home-1: %w", remote.ErrNotConnected), conn: home})
	m.splashDone = true
	f := frameOf(m)
	all := strings.Join(f, "\n")
	if strings.Contains(all, "is not connected to this console") {
		t.Fatalf("the whole screen is the blocked text:\n%s", all)
	}
	if n := strings.Count(all, "home-1  is not connected  ⏎ connect"); n != 1 {
		t.Fatalf("%d blocked rows, want 1:\n%s", n, all)
	}
	if lineWith(f, "train-loop") == "" {
		t.Fatalf("the peer's row is gone:\n%s", all)
	}
	var next tea.Model = m
	for range m.rowCount() {
		if s, ok := next.(Model).selectedSession(); ok && s.Host == "ws-1" {
			break
		}
		next, _ = key(next, "down")
	}
	next, _ = key(next, "x")
	if p := next.(Model).pending; p == nil || p.host != "ws-1" {
		t.Fatalf("x on the peer's row is %+v, flash %q; want its kill", p, next.(Model).flash)
	}
	next, _ = key(next, "esc")
	next, _ = key(toBlocked(t, next), "enter")
	if p := next.(Model).pending; p == nil || !p.connect || p.String() != "matchblox connect home-1" {
		t.Fatalf("⏎ on the blocked row is %+v, want the connect of home-1", p)
	}
}

// toBlocked moves the selection down to the first blocked host's row.
func toBlocked(t *testing.T, m tea.Model) tea.Model {
	t.Helper()
	for range m.(Model).rowCount() {
		if _, ok := m.(Model).selectedBlocked(); ok {
			return m
		}
		m, _ = key(m, "down")
	}
	t.Fatal("no blocked row to select")
	return nil
}

func TestOneHostStaleIsToday(t *testing.T) {
	now := lostAt
	m, home, _ := hostsAt(t, &now, "", 132, 30)
	m = upd(t, m, stateMsg(hostState(sess(1, "api-auth", "%1", nil))))
	m = upd(t, m, lostMsg{err: io.EOF, conn: home})
	if h := frameOf(m)[0]; !strings.Contains(h, "matchblox · stale") {
		t.Fatalf("the header is %q, want stale at once", h)
	}
}

// TestPeerOlderVersionIsABlockedRow: a peer that runs another matchblox
// is a blocked row with today's words, not a stream.
func TestPeerOlderVersionIsABlockedRow(t *testing.T) {
	now := lostAt
	m, _, conns := hostsAt(t, &now, "", 132, 30, "ws-1")
	m = upd(t, m, stateMsg(hostState(sess(1, "api-auth", "%1", nil))))
	next, cmd := m.Update(peerMsg{host: "ws-1", conn: conns["ws-1"], msg: helloMsg(proto.Hello{Version: proto.Version - 1})})
	if cmd != nil {
		t.Fatal("the console still reads a peer of another version")
	}
	m = next.(Model)
	if why := m.refusal("ws-1"); why == "" {
		t.Fatal("a peer of another version takes acts")
	}
	want := fmt.Sprintf("ws-1  is version %d, the console is version %d  ⏎ connect", proto.Version-1, proto.Version)
	if l := lineWith(frameOf(m), "ws-1  "); !strings.Contains(l, want) {
		t.Fatalf("the blocked row is %q, want %q", l, want)
	}
}

func TestEnterOnABlockedPeerConnectsIt(t *testing.T) {
	now := lostAt
	m, _, conns := hostsAt(t, &now, "", 132, 30, "ws-1")
	m = upd(t, m, stateMsg(hostState(sess(1, "api-auth", "%1", nil))))
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: lostMsg{err: fmt.Errorf("ws-1: %w", remote.ErrNotConnected), conn: conns["ws-1"]}})
	next, _ := key(toBlocked(t, m), "enter")
	p := next.(Model).pending
	if p == nil || !p.connect || p.String() != "matchblox connect ws-1" {
		t.Fatalf("⏎ on the blocked peer is %+v, want its connect", p)
	}
	next, cmd := next.Update(connectedMsg{host: "ws-1"})
	nm := next.(Model)
	if l := nm.peers[0]; l.blocked != nil || !l.redialing || cmd == nil {
		t.Fatalf("after the connect: blocked %v, redialing %v; want a redial", l.blocked, l.redialing)
	}
	if msg, ok := cmd().(peerRedialMsg); !ok || msg.host != "ws-1" {
		t.Fatalf("the connect dials %#v, want ws-1 again", msg)
	}
	if nm.blocked != nil || nm.lost {
		t.Fatal("the peer's connect touched the home host")
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
		{"fine", 86, 30, []string{"matchblox · 3 hosts", "train-loop gpu-1", "api-auth ws-1", "docs-sweep ws-1"}},
		{"fine", 132, 32, []string{"api-auth ws-1 · work"}},
		{"stale", 86, 30, []string{"· gpu-1 stale", "stale", "40s", "gpu-1: connection lost. Tries again in 8s."}},
		{"stale", 132, 32, []string{"as of 22:49"}},
		{"blocked", 86, 30, []string{"gpu-1  is not connected  ⏎ connect"}},
	} {
		all := strings.Join(frameOf(threeHosts(t, st, c.w, c.h, c.gpu)), "\n")
		for _, w := range c.want {
			if !strings.Contains(all, w) {
				t.Errorf("%s at %d: no %q in the frame", c.gpu, c.w, w)
			}
		}
	}
}

// TestHomeGoesByItsOwnName: the home row's word is the name in its
// hello; this machine is only for before it.
func TestHomeGoesByItsOwnName(t *testing.T) {
	now := lostAt
	m, _, conns := hostsAt(t, &now, "", 132, 30, "ws-1")
	m = upd(t, m, stateMsg(hostState(sess(1, "api", "%1", nil))))
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(hostState(sess(2, "tl", "%2", nil)))})
	if l := lineWith(frameOf(m), " api "); !strings.Contains(l, "api this mach") {
		t.Fatalf("before the hello the home row is %q, want this machine", l)
	}
	m = upd(t, m, helloMsg(proto.Hello{Version: proto.Version, Host: "hearth"}))
	if l := lineWith(frameOf(m), " api "); !strings.Contains(l, "api hearth") {
		t.Fatalf("after the hello the home row is %q, want hearth", l)
	}
	if s := m.all.sessions[0]; s.Host != "" {
		t.Fatalf("the home row's routing key changed to %q", s.Host)
	}
}

// TestShortHomeLossStaysQuiet: with a peer, a home loss and reconnect
// under 30 s leaves the frame as it was (anti-goal 5).
func TestShortHomeLossStaysQuiet(t *testing.T) {
	now := lostAt
	m, home, conns := hostsAt(t, &now, "", 132, 12, "ws-1")
	m = upd(t, m, stateMsg(stateAt(hostState(sess(1, "api", "%1", nil)), now)))
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(hostState(sess(2, "tl", "%2", nil)))})
	before := strings.Join(frameOf(m), "\n")
	m = upd(t, m, lostMsg{err: io.EOF, conn: home})
	m = upd(t, m, connMsg{conn: newFake()})
	if got := strings.Join(frameOf(m), "\n"); got != before {
		t.Fatalf("a short home loss changed the frame:\n%s\nwant:\n%s", got, before)
	}
}
