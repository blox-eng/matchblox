package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/doors"
)

// askInstalled is the settings a matchblox before the merge guard wrote:
// every hook, PreToolUse for the question tool only.
const askInstalled = `{
  "hooks": {
    "Notification": [{"hooks": [{"type": "command", "command": "/opt/mb/matchblox hook Notification"}]}],
    "Stop": [{"hooks": [{"type": "command", "command": "/opt/mb/matchblox hook Stop"}]}],
    "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "/opt/mb/matchblox hook UserPromptSubmit"}]}],
    "SessionStart": [{"hooks": [{"type": "command", "command": "/opt/mb/matchblox hook SessionStart"}]}],
    "SessionEnd": [{"hooks": [{"type": "command", "command": "/opt/mb/matchblox hook SessionEnd"}]}],
    "PostToolUse": [{"hooks": [{"type": "command", "command": "/opt/mb/matchblox hook PostToolUse"}]}],
    "PreToolUse": [{"matcher": "AskUserQuestion", "hooks": [{"type": "command", "command": "/opt/mb/matchblox hook PreToolUse"}]}]
  }
}
`

// TestHooksDoorAddsBashMatcher: an install from before the merge guard
// opens the hooks door again, its diff adds the Bash entry and nothing
// else, and the question tool's entry stays.
func TestHooksDoorAddsBashMatcher(t *testing.T) {
	e := env(t)
	p := settings(t, e, askInstalled)
	d := door(t, e, doors.Hooks)
	if d.Done || d.Problem != "" {
		t.Fatalf("door = %+v, want one to open", d)
	}
	want := `+      {"matcher": "Bash", "hooks": [{"type": "command", "command": "` + exe + ` hook PreToolUse"}]}`
	if !strings.Contains(d.Preview, want) {
		t.Fatalf("the diff has no Bash hook:\n%s", d.Preview)
	}
	for line := range strings.SplitSeq(d.Preview, "\n") {
		// The question tool's line comes back with a comma after it.
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "++") && strings.Contains(line, " hook ") &&
			!strings.Contains(line, `"matcher": "Bash"`) && !strings.Contains(line, `"matcher": "AskUserQuestion"`) {
			t.Errorf("the diff adds another hook: %q", line)
		}
	}
	open(t, e, doors.Hooks)
	if got := commands(t, p)["PreToolUse"]; fmt.Sprint(got) != "["+exe+" hook PreToolUse "+exe+" hook PreToolUse]" {
		t.Fatalf("PreToolUse = %q", got)
	}
	if d := door(t, e, doors.Hooks); !d.Done {
		t.Fatalf("door not done after open: %+v", d)
	}
	// One entry whose matcher takes both tools is enough.
	e = env(t)
	settings(t, e, allowingHands(strings.Replace(askInstalled, `"matcher": "AskUserQuestion"`, `"matcher": "AskUserQuestion|Bash"`, 1)))
	if d := door(t, e, doors.Hooks); !d.Done {
		t.Fatalf("a matcher for both tools is not done: %+v", d)
	}
}

// allowingHands is a settings object that also allows HandRules, so a
// test of the hooks reads only the hooks.
func allowingHands(body string) string {
	b, _ := json.Marshal(HandRules)
	i := strings.LastIndex(body, "}")
	return body[:i] + `, "permissions": {"allow": ` + string(b) + "}}" + body[i+1:]
}

// allowRules reads permissions.allow of the settings.
func allowRules(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Permissions struct {
			Allow []string `json:"allow"`
			Deny  []string `json:"deny"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatalf("settings.json is not JSON: %v\n%s", err, b)
	}
	return s.Permissions.Allow
}

// TestHooksDoorAllowsTheStokersHands: F1, the hooks door also lets the
// stoker's hands run without a prompt; an install that has the hooks but
// not the rules opens the door again, and nothing is written twice.
func TestHooksDoorAllowsTheStokersHands(t *testing.T) {
	want := []string{"Bash(matchblox sessions:*)", "Bash(matchblox ships:*)", "Bash(matchblox usage:*)",
		"Bash(matchblox send:*)", "Bash(matchblox compact:*)", "Bash(matchblox hold:*)",
		"Bash(matchblox release:*)", "Bash(matchblox stoker:*)", "Bash(matchblox peek:*)"}
	if !slices.Equal(HandRules, want) {
		t.Fatalf("HandRules = %q", HandRules)
	}
	e := env(t)
	p := settings(t, e, strings.Replace(askInstalled, `"matcher": "AskUserQuestion"`, `"matcher": "AskUserQuestion|Bash"`, 1))
	d := door(t, e, doors.Hooks)
	if d.Done || d.Problem != "" {
		t.Fatalf("hooks without the rules: door = %+v, want one to open", d)
	}
	if !strings.Contains(d.Preview, `+    "allow": [`) || !strings.Contains(d.Preview, `+      "Bash(matchblox stoker:*)"`) {
		t.Fatalf("the diff has no rules:\n%s", d.Preview)
	}
	open(t, e, doors.Hooks)
	if got := allowRules(t, p); !slices.Equal(got, want) {
		t.Fatalf("allow = %q", got)
	}
	if d := door(t, e, doors.Hooks); !d.Done {
		t.Fatalf("door not done after open: %+v", d)
	}
	// Rules of the person's own stay, a rule already there is not added
	// twice, and the other permission keys keep their place.
	e = env(t)
	p = settings(t, e, `{
  "permissions": {
    "allow": ["Bash(go test:*)", "Bash(matchblox send:*)"],
    "deny": ["Bash(rm -rf:*)"]
  },
  "theme": "dark"
}
`)
	open(t, e, doors.Hooks)
	got := allowRules(t, p)
	if got[0] != "Bash(go test:*)" || len(got) != len(want)+1 {
		t.Fatalf("allow = %q", got)
	}
	for _, r := range want {
		if n := strings.Count(strings.Join(got, " "), r); n != 1 {
			t.Errorf("%s is there %d times", r, n)
		}
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), `"deny": ["Bash(rm -rf:*)"]`) || !strings.HasPrefix(string(b), "{\n  \"permissions\": {\n    \"allow\": [\n      \"Bash(go test:*)\",") {
		t.Fatalf("the other keys moved:\n%s", b)
	}
}
