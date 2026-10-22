//go:build unix

package gitscan

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
		got, _ := Find(context.Background(), roots, FindOpts{})
		done <- got
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

func TestFindStopsWaitingOnAHungFolder(t *testing.T) {
	root := tmp(t)
	mkRepo(t, filepath.Join(root, "a"), "main", "")
	stuck := filepath.Join(root, "z")
	if err := os.MkdirAll(stuck, 0o755); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}, 1), make(chan struct{})
	defer close(release)
	readDir, hangGrace = func(dir string) ([]os.DirEntry, error) {
		if dir == stuck {
			entered <- struct{}{}
			<-release
		}
		return os.ReadDir(dir)
	}, 10*time.Millisecond
	defer func() { // the walk read the hooks before the cleanup swaps them back
		select {
		case <-entered:
		case <-time.After(time.Second):
		}
	}()
	t.Cleanup(func() { readDir, hangGrace = os.ReadDir, 5*time.Second })
	start := time.Now()
	got, errs := Find(context.Background(), []string{root}, FindOpts{Budget: 300 * time.Millisecond})
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("Find waited %s on a folder that never answered", took)
	}
	if !slices.Equal(paths(got), []string{filepath.Join(root, "a")}) || len(errs) != 1 {
		t.Fatalf("got %v, errs %v: want what was found before the hang, and why it stopped", paths(got), errs)
	}
}

func TestFindStaysOnTheRootsDisk(t *testing.T) {
	root := tmp(t)
	mkRepo(t, filepath.Join(root, "local", "a"), "main", "")
	mount := filepath.Join(root, "mnt")
	mkRepo(t, filepath.Join(mount, "b"), "main", "")
	devOf = func(path string) (uint64, bool) {
		if path == mount || strings.HasPrefix(path, mount+string(filepath.Separator)) {
			return 2, true
		}
		return 1, true
	}
	t.Cleanup(func() { devOf = statDev })
	if got := paths(find(t, root)); !slices.Equal(got, []string{filepath.Join(root, "local", "a")}) {
		t.Fatalf("got %v, want nothing from the other disk", got)
	}
	if got := paths(find(t, mount)); !slices.Equal(got, []string{filepath.Join(mount, "b")}) {
		t.Fatalf("got %v: a root on another disk is still walked", got)
	}
}
