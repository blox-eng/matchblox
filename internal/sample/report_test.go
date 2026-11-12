package sample

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

// opencodeAt runs OpenCode 930 in pane %23 of the fixture machine, in
// wick-mobile, and writes the report its plugin keeps ("" for none).
func opencodeAt(t *testing.T, root, report string) *Sampler {
	t.Helper()
	addProc(t, root, 930, 1, "opencode", []string{"opencode"}, "%23")
	symlink(t, "/work/wick-mobile", filepath.Join(root, "proc", "930", "cwd"))
	addPane(t, root, "%23\thearth:6.1\twick-mobile\t930\topencode\t/work/wick-mobile")
	writeFile(t, filepath.Join(root, "panes", "23.txt"), "  Should I also update the docs?\n")
	if report != "" {
		writeFile(t, filepath.Join(root, "state", "opencode", "930.json"), report)
	}
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "opencode"}
	smp.OwnEntries = true // the fixture's clock is fixed, as a live one is real
	return smp
}

// TestTheOpencodeReportIsRead: the plugin's report is the agent's own
// word, read over its pane: a wait and its reason, a retry with its attempt
// and next try. The pane still gives what it said last.
func TestTheOpencodeReportIsRead(t *testing.T) {
	s := byPane(opencodeAt(t, copyFixture(t),
		`{"pid":930,"startedAt":1790000000000,"status":"waiting","waitingFor":"question","statusUpdatedAt":1790003300000}`).Sample())["%23"]
	if !s.Reported || s.Status != "waiting" || s.WaitingFor != "question" || s.Busy || s.Asks || s.FromPane ||
		s.Idle != 310*time.Second || s.LastLine != "Should I also update the docs?" {
		t.Fatalf("waiting: %+v", s)
	}
	s = byPane(opencodeAt(t, copyFixture(t),
		`{"pid":930,"startedAt":1790000000000,"status":"retry","attempt":2,"next":1790003640000,"statusUpdatedAt":1790003600000}`).Sample())["%23"]
	if !s.Reported || s.Status != "retry" || !s.Busy || s.Retry != 2 || !s.RetryAt.Equal(time.UnixMilli(1790003640000)) {
		t.Fatalf("retry: %+v", s)
	}
}

// TestADeadProcessReportIsIgnored: a report is the agent's only while its
// process lives and started before it. A report of a pid that is gone
// speaks for no session, and one an earlier process with the same pid
// left behind is no report: the pane decides.
func TestADeadProcessReportIsIgnored(t *testing.T) {
	root := copyFixture(t)
	writeFile(t, filepath.Join(root, "state", "opencode", "931.json"),
		`{"pid":931,"startedAt":1790000000000,"status":"waiting","waitingFor":"permission","statusUpdatedAt":1790003300000}`)
	for _, s := range opencodeAt(t, root, "").Sample().Sessions {
		if s.Reported && s.Agent == "opencode" {
			t.Fatalf("a gone pid's report was used: %+v", s)
		}
	}
	old := `{"pid":930,"startedAt":1789000000000,"status":"waiting","waitingFor":"permission","statusUpdatedAt":1789000100000}`
	if s := byPane(opencodeAt(t, copyFixture(t), old).Sample())["%23"]; s.Reported || s.Status == "waiting" || s.WaitingFor != "" {
		t.Fatalf("an earlier process's report was used: %+v", s)
	}
	root = copyFixture(t)
	writeFile(t, filepath.Join(root, "home", ".claude", "sessions", "200.json"),
		`{"pid":200,"sessionId":"old","cwd":"/work/app","name":"gone","status":"waiting","waitingFor":"permission prompt","startedAt":1789000000000}`)
	smp := newFixtureSampler(root)
	smp.OwnEntries = true
	if s := byPane(smp.Sample())["%1"]; s.Reported || s.WaitingFor != "" {
		t.Fatalf("Claude Code: an earlier process's report was used: %+v", s)
	}
}

// TestTheReportsOfGoneProcessesAreForgotten: the reader keeps a report
// only while its process lives.
func TestTheReportsOfGoneProcessesAreForgotten(t *testing.T) {
	root := copyFixture(t)
	smp := opencodeAt(t, root, `{"pid":930,"startedAt":1790000000000,"status":"busy","statusUpdatedAt":1790003600000}`)
	smp.Sample()
	if len(smp.opencode.reports) != 1 {
		t.Fatalf("reports %v, want 930's", smp.opencode.reports)
	}
	if err := os.RemoveAll(filepath.Join(root, "proc", "930")); err != nil {
		t.Fatal(err)
	}
	smp.Sample()
	if len(smp.opencode.reports) != 0 {
		t.Fatalf("reports %v after 930 is gone", smp.opencode.reports)
	}
}
