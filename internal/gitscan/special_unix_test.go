//go:build unix

package gitscan

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"
)

func mkfifo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skip("no fifo here:", err)
	}
}

// findSoon fails if Find blocks: a pipe read would never end.
func findSoon(t *testing.T, roots ...string) []Checkout {
	t.Helper()
	done := make(chan []Checkout, 1)
	go func() {
		done <- Find(context.Background(), roots, FindOpts{}).Repos
	}()
	select {
	case got := <-done:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("Find hung on a special file")
		return nil
	}
}

func TestFindSkipsSpecialGitFiles(t *testing.T) {
	root := tmp(t)
	mkfifo(t, filepath.Join(root, "pipe", ".git"))
	mkfifo(t, filepath.Join(root, "head", ".git", "HEAD"))
	mkRepo(t, filepath.Join(root, "other"), "main", "")
	if err := os.MkdirAll(filepath.Join(root, "link"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "other", ".git"), filepath.Join(root, "link", ".git")); err != nil {
		t.Fatal(err)
	}
	if got := paths(findSoon(t, root)); !slices.Equal(got, []string{filepath.Join(root, "other")}) {
		t.Fatalf("got %v, want only the real checkout", got)
	}
}
