package stoker

import (
	"slices"
	"testing"

	"github.com/blox-eng/matchblox/internal/config"
)

func TestRecipesInOrder(t *testing.T) {
	md := "# Standing rules\n\n- x\n\n# Recipe: merge-order\n\n1. a\n\n#   Recipe:  release-night  \nbody\n# Recipe: tidy\n"
	want := []string{"merge-order", "release-night", "tidy"}
	if got := Recipes([]byte(md)); !slices.Equal(got, want) {
		t.Fatalf("Recipes = %q, want %q", got, want)
	}
}

func TestRecipesIgnoresOtherHeadings(t *testing.T) {
	md := "# Recipes\n## Recipe: nested\n# Recipe:\n# Recipe:    \n# Priority\nRecipe: plain\n    # Recipe: indented\n"
	if got := Recipes([]byte(md)); len(got) != 0 {
		t.Fatalf("Recipes = %q, want none", got)
	}
}

func TestWaysPathSitsBesideTheConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if got, want := config.WaysPath(), config.Path()[:len(config.Path())-len("config.toml")]+"ways.md"; got != want {
		t.Fatalf("WaysPath = %s, want %s", got, want)
	}
}
