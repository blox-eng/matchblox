package menu

import (
	"maps"
	"os"
	"strings"
	"testing"
)

const permissionPrompt = `● Bash(go test ./...)
  ⎿  Running…

╭──────────────────────────────────────────────────────────╮
│ Bash command                                             │
│                                                          │
│   go test ./...                                          │
│                                                          │
│ Do you want to proceed?                                  │
│ ❯ 1. Yes                                                 │
│   2. Yes, and don't ask again for go test commands       │
│   3. No, and tell Claude what to do differently (esc)    │
╰──────────────────────────────────────────────────────────╯
`

var promptMenu = map[int]string{
	1: "Yes",
	2: "Yes, and don't ask again for go test commands",
	3: "No, and tell Claude what to do differently (esc)",
}

// TestMenuReadsAPermissionPrompt: Claude Code's permission prompt, in its
// box, is a menu of three options.
func TestMenuReadsAPermissionPrompt(t *testing.T) {
	if got := Read(permissionPrompt); !maps.Equal(got, promptMenu) {
		t.Fatalf("got %v, want %v", got, promptMenu)
	}
}

// TestMenuReadsAPlainMenu: a menu with no box, in place of the input.
func TestMenuReadsAPlainMenu(t *testing.T) {
	screen := "Which way do we fix the login?\n\n› 1. Fast\n  2. Safe\n\n"
	if got := Read(screen); !maps.Equal(got, map[int]string{1: "Fast", 2: "Safe"}) {
		t.Fatalf("got %v", got)
	}
}

// TestMenuSkipsANumberedReplyAboveIt: the agent's own numbered list above
// the menu is not the menu.
func TestMenuSkipsANumberedReplyAboveIt(t *testing.T) {
	screen := "I can do two things:\n1. Add tests\n2. Ship\n\nWhich way?\n❯ 1. Fast\n  2. Safe\n"
	if got := Read(screen); !maps.Equal(got, map[int]string{1: "Fast", 2: "Safe"}) {
		t.Fatalf("got %v", got)
	}
}

// TestMenuNoneWhenTextFollows: a numbered list in a reply, with prose and
// the input under it, is no menu.
func TestMenuNoneWhenTextFollows(t *testing.T) {
	screen := "Next steps:\n1. Add tests\n2. Ship\n\nTell me which one.\n\n╭────╮\n│ >  │\n╰────╯\n"
	if got := Read(screen); got != nil {
		t.Fatalf("got %v, want no menu", got)
	}
}

// TestMenuReadsStyledText: the pane's colours do not change the menu.
func TestMenuReadsStyledText(t *testing.T) {
	styled := "Do you want to proceed?\n\x1b[36m❯ 1. \x1b[1mYes\x1b[0m\n  2. Yes, and don't ask again for go test commands\n  \x1b[2m3. No, and tell Claude what to do differently (esc)\x1b[0m\n"
	if got := Read(styled); !maps.Equal(got, promptMenu) {
		t.Fatalf("got %v, want %v", got, promptMenu)
	}
}

// TestMenuKeepsAWrappedOption: an option's second line, indented with no
// number, does not end the menu.
func TestMenuKeepsAWrappedOption(t *testing.T) {
	screen := "❯ 1. Yes\n  2. Yes, and don't ask again for\n     go test commands\n  3. No\n"
	if got := Read(screen); len(got) != 3 || got[3] != "No" {
		t.Fatalf("got %v", got)
	}
}

// TestMenuNoneAboveTheInput: a reply that ends on a numbered list, with
// the agent's empty input under it, is no menu: a digit there is text.
func TestMenuNoneAboveTheInput(t *testing.T) {
	screen := "● Two ways:\n  1. Fast\n  2. Safe\n\n────\n❯ \n────\n"
	if got := Read(screen); got != nil {
		t.Fatalf("got %v, want no menu", got)
	}
}

// TestMenuReadsRealCaptures: Claude Code's question tool, captured: a rule
// inside the menu, descriptions under options, the keys hint under it.
// An answered question shows no menu.
func TestMenuReadsRealCaptures(t *testing.T) {
	for file, want := range map[string]map[int]string{
		"colour.txt": {1: "Red A bold red front door.", 2: "Blue A calm blue front door.", 3: "Type something.", 4: "Chat about this"},
		"toppings.txt": {1: "[ ] Cheese Classic melted mozzarella.", 2: "[ ] Olives Sliced black or green olives.",
			3: "[ ] Basil Fresh basil leaves added after baking.", 4: "[ ] Garlic Thinly sliced or roasted garlic.",
			5: "[ ] Type something Submit", 6: "Chat about this"},
		"days-shift-review.txt": {1: "Submit answers", 2: "Cancel"},
		"answered.txt":          nil,
	} {
		b, err := os.ReadFile("../../testdata/menus/" + file)
		if err != nil {
			t.Fatal(err)
		}
		if got := Read(string(b)); !maps.Equal(got, want) || (want == nil) != (got == nil) {
			t.Errorf("%s: got %v, want %v", file, got, want)
		}
	}
}

// TestSignTellsTwoPromptsApart: the same options for two commands sign
// apart; the same prompt drawn again, in other colours, signs the same.
func TestSignTellsTwoPromptsApart(t *testing.T) {
	other := strings.Replace(permissionPrompt, "go test ./...  ", "rm -rf build/  ", 1)
	if Sign(permissionPrompt) == "" || Sign(permissionPrompt) == Sign(other) {
		t.Fatalf("signs %q and %q", Sign(permissionPrompt), Sign(other))
	}
	styled := strings.Replace(permissionPrompt, "❯ 1. Yes", "\x1b[36m❯ 1. Yes\x1b[0m", 1)
	if Sign(styled) != Sign(permissionPrompt) {
		t.Fatal("colours change the signature")
	}
	if Sign("● Done.\n\n❯ \n") != "" {
		t.Fatal("a screen with no menu signs")
	}
}

// TestChosen: the option the agent marks, read with the menu; 0 when no
// menu shows, and a mark inside an option's text is not the mark.
func TestChosen(t *testing.T) {
	for file, want := range map[string]int{
		"marked-boxed.txt":   2,
		"marked-four.txt":    1,
		"marked-plain.txt":   3,
		"marked-wrapped.txt": 2,
		"marked-reply.txt":   0,
		"marked-in-text.txt": 2,
	} {
		b, err := os.ReadFile("../../testdata/menus/" + file)
		if err != nil {
			t.Fatal(err)
		}
		if got := Chosen(string(b)); got != want {
			t.Errorf("%s: got %d, want %d", file, got, want)
		}
	}
}
