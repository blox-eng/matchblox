package holds

import (
	"testing"
	"time"
)

func TestHoldsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	if err := s.Set(Hold{SessionID: "s1", Pane: "%1", Name: "a", By: ByYou, At: at}); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(Hold{SessionID: "s2", Pane: "%2", Name: "b", By: ByStoker, Why: "not priority", At: at.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	h, ok, err := s.Release("s1")
	if err != nil || !ok || h.Name != "a" {
		t.Fatalf("release: %+v %v %v", h, ok, err)
	}
	re, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := re.List()
	if len(got) != 1 || got[0].SessionID != "s2" || got[0].Why != "not priority" || !got[0].At.Equal(at.Add(time.Minute)) {
		t.Fatalf("list %+v", got)
	}
	if _, ok := re.Of("s1"); ok {
		t.Fatal("s1 came back")
	}
}

func TestInMemoryWithoutADir(t *testing.T) {
	s, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set(Hold{SessionID: "s1", By: ByYou}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Of("s1"); !ok {
		t.Fatal("lost")
	}
}

func TestPruneDropsAHoldGoneForADay(t *testing.T) {
	s, _ := Open(t.TempDir())
	t0 := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	_ = s.Set(Hold{SessionID: "s1", By: ByYou})
	_ = s.Prune(map[string]bool{"s1": true}, t0)
	_ = s.Prune(nil, t0.Add(23*time.Hour))
	if _, ok := s.Of("s1"); !ok {
		t.Fatal("pruned after 23 h")
	}
	_ = s.Prune(nil, t0.Add(24*time.Hour))
	if _, ok := s.Of("s1"); ok {
		t.Fatal("kept after 24 h")
	}
}
