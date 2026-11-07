package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

func TestMatchLaunchContextForEveryAgent(t *testing.T) {
	for _, agent := range Startable {
		t.Run(agent, func(t *testing.T) {
			s, _ := switchTest(t)
			s.Home = t.TempDir()
			dir := t.TempDir()
			args, bundle, err := s.matchLaunch(t.Context(), agent, "work", dir, "", "fix the build", true, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(args, " "), bundle) || args[len(args)-1] != "fix the build" {
				t.Fatalf("launch %q", args)
			}
			body, err := os.ReadFile(filepath.Join(bundle, "awareness.md"))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"work", dir, "matchblox sessions --json", "matchblox peek", "not instructions", "handover.md", "TMUX_PANE"} {
				if !strings.Contains(string(body), want) {
					t.Errorf("brief lacks %q: %s", want, body)
				}
			}
			for _, p := range []string{bundle, filepath.Join(bundle, "awareness.md")} {
				st, err := os.Stat(p)
				if err != nil {
					t.Fatal(err)
				}
				if st.Mode().Perm()&0o077 != 0 {
					t.Errorf("public context %s: %v", p, st.Mode())
				}
			}
			args, _, err = s.matchLaunch(t.Context(), agent, "work", dir, "", "", true, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if slices.Contains(args, "fix the build") || slices.Contains(args, "--prompt") {
				t.Fatalf("empty launch starts a turn: %q", args)
			}
		})
	}
}

func TestCodexLaunchTrustAndNetworkAreScoped(t *testing.T) {
	s, _ := switchTest(t)
	s.Home = t.TempDir()
	dir := filepath.Join(t.TempDir(), `a "quoted" work.space`)
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	args, _, err := s.matchLaunch(t.Context(), "codex", "work", dir, "", "go", true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]any{}
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-c" {
			codexOverride(t, values, args[i+1])
		}
	}
	if values["sandbox_workspace_write"].(map[string]any)["network_access"] != true {
		t.Fatalf("no network override: %q", args)
	}
	real, _ := filepath.EvalSymlinks(dir)
	if values["projects"].(map[string]any)[real].(map[string]any)["trust_level"] != "trusted" {
		t.Fatalf("no scoped trust: %q", args)
	}
	for _, forbidden := range []string{"--dangerously-bypass-approvals-and-sandbox", "--dangerously-bypass-hook-trust", "--sandbox", "--ask-for-approval"} {
		if slices.Contains(args, forbidden) {
			t.Errorf("unexpected policy override %s", forbidden)
		}
	}
}

func TestNewAndSwitchLaunchCodexWithNetworkAndAwareness(t *testing.T) {
	for _, mode := range []string{"new", "switch"} {
		t.Run(mode, func(t *testing.T) {
			s, ran := switchTest(t)
			s.Home = t.TempDir()
			var res proto.Result
			if mode == "new" {
				res = s.act(t.Context(), newAct(t, proto.NewSession{Agent: "codex", Name: "fresh", Where: "/work/app", First: "continue"}))
			} else {
				res = s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "codex", Handoff: "continue"}))
			}
			if res.Err != "" || len(*ran) != 1 {
				t.Fatalf("launch %+v %q", res, *ran)
			}
			args := (*ran)[0]
			for _, want := range []string{"sandbox_workspace_write.network_access=true", "--add-dir", "CODEX_HOME=" + filepath.Join(s.Home, ".codex")} {
				if !slices.Contains(args, want) {
					t.Errorf("%s lacks %q: %q", mode, want, args)
				}
			}
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "trust_level=\"trusted\"") || !strings.Contains(joined, "MATCHBLOX_CONTEXT=") || args[len(args)-1] != "continue" {
				t.Fatalf("incomplete context: %q", args)
			}
		})
	}
}

func TestCodexRespectsRestrictiveTrustAndExistingInstructions(t *testing.T) {
	s, _ := switchTest(t)
	s.Home = t.TempDir()
	dir := t.TempDir()
	folder := filepath.Join(s.Home, ".codex")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(folder, "config.toml")
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("developer_instructions = 'Keep my rules.'\n")
	args, _, err := s.matchLaunch(t.Context(), "codex", "work", dir, "", "", true, nil, nil)
	if err != nil || !strings.Contains(strings.Join(args, " "), "Keep my rules.") {
		t.Fatalf("lost instructions: %q %v", args, err)
	}
	real, _ := filepath.EvalSymlinks(dir)
	write("[projects." + tomlString(real) + "]\ntrust_level = 'untrusted'\n")
	if _, _, err := s.matchLaunch(t.Context(), "codex", "work", dir, "", "", true, nil, nil); err == nil || !strings.Contains(err.Error(), "untrusted") {
		t.Fatalf("restrictive trust ignored: %v", err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "untrusted") {
		t.Fatal("changed user config")
	}
}

func TestSwitchCapturesContextBeforeReplacingAgent(t *testing.T) {
	s, ran := switchTest(t)
	s.Home = t.TempDir()
	transcript := filepath.Join(t.TempDir(), "old.jsonl")
	if err := os.WriteFile(transcript, []byte(`{"type":"user","message":{"role":"user","content":"Fix the export. Keep CSV only."}}`+"\n"+`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"The parser works. Next run the export test."}]}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	snap := lifeSnap(false)
	snap.Sessions[0].Transcript = transcript
	s.publish(snap, func(state.Doc) {})
	run := s.Actions.Run
	s.Actions.Run = func(argv []string) error {
		files, _ := filepath.Glob(filepath.Join(s.matchContextRoot(), "*", "capture.md"))
		if len(files) != 1 {
			t.Fatal("replacement starts before the handover is saved")
		}
		return run(argv)
	}
	res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "codex", Handoff: proto.DefaultHandoff}))
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	if len(*ran) != 1 {
		t.Fatalf("ran %q", *ran)
	}
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(s.StatePath), "matches", "*", "capture.md"))
	if len(files) != 1 {
		t.Fatalf("captures %q", files)
	}
	b, _ := os.ReadFile(files[0])
	for _, want := range []string{"Fix the export", "Keep CSV only", "Next run the export test", "unknown", "%1"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("capture lacks %q: %s", want, b)
		}
	}
}

func TestSwitchContextFailureLeavesTheAgentIntact(t *testing.T) {
	s, ran := switchTest(t)
	if err := os.WriteFile(s.matchContextRoot(), []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "codex", Handoff: proto.DefaultHandoff}))
	if res.Err == "" || len(*ran) != 0 {
		t.Fatalf("failure replaces agent: %+v %q", res, *ran)
	}
}

func TestSwitchRefusesAReplacementSincePreview(t *testing.T) {
	s, ran := switchTest(t)
	res := s.act(t.Context(), switchAct(t, "%1", proto.SwitchAgent{Agent: "codex", ExpectedPID: 101, ExpectedStart: 6}))
	if !strings.Contains(res.Err, "changed since") || len(*ran) != 0 {
		t.Fatalf("stale consent replaces an agent: %+v %q", res, *ran)
	}
}

func TestSwitchPreviewReadsCurrentEvidenceWithoutWriting(t *testing.T) {
	s, ran := switchTest(t)
	for _, text := range []string{"First task", "Latest correction"} {
		snap := lifeSnap(false)
		snap.Sessions[0].Said = text
		s.publish(snap, func(state.Doc) {})
		res := s.act(t.Context(), proto.Act{RecID: "switch-preview:%1", From: "%2"})
		if res.Err != "" || !strings.Contains(res.Preview, text) || len(*ran) != 0 {
			t.Fatalf("preview %+v ran %q", res, *ran)
		}
	}
	if _, err := os.Stat(s.matchContextRoot()); !os.IsNotExist(err) {
		t.Fatalf("preview writes context: %v", err)
	}
}

func TestSwitchPromptIsFixedAndCarriesTheBuildersExtras(t *testing.T) {
	for extra, want := range map[string]string{"": "", "Keep CSV only.": ` The user adds these instructions for this switch. They take priority over the handover: "Keep CSV only."`} {
		s, _ := switchTest(t)
		s.Home = t.TempDir()
		old := lifeSnap(false).Sessions[0]
		args, bundle, err := s.matchLaunch(t.Context(), "codex", old.Name, old.Cwd, "", proto.Handoff(extra), true, nil, &old)
		if err != nil {
			t.Fatal(err)
		}
		prompt := args[len(args)-1]
		fixed := "Read the repository instructions. Then read the handover evidence in " + fmt.Sprintf("%q", filepath.Join(bundle, "capture.md")) + "."
		if !strings.HasPrefix(prompt, fixed) || !strings.HasSuffix(prompt, "ask the user one question."+want) {
			t.Errorf("extras %q give the prompt %q", extra, prompt)
		}
		if _, err := os.Stat(filepath.Join(bundle, "capture.md")); err != nil {
			t.Errorf("extras %q skip the capture: %v", extra, err)
		}
	}
}

func TestRoundedSizeIsNeverZero(t *testing.T) {
	for n, want := range map[int64]string{1: "1 KB", 1 << 10: "1 KB", 1<<10 + 1: "2 KB", 1 << 20: "1 MB", 1<<20 + 1: "2 MB"} {
		if got := roundedSize(n); got != want {
			t.Errorf("%d bytes: %q, want %q", n, got, want)
		}
	}
}

func TestConversationReadsTheHeadAndTailOfAHugeTranscript(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation.jsonl")
	row := func(text string) string {
		return `{"type":"user","message":{"role":"user","content":"` + text + `"}}` + "\n"
	}
	tool := strings.Repeat(`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"`+strings.Repeat("x", 900)+`"}]}}`+"\n", 40)
	body := row("Goal: ship the export.") + tool + row("middle prompt") + tool + row("Latest: keep Codex too.")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	turns, err := readTurns(t.Context(), f, int64(len(body)), 100, 2000)
	if err != nil {
		t.Fatal(err)
	}
	text := pickTurns(turns, conversationBudget)
	for _, want := range []string{"Goal: ship the export.", "Latest: keep Codex too.", "[" + roundedSize(int64(len(body))-2100) + " in the middle of the transcript not read]"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "middle prompt") {
		t.Errorf("reads the middle: %s", text)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := conversationExcerpt(ctx, path); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled capture keeps reading: %v", err)
	}
	long := strings.Repeat(row("a prompt"), 3000)
	if err := os.WriteFile(path, []byte(long), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := conversationExcerpt(&endsAfter{Context: t.Context(), n: 1}, path); !errors.Is(err, context.Canceled) {
		t.Errorf("a capture cancelled mid-read keeps reading: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	f2, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()
	tools := strings.Repeat(`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"x"}]}}`+"\n", 200)
	if err := os.WriteFile(path, []byte(tools), 0o600); err != nil {
		t.Fatal(err)
	}
	turns, err = readTurns(t.Context(), f2, int64(len(tools)), 100, 100)
	if err != nil || len(turns) != 1 {
		t.Fatalf("turns %q %v", turns, err)
	}
	if _, err := excerptOf(turns); err == nil {
		t.Error("a transcript of tool output alone hides that intent is unknown")
	}
}

// endsAfter is a context that ends after n checks.
type endsAfter struct {
	context.Context
	n int
}

func (c *endsAfter) Err() error {
	if c.n--; c.n < 0 {
		return context.Canceled
	}
	return nil
}

func TestSwitchKeepsCustomAndEmptyTaskSemantics(t *testing.T) {
	for _, task := range []string{"", "Only explain the failing test; do not edit files."} {
		s, _ := switchTest(t)
		s.Home = t.TempDir()
		old := lifeSnap(false).Sessions[0]
		args, bundle, err := s.matchLaunch(t.Context(), "codex", old.Name, old.Cwd, "", task, true, nil, &old)
		if err != nil {
			t.Fatal(err)
		}
		_, captureErr := os.Stat(filepath.Join(bundle, "capture.md"))
		if task == "" {
			if !os.IsNotExist(captureErr) || args[len(args)-1] == "" {
				t.Fatalf("empty task creates a handover or prompt: %q %v", args, captureErr)
			}
		} else if args[len(args)-1] != task || captureErr != nil {
			t.Fatalf("custom task changed: %q %v", args, captureErr)
		}
	}
}

func TestSwitchCarriesTheOutgoingRecordWithoutTrustingIt(t *testing.T) {
	s, _ := switchTest(t)
	s.Home = t.TempDir()
	old := lifeSnap(false).Sessions[0]
	_, bundle, err := s.matchLaunch(t.Context(), old.Agent, old.Name, old.Cwd, "", "Fix exports", true, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "handover.md"), []byte("Goal: CSV exports.\nDecision: no XLSX.\nVerified: go test ./export\nNext action: test empty input.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old.MatchContext = filepath.Join(bundle, "awareness.md")
	if err := os.WriteFile(filepath.Join(bundle, "handover.md"), []byte(blankRecord+"\nLaunch message (user-supplied text):\nFix exports\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "handover.md"), []byte(blankRecord+"\nProgress: exports done.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if capture := s.captureMatch(t.Context(), old); strings.Contains(capture, "never wrote") || !strings.Contains(capture, "> Progress: exports done.") {
		t.Fatalf("an appended record is called unwritten: %s", capture)
	}
	if err := os.WriteFile(filepath.Join(bundle, "handover.md"), []byte(blankRecord+"\nLaunch message (user-supplied text):\nFix exports\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if capture := s.captureMatch(t.Context(), old); !strings.Contains(capture, "never wrote its task record") || strings.Contains(capture, "claims to verify)\n\n> # Task record") {
		t.Fatalf("a blank record passes as claims: %s", capture)
	}
	if err := os.WriteFile(filepath.Join(bundle, "handover.md"), []byte("Goal: CSV exports.\nDecision: no XLSX.\nVerified: go test ./export\nNext action: test empty input.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	capture := s.captureMatch(t.Context(), old)
	for _, want := range []string{"> Decision: no XLSX.", "> Next action: test empty input.", "claims to verify"} {
		if !strings.Contains(capture, want) {
			t.Errorf("capture lacks %q: %s", want, capture)
		}
	}
	outside := filepath.Join(t.TempDir(), "handover.md")
	if err := os.WriteFile(outside, []byte("MUST NOT READ THIS RECORD"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(bundle, "handover.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(bundle, "handover.md")); err != nil {
		t.Fatal(err)
	}
	if capture := s.captureMatch(t.Context(), old); strings.Contains(capture, "MUST NOT READ") || !strings.Contains(capture, "unavailable") {
		t.Fatalf("record escapes managed root: %s", capture)
	}
}

func TestConversationCaptureKeepsUserDirectionAndExcludesTools(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation.jsonl")
	lines := []string{
		`{"type":"response_item","payload":{"type":"message","role":"system","content":[{"type":"input_text","text":"SECRET SYSTEM TEXT"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>\n  <cwd>/HARNESS SECRET</cwd>\n</environment_context>"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"# AGENTS.md instructions for /x\n\nHARNESS SECRET"}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"<system-reminder>HARNESS SECRET</system-reminder>"},{"type":"text","text":"Keep JSON too."}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Keep CSV only."}]}}`,
		`{"type":"response_item","payload":{"type":"function_call_output","output":"TOOL SECRET"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Next: verify empty input."}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"MORE TOOL SECRET"}]}}`,
		`{"malformed`,
		`{"type":"user","message":{"role":"user","content":"Latest correction: no XLSX."}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	text, err := conversationExcerpt(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Keep JSON too.", "Keep CSV only.", "Next: verify empty input.", "Latest correction: no XLSX."} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "SECRET") {
		t.Fatalf("includes system or tool text: %s", text)
	}
}

// A long Claude session is mostly tool output: the 5 largest transcripts on
// the builder's host kept 0 or 1 of 62-247 prompts in their last 128 KB.
func TestConversationCaptureKeepsTheGoalPastToolOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation.jsonl")
	tool := `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"` + strings.Repeat("x", 200*1024) + `"}]}}`
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"<command-name>/clear</command-name>\n<command-message>clear</command-message>\n<command-args></command-args>"}}`,
		`{"type":"user","message":{"role":"user","content":"<task-notification>HARNESS SECRET</task-notification>"}}`,
		`{"type":"user","message":{"role":"user","content":"<system-reminder><command-name>/HARNESS SECRET</command-name></system-reminder>"}}`,
		`{"type":"user","message":{"role":"user","content":"Pasted log: <command-name>/pasted</command-name> ends."}}`,
		`{"type":"user","message":{"role":"user","content":"<command-message>review</command-message>\n<command-name>/review</command-name>\n<command-args>PR 12</command-args>"}}`,
		`{"type":"user","message":{"role":"user","content":"<command-name>/using-superpowers</command-name>\n<command-message>using-superpowers</command-message>\n<command-args>the handover is lazy</command-args>"}}`,
		`{"type":"user","isMeta":true,"message":{"role":"user","content":[{"type":"text","text":"Base directory for this skill: HARNESS SECRET"}]}}`,
		`{"type":"user","message":{"role":"user","content":"Goal: fix the capture."}}`,
	}
	for range 4 {
		lines = append(lines, tool)
	}
	lines = append(lines, `{"type":"user","message":{"role":"user","content":"Latest correction: keep Codex too."}}`)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	text, err := conversationExcerpt(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/using-superpowers the handover is lazy", "/review PR 12", "Pasted log: <command-name>", "Goal: fix the capture.", "Latest correction: keep Codex too."} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q: %s", want, text)
		}
	}
	tight := pickTurns([]turn{{"user", "/clear"}, {"user", "Goal: fix the capture."}, {"user", strings.Repeat("later ", 10)}, {"user", "Latest."}}, 60)
	if !strings.Contains(tight, "Goal: fix the capture.") || strings.Contains(tight, "/clear") {
		t.Errorf("a bare command takes the goal's place: %s", tight)
	}
	if strings.Contains(text, "SECRET") || strings.Contains(text, "xxxx") {
		t.Fatalf("includes harness or tool text: %s", text)
	}
}

// Each row below is one Claude Code writes on the builder's host: 129
// transcripts hold a compaction summary, 60 an interrupt marker.
func TestConversationCaptureDropsHarnessRowsAndKeepsWholeIntent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation.jsonl")
	long := strings.Repeat("log line\\n", 1000) + "NOW FIX THE NULL CHECK"
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"<command-name>/model</command-name>\n<command-message>model</command-message>\n<command-args>opus</command-args>"}}`,
		`{"type":"user","message":{"role":"user","content":"Goal: ship the export."}}`,
		`{"type":"user","isCompactSummary":true,"message":{"role":"user","content":"This session is being continued from a previous conversation. HARNESS SECRET"}}`,
		`{"type":"user","isVisibleInTranscriptOnly":true,"message":{"role":"user","content":"HARNESS SECRET"}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"[Request interrupted by user for tool use] HARNESS SECRET"}]}}`,
		`{"type":"user","message":{"role":"user","content":"<bash-stdout>HARNESS SECRET</bash-stdout><bash-stderr></bash-stderr>"}}`,
		`{"type":"user","message":{"role":"user","content":"<local-command-stderr>HARNESS SECRET</local-command-stderr>"}}`,
		`{"type":"user","message":{"role":"user","content":"<bash-input>gh run list</bash-input>"}}`,
		`{"type":"user","message":{"role":"user","content":"` + long + `"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Reply one, first part."}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"TOOL SECRET"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Reply one, conclusion."}]}}`,
	}
	for i := range 3 {
		lines = append(lines, fmt.Sprintf(`{"type":"user","message":{"role":"user","content":"later %d"}}`, i),
			fmt.Sprintf(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"later reply %d"}]}}`, i))
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	text, err := conversationExcerpt(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "SECRET") {
		t.Errorf("includes harness or tool text: %s", text)
	}
	for _, want := range []string{"log line", "NOW FIX THE NULL CHECK", "[middle of message omitted]", "user:\n! gh run list\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("a long prompt loses %q", want)
		}
	}
	tight := pickTurns([]turn{{"user", "/model opus"}, {"user", "! gh run list"}, {"user", "Goal: ship the export."}, {"user", strings.Repeat("later ", 10)}, {"user", "Latest."}}, 60)
	if !strings.Contains(tight, "Goal: ship the export.") {
		t.Errorf("a setting command takes the goal's place: %s", tight)
	}
	turns := []turn{}
	for _, l := range lines[9:12] {
		turns = appendTurn(turns, []byte(l))
	}
	if len(turns) != 1 || turns[0].text != "Reply one, first part.\n\nReply one, conclusion." {
		t.Errorf("one reply split over rows stays split: %q", turns)
	}
}

func TestConversationCaptureKeepsFirstAndLatestPromptsWithinBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation.jsonl")
	var lines []string
	for i := range 400 {
		lines = append(lines, fmt.Sprintf(`{"type":"user","message":{"role":"user","content":"prompt %03d %s"}}`, i, strings.Repeat("p", 500)))
		lines = append(lines, fmt.Sprintf(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"reply %03d %s"}]}}`, i, strings.Repeat("r", 500)))
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	text, err := conversationExcerpt(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(text) > conversationBudget+1024 {
		t.Fatalf("excerpt is %d bytes, budget %d", len(text), conversationBudget)
	}
	for _, want := range []string{"prompt 000", "prompt 399", "reply 399", "omitted"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(text, "reply 300") {
		t.Fatal("old assistant text crowds out prompts")
	}
	if strings.Index(text, "prompt 000") > strings.Index(text, "prompt 399") {
		t.Fatal("excerpt is out of order")
	}
}

func TestCodexLaunchUsesSelectedAccountAndKeepsConfig(t *testing.T) {
	s, _ := switchTest(t)
	s.Home = t.TempDir()
	for _, account := range []*onAccount{nil, {folder: t.TempDir(), name: "work"}} {
		folder := s.accountFolder("codex", account)
		if err := os.MkdirAll(folder, 0o700); err != nil {
			t.Fatal(err)
		}
		config := "developer_instructions = 'Account-specific instruction'\nsandbox_mode = 'read-only'\napproval_policy = 'on-request'\n"
		path := filepath.Join(folder, "config.toml")
		if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
		args, _, err := s.matchLaunch(t.Context(), "codex", "work", t.TempDir(), "", "", true, account, nil)
		if err != nil || !strings.Contains(strings.Join(args, " "), "Account-specific instruction") {
			t.Fatalf("lost account config: %q %v", args, err)
		}
		b, err := os.ReadFile(path)
		if err != nil || string(b) != config {
			t.Fatalf("changed config: %q %v", b, err)
		}
	}
}

func TestCodexLaunchRejectsMalformedConfigWithoutLeakingValues(t *testing.T) {
	s, _ := switchTest(t)
	s.Home = t.TempDir()
	acct := &onAccount{folder: t.TempDir()}
	if err := os.WriteFile(filepath.Join(acct.folder, "config.toml"), []byte("token = SECRET-VALUE-DO-NOT-PRINT"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := s.matchLaunch(t.Context(), "codex", "work", t.TempDir(), "", "", true, acct, nil)
	if err == nil || strings.Contains(err.Error(), "SECRET-VALUE") {
		t.Fatalf("leaked config: %v", err)
	}
}

func TestContextPathsNeverLeaveTheHostSnapshot(t *testing.T) {
	session := sample.Session{MatchContext: "/private/context/awareness.md", Transcript: "/private/transcript"}
	b, err := json.Marshal(session)
	if err != nil || strings.Contains(string(b), "/private/") {
		t.Fatalf("private paths in snapshot: %s %v", b, err)
	}
}

// codexOverride applies a -c argument as Codex 0.164 parses it: the key splits
// on every dot with no quoting, and only the value is TOML.
func codexOverride(t *testing.T, into map[string]any, arg string) {
	t.Helper()
	key, raw, ok := strings.Cut(arg, "=")
	if !ok {
		t.Fatalf("override %q has no value", arg)
	}
	var v struct{ V any }
	if _, err := toml.Decode("V = "+raw, &v); err != nil {
		t.Fatalf("override %q: %v", arg, err)
	}
	parts := strings.Split(key, ".")
	m := into
	for _, p := range parts[:len(parts)-1] {
		next, _ := m[p].(map[string]any)
		if next == nil {
			next = map[string]any{}
			m[p] = next
		}
		m = next
	}
	m[parts[len(parts)-1]] = v.V
}

// A folder just cloned is not one the builder chose: the agent asks whether
// to trust it. Network access and the brief still apply.
func TestAFreshCloneIsNotTrusted(t *testing.T) {
	s, _ := switchTest(t)
	s.Home = t.TempDir()
	dir := t.TempDir()
	args, _, err := s.matchLaunch(t.Context(), "codex", "work", dir, "", "go", false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "trust_level") || !slices.Contains(args, "sandbox_workspace_write.network_access=true") || !strings.Contains(joined, "MATCHBLOX_CONTEXT=") {
		t.Fatalf("codex on a fresh clone: %q", args)
	}
	args, _, err = s.matchLaunch(t.Context(), sample.CursorAgent, "work", dir, "", "go", false, nil, nil)
	if err != nil || slices.Contains(args, "--trust") {
		t.Fatalf("cursor on a fresh clone: %q %v", args, err)
	}
}
