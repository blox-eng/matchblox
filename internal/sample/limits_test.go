package sample

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/limits"
)

// codexLimits is a token_count as a ChatGPT login writes it: the usage and
// the plan's windows.
func codexLimits(at time.Time, windows string) string {
	return `{"timestamp":"` + at.UTC().Format(time.RFC3339Nano) + `","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"total_tokens":51680},"model_context_window":258400},"rate_limits":{"limit_id":"codex",` + windows + `,"plan_type":"plus"}}}` + "\n"
}

func byAccount(snap Snapshot) map[string]limits.Account {
	out := map[string]limits.Account{}
	for _, a := range snap.Limits {
		out[a.Provider+" "+a.Account] = a
	}
	return out
}

// TestEachAccountShowsItsLimits: one line per account, from the source of
// its agent: Claude Code's status-line tap for its config directory, the
// newest rate_limits of a Codex rollout. Two sessions on one account are
// one line; a session without a source says how to get one; an API key and
// a cloud have no plan limits and no line.
func TestEachAccountShowsItsLimits(t *testing.T) {
	root := copyFixture(t)
	home := filepath.Join(root, "home")
	stateDir := filepath.Join(root, "state")
	writeFile(t, filepath.Join(home, ".claude.json"), `{"oauthAccount":{"emailAddress":"builder@example.com","organizationType":"claude_max"}}`)
	tap := `{"rate_limits":{"five_hour":{"used_percentage":28,"resets_at":` + itoa(fixtureNow.Add(2*time.Hour).Unix()) +
		`},"seven_day":{"used_percentage":40,"resets_at":` + itoa(fixtureNow.Add(4*24*time.Hour).Unix()) + `}}}`
	// The tap runs inside the agent: it names the home as the process does.
	if err := limits.Tap([]byte(tap), "/home/user/.claude", stateDir, fixtureNow.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	// A second Claude Code account with no tap.
	work := filepath.Join(root, "work-claude")
	writeFile(t, filepath.Join(work, ".claude.json"), `{"oauthAccount":{"emailAddress":"work@example.net","organizationType":"claude_team"}}`)
	addProc(t, root, 950, 1, "claude", []string{"claude"}, "%40")
	setEnv(t, root, 950, "TMUX_PANE=%40", "CLAUDE_CONFIG_DIR="+work)
	addPane(t, root, "%40\twork:1.1\tw\t950\tclaude\t/work/app")
	// Claude Code through a cloud: no plan, no line.
	addProc(t, root, 970, 1, "claude", []string{"claude"}, "%42")
	setEnv(t, root, 970, "TMUX_PANE=%42", "CLAUDE_CODE_USE_BEDROCK=1")
	addPane(t, root, "%42\tcloud:1.1\tw\t970\tclaude\t/work/app")
	// Codex on Plus: an older reading, then the newest.
	rollout := filepath.Join(home, ".codex", "sessions", "2026", "09", "21", "rollout-2026-09-21T10-00-00-01a0-made-up.jsonl")
	resets := itoa(fixtureNow.Add(3 * time.Hour).Unix())
	writeFile(t, rollout, codexRollout("gpt-made-up", 51680, 258400)+
		codexLimits(fixtureNow.Add(-20*time.Minute), `"primary":{"used_percent":10,"window_minutes":300,"resets_at":`+resets+`},"secondary":null`)+
		codexLimits(fixtureNow.Add(-5*time.Minute), `"primary":{"used_percent":61,"window_minutes":300,"resets_at":`+resets+`},"secondary":{"used_percent":12,"window_minutes":10080,"resets_at":`+itoa(fixtureNow.Add(6*24*time.Hour).Unix())+`}`)+
		// Codex also writes the limits of other ids, all null: they do not
		// hide the plan's.
		`{"timestamp":"2026-09-21T10:00:10Z","type":"event_msg","payload":{"type":"token_count","info":null,"rate_limits":{"limit_id":"premium","primary":null,"secondary":null}}}`+"\n"+
		`{"timestamp":"2026-09-21T10:00:11Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[]}}`+"\n")
	writeFile(t, filepath.Join(home, ".codex", "auth.json"), codexAuth(root))
	addProc(t, root, 900, 1, "codex", []string{"codex"}, "%20")
	addPane(t, root, "%20\tapi:1.1\tw\t900\tcodex\t/work/api")
	symlink(t, rollout, filepath.Join(root, "proc", "900", "fd", "21"))

	smp := newFixtureSampler(root)
	smp.Agents = []string{"claude", "codex"}
	smp.LimitsDir = stateDir
	snap := smp.Sample()
	got := byAccount(snap)

	max := got["anthropic builder@example.com · Max"]
	if max.State != limits.Measured || max.Source != limits.SourceClaude || *max.Matches != 4 || *max.Sparks != 6 {
		t.Fatalf("the tapped account: %+v", max)
	}
	team := got["anthropic work@example.net · Team"]
	if team.State != limits.Unmeasured || !strings.Contains(team.Fix, "Show your limits") {
		t.Fatalf("the account without a tap: %+v", team)
	}
	plus := got["openai builder@example.org · Plus"]
	if plus.State != limits.Measured || *plus.Matches != 2 || *plus.Sparks != 9 || !plus.At.Equal(fixtureNow.Add(-5*time.Minute)) {
		t.Fatalf("codex: %+v", plus)
	}
	for k := range got {
		if strings.HasPrefix(k, "bedrock") {
			t.Fatalf("a cloud has a limits line: %v", k)
		}
	}
	n := 0
	for _, a := range snap.Limits {
		if a.Account == "builder@example.com · Max" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d lines for the account every fixture session runs under", n)
	}
	if snap.Limits[0].Provider != "anthropic" || snap.Limits[len(snap.Limits)-1].Provider != "openai" {
		t.Fatalf("lines are not in a stable order: %+v", snap.Limits)
	}

	// A Go plan has a month: it shows its percent, not matches.
	os.WriteFile(rollout, []byte(codexRollout("gpt-made-up", 51680, 258400)+
		codexLimits(fixtureNow.Add(-time.Minute), `"primary":{"used_percent":47,"window_minutes":43200,"resets_at":`+itoa(fixtureNow.Add(9*24*time.Hour).Unix())+`},"secondary":null`)), 0o644)
	if go_ := byAccount(smp.Sample())["openai builder@example.org · Plus"]; go_.Matches != nil || len(go_.Other) != 1 {
		t.Fatalf("a month: %+v", go_)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
