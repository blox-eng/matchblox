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
	for _, f := range []string{"peersend.go", "chains.go", "chainnews.go", "notice.go", "paste.go", "copywatch.go", "size.go"} {
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

// TestPaneActsNameNoTmux: no production file outside internal/mux builds a
// tmux argv or reaches a pane by the tmux helpers. A pane act goes through
// internal/mux, so another multiplexer can run it. English copy, JSON field
// names and column headers say "tmux" without building argv: the denylist
// is the argv forms below, not the word. internal/setup, internal/remote,
// internal/demo and internal/tmuxtest name tmux on purpose and stay out.
func TestPaneActsNameNoTmux(t *testing.T) {
	bad := []string{"sample.Tmux", "TmuxOn", "tmuxOut", `[]string{"tmux"`}
	var files int
	for _, dir := range []string{"sample", "panes", "service", "app", "proto", "advice", filepath.Join("..", "cmd", "matchblox")} {
		ents, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range ents {
			n := e.Name()
			if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, n))
			if err != nil {
				t.Fatal(err)
			}
			files++
			for i, line := range strings.Split(string(b), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					continue
				}
				for _, w := range bad {
					if strings.Contains(line, w) {
						t.Errorf("%s/%s:%d names %s: go through internal/mux", dir, n, i+1, w)
					}
				}
			}
		}
	}
	if files == 0 {
		t.Fatal("the denylist scanned no file")
	}
}

// TestMuxImportsNeitherSampleNorPanes: sample and panes call mux, so mux
// must not call them back.
func TestMuxImportsNeitherSampleNorPanes(t *testing.T) {
	m, ok := listPackages(t)["mux"]
	if !ok {
		t.Fatal("no internal/mux package")
	}
	for _, imp := range m.Imports {
		switch strings.TrimPrefix(imp, mod) {
		case "sample", "panes":
			t.Errorf("internal/mux imports %s", imp)
		}
	}
}
