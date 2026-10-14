package setup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blox-eng/matchblox/internal/doors"
	"github.com/blox-eng/matchblox/internal/stoker"
)

func waysFile(e Env) string { return filepath.Join(e.Home, ".config", "matchblox", "ways.md") }

func TestWaysDoorWritesStarterOnce(t *testing.T) {
	e := env(t)
	d := door(t, e, doors.Ways)
	if !d.Open() || d.Title != "Write your ways" || d.Path != waysFile(e) || !strings.Contains(d.Preview, "+# Recipe: merge-order") {
		t.Fatalf("door = %+v", d)
	}
	if _, err := Open(e, doors.Ways, d.Sum, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(waysFile(e))
	if err != nil || string(got) != starterWays {
		t.Fatalf("file = %q, %v", got, err)
	}
	if names := stoker.Recipes(got); len(names) != 1 || names[0] != "merge-order" {
		t.Fatalf("recipes = %q", names)
	}
	if d := door(t, e, doors.Ways); !d.Done {
		t.Fatalf("door after = %+v", d)
	}
}

func TestWaysDoorNeverOverwrites(t *testing.T) {
	e := env(t)
	d := door(t, e, doors.Ways)
	if err := os.MkdirAll(filepath.Dir(waysFile(e)), 0o700); err != nil {
		t.Fatal(err)
	}
	mine := []byte("# Standing rules\n\n- mine\n")
	if err := os.WriteFile(waysFile(e), mine, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(e, doors.Ways, d.Sum, false); !errors.Is(err, ErrChanged) {
		t.Fatalf("err = %v, want ErrChanged", err)
	}
	if got, _ := os.ReadFile(waysFile(e)); string(got) != string(mine) {
		t.Fatalf("file overwritten: %q", got)
	}
	if d := door(t, e, doors.Ways); !d.Done {
		t.Fatalf("door with a file = %+v", d)
	}
}
