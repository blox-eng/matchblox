package chains

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

func mem(name string) Member { return Member{SessionID: name, Pane: "%" + name} }

func chainOf(t *testing.T, s *Store, names ...string) {
	t.Helper()
	for _, n := range names[1:] {
		s.Ask(mem(names[0]), mem(n), "r", t0)
		s.Ask(mem(n), mem(names[0]), "r", t0)
	}
}

func names(ms []Member) map[string]bool {
	out := map[string]bool{}
	for _, m := range ms {
		out[m.SessionID] = true
	}
	return out
}

func TestOneSideAloneFormsNoChain(t *testing.T) {
	var s Store
	if s.Ask(mem("a"), mem("b"), "r", t0) {
		t.Fatal("formed with one side")
	}
	if _, ok := s.ChainOf(mem("a")); ok {
		t.Fatal("a has a chain")
	}
	if len(s.Requests) != 1 {
		t.Fatalf("requests = %d", len(s.Requests))
	}
}

func TestBothSidesFormAChain(t *testing.T) {
	var s Store
	s.Ask(mem("a"), mem("b"), "r", t0)
	if !s.Ask(mem("b"), mem("a"), "r", t0) {
		t.Fatal("not formed")
	}
	if len(s.Chains) != 1 || len(s.Requests) != 0 {
		t.Fatalf("chains=%d requests=%d", len(s.Chains), len(s.Requests))
	}
	c, _ := s.ChainOf(mem("a"))
	if got := names(c.Members); len(got) != 2 || !got["a"] || !got["b"] || c.Repo != "r" {
		t.Fatalf("chain = %+v", c)
	}
}

func TestAMemberOfTheChainCanAccept(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b")
	s.Ask(mem("c"), mem("a"), "r", t0)
	if !s.Ask(mem("b"), mem("c"), "r", t0) {
		t.Fatal("not formed")
	}
	if len(s.Chains) != 1 || len(s.Chains[0].Members) != 3 || len(s.Requests) != 0 {
		t.Fatalf("state = %+v", s)
	}
}

func TestTwoChainsMergeIntoOne(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b")
	chainOf(t, &s, "c", "d")
	s.Ask(mem("a"), mem("c"), "r", t0)
	s.Ask(mem("d"), mem("b"), "r", t0)
	if len(s.Chains) != 1 || len(s.Chains[0].Members) != 4 {
		t.Fatalf("chains = %+v", s.Chains)
	}
}

func TestARequestLapsesAfterAnHour(t *testing.T) {
	var s Store
	s.Ask(mem("a"), mem("b"), "r", t0)
	live := func(Member) (string, bool) { return "r", true }
	s.Prune(live, t0.Add(59*time.Minute))
	if len(s.Requests) != 1 {
		t.Fatal("lapsed early")
	}
	s.Prune(live, t0.Add(61*time.Minute))
	if len(s.Requests) != 0 {
		t.Fatal("request stayed")
	}
}

func TestAMatchThatMovesLeaves(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b", "c")
	id := s.Chains[0].ID
	live := func(m Member) (string, bool) {
		if m.SessionID == "c" {
			return "r2", true
		}
		return "r", true
	}
	left := s.Prune(live, t0)
	if len(left) != 1 || left[0].Who.SessionID != "c" || left[0].Repo != "r2" || left[0].Chain != id {
		t.Fatalf("left = %+v", left)
	}
	c, ok := s.ChainOf(mem("a"))
	if !ok || len(c.Members) != 2 {
		t.Fatalf("chain = %+v", c)
	}
	if _, ok := s.ChainOf(mem("c")); ok {
		t.Fatal("c still chained")
	}
}

func TestAChainOfOneEnds(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b")
	s.Prune(func(m Member) (string, bool) { return "r", m.SessionID != "b" }, t0)
	if len(s.Chains) != 0 {
		t.Fatalf("chains = %+v", s.Chains)
	}
}

func TestOffLeavesAndRefusesAsks(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b")
	s.SetOff(mem("b"), true)
	if len(s.Chains) != 0 {
		t.Fatal("chain stayed")
	}
	s.Ask(mem("a"), mem("b"), "r", t0)
	if s.Ask(mem("b"), mem("a"), "r", t0) || len(s.Requests) != 0 {
		t.Fatal("off member chained")
	}
	s.SetOff(mem("b"), false)
	s.Ask(mem("a"), mem("b"), "r", t0)
	if !s.Ask(mem("b"), mem("a"), "r", t0) {
		t.Fatal("on again, not formed")
	}
}

func TestTheInboxKeepsTwentyAndCountsTheRest(t *testing.T) {
	var s Store
	for i := range 23 {
		s.Post(mem("a"), strconv.Itoa(i), t0.Add(time.Duration(i)*time.Second))
	}
	got := s.Take(mem("a"))
	if len(got) != InboxMax+1 {
		t.Fatalf("items = %d", len(got))
	}
	if got[0].Text != "3" || got[19].Text != "22" || got[20].Text != "and 3 more" {
		t.Fatalf("items = %+v", got)
	}
	if len(s.Take(mem("a"))) != 0 {
		t.Fatal("second take not empty")
	}
}

func TestSaveAndLoad(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b")
	s.Ask(mem("c"), mem("a"), "r", t0)
	s.SetOff(mem("d"), true)
	s.Post(mem("a"), "news", t0)
	path := filepath.Join(t.TempDir(), "state", "chains.json")
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("stat = %v, %v", fi, err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Chains) != 1 || len(got.Requests) != 1 || len(got.Off) != 1 || len(got.Take(mem("a"))) != 0 {
		t.Fatalf("loaded = %+v", got)
	}
	empty, err := Load(filepath.Join(t.TempDir(), "none.json"))
	if err != nil || len(empty.Chains) != 0 {
		t.Fatalf("missing: %+v, %v", empty, err)
	}
}
