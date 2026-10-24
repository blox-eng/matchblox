package sample

import "testing"

func TestEngineRules(t *testing.T) {
	for _, c := range []struct {
		name          string
		argv          []string
		batch, server bool
	}{
		{"claude", []string{"claude", "-p", "hi"}, true, false},
		{"claude", []string{"node", "/x/claude.js", "--print", "hi"}, true, false},
		{"claude", []string{"claude", "--resume", "x"}, false, false},
		{"claude", nil, false, false},
		{"codex", []string{"codex", "exec", "fix"}, true, false},
		{"codex", []string{"codex", "-m", "gpt-5", "exec", "fix"}, true, false},
		{"codex", []string{"codex", "mcp-server"}, true, true},
		{"codex", []string{"codex", "-c", "x=1", "app-server", "--managed-daemon"}, true, true},
		{"codex", []string{"codex", "app-server", "--managed-daemon"}, true, true},
		{"codex", []string{"node", "/x/codex.js", "app-server"}, true, true},
		{"codex", []string{"codex"}, false, false},
		{"opencode", []string{"opencode", "run", "hi"}, true, false},
		{"opencode", []string{"node", "/x/opencode.js", "run", "hi"}, true, false},
		{"opencode", []string{"opencode", "--print-logs", "run"}, true, false},
		{"opencode", []string{"opencode", "serve"}, false, false},
		{"opencode", []string{"opencode"}, false, false},
		{"aider", []string{"aider"}, false, false},
	} {
		e := engineOf(c.name)
		if got := e.Batch != nil && e.Batch(c.argv); got != c.batch {
			t.Errorf("%s %v: batch %v", c.name, c.argv, got)
		}
		if got := e.Server != nil && e.Server(c.argv); got != c.server {
			t.Errorf("%s %v: server %v", c.name, c.argv, got)
		}
	}
	if engineOf("claude").ParentEnv != "CLAUDE_PID" || engineOf("codex").ParentEnv != "" {
		t.Fatal("parent env")
	}
	if e := engineOf("aider"); e.Name != "aider" || e.Batch != nil || e.Server != nil {
		t.Fatal("zero entry")
	}
}
