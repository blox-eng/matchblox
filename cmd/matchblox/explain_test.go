package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/menu"
	"github.com/blox-eng/matchblox/internal/queue"
	"github.com/blox-eng/matchblox/internal/said"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

var explainNow = time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)

// explainDoc is one Claude Code session, api, whose hook has waited 14s
// for a permission no rule reads.
func explainDoc() state.Doc {
	var d state.Doc
	d.At = explainNow
	d.Sessions = []sample.Session{{Pane: "%3", Target: "app:1.1", Name: "api", SessionID: "s3", Agent: "claude", Version: "2.1.331", Cwd: "/home/tester/dev/api", UnknownScreen: true}}
	d.Queue = []queue.Item{{Pane: "%3", SessionID: "s3", Name: "api", State: queue.StatePermission, Since: explainNow.Add(-14 * time.Second)}}
	return d
}

func explainEnvOf(screen string) explainEnv {
	return explainEnv{
		screenOf: func(string) (string, bool, bool) { return screen, false, true },
		now:      explainNow,
		home:     "/home/tester",
		host:     "buildbox",
		user:     "tester",
	}
}

const unknownScreen = ` tester@buildbox wrote /home/tester/dev/api/notes.md
 Edit /home/tester/dev/api/main.go as tester on buildbox?
 Do you want to make this edit?
 ❯ 1. Accept
   2. Accept, and don't ask again this session
   3. Decline
 Esc to cancel · Tab to amend
`

// TestExplainLeavesNoNameOrPath: the output is made to be posted in a
// public issue, so it carries no home path, host or user.
func TestExplainLeavesNoNameOrPath(t *testing.T) {
	for _, asJSON := range []bool{false, true} {
		var buf bytes.Buffer
		if err := writeExplain(&buf, explainDoc(), "api", explainEnvOf(unknownScreen), asJSON); err != nil {
			t.Fatal(err)
		}
		for _, leak := range []string{"/home/tester", "buildbox", "tester"} {
			if strings.Contains(buf.String(), leak) {
				t.Errorf("json %v: the output holds %q:\n%s", asJSON, leak, buf.String())
			}
		}
	}
}

// TestExplainLeavesNoShortHost: a prompt or a statusline prints the host
// name only up to its first dot, as on a Mac's buildbox.local.
func TestExplainLeavesNoShortHost(t *testing.T) {
	x := explainEnvOf(" tester@buildbox ~/dev/api main\n" + readMenu)
	x.host = "buildbox.local"
	var buf bytes.Buffer
	if err := writeExplain(&buf, explainDoc(), "api", x, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "buildbox") {
		t.Errorf("the output holds the short host:\n%s", buf.String())
	}
}

func TestExplainUnknownScreen(t *testing.T) {
	var buf bytes.Buffer
	if err := writeExplain(&buf, explainDoc(), "api", explainEnvOf(unknownScreen), false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"Session  api · Claude Code 2.1.331\n",
		"Hook     waits for a permission, for 14s\n",
		"Verdict  unknown screen: no rule reads a menu\n",
		"Rules    built into matchblox " + version + "\n",
		"Rules on the last 8 lines with words\n",
		"working line        no match\n",
		"approval prompt     no match\n",
		"question            line 3  Do you want to make this edit?  (decides)\n",
		"menu key hint       line 7  Esc to cancel · Tab to amend\n",
		"fallback            not tried\n",
		"Menu     not read\n",
		"Screen (home path, host and user removed; check it before you post it)\n",
		"  │ 1. Accept\n",
		"\nFix: report it at github.com/blox-eng/matchblox/issues with this output\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the output misses %q:\n%s", want, out)
		}
	}
}

// TestExplainUnknownClaudeMenuShowsTheKeyHint: a rule's text on screen
// shows as its match even when an earlier rule decided, and only the rule
// that decided says so.
func TestExplainUnknownClaudeMenuShowsTheKeyHint(t *testing.T) {
	var buf bytes.Buffer
	if err := writeExplain(&buf, explainDoc(), "api", explainEnvOf(unknownScreen), false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if want := "menu key hint       line 7  Esc to cancel · Tab to amend\n"; !strings.Contains(out, want) {
		t.Errorf("the output misses %q:\n%s", want, out)
	}
	if n := strings.Count(out, "(decides)"); n != 1 {
		t.Errorf("%d rules say they decide:\n%s", n, out)
	}
}

const readMenu = ` Bash command
   go test ./...
 Do you want to proceed?
 ❯ 1. Yes
   2. Yes, and don't ask again for go test commands
   3. No
 Esc to cancel · Tab to amend
`

func TestExplainReadMenuHasNoFix(t *testing.T) {
	d := explainDoc()
	d.Sessions[0].UnknownScreen, d.Sessions[0].Permits, d.Sessions[0].LastLine = false, true, "Do you want to proceed?"
	var buf bytes.Buffer
	if err := writeExplain(&buf, d, "api", explainEnvOf(readMenu), false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"Verdict  waits for an approval: Do you want to proceed?\n",
		"Menu     1 Yes · 2 Yes, and don't ask again for go test commands · 3 No\n",
		"menu key hint       line 7  Esc to cancel · Tab to amend  (decides)\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the output misses %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Fix:") {
		t.Errorf("a read menu has a fix line:\n%s", out)
	}
}

// TestExplainMenuIsWhatADigitPicks: the Menu line shows the rows a digit
// picks, read as the console reads them, not a numbered list above.
func TestExplainMenuIsWhatADigitPicks(t *testing.T) {
	const screen = ` Bash command
   git commit -m "fix: two things
   1. update the docs
   2. bump the version"
 Do you want to proceed?
 ❯ 1. Yes
   2. No
 Esc to cancel
`
	if got := menu.Read(screen); len(got) != 2 || got[1] != "Yes" || got[2] != "No" {
		t.Fatalf("menu.Read = %v, want 1 Yes and 2 No", got)
	}
	d := explainDoc()
	d.Sessions[0].UnknownScreen, d.Sessions[0].Permits, d.Sessions[0].LastLine = false, true, "Do you want to proceed?"
	var buf bytes.Buffer
	if err := writeExplain(&buf, d, "api", explainEnvOf(screen), false); err != nil {
		t.Fatal(err)
	}
	if want := "Menu     1 Yes · 2 No\n"; !strings.Contains(buf.String(), want) {
		t.Errorf("the output misses %q:\n%s", want, buf.String())
	}
}

func TestExplainHooklessAgent(t *testing.T) {
	d := explainDoc()
	d.Sessions[0].Agent, d.Sessions[0].UnknownScreen, d.Sessions[0].LastLine = "codex", false, "Done, the tests pass."
	d.Queue = nil
	var buf bytes.Buffer
	if err := writeExplain(&buf, d, "api", explainEnvOf("  Done, the tests pass.\n"), false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Session  api · Codex 2.1.331\n", "Hook     no hooks: screen only\n", "Verdict  none: Done, the tests pass.\n"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("the output misses %q:\n%s", want, buf.String())
		}
	}
}

func TestExplainUnknownSession(t *testing.T) {
	d := explainDoc()
	d.Sessions[0].Name = "web"
	var buf bytes.Buffer
	err := writeExplain(&buf, d, "api", explainEnvOf(unknownScreen), false)
	if err == nil || err.Error() != "no session api: matchblox sessions lists them" {
		t.Fatalf("an unknown session: %v", err)
	}
	if buf.Len() > 0 {
		t.Errorf("an unknown session printed %q", buf.String())
	}
}

func TestExplainJSON(t *testing.T) {
	d := explainDoc()
	d.Sessions[0].UnknownScreen, d.Sessions[0].Permits, d.Sessions[0].LastLine = false, true, "Do you want to proceed?"
	var buf bytes.Buffer
	if err := writeExplain(&buf, d, "api", explainEnvOf(readMenu), true); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Verdict string `json:"verdict"`
		Hook    string `json:"hook"`
		Hits    []struct {
			Rule    string `json:"rule"`
			Line    int    `json:"line"`
			Text    string `json:"text"`
			Decides bool   `json:"decides"`
		} `json:"hits"`
		Options []struct {
			N     int    `json:"n"`
			Label string `json:"label"`
		} `json:"options"`
		Screen []string `json:"screen"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err, buf.String())
	}
	if got.Verdict != "waits for an approval: Do you want to proceed?" || got.Hook != "waits for a permission, for 14s" {
		t.Errorf("verdict %q, hook %q", got.Verdict, got.Hook)
	}
	if len(got.Hits) != 5 || got.Hits[1].Rule != said.RulePermits || got.Hits[1].Line != 4 || got.Hits[1].Text != "1. Yes" || !got.Hits[1].Decides {
		t.Errorf("hits %+v", got.Hits)
	}
	if q := got.Hits[2]; q.Rule != said.RuleQuestion || q.Line != 3 || q.Decides {
		t.Errorf("the question matches line 3 and does not decide: %+v", q)
	}
	if len(got.Options) != 3 || got.Options[2].N != 3 || got.Options[2].Label != "No" {
		t.Errorf("options %+v", got.Options)
	}
	if len(got.Screen) != 7 || got.Screen[2] != "Do you want to proceed?" {
		t.Errorf("screen %q", got.Screen)
	}
}

// TestExplainFileUnreadable: the error names the path, with the home path
// scrubbed, since it may be pasted into the report too.
func TestExplainFileUnreadable(t *testing.T) {
	x := explainEnvOf("")
	x.home = t.TempDir()
	var buf bytes.Buffer
	err := explainFile(&buf, x.home+"/screens/api.txt", x, false)
	if err == nil || !strings.Contains(err.Error(), "<dir>/screens/api.txt") || strings.Contains(err.Error(), x.home) {
		t.Fatalf("a missing file: %v", err)
	}
	if buf.Len() > 0 {
		t.Errorf("a missing file printed %q", buf.String())
	}
}

// TestExplainSaysTheScreenChanged: the note comes from the service, the
// rules run on a new capture; when the two disagree, a line says so.
func TestExplainSaysTheScreenChanged(t *testing.T) {
	permits := explainDoc()
	permits.Sessions[0].UnknownScreen, permits.Sessions[0].Permits, permits.Sessions[0].LastLine = false, true, "Do you want to proceed?"
	unread := explainDoc()
	unread.Sessions[0].UnknownScreen, unread.Sessions[0].MenuUnread = false, true
	none := explainDoc()
	none.Sessions[0].UnknownScreen, none.Sessions[0].LastLine, none.Queue = false, "All tests pass.", nil
	asks := explainDoc()
	asks.Sessions[0].Agent, asks.Sessions[0].UnknownScreen, asks.Queue = "codex", false, nil
	asks.Sessions[0].Asks, asks.Sessions[0].LastLine = true, "Should I also update the docs?"
	edit := explainDoc()
	edit.Sessions[0].UnknownScreen, edit.Sessions[0].Permits, edit.Sessions[0].LastLine = false, true, "Do you want to make this edit?"
	for _, c := range []struct {
		name    string
		doc     state.Doc
		screen  string
		changed bool
	}{
		{"an unknown note, a read menu", explainDoc(), readMenu, true},
		{"an unread note, a read menu", unread, readMenu, true},
		{"an approval note, a screen no rule reads", permits, "  Done, the tests pass.\n", true},
		{"an approval note, a read menu", permits, readMenu, false},
		{"an unknown note, an unknown screen", explainDoc(), unknownScreen, false},
		{"no note, a read menu", none, readMenu, true},
		{"an approval note, another approval on screen", edit, readMenu, true},
		{"no note, the same line", none, "  All tests pass.\n", false},
		{"a question read from the pane, the same question", asks, "  Should I also update the docs?\n", false},
	} {
		var buf bytes.Buffer
		if err := writeExplain(&buf, c.doc, "api", explainEnvOf(c.screen), false); err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(buf.String(), explainChanged); got != c.changed {
			t.Errorf("%s: the changed line shows %v, want %v:\n%s", c.name, got, c.changed, buf.String())
		}
	}
}

// TestExplainFile: a saved screen has no session and no hook, so neither
// line shows; the rules and the menu do.
func TestExplainFile(t *testing.T) {
	var buf bytes.Buffer
	if err := explainFile(&buf, "../../testdata/menus/claude-bash-2.1.330.txt", explainEnvOf(""), false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	want := "Menu     1 Yes · 2 Yes, and always allow access to <work> from this project · 3 Yes, and switch to auto mode · auto mode handles these prompts for you · 4 No\n"
	if !strings.Contains(out, want) {
		t.Errorf("the output misses %q:\n%s", want, out)
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "Session") || strings.HasPrefix(l, "Hook") {
			t.Errorf("a saved screen shows %q:\n%s", l, out)
		}
	}
	if strings.Contains(out, explainChanged) {
		t.Errorf("a saved screen has no note to disagree with:\n%s", out)
	}
}

func TestExplainNeedsOneOf(t *testing.T) {
	for _, c := range []struct {
		words []string
		file  string
		want  string
	}{
		{[]string{"api"}, "screen.txt", "explain takes a session or --file, not both"},
		{nil, "", "explain needs a session: matchblox agents explain <who>"},
	} {
		var buf bytes.Buffer
		err := cliExplain(c.words, &buf, "", c.file, false)
		if err == nil || err.Error() != c.want {
			t.Errorf("words %q, file %q: %v, want %q", c.words, c.file, err, c.want)
		}
	}
}
