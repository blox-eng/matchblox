package said

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// canaryCheck is the check an agent's live screen fails, or "" and true when
// matchblox's rules read its waiting state, its menu and its options.
func canaryCheck(screen string) (string, bool) {
	if _, kind := Screen(screen); kind != Permits {
		return "no waiting state", false
	}
	if _, ok := Menu(screen); !ok {
		return "no menu", false
	}
	var labels []string
	for _, l := range menuLines(screen) {
		n, label, ok := option(l)
		if !ok {
			continue
		}
		if n != len(labels)+1 {
			return "wrong options", false
		}
		labels = append(labels, strings.ToLower(label))
	}
	if len(labels) < 2 || !strings.HasPrefix(labels[0], "yes") || !strings.HasPrefix(labels[len(labels)-1], "no") {
		return "wrong options", false
	}
	return "", true
}

func TestCanaryCheck(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "menus", "claude-bash-2.1.330.txt"))
	if err != nil {
		t.Fatal(err)
	}
	screen := string(b)
	lastLine := strings.TrimSuffix(screen, " Esc to cancel · Tab to amend\n")
	swapped := strings.Replace(screen,
		"1. Yes\n   2. Yes, and always allow access to <work> from this project",
		"1. Allow access to <work> from this project\n   2. Yes", 1)
	single := " Do you want to proceed?\n ❯ 1. Yes\n\n Esc to cancel · Tab to amend\n"
	idle := " ▐██████   Claude Code v2.1.330\n\n❯ \n\n  ? for shortcuts\n"

	for _, c := range []struct {
		name, screen, check string
		ok                  bool
	}{
		{"live screen", screen, "", true},
		{"last line removed", lastLine, "no menu", false},
		{"option 4 renumbered 5", strings.Replace(screen, "4. No", "5. No", 1), "wrong options", false},
		{"Yes not first", swapped, "wrong options", false},
		{"last option not No", strings.Replace(screen, "4. No", "4. Never", 1), "wrong options", false},
		{"one option", single, "wrong options", false},
		{"idle prompt", idle, "no waiting state", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			check, ok := canaryCheck(c.screen)
			if check != c.check || ok != c.ok {
				t.Errorf("canaryCheck = %q, %v; want %q, %v", check, ok, c.check, c.ok)
			}
		})
	}
}
