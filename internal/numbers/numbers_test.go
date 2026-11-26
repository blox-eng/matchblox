package numbers

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/blox-eng/matchblox/internal/chains"
)

func mem(id string) chains.Member {
	return chains.Member{SessionID: id, Pane: "%" + id, PID: 100, Start: 7}
}

func assign(t *testing.T, s *Store, ms ...chains.Member) []int {
	t.Helper()
	got, err := s.Assign(ms)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func open(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNumbersGoLowestFirst(t *testing.T) {
	s := open(t, t.TempDir())
	if got := assign(t, s, mem("a"), mem("b"), mem("c")); !slices.Equal(got, []int{1, 2, 3}) {
		t.Fatalf("numbers = %v, want [1 2 3]", got)
	}
}

func TestANumberStaysWithItsSession(t *testing.T) {
	s := open(t, t.TempDir())
	assign(t, s, mem("a"), mem("b"), mem("c"))
	if got := assign(t, s, mem("a"), mem("c")); !slices.Equal(got, []int{1, 3}) {
		t.Fatalf("after b ends = %v, want [1 3]", got)
	}
	if got := assign(t, s, mem("a"), mem("c"), mem("d")); !slices.Equal(got, []int{1, 3, 2}) {
		t.Fatalf("after d starts = %v, want [1 3 2]", got)
	}
}

func TestNoNumberPastNine(t *testing.T) {
	s := open(t, t.TempDir())
	var ms []chains.Member
	for i := range 11 {
		ms = append(ms, mem(strconv.Itoa(i)))
	}
	want := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 0}
	if got := assign(t, s, ms...); !slices.Equal(got, want) {
		t.Fatalf("numbers = %v, want %v", got, want)
	}
}

func TestNumbersSurviveReopen(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	assign(t, s, mem("a"), mem("b"), mem("c"))
	assign(t, s, mem("a"), mem("c"))
	again := open(t, dir)
	if got := assign(t, again, mem("c"), mem("a")); !slices.Equal(got, []int{3, 1}) {
		t.Fatalf("after reopen = %v, want [3 1]", got)
	}
}

func TestAClearedChatKeepsItsNumber(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	assign(t, s, mem("a"), mem("b"))
	cleared := mem("b")
	cleared.SessionID = "b2"
	if got := assign(t, s, mem("a"), cleared); !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("after clear = %v, want [1 2]", got)
	}
	again := open(t, dir)
	cleared.Pane = "%moved"
	if got := assign(t, again, cleared, mem("a")); !slices.Equal(got, []int{2, 1}) {
		t.Fatalf("the new session id was not kept: %v, want [2 1]", got)
	}
}

func TestACorruptFileStartsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "numbers.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := open(t, dir)
	if got := assign(t, s, mem("z")); !slices.Equal(got, []int{1}) {
		t.Fatalf("numbers = %v, want [1]", got)
	}
	bad, err := filepath.Glob(filepath.Join(dir, "numbers.json.bad-*"))
	if err != nil || len(bad) != 1 {
		t.Fatalf("moved aside = %v, %v", bad, err)
	}
}

// TestTwoSessionsWithOneIDKeepTheirNumbers: a resumed chat in a second pane
// reports the first one's session id. Each keeps its own number when the
// list re-sorts.
func TestTwoSessionsWithOneIDKeepTheirNumbers(t *testing.T) {
	s := open(t, t.TempDir())
	a := chains.Member{SessionID: "X", Pane: "%1", PID: 10, Start: 1}
	b := chains.Member{SessionID: "X", Pane: "%2", PID: 20, Start: 2}
	c := chains.Member{SessionID: "Y", Pane: "%3", PID: 30, Start: 3}
	if got := assign(t, s, a, c, b); !slices.Equal(got, []int{1, 2, 3}) {
		t.Fatalf("order a, c, b = %v, want [1 2 3]", got)
	}
	if got := assign(t, s, b, a, c); !slices.Equal(got, []int{3, 1, 2}) {
		t.Fatalf("order b, a, c = %v, want [3 1 2]", got)
	}
}
