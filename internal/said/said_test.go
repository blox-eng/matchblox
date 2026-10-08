package said

import (
	"strings"
	"testing"
)

// The screens are the agents' own words, read from their binaries and live
// panes: Codex and OpenCode ask for approval in these words.
func TestScreen(t *testing.T) {
	for _, c := range []struct {
		name, screen, line string
		asks               bool
	}{
		{"codex approval", `
› Run the tests
• I'll run the test suite.

  Would you like to run the following command?

  $ go test ./...

› 1. Yes, proceed (y)
  2. Yes, and don't ask again for this command (a)
  3. No, and tell Codex what to do differently (esc)

  Press enter to confirm or esc to cancel
`, "Would you like to run the following command?", true},
		{"opencode permission", `
┃  Permission required
┃  $ rm -rf build
┃
┃  Allow once   Allow always   Reject
   ctrl+f fullscreen  ⇆ select  enter confirm
`, "Permission required", true},
		{"claude permission", `
 Bash command
   rm -rf build
 Do you want to proceed?
 ❯ 1. Yes
   2. No, and tell Claude what to do differently (esc)
`, "Do you want to proceed?", true},
		{"aider confirm", `
Add main.go to the chat? (Y)es/(N)o/(D)on't ask again [Yes]:
`, "Add main.go to the chat? (Y)es/(N)o/(D)on't ask again [Yes]:", true},
		{"the agent asks in its reply", `
⏺ I changed the parser and the tests pass.
  Should I also update the docs?

╭──────────────────────────────────────────╮
│ >                                        │
╰──────────────────────────────────────────╯
  ? for shortcuts
`, "Should I also update the docs?", true},
		{"finished, nothing asked", `
  Done. All 42 tests pass.
                                           ┃
                                           ┃  Ask anything... "Fix broken tests"
                                           ╹▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
                                     ctrl+t variants  tab agents  ctrl+p commands
  /home/u/app:main                                                          1.1.48
`, "Done. All 42 tests pass.", false},
		{"an old question further up", `
  Want me to continue?
  Step one: read the parser.
  Step two: split the tokens.
  Step three: test the edges.
  Step four: fix the edges.
  Step five: run the suite.
  Step six: read the output.
  Step seven: fix the lint.
  Step eight: write the notes.
  Finished the refactor.
`, "Finished the refactor.", false},
		{"only key hints", "\n  ? for shortcuts\n", "", false},
	} {
		line, asks := Screen(c.screen)
		if line != c.line || asks != c.asks {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", c.name, line, asks, c.line, c.asks)
		}
	}
}

// TestReply: Claude Code ends each reply with the progress line, so the
// question sits above it. A reply that asks waits for the person.
func TestReply(t *testing.T) {
	for _, c := range []struct {
		name, text, line string
		asks             bool
	}{
		{"asks above the progress line", "I fixed the parser.\n\n**Shall I open the PR?**\n\nProgress [██████░░] review", "Shall I open the PR?", true},
		{"asks mid-line", "Both are ready. Want me to merge? I can also tag it.\n\nProgress [████████] done", "Both are ready. Want me to merge? I can also tag it.", true},
		{"a numbered choice", "Two ways:\n1. Raise it to 40\n2. Keep 28\n\nWhich one do you want?", "Which one do you want?", true},
		{"done, nothing asked", "All 42 tests pass and the PR is merged.\n\nProgress [████████] done", "All 42 tests pass and the PR is merged.", false},
		{"a question long answered", "Why did it fail?\n" + strings.Repeat("It read the wrong file.\n", 9) + "Fixed now.", "Fixed now.", false},
	} {
		line, asks := Reply(c.text)
		if line != c.line || asks != c.asks {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", c.name, line, asks, c.line, c.asks)
		}
	}
}

// TestControlKeysNeverReachTheConsole: a reply or a screen is not trusted;
// an escape in it would drive the person's terminal.
func TestControlKeysNeverReachTheConsole(t *testing.T) {
	for _, read := range []func(string) (string, bool){Reply, Screen} {
		line, _ := read("Shall I \x1b]52;c;eHg=\x07open it?")
		if strings.ContainsFunc(line, func(r rune) bool { return r < 0x20 || r == 0x7f }) || line != "Shall I ]52;c;eHg=open it?" {
			t.Fatalf("line %q", line)
		}
	}
}
