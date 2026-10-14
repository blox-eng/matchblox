package transport

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPeerPIDIsTheConnectingProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	c, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	nc, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	pid, ok := PeerPID(nc)
	switch runtime.GOOS {
	case "linux", "darwin":
		if !ok || pid != os.Getpid() {
			t.Fatalf("peer %d %v, want %d", pid, ok, os.Getpid())
		}
	default:
		if ok {
			t.Fatalf("a peer pid on %s", runtime.GOOS)
		}
	}
	if _, ok := PeerPID(&net.TCPConn{}); ok {
		t.Fatal("a pid for a TCP connection")
	}
}
