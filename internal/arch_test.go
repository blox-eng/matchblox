package internal

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const mod = "github.com/blox-eng/matchblox/internal/"

type pkg struct {
	ImportPath string
	Dir        string
	Imports    []string
	GoFiles    []string
}

func listPackages(t *testing.T) map[string]pkg {
	t.Helper()
	out, err := exec.Command("go", "list", "-json", "./...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	pkgs := map[string]pkg{}
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p pkg
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		pkgs[strings.TrimPrefix(p.ImportPath, mod)] = p
	}
	return pkgs
}

// TestArchBoundaries holds design §4.2: the console draws only what the
// service sends, and the service does not depend on the console.
func TestArchBoundaries(t *testing.T) {
	pkgs := listPackages(t)
	app, ok := pkgs["app"]
	if !ok {
		t.Fatal("no internal/app package")
	}
	for _, imp := range app.Imports {
		switch strings.TrimPrefix(imp, mod) {
		case "procfs", "host", "gitscan", "actions", "panes", "hooks", "setup", "stoker", "service":
			t.Errorf("internal/app imports %s: the console must not read the machine", imp)
		}
	}
	for _, f := range app.GoFiles {
		b, err := os.ReadFile(filepath.Join(app.Dir, f))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(b, []byte("sample.Sampler")) {
			t.Errorf("internal/app/%s uses sample.Sampler: the console must not sample", f)
		}
	}
	for name, p := range pkgs {
		if name == "app" {
			continue
		}
		for _, imp := range p.Imports {
			if imp == mod+"app" {
				t.Errorf("%s imports internal/app", p.ImportPath)
			}
		}
	}
}
