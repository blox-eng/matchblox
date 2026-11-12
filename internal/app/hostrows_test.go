package app

import (
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/blox-eng/matchblox/internal/proto"
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
