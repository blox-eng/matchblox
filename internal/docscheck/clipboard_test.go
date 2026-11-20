package docscheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestNothingSetsSetClipboard: matchblox never changes tmux's set-clipboard.
// Under on, any program in any pane reads the person's last copy by OSC 52
// (measured, spec R5); copies reach the screen by tmux's own load-buffer -w
// instead. A string that names the option in product code is the start of
// such a change: say why in the spec first.
func TestNothingSetsSetClipboard(t *testing.T) {
	var found []string
	checked := 0
	for _, root := range []string{"../../internal", "../../cmd"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			checked++
			ast.Inspect(f, func(n ast.Node) bool {
				if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && strings.Contains(lit.Value, "set-clipboard") {
					found = append(found, path+": "+lit.Value)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if checked < 50 {
		t.Fatalf("the guard read %d Go files: it checks nothing", checked)
	}
	for _, f := range found {
		t.Errorf("product code names set-clipboard: %s", f)
	}
}
