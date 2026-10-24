package clone

import (
	"math/rand/v2"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestFreeName(t *testing.T) {
	rnd := rand.New(rand.NewPCG(1, 2))
	taken := map[string]bool{"r": true}
	got := FreeName("r", func(n string) bool { return taken[n] }, rnd)
	if !strings.HasPrefix(got, "r-") || !slices.Contains(Words, strings.TrimPrefix(got, "r-")) {
		t.Fatalf("FreeName = %q", got)
	}
	for _, w := range Words {
		taken["r-"+w] = true
	}
	got = FreeName("r", func(n string) bool { return taken[n] }, rnd)
	if !regexp.MustCompile(`^r-(ember|flint|spark|cinder|tinder|wick|flare|char|soot|coal)-[a-z0-9]{4}$`).MatchString(got) {
		t.Fatalf("FreeName with every word taken = %q", got)
	}
}
