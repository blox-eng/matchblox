package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/blox-eng/matchblox/internal/config"
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
