package demo

import (
	"context"
	"encoding/json"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/transport"
)

// TestANewSessionWritesNothingOutsideTheCopy starts a session as the console's
// N does, with the person's home and state in an empty folder, and only a fake
// agent on PATH, so that no real command can run.
func TestANewSessionWritesNothingOutsideTheCopy(t *testing.T) {
	outside := t.TempDir()
	for _, env := range []string{"HOME", "XDG_STATE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_RUNTIME_DIR"} {
		t.Setenv(env, filepath.Join(outside, strings.ToLower(env)))
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\n"), 0o700); err != nil { //nolint:gosec // an executable, for LookPath
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("TMPDIR", t.TempDir())

	root, err := copyTree("../../testdata/demo")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	a, b := net.Pipe()
	svc, err := newService(root)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = svc.Handle(ctx, transport.NewConn(b)) }()
	c := transport.NewConn(a)
	defer func() { _ = c.Close() }()

	// net.Pipe has no buffer: read the service's hello while sending ours.
	sent := make(chan error, 1)
	go func() { sent <- c.Send(proto.KindHello, "", proto.Hello{Version: proto.Version}) }()
	if _, err := c.Recv(); err != nil {
		t.Fatal(err)
	}
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
	text, err := json.Marshal(proto.NewSession{Where: "/work/flint-auth", Agent: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		sent <- c.Send(proto.KindAct, "1", proto.Act{RecID: "new:session", Confirm: "y", Text: string(text)})
	}()
	for {
		env, err := c.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if env.Kind != proto.KindResult || env.ID != "1" {
			continue
		}
		var r proto.Result
		if err := json.Unmarshal(env.Body, &r); err != nil {
			t.Fatal(err)
		}
		if r.Err != "" {
			t.Errorf("the session did not start, so this proves little: %+v", r)
		}
		break
	}
	if err := <-sent; err != nil {
		t.Fatal(err)
	}

	panes, err := os.ReadFile(filepath.Join(root, "tmux-panes.txt"))
	if err != nil || !strings.Contains(string(panes), "\tclaude\t/work/flint-auth") {
		t.Errorf("the new session is not in the copy's tmux panes (%v):\n%s", err, panes)
	}

	_ = filepath.WalkDir(outside, func(p string, _ fs.DirEntry, err error) error {
		if err == nil && p != outside {
			t.Errorf("the demo wrote outside its copy: %s", p)
		}
		return nil
	})
}
