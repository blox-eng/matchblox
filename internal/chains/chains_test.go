package chains

import (
	"os"
	"path/filepath"
	"slices"
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
	live := func(Member) bool { return true }
	s.Prune(live, t0.Add(59*time.Minute))
	if len(s.Requests) != 1 {
		t.Fatal("lapsed early")
	}
	s.Prune(live, t0.Add(61*time.Minute))
	if len(s.Requests) != 0 {
		t.Fatal("request stayed")
	}
}

func TestAGoneMatchLeaves(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b", "c")
	id := s.Chains[0].ID
	left := s.Prune(func(m Member) bool { return m.SessionID != "c" }, t0)
	if len(left) != 1 || left[0].Who.SessionID != "c" || left[0].Chain != id {
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
	s.Prune(func(m Member) bool { return m.SessionID != "b" }, t0)
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
	got := s.Take(mem("a")).Texts()
	if len(got) != InboxMax+1 {
		t.Fatalf("items = %d", len(got))
	}
	if got[0] != "3" || got[19] != "22" || got[20] != "and 3 more" {
		t.Fatalf("items = %q", got)
	}
	if len(s.Take(mem("a")).Texts()) != 0 {
		t.Fatal("second take not empty")
	}
}

// TestGiveBackGoesFirst: what a match did not hear goes back before what
// came since, in order, and the inbox still keeps twenty and counts the rest.
func TestGiveBackGoesFirst(t *testing.T) {
	var s Store
	for i := range 22 {
		s.Post(mem("a"), strconv.Itoa(i), t0)
	}
	taken := s.Take(mem("a")) // 2 .. 21, and 2 more
	s.Post(mem("a"), "new", t0)
	s.GiveBack(mem("a"), taken)
	got := s.Take(mem("a")).Texts()
	if len(got) != InboxMax+1 || got[0] != "3" || got[18] != "21" || got[19] != "new" || got[20] != "and 3 more" {
		t.Fatalf("items = %q", got)
	}
	s.GiveBack(mem("b"), Taken{})
	if len(s.Take(mem("b")).Texts()) != 0 {
		t.Fatal("an empty give-back made news")
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
	if len(got.Chains) != 1 || len(got.Requests) != 1 || len(got.Off) != 1 || len(got.Take(mem("a")).Items) != 0 {
		t.Fatalf("loaded = %+v", got)
	}
	empty, err := Load(filepath.Join(t.TempDir(), "none.json"))
	if err != nil || len(empty.Chains) != 0 {
		t.Fatalf("missing: %+v, %v", empty, err)
	}
}

// TestPruneDropsTheOffOfGoneMatches: an off mark lasts as long as its match.
func TestPruneDropsTheOffOfGoneMatches(t *testing.T) {
	var s Store
	s.SetOff(mem("a"), true)
	s.SetOff(mem("b"), true)
	s.Prune(func(m Member) bool { return m.SessionID == "b" }, t0)
	if len(s.Off) != 1 || s.Off[0].SessionID != "b" {
		t.Fatalf("off = %+v", s.Off)
	}
}

// TestRefreshKeepsAMatchWithANewSessionID: a match whose session id changed
// (a cleared chat) stays in its chain, its requests, its off mark and its
// inbox.
func TestRefreshKeepsAMatchWithANewSessionID(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b")
	s.Ask(mem("c"), mem("a"), "r", t0)
	s.SetOff(mem("d"), true)
	s.Post(mem("a"), "news", t0)
	renew := func(m Member) (Member, bool) {
		if m.SessionID == "a" || m.SessionID == "d" {
			m.SessionID += "2"
			return m, true
		}
		return Member{}, false
	}
	if !s.Refresh(renew) {
		t.Fatal("nothing changed")
	}
	a2 := Member{SessionID: "a2", Pane: "%a"}
	if c, ok := s.ChainOf(a2); !ok || len(c.Members) != 2 {
		t.Fatalf("chain = %+v", s.Chains)
	}
	if len(s.Requests) != 1 || s.Requests[0].To != a2 {
		t.Fatalf("requests = %+v", s.Requests)
	}
	if len(s.Off) != 1 || s.Off[0].SessionID != "d2" {
		t.Fatalf("off = %+v", s.Off)
	}
	if got := s.Take(a2).Texts(); len(got) != 1 || got[0] != "news" {
		t.Fatalf("inbox = %q", got)
	}
	if s.Refresh(renew) {
		t.Fatal("a second refresh changed it again")
	}
}

// TestGiveBackKeepsTheOrderOfTwoDeliveries: two deliveries handed back in
// either order leave the inbox in the order its items came.
func TestGiveBackKeepsTheOrderOfTwoDeliveries(t *testing.T) {
	var s Store
	s.Post(mem("a"), "1", t0)
	s.Post(mem("a"), "2", t0)
	first := s.Take(mem("a"))
	s.Post(mem("a"), "3", t0)
	second := s.Take(mem("a"))
	s.Post(mem("a"), "4", t0)
	s.GiveBack(mem("a"), first)
	s.GiveBack(mem("a"), second)
	if got := s.Take(mem("a")).Texts(); !slices.Equal(got, []string{"1", "2", "3", "4"}) {
		t.Fatalf("items = %q", got)
	}
}

// TestRefreshMergesIntoAnInboxThatHasNews: news posted to the new id
// before the refresh and the old id's news meet in the order they came.
func TestRefreshMergesIntoAnInboxThatHasNews(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b")
	a2 := Member{SessionID: "a2", Pane: "%a"}
	s.Post(mem("a"), "1", t0)
	s.Post(a2, "2", t0)
	s.Post(mem("a"), "3", t0)
	s.Refresh(func(m Member) (Member, bool) { return a2, m.SessionID == "a" })
	if got := s.Take(a2).Texts(); !slices.Equal(got, []string{"1", "2", "3"}) {
		t.Fatalf("items = %q", got)
	}
}

// TestGiveBackAfterARefreshGoesToTheNewID: a delivery taken before the
// refresh gives its news back to the match as it is named now.
func TestGiveBackAfterARefreshGoesToTheNewID(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b")
	s.Post(mem("a"), "1", t0)
	taken := s.Take(mem("a"))
	a2 := Member{SessionID: "a2", Pane: "%a"}
	s.Refresh(func(m Member) (Member, bool) { return a2, m.SessionID == "a" })
	s.GiveBack(mem("a"), taken)
	if got := s.Take(a2).Texts(); !slices.Equal(got, []string{"1"}) {
		t.Fatalf("items = %q", got)
	}
}

// TestARenameGoesWithItsMatch: once a refreshed match is gone, its old
// name maps to nothing any more.
func TestARenameGoesWithItsMatch(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b")
	s.Post(mem("a"), "1", t0)
	a2 := Member{SessionID: "a2", Pane: "%a"}
	s.Refresh(func(m Member) (Member, bool) { return a2, m.SessionID == "a" })
	if len(s.renamed) != 1 {
		t.Fatalf("renamed = %v", s.renamed)
	}
	s.Prune(func(m Member) bool { return m.SessionID == "b" }, t0)
	if len(s.renamed) != 0 {
		t.Fatalf("renamed = %v", s.renamed)
	}
}

func TestJoinRefusesOff(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b")
	c, _ := s.ChainOf(mem("a"))
	if err := s.Join(c.ID, mem("n")); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ChainOf(mem("n")); got.ID != c.ID || len(got.Members) != 3 || len(s.Requests) != 0 {
		t.Fatalf("n did not join with no ask: %+v %+v", got, s.Requests)
	}
	if err := s.Join(c.ID, mem("n")); err != nil {
		t.Fatalf("a member joins its own chain again: %v", err)
	}
	s.SetOff(mem("off"), true)
	if err := s.Join(c.ID, mem("off")); err == nil {
		t.Fatal("an off match joined")
	}
	chainOf(t, &s, "x", "y")
	if err := s.Join(c.ID, mem("x")); err == nil {
		t.Fatal("a match in another chain joined")
	}
	if err := s.Join("gone", mem("z")); err == nil {
		t.Fatal("z joined a chain that is not there")
	}
	if _, ok := s.ChainOf(mem("off")); ok {
		t.Fatal("off is in a chain")
	}
	if got, _ := s.ChainOf(mem("x")); got.ID == c.ID {
		t.Fatal("x left its chain")
	}
}
