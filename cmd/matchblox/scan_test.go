package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/transport"
)

func TestScanActYes(t *testing.T) {
	a, err := scanAct(nil, "/h", "/h")
	if err != nil || a != (proto.Act{RecID: "scan:yes", Confirm: "y"}) {
		t.Fatalf("%+v %v", a, err)
	}
}

func TestScanActFolders(t *testing.T) {
	home, here := t.TempDir(), t.TempDir()
	mkdir(t, filepath.Join(home, "w"))
	mkdir(t, filepath.Join(here, "rel"))
	a, err := scanAct([]string{"~/w", "rel"}, here, home)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "w") + "\n" + filepath.Join(here, "rel"); a.RecID != "scan:yes" || a.Text != want {
		t.Fatalf("%+v, want Text %q", a, want)
	}
}

func TestScanActOff(t *testing.T) {
	a, err := scanAct([]string{"off"}, "/h", "/h")
	if err != nil || a.RecID != "scan:no" {
		t.Fatalf("%+v %v", a, err)
	}
}

func TestScanActMissingFolder(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "gone")
	if _, err := scanAct([]string{missing}, dir, dir); err == nil || err.Error() != "scan: no folder "+missing {
		t.Fatalf("err %v", err)
	}
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

// actService answers one act with res, after hello and a snapshot.
func actService(t *testing.T, sock string, res proto.Result) <-chan proto.Act {
	t.Helper()
	l, err := transport.Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	got := make(chan proto.Act, 1)
	go func() {
		nc, err := l.Accept()
		if err != nil {
			return
		}
		c := transport.NewConn(nc)
		defer c.Close()
		c.Send(proto.KindHello, "", proto.Hello{Version: proto.Version})
		if env, err := c.Recv(); err != nil || env.Kind != proto.KindHello {
			return
		}
		env, err := c.Recv()
		if err != nil || env.Kind != proto.KindAct {
			return
		}
		var a proto.Act
		json.Unmarshal(env.Body, &a)
		got <- a
		res.ActID = env.ID
		c.Send(proto.KindResult, env.ID, res)
	}()
	return got
}

func TestCliScanSays(t *testing.T) {
	sock := isolate(t)
	repos := filepath.Join(t.TempDir(), "repos.json")
	state.WriteFound(repos, gitscan.Found{Consent: "yes", Took: 1300 * time.Millisecond, Repos: []gitscan.Checkout{{Path: "/a"}, {Path: "/b"}}})
	actService(t, sock, proto.Result{})
	var out bytes.Buffer
	if err := cliScan(nil, &out, sock, repos); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "found 2 repositories in 1.3s\n" {
		t.Fatalf("got %q", got)
	}

	sock2 := filepath.Join(filepath.Dir(sock), "two.sock")
	actService(t, sock2, proto.Result{})
	out.Reset()
	if err := cliScan([]string{"off"}, &out, sock2, repos); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "the scan is off, and its list is gone\n" {
		t.Fatalf("off: got %q", got)
	}
}

func TestCliScanRefused(t *testing.T) {
	sock := isolate(t)
	actService(t, sock, proto.Result{Err: "only you say yes to a scan, at a console or in your own shell"})
	if err := cliScan(nil, &bytes.Buffer{}, sock, filepath.Join(t.TempDir(), "r.json")); err == nil || err.Error() != "scan: refused: only you say yes to a scan, at a console or in your own shell" {
		t.Fatalf("err %v", err)
	}
}
