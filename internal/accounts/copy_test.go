package accounts

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// setFor sets *v to x for the test.
func setFor[T any](t *testing.T, v *T, x T) {
	t.Helper()
	old := *v
	*v = x
	t.Cleanup(func() { *v = old })
}

func link(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCopyDoesNotFollowTheDefaultThroughLinks(t *testing.T) {
	for _, agent := range []string{"claude", "cursor-agent"} {
		t.Run(agent, func(t *testing.T) {
			home := t.TempDir()
			def := DefaultDir(home, agent)
			write(t, filepath.Join(def, "skills", "pack", "x", "SKILL.md"), "v1")
			// An absolute link, as the builder's own ~/.claude has them.
			link(t, filepath.Join(def, "skills", "pack", "x"), filepath.Join(def, "skills", "x"))
			// A relative one, and one through the default's resolved path.
			link(t, filepath.Join("skills", "pack", "x", "SKILL.md"), filepath.Join(def, "RULES.md"))
			real, err := filepath.EvalSymlinks(def)
			if err != nil {
				t.Fatal(err)
			}
			link(t, filepath.Join(real, "skills", "pack"), filepath.Join(def, "pack"))
			f := Folder(home, agent, "work")
			rep, err := Prepare(home, agent, f, Copy)
			if err != nil {
				t.Fatal(err)
			}
			if len(rep.Shared) != 0 || len(rep.Skipped) != 0 {
				t.Errorf("report = %+v, want nothing shared or skipped", rep)
			}
			acct := f
			if agent == "cursor-agent" {
				acct = filepath.Join(f, "home", ".cursor")
			}
			write(t, filepath.Join(def, "skills", "pack", "x", "SKILL.md"), "v2")
			for _, p := range []string{
				filepath.Join("skills", "x", "SKILL.md"),
				"RULES.md",
				filepath.Join("pack", "x", "SKILL.md"),
			} {
				if got := read(t, filepath.Join(acct, p)); got != "v1" {
					t.Errorf("%s reads %q, want the account's own v1", p, got)
				}
			}
			for _, l := range []string{filepath.Join("skills", "x"), "RULES.md", "pack"} {
				got, err := os.Readlink(filepath.Join(acct, l))
				if err != nil || filepath.IsAbs(got) {
					t.Errorf("%s -> %q, %v: want a relative link", l, got, err)
				}
			}
		})
	}
}

func TestCopyNamesLinksOutsideTheDefault(t *testing.T) {
	home := t.TempDir()
	def := fakeClaude(t, home)
	out := filepath.Join(home, "elsewhere", "notes.md")
	write(t, out, "x")
	link(t, out, filepath.Join(def, "notes.md"))
	ownTarget := filepath.Join(def, "projects", "p")
	write(t, filepath.Join(ownTarget, "mark"), "the default's")
	link(t, ownTarget, filepath.Join(def, "skills", "proj"))
	// The same two, written relative: kept, they still reach the default.
	link(t, filepath.Join("..", "elsewhere", "notes.md"), filepath.Join(def, "rel-notes.md"))
	link(t, filepath.Join("..", "projects", "p"), filepath.Join(def, "skills", "rel-proj"))
	f := Folder(home, "claude", "work")
	if _, err := Prepare(home, "claude", f, Copy); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(f, "projects", "p", "mark"), "the account's")
	write(t, filepath.Join(home, ".matchblox", "accounts", "claude", "elsewhere", "notes.md"), "not the default's")
	write(t, out, "the default's")
	for l, want := range map[string]string{
		"notes.md": out, "rel-notes.md": out,
		filepath.Join("skills", "proj", "mark"): filepath.Join(ownTarget, "mark"), filepath.Join("skills", "rel-proj", "mark"): filepath.Join(ownTarget, "mark"),
	} {
		if got := read(t, filepath.Join(f, l)); got != "the default's" {
			t.Errorf("%s reads %q, want %s", l, got, want)
		}
	}
	rep, err := Prepare(home, "claude", Folder(home, "claude", "again"), Copy)
	if err != nil {
		t.Fatal(err)
	}
	f = Folder(home, "claude", "again")
	for l, target := range map[string]string{"notes.md": out, filepath.Join("skills", "proj"): ownTarget, "rel-notes.md": out, filepath.Join("skills", "rel-proj"): ownTarget} {
		got, err := os.Readlink(filepath.Join(f, l))
		if err != nil || got != target {
			t.Errorf("%s -> %q, %v: want it left on %s", l, got, err, target)
		}
		if !slices.ContainsFunc(rep.Shared, func(s string) bool { return strings.Contains(s, filepath.Join(f, l)) }) {
			t.Errorf("Shared = %q, does not name %s", rep.Shared, l)
		}
	}
	if !slices.ContainsFunc(rep.Shared, func(s string) bool { return strings.Contains(s, "projects") && strings.Contains(s, "own") }) {
		t.Errorf("Shared = %q, does not say why the link to projects stays", rep.Shared)
	}
}

// skipped is the paths of rep's skips.
func skipped(rep Report) []string {
	var ps []string
	for _, s := range rep.Skipped {
		ps = append(ps, s.Path)
	}
	return ps
}

func TestCopyStopsAtItsBound(t *testing.T) {
	t.Run("entries", func(t *testing.T) {
		home := t.TempDir()
		def := DefaultDir(home, "claude")
		for _, n := range []string{"a/1", "a/2", "a/3", "b", "c"} {
			write(t, filepath.Join(def, n), "x")
		}
		setFor(t, &copyLimit, Limit{Entries: 3, Bytes: CopyLimit.Bytes, Time: CopyLimit.Time})
		f := Folder(home, "claude", "work")
		rep, err := Prepare(home, "claude", f, Copy)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range []string{"a/1", "a/2"} {
			if !exists(filepath.Join(f, n)) {
				t.Errorf("%s was not copied", n)
			}
		}
		want := []string{filepath.Join(def, "a", "3"), filepath.Join(def, "b"), filepath.Join(def, "c")}
		for _, n := range []string{"a/3", "b", "c"} {
			if exists(filepath.Join(f, n)) {
				t.Errorf("%s was copied past the bound", n)
			}
		}
		if got := skipped(rep); !slices.Equal(got, want) {
			t.Errorf("skipped %q, want %q", got, want)
		}
		for _, s := range rep.Skipped {
			if !strings.Contains(s.Why, "3 entries") {
				t.Errorf("%s: why = %q", s.Path, s.Why)
			}
		}
	})
	t.Run("time", func(t *testing.T) {
		home := t.TempDir()
		def := DefaultDir(home, "claude")
		for _, n := range []string{"a", "b", "c"} {
			write(t, filepath.Join(def, n), "x")
		}
		// Each reading of the clock is a minute after the last: the copy
		// starts, copies a, and is past 90 seconds at b.
		at := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
		setFor(t, &now, func() time.Time { at = at.Add(time.Minute); return at })
		f := Folder(home, "claude", "work")
		rep, err := Prepare(home, "claude", f, Copy)
		if err != nil {
			t.Fatal(err)
		}
		if !exists(filepath.Join(f, "a")) {
			t.Error("a was not copied")
		}
		want := []string{filepath.Join(def, "b"), filepath.Join(def, "c")}
		if got := skipped(rep); !slices.Equal(got, want) {
			t.Errorf("skipped %q, want %q", got, want)
		}
		for _, s := range rep.Skipped {
			if !strings.Contains(s.Why, "1m30s") {
				t.Errorf("%s: why = %q", s.Path, s.Why)
			}
		}
	})
}

func TestCopyFallsBackWhenCloneIsRefused(t *testing.T) {
	for _, no := range refusals {
		home := t.TempDir()
		def := DefaultDir(home, "claude")
		write(t, filepath.Join(def, "a"), "12345")
		write(t, filepath.Join(def, "b"), "12345")
		if err := os.Chmod(filepath.Join(def, "a"), 0o750); err != nil {
			t.Fatal(err)
		}
		setFor(t, &clone, func(from, to string, _ fs.FileMode) error {
			return &fs.PathError{Op: "clone", Path: to, Err: no}
		})
		// Bytes count only byte copies: a fits, b would pass 8 bytes.
		setFor(t, &copyLimit, Limit{Entries: CopyLimit.Entries, Bytes: 8, Time: CopyLimit.Time})
		f := Folder(home, "claude", "work")
		rep, err := Prepare(home, "claude", f, Copy)
		if err != nil {
			t.Fatalf("%v: %v", no, err)
		}
		if got := read(t, filepath.Join(f, "a")); got != "12345" {
			t.Errorf("%v: a = %q", no, got)
		}
		if fi, err := os.Stat(filepath.Join(f, "a")); err != nil || fi.Mode().Perm() != 0o750 {
			t.Errorf("%v: a's mode = %v, %v", no, fi.Mode().Perm(), err)
		}
		if got := skipped(rep); !slices.Equal(got, []string{filepath.Join(def, "b")}) {
			t.Errorf("%v: skipped %q, want b past the byte bound", no, got)
		}
	}
	// A clone is not counted.
	home := t.TempDir()
	def := DefaultDir(home, "claude")
	write(t, filepath.Join(def, "a"), "12345")
	write(t, filepath.Join(def, "b"), "12345")
	setFor(t, &clone, func(from, to string, perm fs.FileMode) error {
		b, err := os.ReadFile(from)
		if err != nil {
			return err
		}
		return os.WriteFile(to, b, perm)
	})
	setFor(t, &copyLimit, Limit{Entries: CopyLimit.Entries, Bytes: 8, Time: CopyLimit.Time})
	rep, err := Prepare(home, "claude", Folder(home, "claude", "work"), Copy)
	if err != nil || len(rep.Skipped) != 0 {
		t.Errorf("cloned copy: %+v, %v", rep, err)
	}
}

// TestCloneFileClonesOrIsRefused runs the platform's clone: it copies the
// file with its mode, or it is refused and the copy falls back.
func TestCloneFileClonesOrIsRefused(t *testing.T) {
	d := t.TempDir()
	from := filepath.Join(d, "from")
	write(t, from, "body")
	if err := os.Chmod(from, 0o750); err != nil {
		t.Fatal(err)
	}
	to := filepath.Join(d, "to")
	err := cloneFile(from, to, 0o750)
	if refused(err) {
		t.Skipf("this file system refuses a clone: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, to); got != "body" {
		t.Errorf("clone reads %q", got)
	}
	if fi, err := os.Stat(to); err != nil || fi.Mode().Perm() != 0o750 {
		t.Errorf("clone's mode = %v, %v", fi.Mode().Perm(), err)
	}
	t.Log("cloned copy-on-write")
}

func TestRelinkNamesEveryEntryNotLinked(t *testing.T) {
	home := t.TempDir()
	def := fakeClaude(t, home)
	dst := t.TempDir()
	if err := os.Chmod(dst, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dst, 0o700) })
	_, err := share(def, dst, LikeDefault, func(n string) bool { return isOwn("claude", n) }, false)
	if err == nil {
		t.Fatal("no error from a folder that takes no link")
	}
	for _, n := range []string{"settings.json", "skills"} {
		if !strings.Contains(err.Error(), filepath.Join(dst, n)) {
			t.Errorf("error %q does not name %s", err, n)
		}
	}
}

// refuseClones makes every clone refused, so each file is copied byte by
// byte.
func refuseClones(t *testing.T) {
	t.Helper()
	setFor(t, &clone, func(from, to string, _ fs.FileMode) error {
		return &fs.PathError{Op: "clone", Path: to, Err: refusals[0]}
	})
}

func TestALongByteCopyStopsAtTheDeadline(t *testing.T) {
	home := t.TempDir()
	def := DefaultDir(home, "claude")
	write(t, filepath.Join(def, "big"), "abcdef")
	refuseClones(t)
	setFor(t, &chunk, 2)
	// Each reading of the clock is a minute after the last: the copy
	// starts, begins big, and is past 90 seconds after its first chunk.
	at := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	setFor(t, &now, func() time.Time { at = at.Add(time.Minute); return at })
	f := Folder(home, "claude", "work")
	rep, err := Prepare(home, "claude", f, Copy)
	if err != nil {
		t.Fatal(err)
	}
	if es, _ := os.ReadDir(f); len(es) != 1 {
		t.Errorf("the account has %d entries, want only the marker: a half file stayed", len(es))
	}
	if len(rep.Skipped) != 1 || rep.Skipped[0].Path != filepath.Join(def, "big") || !strings.Contains(rep.Skipped[0].Why, "1m30s") {
		t.Errorf("skipped %+v, want big past the time", rep.Skipped)
	}
}

func TestCopyNamesWhatIsNoFileFolderOrLink(t *testing.T) {
	home := t.TempDir()
	def := DefaultDir(home, "claude")
	write(t, filepath.Join(def, "a"), "x")
	// A FIFO stands for a socket or a device: none can be copied.
	if err := syscall.Mkfifo(filepath.Join(def, "pipe"), 0o600); err != nil {
		t.Skipf("no FIFO here: %v", err)
	}
	rep, err := Prepare(home, "claude", Folder(home, "claude", "work"), Copy)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Skipped) != 1 || rep.Skipped[0].Path != filepath.Join(def, "pipe") || rep.Skipped[0].Why != "not a file, folder or link" {
		t.Errorf("skipped %+v, want the pipe named", rep.Skipped)
	}
}

func TestCopyNamesALinkWhoseTargetWasLeftOut(t *testing.T) {
	home := t.TempDir()
	def := DefaultDir(home, "claude")
	link(t, filepath.Join(def, "z", "x"), filepath.Join(def, "a"))
	write(t, filepath.Join(def, "z", "x"), "x")
	setFor(t, &copyLimit, Limit{Entries: 1, Bytes: CopyLimit.Bytes, Time: CopyLimit.Time})
	rep, err := Prepare(home, "claude", Folder(home, "claude", "work"), Copy)
	if err != nil {
		t.Fatal(err)
	}
	if got := skipped(rep); !slices.Contains(got, filepath.Join(def, "a")) {
		t.Errorf("skipped %q, does not name the link a to what was left out", got)
	}
}

func TestACutCopyIsMarkedIncomplete(t *testing.T) {
	home := t.TempDir()
	def := DefaultDir(home, "claude")
	for _, n := range []string{"a", "b"} {
		write(t, filepath.Join(def, n), "x")
	}
	whole := Folder(home, "claude", "whole")
	if _, err := Prepare(home, "claude", whole, Copy); err != nil {
		t.Fatal(err)
	}
	if SetupOf(whole) != Copy || Incomplete(whole) {
		t.Errorf("a whole copy: %q, incomplete %v", SetupOf(whole), Incomplete(whole))
	}
	setFor(t, &copyLimit, Limit{Entries: 1, Bytes: CopyLimit.Bytes, Time: CopyLimit.Time})
	cut := Folder(home, "claude", "cut")
	if _, err := Prepare(home, "claude", cut, Copy); err != nil {
		t.Fatal(err)
	}
	if SetupOf(cut) != Copy || !Incomplete(cut) {
		t.Errorf("a cut copy: %q, incomplete %v", SetupOf(cut), Incomplete(cut))
	}
}

// tree is every path under dir with its content ("" for a folder, the
// target for a link), but SetupFile.
func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	m := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		switch {
		case rel == "." || rel == SetupFile:
		case d.Type()&fs.ModeSymlink != 0:
			got, _ := os.Readlink(p)
			m[rel] = "-> " + filepath.Base(got)
		case d.IsDir():
			m[rel] = ""
		default:
			m[rel] = read(t, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestACopyResumesWhereItWasCut(t *testing.T) {
	home := t.TempDir()
	def := DefaultDir(home, "claude")
	for _, n := range []string{"a/1", "a/2", "a/b/3", "c", "d/4"} {
		write(t, filepath.Join(def, n), n)
	}
	link(t, filepath.Join(def, "d", "4"), filepath.Join(def, "e"))
	f := Folder(home, "claude", "work")
	setFor(t, &copyLimit, Limit{Entries: 3, Bytes: CopyLimit.Bytes, Time: CopyLimit.Time})
	if _, err := Prepare(home, "claude", f, Copy); err != nil {
		t.Fatal(err)
	}
	if !Incomplete(f) {
		t.Fatal("the copy was not cut")
	}
	copyLimit = CopyLimit
	rep, err := Prepare(home, "claude", f, Copy)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Skipped) != 0 {
		t.Errorf("skipped %+v", rep.Skipped)
	}
	if got, want := tree(t, f), tree(t, def); !maps.Equal(got, want) {
		t.Errorf("account %v, want the default's %v", got, want)
	}
	if Incomplete(f) || SetupOf(f) != Copy {
		t.Errorf("after the resume: %q, incomplete %v", SetupOf(f), Incomplete(f))
	}
}

func TestCursorCopiesItsDefaultWhenHomeCannotBeMirrored(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".cursor", "hooks.json"), "{}")
	if err := os.Chmod(home, 0o300); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(home, 0o700) })
	c := Folder(home, "cursor-agent", "work")
	_, err := Prepare(home, "cursor-agent", c, Copy)
	if err == nil || !strings.Contains(err.Error(), home) {
		t.Errorf("err = %v, want the home that was not mirrored named", err)
	}
	if got := read(t, filepath.Join(c, "home", ".cursor", "hooks.json")); got != "{}" {
		t.Errorf("hooks.json = %q", got)
	}
}
