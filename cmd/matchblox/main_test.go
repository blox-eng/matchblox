package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/transport"
)

// TestMain lets a test start this binary as `matchblox <args>`.
func TestMain(m *testing.M) {
	if args := os.Getenv("MATCHBLOX_TEST_ARGS"); args != "" {
		if err := run(strings.Split(args, "\x1f")); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fixtures(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("../../testdata/machine")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// isolate points the socket and state at fresh short directories.
func isolate(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "mb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	return transport.SocketPath()
}

func hello(t *testing.T, c transport.Conn) proto.Hello {
	t.Helper()
	if err := c.Send(proto.KindHello, "", proto.Hello{Version: proto.Version}); err != nil {
		t.Fatal(err)
	}
	env, err := c.Recv()
	if err != nil || env.Kind != proto.KindHello {
		t.Fatalf("hello: %+v, %v", env, err)
	}
	var h proto.Hello
	_ = json.Unmarshal(env.Body, &h)
	return h
}

func TestConsoleStartsService(t *testing.T) {
	path := isolate(t)
	if _, err := transport.Dial(path, 100*time.Millisecond); err == nil {
		t.Fatal("a service already runs on the fresh socket")
	}
	var child *exec.Cmd
	spawnService = func() error {
		child = exec.Command(os.Args[0])
		child.Env = append(os.Environ(), "MATCHBLOX_TEST_ARGS=serve\x1f--fixtures\x1f"+fixtures(t))
		detach(child)
		return child.Start()
	}
	t.Cleanup(func() {
		if child != nil && child.Process != nil {
			child.Process.Kill()
			child.Wait()
		}
	})
	c, err := ensureService(path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if h := hello(t, c); h.Version != proto.Version {
		t.Fatalf("hello %+v", h)
	}
	// A second console finds the running service and starts nothing.
	spawnService = func() error { t.Fatal("started a second service"); return nil }
	c2, err := ensureService(path)
	if err != nil {
		t.Fatal(err)
	}
	c2.Close()
}

func TestEnsureServiceGivesUpWithTheFix(t *testing.T) {
	path := isolate(t)
	spawnService = func() error { return nil } // starts nothing
	start := time.Now()
	_, err := ensureService(path)
	if err == nil || !strings.Contains(err.Error(), "matchblox serve") {
		t.Fatalf("err %v, want the command to run", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("gave up after %v, want about 2 s", d)
	}
}

func serveInProcess(t *testing.T, path string) {
	t.Helper()
	l, err := transport.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := newService(config.Default(), fixtures(t))
	go s.Serve(ctx, l)
}

func TestStatusReadsService(t *testing.T) {
	path := isolate(t)
	serveInProcess(t, path)
	var out bytes.Buffer
	if err := status(&out, path, config.Default(), "", true); err != nil {
		t.Fatal(err)
	}
	// Only the fixture machine has this session; the live machine does not.
	if !strings.Contains(out.String(), "app-feature") {
		t.Fatalf("status did not read the service:\n%s", out.String())
	}
}

func TestServeStdioRelaysToService(t *testing.T) {
	path := isolate(t)
	serveInProcess(t, path)
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	go serveStdio(path, inR, outW) //nolint:errcheck // ends with the test
	c := transport.Stdio(outR, inW)
	defer c.Close()
	if h := hello(t, c); h.Version != proto.Version {
		t.Fatalf("hello over stdio %+v", h)
	}
}
