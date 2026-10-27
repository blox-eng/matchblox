package service

import (
	"encoding/json"
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
	text, err := conversationTail(path)
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
	if err := os.WriteFile(path, []byte(strings.Repeat("x", matchEvidenceLimit+100)+"\n"+lines[len(lines)-1]+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	text, err = conversationTail(path)
	if err != nil || !strings.Contains(text, "Latest correction") {
		t.Fatalf("bounded tail loses latest direction: %q %v", text, err)
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
