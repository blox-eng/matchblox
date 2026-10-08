package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tui.toml")
	os.WriteFile(path, []byte(body), 0o644)
	return path
}

func TestMissingFileIsDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "none.toml"))
	if err != nil || c.Interval.Duration != 2*time.Second || c.Orphans.CPUOver != 80 {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestOverlay(t *testing.T) {
	c, err := Load(write(t, `
interval = "5s"
[alerts]
temp_over_c = 85
load1_over = 40
load1_for = "3m"
power_limits_w = [100, 150]
[orphans]
cpu_over = 70
[[groups]]
name = "CI"
container = "^ci-"
[sessions.windows]
"model-x" = 1000000
[hooks]
alert = ["notify-send", "console"]
`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Interval.Duration != 5*time.Second || c.Alerts.TempOverC != 85 || c.Alerts.Load1For.Duration != 3*time.Minute ||
		len(c.Alerts.PowerLimitsW) != 2 || c.Orphans.CPUOver != 70 || c.Sessions.Windows["model-x"] != 1_000_000 ||
		len(c.Hooks.Alert) != 2 {
		t.Fatalf("%+v", c)
	}
	// Unset values keep their defaults.
	if c.Orphans.For.Duration != 30*time.Second || c.Sessions.CompactAt != 85 {
		t.Fatalf("defaults lost: %+v", c)
	}
	if !c.Groups[0].Match("ci-runner-2") || c.Groups[0].Match("app-ci-") {
		t.Fatal("group pattern")
	}
}

func TestBadInput(t *testing.T) {
	if _, err := Load(write(t, "[[groups]]\nname = \"x\"\ncontainer = \"(\"\n")); err == nil {
		t.Error("bad regexp accepted")
	}
	if _, err := Load(write(t, "interval = \"soon\"\n")); err == nil {
		t.Error("bad duration accepted")
	}
}

func TestExampleLoads(t *testing.T) {
	c, err := Load("../../config.example.toml")
	if err != nil {
		t.Fatal(err)
	}
	if d := Default(); c.Interval != d.Interval || c.Sessions.CompactAt != d.Sessions.CompactAt || c.Sessions.ProgressPrompt != d.Sessions.ProgressPrompt || c.Orphans.CPUOver != d.Orphans.CPUOver {
		t.Fatalf("the example must state the defaults: %+v", c)
	}
}

// TestAgentsSetTellsAConfigWithoutAgents: a config written before agents
// were found has no agents key; the console then finds them at start.
func TestAgentsSetTellsAConfigWithoutAgents(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.toml")
	os.WriteFile(old, []byte("[alerts]\nload1_over = 8\n"), 0o600)
	if c, err := Load(old); err != nil || c.AgentsSet {
		t.Fatalf("old config: set %v, %v", c.AgentsSet, err)
	}
	set := filepath.Join(dir, "set.toml")
	os.WriteFile(set, []byte("agents = [\"codex\"]\n"), 0o600)
	if c, _ := Load(set); !c.AgentsSet || strings.Join(c.Agents, ",") != "codex" {
		t.Fatalf("set config: %+v", c.Agents)
	}
}
