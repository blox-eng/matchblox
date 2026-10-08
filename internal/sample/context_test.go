package sample

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The made-up credentials of the fixtures. None of them may reach a
// snapshot or the screen.
const (
	secretAccess  = "SECRET-access-0f9e8d"
	secretRefresh = "SECRET-refresh-7c6b5a"
	secretAPIKey  = "SECRET-sk-proj-4d3c2b"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func setEnv(t *testing.T, root string, pid int, kv ...string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "proc", strconv.Itoa(pid), "environ"), strings.Join(kv, "\x00")+"\x00")
}

// idToken is an unsigned JWT with the claims the Codex login writes.
func idToken(email, plan string) string {
	enc := base64.RawURLEncoding.EncodeToString
	claims, _ := json.Marshal(map[string]any{
		"email": email,
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_plan_type":  plan,
			"chatgpt_account_id": "acct-made-up",
		},
	})
	return enc([]byte(`{"alg":"RS256"}`)) + "." + enc(claims) + ".c2lnbmF0dXJl"
}

// codexRollout is a rollout as codex 0.147 writes it: the session meta, a
// turn context with the model, a token_count without info (rate limits
// only), then one with the last turn's usage and the window.
func codexRollout(model string, total, window int) string {
	return strings.Join([]string{
		`{"timestamp":"2026-09-21T10:00:00Z","type":"session_meta","payload":{"id":"01a0-made-up","cwd":"/work/api","cli_version":"0.147.0"}}`,
		`{"timestamp":"2026-09-21T10:00:01Z","type":"turn_context","payload":{"model":"` + model + `","cwd":"/work/api"}}`,
		`{"timestamp":"2026-09-21T10:00:02Z","type":"event_msg","payload":{"type":"token_count","info":null,"rate_limits":{"primary":null}}}`,
		`{"timestamp":"2026-09-21T10:00:09Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":900000,"total_tokens":960826},"last_token_usage":{"input_tokens":50000,"cached_input_tokens":40000,"output_tokens":1680,"reasoning_output_tokens":500,"total_tokens":` + strconv.Itoa(total) + `},"model_context_window":` + strconv.Itoa(window) + `}}}`,
		`{"timestamp":"2026-09-21T10:00:10Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}}`,
	}, "\n") + "\n"
}

func codexAuth(root string) string {
	return `{"OPENAI_API_KEY":null,"auth_mode":"chatgpt","last_refresh":"2026-09-21T09:00:00Z","tokens":{"id_token":"` +
		idToken("builder@example.org", "plus") + `","access_token":"` + secretAccess + `","refresh_token":"` + secretRefresh + `","account_id":"acct-made-up"}}`
}

// TestCodexContextUseComesFromTheRolloutItHoldsOpen: the process holds its
// rollout file open; the newest token_count with info gives the tokens of
// the last turn and the window.
func TestCodexContextUseComesFromTheRolloutItHoldsOpen(t *testing.T) {
	root := copyFixture(t)
	home := filepath.Join(root, "home")
	rollout := filepath.Join(home, ".codex", "sessions", "2026", "09", "21", "rollout-2026-09-21T10-00-00-01a0-made-up.jsonl")
	writeFile(t, rollout, codexRollout("gpt-made-up", 51680, 258400))
	writeFile(t, filepath.Join(home, ".codex", "auth.json"), codexAuth(root))

	addProc(t, root, 900, 1, "codex", []string{"/opt/codex/bin/codex"}, "%20")
	addPane(t, root, "%20\tapi:1.1\tw\t900\tcodex\t/work/api")
	symlink(t, "/dev/pts/3", filepath.Join(root, "proc", "900", "fd", "0"))
	symlink(t, rollout, filepath.Join(root, "proc", "900", "fd", "21"))
	// Before its first turn a Codex process holds no rollout: fresh. (One
	// whose open files cannot be read is not measured: TestAPaneShows….)
	addProc(t, root, 910, 1, "codex", []string{"codex"}, "%21")
	addPane(t, root, "%21\tweb:1.1\tw\t910\tcodex\t/work/web")
	symlink(t, "/dev/pts/4", filepath.Join(root, "proc", "910", "fd", "0"))
	// An npm install runs the native binary as a child: the child holds it.
	child := filepath.Join(home, ".codex", "sessions", "2026", "09", "21", "rollout-2026-09-21T11-00-00-01a0-child.jsonl")
	writeFile(t, child, codexRollout("gpt-made-up", 129200, 258400))
	addProc(t, root, 920, 1, "MainThread", []string{"node", "/usr/local/bin/codex"}, "%22")
	addProc(t, root, 921, 920, "codex", []string{"/usr/local/lib/node_modules/@openai/codex/vendor/codex"}, "%22")
	addPane(t, root, "%22\tops:1.1\tw\t920\tnode\t/work/ops")
	symlink(t, child, filepath.Join(root, "proc", "921", "fd", "30"))

	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "codex"}
	got := byPane(smp.Sample())

	s := got["%20"]
	if s.Agent != "codex" || s.Context != "known" || s.Tokens != 51680 || s.Window != 258400 || s.Model != "gpt-made-up" {
		t.Fatalf("codex session: %+v", s)
	}
	if s.ContextPct < 19.9 || s.ContextPct > 20.1 {
		t.Errorf("context %.1f%%, want 20%%", s.ContextPct)
	}
	if s.Account != "builder@example.org · Plus" || s.Provider != "openai" {
		t.Errorf("account %q provider %q", s.Account, s.Provider)
	}
	if f := got["%21"]; f.Context != "fresh" || f.Tokens != 0 {
		t.Errorf("before its first turn: %+v", f)
	}
	if c := got["%22"]; c.Context != "known" || c.Tokens != 129200 {
		t.Errorf("npm codex: %+v", c)
	}
}

// TestTheConfiguredWindowWinsForEveryAgent: [sessions.windows] overrides the
// window that the agent reports.
func TestTheConfiguredWindowWinsForEveryAgent(t *testing.T) {
	root := copyFixture(t)
	home := filepath.Join(root, "home")
	rollout := filepath.Join(home, ".codex", "sessions", "2026", "rollout-2026-09-21T10-00-00-made-up.jsonl")
	writeFile(t, rollout, codexRollout("gpt-made-up", 51680, 258400))
	addProc(t, root, 900, 1, "codex", []string{"codex"}, "%20")
	addPane(t, root, "%20\tapi:1.1\tw\t900\tcodex\t/work/api")
	symlink(t, rollout, filepath.Join(root, "proc", "900", "fd", "21"))

	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "codex"}
	smp.Rules.Windows = map[string]int{"gpt-made": 100_000}
	if s := byPane(smp.Sample())["%20"]; s.Window != 100_000 {
		t.Fatalf("window %d, want the configured 100000", s.Window)
	}
}

// opencodeStore writes a session of OpenCode 1.1 storage: the session file,
// one finished assistant message and one that has not written output yet.
func opencodeStore(t *testing.T, data, id, dir string, updated int64, parent string) {
	t.Helper()
	sess := map[string]any{"id": id, "directory": dir, "projectID": "p1", "time": map[string]any{"created": updated - 60_000, "updated": updated}}
	if parent != "" {
		sess["parentID"] = parent
	}
	b, _ := json.Marshal(sess)
	writeFile(t, filepath.Join(data, "storage", "session", "p1", id+".json"), string(b))
	msg := func(n, out int) string {
		return `{"id":"msg_` + n2(n) + `","sessionID":"` + id + `","role":"assistant","modelID":"model-made-up","providerID":"zen",` +
			`"tokens":{"input":3760,"output":` + strconv.Itoa(out) + `,"reasoning":86,"cache":{"read":37544,"write":10}},"time":{"created":1}}`
	}
	writeFile(t, filepath.Join(data, "storage", "message", id, "msg_"+n2(1)+".json"), `{"id":"msg_0","sessionID":"`+id+`","role":"user","time":{"created":1}}`)
	writeFile(t, filepath.Join(data, "storage", "message", id, "msg_"+n2(2)+".json"), msg(2, 600))
	writeFile(t, filepath.Join(data, "storage", "message", id, "msg_"+n2(3)+".json"), msg(3, 0))
}

func n2(n int) string { return "c1ed00000" + strconv.Itoa(n) + "abc" }

// TestOpenCodeContextUseIsCountedTheWayOpenCodeCountsIt: the newest
// assistant message with output, input + output + reasoning + cache read +
// cache write, against the model's limit in OpenCode's models cache.
func TestOpenCodeContextUseIsCountedTheWayOpenCodeCountsIt(t *testing.T) {
	root := copyFixture(t)
	home := filepath.Join(root, "home")
	data := filepath.Join(home, ".local", "share", "opencode")
	updated := fixtureNow.UnixMilli() - 60_000
	opencodeStore(t, data, "ses_main", "/work/web", updated, "")
	// A subagent's session in the same directory, newer: never the TUI's.
	opencodeStore(t, data, "ses_sub", "/work/web", updated+1000, "ses_main")
	writeFile(t, filepath.Join(home, ".cache", "opencode", "models.json"), `{"zen":{"models":{"model-made-up":{"limit":{"context":200000,"output":128000}}}}}`)
	writeFile(t, filepath.Join(data, "auth.json"), `{"zen":{"type":"api","key":"`+secretAPIKey+`"}}`)

	addProc(t, root, 930, 1, "opencode", []string{"opencode"}, "%23")
	symlink(t, "/work/web", filepath.Join(root, "proc", "930", "cwd"))
	addPane(t, root, "%23\tweb:1.1\tw\t930\topencode\t/work/web")

	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "opencode"}
	s := byPane(smp.Sample())["%23"]
	want := 3760 + 600 + 86 + 37544 + 10
	if s.Agent != "opencode" || s.Context != "known" || s.Tokens != want || s.Window != 200000 || s.Model != "model-made-up" {
		t.Fatalf("opencode session: %+v (want %d tokens)", s, want)
	}
	if s.Account != "API key" || s.Provider != "zen" {
		t.Errorf("account %q provider %q", s.Account, s.Provider)
	}
}

// TestOpenCodeWithoutAWindowOrWithATwinIsNotMeasured: a model the cache
// does not know has no window, and two OpenCode processes in one directory
// cannot be told apart: both show "not measured", never a guess.
func TestOpenCodeWithoutAWindowOrWithATwinIsNotMeasured(t *testing.T) {
	root := copyFixture(t)
	data := filepath.Join(root, "home", ".local", "share", "opencode")
	opencodeStore(t, data, "ses_a", "/work/web", fixtureNow.UnixMilli()-60_000, "")
	opencodeStore(t, data, "ses_b", "/work/docs", fixtureNow.UnixMilli()-60_000, "")
	for i, p := range []struct {
		pid int
		cwd string
	}{{940, "/work/web"}, {941, "/work/docs"}, {942, "/work/docs"}} {
		pane := "%" + strconv.Itoa(30+i)
		addProc(t, root, p.pid, 1, "opencode", []string{"opencode"}, pane)
		symlink(t, p.cwd, filepath.Join(root, "proc", strconv.Itoa(p.pid), "cwd"))
		addPane(t, root, pane+"\tw:"+strconv.Itoa(i)+".1\tw\t"+strconv.Itoa(p.pid)+"\topencode\t"+p.cwd)
	}
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "opencode"}
	got := byPane(smp.Sample())
	if s := got["%30"]; s.Context != "unmeasured" || s.Model != "model-made-up" || s.Tokens == 0 {
		t.Errorf("no window: %+v", s)
	}
	for _, pane := range []string{"%31", "%32"} {
		if s := got[pane]; s.Context != "unmeasured" || s.Tokens != 0 {
			t.Errorf("twin %s: %+v", pane, s)
		}
	}
}

// TestEachClaudeSessionShowsTheAccountOfItsConfigDirectory: the login comes
// from .claude.json next to the config directory the process uses, so two
// sessions under two accounts show two labels.
func TestEachClaudeSessionShowsTheAccountOfItsConfigDirectory(t *testing.T) {
	root := copyFixture(t)
	home := filepath.Join(root, "home")
	writeFile(t, filepath.Join(home, ".claude.json"), `{"userID":"`+secretAccess+`","oauthAccount":{"emailAddress":"builder@example.com","organizationType":"claude_max","organizationName":"Example"},"projects":{}}`)
	work := filepath.Join(root, "work-claude")
	writeFile(t, filepath.Join(work, ".claude.json"), `{"oauthAccount":{"emailAddress":"work@example.net","organizationType":"claude_team"}}`)
	// The work session's registry lives in its own config directory.
	b, err := os.ReadFile(filepath.Join(home, ".claude", "sessions", "400.json"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(work, "sessions", "950.json"), string(b))
	addProc(t, root, 950, 1, "claude", []string{"claude"}, "%40")
	setEnv(t, root, 950, "TMUX_PANE=%40", "CLAUDE_CONFIG_DIR="+work)
	addPane(t, root, "%40\twork:1.1\tw\t950\tclaude\t/work/app")
	// An API key session has no login.
	addProc(t, root, 960, 1, "claude", []string{"claude"}, "%41")
	setEnv(t, root, 960, "TMUX_PANE=%41", "CLAUDE_CONFIG_DIR="+filepath.Join(root, "empty"), "ANTHROPIC_API_KEY="+secretAPIKey)
	addPane(t, root, "%41\tkey:1.1\tw\t960\tclaude\t/work/app")

	smp := newFixtureSampler(root)
	got := byPane(smp.Sample())
	if s := got["%1"]; s.Agent != "claude" || s.Account != "builder@example.com · Max" || s.Provider != "anthropic" {
		t.Errorf("default config: agent %q account %q provider %q", s.Agent, s.Account, s.Provider)
	}
	if s := got["%40"]; s.Account != "work@example.net · Team" || s.SessionID == "" {
		t.Errorf("CLAUDE_CONFIG_DIR: account %q session %q", s.Account, s.SessionID)
	}
	if s := got["%41"]; s.Account != "API key" {
		t.Errorf("API key session: %q", s.Account)
	}
	// Claude Code through a cloud: the cloud is the provider, no login.
	addProc(t, root, 970, 1, "claude", []string{"claude"}, "%42")
	setEnv(t, root, 970, "TMUX_PANE=%42", "CLAUDE_CODE_USE_BEDROCK=1")
	addPane(t, root, "%42\tcloud:1.1\tw\t970\tclaude\t/work/app")
	if s := byPane(newFixtureSampler(root).Sample())["%42"]; s.Provider != "bedrock" || s.Account != "" {
		t.Errorf("bedrock session: provider %q account %q", s.Provider, s.Account)
	}
}

// TestAnAccountLabelCarriesNoControlKeys: the files are untrusted.
func TestAnAccountLabelCarriesNoControlKeys(t *testing.T) {
	root := copyFixture(t)
	writeFile(t, filepath.Join(root, "home", ".claude.json"), `{"oauthAccount":{"emailAddress":"a\u001b]52;c;aGk=\u0007@example.com","organizationType":"claude_pro"}}`)
	s := byPane(newFixtureSampler(root).Sample())["%1"]
	if strings.ContainsAny(s.Account, "\x1b\x07") || !strings.HasSuffix(s.Account, "· Pro") {
		t.Fatalf("account %q", s.Account)
	}
}

// TestNoCredentialReachesTheSnapshot: the auth files hold tokens and keys
// next to the fields matchblox reads. None of them is in the snapshot.
func TestNoCredentialReachesTheSnapshot(t *testing.T) {
	root := copyFixture(t)
	home := filepath.Join(root, "home")
	rollout := filepath.Join(home, ".codex", "sessions", "2026", "rollout-2026-09-21T10-00-00-made-up.jsonl")
	writeFile(t, rollout, codexRollout("gpt-made-up", 51680, 258400))
	writeFile(t, filepath.Join(home, ".codex", "auth.json"), codexAuth(root))
	writeFile(t, filepath.Join(home, ".claude.json"), `{"primaryApiKey":"`+secretAPIKey+`","oauthAccount":{"emailAddress":"builder@example.com","organizationType":"claude_max"}}`)
	writeFile(t, filepath.Join(home, ".claude", ".credentials.json"), `{"claudeAiOauth":{"accessToken":"`+secretAccess+`","refreshToken":"`+secretRefresh+`"}}`)
	addProc(t, root, 900, 1, "codex", []string{"codex"}, "%20")
	addPane(t, root, "%20\tapi:1.1\tw\t900\tcodex\t/work/api")
	symlink(t, rollout, filepath.Join(root, "proc", "900", "fd", "21"))

	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "codex"}
	b, err := json.Marshal(smp.Sample())
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{secretAccess, secretRefresh, secretAPIKey, idToken("builder@example.org", "plus"), "acct-made-up", "SECRET"} {
		if strings.Contains(string(b), secret) {
			t.Errorf("the snapshot holds %q", secret)
		}
	}
	if !strings.Contains(string(b), "builder@example.org") {
		t.Fatal("positive control: the codex account label is missing, so the check above proves nothing")
	}
}

// TestAProcessOfAnotherHomeGetsNoLabelOfOurs: an agent started with
// another HOME keeps its files there. Without its own config directory in
// its environment, matchblox shows no account and no OpenCode figure for
// it, never those of the service's home.
func TestAProcessOfAnotherHomeGetsNoLabelOfOurs(t *testing.T) {
	root := copyFixture(t)
	home := filepath.Join(root, "home")
	writeFile(t, filepath.Join(home, ".claude.json"), `{"oauthAccount":{"emailAddress":"builder@example.com","organizationType":"claude_max"}}`)
	writeFile(t, filepath.Join(root, "other-claude", ".claude.json"), `{"oauthAccount":{"emailAddress":"builder@example.com","organizationType":"claude_max"}}`)
	writeFile(t, filepath.Join(home, ".codex", "auth.json"), codexAuth(root))
	data := filepath.Join(home, ".local", "share", "opencode")
	opencodeStore(t, data, "ses_main", "/work/web", fixtureNow.UnixMilli()-60_000, "")
	writeFile(t, filepath.Join(home, ".cache", "opencode", "models.json"), `{"zen":{"models":{"model-made-up":{"limit":{"context":200000}}}}}`)

	for i, p := range []struct {
		pid  int
		comm string
		env  []string
	}{
		{980, "claude", []string{"HOME=/home/other"}},
		{981, "codex", []string{"HOME=/home/other"}},
		{982, "opencode", []string{"HOME=/home/other"}},
		{983, "claude", []string{"HOME=/home/other", "CLAUDE_CONFIG_DIR=" + filepath.Join(root, "other-claude")}},
	} {
		pane := "%" + strconv.Itoa(50+i)
		addProc(t, root, p.pid, 1, p.comm, []string{p.comm}, pane)
		setEnv(t, root, p.pid, append([]string{"TMUX_PANE=" + pane}, p.env...)...)
		symlink(t, "/work/web", filepath.Join(root, "proc", strconv.Itoa(p.pid), "cwd"))
		addPane(t, root, pane+"\tother:"+strconv.Itoa(i)+".1\tw\t"+strconv.Itoa(p.pid)+"\t"+p.comm+"\t/work/web")
	}
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "codex", "opencode"}
	got := byPane(smp.Sample())
	for _, pane := range []string{"%50", "%51", "%52"} {
		if s := got[pane]; s.Account != "" || s.Context == "known" {
			t.Errorf("%s (%s, another HOME): account %q context %q", pane, s.Agent, s.Account, s.Context)
		}
	}
	if s := got["%53"]; s.Account != "builder@example.com · Max" {
		t.Errorf("its own CLAUDE_CONFIG_DIR: account %q", s.Account)
	}
	// Positive control: our own processes still get their label.
	if s := got["%1"]; s.Account == "" {
		t.Fatal("our own session lost its label")
	}
}

// TestCodexBurnIsMeasured: the 30-minute burn of a Codex session follows
// its rollout; it is never a stale 0.
func TestCodexBurnIsMeasured(t *testing.T) {
	root := copyFixture(t)
	rollout := filepath.Join(root, "home", ".codex", "sessions", "rollout-2026-09-21T10-00-00-burn.jsonl")
	writeFile(t, rollout, codexRollout("gpt-made-up", 10_000, 258400))
	addProc(t, root, 900, 1, "codex", []string{"codex"}, "%20")
	addPane(t, root, "%20\tapi:1.1\tw\t900\tcodex\t/work/api")
	symlink(t, rollout, filepath.Join(root, "proc", "900", "fd", "21"))
	now := fixtureNow
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "codex"}
	smp.Now = func() time.Time { return now }
	smp.Sample()
	now = now.Add(5 * time.Minute)
	writeFile(t, rollout, codexRollout("gpt-made-up", 90_000, 258400)+"\n")
	if s := byPane(smp.Sample())["%20"]; s.Burn30m != 80_000 {
		t.Fatalf("burn %d, want 80000", s.Burn30m)
	}
}

// TestCodexWithoutAWindowIsNotMeasuredUnlessConfigured: a model Codex has
// no window for writes null; the config can give one.
func TestCodexWithoutAWindowIsNotMeasuredUnlessConfigured(t *testing.T) {
	root := copyFixture(t)
	rollout := filepath.Join(root, "home", ".codex", "sessions", "rollout-2026-09-21T10-00-00-oss.jsonl")
	writeFile(t, rollout, strings.Replace(codexRollout("local-oss", 42_000, 1), `"model_context_window":1`, `"model_context_window":null`, 1))
	addProc(t, root, 900, 1, "codex", []string{"codex"}, "%20")
	addPane(t, root, "%20\tapi:1.1\tw\t900\tcodex\t/work/api")
	symlink(t, rollout, filepath.Join(root, "proc", "900", "fd", "21"))
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "codex"}
	if s := byPane(smp.Sample())["%20"]; s.Context != "unmeasured" || s.Tokens != 42_000 || s.Model != "local-oss" {
		t.Fatalf("no window: %+v", s)
	}
	smp = newFixtureSampler(root)
	smp.Agents = []string{"claude", "codex"}
	smp.Rules.Windows = map[string]int{"local": 64_000, "local-oss": 128_000}
	if s := byPane(smp.Sample())["%20"]; s.Context != "known" || s.Window != 128_000 {
		t.Fatalf("the longest configured prefix wins: %+v", s)
	}
}

// TestACodexRolloutWithoutUsageInItsTailIsNotMeasured: a session whose
// last 512 KB holds no token_count is not fresh: it is not measured.
func TestACodexRolloutWithoutUsageInItsTailIsNotMeasured(t *testing.T) {
	root := copyFixture(t)
	rollout := filepath.Join(root, "home", ".codex", "sessions", "rollout-2026-09-21T10-00-00-long.jsonl")
	long := strings.Repeat(`{"type":"response_item","payload":{"type":"function_call_output","output":"`+strings.Repeat("x", 1000)+`"}}`+"\n", 600)
	writeFile(t, rollout, codexRollout("gpt-made-up", 51680, 258400)+long)
	addProc(t, root, 900, 1, "codex", []string{"codex"}, "%20")
	addPane(t, root, "%20\tapi:1.1\tw\t900\tcodex\t/work/api")
	symlink(t, rollout, filepath.Join(root, "proc", "900", "fd", "21"))
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "codex"}
	if s := byPane(smp.Sample())["%20"]; s.Context != "unmeasured" {
		t.Fatalf("context %q, want unmeasured", s.Context)
	}
}

// TestACodexProcessWithTwoRolloutsShowsTheNewest: the rollout written last
// is the session on screen, whatever the fd numbers.
func TestACodexProcessWithTwoRolloutsShowsTheNewest(t *testing.T) {
	root := copyFixture(t)
	dir := filepath.Join(root, "home", ".codex", "sessions")
	old, cur := filepath.Join(dir, "rollout-a-old.jsonl"), filepath.Join(dir, "rollout-b-new.jsonl")
	writeFile(t, old, codexRollout("gpt-made-up", 200_000, 258400))
	writeFile(t, cur, codexRollout("gpt-made-up", 25_840, 258400))
	past := fixtureNow.Add(-time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	addProc(t, root, 900, 1, "codex", []string{"codex"}, "%20")
	addPane(t, root, "%20\tapi:1.1\tw\t900\tcodex\t/work/api")
	symlink(t, old, filepath.Join(root, "proc", "900", "fd", "21"))
	symlink(t, cur, filepath.Join(root, "proc", "900", "fd", "3"))
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "codex"}
	if s := byPane(smp.Sample())["%20"]; s.Tokens != 25_840 {
		t.Fatalf("tokens %d, want the newest rollout's 25840", s.Tokens)
	}
}

// TestOpenCodeWithTwoSessionsSinceItStartedIsNotMeasured: a finished
// `opencode run` in the same directory, or an `opencode run` that another
// agent started there, makes the TUI's session ambiguous: not measured.
// With no session since it started, the TUI is fresh.
func TestOpenCodeWithTwoSessionsSinceItStartedIsNotMeasured(t *testing.T) {
	root := copyFixture(t)
	data := filepath.Join(root, "home", ".local", "share", "opencode")
	opencodeStore(t, data, "ses_tui", "/work/web", fixtureNow.UnixMilli()-60_000, "")
	opencodeStore(t, data, "ses_run", "/work/web", fixtureNow.UnixMilli()-30_000, "")
	writeFile(t, filepath.Join(root, "home", ".cache", "opencode", "models.json"), `{"zen":{"models":{"model-made-up":{"limit":{"context":200000}}}}}`)
	addProc(t, root, 930, 1, "opencode", []string{"opencode"}, "%23")
	symlink(t, "/work/web", filepath.Join(root, "proc", "930", "cwd"))
	addPane(t, root, "%23\tweb:1.1\tw\t930\topencode\t/work/web")
	// A TUI in a directory with no session yet.
	addProc(t, root, 931, 1, "opencode", []string{"opencode"}, "%24")
	symlink(t, "/work/new", filepath.Join(root, "proc", "931", "cwd"))
	addPane(t, root, "%24\tnew:1.1\tw\t931\topencode\t/work/new")
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "opencode"}
	got := byPane(smp.Sample())
	if s := got["%23"]; s.Context != "unmeasured" || s.Tokens != 0 {
		t.Errorf("two sessions: %+v", s)
	}
	if s := got["%24"]; s.Context != "fresh" {
		t.Errorf("no session yet: context %q, want fresh", s.Context)
	}
}

// TestAnOpencodeRunUnderAnotherAgentCountsAsATwin: an `opencode run` a
// Claude session started in /work/web shares the TUI's directory.
func TestAnOpencodeRunUnderAnotherAgentCountsAsATwin(t *testing.T) {
	root := copyFixture(t)
	data := filepath.Join(root, "home", ".local", "share", "opencode")
	opencodeStore(t, data, "ses_tui", "/work/web", fixtureNow.UnixMilli()-60_000, "")
	writeFile(t, filepath.Join(root, "home", ".cache", "opencode", "models.json"), `{"zen":{"models":{"model-made-up":{"limit":{"context":200000}}}}}`)
	addProc(t, root, 930, 1, "opencode", []string{"opencode"}, "%23")
	symlink(t, "/work/web", filepath.Join(root, "proc", "930", "cwd"))
	addPane(t, root, "%23\tweb:1.1\tw\t930\topencode\t/work/web")
	addProc(t, root, 932, 200, "opencode", []string{"opencode", "run", "fix it"}, "")
	symlink(t, "/work/web", filepath.Join(root, "proc", "932", "cwd"))
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "opencode"}
	if s := byPane(smp.Sample())["%23"]; s.Context != "unmeasured" {
		t.Fatalf("context %q, want unmeasured", s.Context)
	}
}

// TestAModelNameCarriesNoControlKeys: the model comes from the agent's
// files and goes on screen.
func TestAModelNameCarriesNoControlKeys(t *testing.T) {
	root := copyFixture(t)
	rollout := filepath.Join(root, "home", ".codex", "sessions", "rollout-2026-09-21T10-00-00-esc.jsonl")
	writeFile(t, rollout, codexRollout(`gpt\u001b]0;pwned\u0007`, 1000, 258400))
	addProc(t, root, 900, 1, "codex", []string{"codex"}, "%20")
	addPane(t, root, "%20\tapi:1.1\tw\t900\tcodex\t/work/api")
	symlink(t, rollout, filepath.Join(root, "proc", "900", "fd", "21"))
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "codex"}
	if s := byPane(smp.Sample())["%20"]; s.Model == "" || strings.ContainsAny(s.Model, "\x1b\x07") {
		t.Fatalf("model %q", s.Model)
	}
}

// TestAnotherHomeWithAKeyInItsEnvironmentShowsNoAPIKey: whether such an
// agent uses its key or its login lives in its own home: no label.
func TestAnotherHomeWithAKeyInItsEnvironmentShowsNoAPIKey(t *testing.T) {
	root := copyFixture(t)
	addProc(t, root, 990, 1, "claude", []string{"claude"}, "%60")
	setEnv(t, root, 990, "TMUX_PANE=%60", "HOME=/home/other", "ANTHROPIC_API_KEY="+secretAPIKey)
	addPane(t, root, "%60\tother:1.1\tw\t990\tclaude\t/work/app")
	addProc(t, root, 991, 1, "codex", []string{"codex"}, "%61")
	setEnv(t, root, 991, "TMUX_PANE=%61", "HOME=/home/other", "OPENAI_API_KEY="+secretAPIKey)
	addPane(t, root, "%61\tother:2.1\tw\t991\tcodex\t/work/app")
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "codex"}
	got := byPane(smp.Sample())
	for _, pane := range []string{"%60", "%61"} {
		if s := got[pane]; s.Account != "" {
			t.Errorf("%s: account %q, want none", pane, s.Account)
		}
	}
}
