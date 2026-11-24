package said

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// standInCommand is the one shell command each stand-in model asks for.
const standInCommand = "touch canary-ran"

// waitingCheck is the check for Codex and OpenCode: matchblox reads only that
// they wait (`asks`), and the builder answers in the pane.
func waitingCheck(screen string) (string, bool) {
	if _, kind := Screen(screen); kind != Permits {
		return "no waiting state", false
	}
	return "", true
}

// canaryCheck is the check for Claude Code, which matchblox answers with a
// digit: the check its live screen fails, or "" and true when matchblox's
// rules read its waiting state, its menu and its options.
func canaryCheck(screen string) (string, bool) {
	if _, kind := Screen(screen); kind != Permits {
		return "no waiting state", false
	}
	// The question must be the menu's own, not the command it asks about.
	if q, ok := Menu(screen); !ok || !strings.HasSuffix(q, "?") || strings.Contains(q, standInCommand) {
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

func menuFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "menus", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCanaryCheck(t *testing.T) {
	screen := menuFixture(t, "claude-bash-2.1.330.txt")
	lastLine := strings.TrimSuffix(screen, " Esc to cancel · Tab to amend\n")
	swapped := strings.Replace(screen,
		"1. Yes\n   2. Yes, and always allow access to <work> from this project",
		"1. Allow access to <work> from this project\n   2. Yes", 1)
	single := " Do you want to proceed?\n ❯ 1. Yes\n\n Esc to cancel · Tab to amend\n"
	idle := " ▐██████   Claude Code v2.1.330\n\n❯ \n\n  ? for shortcuts\n"
	noQuestion := strings.Replace(screen, " Do you want to proceed?\n", "", 1)
	asksTheCommand := strings.Replace(screen, "Do you want to proceed?", "Run "+standInCommand+"?", 1)
	opencode := menuFixture(t, "opencode-bash-1.19.6.txt")
	reworded := strings.NewReplacer("Permission required", "Approval needed", "Allow once", "Run once").Replace(opencode)

	for _, c := range []struct {
		name, screen string
		check        func(string) (string, bool)
		want         string
		ok           bool
	}{
		{"live screen", screen, canaryCheck, "", true},
		{"last line removed", lastLine, canaryCheck, "no menu", false},
		{"option 4 renumbered 5", strings.Replace(screen, "4. No", "5. No", 1), canaryCheck, "wrong options", false},
		{"Yes not first", swapped, canaryCheck, "wrong options", false},
		{"last option not No", strings.Replace(screen, "4. No", "4. Never", 1), canaryCheck, "wrong options", false},
		{"one option", single, canaryCheck, "wrong options", false},
		{"idle prompt", idle, canaryCheck, "no waiting state", false},
		{"no question", noQuestion, canaryCheck, "no menu", false},
		{"the question is the command", asksTheCommand, canaryCheck, "no menu", false},
		{"OpenCode waits", opencode, waitingCheck, "", true},
		{"OpenCode has no Claude Code menu", opencode, canaryCheck, "no menu", false},
		{"OpenCode reworded", reworded, waitingCheck, "no waiting state", false},
		{"idle prompt does not wait", idle, waitingCheck, "no waiting state", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			check, ok := c.check(c.screen)
			if check != c.want || ok != c.ok {
				t.Errorf("check = %q, %v; want %q, %v", check, ok, c.want, c.ok)
			}
		})
	}
}

func TestScrubRemovesTheRunsFolders(t *testing.T) {
	const work, home = "/home/runner/work/_temp/canary/work", "/home/runner"
	screen := " Read " + work + "/main.go\n" +
		" Config at " + home + "/.claude\n" +
		" runner@fv-az123:~$\n"

	got := Scrub(screen, []string{work, home}, []string{"fv-az123", "runner"})
	for _, s := range []string{work, home, "runner", "fv-az123"} {
		if strings.Contains(got, s) {
			t.Errorf("scrub left %q in:\n%s", s, got)
		}
	}
	for _, s := range []string{"<dir>/main.go", "<dir>/.claude", "<user>@<host>"} {
		if !strings.Contains(got, s) {
			t.Errorf("scrub wrote no %q in:\n%s", s, got)
		}
	}
}

func TestScrubKeepsShortNames(t *testing.T) {
	const screen = " Do you want to proceed?\n ❯ 1. Yes\n"
	for _, name := range []string{"", "o", "ye"} {
		if got := Scrub(screen, nil, []string{name, name}); got != screen {
			t.Errorf("scrub with host and user %q changed the screen:\n%s", name, got)
		}
	}
}

// On CI the agents run as the workflow's own user, so its name is scrubbed:
// it must not be a word of the prompt, the command or a config name, or the
// screen an issue asks to paste would not be the one the agent drew.
func TestScrubKeepsThePrompt(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "canary.yml"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`useradd [^\n]* (\w+)\n`).FindSubmatch(b)
	if m == nil {
		t.Fatal("canary.yml makes no user with useradd")
	}
	for _, s := range []string{"make the canary file", standInCommand, "canary/stand-in", "Making the canary file"} {
		if got := Scrub(s, nil, []string{"", string(m[1])}); got != s {
			t.Errorf("scrub with the CI user %q turns %q into %q", m[1], s, got)
		}
	}
}
