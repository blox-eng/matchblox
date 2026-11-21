package sample

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const noVersion = "the install path names no version"

var versionCases = []struct {
	name, agent, exe string
	argv             []string
	version, why     string
}{
	{name: "claude's exe", agent: "claude", exe: "/home/u/.local/share/claude/versions/2.3.1", version: "2.3.1"},
	{name: "cursor's script", agent: "cursor-agent", argv: []string{"node", "/home/u/.local/share/cursor-agent/versions/2025.11.06-8fe3a/index.js"}, version: "2025.11.06-8fe3a"},
	{name: "an npm install", agent: "claude", argv: []string{"node", "/usr/lib/node_modules/@anthropic-ai/claude-code/cli.js"}, why: noVersion},
	{name: "another agent's versions", agent: "claude", exe: "/home/u/.local/share/cursor-agent/versions/2025.11.06-8fe3a", why: noVersion},
	{name: "nothing to read", agent: "claude", why: noVersion},
	{name: "a dot-dot segment", agent: "claude", exe: "/x/claude/versions/../../etc/passwd", why: noVersion},
	{name: "a shell command", agent: "claude", exe: "/x/claude/versions/2.3.1;rm -rf ~", why: noVersion},
	{name: "a 300-byte segment", agent: "claude", exe: "/x/claude/versions/" + strings.Repeat("a", 300), why: noVersion},
	{name: "a terminal escape", agent: "claude", exe: "/x/claude/versions/\x1b[2J", why: noVersion},
}

func TestAVersionComesFromThePath(t *testing.T) {
	for _, c := range versionCases {
		t.Run(c.name, func(t *testing.T) {
			version, why := versionOf(c.agent, c.exe, c.argv)
			if version != c.version || why != c.why {
				t.Fatalf("versionOf(%q, %q, %q) = %q, %q; want %q, %q", c.agent, c.exe, c.argv, version, why, c.version, c.why)
			}
		})
	}
}

func TestAWhyNamesNoPath(t *testing.T) {
	for _, c := range versionCases {
		if _, why := versionOf(c.agent, c.exe, c.argv); strings.Contains(why, "/") {
			t.Errorf("%s: the why %q names a path", c.name, why)
		}
	}
}

func TestAVersionRunsNothing(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "version.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range f.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		if path == "os/exec" || path == "syscall" {
			t.Errorf("version.go imports %s: reading a version must run nothing", path)
		}
	}
}

func TestAnAgentHasAName(t *testing.T) {
	for agent, want := range map[string]string{
		"claude":       "Claude Code",
		"cursor-agent": "Cursor",
		"codex":        "Codex",
		"opencode":     "OpenCode",
		"aider":        "aider",
	} {
		if got := AgentLabel(agent); got != want {
			t.Errorf("AgentLabel(%q) = %q, want %q", agent, got, want)
		}
	}
}

// claudeAt is a sampler over the fixture with one Claude Code, pid 950,
// whose exe and argv are given.
func claudeAt(t *testing.T, comm, exe string, argv []string) *Sampler {
	t.Helper()
	root := copyFixture(t)
	addProc(t, root, 950, 1, comm, argv, "%50")
	addPane(t, root, "%50\tver:1.1\tw\t950\t"+comm+"\t/work")
	if exe != "" {
		symlink(t, exe, filepath.Join(root, "proc", "950", "exe"))
	}
	return newFixtureSampler(root)
}

func TestASessionCarriesItsVersion(t *testing.T) {
	const exe = "/home/u/.local/share/claude/versions/2.3.1"
	got := sessionOf(t, claudeAt(t, "claude", exe, []string{"claude"}).Sample(), 950)
	if got.Version != "2.3.1" || got.VersionWhy != "" {
		t.Errorf("version %q, why %q; want 2.3.1 and no why", got.Version, got.VersionWhy)
	}
}

func TestAnUnreadVersionSaysWhy(t *testing.T) {
	argv := []string{"node", "/usr/lib/node_modules/@anthropic-ai/claude-code/cli.js"}
	got := sessionOf(t, claudeAt(t, "claude", "/usr/bin/node", argv).Sample(), 950)
	if got.Version != "" || got.VersionWhy != noVersion {
		t.Errorf("version %q, why %q; want none and %q", got.Version, got.VersionWhy, noVersion)
	}
}

func TestACodexSessionTakesTheRolloutVersion(t *testing.T) {
	smp, root := daemonSession(t)
	held := daemonFile(t, filepath.Join(root, "home", ".codex"), "held", "/work/app", daemonStart.Add(time.Minute), 11)
	symlink(t, held, filepath.Join(root, "proc", "900", "fd", "21"))
	if got := sessionOf(t, smp.Sample(), 900); got.Version != "0.163.0" || got.VersionWhy != "" {
		t.Errorf("version %q, why %q; want the rollout's 0.163.0 and no why", got.Version, got.VersionWhy)
	}
}
