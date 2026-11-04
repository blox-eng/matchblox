//go:build cgo

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const tree = "../../testdata/demo"

// drawn collects what the console draws until the read says the end.
func drawn() (*syncBuffer, chan struct{}) {
	b, ended := &syncBuffer{ch: make(chan string, 1024)}, make(chan struct{})
	go func() {
		defer close(ended)
		buf := make([]byte, 64*1024)
		for {
			n := read(buf)
			if n < 0 {
				return
			}
			b.ch <- string(buf[:n])
		}
	}()
	return b, ended
}

type syncBuffer struct {
	ch   chan string
	seen bytes.Buffer
}

func (b *syncBuffer) waitFor(t *testing.T, want string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for !strings.Contains(b.seen.String(), want) {
		select {
		case s := <-b.ch:
			b.seen.WriteString(s)
		case <-deadline:
			t.Fatalf("the console never drew %q", want)
		}
	}
}

func startDemo(t *testing.T) (*syncBuffer, chan struct{}) {
	t.Helper()
	t.Setenv("TMPDIR", t.TempDir())
	if code := start(tree, 44, 31); code != 0 {
		t.Fatalf("start: %d: %s", code, errorText())
	}
	t.Cleanup(stop)
	b, ended := drawn()
	b.waitFor(t, "hearth-api")
	return b, ended
}

// endsWithin waits for the reads to end, emptying what is left to read.
func endsWithin(t *testing.T, b *syncBuffer, ended chan struct{}) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-b.ch:
		case <-ended:
			return
		case <-deadline:
			t.Fatal("the reads did not end")
		}
	}
}

func TestAConsoleThatDiesSaysWhyBeforeTheReadEnds(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root removes a read-only folder")
	}
	b, ended := startDemo(t)
	copies, err := filepath.Glob(filepath.Join(os.TempDir(), "matchblox-demo-*"))
	if err != nil || len(copies) != 1 {
		t.Fatalf("copies %v, %v", copies, err)
	}
	// A failed removal is the one failure of a run a test can cause from here.
	home := filepath.Join(copies[0], "home")
	if err := os.Chmod(home, 0o500); err != nil { //nolint:gosec // a folder that cannot be emptied
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(home, 0o700) }) //nolint:gosec // so the test's folder can go

	write([]byte("q"))
	endsWithin(t, b, ended)

	if why := errorText(); !strings.Contains(why, copies[0]) {
		t.Errorf("the reason at the end of the reads = %q, want it to name %s", why, copies[0])
	}
}

func TestACleanStopLeavesNoReason(t *testing.T) {
	b, ended := startDemo(t)
	stop()
	endsWithin(t, b, ended)
	if why := errorText(); why != "" {
		t.Errorf("a clean stop says %q", why)
	}
}

func TestAfterAConsoleQuitsAWriteReturnsAndAStartWorks(t *testing.T) {
	b, ended := startDemo(t)
	write([]byte("q"))
	endsWithin(t, b, ended)

	wrote := make(chan struct{})
	go func() { // the first may go to the reader the console left behind
		write([]byte("x"))
		write([]byte("x"))
		close(wrote)
	}()
	select {
	case <-wrote:
	case <-time.After(5 * time.Second):
		t.Fatal("a write to a console that quit hung: its input pipe is still open")
	}

	if code := start(tree, 44, 31); code != 0 {
		t.Fatalf("start after a quit: %d: %s", code, errorText())
	}
	again, _ := drawn()
	again.waitFor(t, "hearth-api")
}
