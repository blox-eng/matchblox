package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/proto"
)

func TestKillAndRenameNameThePane(t *testing.T) {
	a, err := lifeAct(cliDoc(), "kill", []string{"api"}, false, "%9")
	if err != nil || a.RecID != "kill:%3" || a.Text != "" || a.Confirm != "y" || a.From != "%9" {
		t.Fatalf("%+v %v", a, err)
	}
	if a, err = lifeAct(cliDoc(), "kill", []string{"%4"}, true, ""); err != nil || a.Text != "anyway" {
		t.Fatalf("%+v %v", a, err)
	}
	if a, err = lifeAct(cliDoc(), "rename", []string{"api", "ships-flake"}, false, ""); err != nil || a.RecID != "rename:%3" || a.Text != "ships-flake" {
		t.Fatalf("%+v %v", a, err)
	}
}

func TestKillAndRenameRefusals(t *testing.T) {
	for name, c := range map[string]struct {
		cmd   string
		words []string
		want  string
	}{
		"kill nobody":   {"kill", nil, "kill needs a session: matchblox kill <who>"},
		"kill two":      {"kill", []string{"api", "web"}, "kill takes one session"},
		"rename bare":   {"rename", []string{"api"}, "rename needs a name: matchblox rename <who> <name>"},
		"rename spaces": {"rename", []string{"api", "a", "b"}, "a name is one word"},
		"rename bad":    {"rename", []string{"api", "a.b"}, "a name is letters, digits"},
		"unknown":       {"kill", []string{"nope"}, "no session named nope"},
	} {
		if _, err := lifeAct(cliDoc(), c.cmd, c.words, false, ""); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}

func TestNewTakesWhereAgentAndFirstMessage(t *testing.T) {
	a, err := newSessionAct([]string{"work", "on", "#88"}, newOpts{where: "/src/api", agent: "codex", branch: "feat/88", name: "n88"}, "%9", "/home/u")
	if err != nil || a.RecID != "new:session" || a.From != "%9" {
		t.Fatalf("%+v %v", a, err)
	}
	var n proto.NewSession
	if err := json.Unmarshal([]byte(a.Text), &n); err != nil {
		t.Fatal(err)
	}
	if n != (proto.NewSession{Where: "/src/api", Agent: "codex", Branch: "feat/88", Name: "n88", First: "work on #88"}) {
		t.Fatalf("%+v", n)
	}
	// Bare, it starts Claude Code here.
	if a, err = newSessionAct(nil, newOpts{}, "", "/home/u/src"); err != nil {
		t.Fatal(err)
	}
	var bare proto.NewSession
	if err := json.Unmarshal([]byte(a.Text), &bare); err != nil || bare != (proto.NewSession{Where: "/home/u/src", Agent: "claude"}) {
		t.Fatalf("%+v %v", bare, err)
	}
	// A relative --where is from here.
	if a, err = newSessionAct(nil, newOpts{where: "api"}, "", "/home/u/src"); err != nil || !strings.Contains(a.Text, `"where":"/home/u/src/api"`) {
		t.Fatalf("%s %v", a.Text, err)
	}
}
