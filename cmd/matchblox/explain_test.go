package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

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
		"question            line 3  Do you want to make this edit?\n",
		"menu key hint       no match\n",
		"fallback            no match\n",
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
		"menu key hint       line 7  Esc to cancel · Tab to amend\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the output misses %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Fix:") {
		t.Errorf("a read menu has a fix line:\n%s", out)
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
			Rule string `json:"rule"`
			Line int    `json:"line"`
			Text string `json:"text"`
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
	if len(got.Hits) != 5 || got.Hits[1].Rule != said.RulePermits || got.Hits[1].Line != 4 || got.Hits[1].Text != "1. Yes" {
		t.Errorf("hits %+v", got.Hits)
	}
	if len(got.Options) != 3 || got.Options[2].N != 3 || got.Options[2].Label != "No" {
		t.Errorf("options %+v", got.Options)
	}
	if len(got.Screen) != 7 || got.Screen[2] != "Do you want to proceed?" {
		t.Errorf("screen %q", got.Screen)
	}
}
