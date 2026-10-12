package sample

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// claudeTurnEnded is a turn as Claude Code 2.1.296 ends it while a
// subagent it started still works: the reply, the stop hooks, then the
// turn's duration with the background work left.
const claudeTurnEnded = `{"type":"assistant","isSidechain":false,"timestamp":"2026-10-12T05:15:10Z","message":{"model":"claude-opus-5-5","content":[{"type":"text","text":"Task 9 is done and passed review."}],"usage":{"input_tokens":10,"cache_read_input_tokens":692000,"cache_creation_input_tokens":0,"output_tokens":90}}}
{"type":"attachment","isSidechain":false,"timestamp":"2026-10-12T05:15:16.100Z"}
{"type":"system","subtype":"stop_hook_summary","isSidechain":false,"timestamp":"2026-10-12T05:15:16.109Z"}
{"type":"system","subtype":"turn_duration","isSidechain":false,"durationMs":47372,"pendingBackgroundAgentCount":1,"timestamp":"2026-10-12T05:15:16.116Z"}
`

func TestATurnEndTellsWhenAndWhatWorkIsLeft(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	writeFile(t, path, claudeTurnEnded)
	u, ok := lastUsage(path, int64(len(claudeTurnEnded)))
	if !ok || u.Tokens != 692100 {
		t.Fatalf("usage %+v ok=%v", u, ok)
	}
	if want := time.Date(2026, 10, 12, 5, 15, 16, 116e6, time.UTC); !u.Ended.Equal(want) {
		t.Fatalf("ended %v, want %v", u.Ended, want)
	}
	if u.Background != 1 {
		t.Fatalf("background %d, want 1", u.Background)
	}
}

// A line of the next turn after the end: the session works again. A
// subagent's lines (sidechain) and local bookkeeping do not start a turn.
func TestWhatStartsTheNextTurn(t *testing.T) {
	for _, c := range []struct {
		name, line string
		ended      bool
	}{
		{"a prompt", `{"type":"user","isSidechain":false,"timestamp":"2026-10-12T05:20:00Z","message":{"role":"user","content":"go on"}}`, false},
		{"a reply", `{"type":"assistant","isSidechain":false,"timestamp":"2026-10-12T05:20:00Z","message":{"model":"claude-opus-5-5","content":[{"type":"text","text":"On it."}],"usage":{"input_tokens":1,"output_tokens":1}}}`, false},
		{"a subagent", `{"type":"assistant","isSidechain":true,"timestamp":"2026-10-12T05:20:00Z","message":{"model":"claude-opus-5-5","content":[{"type":"text","text":"Reading."}],"usage":{"input_tokens":1,"output_tokens":1}}}`, true},
		{"an attachment", `{"type":"attachment","isSidechain":false,"timestamp":"2026-10-12T05:20:00Z"}`, true},
		{"a title", `{"type":"custom-title","customTitle":"Billed","sessionId":"x"}`, true},
	} {
		body := claudeTurnEnded + c.line + "\n"
		path := filepath.Join(t.TempDir(), "s.jsonl")
		writeFile(t, path, body)
		u, _ := lastUsage(path, int64(len(body)))
		if got := !u.Ended.IsZero(); got != c.ended {
			t.Errorf("%s: ended %v, want %v", c.name, got, c.ended)
		}
	}
}

// A turn still running has no end, and a transcript that never ended a
// turn (an older Claude Code) has none either.
func TestARunningTurnHasNoEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	writeFile(t, path, claudeOut)
	if u, _ := lastUsage(path, int64(len(claudeOut))); !u.Ended.IsZero() || u.Background != 0 {
		t.Fatalf("ended %v background %d", u.Ended, u.Background)
	}
}

// A session whose registry says busy because its background work runs,
// while its transcript ended the turn, is idle: no turn runs. It shows the
// work it left: the subagents Claude Code counted and its background shells.
func TestAnEndedTurnIsIdleWhileItsBackgroundWorks(t *testing.T) {
	root := copyFixture(t)
	transcript := filepath.Join(root, "home", ".claude", "projects", "-work-app", "11111111-aaaa-4000-8000-000000000001.jsonl")
	b, err := os.ReadFile(transcript)
	if err != nil {
		t.Fatal(err)
	}
	ended := fixtureNow.Add(-20 * time.Minute).UTC()
	line := `{"type":"system","subtype":"turn_duration","isSidechain":false,"pendingBackgroundAgentCount":1,"timestamp":"` + ended.Format(time.RFC3339Nano) + `"}` + "\n"
	writeFile(t, transcript, string(b)+line)
	// A background shell of its Bash tool: started 2 hours before the
	// fixture's clock (uptime 12345.6 s, 100 ticks a second).
	shell := filepath.Join(root, "proc", "220")
	writeFile(t, filepath.Join(shell, "stat"), "220 (zsh) S 200 220 220 0 -1 0 0 0 0 0 1 0 0 0 20 0 1 0 514560 100000 2500")
	writeFile(t, filepath.Join(shell, "cmdline"), "/usr/bin/zsh\x00-c\x00source /home/user/.claude/shell-snapshots/snapshot-zsh-1-x.sh 2>/dev/null || true && eval 'until ! pgrep -f ratchet >/dev/null; do sleep 5; done' < /dev/null && pwd -P >| /tmp/claude-1-cwd\x00")

	s := byPane(newFixtureSampler(root).Sample())["%1"]
	if s.Busy || s.Status != "idle" || s.Idle != 20*time.Minute {
		t.Fatalf("busy %v status %q idle %v", s.Busy, s.Status, s.Idle)
	}
	if s.Agents != 1 || len(s.Shells) != 1 {
		t.Fatalf("agents %d shells %+v", s.Agents, s.Shells)
	}
	if sh := s.Shells[0]; sh.PID != 220 || sh.Command != "until ! pgrep -f ratchet >/dev/null; do sleep 5; done" || fixtureNow.Sub(sh.Started).Round(time.Minute) != 2*time.Hour {
		t.Fatalf("shell %+v (age %v)", sh, fixtureNow.Sub(sh.Started))
	}
}

// The command shown is the one inside the Bash tool's wrapper, with or
// without its stdin redirect.
func TestTheShellCommandIsTheAgentsOwn(t *testing.T) {
	for in, want := range map[string]string{
		"zsh -c source /h/.claude/shell-snapshots/snapshot-zsh-1.sh && eval 'npm run dev' < /dev/null && pwd -P >| /tmp/c":      "npm run dev",
		"zsh -c source /h/.claude/shell-snapshots/snapshot-zsh-1.sh && eval 'python3 -c \"save(1)\"' && pwd -P >| /tmp/c":       `python3 -c "save(1)"`,
		"zsh -c source /h/.claude/shell-snapshots/snapshot-zsh-1.sh && eval 'echo '\"'\"'a'\"'\"'' < /dev/null && pwd -P >| /x": "echo 'a'",
	} {
		if got := shellCommand(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

// A registry that says idle says no background work runs: the count the
// turn's end wrote is not shown.
func TestAnIdleRegistryShowsNoStaleAgents(t *testing.T) {
	root := copyFixture(t)
	transcript := filepath.Join(root, "home", ".claude", "projects", "-work-app", "11111111-aaaa-4000-8000-000000000002.jsonl")
	b, err := os.ReadFile(transcript)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, transcript, string(b)+`{"type":"system","subtype":"turn_duration","isSidechain":false,"pendingBackgroundAgentCount":2,"timestamp":"2026-09-21T13:00:00Z"}`+"\n")
	for _, s := range newFixtureSampler(root).Sample().Sessions {
		if s.PID == 310 && s.Agents != 0 {
			t.Fatalf("agents %d on an idle registry", s.Agents)
		}
	}
}

// A local command (a /compact, a /model) writes its lines as user lines and
// no turn_duration: its output is the end. While it runs, its command line
// is the newest, and the turn is not over.
func TestALocalCommandEndsWhenItsOutputIsWritten(t *testing.T) {
	running := claudeTurnEnded + `{"type":"user","isSidechain":false,"timestamp":"2026-10-12T04:55:05.555Z","message":{"role":"user","content":"/compact"}}` + "\n"
	done := running + `{"type":"system","subtype":"compact_boundary","isSidechain":false,"timestamp":"2026-10-12T04:56:16.914Z","compactMetadata":{"trigger":"manual","preTokens":710736,"postTokens":34796}}
{"type":"user","isSidechain":false,"isCompactSummary":true,"timestamp":"2026-10-12T04:56:15.504Z","message":{"role":"user","content":"This session is being continued from a previous conversation."}}
{"type":"user","isSidechain":false,"isMeta":true,"timestamp":"2026-10-12T04:55:05.562Z","message":{"role":"user","content":"<local-command-caveat>The command below was run directly.</local-command-caveat>"}}
{"type":"user","isSidechain":false,"timestamp":"2026-10-12T04:55:05.562Z","message":{"role":"user","content":"<command-name>/compact</command-name>"}}
{"type":"user","isSidechain":false,"timestamp":"2026-10-12T04:56:17.013Z","message":{"role":"user","content":"<local-command-stdout>Compacted</local-command-stdout>"}}
{"type":"attachment","isSidechain":false,"timestamp":"2026-10-12T04:56:17.100Z"}
`
	for body, want := range map[string]time.Time{
		running: {},
		done:    time.Date(2026, 10, 12, 4, 56, 17, 13e6, time.UTC),
	} {
		path := filepath.Join(t.TempDir(), "s.jsonl")
		writeFile(t, path, body)
		if u, _ := lastUsage(path, int64(len(body))); !u.Ended.Equal(want) {
			t.Errorf("ended %v, want %v", u.Ended, want)
		}
	}
}
