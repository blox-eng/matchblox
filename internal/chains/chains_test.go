package chains

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

func mem(name string) Member { return Member{SessionID: name, Pane: "%" + name} }

// chainOf makes one team of names, named after the first.
func chainOf(t *testing.T, s *Store, names ...string) Chain {
	t.Helper()
	c, err := s.Form(mem(names[0]), mem(names[1]), names[0], "r")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names[2:] {
		if c, err = s.Join(mem(n), c.Name); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func names(ms []Member) map[string]bool {
	out := map[string]bool{}
	for _, m := range ms {
		out[m.SessionID] = true
	}
	return out
}

func teamNames(cs []Chain) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return out
}

func TestFormNamesTheTeamAfterItsStarter(t *testing.T) {
	var s Store
	c, err := s.Form(mem("a"), mem("b"), "hearth", "/r")
	if err != nil || c.Name != "hearth" || c.Repo != "/r" || len(c.Members) != 2 {
		t.Fatalf("%+v %v", c, err)
	}
	c2, err := s.Form(mem("c"), mem("d"), "hearth", "/r")
	if err != nil || c2.Name != "hearth-2" {
		t.Fatalf("clash: %+v %v", c2, err)
	}
	c3, _ := s.Form(mem("e"), mem("f"), "hearth", "/r")
	if c3.Name != "hearth-3" {
		t.Fatalf("second clash: %q", c3.Name)
	}
}

func TestJoinAddsToOneTeamOnly(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b")
	if _, err := s.Form(mem("b"), mem("c"), "web", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Join(mem("d"), "web"); err != nil {
		t.Fatal(err)
	}
	if got := teamNames(s.TeamsOf(mem("b"))); !slices.Equal(got, []string{"a", "web"}) {
		t.Fatalf("b teams = %q", got)
	}
	if got := names(s.Mates(mem("a"))); len(got) != 1 || !got["b"] {
		t.Fatalf("a hears %v", got)
	}
	if got := names(s.Mates(mem("b"))); len(got) != 3 || !got["a"] || !got["c"] || !got["d"] {
		t.Fatalf("b hears %v", got)
	}
	if c, ok := s.Shared(mem("b"), mem("d")); !ok || c.Name != "web" {
		t.Fatalf("shared = %+v %v", c, ok)
	}
	if _, ok := s.Shared(mem("a"), mem("d")); ok {
		t.Fatal("a and d share a team")
	}
}

func TestJoinRefusesOffAndAlreadyIn(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b")
	if _, err := s.Join(mem("b"), "a"); !errors.Is(err, ErrIn) {
		t.Fatalf("again: %v", err)
	}
	s.SetOff(mem("c"), true)
	if _, err := s.Join(mem("c"), "a"); !errors.Is(err, ErrOff) {
		t.Fatalf("off: %v", err)
	}
	if _, err := s.Join(mem("c"), "none"); err == nil {
		t.Fatal("joined a team that is not there")
	}
}

func TestFormRefusesOffSelf(t *testing.T) {
	var s Store
	if _, err := s.Form(mem("a"), mem("a"), "a", ""); !errors.Is(err, ErrSelf) {
		t.Fatalf("self: %v", err)
	}
	s.SetOff(mem("b"), true)
	if _, err := s.Form(mem("a"), mem("b"), "a", ""); !errors.Is(err, ErrOff) {
		t.Fatalf("to off: %v", err)
	}
	if _, err := s.Form(mem("b"), mem("a"), "b", ""); !errors.Is(err, ErrOff) {
		t.Fatalf("from off: %v", err)
	}
	if len(s.Chains) != 0 {
		t.Fatalf("chains = %+v", s.Chains)
	}
}

func TestLeaveTeamEndsATeamOfOne(t *testing.T) {
	var s Store
	chainOf(t, &s, "h", "b")
	if !s.LeaveTeam(mem("h"), "h") {
		t.Fatal("not left")
	}
	if _, ok := s.Team("h"); ok || len(s.Chains) != 0 {
		t.Fatalf("chains = %+v", s.Chains)
	}
	if s.LeaveTeam(mem("h"), "h") {
		t.Fatal("left twice")
	}
}

func TestNameStaysWhenTheStarterLeaves(t *testing.T) {
	var s Store
	chainOf(t, &s, "h", "b", "c")
	s.LeaveTeam(mem("h"), "h")
	c, ok := s.Team("h")
	if got := names(c.Members); !ok || len(got) != 2 || !got["b"] || !got["c"] {
		t.Fatalf("team = %+v %v", c, ok)
	}
}

func TestLeaveLeavesEveryTeam(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b", "c")
	s.Form(mem("b"), mem("d"), "web", "")
	s.Leave(mem("b"))
	if len(s.TeamsOf(mem("b"))) != 0 {
		t.Fatalf("b teams = %+v", s.TeamsOf(mem("b")))
	}
	if _, ok := s.Team("web"); ok {
		t.Fatal("web of one stayed")
	}
}

func TestPruneLeavesEveryTeam(t *testing.T) {
	var s Store
	a := chainOf(t, &s, "a", "b", "c")
	s.Form(mem("b"), mem("d"), "web", "")
	web, _ := s.Join(mem("e"), "web")
	left := s.Prune(func(m Member) bool { return m.SessionID != "b" }, t0)
	if len(left) != 2 || left[0].Chain != a.ID || left[1].Chain != web.ID {
		t.Fatalf("left = %+v", left)
	}
	for _, l := range left {
		if l.Who.SessionID != "b" {
			t.Fatalf("left = %+v", left)
		}
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

func TestOffLeavesAndRefusesJoins(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b")
	s.SetOff(mem("b"), true)
	if len(s.Chains) != 0 {
		t.Fatal("chain stayed")
	}
	if _, err := s.Form(mem("a"), mem("b"), "a", ""); !errors.Is(err, ErrOff) {
		t.Fatalf("off member chained: %v", err)
	}
	s.SetOff(mem("b"), false)
	if _, err := s.Form(mem("a"), mem("b"), "a", ""); err != nil {
		t.Fatalf("on again: %v", err)
	}
}

// TestLoadOldStoreNamesChains: a store an older build saved, with requests
// and chains with no name, loads; its requests are gone.
func TestLoadOldStoreNamesChains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chains.json")
	old := `{"chains":[{"id":"%a#1","repo":"r","members":[{"session_id":"a","pane":"%a"},{"session_id":"b","pane":"%b"}]}],` +
		`"requests":[{"from":{"session_id":"c"},"to":{"session_id":"a"},"at":"2026-11-06T12:00:00Z"}]}`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil || len(s.Chains) != 1 || s.Chains[0].Name != "" || len(s.Chains[0].Members) != 2 {
		t.Fatalf("loaded = %+v, %v", s, err)
	}
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); strings.Contains(string(b), "requests") {
		t.Fatalf("saved = %s", b)
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
	if len(got.Chains) != 1 || got.Chains[0].Name != "a" || len(got.Off) != 1 || len(got.Take(mem("a")).Items) != 0 {
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
// (a cleared chat) stays in its team, its off mark and its inbox.
func TestRefreshKeepsAMatchWithANewSessionID(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b")
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
	if ts := s.TeamsOf(a2); len(ts) != 1 || len(ts[0].Members) != 2 {
		t.Fatalf("chain = %+v", s.Chains)
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

func TestDroppedLineTellsItsSender(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b")
	s.PostLine(mem("b"), mem("a"), "line", t0)
	for i := range InboxMax {
		s.Post(mem("b"), strconv.Itoa(i), t0)
	}
	lost := s.TakeLost()
	if len(lost) != 1 || !lost[0].To.Same(mem("b")) || !lost[0].Item.From.Same(mem("a")) || lost[0].Why != LostFull {
		t.Fatalf("lost = %+v", lost)
	}
	if len(s.TakeLost()) != 0 {
		t.Fatal("told twice")
	}
	s.Post(mem("b"), "news", t0) // news has no sender to tell
	if lost := s.TakeLost(); len(lost) != 0 {
		t.Fatalf("news lost = %+v", lost)
	}
}

func TestInboxCountFull(t *testing.T) {
	var s Store
	for i := range InboxMax + 1 {
		s.Post(mem("a"), strconv.Itoa(i), t0)
	}
	if n, full := s.InboxCount(mem("a")); n != InboxMax || !full {
		t.Fatalf("count = %d %v", n, full)
	}
	s.Take(mem("a"))
	if n, full := s.InboxCount(mem("a")); n != 0 || full {
		t.Fatalf("after take = %d %v", n, full)
	}
}

func TestLeavingDropsTheLinesOfSendersItNoLongerShares(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b", "c")
	s.Form(mem("b"), mem("d"), "web", "")
	s.PostLine(mem("b"), mem("a"), "from a", t0)
	s.PostLine(mem("b"), mem("d"), "from d", t0)
	s.Post(mem("b"), "news", t0)
	s.LeaveTeam(mem("b"), "a")
	lost := s.TakeLost()
	if len(lost) != 1 || !lost[0].Item.From.Same(mem("a")) || lost[0].Why != LostUnchained {
		t.Fatalf("lost = %+v", lost)
	}
	if got := s.Take(mem("b")); len(got.Items) != 2 || !got.Items[0].From.Same(mem("d")) {
		t.Fatalf("kept = %+v", got.Items)
	}
	s.PostLine(mem("b"), mem("d"), "again", t0)
	s.Leave(mem("b"))
	if lost := s.TakeLost(); len(lost) != 1 || !lost[0].Item.From.Same(mem("d")) || lost[0].Why != LostUnchained {
		t.Fatalf("leave all lost = %+v", lost)
	}
}

func TestUnseenKeepsTheFirstMiss(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b")
	s.MarkUnseen(mem("b"), t0)
	s.MarkUnseen(mem("b"), t0.Add(time.Minute))
	if at, ok := s.UnseenSince(mem("b")); !ok || !at.Equal(t0) {
		t.Fatalf("unseen = %v %v", at, ok)
	}
	s.MarkSeen(mem("b"))
	if _, ok := s.UnseenSince(mem("b")); ok {
		t.Fatal("seen again, still unseen")
	}
}

func TestUnseenFollowsARenameAndGoesWithItsMatch(t *testing.T) {
	var s Store
	chainOf(t, &s, "a", "b")
	s.MarkUnseen(mem("a"), t0)
	a2 := Member{SessionID: "a2", Pane: "%a"}
	s.Refresh(func(m Member) (Member, bool) { return a2, m.SessionID == "a" })
	if at, ok := s.UnseenSince(a2); !ok || !at.Equal(t0) {
		t.Fatalf("after rename = %v %v", at, ok)
	}
	s.Prune(func(m Member) bool { return m.SessionID == "b" }, t0)
	if len(s.Unseen) != 0 {
		t.Fatalf("unseen = %v", s.Unseen)
	}
}
