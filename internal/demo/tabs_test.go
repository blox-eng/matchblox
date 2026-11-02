package demo_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// emptyTexts are what a tab says when it has nothing to show.
var emptyTexts = []string{"No ships yet.", "scanning repositories…", "collecting history", "this service keeps no stoker", "could not read"}

// fillBound is how long a tab may take to fill after its key is pressed.
const fillBound = 2 * time.Second

// tabs are the seven tabs by key, each with a string only its seeded data
// draws: a screen that is blank or still loading cannot pass on absence alone.
var tabs = []struct{ key, name, seeded string }{
	{"2", "Ships", "Rate limits on the public API"},
	{"3", "Machine", "NETWORK AND DISK"},
	{"4", "Git", "9 WORKTREES"},
	{"5", "History", "LAST 2H"},
	{"6", "Panes", "hearth:2.1"},
	{"7", "Stoker", "n starts a night"},
	{"1", "Sessions", "hearth-api"},
}

func TestEveryTabHasData(t *testing.T) {
	for _, size := range [][2]int{{44, 31}, {132, 32}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Setenv("TMPDIR", t.TempDir())
			started := time.Now()
			_, term := startAt(t, size[0], size[1])
			term.waitFor(t, "hearth-api")
			t.Logf("Sessions drew the machine %v after Start", time.Since(started).Round(10*time.Millisecond))
			for _, tab := range tabs {
				from := len(term.text())
				if _, err := term.in.Write([]byte(tab.key)); err != nil {
					t.Fatal(err)
				}
				pressed := time.Now()
				var drawn string
				for !strings.Contains(drawn, tab.seeded) {
					if time.Since(pressed) > fillBound {
						t.Fatalf("%s: %q not drawn within %v; it drew:\n%s", tab.name, tab.seeded, fillBound, drawn)
					}
					time.Sleep(10 * time.Millisecond)
					drawn = term.text()[from:]
				}
				t.Logf("%s filled %v after its key", tab.name, time.Since(pressed).Round(10*time.Millisecond))
				for _, empty := range emptyTexts {
					if strings.Contains(drawn, empty) {
						t.Errorf("%s drew %q:\n%s", tab.name, empty, drawn)
					}
				}
			}
		})
	}
}

// TestEveryAgentPaneReads: a session whose pane has no file opens in the chat
// as "could not read".
func TestEveryAgentPaneReads(t *testing.T) {
	list, err := os.ReadFile(filepath.Join(tree, "tmux-panes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(list)), "\n") {
		f := strings.Split(line, "\t")
		if f[4] == "zsh" {
			continue
		}
		file := filepath.Join(tree, "panes", strings.TrimPrefix(f[0], "%")+".txt")
		if _, err := os.Stat(file); err != nil {
			t.Errorf("%s runs %s and has no pane: %v", f[5], f[4], err)
		}
	}
}
