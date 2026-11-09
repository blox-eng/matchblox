package internal

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
		case "procfs", "host", "gitscan", "ships", "actions", "panes", "hooks", "setup", "stoker", "service", "clone":
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
		// demo joins the console and the service, as cmd/matchblox does.
		if name != "app" && name != "demo" && slices.Contains(p.Imports, mod+"app") {
			t.Errorf("%s imports internal/app", p.ImportPath)
		}
		if slices.Contains(p.Imports, mod+"demo") {
			t.Errorf("%s imports internal/demo: only a main joins the console and the service", p.ImportPath)
		}
	}
}

// TestChainAndSendNameNoTmux: chains and sends reach panes only through
// internal/mux, so another multiplexer can run them.
func TestChainAndSendNameNoTmux(t *testing.T) {
	for _, f := range []string{"peersend.go", "chains.go", "chainnews.go", "notice.go"} {
		b, err := os.ReadFile(filepath.Join("service", f))
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{"sample.Tmux", "tmuxOut", "panes.Literal", "panes.Keys", "s.Capture(", `"tmux"`} {
			if bytes.Contains(b, []byte(bad)) {
				t.Errorf("service/%s names %s: go through s.mux()", f, bad)
			}
		}
	}
}
