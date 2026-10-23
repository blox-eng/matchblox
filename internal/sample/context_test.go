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

// TestTheAgentsWindowWinsOverTheConfig: the window the agent reports is a
// measurement of the session; the config is a rule for a model.
func TestTheAgentsWindowWinsOverTheConfig(t *testing.T) {
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
	if s := byPane(smp.Sample())["%20"]; s.Window != 258400 {
		t.Fatalf("window %d, want the 258400 codex reports", s.Window)
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

// TestACodexThatHoldsNoRolloutOpenIsFoundByItsDirectory: Codex 0.163
// closes its rollout after each write (seen on a live machine, #179), so
// no open file names it. The session is then the newest rollout of the
// process's directory written since the process started.
func TestACodexThatHoldsNoRolloutOpenIsFoundByItsDirectory(t *testing.T) {
	root := copyFixture(t)
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "codex"}
	started := fixtureNow.Add(-time.Duration((smp.FS.Uptime() - 10) * float64(time.Second)))
	day := filepath.Join(root, "home", ".codex", "sessions", fixtureNow.Format("2006/01/02"))
	for _, r := range []struct {
		name, cwd string
		tokens    int
		at        time.Time
	}{
		{"rollout-a-cur.jsonl", "/work/api", 51680, fixtureNow.Add(-time.Second)},
		{"rollout-b-before.jsonl", "/work/api", 200_000, started.Add(-time.Hour)}, // an earlier session here
		{"rollout-c-other.jsonl", "/work/web", 129200, fixtureNow},                // another directory's
	} {
		path := filepath.Join(day, r.name)
		writeFile(t, path, strings.ReplaceAll(codexRollout("gpt-made-up", r.tokens, 258400), "/work/api", r.cwd))
		if err := os.Chtimes(path, r.at, r.at); err != nil {
			t.Fatal(err)
		}
	}
	addProc(t, root, 900, 1, "codex", []string{"codex"}, "%20")
	addPane(t, root, "%20\tapi:1.1\tw\t900\tcodex\t/work/api")
	symlink(t, "/dev/pts/3", filepath.Join(root, "proc", "900", "fd", "0"))
	symlink(t, "/work/api", filepath.Join(root, "proc", "900", "cwd"))
	// Nothing written in its directory since it started: fresh.
	addProc(t, root, 910, 1, "codex", []string{"codex"}, "%21")
	addPane(t, root, "%21\tops:1.1\tw\t910\tcodex\t/work/ops")
	symlink(t, "/dev/pts/4", filepath.Join(root, "proc", "910", "fd", "0"))
	symlink(t, "/work/ops", filepath.Join(root, "proc", "910", "cwd"))

	writeFile(t, filepath.Join(root, "panes", "20.txt"), "› fix the build\n")

	got := byPane(smp.Sample())
	if s := got["%20"]; s.Context != "known" || s.Tokens != 51680 || s.Window != 258400 {
		t.Errorf("codex with no rollout open: %+v", s)
	}
	// Its rollout names the session, so z can park it; with no hooks its
	// pane still tells busy from idle.
	if s := got["%20"]; s.SessionID != "01a0-made-up" || !s.FromPane {
		t.Errorf("codex session id %q, from its pane %v: want 01a0-made-up, true", s.SessionID, s.FromPane)
	}
	if s := got["%21"]; s.Context != "fresh" || s.Tokens != 0 {
		t.Errorf("codex with no rollout since it started: %+v", s)
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

// codexMeta is a rollout's first line as Codex 0.163 writes it, with the
// fields the directory fallback reads.
func codexMeta(id, cwd, originator string, at time.Time) string {
	return `{"timestamp":"` + at.UTC().Format(time.RFC3339Nano) + `","type":"session_meta","payload":{"id":"` + id + `","timestamp":"` +
		at.UTC().Format(time.RFC3339Nano) + `","cwd":"` + cwd + `","originator":"` + originator + `","cli_version":"0.163.0"}}` + "\n"
}

// TestTheDirectoryFallbackTakesOnlyARolloutThatIsSurelyThisSessions: the
// fallback names a session by its directory, so it must not guess (#187
// review). Two Codex processes in one directory, a `codex exec` run
// there, or a rollout that began before the process started belong to
// another session: no id, or z would park the wrong pane.
func TestTheDirectoryFallbackTakesOnlyARolloutThatIsSurelyThisSessions(t *testing.T) {
	root := copyFixture(t)
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "codex"}
	started := fixtureNow.Add(-time.Duration((smp.FS.Uptime() - 10) * float64(time.Second)))
	day := filepath.Join(root, "home", ".codex", "sessions", fixtureNow.Format("2006/01/02"))
	tail := strings.SplitN(codexRollout("gpt-made-up", 51680, 258400), "\n", 2)[1]
	write := func(name, meta string, at time.Time) {
		path := filepath.Join(day, name)
		writeFile(t, path, meta+tail)
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	proc := func(pid int, pane, cwd string) {
		addProc(t, root, pid, 1, "codex", []string{"codex"}, pane)
		addPane(t, root, pane+"\tw:"+strconv.Itoa(pid)+".1\tw\t"+strconv.Itoa(pid)+"\tcodex\t"+cwd)
		symlink(t, "/dev/pts/3", filepath.Join(root, "proc", strconv.Itoa(pid), "fd", "0"))
		symlink(t, cwd, filepath.Join(root, "proc", strconv.Itoa(pid), "cwd"))
	}
	after := started.Add(2 * time.Second)
	// Two in /work/two, one rollout: whose it is cannot be told.
	proc(900, "%20", "/work/two")
	proc(901, "%21", "/work/two")
	write("rollout-a-two.jsonl", codexMeta("id-two", "/work/two", "codex-tui", after), fixtureNow.Add(-time.Second))
	// /work/tui: its own rollout, and a newer `codex exec` run there.
	proc(910, "%22", "/work/tui")
	write("rollout-b-tui.jsonl", codexMeta("id-tui", "/work/tui", "codex-tui", after), fixtureNow.Add(-5*time.Second))
	write("rollout-c-exec.jsonl", codexMeta("id-exec", "/work/tui", "codex_exec", after), fixtureNow.Add(-time.Second))
	// /work/old: a session that began before this process, still written.
	proc(920, "%23", "/work/old")
	write("rollout-d-old.jsonl", codexMeta("id-old", "/work/old", "codex-tui", started.Add(-time.Hour)), fixtureNow.Add(-time.Second))

	got := byPane(smp.Sample())
	for _, pane := range []string{"%20", "%21"} {
		if s := got[pane]; s.SessionID != "" || s.Tokens != 0 || s.Context != "unmeasured" {
			t.Errorf("%s, one of two in a directory: id %q, %d tokens, context %q; want none, unmeasured", pane, s.SessionID, s.Tokens, s.Context)
		}
	}
	if s := got["%22"]; s.SessionID != "id-tui" {
		t.Errorf("the TUI next to a codex exec run: id %q, want id-tui", s.SessionID)
	}
	if s := got["%23"]; s.SessionID != "" {
		t.Errorf("a rollout begun before the process: id %q, want none", s.SessionID)
	}
}

// TestTheDirectoryFallbackCountsOnlyCodexSessions: a `codex exec` that
// Claude Code runs in the TUI's directory, or a `codex mcp-server` it keeps,
// is no second session there: the TUI keeps its rollout. Two rollouts
// begun since the TUI started (an app thread, a /new) cannot be told
// apart: no id, unmeasured. A rollout in the next day's directory (a
// Codex in a zone ahead) is found. A process whose open files cannot be
// read (macOS) is fresh once its directory shows no rollout of its own.
func TestTheDirectoryFallbackCountsOnlyCodexSessions(t *testing.T) {
	root := copyFixture(t)
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "codex"}
	started := fixtureNow.Add(-time.Duration((smp.FS.Uptime() - 10) * float64(time.Second)))
	sessions := filepath.Join(root, "home", ".codex", "sessions")
	tail := strings.SplitN(codexRollout("gpt-made-up", 51680, 258400), "\n", 2)[1]
	write := func(day time.Time, name, meta string, at time.Time) {
		path := filepath.Join(sessions, day.Format("2006/01/02"), name)
		writeFile(t, path, meta+tail)
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	proc := func(pid, ppid int, comm string, argv []string, pane, cwd string, fds bool) {
		addProc(t, root, pid, ppid, comm, argv, pane)
		if pane != "" {
			addPane(t, root, pane+"\tw:"+strconv.Itoa(pid)+".1\tw\t"+strconv.Itoa(pid)+"\t"+comm+"\t"+cwd)
		}
		if fds {
			symlink(t, "/dev/pts/3", filepath.Join(root, "proc", strconv.Itoa(pid), "fd", "0"))
		}
		symlink(t, cwd, filepath.Join(root, "proc", strconv.Itoa(pid), "cwd"))
	}
	after := started.Add(2 * time.Second)
	// /work/tui: the TUI, a claude running `codex exec`, a `codex mcp-server`.
	proc(900, 1, "codex", []string{"codex"}, "%20", "/work/tui", true)
	proc(950, 1, "claude", []string{"claude"}, "%29", "/work/tui", true)
	proc(951, 950, "codex", []string{"codex", "exec", "review"}, "", "/work/tui", false)
	proc(960, 1, "codex", []string{"codex", "mcp-server"}, "", "/work/tui", false)
	write(fixtureNow, "rollout-a-tui.jsonl", codexMeta("id-tui", "/work/tui", "codex-tui", after), fixtureNow.Add(-time.Second))
	// /work/two: one TUI, two rollouts begun since it started.
	proc(910, 1, "codex", []string{"codex"}, "%21", "/work/two", true)
	write(fixtureNow, "rollout-b-one.jsonl", codexMeta("id-one", "/work/two", "codex-tui", after), fixtureNow.Add(-5*time.Second))
	write(fixtureNow, "rollout-c-two.jsonl", codexMeta("id-two", "/work/two", "codex-tui", after.Add(time.Minute)), fixtureNow.Add(-time.Second))
	// /work/ahead: its rollout under tomorrow's date.
	proc(920, 1, "codex", []string{"codex"}, "%22", "/work/ahead", true)
	write(fixtureNow.AddDate(0, 0, 1), "rollout-d-ahead.jsonl", codexMeta("id-ahead", "/work/ahead", "codex-tui", after), fixtureNow.Add(-time.Second))
	// /work/mac: no open files to read, no rollout yet.
	proc(930, 1, "codex", []string{"codex"}, "%23", "/work/mac", false)

	got := byPane(smp.Sample())
	if s := got["%20"]; s.SessionID != "id-tui" || s.Context != "known" {
		t.Errorf("the TUI beside a codex exec and an mcp-server: id %q, context %q; want id-tui, known", s.SessionID, s.Context)
	}
	if s := got["%21"]; s.SessionID != "" || s.Context != "unmeasured" {
		t.Errorf("two rollouts begun since the start: id %q, context %q; want none, unmeasured", s.SessionID, s.Context)
	}
	if s := got["%22"]; s.SessionID != "id-ahead" {
		t.Errorf("a rollout in the next day's directory: id %q, want id-ahead", s.SessionID)
	}
	if s := got["%23"]; s.Context != "fresh" {
		t.Errorf("no open files, no rollout since the start: context %q, want fresh", s.Context)
	}
}

// codexDir is a fixture for the directory fallback: Codex processes in
// panes and rollouts under CODEX_HOME. A process has a tty, so its open
// files read as none (Codex 0.163 closes its rollout).
type codexDir struct {
	t       *testing.T
	root    string
	smp     *Sampler
	started time.Time
}

func newCodexDir(t *testing.T) *codexDir {
	t.Helper()
	root := copyFixture(t)
	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "codex"}
	return &codexDir{t, root, smp, fixtureNow.Add(-time.Duration((smp.FS.Uptime() - 10) * float64(time.Second)))}
}

// proc starts argv in pane; an npm install runs as node, named MainThread.
func (d *codexDir) proc(pid int, argv []string, pane, cwd string) {
	comm := "codex"
	if argv[0] == "node" {
		comm = "MainThread"
	}
	addProc(d.t, d.root, pid, 1, comm, argv, pane)
	addPane(d.t, d.root, pane+"\tw:"+strconv.Itoa(pid)+".1\tw\t"+strconv.Itoa(pid)+"\t"+comm+"\t"+cwd)
	symlink(d.t, "/dev/pts/3", filepath.Join(d.root, "proc", strconv.Itoa(pid), "fd", "0"))
	symlink(d.t, cwd, filepath.Join(d.root, "proc", strconv.Itoa(pid), "cwd"))
}

// write puts a rollout of tokens in day's directory, last written at.
func (d *codexDir) write(day time.Time, name, meta string, tokens int, at time.Time) string {
	tail := strings.SplitN(codexRollout("gpt-made-up", tokens, 258400), "\n", 2)[1]
	path := filepath.Join(d.root, "home", ".codex", "sessions", day.Format("2006/01/02"), name)
	writeFile(d.t, path, meta+tail)
	if err := os.Chtimes(path, at, at); err != nil {
		d.t.Fatal(err)
	}
	return path
}

// TestACodexExecTakesNoSessionsRollout: a `codex exec` run in a pane is
// no session of the directory, so it must not take the rollout of the TUI
// that started there after it: z on the exec row would park the TUI. An
// npm install runs it as `node <script> exec`.
func TestACodexExecTakesNoSessionsRollout(t *testing.T) {
	d := newCodexDir(t)
	d.proc(900, []string{"codex", "exec", "fix the tests"}, "%20", "/work/both")
	d.proc(901, []string{"codex"}, "%21", "/work/both")
	d.write(fixtureNow, "rollout-a-tui.jsonl", codexMeta("id-tui", "/work/both", "codex-tui", d.started.Add(2*time.Second)), 51680, fixtureNow.Add(-time.Second))
	d.proc(910, []string{"node", "/usr/local/bin/codex", "exec", "fix it"}, "%22", "/work/npm")
	// An option before exec, and codex review: the TUI beside them keeps
	// its rollout.
	d.proc(920, []string{"codex", "-m", "gpt-made-up", "exec", "fix it"}, "%23", "/work/opt")
	d.proc(922, []string{"codex", "--remote-auth-token-env", "TOKEN", "exec", "fix it"}, "%25", "/work/opt")
	d.proc(923, []string{"codex", "review"}, "%26", "/work/opt")
	d.proc(921, []string{"codex"}, "%24", "/work/opt")
	d.write(fixtureNow, "rollout-b-opt.jsonl", codexMeta("id-opt", "/work/opt", "codex-tui", d.started.Add(2*time.Second)), 51680, fixtureNow.Add(-time.Second))

	got := byPane(d.smp.Sample())
	if s := got["%24"]; s.SessionID != "id-opt" || s.Context != "known" {
		t.Errorf("the TUI beside codex -m X exec: id %q, context %q; want id-opt, known", s.SessionID, s.Context)
	}
	for _, pane := range []string{"%20", "%22", "%23", "%25", "%26"} {
		if s := got[pane]; s.SessionID != "" || s.Context != "unmeasured" {
			t.Errorf("%s, a codex exec row: id %q, context %q; want none, unmeasured", pane, s.SessionID, s.Context)
		}
	}
	if s := got["%21"]; s.SessionID != "id-tui" || s.Context != "known" {
		t.Errorf("the TUI: id %q, context %q; want id-tui, known", s.SessionID, s.Context)
	}
}

// TestAFreshCodexDoesNotTakeAnAppThread: before its first turn a TUI has
// no rollout. A thread of the Codex app or an editor begun in its
// directory is another's (its originator is not the CLI's): the TUI stays
// fresh, with no id. The CLI writes codex-tui (codex_cli_rs before it).
func TestAFreshCodexDoesNotTakeAnAppThread(t *testing.T) {
	d := newCodexDir(t)
	after := d.started.Add(2 * time.Second)
	d.proc(900, []string{"codex"}, "%20", "/work/app")
	d.write(fixtureNow, "rollout-a-app.jsonl", codexMeta("id-app", "/work/app", "Codex Desktop", after), 51680, fixtureNow.Add(-time.Second))
	d.proc(910, []string{"codex"}, "%21", "/work/cli")
	d.write(fixtureNow, "rollout-b-cli.jsonl", codexMeta("id-cli", "/work/cli", "codex_cli_rs", after), 51680, fixtureNow.Add(-time.Second))

	got := byPane(d.smp.Sample())
	if s := got["%20"]; s.SessionID != "" || s.Context != "fresh" {
		t.Errorf("a TUI beside an app thread: id %q, context %q; want none, fresh", s.SessionID, s.Context)
	}
	if s := got["%21"]; s.SessionID != "id-cli" || s.Context != "known" {
		t.Errorf("an older CLI's rollout: id %q, context %q; want id-cli, known", s.SessionID, s.Context)
	}
}

// TestAResumedCodexIsNotFresh: `codex resume <id>` writes on in the
// rollout it began on another day: it is found by its id, also from an
// npm install, after global options, or with the id in capitals. Any
// other session whose directory has a rollout begun before it started and
// written since (a resume without an id, a session that ended after it
// started) cannot be told apart: unmeasured, not fresh. So is a resume
// whose rollout is not found: it began on a day the scan does not read.
func TestAResumedCodexIsNotFresh(t *testing.T) {
	d := newCodexDir(t)
	before := d.started.Add(-72 * time.Hour)
	for i, argv := range [][]string{
		{"codex", "resume", "ID"},
		{"node", "/usr/local/bin/codex", "resume", "ID"},
		{"codex", "-m", "gpt-made-up", "resume", "ID"},
		{"codex", "resume", "UPPER"},
		{"codex", "resume", "-m", "gpt-made-up", "ID"},
		{"codex", "--remote", "ws://example.com:1", "resume", "ID"},
	} {
		id := "0199a0b1-5e5e-7000-8000-00000000abc" + strconv.Itoa(i)
		for j := range argv {
			switch argv[j] {
			case "ID":
				argv[j] = id
			case "UPPER":
				argv[j] = strings.ToUpper(id)
			}
		}
		cwd := "/work/resumed" + strconv.Itoa(i)
		d.proc(900+i, argv, "%2"+strconv.Itoa(i), cwd)
		d.write(before, "rollout-2026-09-18T10-00-00-"+id+".jsonl", codexMeta(id, cwd, "codex-tui", before), 51680, fixtureNow.Add(-time.Second))
	}
	d.proc(910, []string{"codex", "resume", "--last"}, "%30", "/work/last")
	d.write(fixtureNow, "rollout-b-last.jsonl", codexMeta("id-last", "/work/last", "codex-tui", d.started.Add(-time.Hour)), 51680, fixtureNow.Add(-time.Second))
	d.proc(920, []string{"node", "/usr/local/bin/codex", "resume", "--last"}, "%31", "/work/away")
	d.write(before, "rollout-c-away.jsonl", codexMeta("id-away", "/work/away", "codex-tui", before), 51680, fixtureNow.Add(-time.Second))

	got := byPane(d.smp.Sample())
	for i := range 6 {
		id, pane := "0199a0b1-5e5e-7000-8000-00000000abc"+strconv.Itoa(i), "%2"+strconv.Itoa(i)
		if s := got[pane]; s.SessionID != id || s.Context != "known" || s.Tokens != 51680 {
			t.Errorf("%s, resumed by its id: id %q, context %q, %d tokens; want %s, known, 51680", pane, s.SessionID, s.Context, s.Tokens, id)
		}
	}
	if s := got["%30"]; s.SessionID != "" || s.Context != "unmeasured" {
		t.Errorf("codex resume --last: id %q, context %q; want none, unmeasured", s.SessionID, s.Context)
	}
	if s := got["%31"]; s.SessionID != "" || s.Context != "unmeasured" {
		t.Errorf("codex resume --last of an older day's rollout: id %q, context %q; want none, unmeasured", s.SessionID, s.Context)
	}
}

// TestACodexFollowsItsRevertedRollout: editing an earlier prompt (Esc Esc)
// reverts the thread, and Codex writes on in
// rollout-<ts>-<id>_<rollout>.jsonl, with the same session id. A resumed
// session reads the newer file, and a TUI with both files keeps its id.
func TestACodexFollowsItsRevertedRollout(t *testing.T) {
	d := newCodexDir(t)
	before := d.started.Add(-72 * time.Hour)
	resumed := "0199a0b1-5e5e-7000-8000-00000000abcd"
	d.proc(900, []string{"codex", "resume", resumed}, "%20", "/work/resumed")
	d.write(before, "rollout-2026-09-18T10-00-00-"+resumed+".jsonl", codexMeta(resumed, "/work/resumed", "codex-tui", before), 200000, d.started.Add(-time.Hour))
	d.write(fixtureNow, "rollout-2026-09-21T10-00-00-"+resumed+"_0199a0b1-5e5e-7000-8000-0000000fffff.jsonl", codexMeta(resumed, "/work/resumed", "codex-tui", before), 30000, fixtureNow.Add(-time.Second))
	fresh := "0199a0b1-5e5e-7000-8000-000000001111"
	after := d.started.Add(2 * time.Second)
	d.proc(910, []string{"codex"}, "%21", "/work/tui")
	d.write(fixtureNow, "rollout-2026-09-21T10-00-00-"+fresh+".jsonl", codexMeta(fresh, "/work/tui", "codex-tui", after), 90000, fixtureNow.Add(-time.Minute))
	d.write(fixtureNow, "rollout-2026-09-21T10-05-00-"+fresh+"_0199a0b1-5e5e-7000-8000-0000000eeeee.jsonl", codexMeta(fresh, "/work/tui", "codex-tui", after), 30000, fixtureNow.Add(-time.Second))

	got := byPane(d.smp.Sample())
	if s := got["%20"]; s.SessionID != resumed || s.Tokens != 30000 {
		t.Errorf("a resumed session after a revert: id %q, %d tokens; want %s, 30000", s.SessionID, s.Tokens, resumed)
	}
	if s := got["%21"]; s.SessionID != fresh || s.Tokens != 30000 {
		t.Errorf("a TUI after a revert: id %q, %d tokens; want %s, 30000", s.SessionID, s.Tokens, fresh)
	}
}

// TestAResumeFollowsARevertFromAnEarlierDay: Codex resumes into the
// newest file of the thread, a revert's on whatever day it was made.
func TestAResumeFollowsARevertFromAnEarlierDay(t *testing.T) {
	d := newCodexDir(t)
	id := "0199a0b1-5e5e-7000-8000-00000000abcd"
	orig, reverted := d.started.Add(-5*24*time.Hour), d.started.Add(-3*24*time.Hour)
	d.proc(900, []string{"codex", "resume", id}, "%20", "/work/rev")
	d.write(orig, "rollout-2026-09-16T10-00-00-"+id+".jsonl", codexMeta(id, "/work/rev", "codex-tui", orig), 200000, orig)
	d.write(reverted, "rollout-2026-09-18T10-00-00-"+id+"_0199a0b1-5e5e-7000-8000-0000000fffff.jsonl", codexMeta(id, "/work/rev", "codex-tui", reverted), 30000, fixtureNow.Add(-time.Second))
	if s := byPane(d.smp.Sample())["%20"]; s.SessionID != id || s.Tokens != 30000 {
		t.Errorf("a resume of a thread reverted days ago: id %q, %d tokens; want %s, 30000", s.SessionID, s.Tokens, id)
	}
}

// codexSubagentMeta is the first line of a subagent's rollout: the
// parent's directory and originator, with the parent named.
func codexSubagentMeta(id, parent, cwd string, at time.Time) string {
	return `{"timestamp":"` + at.UTC().Format(time.RFC3339Nano) + `","type":"session_meta","payload":{"id":"` + id + `","timestamp":"` +
		at.UTC().Format(time.RFC3339Nano) + `","cwd":"` + cwd + `","originator":"codex-tui","source":{"subagent":{"thread_spawn":{"parent_thread_id":"` +
		parent + `","depth":1}}},"cli_version":"0.164.0"}}` + "\n"
}

// TestAResumedCodexKeepsItsOwnRollout: a pane begun as `codex resume A`
// is A's while A is written last. Another session's rollout in its
// directory (one that ran and exited there, A's subagent) never gives it
// another id. Once another session begun since the start is written after
// A (a /new in A's TUI, or a session beside it), which is the pane's cannot
// be told: not measured, no id.
func TestAResumedCodexKeepsItsOwnRollout(t *testing.T) {
	d := newCodexDir(t)
	before := d.started.Add(-72 * time.Hour)
	resumed := func(n int, cwd string, lastWrite time.Time) string {
		id := "0199a0b1-5e5e-7000-8000-00000000000" + strconv.Itoa(n)
		d.proc(900+n, []string{"codex", "resume", id}, "%2"+strconv.Itoa(n), cwd)
		d.write(before, "rollout-2026-09-18T10-00-00-"+id+".jsonl", codexMeta(id, cwd, "codex-tui", before), 200000, lastWrite)
		return id
	}
	// %20: a session ran and exited there while A kept working.
	a0 := resumed(0, "/work/exited", fixtureNow.Add(-time.Second))
	d.write(fixtureNow, "rollout-b-exited.jsonl", codexMeta("id-exited", "/work/exited", "codex-tui", d.started.Add(2*time.Minute)), 12000, d.started.Add(5*time.Minute))
	// %21: A's subagent, written after A.
	a1 := resumed(1, "/work/sub", d.started.Add(time.Minute))
	d.write(fixtureNow, "rollout-c-sub.jsonl", codexSubagentMeta("id-sub", a1, "/work/sub", d.started.Add(2*time.Minute)), 12000, fixtureNow.Add(-time.Second))
	// %22: a /new in A's TUI.
	resumed(2, "/work/new", d.started.Add(time.Minute))
	d.write(fixtureNow, "rollout-d-new.jsonl", codexMeta("id-new", "/work/new", "codex-tui", d.started.Add(2*time.Minute)), 12000, fixtureNow.Add(-time.Second))

	got := byPane(d.smp.Sample())
	if s := got["%20"]; s.SessionID != a0 || s.Tokens != 200000 {
		t.Errorf("beside an exited session: id %q, %d tokens; want %s, 200000", s.SessionID, s.Tokens, a0)
	}
	if s := got["%21"]; s.SessionID != a1 || s.Tokens != 200000 {
		t.Errorf("beside its subagent: id %q, %d tokens; want %s, 200000", s.SessionID, s.Tokens, a1)
	}
	if s := got["%22"]; s.SessionID != "" || s.Context != "unmeasured" {
		t.Errorf("after a /new in A's TUI: id %q, context %q; want none, unmeasured", s.SessionID, s.Context)
	}
}

// TestAResumeInAnotherDirectoryLosesItsIDAfterANew: Codex works in the
// resumed session's directory, or the one -C names, not the process's
// own. A /new written there after A clears the id.
func TestAResumeInAnotherDirectoryLosesItsIDAfterANew(t *testing.T) {
	d := newCodexDir(t)
	before := d.started.Add(-72 * time.Hour)
	for i, argv := range [][]string{
		{"codex", "resume", "ID"},
		{"codex", "-C", "../proj1", "resume", "ID"},
	} {
		id := "0199a0b1-5e5e-7000-8000-00000000000" + strconv.Itoa(i)
		proj := "/work/proj" + strconv.Itoa(i)
		for j := range argv {
			if argv[j] == "ID" {
				argv[j] = id
			}
		}
		d.proc(900+i, argv, "%2"+strconv.Itoa(i), "/work/home"+strconv.Itoa(i))
		d.write(before, "rollout-2026-09-18T10-00-00-"+id+".jsonl", codexMeta(id, proj, "codex-tui", before), 200000, d.started.Add(time.Minute))
		d.write(fixtureNow, "rollout-new-"+strconv.Itoa(i)+".jsonl", codexMeta("id-new"+strconv.Itoa(i), proj, "codex-tui", d.started.Add(2*time.Minute)), 12000, fixtureNow.Add(-time.Second))
	}
	got := byPane(d.smp.Sample())
	for _, pane := range []string{"%20", "%21"} {
		if s := got[pane]; s.SessionID != "" || s.Context != "unmeasured" {
			t.Errorf("%s, a /new in Codex's own directory: id %q, context %q; want none, unmeasured", pane, s.SessionID, s.Context)
		}
	}
}

// TestAnOpenSubagentRolloutIsNotTheSessions: a subagent runs in the TUI's
// process, so its rollout is among the open files; the session's is the
// main one.
func TestAnOpenSubagentRolloutIsNotTheSessions(t *testing.T) {
	d := newCodexDir(t)
	d.proc(900, []string{"codex"}, "%20", "/work/sub")
	main := d.write(fixtureNow, "rollout-a-main.jsonl", codexMeta("id-main", "/work/sub", "codex-tui", d.started.Add(2*time.Second)), 150000, fixtureNow.Add(-time.Minute))
	sub := d.write(fixtureNow, "rollout-b-sub.jsonl", codexSubagentMeta("id-sub", "id-main", "/work/sub", d.started.Add(time.Minute)), 12000, fixtureNow.Add(-time.Second))
	symlink(t, main, filepath.Join(d.root, "proc", "900", "fd", "21"))
	symlink(t, sub, filepath.Join(d.root, "proc", "900", "fd", "22"))
	if s := byPane(d.smp.Sample())["%20"]; s.SessionID != "id-main" || s.Tokens != 150000 {
		t.Errorf("a subagent's rollout open: id %q, %d tokens; want id-main, 150000", s.SessionID, s.Tokens)
	}
}

// TestAFreshCodexDoesNotTakeAnExitedNeighbour: a Codex that started after
// the TUI in its directory, worked and exited, left a rollout begun since
// the TUI started. It is not the TUI's.
func TestAFreshCodexDoesNotTakeAnExitedNeighbour(t *testing.T) {
	d := newCodexDir(t)
	d.proc(900, []string{"codex"}, "%20", "/work/both")
	d.proc(901, []string{"codex"}, "%21", "/work/both")
	d.smp.Sample()
	if err := os.RemoveAll(filepath.Join(d.root, "proc", "901")); err != nil {
		t.Fatal(err)
	}
	d.write(fixtureNow, "rollout-a-exited.jsonl", codexMeta("id-exited", "/work/both", "codex-tui", d.started.Add(time.Minute)), 90000, fixtureNow.Add(-time.Second))
	if s := byPane(d.smp.Sample())["%20"]; s.SessionID != "" || s.Context == "known" {
		t.Errorf("beside an exited neighbour's rollout: id %q, context %q; want none, not known", s.SessionID, s.Context)
	}
}

// TestAnExitedNeighbourWithoutARolloutKeepsTheTUIMeasured: a Codex that
// started after the TUI and exited (`codex login`, or a TUI quit before
// its first turn) owns no rollout begun before it started: the TUI keeps
// its own.
func TestAnExitedNeighbourWithoutARolloutKeepsTheTUIMeasured(t *testing.T) {
	for _, argv := range [][]string{{"codex", "login"}, {"codex"}} {
		d := newCodexDir(t)
		// The fixture starts both at one tick: A's rollout began before
		// the neighbour did, within the tick.
		d.proc(900, []string{"codex"}, "%20", "/work/r")
		d.write(fixtureNow, "rollout-a.jsonl", codexMeta("id-a", "/work/r", "codex-tui", d.started.Add(-500*time.Millisecond)), 150000, fixtureNow.Add(-time.Second))
		d.proc(901, argv, "%21", "/work/r")
		d.smp.Sample()
		if err := os.RemoveAll(filepath.Join(d.root, "proc", "901")); err != nil {
			t.Fatal(err)
		}
		if s := byPane(d.smp.Sample())["%20"]; s.SessionID != "id-a" || s.Tokens != 150000 {
			t.Errorf("after %q exited: id %q, %d tokens; want id-a, 150000", argv, s.SessionID, s.Tokens)
		}
	}
}

// TestACodexStartedWithCdCountsInItsDirectory: `codex -C /work/r` works
// in /work/r whatever its shell's directory: it counts there.
func TestACodexStartedWithCdCountsInItsDirectory(t *testing.T) {
	d := newCodexDir(t)
	d.proc(900, []string{"codex"}, "%20", "/work/r")
	d.proc(901, []string{"codex", "-C", "/work/r"}, "%21", "/work/home")
	d.write(fixtureNow, "rollout-b.jsonl", codexMeta("id-b", "/work/r", "codex-tui", d.started.Add(2*time.Second)), 120000, fixtureNow.Add(-time.Second))
	d.proc(910, []string{"codex", "--cd", "../alone"}, "%22", "/work/home")
	d.write(fixtureNow, "rollout-c.jsonl", codexMeta("id-c", "/work/alone", "codex-tui", d.started.Add(2*time.Second)), 200000, fixtureNow.Add(-time.Second))

	got := byPane(d.smp.Sample())
	for _, pane := range []string{"%20", "%21"} {
		if s := got[pane]; s.SessionID != "" || s.Context == "known" || s.Context == "fresh" {
			t.Errorf("%s, two Codex in /work/r: id %q, context %q; want none, unmeasured", pane, s.SessionID, s.Context)
		}
	}
	if s := got["%22"]; s.SessionID != "id-c" || s.Tokens != 200000 {
		t.Errorf("codex --cd alone: id %q, %d tokens; want id-c, 200000", s.SessionID, s.Tokens)
	}
}

// TestAFreshCodexDoesNotTakeASubagent: a subagent of another session
// begun in the directory is not the TUI's own rollout.
func TestAFreshCodexDoesNotTakeASubagent(t *testing.T) {
	d := newCodexDir(t)
	d.proc(900, []string{"codex"}, "%20", "/work/sub")
	d.write(fixtureNow, "rollout-a-sub.jsonl", codexSubagentMeta("id-sub", "id-other", "/work/sub", d.started.Add(2*time.Second)), 51680, fixtureNow.Add(-time.Second))
	if s := byPane(d.smp.Sample())["%20"]; s.SessionID != "" || s.Context != "fresh" {
		t.Errorf("a TUI beside a subagent's rollout: id %q, context %q; want none, fresh", s.SessionID, s.Context)
	}
}

// TestAResumeFindsARolloutThatAppearsLater: Codex can write the rollout of
// a resume after the process starts (it unpacks a compressed one). A miss
// is tried again after 30 seconds.
func TestAResumeFindsARolloutThatAppearsLater(t *testing.T) {
	d := newCodexDir(t)
	before := d.started.Add(-72 * time.Hour)
	id := "0199a0b1-5e5e-7000-8000-00000000abcd"
	d.proc(900, []string{"codex", "resume", id}, "%20", "/work/late")
	if s := byPane(d.smp.Sample())["%20"]; s.SessionID != "" {
		t.Fatalf("no rollout yet: id %q", s.SessionID)
	}
	d.write(before, "rollout-2026-09-18T10-00-00-"+id+".jsonl", codexMeta(id, "/work/late", "codex-tui", before), 51680, fixtureNow.Add(-time.Second))
	d.smp.Now = func() time.Time { return fixtureNow.Add(31 * time.Second) }
	if s := byPane(d.smp.Sample())["%20"]; s.SessionID != id || s.Context != "known" {
		t.Errorf("the rollout appeared: id %q, context %q; want %s, known", s.SessionID, s.Context, id)
	}
}
