package sample

import (
	"path/filepath"
	"testing"
)

// claudeReport writes the session file of the fixture's Claude Code 200 in
// pane %1 with a status and a reason.
func claudeReport(t *testing.T, root, status, waitingFor string) {
	t.Helper()
	why := ""
	if waitingFor != "" {
		why = `,"waitingFor":"` + waitingFor + `"`
	}
	writeFile(t, filepath.Join(root, "home", ".claude", "sessions", "200.json"),
		`{"pid":200,"sessionId":"11111111-aaaa-4000-8000-000000000001","cwd":"/work/app","name":"hearth-api","status":"`+status+`"`+why+
			`,"startedAt":1790000000000,"statusUpdatedAt":1790003000000,"tmux":"work:@1.%1"}`)
}

// TestClaudeCodesReportIsRead: the session file's status and reason are
// the agent's own word: a wait keeps its reason, and "shell" works.
func TestClaudeCodesReportIsRead(t *testing.T) {
	root := copyFixture(t)
	claudeReport(t, root, "waiting", "permission prompt")
	s := byPane(newFixtureSampler(root).Sample())["%1"]
	if !s.Reported || s.Status != "waiting" || s.WaitingFor != "permission prompt" || s.Busy || s.Works() {
		t.Fatalf("waiting: %+v", s)
	}

	root = copyFixture(t)
	claudeReport(t, root, "shell", "")
	s = byPane(newFixtureSampler(root).Sample())["%1"]
	if !s.Reported || s.Status != "shell" || s.WaitingFor != "" || !s.Works() || s.Resting() {
		t.Fatalf("shell: %+v", s)
	}
}
