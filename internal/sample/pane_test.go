package sample

import (
	"strings"
	"testing"
	"time"
)

// The screens are the agents' own words, read from their binaries and live
// panes: Codex and OpenCode ask for approval in these words.
func TestPaneLine(t *testing.T) {
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
		line, asks := paneLine(c.screen)
		if line != c.line || asks != c.asks {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", c.name, line, asks, c.line, c.asks)
		}
	}
}

// TestAPaneShowsBusyIdleAndWhatItAsks: an agent without an adapter is busy
// while its pane changes, idle from the last change, and its last line and
// question come from the pane. Context use is not measured.
func TestAPaneShowsBusyIdleAndWhatItAsks(t *testing.T) {
	root := copyFixture(t)
	addProc(t, root, 800, 1, "codex", []string{"codex"}, "%10")
	addPane(t, root, "%10\tapi:1.1\tapi\t800\tcodex\t/work/api")
	screen := "• Working (3s • esc to interrupt)\n"
	var asked [][]string
	now := fixtureNow
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "codex"}
	smp.Now = func() time.Time { return now }
	smp.Capture = func(panes []string) map[string]string {
		asked = append(asked, panes)
		return map[string]string{"%10": screen}
	}

	if s := byPane(smp.Sample())["%10"]; s.Status != "" || s.Busy {
		t.Fatalf("first sight: nothing is known yet, got %+v", s)
	}
	now = now.Add(2 * time.Second)
	screen = "• Working (5s • esc to interrupt)\n"
	if s := byPane(smp.Sample())["%10"]; !s.Busy || s.Status != "busy" {
		t.Fatalf("the pane changed: busy, got %+v", s)
	}
	now = now.Add(2 * time.Second)
	screen = "  Would you like to run the following command?\n  $ make test\n"
	smp.Sample()
	now = now.Add(40 * time.Second)
	s := byPane(smp.Sample())["%10"]
	if s.Busy || s.Status != "idle" || s.Idle != 40*time.Second {
		t.Fatalf("unchanged for 40 s: idle 40s, got busy=%v status=%q idle=%v", s.Busy, s.Status, s.Idle)
	}
	if !s.Asks || s.LastLine != "Would you like to run the following command?" {
		t.Fatalf("the pane asks: got asks=%v line=%q", s.Asks, s.LastLine)
	}
	if s.Context != "unmeasured" {
		t.Fatalf("no adapter: context %q, want unmeasured", s.Context)
	}
	for _, panes := range asked {
		if strings.Join(panes, ",") != "%10" {
			t.Fatalf("only panes of agents without an adapter are read, got %q", panes)
		}
	}
}
