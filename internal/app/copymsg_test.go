package app

import (
	"errors"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/remote"
	"github.com/blox-eng/matchblox/internal/transport"
)

// TestACopyMessageBecomesACopyMsg: a service's copy reaches the console's
// update loop as a copyMsg, tagged with its conn, and the next message
// still comes after it.
func TestACopyMessageBecomesACopyMsg(t *testing.T) {
	a, b := net.Pipe()
	svc, console := transport.NewConn(a), transport.NewConn(b)
	t.Cleanup(func() { _ = svc.Close(); _ = console.Close() })
	go func() {
		_ = svc.Send(proto.KindCopy, "", proto.Copy{Text: "x"})
		_ = svc.Send(proto.KindSnapshot, "", proto.State{})
	}()
	got, ok := recvOn(console)().(fromConn)
	if !ok || got.conn != console {
		t.Fatalf("got %#v, want a fromConn of the console's conn", got)
	}
	if c, ok := got.msg.(copyMsg); !ok || c.Text != "x" {
		t.Fatalf("got %#v, want copyMsg{Text: x}", got.msg)
	}
	if next, ok := recvOn(console)().(fromConn); !ok {
		t.Fatalf("the snapshot after the copy did not come: %#v", next)
	} else if _, ok := next.msg.(stateMsg); !ok {
		t.Fatalf("got %#v after the copy, want the snapshot", next.msg)
	}
}

// TestAnUnknownKindIsSkipped: a console skips a kind it does not know, so a
// newer service's messages never stop an older console's reads.
func TestAnUnknownKindIsSkipped(t *testing.T) {
	a, b := net.Pipe()
	svc, console := transport.NewConn(a), transport.NewConn(b)
	t.Cleanup(func() { _ = svc.Close(); _ = console.Close() })
	go func() {
		_ = svc.Send(proto.Kind("a-later-kind"), "", proto.Copy{Text: "x"})
		_ = svc.Send(proto.KindSnapshot, "", proto.State{})
	}()
	got, ok := recvOn(console)().(fromConn)
	if !ok {
		t.Fatalf("got %#v", got)
	}
	if _, ok := got.msg.(stateMsg); !ok {
		t.Fatalf("got %#v, want the snapshot after the unknown kind", got.msg)
	}
}

// screenTmux is the console's tmux, the console in session matchblox: its
// clients are "name\tactivity\ttty\tsession"; the panes' ttys are paneTTYs. It keeps each
// load-buffer as its argv and the text of its file.
type screenTmux struct {
	clients  []string
	paneTTYs []string
	loadErr  error
	loads    [][]string
	loaded   []string
}

func (s *screenTmux) run(args ...string) (string, error) {
	switch {
	case slices.Equal(args, []string{"display", "-p", "-t", "%1", "#{session_name}"}):
		return "matchblox\n", nil
	case slices.Equal(args, []string{"list-clients", "-F", "#{client_name}\t#{client_activity}\t#{client_tty}\t#{client_session}"}):
		return strings.Join(s.clients, "\n") + "\n", nil
	case slices.Equal(args, []string{"list-panes", "-a", "-F", "#{pane_tty}"}):
		return strings.Join(s.paneTTYs, "\n") + "\n", nil
	case len(args) == 7 && args[0] == "load-buffer":
		s.loads = append(s.loads, args[:6])
		b, _ := os.ReadFile(args[6])
		s.loaded = append(s.loaded, string(b))
		return "", s.loadErr
	}
	return "", errors.New("unknown command")
}

// copyConsole is a console in pane %1 of tmux, at clock.
func copyConsole(tm *screenTmux, clock time.Time, peers ...Peer) Model {
	m := New(Options{NoMotion: true, Pane: "%1", Tmux: tm.run, Now: func() time.Time { return clock }, Peers: peers})
	return m
}

func activeAt(t time.Time) string { return strconv.FormatInt(t.Unix(), 10) }

// TestACopyGoesToTheConsolesOwnClient: the copy goes to the client of the
// console's session that had input last, within 10 s; a nested view's
// client (its tty is a pane's) is never it.
func TestACopyGoesToTheConsolesOwnClient(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tm := &screenTmux{
		clients: []string{
			"/dev/pts/3\t" + activeAt(now.Add(-5*time.Minute)) + "\t/dev/pts/3" + "\tmatchblox",
			"/dev/pts/4\t" + activeAt(now.Add(-2*time.Second)) + "\t/dev/pts/4" + "\tmatchblox",
			"/dev/pts/9\t" + activeAt(now) + "\t/dev/pts/9" + "\tmatchblox",
		},
		paneTTYs: []string{"/dev/pts/9"},
	}
	next, _ := copyConsole(tm, now).Update(fromConn{msg: copyMsg{Text: "hi"}})
	m := next.(Model)
	want := [][]string{{"load-buffer", "-w", "-b", proto.CopyBuffer, "-t", "/dev/pts/4"}}
	if !slices.EqualFunc(tm.loads, want, slices.Equal) || !slices.Equal(tm.loaded, []string{"hi"}) {
		t.Fatalf("loaded %q with %q, want %q with hi", tm.loaded, tm.loads, want)
	}
	if !strings.HasPrefix(m.noticeText, "copied: 2 characters") {
		t.Errorf("said %q, want copied: 2 characters", m.noticeText)
	}
}

// TestNoRecentInputMeansNoCopy: when no client had input in 10 s, the person
// is at another screen: nothing is loaded and nothing said.
func TestNoRecentInputMeansNoCopy(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tm := &screenTmux{clients: []string{"/dev/pts/4\t" + activeAt(now.Add(-30*time.Second)) + "\t/dev/pts/4" + "\tmatchblox"}}
	next, _ := copyConsole(tm, now).Update(fromConn{msg: copyMsg{Text: "hi"}})
	if m := next.(Model); len(tm.loads) != 0 || m.noticeText != "" {
		t.Fatalf("loaded %q, said %q", tm.loads, m.noticeText)
	}
}

// TestAFailedLoadSaysWhy: a load tmux refuses says tmux's words, never
// copied.
func TestAFailedLoadSaysWhy(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tm := &screenTmux{clients: []string{"/dev/pts/4\t" + activeAt(now) + "\t/dev/pts/4" + "\tmatchblox"}, loadErr: errors.New("no current client")}
	next, _ := copyConsole(tm, now).Update(fromConn{msg: copyMsg{Text: "hi"}})
	if m := next.(Model); m.noticeText != "not copied: no current client" {
		t.Fatalf("said %q", m.noticeText)
	}
}

// TestACopyOutsideTmuxGoesToTheTerminal: a console with no tmux writes the
// copy to its terminal itself.
func TestACopyOutsideTmuxGoesToTheTerminal(t *testing.T) {
	m := New(Options{NoMotion: true, OutsideTmux: true})
	next, cmd := m.Update(fromConn{msg: copyMsg{Text: "hi"}})
	if !strings.HasPrefix(next.(Model).noticeText, "copied: 2 characters") || cmd == nil {
		t.Fatalf("said %q, cmd %v", next.(Model).noticeText, cmd)
	}
}

// TestAPhoneTakesNoCopies: a console with its own clipboard (Pocket) takes
// no copy from the service, so a desk's copy never lands on the phone.
func TestAPhoneTakesNoCopies(t *testing.T) {
	var clipped []string
	m := New(Options{NoMotion: true, Clip: func(s string) { clipped = append(clipped, s) }})
	next, _ := m.Update(fromConn{msg: copyMsg{Text: "hi"}})
	if len(clipped) != 0 || next.(Model).noticeText != "" {
		t.Fatalf("the phone took %q and said %q", clipped, next.(Model).noticeText)
	}
}

// TestAHostCopySaysWhereFrom: a copy from a peer's service names its host.
func TestAHostCopySaysWhereFrom(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tm := &screenTmux{clients: []string{"/dev/pts/4\t" + activeAt(now) + "\t/dev/pts/4" + "\tmatchblox"}}
	ws := newFake()
	m := copyConsole(tm, now, Peer{Host: "ws-1", Conn: ws})
	m.besideShows = paneKey{"ws-1", "%7"}
	next, _ := m.Update(peerMsg{host: "ws-1", conn: ws, msg: copyMsg{Text: "hi"}})
	if got := next.(Model).noticeText; !strings.HasPrefix(got, "copied: 2 characters from ws-1") {
		t.Fatalf("said %q, want copied: 2 characters from ws-1", got)
	}
}

// TestAHostCopyNeedsItsHostOnScreen: a host's copy goes to the clipboard
// only while the console shows that host beside the list and the person
// touched the screen in the last 2 s (owner, 2026-11-20): a program on a
// host cannot fill the clipboard while the person looks elsewhere.
func TestAHostCopyNeedsItsHostOnScreen(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	for _, c := range []struct {
		name   string
		shows  paneKey
		active time.Duration
		loads  bool
	}{
		{"its host beside, just touched", paneKey{"ws-1", "%7"}, time.Second, true},
		{"another host beside", paneKey{"ws-2", "%7"}, time.Second, false},
		{"nothing beside", paneKey{}, time.Second, false},
		{"its host beside, touched 3 s ago", paneKey{"ws-1", "%7"}, 3 * time.Second, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			tm := &screenTmux{clients: []string{"/dev/pts/4\t" + activeAt(now.Add(-c.active)) + "\t/dev/pts/4\tmatchblox"}}
			ws := newFake()
			m := copyConsole(tm, now, Peer{Host: "ws-1", Conn: ws})
			m.besideShows = c.shows
			next, _ := m.Update(peerMsg{host: "ws-1", conn: ws, msg: copyMsg{Text: "hi"}})
			if got := len(tm.loads) == 1; got != c.loads {
				t.Fatalf("loaded %q, said %q; want a load: %v", tm.loads, next.(Model).noticeText, c.loads)
			}
		})
	}
}

// TestAHostCopyDuringAJump: in a jump to the host, the person's client is on
// the jump's session, which shows that host: the copy goes to it.
func TestAHostCopyDuringAJump(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tm := &screenTmux{clients: []string{"/dev/pts/4\t" + activeAt(now) + "\t/dev/pts/4\t" + remote.HostSessionName("ws-1", "0a1b2c3d")}}
	ws := newFake()
	next, _ := copyConsole(tm, now, Peer{Host: "ws-1", Conn: ws}).Update(peerMsg{host: "ws-1", conn: ws, msg: copyMsg{Text: "hi"}})
	if len(tm.loads) != 1 || tm.loads[0][5] != "/dev/pts/4" {
		t.Fatalf("loaded %q, said %q", tm.loads, next.(Model).noticeText)
	}
	tm = &screenTmux{clients: []string{"/dev/pts/4\t" + activeAt(now) + "\t/dev/pts/4\t" + remote.HostSessionName("ws-2", "0a1b2c3d")}}
	if _, _ = copyConsole(tm, now, Peer{Host: "ws-1", Conn: ws}).Update(peerMsg{host: "ws-1", conn: ws, msg: copyMsg{Text: "hi"}}); len(tm.loads) != 0 {
		t.Fatalf("a jump to ws-2 took ws-1's copy: %q", tm.loads)
	}
}
