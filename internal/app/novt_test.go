package app

import (
	"os/exec"
	"strings"
	"testing"
)

// TestNoVtDependency: the desk must not add a vt wrapper or a new pty
// module. Match whole module paths, not a substring (plan9stats has "vt").
func TestNoVtDependency(t *testing.T) {
	out, err := exec.Command("go", "list", "-m", "all").Output()
	if err != nil {
		t.Fatalf("go list -m all: %v", err)
	}
	for line := range strings.Lines(string(out)) {
		path := strings.Fields(strings.TrimSpace(line))
		if len(path) == 0 {
			continue
		}
		switch path[0] {
		case "github.com/charmbracelet/x/vt", "github.com/creack/pty":
			t.Errorf("go list -m all has %s", path[0])
		}
	}
}
