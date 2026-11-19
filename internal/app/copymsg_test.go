package app

import (
	"net"
	"testing"

	"github.com/blox-eng/matchblox/internal/proto"
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
