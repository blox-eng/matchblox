package mux_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/mux"
	"github.com/blox-eng/matchblox/internal/service"
)

var forbiddenMuxMods = []string{
	"github.com/charmbracelet/x/vt",
	"github.com/creack/pty",
}

// TestNoVtDependency: this extract must not add a vt wrapper or a pty
// module. Match whole module paths, not a substring (plan9stats has "vt").
func TestNoVtDependency(t *testing.T) {
	current := modulePaths(t, readFile(t, filepath.Join(modRoot(t), "go.mod")))
	for _, path := range forbiddenMuxMods {
		if _, ok := current[path]; ok {
			t.Errorf("go.mod requires %s: this extract must not add it", path)
		}
	}
	listed := goListModules(t)
	for _, path := range forbiddenMuxMods {
		if _, ok := listed[path]; ok {
			t.Errorf("go list -m all has %s: this extract must not add it", path)
		}
	}
}

// TestConsoleStartsWithoutHerdr: the default mux is tmux. Constructing
// mux.Tmux{} or service.Service does not look up herdr, and neither package
// imports it.
func TestConsoleStartsWithoutHerdr(t *testing.T) {
	var looked []string
	look := func(name string) (string, error) {
		looked = append(looked, name)
		if name == "herdr" {
			t.Error(`LookPath("herdr")`)
		}
		return "", exec.ErrNotFound
	}

	tm := mux.Tmux{LookPath: look}
	if tm.ViewCmd() != "tmux" {
		t.Fatalf("default mux ViewCmd = %q, want tmux", tm.ViewCmd())
	}
	if _, err := tm.List(); err != nil {
		t.Fatalf("List with no tmux: %v", err)
	}
	if len(looked) == 0 || looked[0] != "tmux" {
		t.Fatalf("List LookPath = %q, want tmux", looked)
	}

	s := service.New(config.Config{}, nil, nil)
	s.LookPath = look
	if s.Mux != nil {
		t.Fatal("New must leave Mux nil so the default mux is tmux")
	}
	_ = service.Service{LookPath: look}

	for _, name := range looked {
		if name == "herdr" {
			t.Fatalf("constructing looked up herdr")
		}
	}

	noHerdrImport(t, "github.com/blox-eng/matchblox/internal/mux")
	noHerdrImport(t, "github.com/blox-eng/matchblox/internal/service")
	noHerdrLookPath(t, filepath.Join(modRoot(t), "internal", "mux"))
	noHerdrLookPath(t, filepath.Join(modRoot(t), "internal", "service"))
}

func noHerdrImport(t *testing.T, pkg string) {
	t.Helper()
	out, err := exec.Command("go", "list", "-json", pkg).Output()
	if err != nil {
		t.Fatalf("go list -json %s: %v", pkg, err)
	}
	var p struct {
		ImportPath string
		Imports    []string
	}
	if err := json.Unmarshal(out, &p); err != nil {
		t.Fatal(err)
	}
	for _, imp := range p.Imports {
		if strings.Contains(imp, "herdr") {
			t.Errorf("%s imports %s", p.ImportPath, imp)
		}
	}
}

func noHerdrLookPath(t *testing.T, dir string) {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var files int
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
			if strings.Contains(line, `LookPath("herdr")`) || strings.Contains(line, `"herdr"`) {
				t.Errorf("%s:%d names herdr", n, i+1)
			}
		}
	}
	if files == 0 {
		t.Fatalf("scanned no production file in %s", dir)
	}
}

func goListModules(t *testing.T) map[string]struct{} {
	t.Helper()
	out, err := exec.Command("go", "list", "-m", "all").Output()
	if err != nil {
		t.Fatalf("go list -m all: %v", err)
	}
	mods := map[string]struct{}{}
	for line := range strings.Lines(string(out)) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		mods[fields[0]] = struct{}{}
	}
	return mods
}

func modulePaths(t *testing.T, modText string) map[string]struct{} {
	t.Helper()
	out := map[string]struct{}{}
	inRequire := false
	for line := range strings.Lines(modText) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "require (") {
			inRequire = true
			continue
		}
		if inRequire {
			if line == ")" {
				inRequire = false
				continue
			}
			fields := strings.Fields(line)
			if len(fields) > 0 {
				out[fields[0]] = struct{}{}
			}
			continue
		}
		if rest, ok := strings.CutPrefix(line, "require "); ok {
			fields := strings.Fields(rest)
			if len(fields) > 0 {
				out[fields[0]] = struct{}{}
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("parsed no module path from go.mod")
	}
	return out
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func modRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	p := strings.TrimSpace(string(out))
	if p == "" || p == os.DevNull {
		t.Fatal("go env GOMOD is empty")
	}
	return filepath.Dir(p)
}
