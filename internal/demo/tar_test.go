package demo_test

import (
	"archive/tar"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tarOf writes dir as one tar file, symlinks kept as entries.
func tarOf(t *testing.T, dir string) string {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	// As tar -C dir -cf out . writes it: every name begins with "./".
	if err := tw.WriteHeader(&tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == dir {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			if link, err = os.Readlink(path); err != nil {
				return err
			}
		}
		h, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		h.Name = "./" + filepath.ToSlash(rel)
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			_, err = tw.Write(b)
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return writeTar(t, buf.Bytes())
}

func writeTar(t *testing.T, b []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "machine.tar")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// tarWith is a tar of the given headers, each with its body.
func tarWith(t *testing.T, entries ...tar.Header) string {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, h := range entries {
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return writeTar(t, buf.Bytes())
}

func TestATarDrawsTheFirstScreenOfTheFolder(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	fromFolder, term := start(t)
	term.waitFor(t, "SESSIONS 7", "hearth-api", "start a new session")
	want := firstScreen(term.text())
	fromFolder.Stop()

	_, fromTar, err := tryStartTree(t, tarOf(t, tree), 44, 31)
	if err != nil {
		t.Fatal(err)
	}
	fromTar.waitFor(t, "SESSIONS 7", "hearth-api", "start a new session")
	if got := firstScreen(fromTar.text()); got != want {
		t.Errorf("the tar's first screen differs from the folder's.\nfolder:\n%s\ntar:\n%s", want, got)
	}
}

func TestATarEntryThatEscapesIsRefusedByName(t *testing.T) {
	for _, name := range []string{"../escaped", "proc/../../escaped", "/tmp/escaped"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("TMPDIR", t.TempDir())
			path := tarWith(t, tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644})

			_, _, err := tryStartTree(t, path, 44, 31)

			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("want an error naming %q, got %v", name, err)
			}
			if left := tempCopies(t); len(left) != 0 {
				t.Errorf("the refused tar left %v", left)
			}
		})
	}
}

func TestATarNeverWritesThroughASymlink(t *testing.T) {
	outside := t.TempDir()
	t.Setenv("TMPDIR", t.TempDir())
	path := tarWith(t,
		tar.Header{Name: "door", Typeflag: tar.TypeSymlink, Linkname: outside, Mode: 0o777},
		tar.Header{Name: "door/planted", Typeflag: tar.TypeReg, Mode: 0o644},
	)

	_, _, err := tryStartTree(t, path, 44, 31)

	if err == nil || !strings.Contains(err.Error(), "door/planted") {
		t.Errorf("want an error naming door/planted, got %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(outside, "planted")); statErr == nil {
		t.Error("the tar wrote through a symlink to a folder outside the copy")
	}
}

func TestATruncatedTarIsAnErrorNotABlankScreen(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	whole, err := os.ReadFile(tarOf(t, tree))
	if err != nil {
		t.Fatal(err)
	}
	for _, cut := range []int{len(whole) / 2, len(whole) - 1024 - 100} {
		path := writeTar(t, whole[:cut])

		_, _, err := tryStartTree(t, path, 44, 31)

		if err == nil {
			t.Errorf("a tar cut at %d of %d bytes started", cut, len(whole))
		}
		if left := tempCopies(t); len(left) != 0 {
			t.Errorf("the failed start left %v", left)
		}
	}
}
