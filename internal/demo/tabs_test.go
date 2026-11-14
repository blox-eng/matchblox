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
var emptyTexts = []string{"scanning repositories…", "collecting history", "this service keeps no stoker", "could not read"}

// fillBound is how long a tab may take to fill after its key is pressed.
const fillBound = 2 * time.Second

// tabs are the screens in turn, each with the keys that open it from the
// one before and a string only its seeded data draws: a screen that is
// blank or still loading cannot pass on absence alone. Tab turns between
// the board and Machine (a digit on the board answers its card); the
// stoker, Git, History and Panes are Machine's rows, in that order.
var tabs = []struct{ key, name, seeded string }{
	{"\t", "Machine", "load 6.2"},
	{"j\r", "Git", "9 WORKTREES"},             // Machine's second row
	{"\t\tj\r", "History", "LAST 2H"},         // the board, Machine on Git's row, the next
	{"\t\tj\r", "Panes", "hearth:2.1"},        // the board, Machine on History's row, the next
	{"\t", "Board", "flint-auth"},             // Tab from a view of Machine
	{"\tkkk\r", "Stoker", "n starts a night"}, // Machine on Panes' row, up to the first
}

func TestEveryTabHasData(t *testing.T) {
	for _, size := range [][2]int{{44, 31}, {132, 32}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Setenv("TMPDIR", t.TempDir())
			started := time.Now()
			_, term := startAt(t, size[0], size[1])
			term.waitFor(t, "flint-auth")
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

// TestATabPressedAtTheFirstDrawFillsAtOnce presses each tab key the moment
// the console first draws, and presses it again every 100 ms as a person
// would, as the console takes no key until its first state has come. It times
// how long a loading text can show.
func TestATabPressedAtTheFirstDrawFillsAtOnce(t *testing.T) {
	for _, tab := range tabs {
		if tab.key != "\t" {
			continue // a view of Machine opens from its row: no one key to press at the first draw
		}
		t.Run(tab.name, func(t *testing.T) {
			t.Setenv("TMPDIR", t.TempDir())
			_, term := startAt(t, 44, 31)
			for term.text() == "" {
				time.Sleep(time.Millisecond)
			}
			pressed := time.Now()
			var emptyAt, lastKey time.Time
			for !strings.Contains(term.text(), tab.seeded) {
				if time.Since(lastKey) >= 100*time.Millisecond {
					if _, err := term.in.Write([]byte(tab.key)); err != nil {
						t.Fatal(err)
					}
					lastKey = time.Now()
				}
				if time.Since(pressed) > 10*time.Second {
					t.Fatalf("%q not drawn within 10s of the first draw", tab.seeded)
				}
				if emptyAt.IsZero() && hasEmpty(term.text()) {
					emptyAt = time.Now()
				}
				time.Sleep(5 * time.Millisecond)
			}
			shown := time.Duration(0)
			if !emptyAt.IsZero() {
				shown = time.Since(emptyAt)
			}
			t.Logf("%s filled %v after the first draw; a loading text showed up to %v", tab.name, time.Since(pressed).Round(10*time.Millisecond), shown.Round(10*time.Millisecond))
			if shown > time.Second {
				t.Errorf("a loading text showed for %v", shown)
			}
		})
	}
}

func hasEmpty(text string) bool {
	for _, e := range emptyTexts {
		if strings.Contains(text, e) {
			return true
		}
	}
	return false
}
