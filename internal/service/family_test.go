package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/matchblox/internal/advice"
	"github.com/blox-eng/matchblox/internal/chains"
	"github.com/blox-eng/matchblox/internal/gitscan"
	"github.com/blox-eng/matchblox/internal/holds"
	"github.com/blox-eng/matchblox/internal/hooks"
	"github.com/blox-eng/matchblox/internal/proto"
	"github.com/blox-eng/matchblox/internal/sample"
	"github.com/blox-eng/matchblox/internal/state"
	"github.com/blox-eng/matchblox/internal/stoker"
)

func TestFamilyIsHostOnlyWithoutChains(t *testing.T) {
	s, _, _ := stokerTest(t)
	if err := s.loadFamily(); err != nil {
		t.Fatal(err)
	}
	fam := s.family()
	if len(fam) != 1 || fam[0].id != stoker.HostID || fam[0].store != s.Stoker {
		t.Fatalf("the family is not the host alone: %+v", fam)
	}
	if s.unitByID(stoker.HostID) != fam[0] || s.hostUnit() != fam[0] {
		t.Fatal("the host unit is not one unit")
	}
	s.Stoker = nil
	if fam := s.family(); len(fam) != 0 || s.hostUnit() != nil {
		t.Fatalf("a service with no stoker has a family: %+v", fam)
	}
}

func TestFamilyReloadsAfterRestart(t *testing.T) {
	s, _, _ := stokerTest(t)
	root := s.Stoker.Dir()
	dir := stoker.FamilyDir(root, "s1")
	if err := stoker.WriteLink(dir, stoker.Link{ID: "s1", Chain: "c1", Giver: "person"}); err != nil {
		t.Fatal(err)
	}
	st, err := stoker.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Set(stoker.Mode{}.TurnOn(night0)); err != nil {
		t.Fatal(err)
	}

	again := newTest(t, time.Hour) // the service restarts
	if again.Stoker, err = stoker.Open(root); err != nil {
		t.Fatal(err)
	}
	if err := again.loadFamily(); err != nil {
		t.Fatal(err)
	}
	u := again.unitByID("s1")
	if u == nil || !u.store.Mode().On || u.link.Chain != "c1" || u.store.Dir() != dir {
		t.Fatalf("the chain unit did not come back: %+v", u)
	}
	if fam := again.family(); len(fam) != 2 || fam[0].id != stoker.HostID || fam[1] != u {
		t.Fatalf("the family is not host, then s1: %+v", fam)
	}
	if again.unitOfChain("c1") != u || again.unitOfChain("c2") != nil {
		t.Fatal("the unit of chain c1 is not s1")
	}
}

func TestUnknownStokerSessionFailsClosed(t *testing.T) {
	s, ran := handsTest(t)
	s.publish(sample.Snapshot{At: night0, Sessions: []sample.Session{
		{Agent: "claude", Pane: "%1", Target: "app:1.1", SessionID: "s1", Name: "work", Status: "idle", Idle: 5 * time.Minute, Tokens: 176000, ContextPct: 88},
		{Agent: "claude", Pane: "%9@mbtest", Target: "stoker:1.1", SessionID: "s9", Name: "stoker", Status: "busy", Busy: true},
		{Agent: "claude", Pane: "%8@mbtest", Target: "stoker-s9:1.1", SessionID: "s8", Name: "stoker-s9", Status: "idle"},
	}}, func(state.Doc) {})
	if u, on := s.unitOf(proto.Act{From: "%9@mbtest"}); !on || u != s.hostUnit() {
		t.Fatalf("the host's pane is not the host's: %+v %v", u, on)
	}
	if u, on := s.unitOf(proto.Act{From: "%8@mbtest"}); !on || u != nil {
		t.Fatalf("an unknown stoker session has a unit: %+v %v", u, on)
	}
	for _, a := range []proto.Act{
		{RecID: "message:%1", Text: "rebase on main"},
		{RecID: "stoker:plan", Text: `{"steps":[{"n":1,"session":"work","text":"x"}]}`},
		{RecID: "stoker:step", Text: `{"step":1,"state":"doing","text":"x"}`},
	} {
		a.Confirm, a.From, a.FromPlaced = "y", "%8@mbtest", true
		if res := s.act(t.Context(), a); res.Err != "no agent acts on the stoker's own session" {
			t.Fatalf("%s: %q", a.RecID, res.Err)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

// A stoker pane the sample does not name yet is the host's while the host
// is the whole family, and fails closed once a chain unit runs.
func TestAnUnsampledStokerPaneIsTheHostsAlone(t *testing.T) {
	s, _ := handsTest(t)
	if u, on := s.unitOf(proto.Act{From: "%7@mbtest"}); !on || u != s.hostUnit() {
		t.Fatalf("an unsampled stoker pane is not the host's: %+v %v", u, on)
	}
	dir := stoker.FamilyDir(s.Stoker.Dir(), "s1")
	if err := stoker.WriteLink(dir, stoker.Link{ID: "s1", Chain: "c1", Giver: "person"}); err != nil {
		t.Fatal(err)
	}
	if err := s.loadFamily(); err != nil {
		t.Fatal(err)
	}
	if u, on := s.unitOf(proto.Act{From: "%7@mbtest"}); !on || u != nil {
		t.Fatalf("with a chain unit, an unsampled stoker pane has a unit: %+v %v", u, on)
	}
	s.publish(sample.Snapshot{At: night0, Panes: []sample.Pane{{ID: "%7@mbtest", Target: "stoker-s1:1.1"}}}, func(state.Doc) {})
	if u, on := s.unitOf(proto.Act{From: "%7@mbtest"}); !on || u == nil || u.id != "s1" {
		t.Fatalf("the pane of stoker-s1 is not s1's: %+v %v", u, on)
	}
}

func TestLoadFamilySkipsABrokenLink(t *testing.T) {
	s, _, _ := stokerTest(t)
	root := s.Stoker.Dir()
	if err := stoker.WriteLink(stoker.FamilyDir(root, "s1"), stoker.Link{ID: "s1", Chain: "c1", Giver: "person"}); err != nil {
		t.Fatal(err)
	}
	bad := stoker.FamilyDir(root, "s2")
	if err := os.MkdirAll(bad, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "link.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := s.loadFamily()
	if err == nil || !strings.Contains(err.Error(), "stoker s2") {
		t.Fatalf("a broken link.json gave %v", err)
	}
	if s.unitByID("s1") == nil || s.unitByID("s2") != nil {
		t.Fatalf("the family is %+v", s.family())
	}
}

func TestLoadFamilyKeepsItsUnits(t *testing.T) {
	s, _, _ := stokerTest(t)
	if err := stoker.WriteLink(stoker.FamilyDir(s.Stoker.Dir(), "s1"), stoker.Link{ID: "s1", Chain: "c1", Giver: "person"}); err != nil {
		t.Fatal(err)
	}
	if err := s.loadFamily(); err != nil {
		t.Fatal(err)
	}
	u := s.unitByID("s1")
	if err := s.loadFamily(); err != nil {
		t.Fatal(err)
	}
	if u == nil || s.unitByID("s1") != u {
		t.Fatal("a second load made a new unit: its memory is lost")
	}
}

// chainStokerTest is agentTest with the chain c1 of api-auth (%3) and
// api-db (%4) in /dev/api, and work (%1) in no chain. The clock is the
// test's.
func chainStokerTest(t *testing.T) (*Service, *[][]string, *time.Time) {
	t.Helper()
	s, ran := agentTest(t)
	now := night0
	s.Now = func() time.Time { return now }
	auth := sample.Session{Agent: "claude", Pane: "%3", Target: "api:1.1", SessionID: "s3", Name: "api-auth", Status: "idle", Idle: 5 * time.Minute}
	db := sample.Session{Agent: "claude", Pane: "%4", Target: "api:1.2", SessionID: "s4", Name: "api-db", Status: "idle", Idle: 5 * time.Minute}
	s.mu.Lock() // a sample's chain news may read them meanwhile
	s.Chains = &chains.Store{Chains: []chains.Chain{{ID: "c1", Repo: "/dev/api", Members: []chains.Member{memberOf(auth), memberOf(db)}}}}
	s.mu.Unlock()
	s.publish(sample.Snapshot{At: night0, Sessions: []sample.Session{
		{Agent: "claude", Pane: "%1", Target: "app:1.1", SessionID: "s1", Name: "work", Status: "idle", Idle: 5 * time.Minute},
		auth, db,
	}}, func(state.Doc) {})
	return s, ran, &now
}

// chainStokerUp is the sample once the agent of stoker-s1 rests.
func chainStokerUp(s *Service) {
	s.mu.Lock()
	sessions := slices.Clone(s.cur.Sessions)
	s.mu.Unlock()
	sessions = append(sessions, sample.Session{Agent: "claude", Pane: "%5@mbtest", Target: "stoker-s1:1.1", SessionID: "s5", Name: "stoker-s1", Status: "idle"})
	s.publish(sample.Snapshot{At: night0, Sessions: sessions}, func(state.Doc) {})
}

func cardTexts(st *stoker.Store) []string {
	var out []string
	for _, e := range st.Entries() {
		out = append(out, e.Text)
	}
	return out
}

func TestStartChainStokerMakesItsUnit(t *testing.T) {
	s, ran, _ := chainStokerTest(t)
	root := s.Stoker.Dir()
	dir := stoker.FamilyDir(root, "s1")
	run := s.Actions.Run
	linkFirst := false
	s.Actions.Run = func(argv []string) error {
		if slices.Contains(argv, "new-session") {
			_, link := os.Stat(filepath.Join(dir, "link.json"))
			_, mode := os.Stat(filepath.Join(dir, "stoker.json"))
			linkFirst = link == nil && mode != nil
		}
		return run(argv)
	}
	u, err := s.startChainStoker(t.Context(), "api-db", "merge on green", []string{"merge"}, "person")
	if err != nil {
		t.Fatal(err)
	}
	if !linkFirst {
		t.Fatal("the agent started before link.json, or after stoker.json")
	}
	l, err := stoker.ReadLink(dir)
	if err != nil || l.ID != "s1" || l.Chain != "c1" || l.Giver != "person" || !slices.Equal(l.Grants, []string{"merge"}) {
		t.Fatalf("link %+v, %v", l, err)
	}
	home, _ := os.UserHomeDir()
	start := []string{"env", "-u", "GIT_TERMINAL_PROMPT", "-u", "GIT_SSH_COMMAND", "-u", "MATCHBLOX_HOME",
		"tmux", "-L", "mbtest", "-f", os.DevNull, "new-session", "-d", "-s", "stoker-s1", "-c", home, "claude", ";", "set-option", "-s", "exit-empty", "on"}
	if !slices.ContainsFunc(*ran, func(argv []string) bool { return slices.Equal(argv, start) }) {
		t.Fatalf("ran %q", *ran)
	}
	if u == nil || s.unitByID("s1") != u || s.unitOfChain("c1") != u || u.store.Dir() != dir {
		t.Fatalf("unit %+v", u)
	}
	m := u.store.Mode()
	if !m.On || m.Nightly || m.Order == nil || m.Order.Words != "merge on green" || !slices.Equal(m.Order.Grants, []string{"merge"}) {
		t.Fatalf("mode %+v", m)
	}
	if !slices.Contains(cardTexts(s.Stoker), "started the stoker of chain api-auth (s1)") {
		t.Fatalf("host card %q", cardTexts(s.Stoker))
	}
	brief := filepath.Join(dir, "stoker-brief.md")
	b, err := os.ReadFile(brief)
	if err != nil || !strings.Contains(string(b), "You are the stoker of chain api-auth.") || strings.Contains(string(b), "work (%1)") {
		t.Fatalf("brief %v:\n%s", err, b)
	}
	if got := typed(*ran, "%5@mbtest"); len(got) != 0 {
		t.Fatalf("typed into an agent that has not come up: %q", got)
	}

	s.deliverBriefs(t.Context()) // not up yet: the line waits
	chainStokerUp(s)
	s.deliverBriefs(t.Context())
	s.deliverBriefs(t.Context()) // once only
	if got := typed(*ran, "%5@mbtest"); !slices.Equal(got, []string{"Read " + brief + " and follow it."}) {
		t.Fatalf("typed %q", got)
	}
	if len(typed(*ran, "%9@mbtest")) != 0 {
		t.Fatal("the host stoker was typed into")
	}
}

func TestStartChainStokerDropsAnUntypedBrief(t *testing.T) {
	s, ran, now := chainStokerTest(t)
	u, err := s.startChainStoker(t.Context(), "api-auth", "", nil, "person")
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(stokerStartsIn)
	s.deliverBriefs(t.Context())
	chainStokerUp(s) // too late: the line is gone
	s.deliverBriefs(t.Context())
	if got := typed(*ran, "%5@mbtest"); len(got) != 0 {
		t.Fatalf("typed %q", got)
	}
	const line = "the stoker of chain api-auth did not start: its brief was not typed"
	for name, st := range map[string]*stoker.Store{"unit": u.store, "host": s.Stoker} {
		if n := slices.Index(cardTexts(st), line); n < 0 || slices.Contains(cardTexts(st)[n+1:], line) {
			t.Fatalf("%s card %q", name, cardTexts(st))
		}
	}
}

func TestStartChainStokerAtCap(t *testing.T) {
	s, ran, _ := chainStokerTest(t)
	s.MaxChains = 1
	other := stoker.FamilyDir(s.Stoker.Dir(), "s1")
	if err := stoker.WriteLink(other, stoker.Link{ID: "s1", Chain: "c9", Giver: "person"}); err != nil {
		t.Fatal(err)
	}
	st, err := stoker.Open(other)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Set(stoker.Mode{}.TurnOn(night0)); err != nil {
		t.Fatal(err)
	}
	if err := s.loadFamily(); err != nil {
		t.Fatal(err)
	}
	*ran = nil
	_, err = s.startChainStoker(t.Context(), "api-auth", "", nil, "person")
	if err == nil || err.Error() != "the family has its 1 chain stokers: max_chains. Quit one on the STOKER tab first." {
		t.Fatalf("err %v", err)
	}
	if len(*ran) != 0 || s.unitOfChain("c1") != nil {
		t.Fatalf("ran %q", *ran)
	}
	// A unit that quit with its run off counts no more.
	if err := st.Set(stoker.Mode{}); err != nil {
		t.Fatal(err)
	}
	s.unitByID("s1").store = st
	u, err := s.startChainStoker(t.Context(), "api-auth", "", nil, "person")
	if err != nil || u.id != "s2" {
		t.Fatalf("unit %+v, %v", u, err)
	}
}

func TestStartNeedsAChain(t *testing.T) {
	s, ran, _ := chainStokerTest(t)
	for _, who := range []string{"work", "nobody"} {
		if _, err := s.startChainStoker(t.Context(), who, "", nil, "person"); err == nil || err.Error() != who+" is in no chain." {
			t.Fatalf("%s: %v", who, err)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
	for _, who := range []string{"c1", "%4"} {
		u, err := s.startChainStoker(t.Context(), who, "", nil, "person")
		switch {
		case who == "c1" && (err != nil || u.link.Chain != "c1"):
			t.Fatalf("by chain id: %+v, %v", u, err)
		case who == "%4" && (err == nil || err.Error() != "chain api-auth has its stoker: give it an order."):
			t.Fatalf("a second start: %v", err)
		}
	}
}

func TestStokerNewIsThePersons(t *testing.T) {
	s, ran, _ := chainStokerTest(t)
	act := proto.Act{RecID: "stoker:new", Confirm: "y", Text: `{"chain":"api-auth","words":"go","grants":["merge"]}`}
	// A chain unit of another chain, whose pane is stoker-s1's.
	if err := stoker.WriteLink(stoker.FamilyDir(s.Stoker.Dir(), "s1"), stoker.Link{ID: "s1", Chain: "c9", Giver: "person"}); err != nil {
		t.Fatal(err)
	}
	if err := s.loadFamily(); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.cur.Panes = append(s.cur.Panes, sample.Pane{ID: "%6@mbtest", Target: "stoker-s1:1.1"}, sample.Pane{ID: "%9@mbtest", Target: "stoker:1.1"})
	s.mu.Unlock()
	for from, want := range map[string]string{
		"%1":        notTheBuilders,                                   // an agent's pane
		"%9@mbtest": "the stoker does not start, stop or rest itself", // the host stoker's
		"%6@mbtest": "the stoker does not start, stop or rest itself", // a chain stoker's
	} {
		for _, console := range []bool{false, true} { // a console hello does not make it a person's
			a := act
			a.From, a.FromPlaced, a.Console = from, true, console
			if res := s.act(t.Context(), a); res.Err != want {
				t.Fatalf("stoker:new from %s (console %v): %q", from, console, res.Err)
			}
			if res := s.newChainAct(t.Context(), a); res.Err != want {
				t.Fatalf("newChainAct from %s (console %v): %q", from, console, res.Err)
			}
		}
	}
	if len(*ran) != 0 || s.unitOfChain("c1") != nil {
		t.Fatalf("ran %q", *ran)
	}
	if res := s.act(t.Context(), act); res.Err != "not started: matchblox cannot tell who starts this. Start from a terminal, a tmux pane or the console." {
		t.Fatalf("an unplaced caller: %q", res.Err)
	}
	if len(*ran) != 0 || s.unitOfChain("c1") != nil {
		t.Fatalf("an unplaced caller started it: %q", *ran)
	}
	act.FromKnown, act.FromPlaced = true, true
	mustAct(t, s, act) // the person's shell
	u := s.unitOfChain("c1")
	if u == nil || u.link.Giver != "person" || !slices.Equal(u.link.Grants, []string{"merge"}) {
		t.Fatalf("unit %+v", u)
	}
}

func TestQuitAChainStoker(t *testing.T) {
	s, ran, _ := chainStokerTest(t)
	u, err := s.startChainStoker(t.Context(), "api-auth", "", nil, "person")
	if err != nil {
		t.Fatal(err)
	}
	*ran = nil
	s.mu.Lock()
	s.cur.Stoker = nil
	s.mu.Unlock()
	if res := s.quitAct(t.Context(), u, proto.Act{Console: true, Confirm: "y"}); res.Err != "" {
		t.Fatal(res.Err)
	}
	if !slices.ContainsFunc(*ran, func(argv []string) bool {
		return slices.Equal(argv, []string{"tmux", "-L", "mbtest", "kill-session", "-t", "=stoker-s1"})
	}) {
		t.Fatalf("ran %q", *ran)
	}
	s.mu.Lock()
	refreshed := s.cur.Stoker != nil
	s.mu.Unlock()
	if !refreshed {
		t.Fatal("the quit did not refresh the view")
	}
	chainStokerUp(s) // an agent of stoker-s1 comes up after the quit
	s.deliverBriefs(t.Context())
	if got := typed(*ran, "%5@mbtest"); len(got) != 0 {
		t.Fatalf("a quit unit's brief was typed: %q", got)
	}
}

// killed tells if ran ended the tmux session of stoker id.
func killed(ran [][]string, id string) bool {
	return slices.ContainsFunc(ran, func(argv []string) bool {
		return slices.Equal(argv, []string{"tmux", "-L", "mbtest", "kill-session", "-t", "=" + stoker.SessionName(id)})
	})
}

func TestStartChainStokerRollsBackAFailedStart(t *testing.T) {
	s, ran, _ := chainStokerTest(t)
	run := s.Actions.Run
	s.Actions.Run = func(argv []string) error {
		if slices.Contains(argv, "new-session") {
			_ = run(nil)
			*ran = append(*ran, argv)
			return errors.New("no server")
		}
		return run(argv)
	}
	if _, err := s.startChainStoker(t.Context(), "api-auth", "", nil, "person"); err == nil || !strings.Contains(err.Error(), "the stoker did not start") {
		t.Fatalf("err %v", err)
	}
	dir := stoker.FamilyDir(s.Stoker.Dir(), "s1")
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) || s.unitOfChain("c1") != nil || s.unitByID("s1") != nil {
		t.Fatalf("the failed unit stays: %v %+v", err, s.family())
	}
	s.Actions.Run = run
	if u, err := s.startChainStoker(t.Context(), "api-auth", "", nil, "person"); err != nil || u.id != "s1" {
		t.Fatalf("a second start: %+v, %v", u, err)
	}
}

func TestStartChainStokerRollsBackAnUnwrittenBrief(t *testing.T) {
	s, ran, _ := chainStokerTest(t)
	root := s.Stoker.Dir()
	run := s.Actions.Run
	s.Actions.Run = func(argv []string) error { // as the agent starts, a dir takes the brief's place
		if slices.Contains(argv, "new-session") {
			if err := os.MkdirAll(filepath.Join(stoker.FamilyDir(root, "s1"), "stoker-brief.md"), 0o700); err != nil {
				t.Error(err)
			}
		}
		return run(argv)
	}
	if _, err := s.startChainStoker(t.Context(), "api-auth", "", nil, "person"); err == nil {
		t.Fatal("a brief that cannot be written started the unit")
	}
	if !killed(*ran, "s1") || s.unitOfChain("c1") != nil || s.unitByID("s1") != nil {
		t.Fatalf("ran %q, family %+v", *ran, s.family())
	}
	if _, err := os.Stat(stoker.FamilyDir(root, "s1")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("its dir stays: %v", err)
	}
	s.Actions.Run = run
	if u, err := s.startChainStoker(t.Context(), "api-auth", "", nil, "person"); err != nil || u.id != "s1" {
		t.Fatalf("a second start: %+v, %v", u, err)
	}
}

func TestMaxChainsUnsetIsTheDefault(t *testing.T) {
	s, _, _ := chainStokerTest(t)
	for _, n := range []int{0, -1} {
		if s.MaxChains = n; s.maxChains() != 4 {
			t.Fatalf("max_chains %d gives %d", n, s.maxChains())
		}
	}
	if _, err := s.startChainStoker(t.Context(), "api-auth", "", nil, "person"); err != nil {
		t.Fatalf("max_chains -1 refuses the first start: %v", err)
	}
	if s.MaxChains = 3; s.maxChains() != 3 {
		t.Fatal("max_chains 3 is not 3")
	}
}

// familyTest is chainStokerTest with c1's repo a real directory the scan
// knows, api-x (%9) in it but out of the chain, the host stoker (%9@mbtest)
// with its run off, and the stoker of c1 (s1, %5@mbtest) up with its run on.
func familyTest(t *testing.T) (*Service, *stokerUnit, *[][]string) {
	t.Helper()
	s, ran, _ := familySetup(t)
	u, err := s.startChainStoker(t.Context(), "api-auth", "", nil, "person")
	if err != nil {
		t.Fatal(err)
	}
	chainStokerUp(s)
	*ran = nil
	return s, u, ran
}

// familySetup is familyTest before the stoker of c1 starts.
func familySetup(t *testing.T) (*Service, *[][]string, *time.Time) {
	t.Helper()
	s, ran, now := chainStokerTest(t)
	tmp, err := filepath.EvalSymlinks(t.TempDir()) // macOS's temp dir is a link
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(tmp, "api")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	s.IsDir = func(string) bool { return true }
	s.TmuxOut = func([]string) (string, error) { return "%42\n", nil }
	s.mu.Lock()
	s.Chains.Chains[0].Repo = repo
	s.cur.Git = &gitscan.Report{Repos: []gitscan.Repo{{Path: repo}, {Path: "/w/app"}}}
	sessions := slices.Clone(s.cur.Sessions)
	s.mu.Unlock()
	for i := range sessions {
		sessions[i].Cwd = repo
	}
	sessions = append(sessions,
		sample.Session{Agent: "claude", Pane: "%9", Target: "x:1.1", SessionID: "s9x", Name: "api-x", Status: "idle", Idle: 5 * time.Minute, Cwd: repo},
		sample.Session{Agent: "claude", Pane: "%9@mbtest", Target: "stoker:1.1", SessionID: "s9", Name: "stoker", Status: "busy", Busy: true})
	s.publish(sample.Snapshot{At: night0, Sessions: sessions}, func(state.Doc) {})
	return s, ran, now
}

// apiRepo is the repo of c1 in a familyTest.
func apiRepo(s *Service) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Chains.Chains[0].Repo
}

func fromChainStoker(id, text string) proto.Act {
	return proto.Act{RecID: id, Confirm: "y", Text: text, From: "%5@mbtest", FromPlaced: true}
}

func TestChainStokerActsOnMatesOnly(t *testing.T) {
	s, _, ran := familyTest(t)
	mustAct(t, s, fromChainStoker("message:%3", "go on"))
	if got := typed(*ran, "%3"); !slices.Equal(got, []string{"go on"}) {
		t.Fatalf("typed into the mate %q", got)
	}
	*ran = nil
	for _, id := range []string{"message:%9", "kill:%9", "rename:%9", "hold:%9", "compact:%9", "message:%1"} {
		res := s.act(t.Context(), fromChainStoker(id, "x"))
		want := "api-x is not in the chain of this stoker."
		if id == "message:%1" {
			want = "work is not in the chain of this stoker."
		}
		if res.Err != want {
			t.Errorf("%s: %q", id, res.Err)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

// Every pane act of the host on a match whose chain stoker runs is refused.
func TestHostRefusedOnOwnedMatch(t *testing.T) {
	s, _, ran := familyTest(t)
	if err := s.Stoker.Set(stoker.Mode{}.TurnOn(night0)); err != nil {
		t.Fatal(err)
	}
	host := s.hostUnit()
	const owned = "api-auth has the stoker of chain api-auth: give it an order."
	for _, p := range []string{"message:", "answer:", proto.PickPrefix, "compact:", "hold:", "release:", "kill:", proto.KeysPrefix, proto.CommandPrefix, "rename:", "switch:"} {
		err := s.orderable(host, proto.Act{RecID: p + "%3", From: "%9@mbtest"})
		if err == nil || err.Error() != owned {
			t.Errorf("%s%%3: %v", p, err)
		}
	}
	if res := s.act(t.Context(), fromStokerAct("message:%3", "go on")); res.Err != owned {
		t.Fatalf("the host's send: %q", res.Err)
	}
	for _, pane := range []string{"%1", "%9"} { // in no chain: the host's
		if err := s.orderable(host, proto.Act{RecID: "message:" + pane, From: "%9@mbtest"}); err != nil {
			t.Errorf("%s: %v", pane, err)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

func TestHostActsWhenChainStokerRests(t *testing.T) {
	s, u, ran := familyTest(t)
	if err := s.Stoker.Set(stoker.Mode{}.TurnOn(night0)); err != nil {
		t.Fatal(err)
	}
	if err := u.store.Set(stoker.Mode{}); err != nil {
		t.Fatal(err)
	}
	if s.ownerOf(sample.Session{Pane: "%3", SessionID: "s3"}) != s.hostUnit() {
		t.Fatal("a chain whose stoker's run is off is not the host's")
	}
	mustAct(t, s, fromStokerAct("message:%3", "go on"))
	if got := typed(*ran, "%3"); !slices.Equal(got, []string{"go on"}) {
		t.Fatalf("typed %q", got)
	}
}

func TestScopeIsCheckedAtActTime(t *testing.T) {
	s, _, _ := familyTest(t)
	x := sample.Session{Pane: "%9", SessionID: "s9x"}
	s.mu.Lock()
	err := s.Chains.Join("c1", memberOf(x))
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	mustAct(t, s, fromChainStoker("message:%9", "go on"))
	s.mu.Lock()
	s.Chains.Leave(memberOf(x))
	s.mu.Unlock()
	if res := s.act(t.Context(), fromChainStoker("message:%9", "again")); res.Err != "api-x is not in the chain of this stoker." {
		t.Fatalf("a match that left: %q", res.Err)
	}
}

func TestScopeFollowsRenamedMember(t *testing.T) {
	s, _, ran := familyTest(t)
	s.mu.Lock()
	sessions := slices.Clone(s.cur.Sessions)
	s.mu.Unlock()
	for i := range sessions {
		if sessions[i].SessionID == "s3" {
			sessions[i].Pane, sessions[i].Target = "%7", "api:2.1"
		}
	}
	s.publish(sample.Snapshot{At: night0, Sessions: sessions}, func(state.Doc) {})
	mustAct(t, s, fromChainStoker("message:%7", "go on"))
	if got := typed(*ran, "%7"); !slices.Equal(got, []string{"go on"}) {
		t.Fatalf("typed %q", got)
	}
}

func newFromChainStoker(t *testing.T, where string) proto.Act {
	t.Helper()
	b, err := json.Marshal(proto.NewSession{Where: where, Agent: "claude", Name: "api-new"})
	if err != nil {
		t.Fatal(err)
	}
	return fromChainStoker(NewSessionID, string(b))
}

// newSample is the sample with the session api-new started in pane, and
// more panes of its tmux session.
func newSample(s *Service, fresh sample.Session, panes ...sample.Pane) sample.Snapshot {
	s.mu.Lock()
	sessions := slices.Clone(s.cur.Sessions)
	s.mu.Unlock()
	sessions = slices.DeleteFunc(sessions, func(x sample.Session) bool { return x.Pane == fresh.Pane })
	if fresh.Pane != "" {
		sessions = append(sessions, fresh)
	}
	return sample.Snapshot{At: night0, Sessions: sessions, Panes: panes}
}

func apiNew(repo, pane, target, id string) sample.Session {
	return sample.Session{Agent: "claude", Pane: pane, Target: target, SessionID: id, Name: "api-new", Status: "idle", Idle: time.Minute, Cwd: repo}
}

func inChain(s *Service, x sample.Session) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, in := s.Chains.ChainOf(memberOf(x))
	return in && c.ID == "c1"
}

func TestChainStokerNewJoinsChain(t *testing.T) {
	s, _, _ := familyTest(t)
	repo := apiRepo(s)
	mustAct(t, s, newFromChainStoker(t, repo))
	fresh := apiNew(repo, "%42", "api-new:1.1", "s42")
	if inChain(s, fresh) {
		t.Fatal("the session joined before a sample found it")
	}
	for range 2 {
		s.publish(newSample(s, fresh, sample.Pane{ID: "%42", Target: "api-new:1.1"}), func(state.Doc) {})
	}
	s.mu.Lock()
	c, _ := s.Chains.ChainOf(memberOf(fresh))
	s.mu.Unlock()
	if !inChain(s, fresh) || len(c.Members) != 3 {
		t.Fatalf("the new session is not in c1 once: %+v", c)
	}
	mustAct(t, s, fromChainStoker("message:%42", "go on"))
}

// Another agent in the tmux session of the start is not the session the
// start made: it does not join.
func TestChainStokerNewJoinsByItsPane(t *testing.T) {
	s, _, _ := familyTest(t)
	repo := apiRepo(s)
	mustAct(t, s, newFromChainStoker(t, repo))
	other := apiNew(repo, "%43", "api-new:1.2", "s43")
	panes := []sample.Pane{{ID: "%42", Target: "api-new:1.1"}, {ID: "%43", Target: "api-new:1.2"}}
	s.publish(newSample(s, other, panes...), func(state.Doc) {})
	if inChain(s, other) {
		t.Fatal("a second agent in the session joined")
	}
	fresh := apiNew(repo, "%42", "api-new:1.1", "s42")
	snap := newSample(s, fresh, panes...)
	snap.Sessions = append(snap.Sessions, other)
	s.publish(snap, func(state.Doc) {})
	if !inChain(s, fresh) || inChain(s, other) {
		t.Fatal("the pane of the start did not join alone")
	}
}

// A pane that is gone, or holds another tmux session, joins nothing later.
func TestChainStokerJoinLetsGoAGonePane(t *testing.T) {
	for name, panes := range map[string][]sample.Pane{
		"gone":  nil,
		"moved": {{ID: "%42", Target: "elsewhere:1.1"}},
	} {
		t.Run(name, func(t *testing.T) {
			s, u, _ := familyTest(t)
			repo := apiRepo(s)
			mustAct(t, s, newFromChainStoker(t, repo))
			s.publish(newSample(s, sample.Session{}, panes...), func(state.Doc) {})
			fresh := apiNew(repo, "%42", "api-new:1.1", "s42")
			s.publish(newSample(s, fresh, sample.Pane{ID: "%42", Target: "api-new:1.1"}), func(state.Doc) {})
			s.mu.Lock()
			n := len(u.joins)
			s.mu.Unlock()
			if inChain(s, fresh) || n != 0 {
				t.Fatalf("joined %v, %d waiting", inChain(s, fresh), n)
			}
		})
	}
}

func TestChainStokerJoinExpires(t *testing.T) {
	s, u, _ := familyTest(t)
	repo := apiRepo(s)
	mustAct(t, s, newFromChainStoker(t, repo))
	pane := sample.Pane{ID: "%42", Target: "api-new:1.1"}
	s.publish(newSample(s, sample.Session{}, pane), func(state.Doc) {}) // no agent yet
	s.mu.Lock()
	n := len(u.joins)
	s.mu.Unlock()
	if n != 1 {
		t.Fatalf("%d joins wait before the window ends", n)
	}
	later := night0.Add(stokerStartsIn)
	s.Now = func() time.Time { return later }
	s.publish(newSample(s, sample.Session{}, pane), func(state.Doc) {})
	fresh := apiNew(repo, "%42", "api-new:1.1", "s42")
	s.publish(newSample(s, fresh, pane), func(state.Doc) {})
	if inChain(s, fresh) {
		t.Fatal("a start past stokerStartsIn joined")
	}
}

func TestChainStokerFailedJoinSays(t *testing.T) {
	s, u, _ := familyTest(t)
	repo := apiRepo(s)
	mustAct(t, s, newFromChainStoker(t, repo))
	fresh := apiNew(repo, "%42", "api-new:1.1", "s42")
	s.mu.Lock()
	s.Chains.SetOff(memberOf(fresh), true)
	s.mu.Unlock()
	s.publish(newSample(s, fresh, sample.Pane{ID: "%42", Target: "api-new:1.1"}), func(state.Doc) {})
	if inChain(s, fresh) {
		t.Fatal("an off session joined")
	}
	if want := "api-new did not join chain api-auth: %42 is off: it takes no chains"; !slices.Contains(cardTexts(u.store), want) {
		t.Fatalf("card %q", cardTexts(u.store))
	}
}

func TestChainStokerNewOutsideRepo(t *testing.T) {
	s, _, ran := familyTest(t)
	repo := apiRepo(s)
	out := t.TempDir()
	if err := os.Symlink(out, filepath.Join(repo, "out")); err != nil {
		t.Fatal(err)
	}
	for _, where := range []string{"/w/app", repo + "-other", "/elsewhere", "dev/api", filepath.Join(repo, "out"), filepath.Join(repo, "missing")} {
		res := s.act(t.Context(), newFromChainStoker(t, where))
		if res.Err != "a session of chain api-auth starts in its repo: "+repo {
			t.Errorf("%s: %q", where, res.Err)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

// compactRec is a rec whose second action types /compact into pane.
func compactRec(id, pane string) advice.Rec {
	return advice.Rec{ID: id, Title: "compact " + pane, Second: &advice.Action{Label: "send /compact",
		Steps: [][]string{{"tmux", "send-keys", "-t", pane, "/compact", "Enter"}}, Destructive: true,
		Guards: []advice.Guard{{IdlePane: pane}}}}
}

func setRecs(s *Service, recs ...advice.Rec) {
	s.mu.Lock()
	s.cur.Recommendations = recs
	s.mu.Unlock()
}

func TestChainStokerActsOnNoRec(t *testing.T) {
	s, _, ran := familyTest(t)
	setRecs(s, compactRec("r9", "%9"), compactRec("r3", "%3"))
	for _, id := range []string{"r9", "r3", "ship:o/r#1", "worktree:/w/wt/a", "orphan:42", "rescan:git"} {
		a := fromChainStoker(id, "")
		a.Which = "secondary"
		if res := s.act(t.Context(), a); res.Err != "a chain stoker acts on the matches of its chain only" {
			t.Errorf("%s: %q", id, res.Err)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

func TestHostRecOnOwnedMatch(t *testing.T) {
	s, _, ran := familyTest(t)
	setRecs(s, compactRec("r3", "%3"), compactRec("r1", "%1"))
	a := fromStokerAct("r3", "")
	a.Which = "secondary"
	if res := s.act(t.Context(), a); res.Err != "api-auth has the stoker of chain api-auth: give it an order." {
		t.Fatalf("the host's rec on api-auth: %q", res.Err)
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
	a.RecID = "r1"
	mustAct(t, s, a)
	if !slices.ContainsFunc(*ran, func(argv []string) bool { return slices.Contains(argv, "%1") && slices.Contains(argv, "/compact") }) {
		t.Fatalf("the host's rec on work did not run: %q", *ran)
	}
}

// A pane the host and the chain stoker both drove speaks for its owner.
func TestTwiceDrivenPaneSpeaksForItsOwner(t *testing.T) {
	s, u, _ := familyTest(t)
	if err := s.Stoker.Set(stoker.Mode{}.TurnOn(night0)); err != nil {
		t.Fatal(err)
	}
	s.drive(s.hostUnit(), "%3")
	s.drive(u, "%3")
	if got, ok := s.agentsUnit(proto.Act{From: "%3"}); !ok || got != u {
		t.Fatalf("%%3 speaks for %+v", got)
	}
	from3 := func(id string) proto.Act {
		return proto.Act{RecID: id, Confirm: "y", Text: "go on", From: "%3", FromPlaced: true}
	}
	mustAct(t, s, from3("message:%4"))
	if res := s.act(t.Context(), from3("message:%9")); res.Err != "api-x is not in the chain of this stoker." {
		t.Fatalf("to a non-member: %q", res.Err)
	}
}

func TestChainStokerGivesNoOrders(t *testing.T) {
	s, _, _ := familyTest(t)
	for _, id := range []string{"stoker:order", "stoker:order-chain"} {
		if res := s.act(t.Context(), fromChainStoker(id, `{"words":"x"}`)); res.Err != "a chain stoker gives no orders" {
			t.Errorf("%s: %q", id, res.Err)
		}
	}
	if res := s.act(t.Context(), fromStokerAct("stoker:order", `{"words":"x"}`)); res.Err != "the stoker does not give itself orders" {
		t.Errorf("the host: %q", res.Err)
	}
}

// A chain stoker reads the whole host: peek reads the doc, and the read
// acts pass the act's gate on a match out of its chain.
func TestChainStokerReadsWholeHost(t *testing.T) {
	s, _, ran := familyTest(t)
	s.mu.Lock()
	doc := s.cur
	s.mu.Unlock()
	i := slices.IndexFunc(doc.Sessions, func(x sample.Session) bool { return x.Name == "api-x" })
	if i < 0 {
		t.Fatal("the doc a chain stoker reads has no api-x")
	}
	if p := stoker.PeekOf(doc.Sessions[i], doc.Queue); p.Name != "api-x" {
		t.Fatalf("peek %+v", p)
	}
	res := s.act(t.Context(), fromChainStoker("switch-preview:%9", ""))
	if res.Err != "" || !strings.Contains(res.Preview, "api-x") {
		t.Fatalf("the preview of api-x: %+v", res)
	}
	if res := s.act(t.Context(), fromChainStoker(NamesAct, "")); res.Err != "" {
		t.Fatalf("names: %q", res.Err)
	}
	if len(typed(*ran, "%9")) != 0 {
		t.Fatal("a read typed into api-x")
	}
}

// hostGiven is familySetup with the host's run on under an order that
// gives hostGoes, and the stoker of c1 started by the host with goes.
func hostGiven(t *testing.T, night bool, hostGoes, goes []string) (*Service, *stokerUnit, *time.Time) {
	t.Helper()
	s, _, now := familySetup(t)
	mustAct(t, s, orderAct(t, "go", night, hostGoes...))
	u, err := s.startChainStoker(t.Context(), "api-auth", "", goes, "host")
	if err != nil {
		t.Fatal(err)
	}
	chainStokerUp(s)
	return s, u, now
}

// drivenGoes are the goes pane carries in unit u's driven.json.
func drivenGoes(t *testing.T, u *stokerUnit, pane string) []string {
	t.Helper()
	d, err := stoker.ReadDriven(u.store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	return d.Panes[pane]
}

func TestGrantsOfHostGivenIsIntersection(t *testing.T) {
	s, u, _ := hostGiven(t, false, []string{"merge", "release"}, []string{"merge"})
	if got := s.grantsOf(u); !slices.Equal(got, []string{"merge"}) {
		t.Fatalf("the unit's goes %q", got)
	}
	if got := s.grantsOf(s.hostUnit()); !slices.Equal(got, []string{"merge", "release"}) {
		t.Fatalf("the host's goes %q", got)
	}
}

func TestGiverLosesGoReachesGuard(t *testing.T) {
	s, u, _ := hostGiven(t, false, []string{"merge", "release"}, []string{"merge"})
	s.drive(u, "%3")
	state := s.Stoker.Dir()
	merge := stoker.Guards("gh pr merge 4")
	if got := drivenGoes(t, u, "%3"); !slices.Equal(got, []string{"merge"}) {
		t.Fatalf("stokers/s1/driven.json gives %%3 %q", got)
	}
	if why := stoker.DenyFamily(state, "", "%3", merge); why != "" {
		t.Fatalf("the merge the host gives is denied: %s", why)
	}
	mustAct(t, s, orderAct(t, "no merges now", false, "release"))
	if got := drivenGoes(t, u, "%3"); len(got) != 0 {
		t.Fatalf("stokers/s1/driven.json still gives %%3 %q", got)
	}
	if why := stoker.DenyFamily(state, "", "%3", merge); why != stoker.DenyNoGo {
		t.Fatalf("the merge the host took back: %q", why)
	}
	mustAct(t, s, orderAct(t, "merge again", false, "merge"))
	if got := drivenGoes(t, u, "%3"); !slices.Equal(got, []string{"merge"}) {
		t.Fatalf("the host's merge given back: %q", got)
	}
}

func TestNightEndClearsHostGivenGos(t *testing.T) {
	s, u, now := hostGiven(t, true, []string{"merge"}, []string{"merge"})
	s.drive(u, "%3")
	if got := drivenGoes(t, u, "%3"); !slices.Equal(got, []string{"merge"}) {
		t.Fatalf("by night %%3 has %q", got)
	}
	*now = s.Stoker.Mode().Until.Add(time.Minute)
	s.stoke(t.Context(), *now)
	if s.Stoker.Mode().On {
		t.Fatal("the night did not end")
	}
	// The night ends the chain's run too: its goes and its driven panes go with it.
	if u.store.Mode().On {
		t.Fatal("the chain's night did not end")
	}
	if got := s.grantsOf(u); len(got) != 0 {
		t.Fatalf("the unit's goes after the night %q", got)
	}
	if _, err := stoker.ReadDriven(u.store.Dir()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stokers/s1/driven.json after the night: %v", err)
	}
}

func TestPersonGivenKeepsItsGos(t *testing.T) {
	s, _, _ := familySetup(t)
	u, err := s.startChainStoker(t.Context(), "api-auth", "", []string{"merge"}, "person")
	if err != nil {
		t.Fatal(err)
	}
	if s.Stoker.Mode().On {
		t.Fatal("the host's run is on")
	}
	s.drive(u, "%3")
	if got := s.grantsOf(u); !slices.Equal(got, []string{"merge"}) {
		t.Fatalf("the unit's goes %q", got)
	}
	if got := drivenGoes(t, u, "%3"); !slices.Equal(got, []string{"merge"}) {
		t.Fatalf("%%3 has %q", got)
	}
}

func TestCheckGiven(t *testing.T) {
	for _, c := range []struct {
		grants, giver []string
		want          string
	}{
		{nil, nil, ""},
		{[]string{"merge"}, []string{"merge", "release"}, ""},
		{[]string{"merge:#12"}, []string{"merge"}, ""},
		{[]string{"merge", "deploy"}, []string{"merge"}, "the order gives deploy, which its giver does not have"},
		{[]string{"merge"}, []string{"merge:#12"}, "the order gives merge, which its giver does not have"},
		{[]string{"kill"}, nil, "the order gives kill, which its giver does not have"},
	} {
		got := ""
		if err := checkGiven(c.grants, c.giver); err != nil {
			got = err.Error()
		}
		if got != c.want {
			t.Errorf("checkGiven(%q, %q) = %q, want %q", c.grants, c.giver, got, c.want)
		}
	}
}

func TestHostGivesNoGoItLacks(t *testing.T) {
	s, _, _ := familySetup(t)
	mustAct(t, s, orderAct(t, "go", false, "release"))
	_, err := s.startChainStoker(t.Context(), "api-auth", "", []string{"merge"}, "host")
	if err == nil || err.Error() != "the order gives merge, which its giver does not have" {
		t.Fatalf("start: %v", err)
	}
	if s.unitOfChain("c1") != nil {
		t.Fatal("the refused start left a unit")
	}
}

// orderChain is stoker:order-chain from pane from ("" for the person's
// shell, placed by its terminal), with no "grants" when grants is nil.
func orderChain(t *testing.T, from, chain, words string, grants []string) proto.Act {
	t.Helper()
	ask := map[string]any{"chain": chain, "words": words}
	if grants != nil {
		ask["grants"] = grants
	}
	b, err := json.Marshal(ask)
	if err != nil {
		t.Fatal(err)
	}
	return proto.Act{RecID: "stoker:order-chain", Confirm: "y", Text: string(b), From: from, FromPlaced: true, FromKnown: from == ""}
}

const hostPane = "%9@mbtest"

func TestHostOrderStartsChainStoker(t *testing.T) {
	s, _, _ := familySetup(t)
	mustAct(t, s, orderAct(t, "go", false, "merge"))
	res := s.act(t.Context(), orderChain(t, hostPane, "api-db", "merge on green", []string{"merge"}))
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	u := s.unitOfChain("c1")
	if u == nil || u.link.Giver != "host" || !slices.Equal(u.link.Grants, []string{"merge"}) {
		t.Fatalf("unit %+v", u)
	}
	if m := u.store.Mode(); !m.On || m.Order == nil || m.Order.Words != "merge on green" {
		t.Fatalf("mode %+v", m)
	}
	card := cardTexts(s.Stoker)
	if !slices.Contains(card, "started the stoker of chain api-auth (s1)") || !slices.Contains(card, "ordered chain api-auth: merge on green") {
		t.Fatalf("host card %q", card)
	}
}

// hostOrdered is hostGiven with the brief of the stoker of c1 typed: its
// agent rests and takes a line at once.
func hostOrdered(t *testing.T, hostGoes, goes []string) (*Service, *stokerUnit, *[][]string) {
	t.Helper()
	s, ran, _ := familySetup(t)
	mustAct(t, s, orderAct(t, "go", false, hostGoes...))
	u, err := s.startChainStoker(t.Context(), "api-auth", "first", goes, "host")
	if err != nil {
		t.Fatal(err)
	}
	chainStokerUp(s)
	s.deliverBriefs(t.Context())
	*ran = nil
	return s, u, ran
}

func TestHostOrderToRunningUnitTypesNewOrder(t *testing.T) {
	s, u, ran := hostOrdered(t, []string{"merge", "release"}, []string{"merge"})
	mustAct(t, s, orderChain(t, hostPane, "api-auth", "merge on green", []string{"release"}))
	if got := typed(*ran, "%5@mbtest"); !slices.Equal(got, []string{"New order: merge on green"}) {
		t.Fatalf("typed %q", got)
	}
	if len(typed(*ran, hostPane)) != 0 {
		t.Fatal("the host stoker was typed into")
	}
	if s.unitByID("s2") != nil {
		t.Fatal("the order started a second unit")
	}
	m := u.store.Mode()
	if m.Order == nil || m.Order.Words != "merge on green" || len(m.Order.Earlier) != 1 || m.Order.Earlier[0].Words != "first" {
		t.Fatalf("order %+v", m.Order)
	}
	// The order's goes replace the unit's: none is cut by the old ones.
	l, err := stoker.ReadLink(u.store.Dir())
	if err != nil || !slices.Equal(l.Grants, []string{"release"}) || !slices.Equal(m.Order.Grants, []string{"release"}) {
		t.Fatalf("link %+v %v, order goes %q", l, err, m.Order.Grants)
	}
	if got := s.grantsOf(u); !slices.Equal(got, []string{"release"}) {
		t.Fatalf("the unit's goes %q", got)
	}
	if !slices.Contains(cardTexts(s.Stoker), "ordered chain api-auth: merge on green") {
		t.Fatalf("host card %q", cardTexts(s.Stoker))
	}
}

func TestOrderReplacesUnitGoesInDriven(t *testing.T) {
	s, u, _ := familyTest(t)
	s.deliverBriefs(t.Context())
	s.drive(u, "%3")
	mustAct(t, s, orderChain(t, "", "api-auth", "release it", []string{"release", "merge"}))
	if got := drivenGoes(t, u, "%3"); !slices.Equal(got, []string{"release", "merge"}) {
		t.Fatalf("%%3 has %q", got)
	}
	if u.link.Giver != "person" {
		t.Fatalf("giver %q", u.link.Giver)
	}
}

func TestHostOrderGivesNoGoItLacks(t *testing.T) {
	s, u, ran := hostOrdered(t, []string{"merge"}, []string{"merge"})
	res := s.act(t.Context(), orderChain(t, hostPane, "api-auth", "deploy", []string{"deploy"}))
	if res.Err != "the order gives deploy, which its giver does not have" {
		t.Fatalf("err %q", res.Err)
	}
	if len(typed(*ran, "%5@mbtest")) != 0 || u.store.Mode().Order.Words != "first" || !slices.Equal(u.link.Grants, []string{"merge"}) {
		t.Fatal("the refused order reached the unit")
	}
}

func TestPersonOrderAtNightIsSealed(t *testing.T) {
	s, ran, _ := familySetup(t)
	mustAct(t, s, orderAct(t, "night", true))
	*ran = nil
	const want = "the night is sealed: answer the stoker's prompts, or give the order in the morning"
	for _, console := range []bool{false, true} {
		a := orderChain(t, "", "api-auth", "go", nil)
		a.Console = console
		if res := s.act(t.Context(), a); res.Err != want {
			t.Fatalf("console %v: %q", console, res.Err)
		}
	}
	if s.unitOfChain("c1") != nil || len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

func TestHostOrderAtNightPasses(t *testing.T) {
	s, _, _ := familySetup(t)
	mustAct(t, s, orderAct(t, "night", true, "merge"))
	mustAct(t, s, orderChain(t, hostPane, "api-auth", "merge on green", nil))
	if u := s.unitOfChain("c1"); u == nil || u.link.Giver != "host" {
		t.Fatalf("unit %+v", u)
	}
}

func TestOrderAtCapIsRefusedAndLogged(t *testing.T) {
	s, _, _ := familySetup(t)
	mustAct(t, s, orderAct(t, "go", false)) // an off host gives no order
	s.MaxChains = 1
	other := stoker.FamilyDir(s.Stoker.Dir(), "s1")
	if err := stoker.WriteLink(other, stoker.Link{ID: "s1", Chain: "c9", Giver: "person"}); err != nil {
		t.Fatal(err)
	}
	st, err := stoker.Open(other)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Set(stoker.Mode{}.TurnOn(night0)); err != nil {
		t.Fatal(err)
	}
	if err := s.loadFamily(); err != nil {
		t.Fatal(err)
	}
	const want = "the family has its 1 chain stokers: max_chains. Quit one on the STOKER tab first."
	if res := s.act(t.Context(), orderChain(t, hostPane, "api-auth", "go", nil)); res.Err != want {
		t.Fatalf("err %q", res.Err)
	}
	card := cardTexts(s.Stoker)
	if !slices.Contains(card, want) || slices.ContainsFunc(card, func(c string) bool { return strings.HasPrefix(c, "ordered chain") }) {
		t.Fatalf("host card %q", card)
	}
	if s.unitOfChain("c1") != nil {
		t.Fatal("the refused order made a unit")
	}
}

func TestHostOrderDefaultsToItsGrants(t *testing.T) {
	s, _, _ := familySetup(t)
	mustAct(t, s, orderAct(t, "go", false, "merge", "release"))
	mustAct(t, s, orderChain(t, hostPane, "api-auth", "go", nil))
	u := s.unitOfChain("c1")
	if u == nil || !slices.Equal(u.link.Grants, []string{"merge", "release"}) {
		t.Fatalf("unit %+v", u)
	}
}

func TestPersonOrderDefaultsToNoGo(t *testing.T) {
	s, _, _ := familySetup(t)
	mustAct(t, s, orderAct(t, "go", false, "merge"))
	mustAct(t, s, orderChain(t, "", "api-auth", "go", nil))
	if u := s.unitOfChain("c1"); u == nil || u.link.Giver != "person" || len(u.link.Grants) != 0 {
		t.Fatalf("unit %+v", u)
	}
}

func TestOrderChainIsNotFromAnAgentsPane(t *testing.T) {
	s, _, ran := familyTest(t)
	for _, console := range []bool{false, true} { // a console hello does not make it a person's
		a := orderChain(t, "%1", "api-auth", "go", nil)
		a.Console = console
		if res := s.act(t.Context(), a); res.Err != notTheBuilders {
			t.Fatalf("console %v: %q", console, res.Err)
		}
	}
	// A session the host stoker drove speaks for it, but gives no order.
	mustAct(t, s, orderAct(t, "go", false))
	*ran = nil
	s.drive(s.hostUnit(), "%1")
	for _, console := range []bool{false, true} {
		a := orderChain(t, "%1", "api-auth", "go", nil)
		a.Console = console
		if res := s.act(t.Context(), a); res.Err != "a session gives no orders: only the person and the host stoker do" {
			t.Fatalf("a driven session (console %v): %q", console, res.Err)
		}
		if res := s.chainOrderAct(t.Context(), a); res.Err != "a session gives no orders: only the person and the host stoker do" {
			t.Fatalf("chainOrderAct from a driven session (console %v): %q", console, res.Err)
		}
	}
	// A caller the service cannot place may be any agent's.
	a := orderChain(t, "", "api-auth", "go", nil)
	a.FromKnown, a.FromPlaced = false, false
	if res := s.act(t.Context(), a); res.Err != "not ordered: matchblox cannot tell who orders this. Order from a terminal, a tmux pane or the console." {
		t.Fatalf("an unplaced caller: %q", res.Err)
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
	if u := s.unitOfChain("c1"); u.store.Mode().Order.Words != "" {
		t.Fatalf("a refused order reached the unit: %q", u.store.Mode().Order.Words)
	}
}

func TestStokerNewAtNightIsSealed(t *testing.T) {
	s, ran, _ := familySetup(t)
	mustAct(t, s, orderAct(t, "night", true))
	*ran = nil
	a := proto.Act{RecID: "stoker:new", Confirm: "y", Text: `{"chain":"api-auth","words":"go"}`, FromKnown: true, FromPlaced: true}
	if res := s.act(t.Context(), a); res.Err != "the night is sealed: answer the stoker's prompts, or give the order in the morning" {
		t.Fatalf("err %q", res.Err)
	}
	if s.unitOfChain("c1") != nil || len(*ran) != 0 {
		t.Fatalf("ran %q", *ran)
	}
}

func TestOrderToAUnitNeedsWords(t *testing.T) {
	s, u, ran := familyTest(t)
	s.deliverBriefs(t.Context())
	*ran = nil
	if res := s.act(t.Context(), orderChain(t, "", "api-auth", "  ", nil)); res.Err != "an order needs words" {
		t.Fatalf("err %q", res.Err)
	}
	if len(typed(*ran, "%5@mbtest")) != 0 || u.store.Mode().Order.Words != "" {
		t.Fatal("the empty order reached the unit")
	}
}

// An order after a dropped brief gives the agent its time anew: a sample
// where it does not rest yet drops nothing.
func TestRequeuedBriefWaitsAnew(t *testing.T) {
	s, ran, now := chainStokerTest(t)
	u, err := s.startChainStoker(t.Context(), "api-auth", "first", nil, "person")
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(stokerStartsIn)
	s.deliverBriefs(t.Context()) // dropped
	mustAct(t, s, orderChain(t, "", "api-auth", "merge on green", nil))
	s.deliverBriefs(t.Context()) // not up: it waits, and nothing drops
	s.mu.Lock()
	waiting := len(u.brief)
	s.mu.Unlock()
	if waiting != 2 {
		t.Fatalf("%d lines wait", waiting)
	}
	chainStokerUp(s)
	s.deliverBriefs(t.Context())
	if got := typed(*ran, "%5@mbtest"); !slices.Equal(got, []string{briefLine(u), "New order: merge on green"}) {
		t.Fatalf("typed %q", got)
	}
}

// A queue that holds an order and is dropped keeps every line, says so on
// both cards, and the next order types them all.
func TestDroppedOrderIsKeptAndSaid(t *testing.T) {
	s, ran, now := chainStokerTest(t)
	u, err := s.startChainStoker(t.Context(), "api-auth", "first", nil, "person")
	if err != nil {
		t.Fatal(err)
	}
	mustAct(t, s, orderChain(t, "", "api-auth", "one", nil))
	*now = now.Add(stokerStartsIn)
	s.deliverBriefs(t.Context())
	const line = "the stoker of chain api-auth did not take its order: its agent did not rest"
	for name, st := range map[string]*stoker.Store{"unit": u.store, "host": s.Stoker} {
		if !slices.Contains(cardTexts(st), line) {
			t.Fatalf("%s card %q", name, cardTexts(st))
		}
	}
	mustAct(t, s, orderChain(t, "", "api-auth", "two", nil))
	chainStokerUp(s)
	s.deliverBriefs(t.Context())
	if got := typed(*ran, "%5@mbtest"); !slices.Equal(got, []string{briefLine(u), "New order: one", "New order: two"}) {
		t.Fatalf("typed %q", got)
	}
}

// A queue whose typing stopped after the brief waits anew for the rest.
func TestPartlyTypedQueueWaitsAnew(t *testing.T) {
	s, ran, now := chainStokerTest(t)
	u, err := s.startChainStoker(t.Context(), "api-auth", "first", nil, "person")
	if err != nil {
		t.Fatal(err)
	}
	mustAct(t, s, orderChain(t, "", "api-auth", "one", nil))
	chainStokerUp(s)
	run := s.Actions.Run
	s.Actions.Run = func(argv []string) error {
		if slices.Contains(argv, "New order: one") {
			_ = run(nil)
			return errors.New("the pane is gone")
		}
		return run(argv)
	}
	*now = now.Add(stokerStartsIn - time.Second)
	s.deliverBriefs(t.Context())
	s.mu.Lock()
	rest, at := slices.Clone(u.brief), u.briefAt
	s.mu.Unlock()
	if !slices.Equal(rest, []string{"New order: one"}) || !at.Equal(*now) {
		t.Fatalf("waits %q since %v", rest, at)
	}
	s.Actions.Run = run
	s.deliverBriefs(t.Context())
	if got := typed(*ran, "%5@mbtest"); !slices.Equal(got, []string{briefLine(u), "New order: one"}) {
		t.Fatalf("typed %q", got)
	}
}

// An order that comes while the brief waits, or after it was dropped,
// waits behind the brief: the agent reads the brief first.
func TestOrderWaitsBehindTheBrief(t *testing.T) {
	for _, dropped := range []bool{false, true} {
		s, ran, now := chainStokerTest(t)
		u, err := s.startChainStoker(t.Context(), "api-auth", "first", nil, "person")
		if err != nil {
			t.Fatal(err)
		}
		brief := "Read " + filepath.Join(u.store.Dir(), "stoker-brief.md") + " and follow it."
		if dropped {
			*now = now.Add(stokerStartsIn)
			s.deliverBriefs(t.Context())
		}
		mustAct(t, s, orderChain(t, "", "api-auth", "merge on green", nil))
		if got := typed(*ran, "%5@mbtest"); len(got) != 0 {
			t.Fatalf("dropped %v: typed before the agent rests: %q", dropped, got)
		}
		chainStokerUp(s)
		s.deliverBriefs(t.Context())
		s.deliverBriefs(t.Context())
		if got := typed(*ran, "%5@mbtest"); !slices.Equal(got, []string{brief, "New order: merge on green"}) {
			t.Fatalf("dropped %v: typed %q", dropped, got)
		}
	}
}

// A unit whose run ended takes an order as a new run, with a new brief.
func TestOrderToAUnitWithNoRunStartsOne(t *testing.T) {
	s, u, ran := familyTest(t)
	s.deliverBriefs(t.Context())
	if err := u.store.Set(stoker.Mode{}); err != nil {
		t.Fatal(err)
	}
	*ran = nil
	mustAct(t, s, orderChain(t, "", "api-auth", "again", []string{"merge"}))
	if s.unitOfChain("c1") != u || s.unitByID("s2") != nil {
		t.Fatal("the order made a second unit")
	}
	m := u.store.Mode()
	if !m.On || m.Order == nil || m.Order.Words != "again" || !slices.Equal(m.Order.Grants, []string{"merge"}) || !slices.Equal(u.link.Grants, []string{"merge"}) {
		t.Fatalf("mode %+v link %+v", m, u.link)
	}
	s.deliverBriefs(t.Context())
	brief := filepath.Join(u.store.Dir(), "stoker-brief.md")
	if got := typed(*ran, "%5@mbtest"); !slices.Equal(got, []string{"Read " + brief + " and follow it."}) {
		t.Fatalf("typed %q", got)
	}
	if b, err := os.ReadFile(brief); err != nil || !strings.Contains(string(b), "again") || !strings.Contains(string(b), "You are the stoker of chain api-auth.") {
		t.Fatalf("brief %v:\n%s", err, b)
	}
}

func consoleStoker(verb string) proto.Act {
	return proto.Act{RecID: "stoker:" + verb, Which: "primary", Console: true}
}

// n puts each chain stoker whose run is on into the night, and the host's
// orders at night still reach it.
func TestNightSealsRunningChainUnits(t *testing.T) {
	s, u, ran := familyTest(t)
	s.deliverBriefs(t.Context())
	mustAct(t, s, consoleStoker("night"))
	host := s.Stoker.Mode()
	if m := u.store.Mode(); !m.On || !m.Nightly || !m.Until.Equal(host.Until) {
		t.Fatalf("the chain's run in the night %+v", m)
	}
	*ran = nil
	mustAct(t, s, orderChain(t, hostPane, "api-auth", "merge on green", nil))
	if got := typed(*ran, "%5@mbtest"); !slices.Equal(got, []string{"New order: merge on green"}) {
		t.Fatalf("typed %q", got)
	}
	if o := u.store.Mode().Order; o == nil || o.Words != "merge on green" {
		t.Fatalf("order %+v", o)
	}
}

// A chain stoker that starts during the night is part of it.
func TestChainStartedAtNightIsANightRun(t *testing.T) {
	s, u, _ := hostGiven(t, true, []string{"merge"}, []string{"merge"})
	m := u.store.Mode()
	if !m.On || !m.Nightly || !m.Until.Equal(s.Stoker.Mode().Until) || m.Order == nil || !m.Order.Night {
		t.Fatalf("mode %+v", m)
	}
	b, err := os.ReadFile(filepath.Join(u.store.Dir(), "stoker-brief.md"))
	if err != nil || !strings.Contains(string(b), "A new order from the host stoker comes as one line") {
		t.Fatalf("brief %v:\n%s", err, b)
	}
}

func TestNightEndStopsTheFamily(t *testing.T) {
	for _, end := range []string{"morning", "n again"} {
		t.Run(end, func(t *testing.T) {
			s, u, _ := familyTest(t)
			mustAct(t, s, consoleStoker("night"))
			switch end {
			case "morning":
				at := s.Stoker.Mode().Until.Add(time.Minute)
				s.Now = func() time.Time { return at }
				s.stoke(t.Context(), at)
			default:
				mustAct(t, s, consoleStoker("off"))
			}
			if s.Stoker.Mode().On {
				t.Fatal("the host's night did not end")
			}
			m := u.store.Mode()
			if m.On || m.Card == nil || !m.Card.Night {
				t.Fatalf("the chain's run after the night %+v", m)
			}
		})
	}
}

func TestMorningCardListsChains(t *testing.T) {
	s, u, _ := familyTest(t)
	mustAct(t, s, consoleStoker("night"))
	at := s.Stoker.Mode().Until.Add(time.Minute)
	s.Now = func() time.Time { return at }
	s.stoke(t.Context(), at)
	c := s.Stoker.Mode().Card
	if c == nil || len(c.Chains) != 1 {
		t.Fatalf("the host's card %+v", c)
	}
	got, own := c.Chains[0], u.store.Mode().Card
	if got.Name != "api-auth" || got.Stoker != "s1" || own == nil || got.Card.Of != own.Of || got.Card.Of != 2 || !got.Card.Ended.Equal(own.Ended) {
		t.Fatalf("the chain on the host's card %+v, its own %+v", got, own)
	}
}

// f by day turns the host stoker on and off, and no chain stoker's run.
func TestFTouchesHostOnly(t *testing.T) {
	s, u, _ := familyTest(t)
	mustAct(t, s, consoleStoker("on"))
	mustAct(t, s, consoleStoker("off"))
	if s.Stoker.Mode().On {
		t.Fatal("the host is on")
	}
	if m := u.store.Mode(); !m.On || m.Nightly || m.Card != nil {
		t.Fatalf("f touched the chain's run %+v", m)
	}
}

// hostWorks is familySetup with the host's run on: it drives api-auth,
// holds api-db and sets a trigger on each of api-auth and work.
func hostWorks(t *testing.T) *Service {
	t.Helper()
	s, _, _ := familySetup(t)
	hs, err := holds.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.Holds = hs
	mustAct(t, s, orderAct(t, "go", false))
	mustAct(t, s, fromStokerAct("message:%3", "go on"))
	mustAct(t, s, fromStokerAct("hold:%4", "wait for the merge"))
	mustAct(t, s, whenAct(`{"rest":"api-auth","do":"send","who":"api-auth","text":"rebase on main"}`))
	mustAct(t, s, whenAct(`{"rest":"api-auth","do":"compact","who":"work"}`))
	return s
}

func TestHandOverMovesHoldsAndTriggers(t *testing.T) {
	s := hostWorks(t)
	if _, in := drivenPanes(t, s.hostUnit())["%3"]; !in {
		t.Fatal("the host does not drive api-auth")
	}
	u, err := s.startChainStoker(t.Context(), "api-auth", "", nil, "person")
	if err != nil {
		t.Fatal(err)
	}
	chainStokerUp(s)
	if tr := s.Stoker.Mode().Triggers; len(tr) != 1 || tr[0].Who != "work" {
		t.Fatalf("the host's triggers %+v", tr)
	}
	if tr := u.store.Mode().Triggers; len(tr) != 1 || tr[0].Who != "api-auth" || tr[0].ID != "t1" || tr[0].Text != "rebase on main" {
		t.Fatalf("the chain's triggers %+v", tr)
	}
	if _, in := drivenPanes(t, s.hostUnit())["%3"]; in {
		t.Fatal("the host still drives api-auth")
	}
	// The hold on api-db is the chain stoker's now: it lets it go, the host does not.
	const owned = "api-db has the stoker of chain api-auth: give it an order."
	if res := s.act(t.Context(), fromStokerAct("release:%4", "")); res.Err != owned {
		t.Fatalf("the host's release: %q", res.Err)
	}
	mustAct(t, s, fromChainStoker("release:%4", ""))
	if _, held := s.heldOf("s4"); held {
		t.Fatal("api-db is still held")
	}
}

func TestHandOverDropsWaitingKillWithLine(t *testing.T) {
	s := hostWorks(t)
	if res := s.act(t.Context(), fromStokerAct("kill:%4", "")); !strings.Contains(res.Err, "the kill waits") {
		t.Fatalf("the host's kill: %q", res.Err)
	}
	if _, err := s.startChainStoker(t.Context(), "api-auth", "", nil, "person"); err != nil {
		t.Fatal(err)
	}
	run := s.Stoker.View().Run
	last := stoker.LastGo(run, stoker.Entry{Pane: hostPane, Needs: "kill:%4"})
	if last.State != stoker.StepFailed || last.Text != "dropped the kill for api-db: the stoker of chain api-auth owns it now" {
		t.Fatalf("the kill on the host's card %+v", last)
	}
	if err := s.grant(s.hostUnit(), last.ID); err == nil {
		t.Fatal("a y still gives the dropped kill")
	}
}

// drivenPanes are the panes in unit u's driven.json.
func drivenPanes(t *testing.T, u *stokerUnit) map[string][]string {
	t.Helper()
	d, err := stoker.ReadDriven(u.store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	return d.Panes
}

// The host's brief lists each chain, its stoker and that stoker's plan.
func TestHostBriefListsChainStokers(t *testing.T) {
	s, u, _ := familyTest(t)
	mustAct(t, s, fromChainStoker("stoker:plan", `{"steps":[{"n":1,"session":"api-auth","text":"merge on green"},{"n":2,"session":"api-db","text":"rebase"}]}`))
	mustAct(t, s, fromChainStoker("stoker:step", `{"step":1,"state":"done","text":"merged"}`))
	mustAct(t, s, orderAct(t, "night", true))
	if m := u.store.Mode(); !m.Nightly || !m.Until.Equal(s.Stoker.Mode().Until) {
		t.Fatalf("the night the launch sheet starts left the chain out: %+v", m)
	}
	b, err := os.ReadFile(filepath.Join(s.Stoker.Dir(), "stoker-brief.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"- api-auth: stoker s1 · night · plan 1/2", `Give a chain an order: matchblox stoker order <chain> "<text>"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the host's brief lacks %q:\n%s", want, b)
		}
	}
}

// A chain stoker whose agent is gone says how to start it, and an order
// starts it again.
func TestChainStokerNotRunningSaysTheOrder(t *testing.T) {
	s, u, ran := familyTest(t)
	s.deliverBriefs(t.Context())
	s.mu.Lock()
	sessions := slices.DeleteFunc(slices.Clone(s.cur.Sessions), func(x sample.Session) bool { return x.Pane == "%5@mbtest" })
	s.mu.Unlock()
	s.publish(sample.Snapshot{At: night0, Sessions: sessions}, func(state.Doc) {})
	_ = s.Actions.Run([]string{"tmux", "kill-session"}) // the test's tmux loses stoker-s1
	const want = `the stoker of chain api-auth is not running: matchblox stoker order api-auth "<text>" starts it`
	if _, err := s.stokerAgent(u); err == nil || err.Error() != want {
		t.Fatalf("stokerAgent: %v", err)
	}
	*ran = nil
	mustAct(t, s, orderChain(t, "", "api-auth", "carry on", nil))
	if !slices.ContainsFunc(*ran, func(argv []string) bool {
		return slices.Contains(argv, "new-session") && slices.Contains(argv, "stoker-s1")
	}) {
		t.Fatalf("the order did not start the agent: %q", *ran)
	}
	chainStokerUp(s)
	*ran = nil
	s.deliverBriefs(t.Context())
	brief := filepath.Join(u.store.Dir(), "stoker-brief.md")
	if got := typed(*ran, "%5@mbtest"); !slices.Equal(got, []string{"Read " + brief + " and follow it.", "New order: carry on"}) {
		t.Fatalf("typed %q", got)
	}
}

// twoChains is familySetup with a second chain, c2 of api-ui (%7) and
// api-ci (%8), in the repo of c1.
func twoChains(t *testing.T) (*Service, *[][]string) {
	t.Helper()
	s, ran, _ := familySetup(t)
	repo := apiRepo(s)
	ui := sample.Session{Agent: "claude", Pane: "%7", Target: "ui:1.1", SessionID: "s7", Name: "api-ui", Status: "idle", Idle: 5 * time.Minute, Cwd: repo}
	ci := sample.Session{Agent: "claude", Pane: "%8", Target: "ui:1.2", SessionID: "s8", Name: "api-ci", Status: "idle", Idle: 5 * time.Minute, Cwd: repo}
	s.mu.Lock()
	s.Chains.Chains = append(s.Chains.Chains, chains.Chain{ID: "c2", Repo: repo, Members: []chains.Member{memberOf(ui), memberOf(ci)}})
	sessions := append(slices.Clone(s.cur.Sessions), ui, ci)
	s.mu.Unlock()
	s.publish(sample.Snapshot{At: night0, Sessions: sessions}, func(state.Doc) {})
	return s, ran
}

// stokerUp is the sample once the agent of stoker id rests in pane.
func stokerUp(s *Service, id, pane string) {
	s.mu.Lock()
	sessions := slices.DeleteFunc(slices.Clone(s.cur.Sessions), func(x sample.Session) bool { return x.Pane == pane })
	s.mu.Unlock()
	name := stoker.SessionName(id)
	sessions = append(sessions, sample.Session{Agent: "claude", Pane: pane, Target: name + ":1.1", SessionID: "agent-" + id, Name: name, Status: "idle"})
	s.publish(sample.Snapshot{At: night0, Sessions: sessions}, func(state.Doc) {})
}

// withoutPane is the sample with the session in pane gone.
func withoutPane(s *Service, pane string) {
	s.mu.Lock()
	sessions := slices.DeleteFunc(slices.Clone(s.cur.Sessions), func(x sample.Session) bool { return x.Pane == pane })
	s.mu.Unlock()
	s.publish(sample.Snapshot{At: night0, Sessions: sessions}, func(state.Doc) {})
}

// joinChains joins c1 into c2: api-auth asks api-ui, and api-ui asks back.
func joinChains(t *testing.T, s *Service) {
	t.Helper()
	mustAct(t, s, proto.Act{RecID: proto.ChainPrefix + "%7", From: "%3"})
	mustAct(t, s, proto.Act{RecID: proto.ChainPrefix + "%3", From: "%7"})
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Chains.Chains) != 1 || s.Chains.Chains[0].ID != "c2" || len(s.Chains.Chains[0].Members) != 4 {
		t.Fatalf("the chains did not join into c2: %+v", s.Chains.Chains)
	}
}

func count(texts []string, text string) int {
	n := 0
	for _, x := range texts {
		if x == text {
			n++
		}
	}
	return n
}

func TestChainEndQuitsItsStoker(t *testing.T) {
	s, u, ran := familyTest(t)
	s.deliverBriefs(t.Context())
	*ran = nil
	withoutPane(s, "%4") // api-db is gone: c1 has one match, and ends
	s.followChains(t.Context())
	if got := count(cardTexts(u.store), "its chain ended"); got != 1 {
		t.Fatalf("the card says its chain ended %d times: %q", got, cardTexts(u.store))
	}
	if u.store.Mode().On {
		t.Fatal("the run of a chain that ended is on")
	}
	if !killed(*ran, "s1") {
		t.Fatalf("the agent of stoker-s1 did not quit: %q", *ran)
	}
	if s.unitByID("s1") != u {
		t.Fatal("the unit left the family: its card is lost")
	}
	if _, err := os.Stat(u.store.Dir()); err != nil {
		t.Fatalf("its dir: %v", err)
	}
	withoutPane(s, "%5@mbtest")
	s.followChains(t.Context())
	if n := s.chainsAtWork(); n != 0 {
		t.Fatalf("the cap counts %d chain stokers", n)
	}
	if got := count(cardTexts(u.store), "its chain ended"); got != 1 {
		t.Fatalf("the card says its chain ended %d times", got)
	}
}

func TestChainLeftByAllButOneQuitsItsStoker(t *testing.T) {
	s, u, ran := familyTest(t)
	*ran = nil
	mustAct(t, s, proto.Act{RecID: proto.ChainPrefix + "-", From: "%4"})
	if !slices.Contains(cardTexts(u.store), "its chain ended") || u.store.Mode().On || !killed(*ran, "s1") {
		t.Fatalf("card %q, mode %+v, ran %q", cardTexts(u.store), u.store.Mode(), *ran)
	}
}

func TestJoinKeepsTheOnlyStokerAndRelinks(t *testing.T) {
	s, ran := twoChains(t)
	u, err := s.startChainStoker(t.Context(), "api-auth", "", nil, "person")
	if err != nil {
		t.Fatal(err)
	}
	stokerUp(s, "s1", "%5@mbtest")
	s.deliverBriefs(t.Context())
	*ran = nil
	joinChains(t, s)
	if got := s.chainIDOf(u); got != "c2" {
		t.Fatalf("the unit's chain is %q", got)
	}
	if l, err := stoker.ReadLink(u.store.Dir()); err != nil || l.Chain != "c2" {
		t.Fatalf("link %+v %v", l, err)
	}
	if s.unitOfChain("c2") != u || s.unitOfChain("c1") != nil {
		t.Fatal("the joined chain is not the unit's")
	}
	if !u.store.Mode().On || killed(*ran, "s1") {
		t.Fatal("the only stoker of the joined chain stopped")
	}
	if slices.ContainsFunc(cardTexts(u.store), func(x string) bool { return strings.HasPrefix(x, "its chain") }) {
		t.Fatalf("card %q", cardTexts(u.store))
	}
	mustAct(t, s, fromChainStoker("message:%8", "go on")) // api-ci is its chain's now
}

// twoStokers is twoChains with a stoker for each chain: s1 for c1 and s2
// for c2, their agents up and briefed.
func twoStokers(t *testing.T, words1 string, goes1 []string, words2 string, goes2 []string) (*Service, *stokerUnit, *stokerUnit, *[][]string) {
	t.Helper()
	s, ran := twoChains(t)
	a, err := s.startChainStoker(t.Context(), "api-auth", words1, goes1, "person")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.startChainStoker(t.Context(), "api-ui", words2, goes2, "person")
	if err != nil {
		t.Fatal(err)
	}
	stokerUp(s, "s1", "%5@mbtest")
	stokerUp(s, "s2", "%6@mbtest")
	s.deliverBriefs(t.Context())
	*ran = nil
	return s, a, b, ran
}

func TestJoinOfTwoStokersKeepsSurvivor(t *testing.T) {
	s, a, b, ran := twoStokers(t, "", nil, "", nil)
	joinChains(t, s)
	if got := count(cardTexts(a.store), "its chain joined api-ui"); got != 1 {
		t.Fatalf("the absorbed card %q", cardTexts(a.store))
	}
	if a.store.Mode().On || !killed(*ran, "s1") {
		t.Fatal("the stoker of the absorbed chain goes on")
	}
	if !b.store.Mode().On || killed(*ran, "s2") || s.unitOfChain("c2") != b || s.chainIDOf(b) != "c2" {
		t.Fatal("the stoker of the surviving chain stopped")
	}
	if s.unitByID("s1") != a {
		t.Fatal("the absorbed unit left the family: its card is lost")
	}
	if got := typed(*ran, "%6@mbtest"); len(got) != 0 {
		t.Fatalf("an order with no words was carried: %q", got)
	}
	if res := s.act(t.Context(), fromChainStoker("message:%4", "x")); res.Err == "" {
		t.Fatal("the absorbed stoker still acts on api-db")
	}
	mustAct(t, s, proto.Act{RecID: "message:%4", Confirm: "y", Text: "go on", From: "%6@mbtest", FromPlaced: true})
}

func TestJoinCarriesOrderWithCommonGos(t *testing.T) {
	s, _, b, ran := twoStokers(t, "ship the auth fix", []string{"merge", "release"}, "keep ci green", []string{"merge", "deploy"})
	joinChains(t, s)
	if got := typed(*ran, "%6@mbtest"); !slices.Equal(got, []string{"New order (from the joined chain api-auth): ship the auth fix"}) {
		t.Fatalf("typed into the survivor %q", got)
	}
	m := b.store.Mode()
	if m.Order == nil || m.Order.Words != "ship the auth fix" || !slices.Equal(m.Order.Grants, []string{"merge"}) {
		t.Fatalf("the survivor's order %+v", m.Order)
	}
	if got := s.grantsOf(b); !slices.Equal(got, []string{"merge"}) {
		t.Fatalf("the survivor's goes %q", got)
	}
}

// A survivor whose run is off has no goes: the carried order starts its
// run with none.
func TestJoinStartsTheSurvivorsRunThatIsOff(t *testing.T) {
	s, _, b, _ := twoStokers(t, "ship the auth fix", []string{"merge", "release"}, "keep ci green", []string{"merge", "deploy"})
	if err := b.store.Set(stoker.Mode{}); err != nil {
		t.Fatal(err)
	}
	joinChains(t, s)
	m := b.store.Mode()
	if !m.On || m.Order == nil || m.Order.Words != "ship the auth fix" || len(m.Order.Grants) != 0 {
		t.Fatalf("the survivor's run %+v, order %+v", m, m.Order)
	}
	if got := s.grantsOf(b); len(got) != 0 {
		t.Fatalf("the survivor's goes %q", got)
	}
	b2, err := os.ReadFile(filepath.Join(b.store.Dir(), "stoker-brief.md"))
	if err != nil || !strings.Contains(string(b2), "ship the auth fix") || !strings.Contains(string(b2), "You are the stoker of chain api-ui.") {
		t.Fatalf("brief %v:\n%s", err, b2)
	}
}

// reuseC1 ends c1 (api-db is gone) and forms a new chain of the same id,
// of api-auth and api-x.
func reuseC1(t *testing.T, s *Service) {
	t.Helper()
	withoutPane(s, "%4")
	s.followChains(t.Context())
	s.mu.Lock()
	var auth, x sample.Session
	for _, y := range s.cur.Sessions {
		switch y.Pane {
		case "%3":
			auth = y
		case "%9":
			x = y
		}
	}
	s.Chains.Chains = append(s.Chains.Chains, chains.Chain{ID: "c1", Repo: s.Chains.Chains[0].Repo, Members: []chains.Member{memberOf(auth), memberOf(x)}})
	s.mu.Unlock()
}

func TestAnEndedStokerTakesNoNewChainOfItsID(t *testing.T) {
	s, ran := twoChains(t)
	old, err := s.startChainStoker(t.Context(), "api-auth", "", nil, "person")
	if err != nil {
		t.Fatal(err)
	}
	stokerUp(s, "s1", "%5@mbtest")
	reuseC1(t, s)
	if l, err := stoker.ReadLink(old.store.Dir()); err != nil || !l.Ended {
		t.Fatalf("the ended unit's link %+v %v", l, err)
	}
	card := cardTexts(old.store)
	if s.unitOfChain("c1") != nil {
		t.Fatal("the new c1 has the ended stoker")
	}
	u, err := s.startChainStoker(t.Context(), "api-auth", "", nil, "person")
	if err != nil || u.id != "s2" {
		t.Fatalf("the start: %v", err)
	}
	stokerUp(s, "s2", "%6@mbtest")
	s.deliverBriefs(t.Context())
	*ran = nil
	joinChains(t, s)
	if s.unitOfChain("c2") != u || s.chainIDOf(u) != "c2" || !u.store.Mode().On {
		t.Fatal("the join did not keep the live stoker")
	}
	if got := cardTexts(old.store); !slices.Equal(got, card) {
		t.Fatalf("the ended unit's card changed: %q", got)
	}
	if s.chainIDOf(old) != "c1" {
		t.Fatalf("the ended unit moved to %q", s.chainIDOf(old))
	}
}

func TestJoinAtTheCapCarriesTheOrder(t *testing.T) {
	s, _, b, _ := twoStokers(t, "ship the auth fix", []string{"merge"}, "keep ci green", nil)
	s.MaxChains = 2
	if err := b.store.Set(stoker.Mode{}); err != nil {
		t.Fatal(err)
	}
	if n := s.chainsAtWork(); n != 2 {
		t.Fatalf("the cap counts %d", n)
	}
	joinChains(t, s)
	if m := b.store.Mode(); !m.On || m.Order == nil || m.Order.Words != "ship the auth fix" {
		t.Fatalf("the survivor's run %+v; card %q", m, cardTexts(b.store))
	}
}

func TestJoinAtNightWaitsForTheMorning(t *testing.T) {
	s, _, b, ran := twoStokers(t, "ship the auth fix", nil, "keep ci green", nil)
	mustAct(t, s, orderAct(t, "night", true))
	if !b.store.Mode().Nightly {
		t.Fatal("the survivor's run is no night run")
	}
	*ran = nil
	joinChains(t, s)
	if got := typed(*ran, "%6@mbtest"); len(got) != 0 {
		t.Fatalf("typed into a sealed night %q", got)
	}
	if !slices.Contains(cardTexts(b.store), "the order of the joined chain api-auth waits for the morning: ship the auth fix") {
		t.Fatalf("card %q", cardTexts(b.store))
	}
	if m := b.store.Mode(); m.Order == nil || m.Order.Words != "keep ci green" {
		t.Fatalf("the survivor's order %+v", m.Order)
	}
}

func TestChainEndDropsItsWaitingKill(t *testing.T) {
	s, u, _ := familyTest(t)
	s.deliverBriefs(t.Context())
	if res := s.act(t.Context(), fromChainStoker("kill:%4", "")); !strings.Contains(res.Err, "the kill waits") {
		t.Fatalf("the chain stoker's kill: %q", res.Err)
	}
	withoutPane(s, "%3") // c1 ends; api-db stays
	s.followChains(t.Context())
	last := stoker.LastGo(u.store.View().Run, stoker.Entry{Pane: "%5@mbtest", Needs: "kill:%4"})
	if last.State != stoker.StepFailed || last.Text != "dropped the kill for api-db: its chain ended" {
		t.Fatalf("the kill on the card %+v", last)
	}
	if err := s.grant(u, last.ID); err == nil {
		t.Fatal("a y still gives the dropped kill")
	}
}

func TestJoinIntoTheOnlyStokerTakesTheHostsWork(t *testing.T) {
	s, _ := twoChains(t)
	mustAct(t, s, orderAct(t, "go", false))
	mustAct(t, s, fromStokerAct("message:%3", "go on"))
	if _, in := drivenPanes(t, s.hostUnit())["%3"]; !in {
		t.Fatal("the host does not drive api-auth")
	}
	u, err := s.startChainStoker(t.Context(), "api-ui", "", nil, "person")
	if err != nil {
		t.Fatal(err)
	}
	stokerUp(s, "s1", "%5@mbtest")
	joinChains(t, s)
	if s.unitOfChain("c2") != u || s.chainIDOf(u) != "c2" || !u.store.Mode().On {
		t.Fatal("the survivor's stoker changed")
	}
	if _, in := drivenPanes(t, s.hostUnit())["%3"]; in {
		t.Fatal("the host still drives api-auth")
	}
	mustAct(t, s, fromChainStoker("message:%3", "go on"))
}

func TestChainOffEndsItsStoker(t *testing.T) {
	s, u, ran := familyTest(t)
	*ran = nil
	mustAct(t, s, proto.Act{RecID: proto.ChainPrefix + "off", From: "%4"})
	if !slices.Contains(cardTexts(u.store), "its chain ended") || u.store.Mode().On || !killed(*ran, "s1") {
		t.Fatalf("card %q, mode %+v, ran %q", cardTexts(u.store), u.store.Mode(), *ran)
	}
}

func TestStateCarriesChainStokers(t *testing.T) {
	s, _, _ := familySetup(t)
	if st := s.State().Stokers; st != nil {
		t.Fatalf("stokers without a chain stoker: %+v", st)
	}
	u, err := s.startChainStoker(t.Context(), "api-auth", "", nil, "person")
	if err != nil {
		t.Fatal(err)
	}
	chainStokerUp(s)
	got := s.State().Stokers
	if len(got) != 1 || got[0].ID != "s1" || got[0].Chain != "c1" || got[0].Name != "api-auth" || !got[0].View.On || !got[0].View.PaneUp {
		t.Fatalf("stokers %+v", got)
	}
	if h := s.State().Stoker; h == nil || h.Start == got[0].View.Start {
		t.Fatalf("the host view %+v", h)
	}

	s.fam.mu.Lock()
	u.link.Ended = true
	s.Chains.Chains = nil
	s.fam.mu.Unlock()
	s.refreshStoker()
	got = s.State().Stokers
	if len(got) != 1 || got[0].Name != "api-auth" {
		t.Fatalf("an ended unit keeps its card and its last name: %+v", got)
	}
}

func TestRetiredUnitKeepsItsChainNameAcrossARestart(t *testing.T) {
	s, _, _ := chainStokerTest(t)
	u, err := s.startChainStoker(t.Context(), "api-auth", "", nil, "person")
	if err != nil {
		t.Fatal(err)
	}
	s.fam.mu.Lock()
	s.Chains.Chains = nil // the chain is gone before the retire
	s.fam.mu.Unlock()
	s.retire(t.Context(), u, "its chain ended", "its chain ended")

	again := newTest(t, time.Hour)
	var openErr error
	if again.Stoker, openErr = stoker.Open(s.Stoker.Dir()); openErr != nil {
		t.Fatal(openErr)
	}
	if err := again.loadFamily(); err != nil {
		t.Fatal(err)
	}
	got := again.unitByID("s1")
	if got == nil || !got.link.Ended || again.unitName(got) != "api-auth" {
		t.Fatalf("name %q, unit %+v", again.unitName(got), got)
	}
}

// TestQuitQuitsOneUnit: stoker:quit with a stoker's id, or its chain by a
// member's name, ends that unit's agent only. The person sends it, at a
// console or in a shell of their own; an agent's pane never does.
func TestQuitQuitsOneUnit(t *testing.T) {
	s, ran := twoChains(t)
	for _, who := range []string{"api-auth", "api-ui"} {
		if _, err := s.startChainStoker(t.Context(), who, "", nil, "person"); err != nil {
			t.Fatal(err)
		}
	}
	s.Actions.Run = func(argv []string) error { // both agents stay up
		*ran = append(*ran, argv)
		return nil
	}
	quit := func(a proto.Act) proto.Result {
		*ran = nil
		a.RecID, a.Confirm = "stoker:quit", "y"
		return s.act(t.Context(), a)
	}
	if res := quit(proto.Act{Console: true, Text: "s2"}); res.Err != "" || res.Note != "ended the stoker of chain api-ui (s2) and its tmux session" {
		t.Fatalf("the quit of s2: %+v", res)
	}
	if !killed(*ran, "s2") || killed(*ran, "s1") || slices.ContainsFunc(*ran, func(argv []string) bool { return slices.Contains(argv, "=stoker") }) {
		t.Fatalf("the quit of s2 ran %q", *ran)
	}
	shell := proto.Act{From: "%40", FromKnown: true, FromPlaced: true, Text: "api-db"}
	if res := quit(shell); res.Err != "" || !killed(*ran, "s1") || killed(*ran, "s2") {
		t.Fatalf("matchblox stoker quit --chain api-db: %+v, ran %q", res, *ran)
	}
	if res := quit(proto.Act{Console: true, Text: "s9"}); res.Err != "no stoker s9: matchblox status lists the family" {
		t.Fatalf("an unknown stoker: %q", res.Err)
	}
	for _, from := range []string{"%3", "%9@mbtest"} { // a match's pane, the host stoker's
		if res := quit(proto.Act{From: from, FromKnown: true, FromPlaced: true, Text: "s1"}); res.Err == "" || killed(*ran, "s1") {
			t.Fatalf("the quit from %s ran %q", from, *ran)
		}
	}
	if res := quit(proto.Act{Text: "s1"}); res.Err == "" || killed(*ran, "s1") {
		t.Fatalf("a quit no one placed ran %q", *ran)
	}
}

// C1: a member the host drove stays driven through the handover: the
// chain stoker drives it from the start, so no merge go opens.
func TestHandOverKeepsTheGuardOnAMember(t *testing.T) {
	s := hostWorks(t)
	merge := stoker.Guards("gh pr merge 4")
	state := s.Stoker.Dir()
	if why := stoker.DenyFamily(state, "", "%3", merge); why != stoker.DenyNoGo {
		t.Fatalf("before the handover: %q", why)
	}
	u, err := s.startChainStoker(t.Context(), "api-auth", "", nil, "person")
	if err != nil {
		t.Fatal(err)
	}
	if why := stoker.DenyFamily(state, "", "%3", merge); why != stoker.DenyNoGo {
		t.Fatalf("after the handover: %q", why)
	}
	if _, in := drivenPanes(t, u)["%3"]; !in {
		t.Fatal("the chain stoker does not drive api-auth")
	}
	if _, in := drivenPanes(t, s.hostUnit())["%3"]; in {
		t.Fatal("the host still drives api-auth")
	}
}

// C1: a session the host drove that joins a chain whose stoker runs is
// that stoker's to drive at once.
func TestJoinHandsOverAHostDrivenSession(t *testing.T) {
	s, u, _ := familyTest(t)
	if err := s.Stoker.Set(stoker.Mode{}.TurnOn(night0)); err != nil {
		t.Fatal(err)
	}
	s.drive(s.hostUnit(), "%42")
	repo := apiRepo(s)
	mustAct(t, s, newFromChainStoker(t, repo))
	fresh := apiNew(repo, "%42", "api-new:1.1", "s42")
	merge := stoker.Guards("gh pr merge 4")
	for range 2 {
		s.publish(newSample(s, fresh, sample.Pane{ID: "%42", Target: "api-new:1.1"}), func(state.Doc) {})
	}
	if !inChain(s, fresh) {
		t.Fatal("api-new did not join")
	}
	if why := stoker.DenyFamily(s.Stoker.Dir(), "", "%42", merge); why != stoker.DenyNoGo {
		t.Fatalf("after the join: %q", why)
	}
	if _, in := drivenPanes(t, u)["%42"]; !in {
		t.Fatal("the chain stoker does not drive api-new")
	}
	if _, in := drivenPanes(t, s.hostUnit())["%42"]; in {
		t.Fatal("the host still drives api-new")
	}
}

// I1: a chain quit ends the unit's run, so its slot under max_chains is free.
func TestChainQuitEndsItsRunAndFreesTheCap(t *testing.T) {
	s, ran := twoChains(t)
	s.MaxChains = 1
	if _, err := s.startChainStoker(t.Context(), "api-auth", "", nil, "person"); err != nil {
		t.Fatal(err)
	}
	stokerUp(s, "s1", "%5@mbtest")
	s.Actions.Run = func(argv []string) error {
		*ran = append(*ran, argv)
		return nil
	}
	res := s.act(t.Context(), proto.Act{RecID: "stoker:quit", Confirm: "y", Console: true, Text: "s1"})
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	u := s.unitByID("s1")
	if m := u.store.Mode(); m.On || m.Card == nil {
		t.Fatalf("the quit left the run on: %+v", m)
	}
	if _, err := stoker.ReadDriven(u.store.Dir()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("driven.json after the quit: %v", err)
	}
	withoutPane(s, "%5@mbtest")
	if n := s.chainsAtWork(); n != 0 {
		t.Fatalf("%d chains at work after the quit", n)
	}
	if _, err := s.startChainStoker(t.Context(), "api-ui", "", nil, "person"); err != nil {
		t.Fatalf("a new stoker after the quit: %v", err)
	}
}

// I1: a chain stoker whose agent is gone, and whose run is on, quits too:
// its run ends.
func TestChainQuitWithNoSessionEndsItsRun(t *testing.T) {
	s, ran := twoChains(t)
	if _, err := s.startChainStoker(t.Context(), "api-auth", "", nil, "person"); err != nil {
		t.Fatal(err)
	}
	s.Actions.Run = func(argv []string) error {
		*ran = append(*ran, argv)
		if slices.Contains(argv, "has-session") {
			return errors.New("no session")
		}
		return nil
	}
	*ran = nil
	res := s.act(t.Context(), proto.Act{RecID: "stoker:quit", Confirm: "y", Console: true, Text: "s1"})
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	if res.Note != "ended the stoker of chain api-auth (s1): its tmux session is gone" {
		t.Fatalf("note %q", res.Note)
	}
	if s.unitByID("s1").store.Mode().On || killed(*ran, "s1") {
		t.Fatalf("mode %+v, ran %q", s.unitByID("s1").store.Mode(), *ran)
	}
	if res := s.act(t.Context(), proto.Act{RecID: "stoker:quit", Confirm: "y", Console: true}); res.Err != "the stoker is not running" {
		t.Fatalf("the host's quit with no agent: %q", res.Err)
	}
}

// I2: a host stoker that is off, or rests, starts no chain stoker.
func TestAnOffOrRestingHostGivesNoChainOrder(t *testing.T) {
	s, _, _ := familySetup(t)
	if res := s.act(t.Context(), orderChain(t, hostPane, "api-db", "merge on green", nil)); res.Err != "the stoker is off: no run" {
		t.Fatalf("an off host's order: %q", res.Err)
	}
	mustAct(t, s, orderAct(t, "go", false))
	mustAct(t, s, consoleStoker("rest"))
	if res := s.act(t.Context(), orderChain(t, hostPane, "api-db", "merge on green", nil)); res.Err != "the stoker rests: a person works" {
		t.Fatalf("a resting host's order: %q", res.Err)
	}
	if s.unitOfChain("c1") != nil {
		t.Fatal("a chain stoker started")
	}
}

// I3: the person's y gives the go a chain stoker's run waits for:
// stoker:grant names the stoker, then the entry.
func TestGrantGivesAChainStokersGo(t *testing.T) {
	s, u, _ := familyTest(t)
	s.drive(u, "%3")
	s.Hook(hooks.Event{Name: "PreToolUse", SessionID: "s3", Pane: "%3", At: night0, Command: "gh pr merge 4", Guards: stoker.Guards("gh pr merge 4"), Denied: true})
	run := u.store.View().Run
	i := slices.IndexFunc(run, func(e stoker.Entry) bool { return e.Rule == stoker.RuleGo && e.State == stoker.StepWaits })
	if i < 0 {
		t.Fatalf("no go waits on the chain's card: %q", cardTexts(u.store))
	}
	id := run[i].ID
	if res := s.act(t.Context(), grantAct(id)); res.Err == "" {
		t.Fatal("the host's grant gave the chain's go")
	}
	if res := s.act(t.Context(), proto.Act{RecID: "stoker:grant", Confirm: "y", Text: "s1 " + id, From: "%5@mbtest", FromPlaced: true}); res.Err != "the stoker does not give itself the go" {
		t.Fatalf("the chain stoker's own grant: %q", res.Err)
	}
	if res := s.act(t.Context(), grantAct("s9 "+id)); res.Err != "no stoker s9: matchblox status lists the family" {
		t.Fatalf("an unknown stoker: %q", res.Err)
	}
	mustAct(t, s, grantAct("s1 "+id))
	if why := stoker.DenyFamily(s.Stoker.Dir(), "", "%3", stoker.Guards("gh pr merge 4")); why != "" {
		t.Fatalf("the given go is denied: %q", why)
	}
	if last := stoker.LastGo(u.store.View().Run, run[i]); last.State != stoker.StepDone {
		t.Fatalf("the go's last state %+v", last)
	}
}

// I4: an ended chain stoker's card stays until the person acks the host's
// card after its end, then it leaves State.Stokers.
func TestAnEndedUnitLeavesOnceTheCardIsSeen(t *testing.T) {
	s, _, now := familySetup(t)
	u, err := s.startChainStoker(t.Context(), "api-auth", "", nil, "person")
	if err != nil {
		t.Fatal(err)
	}
	chainStokerUp(s)
	mustAct(t, s, consoleStoker("ack")) // a look before the end
	*now = now.Add(time.Minute)
	s.retire(t.Context(), u, "its chain ended", "its chain ended")
	s.refreshStoker()
	if got := s.State().Stokers; len(got) != 1 || got[0].ID != "s1" {
		t.Fatalf("an ended unit no one saw: %+v", got)
	}
	*now = now.Add(time.Minute)
	mustAct(t, s, consoleStoker("ack"))
	if got := s.State().Stokers; got != nil {
		t.Fatalf("an ended unit after the look: %+v", got)
	}
}

// M4: a stoker's own pane asks, leaves or turns off no chain.
func TestAStokerJoinsNoChain(t *testing.T) {
	s, _, _ := familyTest(t)
	if err := s.Stoker.Set(stoker.Mode{}.TurnOn(night0)); err != nil {
		t.Fatal(err)
	}
	for _, from := range []string{hostPane, "%5@mbtest"} {
		for _, arg := range []string{"%9", "-", "off", "on"} {
			if res := s.act(t.Context(), proto.Act{RecID: proto.ChainPrefix + arg, From: from, FromPlaced: true}); res.Err != "a stoker joins no chain" {
				t.Errorf("%s chain:%s: %+v", from, arg, res)
			}
		}
	}
	s.mu.Lock()
	n := len(s.Chains.Requests)
	s.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d asks", n)
	}
}
