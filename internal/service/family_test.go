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
	if got := s.grantsOf(u); len(got) != 0 {
		t.Fatalf("the unit's goes after the night %q", got)
	}
	if got := drivenGoes(t, u, "%3"); len(got) != 0 {
		t.Fatalf("after the night %%3 has %q", got)
	}
	if why := stoker.DenyFamily(s.Stoker.Dir(), "", "%3", stoker.Guards("gh pr merge 4")); why != stoker.DenyNoGo {
		t.Fatalf("a merge after the night: %q", why)
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
