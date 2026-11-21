package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
)

// TestAConfigListingOnlyClaudeStillSamplesCodex: the first run froze the
// agents it found; Codex installed after it is still a session (#179).
func TestAConfigListingOnlyClaudeStillSamplesCodex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("agents = [\"claude\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := newSampler(cfg, "").Agents; !slices.Contains(got, "codex") {
		t.Fatalf("the sampler's agents %q: no codex", got)
	}
	if got := newSampler(cfg, t.TempDir()).Agents; !slices.Equal(got, []string{"claude"}) {
		t.Fatalf("a fixture tree's agents %q, want its config's", got)
	}
}

func agentsOut(t *testing.T, doc state.Doc, asJSON bool) string {
	t.Helper()
	var buf bytes.Buffer
	if err := writeAgents(&buf, doc, asJSON); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestAgentsNamesNoHomeHostOrUser: the anti-goal. A session's name, target
// and folder may hold the home path, the host name and the user name;
// `matchblox agents`, as a table or as JSON, prints none of them.
func TestAgentsNamesNoHomeHostOrUser(t *testing.T) {
	home, _ := os.UserHomeDir()
	host, _ := os.Hostname()
	var name string
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	var needles []string
	for _, n := range []string{home, host, name} {
		if len(n) >= 3 {
			needles = append(needles, n)
		}
	}
	if len(needles) == 0 {
		t.Skip("no home, host or user name to look for")
	}
	var doc state.Doc
	doc.Agents = []string{"claude", "codex"}
	doc.Sessions = []sample.Session{
		{Agent: "claude", Version: "2.3.1", Name: home + "/" + name, Target: host + ":1.1", Cwd: home + "/src", UnknownScreen: true},
		{Agent: "claude", VersionWhy: "the install path names no version", Name: name, Target: host, Cwd: home},
		{Agent: "codex", Version: "0.9.0", Name: host, Target: name, Cwd: filepath.Join(home, name)},
	}
	for _, asJSON := range []bool{false, true} {
		out := agentsOut(t, doc, asJSON)
		for _, n := range needles {
			if strings.Contains(out, n) {
				t.Errorf("json=%v: the output holds %q:\n%s", asJSON, n, out)
			}
		}
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(agentsOut(t, doc, true)), &rows); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		for k := range r {
			if !slices.Contains([]string{"agent", "version", "why", "sessions", "unknown"}, k) {
				t.Errorf("--json carries %q: %v", k, r)
			}
		}
	}
}

// fields is each line of out, split on runs of two spaces or more.
func fields(out string) [][]string {
	var rows [][]string
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		var cols []string
		for _, c := range strings.Split(line, "  ") {
			if c = strings.TrimSpace(c); c != "" {
				cols = append(cols, c)
			}
		}
		rows = append(rows, cols)
	}
	return rows
}

func hasRow(out string, want ...string) bool {
	return slices.ContainsFunc(fields(out), func(r []string) bool { return slices.Equal(r, want) })
}

func TestAgentsListsEachAgent(t *testing.T) {
	var doc state.Doc
	doc.Agents = []string{"claude", "cursor-agent", "opencode"}
	doc.Sessions = []sample.Session{
		{Agent: "claude", Version: "2.3.1", UnknownScreen: true},
		{Agent: "claude", Version: "2.3.1"},
		{Agent: "cursor-agent", Version: "2025.11"},
	}
	out := agentsOut(t, doc, false)
	for _, want := range [][]string{
		{"AGENT", "VERSION", "SESSIONS", "UNKNOWN SCREEN"},
		{"Claude Code", "2.3.1", "2", "1"},
		{"Cursor", "2025.11", "1", "0"},
		{"OpenCode", "-", "0", "no hooks: screen only"},
	} {
		if !hasRow(out, want...) {
			t.Errorf("no row %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "\n"+agentsFix+"\n") {
		t.Errorf("no fix line under the table:\n%s", out)
	}
	doc.Sessions[0].UnknownScreen = false
	if out := agentsOut(t, doc, false); strings.Contains(out, agentsFix) {
		t.Errorf("a fix line with no unknown screen:\n%s", out)
	}
}

func TestAgentsSaysNotRead(t *testing.T) {
	var doc state.Doc
	why := "the install path names no version"
	doc.Sessions = []sample.Session{
		{Agent: "claude", VersionWhy: why},
		{Agent: "claude", VersionWhy: why},
	}
	out := agentsOut(t, doc, false)
	if !hasRow(out, "Claude Code", "not read", "2", "0") {
		t.Errorf("no not-read row:\n%s", out)
	}
	if n := strings.Count(out, "Claude Code: "+why); n != 1 {
		t.Errorf("the why shows %d times, want once:\n%s", n, out)
	}
}

func TestAgentsCodexHasNoHooks(t *testing.T) {
	var doc state.Doc
	doc.Sessions = []sample.Session{{Agent: "codex", Version: "0.9.0"}}
	if out := agentsOut(t, doc, false); !hasRow(out, "Codex", "0.9.0", "1", "no hooks: screen only") {
		t.Errorf("no Codex row with no hooks:\n%s", out)
	}
}

// TestAgentsTwoVersionsAreTwoRows: two versions of one agent are two rows,
// never one guessed version.
func TestAgentsTwoVersionsAreTwoRows(t *testing.T) {
	var doc state.Doc
	doc.Agents = []string{"claude"}
	doc.Sessions = []sample.Session{
		{Agent: "claude", Version: "2.4.0"},
		{Agent: "claude", Version: "2.3.1", UnknownScreen: true},
	}
	out := agentsOut(t, doc, false)
	if !hasRow(out, "Claude Code", "2.3.1", "1", "1") || !hasRow(out, "Claude Code", "2.4.0", "1", "0") {
		t.Errorf("want a row a version:\n%s", out)
	}
	if n := strings.Count(out, "Claude Code"); n != 2 {
		t.Errorf("%d Claude Code rows, want 2:\n%s", n, out)
	}
}

func TestAgentsIsACommand(t *testing.T) {
	if cmd, _, _, err := parseCommand([]string{"agents", "--json"}); err != nil || cmd != "agents" {
		t.Fatalf("parseCommand(agents) = %q, %v", cmd, err)
	}
	fl, _ := newFlags()
	var buf strings.Builder
	fl.SetOutput(&buf)
	if err := fl.Parse([]string{"--help"}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("--help: %v", err)
	}
	if !strings.Contains(buf.String(), "\n  matchblox agents ") {
		t.Errorf("--help does not list agents:\n%s", buf.String())
	}
}
