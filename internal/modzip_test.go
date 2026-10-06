package internal

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestModuleZips: `go install github.com/blox-eng/matchblox/cmd/matchblox@latest`
// zips the module, and a file name the module zip refuses (a ":" from a
// sysfs fixture) breaks the install for everyone. A directory with its own
// go.mod is left out of the zip, as the Go module rules say.
func TestModuleZips(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && rel != "." {
				return filepath.SkipDir // .git and friends
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil && rel != "." {
				return filepath.SkipDir
			}
			return nil
		}
		for _, r := range filepath.ToSlash(rel) {
			if !zipSafe(r) {
				t.Errorf("%s: %q is not allowed in a module zip, so go install fails", rel, r)
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// zipSafe is the character set of golang.org/x/mod/module.CheckFilePath.
func zipSafe(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '/':
		return true
	}
	return strings.ContainsRune("!#$%&()+,-.=@[]^_{}~ ", r)
}
