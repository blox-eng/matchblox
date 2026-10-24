package state

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestUsedAddKeepsTheNewestFirstOnce(t *testing.T) {
	t0 := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	u := Used{}.Add(t0, "/src/a", "/src/b/").Add(t0.Add(time.Hour), "/src/a")
	if len(u.Repos) != 2 || u.Repos[0].Path != "/src/a" || !u.Repos[0].At.Equal(t0.Add(time.Hour)) || u.Repos[1].Path != "/src/b" {
		t.Fatalf("%+v", u)
	}
	for i := range MaxUsed + 5 {
		u = u.Add(t0, fmt.Sprintf("/src/x%d", i))
	}
	if len(u.Repos) != MaxUsed {
		t.Fatalf("%d repos kept", len(u.Repos))
	}
}

func TestUsedReadsAMissingOrBrokenFileAsEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "used.json")
	if u := ReadUsed(p); len(u.Repos) != 0 {
		t.Fatalf("%+v", u)
	}
	u := Used{}.Add(time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC), "/src/a")
	if err := WriteUsed(p, u); err != nil {
		t.Fatal(err)
	}
	if got := ReadUsed(p); len(got.Repos) != 1 || got.Repos[0].Path != "/src/a" {
		t.Fatalf("%+v", got)
	}
}
