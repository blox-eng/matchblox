package app

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/sample"
)

// The board's tests of the hosts (Task 12 step 0c).

// boardOn is m with the board on, its first frame drawn.
func boardOn(m Model) Model {
	m.splashDone = true
	m.viewPrefs.CopyCheckedPhone, m.viewPrefs.CopyCheckedDesk = true, true
	return m
}

// busy makes a match work: its card is whole in WORKING.
func busy(s *sample.Session) { s.Busy, s.Status, s.LastLine = true, "busy", "Running the suite." }

// twoHostBoard is the board of this machine with hearth-api, and ws-1
// with name, both at work, at w columns.
func twoHostBoard(t *testing.T, w int, name string) (Model, map[string]*fakeConn) {
	t.Helper()
	now := lostAt
	m, _, conns := hostsAt(t, &now, "", w, 30, "ws-1")
	m = upd(t, m, stateMsg(hostState(sess(1, "hearth-api", "%1", busy))))
	m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(hostState(sess(2, name, "%2", busy)))})
	return boardOn(m), conns
}

// TestACardNamesAnotherHost: a card names its host only when the console
// has two, and only another host's; a long name is cut before the host
// word, which stays where the column has the room. The home host goes by
// the name in its hello.
func TestACardNamesAnotherHost(t *testing.T) {
	now := lostAt
	one, _, _ := hostsAt(t, &now, "", 120, 30)
	one = boardOn(upd(t, one, stateMsg(hostState(sess(1, "hearth-api", "%1", nil)))))
	if l := lineWith(frameOf(one), "hearth-api"); l == "" || strings.Contains(l, "@") || strings.Contains(l, hosts1) {
		t.Fatalf("one host: the card is %q, want no host word", l)
	}
	m, _ := twoHostBoard(t, 120, "spark-docs")
	f := frameOf(m)
	if l := lineWith(f, "spark-docs"); !strings.Contains(l, "spark-docs @ws-1") {
		t.Errorf("two hosts: ws-1's card is %q, want its host", l)
	}
	if l := lineWith(f, "hearth-api"); strings.Contains(l, "@") {
		t.Errorf("two hosts: the home card is %q, want no host word", l)
	}
	// A name too long for the column beside its host word is cut before
	// it. A Medium screen's 20 cells of a column leave the name under
	// nameKeep: there the host word goes.
	for _, w := range []int{60, 104, 160} {
		m, _ := twoHostBoard(t, w, "ember-billing")
		f := frameOf(m)
		if l := lineWith(f, "✦ ember-b"); !strings.Contains(l, "@ws-1") && !strings.Contains(l, "@ws…") {
			t.Errorf("%d columns: the long name's card lost its host word: %q\n%s", w, l, strings.Join(f, "\n"))
		}
	}

	m = upd(t, m, helloMsg(proto.Hello{Version: proto.Version, Host: "hearth"}))
	if m = pressOn(m, "s"); m.boardHost != "hearth" || !strings.Contains(frameOf(m)[0], "hearth ▾") {
		t.Errorf("s turned to %q, header %q: want the home host by its hello's name", m.boardHost, frameOf(m)[0])
	}
	if s := m.all.sessions[0]; s.Host != "" {
		t.Errorf("the home card's routing key changed to %q", s.Host)
	}
}

// TestAStaleHostsCardsSayTheirAge: under 30 s a lost host changes no
// frame; from 30 s the header says the host is stale, and its cards say
// stale, faint, with the time of their last state ("as of 14:02", the
// spec's "A host that is not reached"), which stays faint.
func TestAStaleHostsCardsSayTheirAge(t *testing.T) {
	now := lostAt.Add(-time.Second)
	m, _, conns := hostsAt(t, &now, "", 196, 30, "ws-1")
	home, peer := hostState(sess(1, "hearth-api", "%1", busy)), hostState(sess(2, "spark-docs", "%2", busy))
	m = upd(t, m, stateMsg(stateAt(home, now)))
	m = boardOn(upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: stateMsg(peer)}))
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
	asOf := "as of " + m.stateAt("ws-1").Format("15:04")
	for _, after := range []time.Duration{30 * time.Second, time.Minute, 10 * time.Minute} {
		now = lostAt.Add(after)
		m = upd(t, m, stateMsg(stateAt(home, now)))
		f := frameOf(m)
		if !strings.Contains(f[0], "· ws-1 stale") {
			t.Fatalf("%v: the header is %q, want · ws-1 stale", after, f[0])
		}
		d := m.boardPanel(196, m.bodyRows())
		plain := plainLines(d.lines)
		i := slices.IndexFunc(plain, func(l string) bool { return strings.Contains(l, "✦ spark-") })
		if i < 0 || i+1 >= len(plain) {
			t.Fatalf("%v: no card of ws-1:\n%s", after, strings.Join(f, "\n"))
		}
		if !strings.Contains(plain[i], asOf) || !strings.Contains(plain[i+1], "stale") {
			t.Errorf("%v: the card is %q / %q, want %q and the word stale", after, plain[i], plain[i+1], asOf)
		}
		col := d.heads[slices.IndexFunc(d.heads, func(h span) bool { return h.press == colWork.short() })]
		for _, j := range []int{i, i + 1} {
			if cell := string([]rune(plain[j])[col.x0:col.x1]); !strings.Contains(d.lines[j], m.st.faint.Render(cell)) {
				t.Errorf("%v: the card's line %q is not faint", after, cell)
			}
		}
	}
}

// TestABlockedHostSaysWhyAndItsFix (ruling 30): a host the console cannot
// read is a host line above the columns, in the service's own words and
// with its fix; ↑ chooses it and ⏎ (or a tap) runs the fix: matchblox
// connect, or for a host that does not answer, the command to see why.
func TestABlockedHostSaysWhyAndItsFix(t *testing.T) {
	blocked := func(t *testing.T, msg func(*fakeConn) any) Model {
		t.Helper()
		now := lostAt
		m, _, conns := hostsAt(t, &now, "", 120, 30, "ws-1")
		m = upd(t, m, stateMsg(hostState(sess(1, "hearth-api", "%1", nil))))
		m = upd(t, m, peerMsg{host: "ws-1", conn: conns["ws-1"], msg: msg(conns["ws-1"])})
		return firstCard(boardOn(m))
	}
	for _, c := range []struct {
		name string
		msg  func(*fakeConn) any
		line string
	}{
		{"another version", func(*fakeConn) any { return helloMsg(proto.Hello{Version: proto.Version - 1}) },
			fmt.Sprintf("ws-1: is version %d, the console is version %d · ⏎ connect", proto.Version-1, proto.Version)},
		{"not connected", func(c *fakeConn) any { return lostMsg{err: fmt.Errorf("ws-1: %w", remote.ErrNotConnected), conn: c} },
			"ws-1: is not connected · ⏎ connect"},
	} {
		m := blocked(t, c.msg)
		if l := lineWith(frameOf(m), "ws-1:"); !strings.Contains(l, c.line) {
			t.Errorf("%s: the host line is %q, want %q", c.name, strings.TrimSpace(l), c.line)
			continue
		}
		if m = pressOn(m, "up"); func() bool { line, ok := m.aboveChosen(); return !ok || line != "ws-1" }() {
			t.Errorf("%s: ↑ from the first card does not choose the host line", c.name)
		}
		if p := pressOn(m, "enter").pending; p == nil || !p.connect || p.String() != "matchblox connect ws-1" {
			t.Errorf("%s: ⏎ on the host line is %+v, want its connect", c.name, p)
		}
		y := lineOf(t, m, "ws-1:")
		if next, _ := tap(m, 5, y); next.(Model).pending == nil || !next.(Model).pending.connect {
			t.Errorf("%s: a tap on the host line is %+v, want its connect", c.name, next.(Model).pending)
		}
	}

	m := blocked(t, func(c *fakeConn) any { return silentMsg{c} })
	if l := lineWith(frameOf(m), "ws-1:"); !strings.Contains(l, "ws-1: does not answer · run ssh ws-1 matchblox status to see why") {
		t.Fatalf("the silent host's line is %q", strings.TrimSpace(l))
	}
	if want := "ws-1 does not answer: run ssh ws-1 matchblox status to see why"; pressOn(m, "up", "enter").flash != want {
		t.Errorf("⏎ on the silent host's line says %q, want %q", pressOn(m, "up", "enter").flash, want)
	}
}

// TestTheSamePaneOnTwoHostsIsTwoCards: the same pane and PID on two hosts
// are two cards, and the choice stays on its host's card when the other
// host sends a new state.
func TestTheSamePaneOnTwoHostsIsTwoCards(t *testing.T) {
	next, _, _ := samePane(t, nil)
	m := next.(Model)
	got := drawnNames(m)
	slices.Sort(got)
	if !slices.Equal(got, []string{"home-12", "peer-12"}) {
		t.Fatalf("the cards are %v, want home-12 and peer-12", got)
	}
	if s, ok := m.boardSelected(); !ok || s.Name != "peer-12" || s.Host != "ws-1" {
		t.Fatalf("chosen %s on %q, want peer-12 on ws-1", s.Name, s.Host)
	}
	home := hostState(sess(4242, "home-12", "%12", func(s *sample.Session) { s.Agent, s.Cwd = "claude", "/src/api" }))
	next, _ = m.Update(stateMsg(home))
	if s, ok := next.(Model).boardSelected(); !ok || s.Name != "peer-12" || s.Host != "ws-1" {
		t.Fatalf("a new home state moved the choice to %s on %q", s.Name, s.Host)
	}
}
