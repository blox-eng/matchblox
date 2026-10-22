package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/proto"
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
	actService(t, sock, proto.Result{Scan: &proto.ScanSummary{Repos: 2, Took: 1300 * time.Millisecond,
		Errors: []string{"stopped after 100000 folders: some repositories may be missing"}}})
	var out bytes.Buffer
	if err := cliScan(nil, &out, sock); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "found 2 repositories in 1.3s\nstopped after 100000 folders: some repositories may be missing\n" {
		t.Fatalf("got %q", got)
	}

	sock3 := filepath.Join(filepath.Dir(sock), "three.sock")
	actService(t, sock3, proto.Result{Scan: &proto.ScanSummary{Repos: 1, Took: time.Second}})
	out.Reset()
	if err := cliScan(nil, &out, sock3); err != nil || out.String() != "found 1 repository in 1s\n" {
		t.Fatalf("one: %q %v", out.String(), err)
	}

	sock2 := filepath.Join(filepath.Dir(sock), "two.sock")
	actService(t, sock2, proto.Result{})
	out.Reset()
	if err := cliScan([]string{"off"}, &out, sock2); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "the scan is off, and its list is gone\n" {
		t.Fatalf("off: got %q", got)
	}
}

func TestCLIScanListsSkipped(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home")
	}
	sock := isolate(t)
	actService(t, sock, proto.Result{Scan: &proto.ScanSummary{Repos: 3, Took: 2 * time.Second,
		Errors:      []string{"e1"},
		Skipped:     []gitscan.Skip{{Path: filepath.Join(home, "Desktop"), Why: "macOS asks first"}, {Path: "/mnt/nas", Why: "network"}},
		SkippedMore: 4}})
	var out bytes.Buffer
	if err := cliScan(nil, &out, sock); err != nil {
		t.Fatal(err)
	}
	want := "found 3 repositories in 2s\ne1\nskipped ~/Desktop: macOS asks first\nskipped /mnt/nas: network\nand 4 more\n"
	if out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}

func TestCLIScanSuperseded(t *testing.T) {
	sock := isolate(t)
	actService(t, sock, proto.Result{Err: "other folders were picked since: /a"})
	err := cliScan(nil, &bytes.Buffer{}, sock)
	if err == nil || !strings.Contains(err.Error(), "other folders were picked since") {
		t.Fatalf("err %v", err)
	}
}

func TestScanWaitDerivesFromLimit(t *testing.T) {
	o := scanOpts(config.Config{}, "/h")
	if got, want := scanWait(o), 2*(30*time.Second+5*time.Second)+10*time.Second; got != want {
		t.Fatalf("scanWait %s, want %s", got, want)
	}
}

func TestScanOptsProtected(t *testing.T) {
	o := scanOpts(config.Config{}, "/h")
	if want := gitscan.DefaultProtected("/h"); !reflect.DeepEqual(o.Protected, want) {
		t.Fatalf("Protected %v, want %v", o.Protected, want)
	}
	if o.MaxDirs != scanMaxDirs || o.Budget != scanBudget || o.Home != "/h" {
		t.Fatalf("%+v", o)
	}
}

func TestScanActStatBounded(t *testing.T) {
	old := stat
	t.Cleanup(func() { stat = old })
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	stat = func(string) (os.FileInfo, error) { <-block; return nil, nil }
	start := time.Now()
	_, err := scanAct([]string{"/dead"}, "/h", "/h")
	if err == nil || err.Error() != "scan: /dead did not answer in 2s" {
		t.Fatalf("err %v", err)
	}
	if d := time.Since(start); d > 2*time.Second+100*time.Millisecond {
		t.Fatalf("took %s", d)
	}
}

func TestCliScanRefused(t *testing.T) {
	sock := isolate(t)
	actService(t, sock, proto.Result{Err: "only you say yes to a scan, at a console or in your own shell"})
	if err := cliScan(nil, &bytes.Buffer{}, sock); err == nil || err.Error() != "scan: refused: only you say yes to a scan, at a console or in your own shell" {
		t.Fatalf("err %v", err)
	}
}
