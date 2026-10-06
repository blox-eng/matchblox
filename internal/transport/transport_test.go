package transport

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/proto"
)

// shortDir keeps socket paths under the 104-byte limit of darwin.
func shortDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "mb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	return d
}

func recvHello(t *testing.T, c Conn) proto.Hello {
	t.Helper()
	env, err := c.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if env.Kind != proto.KindHello {
		t.Fatalf("got %q, want hello", env.Kind)
	}
	var h proto.Hello
	if err := json.Unmarshal(env.Body, &h); err != nil {
		t.Fatal(err)
	}
	return h
}

func TestSocketHelloRoundTrip(t *testing.T) {
	path := filepath.Join(shortDir(t), "run", "s.sock")
	l, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		nc, err := l.Accept()
		if err != nil {
			return
		}
		c := NewConn(nc)
		_ = c.Send(proto.KindHello, "", proto.Hello{Version: proto.Version, Host: "ws-1"})
		recvHello(t, c)
		c.Close()
	}()
	c, err := Dial(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if h := recvHello(t, c); h.Host != "ws-1" {
		t.Fatalf("host %q", h.Host)
	}
	if err := c.Send(proto.KindHello, "", proto.Hello{Version: proto.Version}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("after close: %v, want EOF", err)
	}
}

func TestStaleSocketIsReplaced(t *testing.T) {
	path := filepath.Join(shortDir(t), "s.sock")
	old, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	// Close without unlinking: the file stays, nothing listens.
	old.(*net.UnixListener).SetUnlinkOnClose(false)
	old.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stale socket file missing: %v", err)
	}
	l, err := Listen(path)
	if err != nil {
		t.Fatalf("stale socket was not replaced: %v", err)
	}
	l.Close()
}

func TestLiveSocketIsNotStolen(t *testing.T) {
	path := filepath.Join(shortDir(t), "s.sock")
	l, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			nc, err := l.Accept()
			if err != nil {
				return
			}
			nc.Close()
		}
	}()
	if _, err := Listen(path); !errors.Is(err, ErrInUse) {
		t.Fatalf("second Listen: %v, want ErrInUse", err)
	}
}

func TestSocketDirIs0700(t *testing.T) {
	dir := filepath.Join(shortDir(t), "run")
	l, err := Listen(filepath.Join(dir, "s.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m := fi.Mode().Perm(); m != 0o700 {
		t.Fatalf("socket dir mode %o, want 700", m)
	}
}

func TestSocketPath(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	if got := SocketPath(); got != "/run/user/1000/matchblox/matchblox.sock" {
		t.Fatalf("got %q", got)
	}
	t.Setenv("XDG_RUNTIME_DIR", "")
	want := filepath.Join(os.TempDir(), "matchblox-"+uid(), "matchblox.sock")
	if got := SocketPath(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStdioPipe(t *testing.T) {
	// console <-> [stdio | Pipe | socket side] <-> service
	consoleR, pipeW := io.Pipe()
	pipeR, consoleW := io.Pipe()
	console := Stdio(consoleR, consoleW)
	stdio := Stdio(pipeR, pipeW)

	a, b := net.Pipe()
	service, socketSide := NewConn(a), NewConn(b)
	go Pipe(stdio, socketSide)

	if err := service.Send(proto.KindHello, "", proto.Hello{Version: proto.Version, Host: "ws-1"}); err != nil {
		t.Fatal(err)
	}
	if h := recvHello(t, console); h.Host != "ws-1" {
		t.Fatalf("console got host %q", h.Host)
	}
	if err := console.Send(proto.KindAct, "a1", proto.Act{RecID: "r1", Which: "primary"}); err != nil {
		t.Fatal(err)
	}
	env, err := service.Recv()
	if err != nil || env.Kind != proto.KindAct || env.ID != "a1" {
		t.Fatalf("service got %+v, %v", env, err)
	}
	service.Close()
	if _, err := console.Recv(); err == nil {
		t.Fatal("console must see the end when the service side closes")
	}
}

func TestConcurrentListenOneWins(t *testing.T) {
	path := filepath.Join(shortDir(t), "s.sock")
	old, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	old.(*net.UnixListener).SetUnlinkOnClose(false)
	old.Close()

	const n = 64
	results := make(chan net.Listener, n)
	for range n {
		go func() {
			l, err := Listen(path)
			if err != nil {
				results <- nil
				return
			}
			results <- l
		}()
	}
	won := 0
	for range n {
		if l := <-results; l != nil {
			won++
			defer l.Close()
		}
	}
	if won != 1 {
		t.Fatalf("%d listeners won, want exactly 1", won)
	}
	c, err := Dial(path, time.Second)
	if err != nil {
		t.Fatalf("the winner's socket was removed: %v", err)
	}
	c.Close()
}

func TestListenRecordsOwnerPID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no lock file")
	}
	path := filepath.Join(shortDir(t), "s.sock")
	l, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if pid := Owner(path); pid != os.Getpid() {
		t.Fatalf("owner %d, want %d", pid, os.Getpid())
	}
}

// On a shared /tmp another user can make the directory first; a console
// must not talk to a service it does not own.
func TestDialRefusesAnUnsafeDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix modes")
	}
	base := shortDir(t)
	open := filepath.Join(base, "open")
	if err := os.Mkdir(open, 0o700); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("unix", filepath.Join(open, "s.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := os.Chmod(open, 0o755); err != nil {
		t.Fatal(err)
	}
	if c, err := Dial(filepath.Join(open, "s.sock"), time.Second); err == nil {
		c.Close()
		t.Fatal("dialed a socket in a directory others can enter")
	}
	link := filepath.Join(base, "link")
	if err := os.Chmod(open, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(open, link); err != nil {
		t.Fatal(err)
	}
	if c, err := Dial(filepath.Join(link, "s.sock"), time.Second); err == nil {
		c.Close()
		t.Fatal("dialed through a symlinked directory")
	}
	if c, err := Dial(filepath.Join(open, "s.sock"), time.Second); err != nil {
		t.Fatalf("a safe directory was refused: %v", err)
	} else {
		c.Close()
	}
}
