package stoker

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFamilyDir(t *testing.T) {
	if got := FamilyDir("/st", HostID); got != "/st" {
		t.Errorf("host dir = %q", got)
	}
	if got := FamilyDir("/st", "s2"); got != "/st/stokers/s2" {
		t.Errorf("chain dir = %q", got)
	}
}

func TestFamilyIDsSortsByNumber(t *testing.T) {
	state := t.TempDir()
	for _, id := range []string{"s10", "s2"} {
		if err := WriteLink(FamilyDir(state, id), Link{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(FamilyDir(state, "junk"), 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := FamilyIDs(state)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"s2", "s10"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ids = %v, want %v", got, want)
	}
}

func TestFamilyIDsMissingDir(t *testing.T) {
	got, err := FamilyIDs(filepath.Join(t.TempDir(), "none"))
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v", got, err)
	}
}

func TestNextID(t *testing.T) {
	if got := NextID(nil); got != "s1" {
		t.Errorf("none = %q", got)
	}
	if got := NextID([]string{"s1", "s3"}); got != "s4" {
		t.Errorf("s1,s3 = %q", got)
	}
}

func TestSessionNameRoundTrip(t *testing.T) {
	if id, ok := IDOfSession(SessionName("s4")); id != "s4" || !ok {
		t.Errorf("s4 = %q, %v", id, ok)
	}
	if id, ok := IDOfSession("stoker"); id != "" || !ok {
		t.Errorf("host = %q, %v", id, ok)
	}
	if _, ok := IDOfSession("api"); ok {
		t.Error("api is no stoker")
	}
}

func TestLinkRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "stokers", "s1")
	want := Link{ID: "s1", Chain: "api", Giver: "person", Grants: []string{"merge"}}
	if err := WriteLink(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLink(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
