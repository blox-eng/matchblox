package tmuxtest

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// runsTmux finds a line that starts a tmux process with exec.Command.
var runsTmux = regexp.MustCompile(`exec\.Command(Context)?\(\s*([A-Za-z_.()]+,\s*)?"tmux"`)

// noSocket marks a test's tmux that names no server on purpose.
const noSocket = "no socket on purpose:"

// TestEveryTestTmuxNamesItsServer: a tmux with no socket goes to the server
// that $TMUX names, and a builder runs the tests inside tmux. A test cleanup
// with no socket once could end the builder's server with every agent in it
// (#215). So each tmux a test runs names its server (-L or -S), or only asks
// the version (-V). A line that must run tmux with no socket says so with
// noSocket and its reason. Production code is not test code: it runs tmux
// on the builder's server on purpose. Each package whose tests run tmux also clears $TMUX
// through testhome.Run, so a tmux that a test's subprocess runs cannot
// inherit the builder's server.
func TestEveryTestTmuxNamesItsServer(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	runs := map[string]string{} // package directory → its first test file that runs tmux
	cleared := map[string]bool{}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != root {
				return filepath.SkipDir // .git, .claude worktrees
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		b, err := os.ReadFile(path) //nolint:gosec // a file of this repository
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		test := strings.HasSuffix(path, "_test.go")
		if !test && filepath.Dir(rel) != filepath.Join("internal", "tmuxtest") {
			return nil
		}
		if test && strings.Contains(string(b), "testhome.Run(") || filepath.Dir(rel) == filepath.Join("internal", "testhome") && strings.Contains(string(b), "Run(m)") {
			cleared[filepath.Dir(rel)] = true
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") || !runsTmux.MatchString(line) {
				continue
			}
			if !strings.Contains(line, `"-L"`) && !strings.Contains(line, `"-S"`) && !strings.Contains(line, `"-V"`) && !strings.Contains(line, noSocket) {
				t.Errorf("%s:%d: the tmux names no server: pass -L or -S on this line, or it reaches the builder's server\n\t%s", rel, i+1, strings.TrimSpace(line))
			}
			if _, seen := runs[filepath.Dir(rel)]; test && !seen {
				runs[filepath.Dir(rel)] = rel
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) == 0 {
		t.Fatal("no test runs tmux: the scan reads the wrong folder")
	}
	for dir, file := range runs {
		if !cleared[dir] {
			t.Errorf("%s runs tmux, but its package has no TestMain with testhome.Run: add main_test.go with os.Exit(testhome.Run(m)), so no subprocess inherits $TMUX", file)
		}
	}
}
