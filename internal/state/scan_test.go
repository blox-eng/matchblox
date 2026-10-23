package state

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/gitscan"
)

func TestConsentRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "m", "scan.json")
	want := Consent{Answer: "yes", Roots: []string{"/h/w"}, At: time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)}
	if err := WriteConsent(p, want); err != nil {
		t.Fatal(err)
	}
	if got := ReadConsent(p); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestFoundRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "repos.json")
	want := gitscan.Found{Consent: "yes", At: time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC), Took: time.Second,
		Repos: []gitscan.Checkout{{Path: "/h/a", Branch: "main", Remote: "git@h:o/a.git", Active: time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)}}}
	if err := WriteFound(p, want); err != nil {
		t.Fatal(err)
	}
	if got := ReadFound(p); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestReadCorruptFiles(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte("{not json"), 0o600)
	if got := ReadConsent(bad); !reflect.DeepEqual(got, Consent{}) {
		t.Fatalf("corrupt consent = %+v, want never asked", got)
	}
	if got := ReadFound(bad); !reflect.DeepEqual(got, gitscan.Found{}) {
		t.Fatalf("corrupt cache = %+v, want empty", got)
	}
	if got := ReadConsent(filepath.Join(dir, "missing")); got.Answer != "" {
		t.Fatalf("missing consent = %+v", got)
	}
}

func TestScanFilesNextToState(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/x")
	if ScanPath() != "/x/matchblox/scan.json" || ReposPath() != "/x/matchblox/repos.json" {
		t.Fatalf("ScanPath %s, ReposPath %s", ScanPath(), ReposPath())
	}
}

func TestWriteIsPrivate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "scan.json")
	if err := WriteConsent(p, Consent{Answer: "no"}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if m := fi.Mode().Perm(); m != 0o600 {
		t.Fatalf("mode %o, want 600", m)
	}
}
