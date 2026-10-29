package app

import (
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/history"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/transport"
)

// peered is a console on a home host with one peer, ws-1.
func peered(redial func() (transport.Conn, error)) (Model, *fakeConn, *fakeConn) {
	home, ws := newFake(), newFake()
	m := New(Options{NoMotion: true, Conn: home, Peers: []Peer{{Host: "ws-1", Conn: ws, Redial: redial}}})
	return m, home, ws
}

// redialTo runs the peer's redial tick and hands its new conn to the console.
func redialTo(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := m.Update(peerRedialMsg{host: "ws-1"})
	if cmd == nil {
		t.Fatal("the redial tick dialed nothing")
	}
	next, _ = next.Update(cmd())
	return next.(Model)
}

func TestPeerStateIsKept(t *testing.T) {
	m, _, ws := peered(nil)
	st := fixtureState()
	st.History = &history.Series{}
	next, cmd := m.Update(peerMsg{host: "ws-1", conn: ws, msg: stateMsg(st)})
	if cmd == nil {
		t.Fatal("the peer's next message is not read")
	}
	nm := next.(Model)
	p := nm.peers[0]
	if !p.have || p.at.IsZero() || len(p.st.Sessions) != len(st.Sessions) || len(st.Sessions) == 0 {
		t.Fatalf("the peer's state was not kept: have %v, at %v, %d sessions", p.have, p.at, len(p.st.Sessions))
	}
	if nm.have || len(nm.homeAll.sessions) != 0 || len(nm.hist.Points) != 0 {
		t.Fatal("a peer's state changed the home host's")
	}
	if len(nm.all.sessions) != len(st.Sessions) || nm.all.sessions[0].Host != "ws-1" {
		t.Fatal("the peer's sessions did not join the list")
	}
	later := fixtureState() // History, Found and Used come in the first state only
	next, _ = next.Update(peerMsg{host: "ws-1", conn: ws, msg: stateMsg(later)})
	if next.(Model).peers[0].st.History == nil {
		t.Fatal("a later state dropped the history of the first")
	}
}

func TestPeerLostRedialsOnItsOwn(t *testing.T) {
	m, home, ws := peered(func() (transport.Conn, error) { return newFake(), nil })
	next, cmd := m.Update(peerMsg{host: "ws-1", conn: ws, msg: lostMsg{err: io.EOF, conn: ws}})
	nm := next.(Model)
	p := nm.peers[0]
	if !p.lost || !p.redialing || p.backoff != 250*time.Millisecond {
		t.Fatalf("lost %v, redialing %v, backoff %v; want lost, redialing, 250ms", p.lost, p.redialing, p.backoff)
	}
	if !ws.closed {
		t.Fatal("the lost peer conn was not closed")
	}
	if home.closed || nm.lost || nm.redialing {
		t.Fatal("a lost peer took the home host down")
	}
	if nm.flash != "" {
		t.Fatalf("flash %q at the loss: a short one must not show", nm.flash)
	}
	nm.peers[0].at, nm.peers[0].have = nm.now().Add(-staleAfter), true
	if want := "ws-1: connection lost. Tries again in 250ms."; nm.lostLine() != want {
		t.Fatalf("once stale the key line is %q, want %q", nm.lostLine(), want)
	}
	if cmd == nil {
		t.Fatal("no redial scheduled")
	}
	if msg, ok := cmd().(peerRedialMsg); !ok || msg.host != "ws-1" {
		t.Fatalf("the tick gives %#v, want a peerRedialMsg of ws-1", msg)
	}
	if _, cmd := next.Update(peerMsg{host: "ws-1", conn: ws, msg: lostMsg{err: io.EOF, conn: ws}}); cmd != nil {
		t.Fatal("a second loss of the same peer conn scheduled a second redial")
	}
}

func TestPeerBlockedStopsRedial(t *testing.T) {
	m, _, ws := peered(func() (transport.Conn, error) { return newFake(), nil })
	err := fmt.Errorf("ws-1: %w", remote.ErrNotConnected)
	next, cmd := m.Update(peerMsg{host: "ws-1", conn: ws, msg: lostMsg{err: err, conn: ws}})
	if p := next.(Model).peers[0]; !errors.Is(p.blocked, remote.ErrNotConnected) || !p.lost || p.redialing {
		t.Fatalf("blocked %v, lost %v, redialing %v; want blocked and lost, no redial", p.blocked, p.lost, p.redialing)
	}
	if cmd != nil {
		t.Fatal("a blocked peer scheduled a redial")
	}
}

func TestPeerDialErrStartsLost(t *testing.T) {
	err := fmt.Errorf("ws-1: %w", remote.ErrNotInstalled)
	m := New(Options{NoMotion: true, Conn: newFake(), Peers: []Peer{{Host: "ws-1", DialErr: err}}})
	msg := m.peerStart(0)()
	next, _ := m.Update(msg)
	if p := next.(Model).peers[0]; !p.lost || !errors.Is(p.blocked, remote.ErrNotInstalled) {
		t.Fatalf("a peer whose dial failed starts lost %v, blocked %v", p.lost, p.blocked)
	}
}

func TestLateMessageOfAnOldPeerConnIsDropped(t *testing.T) {
	y := newFake()
	m, _, x := peered(func() (transport.Conn, error) { return y, nil })
	next, _ := m.Update(peerMsg{host: "ws-1", conn: x, msg: lostMsg{err: io.EOF, conn: x}})
	nm := redialTo(t, next.(Model))
	if p := nm.peers[0]; p.conn != y || p.redialing {
		t.Fatal("the peer did not take the new conn")
	}
	next, cmd := nm.Update(peerMsg{host: "ws-1", conn: x, msg: stateMsg(fixtureState())})
	if cmd != nil || next.(Model).peers[0].have {
		t.Fatal("a message from the old peer conn was handled; it would start a second reader on the new one")
	}
}

func TestPeerResultIsHandledAsHomeResult(t *testing.T) {
	m, home, ws := peered(nil)
	cmd := m.sendPeer(0, proto.Act{RecID: "answer:%12", Text: "yes"})
	if cmd() != nil || len(ws.acts()) != 1 || len(home.acts()) != 0 {
		t.Fatal("the act did not go on the peer's conn")
	}
	id := ws.sent[0].ID
	m.peekSent = map[string]string{id: "app-review"}
	next, cmd := m.Update(peerMsg{host: "ws-1", conn: ws, msg: resultMsg{ActID: id}})
	nm := next.(Model)
	if nm.peekSent[id] != "" || len(nm.peers[0].acts) != 0 || nm.toast != "sent to app-review" {
		t.Fatalf("the peer's result did not end its act: peekSent %v, in flight %v, toast %q", nm.peekSent, nm.peers[0].acts, nm.toast)
	}
	if cmd == nil {
		t.Fatal("the peer's next message is not read")
	}
}

func TestResultOfAnActLostWithItsPeer(t *testing.T) {
	y := newFake()
	m, _, x := peered(func() (transport.Conn, error) { return y, nil })
	_ = m.sendPeer(0, proto.Act{RecID: "answer:%12", Text: "yes"})()
	id := x.sent[0].ID
	m.peekSent = map[string]string{id: "app-review"}
	next, _ := m.Update(peerMsg{host: "ws-1", conn: x, msg: lostMsg{err: io.EOF, conn: x}})
	nm := next.(Model)
	if want := "ws-1: 1 act has no answer"; nm.toast != want || nm.flash != "" {
		t.Fatalf("toast %q, flash %q; want the toast %q", nm.toast, nm.flash, want)
	}
	if len(nm.peers[0].acts) != 0 {
		t.Fatal("the lost acts are still in flight")
	}
	nm = redialTo(t, nm)
	flash := nm.flash
	next, cmd := nm.Update(peerMsg{host: "ws-1", conn: y, msg: resultMsg{ActID: id, Err: "late"}})
	nm = next.(Model)
	if nm.peekSent[id] != "app-review" || nm.toast != "ws-1: 1 act has no answer" || nm.flash != flash {
		t.Fatalf("a result on the new conn was taken for the lost act: peekSent %v, toast %q, flash %q", nm.peekSent, nm.toast, nm.flash)
	}
	if cmd == nil {
		t.Fatal("the peer's next message is not read")
	}
}

// TestPeerBlockedClearsItsFlash: a blocked peer is its row: the flash it
// set goes, and its acts in flight say they have no answer.
func TestPeerBlockedClearsItsFlash(t *testing.T) {
	m, _, ws := peered(func() (transport.Conn, error) { return newFake(), nil })
	_ = m.sendPeer(0, proto.Act{RecID: "answer:%12", Text: "yes"})()
	m.flash = "ws-1: connected again"
	err := fmt.Errorf("ws-1: %w", remote.ErrHostKeyChanged)
	next, _ := m.Update(peerMsg{host: "ws-1", conn: ws, msg: lostMsg{err: err, conn: ws}})
	nm := next.(Model)
	if nm.flash != "" || len(nm.peers[0].acts) != 0 || nm.toast != "ws-1: 1 act has no answer" {
		t.Fatalf("flash %q, in flight %v, toast %q; want no flash, no act, the toast", nm.flash, nm.peers[0].acts, nm.toast)
	}
}

// A reconcile keeps each link's own dial error: the peers before it move,
// and the new one still starts blocked.
func TestReconciledPeerKeepsItsDialErr(t *testing.T) {
	err := fmt.Errorf("gpu-1: %w", remote.ErrNotInstalled)
	m := New(Options{NoMotion: true, Conn: newFake(), Peers: []Peer{{Host: "ws-1", Conn: newFake()}}})
	m, cmd := m.reconcile([]Peer{{Host: "gpu-1", DialErr: err}, {Host: "ws-1"}})
	if cmd == nil || len(m.peers) != 2 || m.peers[0].host != "gpu-1" {
		t.Fatalf("peers %+v", m.peers)
	}
	next, _ := m.Update(m.peerStart(0)())
	if p := next.(Model).peers[0]; !p.lost || !errors.Is(p.blocked, remote.ErrNotInstalled) {
		t.Fatalf("gpu-1 starts lost %v, blocked %v", p.lost, p.blocked)
	}
}
